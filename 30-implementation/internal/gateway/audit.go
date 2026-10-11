package gateway

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloudinfra-sim/internal/store"
)

// 감사 기록(NFR-0306)과 변경 원인 추적(FR-BE-0209)의 Gateway 부분.
//
// 기록 대상: 상태를 바꾸는 요청(NSX 비 GET, vCenter REST 비 GET, SOAP *_Task)과 인증 동작(SOAP Login·Logout,
// REST·NSX 세션 생성·삭제, 실패 포함). 사용자는 세션 식별자(SOAP 쿠키, REST 토큰, NSX JSESSIONID)·Basic 인증에서
// 찾는다. 비밀번호·세션 값은 기록하지 않는다(NFR-0307). SOAP *_Task는 응답의 Task ID를 대상에 남겨 작업 기록
// (simulation_operations)과 잇는다.

// Principals는 세션 식별자 → 사용자 이름이다(상한 있는 맵).
type Principals struct {
	mu    sync.Mutex
	m     map[string]string
	order []string
	limit int
}

func NewPrincipals(limit int) *Principals { return &Principals{m: map[string]string{}, limit: limit} }

func (p *Principals) put(id, user string) {
	if id == "" || user == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.m[id]; !ok {
		p.order = append(p.order, id)
		for len(p.order) > p.limit {
			delete(p.m, p.order[0])
			p.order = p.order[1:]
		}
	}
	p.m[id] = user
}

func (p *Principals) get(id string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.m[id]
}

var (
	soapUserRe = regexp.MustCompile(`<userName>([^<]*)</userName>`)
	soapThisRe = regexp.MustCompile(`<_this[^>]*type="([^"]+)"[^>]*>([^<]+)</_this>`)
	taskRefRe  = regexp.MustCompile(`<returnval type="Task">(task-[0-9]+)</returnval>`)
	restTokRe  = regexp.MustCompile(`^"([0-9a-zA-Z]+)"`)
)

var authMethods = map[string]bool{"Login": true, "Logout": true, "LoginByToken": true, "LoginExtensionByCertificate": true, "CloneSession": true, "TerminateSession": true}

func cookie(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

func setCookie(h http.Header, name string) string {
	for _, sc := range h.Values("Set-Cookie") {
		if v, ok := strings.CutPrefix(sc, name+"="); ok {
			v, _, _ = strings.Cut(v, ";")
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

func basicUser(r *http.Request) string {
	if u, _, ok := r.BasicAuth(); ok {
		return u
	}
	return ""
}

// auditable은 감사 대상 요청인가.
func auditable(svc string, r *http.Request, soapMethod string) bool {
	switch {
	case soapMethod != "":
		return strings.HasSuffix(soapMethod, "_Task") || authMethods[soapMethod]
	case svc == "nsx":
		return r.Method != http.MethodGet && r.Method != http.MethodHead
	case svc == "vcenter" && (strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/rest/")):
		return r.Method != http.MethodGet && r.Method != http.MethodHead
	}
	return false
}

// principal은 요청의 사용자다. 로그인 요청이면 요청 본문·응답에서 새 세션을 기억한다.
func (g *Gateway) principal(svc string, r *http.Request, soapMethod string, soapBody []byte, rec *statusRecorder, formUser string) string {
	p := g.Principals
	switch {
	case soapMethod == "Login":
		user := ""
		if m := soapUserRe.FindSubmatch(soapBody); m != nil {
			user = string(m[1])
		}
		if rec.status == http.StatusOK {
			p.put("soap:"+setCookie(rec.Header(), "vmware_soap_session"), user)
		}
		return user
	case soapMethod != "":
		return p.get("soap:" + cookie(r, "vmware_soap_session"))
	case svc == "vcenter":
		if u := basicUser(r); u != "" {
			if r.Method == http.MethodPost && strings.TrimSuffix(r.URL.Path, "/") == "/api/session" && rec.status/100 == 2 {
				if m := restTokRe.FindSubmatch(rec.head); m != nil {
					p.put("rest:"+string(m[1]), u)
				}
			}
			return u
		}
		return p.get("rest:" + r.Header.Get("vmware-api-session-id"))
	case svc == "nsx":
		if u := basicUser(r); u != "" {
			return u
		}
		if r.URL.Path == "/api/session/create" {
			u := formUser
			if rec.status == http.StatusOK {
				p.put("nsx:"+setCookie(rec.Header(), "JSESSIONID"), u)
			}
			return u
		}
		return p.get("nsx:" + cookie(r, "JSESSIONID"))
	}
	return ""
}

func (g *Gateway) audit(svc, cid string, r *http.Request, soapMethod string, soapBody []byte, rec *statusRecorder, formUser string) {
	if g.Audit == nil || !auditable(svc, r, soapMethod) && !(svc == "nsx" && strings.HasPrefix(r.URL.Path, "/api/session/")) {
		return
	}
	e := store.AuditEvent{CID: cid, Product: svc, Status: rec.status, Cause: "api"}
	e.Principal = g.principal(svc, r, soapMethod, soapBody, rec, formUser)
	if soapMethod != "" {
		e.Action = "SOAP " + soapMethod
		if m := soapThisRe.FindSubmatch(soapBody); m != nil {
			e.Target = string(m[1]) + ":" + string(m[2])
		}
		if m := taskRefRe.FindSubmatch(rec.head); m != nil {
			e.Target += " task=" + string(m[1])
		}
	} else {
		e.Action = r.Method + " " + normalize(r.URL.Path)
	}
	g.Audit(e)
}

// requestUser는 요청 전에 아는 사용자다(Fault 조건 match.user). 로그인 요청은 본문의 사용자 이름.
func (g *Gateway) requestUser(svc string, r *http.Request, soapMethod string, soapBody []byte, formUser string) string {
	switch {
	case soapMethod == "Login":
		if m := soapUserRe.FindSubmatch(soapBody); m != nil {
			return string(m[1])
		}
	case soapMethod != "":
		return g.Principals.get("soap:" + cookie(r, "vmware_soap_session"))
	case svc == "vcenter":
		if u := basicUser(r); u != "" {
			return u
		}
		return g.Principals.get("rest:" + r.Header.Get("vmware-api-session-id"))
	case svc == "nsx":
		if u := basicUser(r); u != "" {
			return u
		}
		if formUser != "" {
			return formUser
		}
		return g.Principals.get("nsx:" + cookie(r, "JSESSIONID"))
	}
	return ""
}

// sessionOf는 요청이 쓰는 세션 식별자다(세션 계층 Fault 대상).
func sessionOf(svc string, r *http.Request, soapMethod string) (kind, id string) {
	switch {
	case soapMethod != "":
		return "soap", cookie(r, "vmware_soap_session")
	case svc == "vcenter":
		return "rest", r.Header.Get("vmware-api-session-id")
	case svc == "nsx":
		return "nsx", cookie(r, "JSESSIONID")
	}
	return "", ""
}

func (g *Gateway) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// abortConnection은 응답 없이 연결(HTTP/2는 스트림)을 끊는다. net/http가 ErrAbortHandler를 조용히 처리한다.
func abortConnection(w http.ResponseWriter) {
	if hj, ok := w.(http.Hijacker); ok {
		if c, _, err := hj.Hijack(); err == nil {
			_ = c.Close()
			return
		}
	}
	panic(http.ErrAbortHandler)
}
