package poc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"cloudinfra-sim/poc/nsxspec"
)

type nsxClient struct {
	t  *testing.T
	hc *http.Client
}

func (c *nsxClient) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "https://nsx."+simDomain+"/policy/api/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func newNSX(t *testing.T, delay time.Duration) (*nsxClient, func()) {
	t.Helper()
	dir := nsxSpecDir()
	var specs []*nsxspec.Spec
	for _, f := range []string{"nsx_api.json", "nsx_policy_api.json"} {
		s, err := nsxspec.Load(filepath.Join(dir, f))
		if err != nil {
			t.Skipf("NSX spec not available: %v", err)
		}
		specs = append(specs, s)
	}
	h := nsxspec.NewHandler(specs...)
	h.RealizationDelay = delay
	g := startGateway(t, "127.0.0.1", map[string]http.Handler{"nsx": h})
	return &nsxClient{t: t, hc: &http.Client{Transport: g.transport()}}, g.close
}

// PoC-08 Segment CRUD, PoC-09 Revision 충돌, PoC-10 Realization 상태 전이.
func TestPoC08_09_10_SegmentRevisionRealization(t *testing.T) {
	c, done := newNSX(t, 500*time.Millisecond)
	defer done()
	seg := "/infra/segments/web"

	// PoC-08 생성(PATCH upsert) → 조회 → 수정 → 삭제 → 삭제 후 조회
	if code, _ := c.do(http.MethodPatch, seg, map[string]any{"display_name": "web", "admin_state": "UP"}); code != 200 {
		t.Fatalf("create: %d", code)
	}
	code, obj := c.do(http.MethodGet, seg, nil)
	if code != 200 || obj["_revision"] != float64(0) || obj["path"] != seg {
		t.Fatalf("get: %d %v", code, obj)
	}

	// PoC-10 설정 지연 동안 IN_PROGRESS, 이후 SUCCESS
	status := func() string {
		code, o := c.do(http.MethodGet, "/infra/realized-state/status?intent_path="+seg, nil)
		if code != 200 {
			return "HTTP " + itoaSafe(code)
		}
		return o["consolidated_status"].(map[string]any)["consolidated_status"].(string)
	}
	first := status()
	time.Sleep(700 * time.Millisecond)
	second := status()
	t.Logf("PoC-10 realization: right after change=%s, after delay=%s", first, second)
	if first != "IN_PROGRESS" || second != "SUCCESS" {
		t.Fatalf("realization transition %s → %s", first, second)
	}

	// PoC-09 Revision: PUT에 _revision 없음 → 412, 오래된 _revision → 412, 최신 → 200
	code, _ = c.do(http.MethodPut, seg, map[string]any{"display_name": "web2"})
	t.Logf("PoC-09 PUT without _revision → %d", code)
	if code != http.StatusPreconditionFailed {
		t.Fatalf("want 412, got %d", code)
	}
	if code, _ = c.do(http.MethodPatch, seg, map[string]any{"description": "v1", "_revision": float64(0)}); code != 200 {
		t.Fatalf("patch with current revision: %d", code)
	}
	code, _ = c.do(http.MethodPatch, seg, map[string]any{"description": "stale", "_revision": float64(0)})
	t.Logf("PoC-09 PATCH with stale _revision 0 (current 1) → %d", code)
	if code != http.StatusPreconditionFailed {
		t.Fatalf("want 412, got %d", code)
	}
	if code, _ = c.do(http.MethodPut, seg, map[string]any{"display_name": "web3", "_revision": float64(1)}); code != 200 {
		t.Fatalf("PUT with current revision: %d", code)
	}
	_, obj = c.do(http.MethodGet, seg, nil)
	t.Logf("PoC-09 after PUT: _revision=%v description=%v (PUT replaces)", obj["_revision"], obj["description"])
	if obj["_revision"] != float64(2) || obj["description"] != nil {
		t.Fatalf("PUT semantics: %v", obj)
	}

	// PoC-08 삭제
	if code, _ = c.do(http.MethodDelete, seg, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	code, _ = c.do(http.MethodGet, seg, nil)
	t.Logf("PoC-08 GET after delete → %d; realization status after delete → %s", code, status())
}

// PoC-16 Hierarchical API: 여러 리소스를 한 번에 생성·삭제, 하나라도 실패하면 전체 미반영.
func TestPoC16_HierarchicalAPI(t *testing.T) {
	c, done := newNSX(t, 0)
	defer done()
	body := map[string]any{"resource_type": "Infra", "children": []any{
		map[string]any{"resource_type": "ChildTier1", "Tier1": map[string]any{"id": "t1-a", "resource_type": "Tier1", "display_name": "t1-a"}},
		map[string]any{"resource_type": "ChildSegment", "Segment": map[string]any{"id": "s1", "resource_type": "Segment", "connectivity_path": "/infra/tier-1s/t1-a"}},
		map[string]any{"resource_type": "ChildSegment", "Segment": map[string]any{"id": "s2", "resource_type": "Segment"}},
	}}
	if code, o := c.do(http.MethodPatch, "/infra", body); code != 200 {
		t.Fatalf("H-API create: %d %v", code, o)
	}
	for _, p := range []string{"/infra/tier-1s/t1-a", "/infra/segments/s1", "/infra/segments/s2"} {
		if code, _ := c.do(http.MethodGet, p, nil); code != 200 {
			t.Fatalf("%s not created: %d", p, code)
		}
	}
	// 삭제(marked_for_delete) + 잘못된 자식 하나 → 400이며 s2는 삭제되지 않아야 한다(원자성).
	bad := map[string]any{"resource_type": "Infra", "children": []any{
		map[string]any{"resource_type": "ChildSegment", "marked_for_delete": true, "Segment": map[string]any{"id": "s2", "resource_type": "Segment"}},
		map[string]any{"resource_type": "ChildSegment", "Segment": map[string]any{"id": "s3", "resource_type": "Segment", "admin_state": "SIDEWAYS"}},
	}}
	code, o := c.do(http.MethodPatch, "/infra", bad)
	t.Logf("PoC-16 H-API with one invalid child → %d %v", code, o["error_message"])
	if code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", code)
	}
	if code, _ := c.do(http.MethodGet, "/infra/segments/s2", nil); code != 200 {
		t.Fatal("s2 deleted despite failed batch (atomicity broken)")
	}
	// 정상 삭제
	del := map[string]any{"resource_type": "Infra", "children": []any{
		map[string]any{"resource_type": "ChildSegment", "marked_for_delete": true, "Segment": map[string]any{"id": "s2", "resource_type": "Segment"}},
	}}
	if code, _ := c.do(http.MethodPatch, "/infra", del); code != 200 {
		t.Fatalf("H-API delete: %d", code)
	}
	code, _ = c.do(http.MethodGet, "/infra/segments/s2", nil)
	t.Logf("PoC-16 after marked_for_delete: GET s2 → %d", code)
	if code != http.StatusNotFound {
		t.Fatal("s2 not deleted")
	}
}

func itoaSafe(i int) string { b, _ := json.Marshal(i); return string(b) }
