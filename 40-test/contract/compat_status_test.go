package contract

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"cloudinfra-sim/test/internal/testenv"
)

// 검증 상한·실행 범위 표시(TR-COV-0409, AC-13): Admin 상태가 검증 상한(Integration Tested, CMP·실환경 미검증)과
// API별 상태·범위를 보여 주고, 실행 범위 밖 API는 지원으로 표시하지 않는다.
// 추적: T-TR-COV-0409 T-FR-API-0403
func TestVerificationCapDisplayed(t *testing.T) {
	e := testenv.Start(t, nil)
	res, err := e.AdminDo(http.MethodGet, "/v1/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var st struct {
		Cap     string `json:"verification_cap"`
		VCenter struct {
			APIs []map[string]string `json:"apis"`
		} `json:"vcenter"`
		NSX struct {
			APIs []map[string]string `json:"apis"`
		} `json:"nsx"`
	}
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Cap, "Integration Tested") || !strings.Contains(st.Cap, "미검증") || strings.Contains(st.Cap, "100%") {
		t.Fatalf("verification cap: %q", st.Cap)
	}
	n := 0
	for _, a := range append(st.VCenter.APIs, st.NSX.APIs...) {
		if a["scope"] == "execution" {
			n++
		} else if !strings.Contains(a["scope"], "미검증") {
			t.Fatalf("out-of-scope API must be marked unverified: %v", a)
		}
	}
	if n != len(e.Sim.Profile.VCenter.ExecutionScope)+len(e.Sim.Profile.NSX.ExecutionScope) {
		t.Fatalf("execution-scope APIs listed %d", n)
	}
}
