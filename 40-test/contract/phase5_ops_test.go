// Phase 5 운영 기능: full 보존·회전(WP 5.9), API Coverage Report(WP 5.5), 이벤트 조회(TR-SCN-0110).
package contract

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

// T-NFR-0115 T-NFR-0504 T-NFR-0503 T-NFR-0506 T-NFR-0508 T-NFR-0501 T-NFR-0302: full 실행 프로필은 Trace 본문을 남기되
// 자격 증명을 가리고 64 KB에서 자르며, 건수·용량·기간(가상 시각 7일) 한도로 회전한다.
func TestFullProfileRetention(t *testing.T) {
	e := testenv.Start(t, func(c *config.Config) { c.RunProfile, c.RetainTrace, c.RetainTraceMB = "full", 40, 1 })
	vu := e.User("vcenter")
	c := e.Login("")
	_, _ = inventory(t, c)
	// REST 세션 생성 응답(토큰)도 가린다
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.SetBasicAuth(vu.Name, vu.Password)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := io.ReadAll(res.Body)
	res.Body.Close()
	_, tr := adminJSON(t, e, http.MethodGet, "/v1/trace?limit=0", "")
	entries, _ := tr["entries"].([]any)
	var login, session map[string]any
	for _, x := range entries {
		m := x.(map[string]any)
		if m["soap_method"] == "Login" {
			login = m
		}
		if m["path"] == "/api/session" && m["method"] == "POST" {
			session = m
		}
	}
	if login == nil || !strings.Contains(fmt.Sprint(login["request_body"]), "<password>***</password>") || strings.Contains(fmt.Sprint(tr), vu.Password) {
		t.Fatalf("SOAP Login body must be recorded with the password masked: %v", login)
	}
	if session == nil || session["response_body"] != `"***"` || strings.Contains(fmt.Sprint(tr), strings.Trim(string(tok), `"`)) {
		t.Fatalf("REST session token must be masked: %v", session)
	}
	// 64 KB에서 자른다
	big := strings.Repeat("d", 100<<10)
	mustNSX(t, e, "PATCH", p4+"/infra/segments/big-body", `{"display_name":"big","description":"`+big+`"}`, 200)
	_, tr = adminJSON(t, e, http.MethodGet, "/v1/trace?limit=3", "")
	truncated := false
	for _, x := range tr["entries"].([]any) {
		m := x.(map[string]any)
		if m["path"] == "/policy/api/v1/infra/segments/big-body" && m["method"] == "PATCH" {
			truncated = m["body_truncated"] == true && len(fmt.Sprint(m["request_body"])) == 64<<10
		}
	}
	if !truncated {
		t.Fatal("request body over 64 KB must be truncated and flagged")
	}
	// 건수 한도 40 → 회전, 회전 건수 표시
	for i := 0; i < 60; i++ {
		nsxDo(t, e, "GET", "/api/v1/node/version", "")
	}
	_, tr = adminJSON(t, e, http.MethodGet, "/v1/trace?limit=0", "")
	if n := len(tr["entries"].([]any)); n > 40 || tr["dropped"].(float64) < 20 {
		t.Fatalf("row limit: %d entries, dropped %v", n, tr["dropped"])
	}
	// 용량 한도 1 MB: 64 KB 본문 30건 → 1 MB 이하
	for i := 0; i < 30; i++ {
		nsxDo(t, e, "PATCH", p4+"/infra/segments/big-body", `{"description":"`+big+`"}`)
	}
	_, st := adminJSON(t, e, http.MethodGet, "/v1/status", "")
	usage := st["retention_usage"].(map[string]any)["trace"].(map[string]any)
	if usage["bytes"].(float64) > 1<<20 || usage["rows"].(float64) == 0 {
		t.Fatalf("byte limit: %v", usage)
	}
	// 기간 한도: 가상 시각 8일 뒤 → 이전 Trace·감사·전이 기록 정리
	adminJSON(t, e, http.MethodPut, "/v1/clock", `{"mode":"test","advance_ms":691200000}`)
	nsxDo(t, e, "GET", "/api/v1/node/version", "")
	_, tr = adminJSON(t, e, http.MethodGet, "/v1/trace?limit=0", "")
	for _, x := range tr["entries"].([]any) {
		if p := x.(map[string]any)["path"]; p != "/api/v1/node/version" && p != "/v1/clock" {
			t.Fatalf("entry older than 7 days kept: %v", x)
		}
	}
	_, au := adminJSON(t, e, http.MethodGet, "/v1/audit?limit=0", "")
	for _, x := range au["events"].([]any) {
		if a := x.(map[string]any)["action"]; strings.Contains(fmt.Sprint(a), "Login") {
			t.Fatalf("audit older than 7 days kept: %v", x)
		}
	}
}

// T-TR-SCN-0112 T-TR-COV-0101 T-TR-COV-0102 T-FR-API-0403 T-FR-API-0404 T-TR-COV-0409
func TestCoverageReport(t *testing.T) {
	e := testenv.Start(t, nil)
	nsxDo(t, e, "GET", "/api/v1/no-such-api", "") // 미지원 호출 관측
	code, rep := adminJSON(t, e, http.MethodGet, "/v1/coverage-report", "")
	if code != 200 || !strings.HasPrefix(fmt.Sprint(rep["verification_cap"]), "Integration Tested") {
		t.Fatalf("report: %d %v", code, rep)
	}
	products := rep["products"].([]any)
	if len(products) != 2 {
		t.Fatalf("products: %v", products)
	}
	for _, p := range products {
		pm := p.(map[string]any)
		inScope, counted := 0, 0
		for _, a := range pm["apis"].([]any) {
			am := a.(map[string]any)
			if am["scope"] == "execution" {
				inScope++
				if am["status"] == "Integration Tested" && am["tests"] == nil {
					t.Fatalf("%s: tested API without test evidence", am["api"])
				}
			}
			if am["counted_as_supported"] == true {
				counted++
				if am["scope"] != "execution" {
					t.Fatalf("%s: out-of-scope API counted as supported", am["api"])
				}
			}
		}
		sum := pm["summary"].(map[string]any)
		if float64(counted) != sum["counted_as_supported"] || inScope == 0 {
			t.Fatalf("%s summary: counted %d, %v", pm["product"], counted, sum)
		}
	}
	unsup := fmt.Sprint(rep["unsupported_calls"])
	if !strings.Contains(unsup, "/api/v1/no-such-api") {
		t.Fatalf("unsupported calls: %s", unsup)
	}
	res, err := e.AdminDo(http.MethodGet, "/v1/coverage-report?format=markdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	md, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, want := range []string{"# API Coverage Report", "| VC-SOAP-017 |", "| NSX-027 |", "no-such-api", "Integration Tested (승인 범위 자체 테스트, CMP·실환경 미검증)"} {
		if !strings.Contains(string(md), want) {
			t.Fatalf("markdown missing %q", want)
		}
	}
}

// T-TR-SCN-0110: 이벤트 조회(vCenter 이벤트, 가상 시각, 타입·엔터티 필터)
func TestEventInspection(t *testing.T) {
	e := testenv.Start(t, nil)
	l := snapVC(t, e)
	ref := l.create("ev-vm")
	code, ev := adminJSON(t, e, http.MethodGet, "/v1/events?type=VmCreatedEvent&entity="+ref.Value, "")
	list, _ := ev["vcenter"].([]any)
	if code != 200 || len(list) != 1 || list[0].(map[string]any)["entity_name"] != "ev-vm" {
		t.Fatalf("events: %d %v", code, ev)
	}
	_, all := adminJSON(t, e, http.MethodGet, "/v1/events?limit=3", "")
	if n := len(all["vcenter"].([]any)); n != 3 {
		t.Fatalf("limit: %d", n)
	}
}
