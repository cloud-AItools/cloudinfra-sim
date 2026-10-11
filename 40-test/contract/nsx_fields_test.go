// NSX 리소스 공통 필드·스키마 계약 (FR-NSX-0301 ~ FR-NSX-0303, FR-NSX-0416). 근거: SRC-NSX-321(Segment 정의의 readonly·enum).
package contract

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"cloudinfra-sim/test/internal/testenv"
)

func nsxDo(t *testing.T, e *testenv.Env, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, "https://nsx."+testenv.Domain+path, strings.NewReader(body))
	u := e.User("nsx")
	req.SetBasicAuth(u.Name, u.Password)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// T-FR-NSX-0301 T-FR-NSX-0302 T-FR-NSX-0303 T-FR-NSX-0416 T-FR-NSX-0407
func TestNSXCommonFieldsAndSchema(t *testing.T) {
	e := testenv.Start(t, nil)
	code, _ := nsxDo(t, e, http.MethodPatch, "/policy/api/v1/infra/segments/web", `{"display_name":"web","admin_state":"UP"}`)
	if code != 200 {
		t.Fatalf("create: %d", code)
	}
	_, seg := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments/web", "")
	for _, f := range []string{"id", "display_name", "path", "relative_path", "parent_path", "_revision", "_create_time", "_last_modified_time", "_create_user", "_last_modified_user", "unique_id"} {
		if _, ok := seg[f]; !ok {
			t.Fatalf("common field %s missing: %v", f, seg)
		}
	}
	if seg["path"] != "/infra/segments/web" || seg["parent_path"] != "/infra/segments" || seg["id"] == seg["unique_id"] {
		t.Fatalf("path/id/unique_id: %v", seg)
	}
	// 스펙 readonly(unique_id)·enum(admin_state) 위반은 ApiError 형식의 400
	code, apiErr := nsxDo(t, e, http.MethodPatch, "/policy/api/v1/infra/segments/web", `{"unique_id":"x","admin_state":"SIDEWAYS"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("schema violation: %d", code)
	}
	for _, f := range []string{"error_code", "error_message", "module_name"} {
		if _, ok := apiErr[f]; !ok {
			t.Fatalf("ApiError field %s missing: %v", f, apiErr)
		}
	}
	if !strings.Contains(apiErr["error_message"].(string), "read-only") || !strings.Contains(apiErr["error_message"].(string), "enum") {
		t.Fatalf("error detail: %v", apiErr)
	}
	// 리소스별 필드 차이: Tier1 스키마에는 Segment의 admin_state가 없으므로 검증 대상이 아니다(무시되지 않고 거부되지도 않음)
	code, _ = nsxDo(t, e, http.MethodPatch, "/policy/api/v1/infra/tier-1s/t1-x", `{"display_name":"t1"}`)
	if code != 200 {
		t.Fatalf("tier1 create: %d", code)
	}
}
