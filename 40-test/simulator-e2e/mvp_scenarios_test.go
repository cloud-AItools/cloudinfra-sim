package e2e

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudinfra-sim/internal/scenario"
	"cloudinfra-sim/test/internal/testenv"
)

// testEnv는 시험 환경을 Scenario Runner의 Env로 쓴다(Admin API Runner와 같은 실행 엔진, 30-implementation/internal/scenario).
type testEnv struct{ e *testenv.Env }

func (t testEnv) Domain() string          { return testenv.Domain }
func (t testEnv) RootCAs() *x509.CertPool { return t.e.Sim.Certs.CAPool }
func (t testEnv) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.e.Transport().DialContext(ctx, network, addr)
}
func (t testEnv) Credentials(product string) (string, string, error) {
	u := t.e.User(product)
	return u.Name, u.Password, nil
}
func (t testEnv) Admin(_ context.Context, method, path string, body io.Reader) (int, []byte, error) {
	res, err := t.e.AdminDo(method, path, body)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b, nil
}

func scenarioDir() string {
	return filepath.Join(testenv.Repo(), "30-implementation", "data", "scenarios")
}

// 선언형 시나리오(data/scenarios/*.yaml) 전체 회귀(TR-COV-0309 Phase 5: 승인된 자체 시나리오 전체 회귀)를 ①~⑨ 단계로
// 실행하고 리포트를 남긴다. 리포트 위치: SCENARIO_REPORT_DIR(기본 임시 디렉터리). `make scenario`는 40-test/reports에 쓴다.
// 추적: T-TR-SCN-0301 T-TR-SCN-0302 T-TR-SCN-0303 T-TR-SCN-0111 T-TR-COV-0302 T-TR-COV-0307 T-TR-COV-0309 T-TR-COV-0409 T-NFR-0404
func TestDeclarativeScenarios(t *testing.T) {
	files, err := scenario.LoadDir(scenarioDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 7 {
		t.Fatalf("scenario files: %d", len(files))
	}
	dir := os.Getenv("SCENARIO_REPORT_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	e := testenv.Start(t, nil)
	for _, f := range files {
		t.Run(f.Name, func(t *testing.T) {
			var log []string
			rep := scenario.Run(context.Background(), testEnv{e}, f, scenario.Options{ReportDir: dir, Log: func(l string) { log = append(log, l) }})
			for _, s := range rep.Steps {
				t.Logf("[%s] %-7s %s %s", s.Stage, s.Status, s.Name, s.Detail)
			}
			if !rep.Passed || rep.Status != "passed" {
				t.Fatalf("scenario %s %s (report %s)", f.Name, rep.Status, filepath.Join(dir, f.Name+"-report.md"))
			}
			if rep.TraceEntries == 0 || !rep.ResetVerified {
				t.Fatalf("trace %d, reset verified %v", rep.TraceEntries, rep.ResetVerified)
			}
			if len(log) < len(f.Steps) {
				t.Fatalf("execution log lines %d < steps %d", len(log), len(f.Steps))
			}
		})
	}
}

func adminCall(t *testing.T, e *testenv.Env, method, path string) (int, map[string]any, string) {
	t.Helper()
	res, err := e.AdminDo(method, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out, string(b)
}

func waitRun(t *testing.T, e *testenv.Env, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		_, run, _ := adminCall(t, e, http.MethodGet, "/v1/scenario-runs/"+id)
		if run["state"] != "running" {
			return run
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish", id)
	return nil
}

// 관리 API Scenario Runner: 목록·시작·진행 조회·리포트·실행 로그·동시 실행 거부·중지(TR-SCN-0106·0107, NFR-0404).
// 추적: T-TR-SCN-0106 T-TR-SCN-0107 T-TR-SCN-0110 T-TR-SCN-0303 T-NFR-0404
func TestScenarioRunnerAdminAPI(t *testing.T) {
	e := testenv.Start(t, nil)
	code, list, _ := adminCall(t, e, http.MethodGet, "/v1/scenarios")
	items, _ := list["scenarios"].([]any)
	if code != 200 || len(items) < 7 {
		t.Fatalf("list: %d %v", code, list)
	}
	if code, _, _ := adminCall(t, e, http.MethodPost, "/v1/scenarios/none/start"); code != 404 {
		t.Fatalf("unknown scenario: %d", code)
	}
	// 시작 → 끝까지 실행
	code, run, raw := adminCall(t, e, http.MethodPost, "/v1/scenarios/fault-task-failure-recovery/start")
	if code != 202 || run["state"] != "running" {
		t.Fatalf("start: %d %s", code, raw)
	}
	id := run["run_id"].(string)
	if code, _, _ := adminCall(t, e, http.MethodPost, "/v1/scenarios/mvp-vm-lifecycle/start"); code != 409 {
		t.Fatalf("concurrent start must be refused: %d", code)
	}
	done := waitRun(t, e, id)
	rep, _ := done["report"].(map[string]any)
	if done["state"] != "passed" || rep["reset_verified"] != true {
		t.Fatalf("run result: %v", done)
	}
	if code, _, md := adminCall(t, e, http.MethodGet, "/v1/scenario-runs/"+id+"/report"); code != 200 || !strings.Contains(md, "Scenario fault-task-failure-recovery — PASSED") {
		t.Fatalf("report: %d %.200s", code, md)
	}
	code, _, logText := adminCall(t, e, http.MethodGet, "/v1/scenario-runs/"+id+"/log")
	if code != 200 || !strings.Contains(logText, "[error] pass VM 생성 → 실패") || !strings.Contains(logText, "scenario fault-task-failure-recovery passed") {
		t.Fatalf("execution log: %d %.400s", code, logText)
	}
	for _, f := range []string{"-report.json", "-report.md", "-trace.json", "-log.txt"} {
		if _, err := os.Stat(filepath.Join(done["report_dir"].(string), "fault-task-failure-recovery"+f)); err != nil {
			t.Fatalf("report file %s: %v", f, err)
		}
	}
	if code, _, _ := adminCall(t, e, http.MethodPost, "/v1/scenario-runs/"+id+"/stop"); code != 409 {
		t.Fatalf("stop finished run: %d", code)
	}
	// 중지: Timeout 단계(2초 붙잡음)가 있는 시나리오를 시작하고 바로 중지 → stopped, 남은 단계 skipped, 환경 초기화 확인
	_, run, _ = adminCall(t, e, http.MethodPost, "/v1/scenarios/fault-nsx-realization/start")
	id2 := run["run_id"].(string)
	time.Sleep(300 * time.Millisecond)
	code, stopped, raw := adminCall(t, e, http.MethodPost, "/v1/scenario-runs/"+id2+"/stop")
	if code != 200 || stopped["state"] != "stopped" {
		t.Fatalf("stop: %d %s", code, raw)
	}
	r2, _ := stopped["report"].(map[string]any)
	steps, _ := r2["steps"].([]any)
	last := steps[len(steps)-2].(map[string]any) // 마지막은 reset 단계
	if last["status"] != "skipped" || r2["reset_verified"] != true {
		t.Fatalf("stopped report: %v", r2)
	}
	if code, f, _ := adminCall(t, e, http.MethodGet, "/v1/faults"); code != 200 || fmt.Sprint(f["rules"]) != "[]" {
		t.Fatalf("faults must be cleared by the stop reset: %v", f)
	}
	_, runs, _ := adminCall(t, e, http.MethodGet, "/v1/scenario-runs")
	if rs, _ := runs["runs"].([]any); len(rs) != 2 || rs[0].(map[string]any)["run_id"] != id2 {
		t.Fatalf("runs list: %v", runs)
	}
}
