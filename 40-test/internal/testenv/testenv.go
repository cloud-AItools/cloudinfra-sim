// Package testenv는 자체 테스트용 시뮬레이터를 프로세스 안에서 띄우고 클라이언트를 만든다.
// 실제 실행 경로(Gateway·TLS·SNI)를 그대로 통과한다(TR-E2E 경계: 클라이언트 → Gateway → 엔진 → 상태).
package testenv

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/profile"
	"cloudinfra-sim/internal/sim"
)

const Domain = "cloudinfra-sim.test"

type Env struct {
	T      testing.TB
	Sim    *sim.Simulator
	Users  []sim.User
	Token  string
	Repo   string
	Client *http.Client // SNI·CA를 적용한 HTTPS 클라이언트
	Admin  *http.Client
}

// Repo는 저장소 루트다(40-test의 상위). SIM_TEST_REPO가 있으면 그 경로(다른 OS에서 미리 빌드한 시험 실행파일을
// 돌릴 때 — 예: Windows에서 GOOS=linux로 빌드해 WSL에서 /mnt/d/... 로 실행, 40-test/ansible/run-wsl.sh).
func Repo() string {
	if v := os.Getenv("SIM_TEST_REPO"); v != "" {
		return v
	}
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// VCenterVersion은 시험 대상 vCenter 프로파일이다. SIM_TEST_VCENTER_VERSION으로 추가 버전 프로파일(WP 4.7)을 고른다.
func VCenterVersion() string {
	if v := os.Getenv("SIM_TEST_VCENTER_VERSION"); v != "" {
		return v
	}
	return BaseVCenterVersion
}

// BaseVCenterVersion은 기준 프로파일(PRD-4 D1 첫 기준 조합)이다.
const BaseVCenterVersion = "7.0.3.02500"

// Start는 임시 데이터 디렉터리로 시뮬레이터를 띄운다. mutate로 설정을 바꿀 수 있다.
func Start(t testing.TB, mutate func(*config.Config)) *Env {
	t.Helper()
	return StartWithLogger(t, mutate, nil)
}

// StartWithLogger는 지정한 로거로 시뮬레이터를 띄운다(nil이면 버림, SIM_TEST_LOG가 있으면 stderr).
func StartWithLogger(t testing.TB, mutate func(*config.Config), log *slog.Logger) *Env {
	t.Helper()
	s, err := newSim(t, mutate, log)
	if err != nil {
		t.Fatal(err)
	}
	return started(t, s)
}

// New는 시뮬레이터 생성 오류를 돌려준다(기동 거부 시험용). 성공하면 Start와 같다.
func New(t testing.TB, mutate func(*config.Config)) (*Env, error) {
	t.Helper()
	s, err := newSim(t, mutate, nil)
	if err != nil {
		return nil, err
	}
	return started(t, s), nil
}

func newSim(t testing.TB, mutate func(*config.Config), log *slog.Logger) (*sim.Simulator, error) {
	repo := Repo()
	impl := filepath.Join(repo, "30-implementation")
	cfg, err := config.Parse([]string{
		"--vcenter-version", VCenterVersion(), "--nsx-version", "3.2.4.3",
		"--vcenter-addr", "127.0.0.2:0", "--nsx-addr", "127.0.0.3:0", "--admin-addr", "127.0.0.1:0",
		"--data-dir", t.TempDir(),
		"--profiles-dir", filepath.Join(impl, "data", "compatibility-profiles"),
		"--fixture", filepath.Join(impl, "data", "fixtures", "default-small.yaml"),
		"--scenarios-dir", filepath.Join(impl, "data", "scenarios"),
		"--verification-mode", "development",
	}, func(string) string { return "" })
	if err != nil {
		return nil, err
	}
	if mutate != nil {
		mutate(cfg)
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if os.Getenv("SIM_TEST_LOG") != "" {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return sim.New(cfg, log)
}

func started(t testing.TB, s *sim.Simulator) *Env {
	t.Helper()
	cfg := s.Cfg
	repo := Repo()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// 검증 모드(SIM_RELATIONS_VERIFY): 증분 관계 조정이 전체 계산과 달랐으면 실패(R-45)
		if n := s.Rel.Mismatches(); n > 0 {
			t.Errorf("incremental reconcile differed from full derive %d time(s)", n)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Shutdown(ctx)
	})
	e := &Env{T: t, Sim: s, Repo: repo}
	b, _ := os.ReadFile(filepath.Join(cfg.DataDir, "users.json"))
	_ = json.Unmarshal(b, &e.Users)
	tok, _ := os.ReadFile(filepath.Join(cfg.DataDir, "admin-token"))
	e.Token = strings.TrimSpace(string(tok))
	e.Client = &http.Client{Transport: e.Transport()}
	e.Admin = &http.Client{}
	return e
}

// Transport는 *.cloudinfra-sim.test를 실제 리스너 주소로 해석한다(hosts 파일 대체).
func (e *Env) Transport() *http.Transport {
	d := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: e.Sim.Certs.CAPool},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			target := e.Sim.Addrs["vcenter"].String()
			if strings.HasPrefix(host, "nsx.") {
				target = e.Sim.Addrs["nsx"].String()
			}
			return d.DialContext(ctx, network, target)
		},
	}
}

func (e *Env) User(product string) sim.User {
	for _, u := range e.Users {
		for _, p := range u.Products {
			if p == product {
				return u
			}
		}
	}
	e.T.Fatalf("no user for %s", product)
	return sim.User{}
}

// SOAP은 govmomi 클라이언트를 만든다(D2). version이 비어 있지 않으면 SOAPAction 버전을 지정한다.
func (e *Env) SOAP(version string) *govmomi.Client {
	e.T.Helper()
	ctx := context.Background()
	u, _ := url.Parse("https://vcenter." + Domain + "/sdk")
	sc := soap.NewClient(u, false)
	tr := sc.DefaultTransport()
	tr.TLSClientConfig = &tls.Config{RootCAs: e.Sim.Certs.CAPool}
	tr.DialTLSContext = nil // govmomi 자체 TLS 다이얼 대신 표준 경로(SNI = URL 호스트)
	tr.DialContext = e.Transport().DialContext
	vc, err := vim25.NewClient(ctx, sc)
	if err != nil {
		e.T.Fatal(err)
	}
	if version != "" {
		vc.Version = version
	}
	return &govmomi.Client{Client: vc, SessionManager: session.NewManager(vc)}
}

// Login은 vCenter 사용자로 로그인한 govmomi 클라이언트를 돌려준다.
func (e *Env) Login(version string) *govmomi.Client {
	e.T.Helper()
	c := e.SOAP(version)
	u := e.User("vcenter")
	if err := c.Login(context.Background(), url.UserPassword(u.Name, u.Password)); err != nil {
		e.T.Fatal(err)
	}
	return c
}

// AdminDo는 Admin API를 호출한다.
func (e *Env) AdminDo(method, path string, body io.Reader) (*http.Response, error) {
	req, _ := http.NewRequest(method, "http://"+e.Sim.Addrs["admin"].String()+path, body)
	req.Header.Set("Authorization", "Bearer "+e.Token)
	return e.Admin.Do(req)
}

// SOAPExchange는 기록된 SOAP 호출 하나다(스키마 대조 검사용).
type SOAPExchange struct {
	Method, SessionVersion string
	Status                 int
	Response               []byte
}

// Recorder는 SOAP 요청·응답 본문을 기록하는 http.RoundTripper다.
type Recorder struct {
	next       http.RoundTripper
	profileMax string
	mu         sync.Mutex
	Exchanges  []SOAPExchange
}

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var method string
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		req.Body.Close()
		method = profile.SOAPMethod(b)
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	res, err := r.next.RoundTrip(req)
	if err != nil || method == "" {
		return res, err
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(b))
	ver := r.profileMax
	if v, ok := strings.CutPrefix(strings.Trim(req.Header.Get("SOAPAction"), `"`), "urn:vim25/"); ok && v != "" && profile.Newer(r.profileMax, v) {
		ver = v
	}
	r.mu.Lock()
	r.Exchanges = append(r.Exchanges, SOAPExchange{Method: method, SessionVersion: ver, Status: res.StatusCode, Response: b})
	r.mu.Unlock()
	return res, nil
}

// RecordingLogin은 모든 SOAP 응답을 기록하는 govmomi 클라이언트로 로그인한다(TR-COV-0304).
func (e *Env) RecordingLogin(version string) (*govmomi.Client, *Recorder) {
	e.T.Helper()
	c := e.SOAP(version)
	rec := &Recorder{next: c.Client.Client.Transport, profileMax: e.Sim.Profile.VCenter.APIVersion}
	c.Client.Client.Transport = rec
	u := e.User("vcenter")
	if err := c.Login(context.Background(), url.UserPassword(u.Name, u.Password)); err != nil {
		e.T.Fatal(err)
	}
	return c, rec
}
