// 전송 계층 Fault Injection (PRD-1 FR-FLT-0117·0120·0123·0125·0205·0206, PRD-2 TR-SCN-0109, Phase 3 종료 게이트).
// Fault Rule의 추가·제거·적용을 Admin API로 시험한다. 오류 응답 형식은 제품 오류 계약(BEHAVIOR_SPEC §5)을 따른다.
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloudinfra-sim/test/internal/testenv"
)

type faultRule struct {
	ID     string `json:"id"`
	Layer  string `json:"layer"`
	Hits   int    `json:"hits"`
	Count  int    `json:"count"`
	Action struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
	} `json:"action"`
}

func addFault(t *testing.T, e *testenv.Env, body string) faultRule {
	t.Helper()
	res, err := e.AdminDo(http.MethodPost, "/v1/faults", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("add fault %s: %d %s", body, res.StatusCode, b)
	}
	var r faultRule
	_ = json.Unmarshal(b, &r)
	return r
}

func listFaults(t *testing.T, e *testenv.Env) []faultRule {
	t.Helper()
	res, err := e.AdminDo(http.MethodGet, "/v1/faults", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Rules []faultRule `json:"rules"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out.Rules
}

// 추적: T-TR-SCN-0109 T-FR-FLT-0117 T-FR-FLT-0120 T-FR-FLT-0123 T-FR-FLT-0125 T-FR-FLT-0205 T-FR-FLT-0206 T-NFR-0403
func TestTransportFaultRules(t *testing.T) {
	var logBuf lockedBuf
	e := testenv.StartWithLogger(t, nil, slog.New(slog.NewJSONHandler(&logBuf, nil)))

	// 잘못된 규칙 거부(검증), Phase 3 범위 밖 계층 거부
	for _, bad := range []string{`{"action":{"type":"status","status":200}}`, `{"layer":"nsx","action":{"type":"unavailable"}}`, `{"action":{"type":"delay"}}`, `{"bogus":1}`} {
		res, _ := e.AdminDo(http.MethodPost, "/v1/faults", strings.NewReader(bad))
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid rule %s: %d", bad, res.StatusCode)
		}
	}

	// ① 일시적 503, 2회 후 복구(NSX, ApiError 형식, Retry-After)
	r1 := addFault(t, e, `{"match":{"product":"nsx","path_prefix":"/policy/api/v1/infra/segments"},"action":{"type":"unavailable","retry_after_sec":3},"count":2}`)
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/policy/api/v1/infra/segments", nil)
		u := e.User("nsx")
		req.SetBasicAuth(u.Name, u.Password)
		res, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if res.StatusCode != 503 || res.Header.Get("Retry-After") != "3" || body["error_code"] != float64(503) || body["module_name"] == nil {
			t.Fatalf("fault hit %d: %d %v %v", i, res.StatusCode, res.Header, body)
		}
	}
	if code, _ := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments", ""); code != 200 {
		t.Fatalf("must recover after 2 failures: %d", code)
	}
	if code, _ := nsxDo(t, e, http.MethodGet, "/api/v1/node/version", ""); code != 200 {
		t.Fatalf("non-matching path affected: %d", code)
	}

	// ② 지정 HTTP 상태: vCenter REST는 vAPI 오류 구조, SOAP은 프록시 수준 HTTP 오류
	r2 := addFault(t, e, `{"match":{"product":"vcenter","path_prefix":"/api/"},"action":{"type":"status","status":500}}`)
	res, err := e.Client.Get("https://vcenter." + testenv.Domain + "/api/appliance/system/version")
	if err != nil {
		t.Fatal(err)
	}
	var vapi map[string]any
	_ = json.NewDecoder(res.Body).Decode(&vapi)
	res.Body.Close()
	if res.StatusCode != 500 || vapi["error_type"] != "ERROR" || vapi["messages"] == nil {
		t.Fatalf("REST status fault: %d %v", res.StatusCode, vapi)
	}
	r3 := addFault(t, e, `{"match":{"product":"vcenter","soap_method":"RetrieveServiceContent"},"action":{"type":"status","status":502},"count":1}`)
	soapReq := func() *http.Response {
		env := `<?xml version="1.0" encoding="UTF-8"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body>` +
			`<RetrieveServiceContent xmlns="urn:vim25"><_this type="ServiceInstance">ServiceInstance</_this></RetrieveServiceContent></soapenv:Body></soapenv:Envelope>`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://vcenter."+testenv.Domain+"/sdk", strings.NewReader(env))
		req.Header.Set("Content-Type", "text/xml")
		req.Header.Set("SOAPAction", "urn:vim25/7.0.3.0")
		res, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res = soapReq()
	sb, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 502 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain") || bytes.Contains(sb, []byte("Envelope")) {
		t.Fatalf("SOAP transport fault: %d %q %s", res.StatusCode, res.Header.Get("Content-Type"), sb)
	}
	if res = soapReq(); res.StatusCode != 200 {
		t.Fatalf("SOAP must recover after 1 failure: %d", res.StatusCode)
	}
	res.Body.Close()

	// ③ 응답 지연: 지정 시간 이상 걸리고 결과는 정상
	r4 := addFault(t, e, `{"match":{"product":"nsx","path_prefix":"/api/v1/node/version"},"action":{"type":"delay","delay_ms":400},"count":1}`)
	t0 := time.Now()
	if code, _ := nsxDo(t, e, http.MethodGet, "/api/v1/node/version", ""); code != 200 || time.Since(t0) < 400*time.Millisecond {
		t.Fatalf("delay fault: %d after %v", code, time.Since(t0))
	}

	// 적용 이력(hits)·제거
	got := map[string]faultRule{}
	for _, r := range listFaults(t, e) {
		got[r.ID] = r
	}
	if got[r1.ID].Hits != 2 || got[r2.ID].Hits != 1 || got[r3.ID].Hits != 1 || got[r4.ID].Hits != 1 {
		t.Fatalf("hits: %+v", got)
	}
	res, _ = e.AdminDo(http.MethodDelete, "/v1/faults/"+r2.ID, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("remove: %d", res.StatusCode)
	}
	res, _ = e.Client.Get("https://vcenter." + testenv.Domain + "/api/appliance/system/version")
	res.Body.Close()
	if res.StatusCode == 500 {
		t.Fatal("removed rule still applied")
	}
	res, _ = e.AdminDo(http.MethodDelete, "/v1/faults/"+r2.ID, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("remove missing: %d", res.StatusCode)
	}

	// Fault Injection Log(NFR-0403)와 Trace의 규칙 표시
	n := 0
	for _, m := range logBuf.lines() {
		if m["msg"] == "fault injected" && m["rule"] != nil {
			n++
		}
	}
	if n < 5 {
		t.Fatalf("fault injection log entries %d, want >= 5", n)
	}
	res, _ = e.AdminDo(http.MethodGet, "/v1/trace?limit=200", nil)
	tb, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var tr struct {
		Entries []struct {
			FaultRule string `json:"fault_rule"`
			Status    int    `json:"status"`
		} `json:"entries"`
	}
	_ = json.Unmarshal(tb, &tr)
	faulted := 0
	for _, en := range tr.Entries {
		if en.FaultRule == r1.ID && en.Status == 503 {
			faulted++
		}
	}
	if faulted != 2 {
		t.Fatal("trace must record applied fault rule")
	}

	// 환경 초기화는 규칙을 모두 지운다
	res, _ = e.AdminDo(http.MethodPost, "/v1/reset", nil)
	res.Body.Close()
	if len(listFaults(t, e)) != 0 {
		t.Fatal("reset must clear fault rules")
	}
}
