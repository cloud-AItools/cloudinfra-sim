// Package gateway는 내장 HTTPS Gateway다: TLS, SNI·Host 라우팅, 상관 ID·로그·마스킹, 호출 기록, 미지원 API 집계,
// 교차 시스템 락 진입점. 관련: NFR-ARC-0301·0304·0305·0307, NFR-0302·0307·0313, NFR-0401~0405, FR-API-0403·0404, FR-BE-0304.
package gateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloudinfra-sim/internal/fault"
	"cloudinfra-sim/internal/profile"
	"cloudinfra-sim/internal/store"
)

// Route는 SNI 서비스 접두어(vcenter, nsx, esx)와 처리기의 연결이다.
type Route struct {
	Service string // "vcenter", "nsx", "esx"(esx-NN 전체)
	Handler http.Handler
}

type Gateway struct {
	Domain string
	TLS    *tls.Config
	// MaxTLS는 서비스(리스너)별 TLS 최대 버전이다(버전 프로파일 tls.max_version, R-45 ③). 없으면 TLS 설정 그대로.
	MaxTLS map[string]uint16
	Log    *slog.Logger
	Trace  TraceSink // lite: 메모리 링버퍼, full: TraceDB(본문 포함)
	// Bodies는 Trace에 요청·응답 본문을 남길지다(full 실행 프로필, NFR-0504). 본문은 64 KB에서 자르고 자격 증명을 가린다.
	Bodies     bool
	Unsup      *Unsupported
	Lock       *CrossLock
	Faults     *fault.Store           // 전송 계층 Fault Rule(WP 3.8). nil이면 적용 안 함
	Audit      func(store.AuditEvent) // 감사 기록 저장(NFR-0306). nil이면 기록 안 함
	Principals *Principals            // 세션 → 사용자(감사용)
	// ExpireSession은 세션 계층 Fault(expire)를 적용한다. kind: soap | rest | nsx, id: 세션 식별자.
	ExpireSession func(kind, id string)
	// Now는 가상 시각(Fault 조건의 시각 범위). nil이면 실제 시간.
	Now func() time.Time

	routes  map[string]http.Handler
	servers []*http.Server
	wg      sync.WaitGroup
}

// IdleTimeout은 요청 사이 유휴 연결을 유지하는 최대 시간이다(가정: 실제 제품 값 미확인, 일반 프록시 수준).
var IdleTimeout = 2 * time.Minute

func New(domain string, cert tls.Certificate, log *slog.Logger, traceLimit int) *Gateway {
	return &Gateway{
		Domain:     domain,
		TLS:        &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		Log:        log,
		Trace:      NewTrace(traceLimit),
		Unsup:      NewUnsupported(1000),
		Lock:       &CrossLock{},
		Faults:     fault.NewStore(),
		Principals: NewPrincipals(10000),
		routes:     map[string]http.Handler{},
	}
}

func (g *Gateway) Handle(service string, h http.Handler) { g.routes[service] = h }

// Listen은 주소 하나에 TLS 리스너를 연다. 바인딩 실패는 바로 오류로 돌려준다(포트 충돌 진단, NFR-ARC-0306).
// service는 이 리스너의 기본 서비스다: SNI·Host가 IP 주소일 때(이름 없이 IP로 접속) 이 서비스로 보낸다.
// 실제 vCenter·NSX도 IP로 접속하면 응답하므로 hosts 등록 없이 govc·pyVmomi 같은 범용 SDK로 접속할 수 있다.
func (g *Gateway) Listen(addr, service string) (net.Addr, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	cfg := g.TLS
	if v := g.MaxTLS[service]; v != 0 {
		cfg = g.TLS.Clone()
		cfg.MaxVersion = v
	}
	srv := &http.Server{
		Handler: g,
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context {
			return context.WithValue(ctx, listenerKey{}, service)
		},
		TLSConfig:         cfg,
		ReadHeaderTimeout: 30 * time.Second,
		// 유휴 keep-alive 연결 정리(R-45 ④): 없으면 닫지 않고 떠난 클라이언트의 연결·고루틴·버퍼가 프로세스 수명 내내 남는다.
		IdleTimeout: IdleTimeout,
		ErrorLog:    slog.NewLogLogger(g.Log.Handler(), slog.LevelDebug),
	}
	g.servers = append(g.servers, srv)
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			g.Log.Error("gateway listener stopped", "addr", addr, "err", err)
		}
	}()
	return ln.Addr(), nil
}

func (g *Gateway) Shutdown(ctx context.Context) {
	for _, s := range g.servers {
		_ = s.Shutdown(ctx)
	}
	g.wg.Wait()
}

// service는 SNI(없으면 Host)에서 서비스 이름을 고른다. esx-NN은 vCenter 엔진으로 보낸다(FR-VC-0211).
func (g *Gateway) service(r *http.Request) (string, string) {
	name := ""
	if r.TLS != nil {
		name = r.TLS.ServerName
	}
	if name == "" {
		name, _, _ = strings.Cut(r.Host, ":")
	}
	name = strings.ToLower(name)
	if net.ParseIP(strings.Trim(name, "[]")) != nil {
		if svc, ok := r.Context().Value(listenerKey{}).(string); ok && svc != "" {
			return svc, name
		}
	}
	svc, ok := strings.CutSuffix(name, "."+g.Domain)
	if !ok {
		return "", name
	}
	if strings.HasPrefix(svc, "esx-") {
		return "esx", name
	}
	return svc, name
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	cid := r.Header.Get("X-Request-ID")
	if cid == "" {
		cid = newCorrelationID()
	}
	w.Header().Set("X-Request-ID", cid)
	svc, host := g.service(r)
	h, ok := g.routes[svc]
	if !ok {
		http.Error(w, "unknown virtual host: "+host, http.StatusMisdirectedRequest)
		g.record(r, cid, host, "", "", http.StatusMisdirectedRequest, start)
		return
	}

	// SOAP 메서드 이름(로그·Trace·락 판정용). lite는 본문을 저장하지 않는다(NFR-0504).
	soapMethod, formUser := "", ""
	var soapBody []byte
	var capture *traceCapture
	if g.Bodies {
		capture = &traceCapture{}
		r = r.WithContext(context.WithValue(r.Context(), captureKey{}, capture))
	}
	if (svc == "vcenter" || svc == "esx") && r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sdk") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		soapMethod, soapBody = profile.SOAPMethod(body), body
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	if svc == "nsx" && r.URL.Path == "/api/session/create" && r.Method == http.MethodPost {
		// 감사용 사용자 이름만 먼저 읽고 본문은 되돌린다(비밀번호는 보관하지 않음)
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		r.Body = io.NopCloser(bytes.NewReader(b))
		if v, err := url.ParseQuery(string(b)); err == nil {
			formUser = v.Get("j_username")
		}
	}
	if capture != nil {
		if soapBody != nil {
			capture.req = soapBody
		} else if r.Body != nil && r.Body != http.NoBody {
			b, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
			capture.req = b
			r.Body = io.NopCloser(bytes.NewReader(b))
		}
		p := r.URL.Path
		capture.session = r.Method == http.MethodPost && (p == "/api/session" || p == "/rest/com/vmware/cis/session")
	}
	r.Header.Set("X-Request-ID", cid) // 엔진이 내부 변경의 원인을 이 요청에 연결한다(FR-BE-0209)
	sessionVersion := ""
	if soapMethod != "" {
		sessionVersion = strings.TrimPrefix(strings.Trim(r.Header.Get("SOAPAction"), `"`), "urn:vim25/")
	}

	// 전송 계층 Fault Injection(FR-FLT-0117·0120·0123·0125). 규칙이 없으면 아무것도 하지 않는다(FR-FLT-0207).
	faultID := ""
	if g.Faults != nil {
		q := fault.Request{Product: svc, Method: r.Method, Path: r.URL.Path, SOAPMethod: soapMethod, User: g.requestUser(svc, r, soapMethod, soapBody, formUser), Now: g.now()}
		g.Faults.Observe(q)
		// 세션 계층(FR-FLT-0203 '특정 사용자 Session 만료'): 이 요청의 세션을 만료시킨 뒤 전달한다 → 엔진이 인증 실패로 응답
		if rule, ok := g.Faults.Apply(fault.LayerSession, q); ok && g.ExpireSession != nil {
			if kind, id := sessionOf(svc, r, soapMethod); id != "" {
				g.ExpireSession(kind, id)
				faultID = rule.ID
				g.Log.Warn("fault injected", "cid", cid, "rule", rule.ID, "layer", rule.Layer, "action", rule.Action.Type, "user", q.User, "session_kind", kind)
			}
		}
		if rule, ok := g.Faults.Apply(fault.LayerTransport, q); ok {
			faultID = rule.ID
			// NFR-0403 Fault Injection Log
			g.Log.Warn("fault injected", "cid", cid, "rule", rule.ID, "layer", rule.Layer, "action", rule.Action.Type,
				"status", rule.Action.Status, "delay_ms", rule.Action.DelayMS, "hit", rule.Hits, "count", rule.Count, "path", normalize(r.URL.Path), "soap", soapMethod)
			switch rule.Action.Type {
			case fault.ActionDelay:
				select {
				case <-time.After(time.Duration(rule.Action.DelayMS) * time.Millisecond):
				case <-r.Context().Done():
					g.recordFault(r, cid, host, soapMethod, sessionVersion, 499, start, faultID)
					return
				}
			case fault.ActionTimeout, fault.ActionReset:
				// HTTP Timeout(FR-FLT-0118): 응답 없이 붙잡았다가 끊는다. Connection Reset(FR-FLT-0119): 바로 끊는다.
				if rule.Action.Type == fault.ActionTimeout {
					select {
					case <-time.After(time.Duration(rule.Action.DelayMS) * time.Millisecond):
					case <-r.Context().Done():
					}
				}
				g.recordFault(r, cid, host, soapMethod, sessionVersion, 0, start, faultID)
				abortConnection(w)
				return
			default:
				writeFaultResponse(w, svc, r, rule)
				g.recordFault(r, cid, host, soapMethod, sessionVersion, rule.Action.Status, start, faultID)
				return
			}
		}
	}
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK, capture: capture}
	if g.mutating(svc, r, soapMethod) {
		g.Lock.Lock()
		h.ServeHTTP(rec, r)
		g.Lock.Unlock()
	} else {
		h.ServeHTTP(rec, r)
	}
	g.recordFault(r, cid, host, soapMethod, sessionVersion, rec.status, start, faultID)
	g.audit(svc, cid, r, soapMethod, soapBody, rec, formUser)
	if rec.status == http.StatusNotFound || rec.status == http.StatusMethodNotAllowed || rec.status == http.StatusNotImplemented || rec.soapUnsupported() {
		key := r.Method + " " + host + normalize(r.URL.Path)
		if soapMethod != "" {
			key = "SOAP " + soapMethod
		}
		g.Unsup.Add(key, sessionVersion)
		// NFR-0403 Unsupported API Log
		g.Log.Warn("unsupported api", "cid", cid, "key", key, "session_version", sessionVersion, "status", rec.status)
	} else if rec.status >= http.StatusInternalServerError && soapMethod == "" {
		// NFR-0403 Error Log(SOAP Fault는 500이 정상 계약이므로 제외)
		g.Log.Error("api error", "cid", cid, "host", host, "method", r.Method, "path", normalize(r.URL.Path), "status", rec.status)
	}
}

// mutating은 교차 락이 필요한 요청인지 판정한다. 장시간 대기하는 조회(WaitForUpdatesEx 등)는 제외한다(R-15).
func (g *Gateway) mutating(svc string, r *http.Request, soapMethod string) bool {
	switch svc {
	case "nsx":
		return r.Method != http.MethodGet && r.Method != http.MethodHead
	case "vcenter", "esx":
		return strings.HasSuffix(soapMethod, "_Task")
	}
	return false
}

func (g *Gateway) record(r *http.Request, cid, host, soapMethod, ver string, status int, start time.Time) {
	g.recordFault(r, cid, host, soapMethod, ver, status, start, "")
}

func (g *Gateway) recordFault(r *http.Request, cid, host, soapMethod, ver string, status int, start time.Time, faultID string) {
	d := time.Since(start)
	e := TraceEntry{
		Time: start, CorrelationID: cid, Host: host, Method: r.Method, Path: normalize(r.URL.Path),
		SOAPMethod: soapMethod, SessionVersion: ver, Status: status, DurationMS: d.Milliseconds(), FaultRule: faultID,
	}
	if c, ok := r.Context().Value(captureKey{}).(*traceCapture); ok {
		var t1, t2 bool
		e.RequestBody, t1 = maskBody(c.req, false)
		e.ResponseBody, t2 = maskBody(c.resp, c.session && status/100 == 2)
		e.BodyTruncated = t1 || t2 || c.respOver
	}
	g.Trace.Add(e)
	g.Log.Info("api request", "cid", cid, "host", host, "method", r.Method, "path", e.Path,
		"soap", soapMethod, "status", status, "duration_ms", e.DurationMS, "headers", maskHeaders(r.Header))
}

// normalize는 쿼리를 떼고 경로만 남긴다. 쿼리 값에는 자격 증명이 들어갈 수 있으므로 기록하지 않는다.
func normalize(p string) string {
	if len(p) > 256 {
		return p[:256]
	}
	return p
}

var sensitive = map[string]bool{
	"authorization": true, "cookie": true, "set-cookie": true, "vmware-api-session-id": true,
	"x-xsrf-token": true, "x-request-id": false,
}

// maskHeaders는 민감 헤더 값을 가린다(NFR-0307).
func maskHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		lk := strings.ToLower(k)
		if lk == "user-agent" || lk == "soapaction" || lk == "content-type" {
			out[lk] = strings.Join(v, ",")
		} else if sensitive[lk] {
			out[lk] = "***"
		}
	}
	return out
}

func newCorrelationID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	head    []byte
	capture *traceCapture // full: 응답 본문(MaxTraceBody까지)
}

// traceCapture는 full Trace의 본문 수집이다(요청 문맥에 실어 recordFault까지 전달).
type traceCapture struct {
	req, resp []byte
	respOver  bool
	session   bool // vAPI 세션 생성(응답이 토큰)
}

type captureKey struct{}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if c := s.capture; c != nil {
		if room := MaxTraceBody - len(c.resp); room > 0 {
			n := room
			if n > len(b) {
				n = len(b)
			}
			c.resp = append(c.resp, b[:n]...)
			if n < len(b) {
				c.respOver = true
			}
		} else if len(b) > 0 {
			c.respOver = true
		}
	}
	if len(s.head) < 2048 {
		n := 2048 - len(s.head)
		if n > len(b) {
			n = len(b)
		}
		s.head = append(s.head, b[:n]...)
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// soapUnsupported는 SOAP 응답이 미지원 계열 Fault인지 본다.
func (s *statusRecorder) soapUnsupported() bool {
	return s.status == http.StatusInternalServerError &&
		(bytes.Contains(s.head, []byte("MethodNotFound")) || bytes.Contains(s.head, []byte("NotImplemented")) ||
			bytes.Contains(s.head, []byte("no vmomi type defined"))) // vcsim: 알 수 없는 메서드 → InvalidRequest
}

// CrossLock은 vCenter·NSX 교차 변경을 직렬화하는 단일 락이다(FR-BE-0304).
// 규칙(R-15): 진입점에서만 획득한다. vcsim 변경 핸들러에서는 획득하지 않고 Outbox만 쓴다.
type CrossLock struct{ mu sync.Mutex }

func (l *CrossLock) Lock()   { l.mu.Lock() }
func (l *CrossLock) Unlock() { l.mu.Unlock() }

type listenerKey struct{}

// writeFaultResponse는 전송 계층 Fault 응답을 활성 제품의 오류 계약 형식으로 쓴다(FR-FLT-0205).
// NSX: ApiError, vCenter REST(/api, /rest): vAPI 오류 구조, SOAP(/sdk): 프록시 수준 HTTP 오류(SOAP Fault 아님, 가정).
func writeFaultResponse(w http.ResponseWriter, svc string, r *http.Request, rule fault.Rule) {
	status := rule.Action.Status
	if rule.Action.Type == fault.ActionUnavailable && rule.Action.RetryAfterSec > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(rule.Action.RetryAfterSec))
	}
	msg := http.StatusText(status)
	switch {
	case svc == "nsx":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": status, "error_message": msg, "module_name": "common-services"})
	case strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/rest/"):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error_type": vapiErrorType(status),
			"messages": []map[string]any{{"id": "cloudinfra-sim.fault", "default_message": msg, "args": []string{}}}})
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, msg+"\n")
	}
}

// vapiErrorType은 HTTP 상태에 대응하는 vAPI 표준 오류 형식이다(SRC-VAPI-SDK-7032 런타임 매핑의 역방향).
func vapiErrorType(status int) string {
	switch status {
	case 400:
		return "INVALID_ARGUMENT"
	case 401:
		return "UNAUTHENTICATED"
	case 403:
		return "UNAUTHORIZED"
	case 404:
		return "NOT_FOUND"
	case 409:
		return "CONCURRENT_CHANGE"
	case 429:
		return "RESOURCE_BUSY"
	case 503:
		return "SERVICE_UNAVAILABLE"
	case 504:
		return "TIMED_OUT"
	}
	return "ERROR"
}
