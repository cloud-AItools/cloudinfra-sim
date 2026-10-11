package sim

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cloudinfra-sim/internal/scenario"
	"cloudinfra-sim/internal/store"
)

// 관리 API Scenario Runner(WP 5.1: TR-SCN-0106·0107·0110·0301 ~ 0303, NFR-0404).
//
// 시나리오 파일은 --scenarios-dir(기본 data/scenarios)의 YAML이다. 한 번에 하나만 실행한다(같은 환경을 공유하고 끝에
// 초기화하므로). 실행 기록은 최근 maxRuns개를 메모리에 두고, 리포트·Trace·실행 로그는 <data-dir>/reports/<run-id>/에
// 쓴다. 보존: 최근 maxRuns개 디렉터리와 총 reportBudget 바이트(RESOURCE_BUDGET §4 '검증 증거 Trace 500 MB') 중 먼저
// 닿는 한도로 오래된 것부터 지운다(NFR-0504).

const (
	maxRuns      = 20
	maxLogLines  = 5000
	reportBudget = 500 << 20
)

var (
	ErrScenarioNotFound = errors.New("시나리오가 없다")
	ErrScenarioRunning  = errors.New("실행 중인 시나리오가 있다")
	ErrRunNotFound      = errors.New("실행 기록이 없다")
	ErrRunFinished      = errors.New("이미 끝난 실행이다")
)

// ScenarioRun은 한 번의 실행이다.
type ScenarioRun struct {
	ID       string
	Scenario string
	State    string // running | passed | failed | stopped
	Started  time.Time
	Ended    *time.Time
	Report   *scenario.Report
	Dir      string

	cancel context.CancelFunc
	done   chan struct{}
	logMu  sync.Mutex
	log    []string
	logOut int // 메모리에서 밀려난 줄 수(파일에는 모두 남는다)
}

type scenarioRuns struct {
	mu     sync.Mutex
	runs   []*ScenarioRun // 오래된 순
	active *ScenarioRun
}

// Scenarios는 시나리오 목록이다.
func (s *Simulator) Scenarios() (any, error) {
	files, err := scenario.LoadDir(s.Cfg.ScenariosDir)
	if err != nil {
		return nil, err
	}
	type item struct {
		*scenario.File
		Steps int `json:"steps"`
	}
	out := []item{}
	for _, f := range files {
		out = append(out, item{f, len(f.Steps)})
	}
	return map[string]any{"scenarios": out, "dir": s.Cfg.ScenariosDir}, nil
}

// StartScenario는 시나리오를 백그라운드로 시작한다(TR-SCN-0106).
func (s *Simulator) StartScenario(name string) (any, error) {
	files, err := scenario.LoadDir(s.Cfg.ScenariosDir)
	if err != nil {
		return nil, err
	}
	var f *scenario.File
	for _, x := range files {
		if x.Name == name {
			f = x
		}
	}
	if f == nil {
		return nil, fmt.Errorf("%w: %s", ErrScenarioNotFound, name)
	}
	s.scen.mu.Lock()
	defer s.scen.mu.Unlock()
	if s.scen.active != nil {
		return nil, fmt.Errorf("%w: %s(%s)", ErrScenarioRunning, s.scen.active.Scenario, s.scen.active.ID)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
	dir := filepath.Join(s.Cfg.DataDir, "reports", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &ScenarioRun{ID: id, Scenario: name, State: "running", Started: time.Now().UTC(), Dir: dir, cancel: cancel, done: make(chan struct{})}
	s.scen.active = run
	s.scen.runs = append(s.scen.runs, run)
	s.pruneRunsLocked()
	logFile, _ := os.OpenFile(filepath.Join(dir, name+"-log.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: "scenario start", Target: name + " " + id, Status: 202, Cause: "admin"})
	go func() {
		defer close(run.done)
		defer cancel()
		rep := scenario.Run(ctx, &runnerEnv{s: s}, f, scenario.Options{ReportDir: dir, Log: func(line string) {
			if logFile != nil {
				_, _ = io.WriteString(logFile, line+"\n")
			}
			run.logMu.Lock()
			run.log = append(run.log, line)
			if len(run.log) > maxLogLines {
				run.logOut += len(run.log) - maxLogLines
				run.log = run.log[len(run.log)-maxLogLines:]
			}
			run.logMu.Unlock()
		}})
		if logFile != nil {
			_ = logFile.Close()
		}
		s.scen.mu.Lock()
		now := time.Now().UTC()
		run.Report, run.State, run.Ended = rep, rep.Status, &now
		if s.scen.active == run {
			s.scen.active = nil
		}
		s.scen.mu.Unlock()
		s.Log.Info("scenario finished", "scenario", name, "run", id, "state", rep.Status, "duration_ms", rep.DurationMS)
		s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: "scenario " + rep.Status, Target: name + " " + id, Status: 200, Cause: "admin"})
	}()
	return run.summary(), nil
}

// StopScenario는 실행 중인 시나리오를 중지한다(TR-SCN-0107). 정리 단계(⑦~⑨)가 끝날 때까지 기다린다.
func (s *Simulator) StopScenario(id string) (any, error) {
	run := s.findRun(id)
	if run == nil {
		return nil, ErrRunNotFound
	}
	s.scen.mu.Lock()
	running := run.State == "running"
	s.scen.mu.Unlock()
	if !running {
		return nil, ErrRunFinished
	}
	run.cancel()
	select {
	case <-run.done:
	case <-time.After(2 * time.Minute):
	}
	s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: "scenario stop", Target: run.Scenario + " " + id, Status: 200, Cause: "admin"})
	return s.ScenarioRun(id)
}

// ScenarioRuns는 실행 기록 요약이다(최근 것 먼저).
func (s *Simulator) ScenarioRuns() any {
	s.scen.mu.Lock()
	defer s.scen.mu.Unlock()
	out := []map[string]any{}
	for i := len(s.scen.runs) - 1; i >= 0; i-- {
		out = append(out, s.scen.runs[i].summary())
	}
	return map[string]any{"runs": out}
}

// ScenarioRun은 실행 상세(리포트 포함)다.
func (s *Simulator) ScenarioRun(id string) (any, error) {
	run := s.findRun(id)
	if run == nil {
		return nil, ErrRunNotFound
	}
	s.scen.mu.Lock()
	defer s.scen.mu.Unlock()
	out := run.summary()
	out["report"] = run.Report
	return out, nil
}

// ScenarioLog는 실행 로그(NFR-0404)다. 메모리 보존분을 돌려주고 밀려난 줄 수를 알린다(파일에는 모두 있다).
func (s *Simulator) ScenarioLog(id string) (string, error) {
	run := s.findRun(id)
	if run == nil {
		return "", ErrRunNotFound
	}
	run.logMu.Lock()
	defer run.logMu.Unlock()
	var b strings.Builder
	if run.logOut > 0 {
		fmt.Fprintf(&b, "# %d earlier lines rotated out (full log: %s)\n", run.logOut, filepath.Join(run.Dir, run.Scenario+"-log.txt"))
	}
	for _, l := range run.log {
		b.WriteString(l + "\n")
	}
	return b.String(), nil
}

// ScenarioReport는 끝난 실행의 Markdown 리포트다.
func (s *Simulator) ScenarioReport(id string) (string, error) {
	run := s.findRun(id)
	if run == nil {
		return "", ErrRunNotFound
	}
	s.scen.mu.Lock()
	defer s.scen.mu.Unlock()
	if run.Report == nil {
		return "", ErrScenarioRunning
	}
	return scenario.Markdown(run.Report), nil
}

func (s *Simulator) findRun(id string) *ScenarioRun {
	s.scen.mu.Lock()
	defer s.scen.mu.Unlock()
	for _, r := range s.scen.runs {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// summary는 호출자가 s.scen.mu를 쥔 상태에서 부른다.
func (r *ScenarioRun) summary() map[string]any {
	m := map[string]any{"run_id": r.ID, "scenario": r.Scenario, "state": r.State, "started": r.Started, "report_dir": r.Dir}
	if r.Ended != nil {
		m["ended"] = *r.Ended
	}
	if r.Report != nil {
		count := map[string]int{"pass": 0, "fail": 0, "skipped": 0}
		for _, st := range r.Report.Steps {
			count[st.Status]++
		}
		m["steps"] = count
	}
	return m
}

// pruneRunsLocked는 오래된 실행 기록·리포트 디렉터리를 지운다(최근 maxRuns개, 총 reportBudget).
func (s *Simulator) pruneRunsLocked() {
	for len(s.scen.runs) > maxRuns && s.scen.runs[0] != s.scen.active {
		_ = os.RemoveAll(s.scen.runs[0].Dir)
		s.scen.runs = s.scen.runs[1:]
	}
	root := filepath.Join(s.Cfg.DataDir, "reports")
	entries, _ := os.ReadDir(root)
	type dir struct {
		name string
		size int64
	}
	var dirs []dir
	var total int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		size := treeSize(filepath.Join(root, e.Name()))
		dirs = append(dirs, dir{e.Name(), size})
		total += size
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name }) // run ID는 시각으로 시작
	for len(dirs) > 0 && (total > reportBudget || len(dirs) > maxRuns) {
		if s.scen.active != nil && dirs[0].name == s.scen.active.ID {
			break
		}
		_ = os.RemoveAll(filepath.Join(root, dirs[0].name))
		total -= dirs[0].size
		dirs = dirs[1:]
	}
}

func treeSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return err
	})
	return total
}

// runnerEnv는 Admin API Runner가 시뮬레이터에 접근하는 방법이다: Gateway 수신 주소로 연결하고(외부 인터페이스 그대로),
// Admin API는 같은 프로세스 안에서 토큰과 함께 호출한다.
type runnerEnv struct{ s *Simulator }

func (e *runnerEnv) Domain() string          { return e.s.Cfg.Domain }
func (e *runnerEnv) RootCAs() *x509.CertPool { return e.s.Certs.CAPool }

func (e *runnerEnv) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, _ := net.SplitHostPort(addr)
	target := e.s.Addrs["vcenter"]
	if strings.HasPrefix(host, "nsx.") {
		target = e.s.Addrs["nsx"]
	}
	if target == nil {
		return nil, fmt.Errorf("listener for %s not started", host)
	}
	return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, target.String())
}

func (e *runnerEnv) Credentials(product string) (string, string, error) {
	u, ok := e.s.Users.Default(product)
	if !ok {
		return "", "", fmt.Errorf("no default %s user", product)
	}
	return u.Name, u.Password, nil
}

func (e *runnerEnv) Admin(ctx context.Context, method, path string, body io.Reader) (int, []byte, error) {
	if body == nil {
		body = http.NoBody
	}
	req := httptest.NewRequestWithContext(ctx, method, path, body)
	req.Header.Set("Authorization", "Bearer "+e.s.adminToken)
	rec := httptest.NewRecorder()
	e.s.adminSrv.Handler.ServeHTTP(rec, req)
	return rec.Code, bytes.TrimSpace(rec.Body.Bytes()), nil
}
