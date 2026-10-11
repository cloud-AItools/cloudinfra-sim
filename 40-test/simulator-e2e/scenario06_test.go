// Scenario 06 — NSX Segment 생성 (PRD-2 TR-E2E-0601 ~ TR-E2E-0604, Phase 2a 종료 게이트).
package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloudinfra-sim/test/internal/testenv"
)

type nsx struct {
	t *testing.T
	e *testenv.Env
}

func (n nsx) do(method, path, body string, headers ...string) (int, map[string]any, []byte) {
	n.t.Helper()
	req, _ := http.NewRequest(method, "https://nsx."+testenv.Domain+path, strings.NewReader(body))
	u := n.e.User("nsx")
	req.SetBasicAuth(u.Name, u.Password)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := n.e.Client.Do(req)
	if err != nil {
		n.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out, raw
}

func (n nsx) realization(path string) string {
	n.t.Helper()
	code, o, _ := n.do(http.MethodGet, "/policy/api/v1/infra/realized-state/status?intent_path="+path, "")
	if code != 200 {
		return http.StatusText(code)
	}
	return o["consolidated_status"].(map[string]any)["consolidated_status"].(string)
}

// TR-E2E-0601 절차, 성공 기준 0602 ~ 0604.
// 추적: T-TR-E2E-0601 T-TR-E2E-0602 T-TR-E2E-0603 T-TR-E2E-0604 T-FR-NSX-0211 T-FR-NSX-0401 T-FR-NSX-0403 T-FR-NSX-0404 T-FR-NSX-0405 T-FR-NSX-0408 T-FR-NSX-0411 T-FR-NSX-0412 T-FR-NSX-0416 T-FR-NSX-0501 T-FR-NSX-0502 T-FR-NSX-0503 T-FR-NSX-0509 T-FR-NSX-0220 T-FR-BE-0606 T-FR-NSX-0107 T-FR-NSX-0108 T-FR-BE-0601
func TestScenario06_SegmentCreate(t *testing.T) {
	e := testenv.Start(t, nil)
	n := nsx{t, e}
	const seg = "/infra/segments/web"
	body := `{"subnets":[{"gateway_address":"10.10.0.1/24"}],"transport_zone_path":"/infra/sites/default/enforcement-points/default/transport-zones/tz-overlay","connectivity_path":"/infra/tier-1s/t1-a"}`

	// ① PATCH 생성 ② 성공 응답: 200, 본문 없음
	code, _, raw := n.do(http.MethodPatch, "/policy/api/v1"+seg, body)
	if code != 200 || len(raw) != 0 {
		t.Fatalf("T-TR-E2E-0602: PATCH create %d body=%q", code, raw)
	}
	// ③ GET: 객체·Path·Revision, 스펙 기본값(display_name=ID, admin_state=UP)
	code, o, _ := n.do(http.MethodGet, "/policy/api/v1"+seg, "")
	if code != 200 || o["path"] != seg || o["parent_path"] != "/infra/segments" || o["_revision"] != float64(0) ||
		o["display_name"] != "web" || o["admin_state"] != "UP" {
		t.Fatalf("T-TR-E2E-0603: %d %v", code, o)
	}
	// ④ 즉시 완료 Realization ⑤ 재조회
	if s := n.realization(seg); s != "SUCCESS" {
		t.Fatalf("T-FR-NSX-0502: immediate realization %s", s)
	}

	// 설정 지연 후 완료(FR-NSX-0503, FR-BE-0606): Admin API로 지연 설정
	res, _ := e.AdminDo(http.MethodPut, "/v1/nsx/realization-delay", strings.NewReader(`{"ms":700}`))
	res.Body.Close()
	const seg2 = "/infra/segments/app"
	if code, _, _ := n.do(http.MethodPatch, "/policy/api/v1"+seg2, `{"display_name":"app"}`); code != 200 {
		t.Fatalf("create app: %d", code)
	}
	first := n.realization(seg2)
	time.Sleep(900 * time.Millisecond)
	second := n.realization(seg2)
	if first != "IN_PROGRESS" || second != "SUCCESS" {
		t.Fatalf("T-TR-E2E-0604: delayed realization %s -> %s", first, second)
	}
	res, _ = e.AdminDo(http.MethodPut, "/v1/nsx/realization-delay", strings.NewReader(`{"ms":0}`))
	res.Body.Close()

	// PATCH 의미(FR-NSX-0405, TR-E2E-0602): 기본(부분 PATCH 비활성)은 본문을 객체 전체로 본다
	n.do(http.MethodPatch, "/policy/api/v1"+seg, `{"description":"v1"}`)
	_, o, _ = n.do(http.MethodGet, "/policy/api/v1"+seg, "")
	if o["description"] != "v1" || o["connectivity_path"] != nil || o["display_name"] != "web" || o["_revision"] != float64(1) {
		t.Fatalf("partial patch disabled (default): %v", o)
	}
	// 요청 헤더로 부분 PATCH 활성: 지정 필드만 바뀌고 나머지 유지, 중첩 객체는 병합
	n.do(http.MethodPatch, "/policy/api/v1"+seg, `{"connectivity_path":"/infra/tier-1s/t1-a","advanced_config":{"connectivity":"ON"}}`, "nsx-enable-partial-patch", "true")
	n.do(http.MethodPatch, "/policy/api/v1"+seg, `{"advanced_config":{"local_egress":true}}`, "nsx-enable-partial-patch", "true")
	_, o, _ = n.do(http.MethodGet, "/policy/api/v1"+seg, "")
	ac, _ := o["advanced_config"].(map[string]any)
	if o["description"] != "v1" || o["connectivity_path"] != "/infra/tier-1s/t1-a" || ac["connectivity"] != "ON" || ac["local_egress"] != true {
		t.Fatalf("partial patch via header: %v", o)
	}
	// 시스템 설정(스펙 /system-config/nsx-partial-patch-config)과 헤더 우선순위
	code, cfg, _ := n.do(http.MethodGet, "/policy/api/v1/system-config/nsx-partial-patch-config", "")
	if code != 200 || cfg["enable_partial_patch"] != false {
		t.Fatalf("partial patch config default: %d %v", code, cfg)
	}
	n.do(http.MethodPatch, "/policy/api/v1/system-config/nsx-partial-patch-config", `{"enable_partial_patch":true}`)
	n.do(http.MethodPatch, "/policy/api/v1"+seg, `{"description":"v2"}`)
	_, o, _ = n.do(http.MethodGet, "/policy/api/v1"+seg, "")
	if o["description"] != "v2" || o["connectivity_path"] == nil {
		t.Fatalf("system partial patch enabled: %v", o)
	}
	n.do(http.MethodPatch, "/policy/api/v1"+seg, `{"description":"v3"}`, "nsx-enable-partial-patch", "false")
	_, o, _ = n.do(http.MethodGet, "/policy/api/v1"+seg, "")
	if o["description"] != "v3" || o["connectivity_path"] != nil {
		t.Fatalf("header false must override system setting: %v", o)
	}

	// PUT: 전체 교체, 기존 객체에는 현재 _revision 필요(SRC-NSX-321)
	if code, _, _ := n.do(http.MethodPut, "/policy/api/v1"+seg, `{"display_name":"web-put"}`); code != http.StatusPreconditionFailed {
		t.Fatalf("PUT without _revision: %d", code)
	}
	rev := o["_revision"].(float64)
	if code, _, _ := n.do(http.MethodPut, "/policy/api/v1"+seg, `{"display_name":"web-put","_revision":`+jsonNum(rev)+`}`); code != 200 {
		t.Fatalf("PUT with current _revision: %d", code)
	}
	_, o, _ = n.do(http.MethodGet, "/policy/api/v1"+seg, "")
	if o["display_name"] != "web-put" || o["description"] != nil || o["_revision"] != rev+1 {
		t.Fatalf("PUT replace: %v", o)
	}
}

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }

// 추적: T-FR-NSX-0402 T-FR-NSX-0414 T-FR-NSX-0406 T-FR-NSX-0508 T-FR-NSX-0219
func TestScenario06_ListPaginationDeleteTags(t *testing.T) {
	e := testenv.Start(t, nil)
	n := nsx{t, e}
	// 태그(스펙 Tag: scope·tag)
	if code, _, raw := n.do(http.MethodPatch, "/policy/api/v1/infra/segments/tagged", `{"tags":[{"scope":"env","tag":"prod"}]}`); code != 200 {
		t.Fatalf("tags: %d %s", code, raw)
	}
	if code, _, _ := n.do(http.MethodPatch, "/policy/api/v1/infra/segments/badtag", `{"tags":[{"scope":1}]}`); code != http.StatusBadRequest {
		t.Fatalf("invalid tag type must be 400: %d", code)
	}
	// 목록 페이지 조회: cursor로 끝까지
	seen := map[string]bool{}
	cursor, pages, total := "", 0, 0
	for {
		path := "/policy/api/v1/infra/segments?page_size=4"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		code, o, _ := n.do(http.MethodGet, path, "")
		if code != 200 {
			t.Fatalf("list: %d", code)
		}
		total = int(o["result_count"].(float64))
		for _, r := range o["results"].([]any) {
			seen[r.(map[string]any)["id"].(string)] = true
		}
		pages++
		c, ok := o["cursor"].(string)
		if !ok {
			break
		}
		cursor = c
	}
	if len(seen) != total || total != 11 || pages != 3 {
		t.Fatalf("pagination: seen=%d total=%d pages=%d", len(seen), total, pages)
	}
	// 삭제 → 조회 404, Realization 조회 404(가정)
	if code, _, _ := n.do(http.MethodDelete, "/policy/api/v1/infra/segments/tagged", ""); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if code, _, _ := n.do(http.MethodGet, "/policy/api/v1/infra/segments/tagged", ""); code != 404 {
		t.Fatalf("get after delete: %d", code)
	}
	if s := n.realization("/infra/segments/tagged"); s != http.StatusText(404) {
		t.Fatalf("realization after delete: %s", s)
	}
}
