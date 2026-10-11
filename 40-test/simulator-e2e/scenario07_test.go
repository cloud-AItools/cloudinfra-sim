// Scenario 07 — NSX Security Policy 변경 (PRD-2 TR-E2E-0701 ~ TR-E2E-0704, Phase 4b 종료 게이트).
package e2e

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"cloudinfra-sim/test/internal/nsxcheck"
	"cloudinfra-sim/test/internal/testenv"
)

// TR-E2E-0701 절차, 성공 기준 0702 ~ 0704.
// 추적: T-TR-E2E-0701 T-TR-E2E-0702 T-TR-E2E-0703 T-TR-E2E-0704 T-FR-NSX-0215 T-FR-NSX-0216 T-FR-NSX-0217 T-FR-NSX-0218 T-FR-NSX-0408 T-FR-NSX-0409
func TestScenario07_SecurityPolicyChange(t *testing.T) {
	e := testenv.Start(t, nil)
	n := nsx{t, e}
	spec, err := nsxcheck.Load(filepath.Join(e.Sim.Profile.NSXDir, "nsx_policy_api.json"))
	if err != nil {
		t.Skipf("NSX 스펙 없음(Get-NsxSpec.ps1): %v", err)
	}
	const (
		base   = "/policy/api/v1"
		group  = "/infra/domains/default/groups/s07-web"
		svc    = "/infra/services/s07-app"
		policy = "/infra/domains/default/security-policies/s07-policy"
		rule   = policy + "/rules/allow-app"
	)
	must := func(step, method, path, body string, want int) map[string]any {
		t.Helper()
		code, out, raw := n.do(method, base+path, body)
		if code != want {
			t.Fatalf("%s: %s %s → %d, want %d: %s", step, method, path, code, want, raw)
		}
		return out
	}

	// ① Group 생성(VM Tag 조건)
	must("① Group", http.MethodPatch, group, `{"display_name":"s07-web","expression":[{"resource_type":"Condition","member_type":"VirtualMachine","key":"Tag","operator":"EQUALS","value":"tier|web"}]}`, 200)
	// ② Service 생성(TCP 8443)
	must("② Service", http.MethodPatch, svc, `{"display_name":"s07-app","service_entries":[{"resource_type":"L4PortSetServiceEntry","id":"tcp-8443","l4_protocol":"TCP","destination_ports":["8443"]}]}`, 200)
	// ③ Security Policy와 Rule 생성(정책 본문에 Rule 포함)
	must("③ Policy", http.MethodPatch, policy, fmt.Sprintf(`{"display_name":"s07-policy","category":"Application","rules":[{"id":"allow-app","action":"ALLOW","source_groups":[%q],"destination_groups":["ANY"],"services":[%q],"scope":["ANY"],"sequence_number":10}]}`, group, svc), 200)
	created := must("③ Rule 조회", http.MethodGet, rule, "", 200)
	rev0, _ := created["_revision"].(float64)
	for _, p := range []string{group, svc, policy, rule} {
		if st := n.realization(p); st != "SUCCESS" {
			t.Fatalf("T-TR-E2E-0702: %s realization %s", p, st)
		}
	}

	// ④ 기존 Rule 수정(최신 Revision 사용): 동작 DROP, 로그 켬
	must("④ Rule 수정", http.MethodPatch, rule, fmt.Sprintf(`{"action":"DROP","logged":true,"source_groups":[%q],"destination_groups":["ANY"],"services":[%q],"scope":["ANY"],"sequence_number":10,"_revision":%v}`, group, svc, rev0), 200)

	// ⑤ 최신 Revision 조회 — TR-E2E-0702 정상 변경 반영, TR-E2E-0703 Revision 증가
	latest := must("⑤ 조회", http.MethodGet, rule, "", 200)
	if latest["action"] != "DROP" || latest["logged"] != true {
		t.Fatalf("T-TR-E2E-0702: change not applied: %v", latest)
	}
	if latest["_revision"] != rev0+1 {
		t.Fatalf("T-TR-E2E-0703: revision %v → %v", rev0, latest["_revision"])
	}
	if latest["rule_id"] != created["rule_id"] {
		t.Fatalf("T-TR-E2E-0702: rule_id changed %v → %v", created["rule_id"], latest["rule_id"])
	}
	pol := must("⑤ 정책 조회", http.MethodGet, policy, "", 200)
	if rs, _ := pol["rules"].([]any); len(rs) != 1 || rs[0].(map[string]any)["action"] != "DROP" {
		t.Fatalf("T-TR-E2E-0702: policy view: %v", pol)
	}
	_, _, raw := n.do(http.MethodGet, base+rule, "")
	if errs := spec.Check("GET", "/infra/domains/{domain-id}/security-policies/{security-policy-id}/rules/{rule-id}", 200, raw); len(errs) > 0 {
		t.Fatalf("T-TR-E2E-0702: Rule response schema: %v", errs)
	}

	// ⑥ 잘못된(오래된) Revision으로 변경 시도 — TR-E2E-0704: 스펙 응답 412 + ApiError, 상태 불변
	code, out, rawErr := n.do(http.MethodPatch, base+rule, fmt.Sprintf(`{"action":"ALLOW","source_groups":["ANY"],"destination_groups":["ANY"],"services":["ANY"],"_revision":%v}`, rev0))
	if code != http.StatusPreconditionFailed {
		t.Fatalf("T-TR-E2E-0704: stale revision → %d (%s)", code, rawErr)
	}
	if errs := spec.Check("PATCH", "/infra/domains/{domain-id}/security-policies/{security-policy-id}/rules/{rule-id}", code, rawErr); len(errs) > 0 {
		t.Fatalf("T-TR-E2E-0704: error response vs spec (412 ApiError): %v", errs)
	}
	if _, ok := out["error_message"]; !ok {
		t.Fatalf("T-TR-E2E-0704: ApiError error_message missing: %v", out)
	}
	// PUT은 _revision이 필수(없으면 412)
	if code, _, _ := n.do(http.MethodPut, base+rule, `{"action":"ALLOW","source_groups":["ANY"],"destination_groups":["ANY"],"services":["ANY"]}`); code != http.StatusPreconditionFailed {
		t.Fatalf("T-TR-E2E-0704: PUT without _revision → %d", code)
	}
	after := must("⑥ 이후 조회", http.MethodGet, rule, "", 200)
	if after["action"] != "DROP" || after["_revision"] != latest["_revision"] {
		t.Fatalf("T-TR-E2E-0704: rejected change modified state: %v", after)
	}
}
