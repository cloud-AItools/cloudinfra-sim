package poc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"cloudinfra-sim/poc/nsxspec"
)

// nsxSpecDir는 NSX-T 3.2.1 공식 스펙 위치다(D11 확정 전에는 저장소 밖 로컬에만 둔다).
func nsxSpecDir() string {
	if p := os.Getenv("NSX_SPEC_DIR"); p != "" {
		return p
	}
	return "D:/AISCP/tools/specs/nsx-t/3.2.1"
}

func heapMB() float64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc) / 1e6
}

// PoC-17: NSX 스펙 기반 범용 핸들러의 기동 시간·메모리(지연 컴파일).
func TestPoC17_NSXSpecHandler(t *testing.T) {
	dir := nsxSpecDir()
	files := []string{filepath.Join(dir, "nsx_api.json"), filepath.Join(dir, "nsx_policy_api.json")}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("NSX spec not available: %v", err)
		}
	}

	// 1) 지연 방식: 경로 색인만 만들고 스키마는 호출 시 컴파일.
	base := heapMB()
	start := time.Now()
	var specs []*nsxspec.Spec
	for _, f := range files {
		s, err := nsxspec.Load(f)
		if err != nil {
			t.Fatal(err)
		}
		specs = append(specs, s)
	}
	lazyLoad := time.Since(start)
	lazyHeap := heapMB() - base
	nPaths, nDefs := 0, 0
	for _, s := range specs {
		nPaths += len(s.Paths)
		nDefs += len(s.Definitions)
	}
	t.Logf("LAZY: load %v, heap +%.1f MB, %d paths indexed, %d definitions (0 compiled)", lazyLoad.Round(time.Millisecond), lazyHeap, nPaths, nDefs)

	// 2) 비교: 전체를 범용 객체로 디코드(정적 모델 생성에 가까운 비용의 하한).
	base2 := heapMB()
	start = time.Now()
	var full []any
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		full = append(full, v)
	}
	fullLoad := time.Since(start)
	fullHeap := heapMB() - base2
	runtime.KeepAlive(full)
	t.Logf("FULL DECODE (comparison): load %v, heap +%.1f MB", fullLoad.Round(time.Millisecond), fullHeap)
	full = nil

	// 3) 경로 색인 정확도: 모든 스펙 경로의 매개변수를 값으로 바꿔도 자기 자신으로 매칭되어야 한다.
	// 쿼리 조건부 템플릿('?action=start')은 같은 쿼리로 요청해야 해당 템플릿으로 매칭된다.
	param := regexp.MustCompile(`\{[^}]+\}`)
	miss, withQuery := 0, 0
	for _, s := range specs {
		for p := range s.Paths {
			pathPart, query, _ := strings.Cut(p, "?")
			if query != "" {
				withQuery++
			}
			q, _ := neturl.ParseQuery(query)
			got, ok := s.Match(param.ReplaceAllString(pathPart, "x1"), q)
			if !ok || !slices.Contains(got, p) {
				miss++
				t.Logf("unresolved: %s (got %v)", p, got)
			}
		}
	}
	t.Logf("path index: %d/%d spec paths resolve (incl. %d query-conditioned templates)", nPaths-miss, nPaths, withQuery)
	if miss > 0 {
		t.Errorf("%d paths did not resolve", miss)
	}

	// 4) Gateway(SNI 'nsx') 경유 Segment 생성·조회·검증.
	h := nsxspec.NewHandler(specs...)
	g := startGateway(t, "127.0.0.1", map[string]http.Handler{"nsx": h})
	defer g.close()
	hc := &http.Client{Transport: g.transport()}
	url := "https://nsx." + simDomain + "/policy/api/v1/infra/segments/poc17-seg"
	do := func(method string, body any) (int, []byte) {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, url, rd)
		req.Header.Set("Content-Type", "application/json")
		res, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, b
	}

	seg := map[string]any{
		"display_name": "poc17-seg",
		"subnets":      []any{map[string]any{"gateway_address": "10.10.0.1/24"}},
		"admin_state":  "UP",
	}
	if code, b := do(http.MethodPatch, seg); code != http.StatusOK || len(b) != 0 {
		t.Fatalf("PATCH valid: %d %s", code, b)
	}
	code, b := do(http.MethodGet, nil)
	if code != http.StatusOK || !strings.Contains(string(b), `"_revision":0`) {
		t.Fatalf("GET: %d %s", code, b)
	}
	_, _ = do(http.MethodPatch, map[string]any{"description": "updated"})
	_, b = do(http.MethodGet, nil)
	if !strings.Contains(string(b), `"_revision":1`) {
		t.Fatalf("revision not incremented: %s", b)
	}

	// 스펙에서 읽기전용 필드 하나와 enum 위반으로 검증 실패를 확인한다.
	var policy *nsxspec.Spec
	for _, s := range specs {
		if s.BasePath == "/policy/api/v1" {
			policy = s
		}
	}
	segSchema := policy.Compile("Segment")
	var readOnly []string
	for k, p := range segSchema.Properties {
		if p.ReadOnly {
			readOnly = append(readOnly, k)
		}
	}
	sort.Strings(readOnly)
	t.Logf("Segment schema: %d properties, read-only from spec: %v", len(segSchema.Properties), readOnly)
	bad := map[string]any{"admin_state": "SIDEWAYS"}
	if len(readOnly) > 0 {
		bad[readOnly[0]] = "x"
	}
	code, b = do(http.MethodPatch, bad)
	t.Logf("PATCH invalid → %d %s", code, strings.TrimSpace(string(b)))
	if code != http.StatusBadRequest {
		t.Fatalf("invalid payload accepted: %d", code)
	}

	// 경로 밖 요청은 성공으로 위장하지 않는다(FR-API-0401).
	req, _ := http.NewRequest(http.MethodGet, "https://nsx."+simDomain+"/policy/api/v1/orgs/default/projects", nil)
	res, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	t.Logf("NSX 4.x-only path /orgs/default/projects → %d", res.StatusCode)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("4.x path should not exist in 3.2.1 profile: %d", res.StatusCode)
	}

	compiled := 0
	for _, s := range specs {
		compiled += s.CompiledCount()
	}
	t.Logf("schemas compiled after requests: %d of %d definitions; heap now +%.1f MB over baseline", compiled, nDefs, heapMB()-base)
}
