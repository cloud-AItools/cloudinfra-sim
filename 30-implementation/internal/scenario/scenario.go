// Package scenario는 선언형 시나리오 파일(YAML, data/scenarios)을 실행하는 Scenario Runner다
// (PRD-2 TR-SCN-0301 단계 구성, TR-SCN-0302 선언형 관리, TR-SCN-0303 장애 단계, TR-SCN-0106·0107 시작·중지,
// TR-SCN-0110 이벤트 조회, TR-SCN-0111 API Call Trace, NFR-0404 테스트 실행 로그).
//
// 단계: ① 초기 환경(Fixture 기준선 기록) → ② 로그인 → ③ API Workflow → ④ 중간 상태 조회 → ⑤ 정상 완료 또는 오류 확인
// (장애 시나리오: Fault 주입·가상 시간·Task 실패·Timeout·재시도·보상·복구) → ⑥ 상태 검증 → ⑦ 마스킹된 API Trace 메타데이터
// 저장 → ⑧ Test Report 생성 → ⑨ 환경 초기화(Admin reset 후 기준선 복귀 확인). ②~⑥은 시나리오 파일의 단계(stage)다.
//
// Runner는 시뮬레이터의 외부 인터페이스(Gateway 경유 SOAP·REST·NSX)와 Admin API만 쓴다. Phase 2b까지 40-test에 있던
// MVP Runner를 Phase 5(WP 5.1)에서 관리 API Runner(`/v1/scenarios`)와 공유하도록 옮겼다.
package scenario

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"go.yaml.in/yaml/v3"
)

// Env는 Runner가 시뮬레이터에 접근하는 방법이다(시험 코드와 Admin API Runner가 각각 구현).
type Env interface {
	Domain() string
	RootCAs() *x509.CertPool
	// DialContext는 vcenter.<도메인>·nsx.<도메인>을 실제 수신 주소로 연결한다(hosts 파일 대체).
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	Credentials(product string) (user, password string, err error)
	Admin(ctx context.Context, method, path string, body io.Reader) (int, []byte, error)
}

type File struct {
	Name         string   `yaml:"name" json:"name"`
	Description  string   `yaml:"description" json:"description"`
	Requirements []string `yaml:"requirements" json:"requirements"`
	Steps        []Step   `yaml:"steps" json:"-"`
	Path         string   `yaml:"-" json:"file"`
}

type Step struct {
	Name   string            `yaml:"name"`
	Stage  string            `yaml:"stage"` // login | workflow | inspect | error | fault | recover | verify
	SOAP   *SOAPStep         `yaml:"soap"`
	HTTP   *HTTPStep         `yaml:"http"`
	WaitMS int               `yaml:"wait_ms"`
	Expect Expect            `yaml:"expect"`
	Save   map[string]string `yaml:"save"`  // 변수 ← JSON 경로 또는 "result"(SOAP 결과 MoRef 값)
	Retry  *Retry            `yaml:"retry"` // 기대 결과가 나올 때까지 반복(재시도·Realization 대기, TR-SCN-0303)
}

type Retry struct {
	Attempts   int `yaml:"attempts"`
	IntervalMS int `yaml:"interval_ms"`
}

type SOAPStep struct {
	// login | logout | create_vm | reconfigure_vm | power_on | power_off | destroy_vm | get_vm | count_vms | find_vm
	Action   string `yaml:"action"`
	VM       string `yaml:"vm"`
	Name     string `yaml:"name"`
	CPUs     int32  `yaml:"cpus"`
	MemoryMB int64  `yaml:"memory_mb"`
}

type HTTPStep struct {
	Target  string            `yaml:"target"` // vcenter | nsx | admin
	Method  string            `yaml:"method"`
	Path    string            `yaml:"path"`
	Body    string            `yaml:"body"`
	Auth    string            `yaml:"auth"` // basic: 해당 제품 사용자
	Headers map[string]string `yaml:"headers"`
	// TimeoutMS는 이 요청의 클라이언트 Timeout이다(전송 계층 Timeout 주입 검증, FR-FLT-0118). 0이면 60초.
	TimeoutMS int `yaml:"timeout_ms"`
}

type Expect struct {
	Status     int            `yaml:"status"`
	Fault      string         `yaml:"fault"`       // SOAP Fault·Task error 타입(예: InvalidPowerState)
	EmptyBody  bool           `yaml:"empty_body"`  // HTTP 본문 없음
	JSON       map[string]any `yaml:"json"`        // 경로 → 기대값. 경로 끝 ".#"은 배열 길이
	PowerState string         `yaml:"power_state"` // get_vm
	CPUs       int32          `yaml:"cpus"`
	MemoryMB   int32          `yaml:"memory_mb"`
	Count      *int           `yaml:"count"` // count_vms(name이 있으면 그 이름의 VM 수)
	// TransportError는 HTTP 응답 대신 전송 오류를 기대한다: timeout | reset(연결 끊김) | any.
	TransportError string `yaml:"transport_error"`
}

// StepResult·Report는 ⑧ Test Report의 내용이다.
type StepResult struct {
	Name       string `json:"name"`
	Stage      string `json:"stage"`
	Status     string `json:"status"` // pass | fail | skipped
	Detail     string `json:"detail,omitempty"`
	Attempts   int    `json:"attempts,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

type Report struct {
	Scenario      string       `json:"scenario"`
	Description   string       `json:"description"`
	Requirements  []string     `json:"requirements"`
	Started       time.Time    `json:"started"`
	DurationMS    int64        `json:"duration_ms"`
	Status        string       `json:"status"` // passed | failed | stopped
	Passed        bool         `json:"passed"`
	Steps         []StepResult `json:"steps"`
	TraceFile     string       `json:"trace_file,omitempty"`
	TraceEntries  int          `json:"trace_entries"`
	ResetVerified bool         `json:"reset_verified"`
	Tested        string       `json:"verification_cap"` // 검증 상한 표시(TR-COV-0409)
}

// Options는 실행 선택 사항이다.
type Options struct {
	ReportDir string            // 리포트·Trace·로그 저장 위치(필수)
	Log       func(line string) // 실행 로그 줄(NFR-0404). nil이면 버린다
}

// Load는 시나리오 파일을 읽고 검사한다(알 수 없는 키 거부).
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Name == "" || len(f.Steps) == 0 {
		return nil, fmt.Errorf("%s: name과 steps가 필요하다", path)
	}
	for i, s := range f.Steps {
		n := 0
		for _, set := range []bool{s.SOAP != nil, s.HTTP != nil, s.WaitMS > 0} {
			if set {
				n++
			}
		}
		if n != 1 {
			return nil, fmt.Errorf("%s: step %d(%s)는 soap·http·wait_ms 중 하나만 가진다", path, i+1, s.Name)
		}
	}
	f.Path = path
	return &f, nil
}

// LoadDir는 디렉터리의 시나리오를 이름순으로 읽는다. 이름이 겹치면 오류다.
func LoadDir(dir string) ([]*File, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	seen := map[string]string{}
	var out []*File
	for _, p := range paths {
		f, err := Load(p)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[f.Name]; dup {
			return nil, fmt.Errorf("시나리오 이름 %s 중복: %s, %s", f.Name, prev, p)
		}
		seen[f.Name] = p
		out = append(out, f)
	}
	return out, nil
}

type runner struct {
	env    Env
	ctx    context.Context
	vars   map[string]string
	vc     *govmomi.Client
	client *http.Client
	logf   func(format string, args ...any)
}

var varRe = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

func (r *runner) sub(s string) string {
	return varRe.ReplaceAllStringFunc(s, func(m string) string { return r.vars[m[2:len(m)-1]] })
}

// Run은 시나리오를 실행하고 리포트를 opts.ReportDir에 쓴다(⑦·⑧). 마지막에 환경을 초기화하고 확인한다(⑨).
// ctx가 취소되면(Scenario Stop, TR-SCN-0107) 진행 중 단계를 중단하고 남은 단계는 skipped로 기록한 뒤 ⑦~⑨를 수행한다.
func Run(ctx context.Context, env Env, f *File, opts Options) *Report {
	var logMu sync.Mutex
	logf := func(format string, args ...any) {
		if opts.Log == nil {
			return
		}
		logMu.Lock()
		defer logMu.Unlock()
		opts.Log(time.Now().UTC().Format("2006-01-02T15:04:05.000Z") + " " + fmt.Sprintf(format, args...))
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: env.RootCAs()}, DialContext: env.DialContext}
	defer tr.CloseIdleConnections()
	r := &runner{env: env, ctx: ctx, vars: map[string]string{}, client: &http.Client{Transport: tr, Timeout: 60 * time.Second}, logf: logf}
	rep := &Report{Scenario: f.Name, Description: f.Description, Requirements: f.Requirements, Started: time.Now().UTC(),
		Tested: "Integration Tested — 승인 범위 자체 테스트, CMP·실환경 미검증(TR-COV-0409)"}
	logf("scenario %s start (%d steps)", f.Name, len(f.Steps))
	// 기본 변수: 제품 사용자 이름(비밀번호는 변수로 노출하지 않는다)
	for _, p := range []string{"vcenter", "nsx"} {
		if u, _, err := env.Credentials(p); err == nil {
			r.vars[p+"_user"] = u
		}
	}
	// 단계 ⑦~⑨는 시나리오가 중지돼도 수행한다(정리 작업은 취소와 무관한 문맥).
	cleanup := context.WithoutCancel(ctx)
	// ① 초기 환경: Fixture 기준선
	baseVMs, baseSegs := r.baseline(cleanup)
	rep.Steps = append(rep.Steps, StepResult{Name: fmt.Sprintf("fixture baseline: %d VMs, %d segments", baseVMs, baseSegs), Stage: "setup", Status: "pass"})
	logf("[setup] baseline vms=%d segments=%d", baseVMs, baseSegs)
	rep.Passed, rep.Status = true, "passed"
	for i, s := range f.Steps {
		if ctx.Err() != nil {
			for _, rest := range f.Steps[i:] {
				rep.Steps = append(rep.Steps, StepResult{Name: rest.Name, Stage: rest.Stage, Status: "skipped", Detail: "scenario stopped"})
			}
			rep.Passed, rep.Status = false, "stopped"
			logf("scenario stopped before step %d", i+1)
			break
		}
		t0 := time.Now()
		attempts, err := r.stepWithRetry(s)
		res := StepResult{Name: s.Name, Stage: s.Stage, Status: "pass", Attempts: attempts, DurationMS: time.Since(t0).Milliseconds()}
		if err != nil {
			res.Status, res.Detail = "fail", err.Error()
			if ctx.Err() != nil {
				res.Status, res.Detail = "skipped", "scenario stopped: "+err.Error()
			}
		}
		rep.Steps = append(rep.Steps, res)
		logf("[%s] %s %s (%d ms, attempts %d) %s", s.Stage, res.Status, s.Name, res.DurationMS, attempts, res.Detail)
		if err != nil {
			rep.Passed, rep.Status = false, "failed"
			if ctx.Err() != nil {
				rep.Status = "stopped"
				for _, rest := range f.Steps[i+1:] {
					rep.Steps = append(rep.Steps, StepResult{Name: rest.Name, Stage: rest.Stage, Status: "skipped", Detail: "scenario stopped"})
				}
			}
			break
		}
	}
	if r.vc != nil {
		_ = r.vc.Logout(cleanup)
	}
	r.ctx = cleanup
	// ⑦ 마스킹된 Trace 메타데이터 저장
	trace, n := r.trace()
	rep.TraceFile = filepath.Join(opts.ReportDir, f.Name+"-trace.json")
	rep.TraceEntries = n
	if err := os.WriteFile(rep.TraceFile, trace, 0o644); err != nil {
		rep.Passed, rep.Status = false, "failed"
		rep.Steps = append(rep.Steps, StepResult{Name: "save trace", Stage: "report", Status: "fail", Detail: err.Error()})
	}
	for _, p := range []string{"vcenter", "nsx"} {
		if _, pw, err := env.Credentials(p); err == nil && pw != "" && bytes.Contains(trace, []byte(pw)) {
			rep.Passed, rep.Status = false, "failed"
			rep.Steps = append(rep.Steps, StepResult{Name: "trace masking", Stage: "report", Status: "fail", Detail: "trace contains a credential"})
		}
	}
	// ⑨ 초기화
	_, _, _ = env.Admin(cleanup, http.MethodPost, "/v1/reset", nil)
	vms, segs := r.baseline(cleanup)
	rep.ResetVerified = vms == baseVMs && segs == baseSegs
	st := StepResult{Name: fmt.Sprintf("reset: %d VMs, %d segments", vms, segs), Stage: "reset", Status: "pass"}
	if !rep.ResetVerified {
		st.Status, rep.Passed = "fail", false
		if rep.Status == "passed" {
			rep.Status = "failed"
		}
	}
	rep.Steps = append(rep.Steps, st)
	logf("[reset] %s %s", st.Status, st.Name)
	rep.DurationMS = time.Since(rep.Started).Milliseconds()
	// ⑧ 리포트
	j, _ := json.MarshalIndent(rep, "", "  ")
	_ = os.WriteFile(filepath.Join(opts.ReportDir, f.Name+"-report.json"), j, 0o644)
	_ = os.WriteFile(filepath.Join(opts.ReportDir, f.Name+"-report.md"), []byte(Markdown(rep)), 0o644)
	logf("scenario %s %s (%d ms)", f.Name, rep.Status, rep.DurationMS)
	return rep
}

// Markdown은 리포트의 사람용 형식이다.
func Markdown(rep *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Scenario %s — %s\n\n%s\n\n- 시작: %s, 소요: %d ms\n- 요구사항: %s\n- Trace: `%s` (%d건, 메타데이터만)\n- 초기화 확인: %v\n- 검증 상한: %s\n\n",
		rep.Scenario, strings.ToUpper(rep.Status), rep.Description, rep.Started.Format(time.RFC3339), rep.DurationMS, strings.Join(rep.Requirements, ", "),
		filepath.Base(rep.TraceFile), rep.TraceEntries, rep.ResetVerified, rep.Tested)
	b.WriteString("| # | 단계 | 이름 | 결과 | ms | 상세 |\n|---|---|---|---|---|---|\n")
	for i, s := range rep.Steps {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d | %s |\n", i+1, s.Stage, s.Name, s.Status, s.DurationMS, strings.ReplaceAll(s.Detail, "|", "\\|"))
	}
	return b.String()
}

func (r *runner) baseline(ctx context.Context) (vms, segs int) {
	saved := r.ctx
	r.ctx = ctx
	defer func() { r.ctx = saved }()
	if c, err := r.login(); err == nil {
		if n, err := countVMs(ctx, c, ""); err == nil {
			vms = n
		}
		_ = c.Logout(ctx)
	}
	code, body, _ := r.http(&HTTPStep{Target: "nsx", Method: http.MethodGet, Path: "/policy/api/v1/infra/segments", Auth: "basic"})
	if code == 200 {
		var o struct {
			ResultCount int `json:"result_count"`
		}
		_ = json.Unmarshal(body, &o)
		segs = o.ResultCount
	}
	return
}

func (r *runner) trace() ([]byte, int) {
	code, b, err := r.env.Admin(r.ctx, http.MethodGet, "/v1/trace?limit=1000", nil)
	if err != nil || code != 200 {
		return []byte("{}"), 0
	}
	var t struct {
		Entries []json.RawMessage `json:"entries"`
	}
	_ = json.Unmarshal(b, &t)
	return b, len(t.Entries)
}

// stepWithRetry는 retry가 있으면 실패한 단계를 간격을 두고 다시 실행한다(재시도·대기 검증).
func (r *runner) stepWithRetry(s Step) (int, error) {
	attempts := 1
	if s.Retry != nil && s.Retry.Attempts > 1 {
		attempts = s.Retry.Attempts
	}
	var err error
	for i := 1; i <= attempts; i++ {
		if err = r.step(s); err == nil {
			return i, nil
		}
		if i < attempts {
			if werr := r.sleep(time.Duration(s.Retry.IntervalMS) * time.Millisecond); werr != nil {
				return i, werr
			}
		}
	}
	return attempts, err
}

func (r *runner) sleep(d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

func (r *runner) step(s Step) error {
	switch {
	case s.SOAP != nil:
		return r.soap(s)
	case s.HTTP != nil:
		code, body, err := r.http(s.HTTP)
		if want := s.Expect.TransportError; want != "" {
			if r.ctx.Err() != nil {
				return r.ctx.Err()
			}
			if err == nil {
				return fmt.Errorf("want transport error %s, got HTTP %d", want, code)
			}
			timeout := strings.Contains(err.Error(), "Timeout") || strings.Contains(err.Error(), "deadline")
			if (want == "timeout" && !timeout) || (want == "reset" && timeout) {
				return fmt.Errorf("transport error %v, want %s", err, want)
			}
			return nil
		}
		if err != nil {
			return err
		}
		return r.checkHTTP(s, code, body)
	case s.WaitMS > 0:
		return r.sleep(time.Duration(s.WaitMS) * time.Millisecond)
	}
	return fmt.Errorf("step %q: no action", s.Name)
}

func (r *runner) http(h *HTTPStep) (int, []byte, error) {
	var product string
	switch h.Target {
	case "nsx", "vcenter":
		product = h.Target
	case "admin":
		return r.env.Admin(r.ctx, h.Method, r.sub(h.Path), strings.NewReader(r.sub(h.Body)))
	default:
		return 0, nil, fmt.Errorf("unknown target %q", h.Target)
	}
	req, err := http.NewRequestWithContext(r.ctx, h.Method, "https://"+product+"."+r.env.Domain()+r.sub(h.Path), strings.NewReader(r.sub(h.Body)))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.Auth == "basic" {
		u, p, err := r.env.Credentials(product)
		if err != nil {
			return 0, nil, err
		}
		req.SetBasicAuth(u, p)
	}
	for k, v := range h.Headers {
		req.Header.Set(k, r.sub(v))
	}
	client := r.client
	if h.TimeoutMS > 0 {
		client = &http.Client{Transport: r.client.Transport, Timeout: time.Duration(h.TimeoutMS) * time.Millisecond}
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b, nil
}

func (r *runner) checkHTTP(s Step, code int, body []byte) error {
	if s.Expect.Status != 0 && code != s.Expect.Status {
		return fmt.Errorf("status %d, want %d: %.200s", code, s.Expect.Status, body)
	}
	if s.Expect.EmptyBody && len(bytes.TrimSpace(body)) != 0 {
		return fmt.Errorf("want empty body, got %.200s", body)
	}
	var doc any
	if len(s.Expect.JSON) > 0 || len(s.Save) > 0 {
		if err := json.Unmarshal(body, &doc); err != nil {
			return fmt.Errorf("invalid JSON: %v", err)
		}
	}
	keys := make([]string, 0, len(s.Expect.JSON))
	for p := range s.Expect.JSON {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		want := s.Expect.JSON[p]
		got, ok := jsonPath(doc, r.sub(p))
		if ws, isStr := want.(string); isStr {
			want = r.sub(ws)
		}
		if !ok || fmt.Sprint(got) != fmt.Sprint(want) {
			return fmt.Errorf("%s = %v, want %v", p, got, want)
		}
	}
	for v, p := range s.Save {
		got, ok := jsonPath(doc, p)
		if !ok {
			return fmt.Errorf("save %s: path %s missing", v, p)
		}
		r.vars[v] = fmt.Sprint(got)
	}
	return nil
}

// jsonPath는 a.b.0.c 형식 경로 값을 찾는다. 끝의 "#"은 배열 길이, "."만 있으면 문서 전체.
func jsonPath(doc any, p string) (any, bool) {
	if p == "." {
		return doc, true
	}
	cur := doc
	for _, seg := range strings.Split(p, ".") {
		switch x := cur.(type) {
		case map[string]any:
			v, ok := x[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			if seg == "#" {
				return len(x), true
			}
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			cur = x[i]
		default:
			return nil, false
		}
	}
	if f, ok := cur.(float64); ok && f == float64(int64(f)) {
		return int64(f), true
	}
	return cur, true
}

// login은 vCenter 사용자로 로그인한 govmomi 클라이언트를 만든다(Gateway 경유, SNI = vcenter.<도메인>).
func (r *runner) login() (*govmomi.Client, error) {
	u, _ := url.Parse("https://vcenter." + r.env.Domain() + "/sdk")
	sc := soap.NewClient(u, false)
	tr := sc.DefaultTransport()
	tr.TLSClientConfig = &tls.Config{RootCAs: r.env.RootCAs()}
	tr.DialTLSContext = nil
	tr.DialContext = r.env.DialContext
	vc, err := vim25.NewClient(r.ctx, sc)
	if err != nil {
		return nil, err
	}
	c := &govmomi.Client{Client: vc, SessionManager: session.NewManager(vc)}
	user, pw, err := r.env.Credentials("vcenter")
	if err != nil {
		return nil, err
	}
	if err := c.Login(r.ctx, url.UserPassword(user, pw)); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *runner) vm(name string) *object.VirtualMachine {
	return object.NewVirtualMachine(r.vc.Client, types.ManagedObjectReference{Type: "VirtualMachine", Value: r.sub(name)})
}

func (r *runner) soap(s Step) error {
	a := s.SOAP
	switch a.Action {
	case "login":
		c, err := r.login()
		if err != nil {
			return r.faultMatches(s, err)
		}
		r.vc = c
		return nil
	case "logout":
		if r.vc != nil {
			_ = r.vc.Logout(r.ctx)
			r.vc = nil
		}
		return nil
	}
	if r.vc == nil {
		return fmt.Errorf("soap %s before login", a.Action)
	}
	var tk *object.Task
	var err error
	switch a.Action {
	case "create_vm":
		f := find.NewFinder(r.vc.Client, true)
		dc, e1 := f.DefaultDatacenter(r.ctx)
		if e1 != nil {
			return r.faultMatches(s, e1)
		}
		f.SetDatacenter(dc)
		folders, _ := dc.Folders(r.ctx)
		pool, e2 := f.DefaultResourcePool(r.ctx)
		if e2 != nil {
			return e2
		}
		dss, _ := f.DatastoreList(r.ctx, "*")
		ds, _ := dss[0].ObjectName(r.ctx)
		tk, err = folders.VmFolder.CreateVM(r.ctx, types.VirtualMachineConfigSpec{Name: r.sub(a.Name), GuestId: "otherGuest64",
			NumCPUs: a.CPUs, MemoryMB: a.MemoryMB, Files: &types.VirtualMachineFileInfo{VmPathName: "[" + ds + "]"}}, pool, nil)
	case "reconfigure_vm":
		tk, err = r.vm(a.VM).Reconfigure(r.ctx, types.VirtualMachineConfigSpec{NumCPUs: a.CPUs, MemoryMB: a.MemoryMB})
	case "power_on":
		tk, err = r.vm(a.VM).PowerOn(r.ctx)
	case "power_off":
		tk, err = r.vm(a.VM).PowerOff(r.ctx)
	case "destroy_vm":
		tk, err = r.vm(a.VM).Destroy(r.ctx)
	case "get_vm":
		var vm mo.VirtualMachine
		err = r.vm(a.VM).Properties(r.ctx, r.vm(a.VM).Reference(), []string{"runtime.powerState", "config.hardware"}, &vm)
		if err == nil {
			if s.Expect.Fault != "" {
				return fmt.Errorf("want fault %s, got success", s.Expect.Fault)
			}
			if s.Expect.PowerState != "" && string(vm.Runtime.PowerState) != s.Expect.PowerState {
				return fmt.Errorf("powerState %s, want %s", vm.Runtime.PowerState, s.Expect.PowerState)
			}
			if s.Expect.CPUs != 0 && vm.Config.Hardware.NumCPU != s.Expect.CPUs {
				return fmt.Errorf("numCPU %d, want %d", vm.Config.Hardware.NumCPU, s.Expect.CPUs)
			}
			if s.Expect.MemoryMB != 0 && vm.Config.Hardware.MemoryMB != s.Expect.MemoryMB {
				return fmt.Errorf("memoryMB %d, want %d", vm.Config.Hardware.MemoryMB, s.Expect.MemoryMB)
			}
			return nil
		}
		return r.faultMatches(s, err)
	case "count_vms":
		n, e := countVMs(r.ctx, r.vc, r.sub(a.Name))
		if e != nil {
			return r.faultMatches(s, e)
		}
		if s.Expect.Fault != "" {
			return fmt.Errorf("want fault %s, got success", s.Expect.Fault)
		}
		if s.Expect.Count != nil && n != *s.Expect.Count {
			return fmt.Errorf("VM count %d, want %d", n, *s.Expect.Count)
		}
		return nil
	case "find_vm":
		ref, e := findVM(r.ctx, r.vc, r.sub(a.Name))
		if e != nil {
			return r.faultMatches(s, e)
		}
		for v, p := range s.Save {
			if p == "result" {
				r.vars[v] = ref.Value
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown soap action %q", a.Action)
	}
	if err != nil {
		return r.faultMatches(s, err)
	}
	info, err := tk.WaitForResult(r.ctx, nil)
	if err != nil {
		return r.faultMatches(s, err)
	}
	if s.Expect.Fault != "" {
		return fmt.Errorf("want fault %s, got success", s.Expect.Fault)
	}
	for v, p := range s.Save {
		if p == "result" {
			if ref, ok := info.Result.(types.ManagedObjectReference); ok {
				r.vars[v] = ref.Value
			}
		}
	}
	return nil
}

func (r *runner) faultMatches(s Step, err error) error {
	if r.ctx.Err() != nil {
		return r.ctx.Err()
	}
	name := ""
	var te task.Error
	switch {
	case errors.As(err, &te):
		name = fmt.Sprintf("%T", te.Fault())
	case soap.IsSoapFault(err):
		name = fmt.Sprintf("%T", soap.ToSoapFault(err).VimFault())
	default:
		return err
	}
	name = strings.TrimPrefix(strings.TrimPrefix(name, "*"), "types.")
	if s.Expect.Fault == "" {
		return fmt.Errorf("unexpected fault %s: %v", name, err)
	}
	if name != s.Expect.Fault {
		return fmt.Errorf("fault %s, want %s", name, s.Expect.Fault)
	}
	return nil
}

func countVMs(ctx context.Context, c *govmomi.Client, name string) (int, error) {
	v, err := view.NewManager(c.Client).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return 0, err
	}
	defer v.Destroy(ctx)
	if name == "" {
		refs, err := v.Find(ctx, []string{"VirtualMachine"}, nil)
		return len(refs), err
	}
	var vms []mo.VirtualMachine
	if err := v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"name"}, &vms); err != nil {
		return 0, err
	}
	n := 0
	for _, vm := range vms {
		if vm.Name == name {
			n++
		}
	}
	return n, nil
}

func findVM(ctx context.Context, c *govmomi.Client, name string) (types.ManagedObjectReference, error) {
	f := find.NewFinder(c.Client, true)
	dc, err := f.DefaultDatacenter(ctx)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}
	f.SetDatacenter(dc)
	vm, err := f.VirtualMachine(ctx, name)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}
	return vm.Reference(), nil
}
