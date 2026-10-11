// Package admin은 시뮬레이터 전용 Admin API다(루프백 전용, VMware API와 분리 — NFR-ARC-0305, NFR-0305).
package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cloudinfra-sim/internal/clock"
	"cloudinfra-sim/internal/fault"
	"cloudinfra-sim/internal/vcenter"
)

// Backend는 Admin API가 호출하는 시뮬레이터 기능이다.
type Backend interface {
	Status() any
	Reset() error
	Trace(limit int) any
	Unsupported() any
	SetRealizationDelayMS(ms int)
	Faults() *fault.Store               // Fault Rule 관리(FR-FLT-0206: Admin API 전용)
	Reconcile() any                     // vCenter ↔ NSX 관계 즉시 조정(FR-DM-0403)
	Relations() any                     // 마지막 조정 결과
	AuditLog(limit int, cid string) any // 감사 기록(NFR-0306)
	AuditAdmin(action string, status int)
	MetricOverrides() any
	ClockStatus() any
	UserList() any
	AddUser(name string, products []string, role, scope string) (any, error)
	RemoveUser(name string) bool
	SetClock(ch clock.Change) error
	SetMetricOverride(o vcenter.MetricOverride) error
	ClearMetricOverrides()
	// Phase 5(WP 5.2): Snapshot/Restore·불일치 검사. ErrorStatus는 오류를 HTTP 상태로 바꾼다.
	Snapshots() any
	CreateSnapshot(name string) (any, error)
	RestoreSnapshot(name string) (any, error)
	DeleteSnapshot(name string) error
	Consistency() any
	ErrorStatus(err error) int
	Events(limit int, typ, entity string) (any, error) // 이벤트 조회(TR-SCN-0110)
	// Scenario Runner(WP 5.1, TR-SCN-0106·0107, NFR-0404)
	Scenarios() (any, error)
	StartScenario(name string) (any, error)
	StopScenario(id string) (any, error)
	ScenarioRuns() any
	ScenarioRun(id string) (any, error)
	ScenarioLog(id string) (string, error)
	ScenarioReport(id string) (string, error)
	Coverage(markdown bool) any // API Coverage Report(TR-SCN-0112). markdown이면 string
}

const tokenFile = "admin-token"

// LoadOrCreateToken은 Admin API 토큰을 데이터 디렉터리에 만든다(0600). 코드에 고정하지 않는다.
func LoadOrCreateToken(dataDir string) (string, error) {
	p := filepath.Join(dataDir, tokenFile)
	if b, err := os.ReadFile(p); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	t := hex.EncodeToString(b)
	return t, os.WriteFile(p, []byte(t+"\n"), 0o600)
}

func Handler(b Backend, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.Status()) })
	mux.HandleFunc("POST /v1/reset", func(w http.ResponseWriter, _ *http.Request) {
		if err := b.Reset(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"result": "reset"})
	})
	mux.HandleFunc("GET /v1/trace", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		writeJSON(w, http.StatusOK, b.Trace(n))
	})
	mux.HandleFunc("GET /v1/unsupported", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.Unsupported()) })
	mux.HandleFunc("PUT /v1/nsx/realization-delay", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MS int `json:"ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MS < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body: {\"ms\": <0 이상 정수>}"})
			return
		}
		b.SetRealizationDelayMS(body.MS)
		writeJSON(w, http.StatusOK, body)
	})
	// Fault Rule 관리(FR-FLT-0205·0206, TR-SCN-0109). 규칙은 환경 초기화 때 모두 지운다.
	// 가상 메트릭 덮어쓰기(FR-MET-0103: 특정 사용량 변경·결측값)
	// Virtual Clock(FR-BE-0602~0604·0609, TR-SCN-0108): mode normal|test, rate, frozen, advance_ms, advance_to
	// 시험용 사용자·역할(FR-API-0211 ~ 0213). 비밀번호는 생성 응답에서 한 번만 돌려준다.
	mux.HandleFunc("GET /v1/users", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.UserList()) })
	mux.HandleFunc("POST /v1/users", func(w http.ResponseWriter, r *http.Request) {
		var u struct {
			Name     string   `json:"name"`
			Products []string `json:"products"`
			Role     string   `json:"role"`
			Scope    string   `json:"scope"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&u); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		out, err := b.AddUser(u.Name, u.Products, u.Role, u.Scope)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, out)
	})
	mux.HandleFunc("DELETE /v1/users/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !b.RemoveUser(r.PathValue("name")) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "시험용 사용자 없음(기본 사용자는 지울 수 없다)"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/clock", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.ClockStatus()) })
	mux.HandleFunc("PUT /v1/clock", func(w http.ResponseWriter, r *http.Request) {
		var ch clock.Change
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := b.SetClock(ch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, b.ClockStatus())
	})
	mux.HandleFunc("GET /v1/metrics/overrides", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.MetricOverrides()) })
	mux.HandleFunc("PUT /v1/metrics/overrides", func(w http.ResponseWriter, r *http.Request) {
		var o vcenter.MetricOverride
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := b.SetMetricOverride(o); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, o)
	})
	mux.HandleFunc("DELETE /v1/metrics/overrides", func(w http.ResponseWriter, _ *http.Request) {
		b.ClearMetricOverrides()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/audit", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		writeJSON(w, http.StatusOK, b.AuditLog(n, r.URL.Query().Get("cid")))
	})
	// Snapshot/Restore(TR-SCN-0104·0105, NFR-0206). 복원 결과에는 불일치 검사(NFR-0207)가 들어 있다.
	mux.HandleFunc("GET /v1/snapshots", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": b.Snapshots()})
	})
	mux.HandleFunc("POST /v1/snapshots", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body: {\"name\": \"<이름>\"}"})
			return
		}
		out, err := b.CreateSnapshot(body.Name)
		if err != nil {
			writeJSON(w, b.ErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, out)
	})
	mux.HandleFunc("POST /v1/snapshots/{name}/restore", func(w http.ResponseWriter, r *http.Request) {
		out, err := b.RestoreSnapshot(r.PathValue("name"))
		if err != nil {
			writeJSON(w, b.ErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("DELETE /v1/snapshots/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := b.DeleteSnapshot(r.PathValue("name")); err != nil {
			writeJSON(w, b.ErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		n, _ := strconv.Atoi(q.Get("limit"))
		out, err := b.Events(n, q.Get("type"), q.Get("entity"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	fail := func(w http.ResponseWriter, err error) {
		writeJSON(w, b.ErrorStatus(err), map[string]string{"error": err.Error()})
	}
	mux.HandleFunc("GET /v1/scenarios", func(w http.ResponseWriter, _ *http.Request) {
		out, err := b.Scenarios()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /v1/scenarios/{name}/start", func(w http.ResponseWriter, r *http.Request) {
		out, err := b.StartScenario(r.PathValue("name"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, out)
	})
	mux.HandleFunc("GET /v1/scenario-runs", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.ScenarioRuns()) })
	mux.HandleFunc("GET /v1/scenario-runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := b.ScenarioRun(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /v1/scenario-runs/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		out, err := b.StopScenario(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	text := func(get func(string) (string, error), ctype string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			out, err := get(r.PathValue("id"))
			if err != nil {
				fail(w, err)
				return
			}
			w.Header().Set("Content-Type", ctype)
			_, _ = w.Write([]byte(out))
		}
	}
	mux.HandleFunc("GET /v1/scenario-runs/{id}/log", text(b.ScenarioLog, "text/plain; charset=utf-8"))
	mux.HandleFunc("GET /v1/scenario-runs/{id}/report", text(b.ScenarioReport, "text/markdown; charset=utf-8"))
	mux.HandleFunc("GET /v1/coverage-report", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") == "markdown" {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = w.Write([]byte(b.Coverage(true).(string)))
			return
		}
		writeJSON(w, http.StatusOK, b.Coverage(false))
	})
	mux.HandleFunc("GET /v1/consistency", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.Consistency()) })
	mux.HandleFunc("POST /v1/reconcile", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.Reconcile()) })
	mux.HandleFunc("GET /v1/relations", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, b.Relations()) })
	mux.HandleFunc("GET /v1/faults", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"rules": b.Faults().List()})
	})
	mux.HandleFunc("POST /v1/faults", func(w http.ResponseWriter, r *http.Request) {
		var rule fault.Rule
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "규칙 JSON: " + err.Error()})
			return
		}
		added, err := b.Faults().Add(rule)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, added)
	})
	mux.HandleFunc("DELETE /v1/faults/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !b.Faults().Remove(r.PathValue("id")) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "규칙 없음"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /v1/faults", func(w http.ResponseWriter, _ *http.Request) {
		b.Faults().Clear()
		w.WriteHeader(http.StatusNoContent)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Admin API 토큰이 필요하다(데이터 디렉터리의 admin-token)"})
			return
		}
		if r.Method == http.MethodGet {
			mux.ServeHTTP(w, r)
			return
		}
		rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(rec, r)
		if r.URL.Path != "/v1/reset" { // reset은 시뮬레이터가 직접 기록
			b.AuditAdmin(r.Method+" "+r.URL.Path, rec.status)
		}
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
