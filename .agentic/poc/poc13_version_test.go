package poc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

const profileMax = "7.0.3.0" // 첫 기준 조합 vCenter 7.0 U3w의 API 버전

// govmomiDir는 모듈 캐시의 govmomi 소스 경로다(GOPATH 기준).
func govmomiDir() string {
	gopath := os.Getenv("GOPATH")
	return filepath.Join(gopath, "pkg", "mod", "github.com", "vmware", "govmomi@v0.56.0")
}

func cloneBody(r *http.Request) func() *http.Request {
	b, _ := io.ReadAll(r.Body)
	return func() *http.Request {
		c := r.Clone(r.Context())
		c.Body = io.NopCloser(bytes.NewReader(b))
		return c
	}
}

func viewRefs(ctx context.Context, c *govmomi.Client, root types.ManagedObjectReference, kind string) ([]types.ManagedObjectReference, error) {
	v, err := view.NewManager(c.Client).CreateContainerView(ctx, root, []string{kind}, true)
	if err != nil {
		return nil, err
	}
	defer v.Destroy(ctx)
	return v.Find(ctx, []string{kind}, nil)
}

// statsTap은 필터 전 응답(vcsim 원본)과 필터 후 응답에 남은 '세션 버전보다 새 항목'을 센다.
type statsTap struct {
	mu            sync.Mutex
	before, after filterStats
}

func (s *statsTap) wrap(next http.Handler, meta *versionMeta) http.Handler {
	filtered := versionGateway(next, profileMax, meta)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			filtered.ServeHTTP(w, r)
			return
		}
		ver := sessionVersion(r, profileMax)
		body := cloneBody(r)
		raw := httptest.NewRecorder()
		next.ServeHTTP(raw, body())
		_, b := filterXML(raw.Body.Bytes(), ver, meta, false)

		rec := httptest.NewRecorder()
		filtered.ServeHTTP(rec, body())
		_, a := filterXML(rec.Body.Bytes(), ver, meta, false)
		s.mu.Lock()
		merge(&s.before, b)
		merge(&s.after, a)
		s.mu.Unlock()

		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	})
}

func merge(dst *filterStats, src filterStats) {
	if dst.Fields == nil {
		dst.Fields, dst.Types = map[string]int{}, map[string]int{}
	}
	for k, v := range src.Fields {
		dst.Fields[k] += v
	}
	for k, v := range src.Types {
		dst.Types[k] += v
	}
	dst.Unknown += src.Unknown
}

func keys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pyVmomiTypeinfo는 extract_pyvmomi.py 출력 경로다(D11 확정 전에는 저장소 밖 로컬에만 둔다).
func pyVmomiTypeinfo() string {
	if p := os.Getenv("PYVMOMI_TYPEINFO"); p != "" {
		return p
	}
	return "D:/AISCP/tools/specs/pyvmomi-9.1.1.0/vim-typeinfo.json"
}

// PoC-13: 요청 버전별 응답 필터링.
// 7.0.3 기준 프로파일에서 urn:vim25/7.0.3.0, 8.0.3.0(프로파일보다 높은 요청 → 7.0.3으로 하향), 6.7.3 세션을 비교한다.
// 메타데이터 'govmomi'(태그·타입 맵만)와 'govmomi+pyvmomi'(필드별 전체 도입 버전 보완)를 각각 실행한다.
func TestPoC13_VersionAwareFilter(t *testing.T) {
	gov := &versionMeta{govTypes: typeMinVersion(govmomiDir())}
	full := &versionMeta{govTypes: gov.govTypes}
	if err := loadPyVmomi(pyVmomiTypeinfo(), full); err != nil {
		t.Logf("pyVmomi metadata unavailable (%v); govmomi-only mode only", err)
		full = nil
	} else {
		t.Logf("metadata: govmomi types=%d, pyVmomi types=%d", len(gov.govTypes), len(full.pyTypes))
	}

	for _, mode := range []struct {
		name string
		meta *versionMeta
	}{{"govmomi", gov}, {"govmomi+pyvmomi", full}} {
		if mode.meta == nil {
			continue
		}
		for _, sessionVer := range []string{"7.0.3.0", "8.0.3.0", "6.7.3"} {
			t.Run(mode.name+"/"+sessionVer, func(t *testing.T) {
				runPoC13(t, mode.name, mode.meta, sessionVer)
			})
		}
	}
}

func runPoC13(t *testing.T, mode string, meta *versionMeta, sessionVer string) {
	m := defaultSmall()
	// 프로파일의 버전 노출 값(FR-API-0306). vcsim 기본값은 6.5이다.
	m.ServiceContent.About.Version = "7.0.3"
	m.ServiceContent.About.Build = "24927011"
	m.ServiceContent.About.ApiVersion = profileMax
	_, vc := newVCenterHandler(t, m)
	// 양성 대조군: 버전별 도입 필드를 채운다(vcsim 기본 데이터에는 거의 없음).
	yes := types.NewBool(true)
	for _, obj := range m.Map().All("VirtualMachine") {
		vm := obj.(*simulator.VirtualMachine)
		vm.Config.SevEnabled = yes                   // 7.0.1.0
		vm.Config.VmOpNotificationToAppEnabled = yes // 7.0.3.0
		vm.Config.RebootPowerOff = yes               // 8.0.0.1
		vm.Runtime.IommuActive = yes                 // 8.0.1.0
	}
	for _, obj := range m.Map().All("DistributedVirtualPortgroup") {
		pg := obj.(*simulator.DistributedVirtualPortgroup)
		pg.Config.BackingType = "nsx" // 7.0.0.0 — govmomi 태그 없음
		pg.Config.SegmentId = "/infra/segments/poc13"
	}
	tap := &statsTap{}
	g := startGateway(t, "127.0.0.1", map[string]http.Handler{"vcenter": tap.wrap(vc, meta)})
	defer g.close()

	c := login(t, g)
	c.Client.Version = sessionVer // SOAPAction: urn:vim25/<sessionVer>
	ctx := context.Background()
	pc := property.DefaultCollector(c.Client)

	var vms []mo.VirtualMachine
	var hosts []mo.HostSystem
	var pgs []mo.DistributedVirtualPortgroup
	for _, q := range []struct {
		kind  string
		props []string
		dst   any
	}{
		{"VirtualMachine", []string{"config", "summary", "runtime", "guest", "layoutEx"}, &vms},
		{"HostSystem", []string{"config", "summary", "hardware", "runtime"}, &hosts},
		{"DistributedVirtualPortgroup", []string{"config"}, &pgs},
	} {
		refs, err := viewRefs(ctx, c, c.ServiceContent.RootFolder, q.kind)
		if err != nil {
			t.Fatal(err)
		}
		if err := pc.Retrieve(ctx, refs, q.props, q.dst); err != nil {
			t.Fatalf("retrieve %s after filter: %v", q.kind, err)
		}
	}
	t.Logf("decoded after filter: %d VMs, %d hosts, %d DV portgroups", len(vms), len(hosts), len(pgs))
	t.Logf("BEFORE filter (newer than session): fields=%v types=%v", keys(tap.before.Fields), keys(tap.before.Types))
	t.Logf("AFTER  filter: newer fields=%d types=%d, untracked elements=%d", len(tap.after.Fields), len(tap.after.Types), tap.after.Unknown)

	// 세션 버전별 기대값: 세션 버전 이하에서 도입된 필드만 남는다.
	eff := profileMax
	if newer(profileMax, sessionVer) {
		eff = sessionVer
	}
	vm := vms[0]
	if len(pgs) == 0 {
		t.Fatal("no DV portgroups")
	}
	pg := pgs[0] // 대조군 값은 모든 포트그룹에 채웠다
	want := map[string]bool{
		"sevEnabled(7.0.1.0)":                   !newer("7.0.1.0", eff),
		"vmOpNotificationToAppEnabled(7.0.3.0)": !newer("7.0.3.0", eff),
		"rebootPowerOff(8.0.0.1)":               !newer("8.0.0.1", eff),
		"iommuActive(8.0.1.0)":                  !newer("8.0.1.0", eff),
		"dvpg.backingType(7.0.0.0)":             !newer("7.0.0.0", eff),
		"dvpg.segmentId(7.0.0.0)":               !newer("7.0.0.0", eff),
	}
	got := map[string]bool{
		"sevEnabled(7.0.1.0)":                   vm.Config.SevEnabled != nil,
		"vmOpNotificationToAppEnabled(7.0.3.0)": vm.Config.VmOpNotificationToAppEnabled != nil,
		"rebootPowerOff(8.0.0.1)":               vm.Config.RebootPowerOff != nil,
		"iommuActive(8.0.1.0)":                  vm.Runtime.IommuActive != nil,
		"dvpg.backingType(7.0.0.0)":             pg.Config.BackingType != "",
		"dvpg.segmentId(7.0.0.0)":               pg.Config.SegmentId != "",
	}
	var gaps []string
	for k := range want {
		if want[k] != got[k] {
			if mode == "govmomi" && strings.HasPrefix(k, "dvpg.") {
				gaps = append(gaps, k) // 알려진 공백: govmomi에는 7.0.0.0 필드 태그가 없다
				continue
			}
			t.Errorf("session %s (effective %s): field %s present=%v, want %v", sessionVer, eff, k, got[k], want[k])
		}
	}
	sort.Strings(gaps)
	t.Logf("[%s] effective session %s, present=%v, known metadata gaps=%v", mode, eff, got, gaps)
	if len(tap.after.Fields)+len(tap.after.Types) != 0 {
		t.Fatalf("leftover newer items: %v %v", tap.after.Fields, tap.after.Types)
	}
}
