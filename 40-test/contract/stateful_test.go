// 상태 관리·ID·동시성 (WP 2.3 상태 관리, WP 2.4 Resource ID, WP 2.6 저장소 무결성).
package contract

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ID 종류를 구분하고 하나의 UUID를 모든 ID로 돌려주지 않는다(DATA_MODEL §4).
// 추적: T-FR-DM-0201 T-FR-DM-0202 T-NFR-0203
func TestIDKinds(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	refs := make([]types.ManagedObjectReference, 0, len(vms))
	for _, v := range vms {
		refs = append(refs, v.Reference())
	}
	var mvs []mo.VirtualMachine
	if err := property.DefaultCollector(c.Client).Retrieve(ctx, refs, []string{"config.uuid", "config.instanceUuid"}, &mvs); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, vm := range mvs {
		mor, bios, inst := vm.Self.Value, vm.Config.Uuid, vm.Config.InstanceUuid
		if !regexp.MustCompile(`^vm-[0-9]+$`).MatchString(mor) || !uuidRe.MatchString(bios) || !uuidRe.MatchString(inst) || bios == inst {
			t.Fatalf("VM ID kinds: moref=%s bios=%s instance=%s", mor, bios, inst)
		}
		for _, id := range []string{bios, inst} {
			if other, dup := seen[id]; dup {
				t.Fatalf("UUID %s shared by %s and %s", id, other, mor)
			}
			seen[id] = mor
		}
	}
	// NSX: 경로 ID·Policy Path·시스템 발급 unique_id는 서로 다르다
	nsxDo(t, e, http.MethodPatch, "/policy/api/v1/infra/segments/idk", `{}`)
	_, seg := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments/idk", "")
	if seg["id"] != "idk" || seg["path"] != "/infra/segments/idk" || !uuidRe.MatchString(fmt.Sprint(seg["unique_id"])) {
		t.Fatalf("NSX ID kinds: %v", seg)
	}
	if _, dup := seen[fmt.Sprint(seg["unique_id"])]; dup {
		t.Fatal("NSX unique_id reuses a VM UUID")
	}
}

// 동시 요청에도 저장 상태가 일관되고, 같은 _revision으로 동시에 PUT하면 하나만 성공한다.
// 추적: T-NFR-0201 T-FR-BE-0205 T-FR-DM-0305 T-NFR-ARC-0106
func TestNSXConcurrentWrites(t *testing.T) {
	e := testenv.Start(t, nil)
	const n = 16
	base := "/policy/api/v1/infra/segments/"
	nsxDo(t, e, http.MethodPatch, base+"shared", `{"display_name":"shared"}`)
	rows0, _ := e.Sim.Store.Count(context.Background(), "nsx_resources")
	var wg sync.WaitGroup
	codes := make(chan int, 3*n)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			code, _ := nsxDo(t, e, http.MethodPatch, fmt.Sprintf("%sc-%02d", base, i), `{}`)
			codes <- code
		}(i)
		go func(i int) {
			defer wg.Done()
			code, _ := nsxDo(t, e, http.MethodPatch, base+"shared", fmt.Sprintf(`{"display_name":"shared","description":"w%d"}`, i))
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != 200 {
			t.Fatalf("concurrent write: %d", c)
		}
	}
	_, shared := nsxDo(t, e, http.MethodGet, base+"shared", "")
	if shared["_revision"] != float64(n) {
		t.Fatalf("lost update: _revision=%v want %d", shared["_revision"], n)
	}
	_, list := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments?page_size=1000", "")
	if got := int(list["result_count"].(float64)); got != 10+1+n {
		t.Fatalf("segments %d, want %d", got, 10+1+n)
	}
	// 저장소(SQLite)와 응답 상태가 같다: 동시 생성한 Segment 수만큼만 행이 늘어난다
	rows, err := e.Sim.Store.Count(context.Background(), "nsx_resources")
	if err != nil {
		t.Fatal(err)
	}
	if rows != rows0+n {
		t.Fatalf("nsx_resources rows %d, want %d", rows, rows0+n)
	}

	// 같은 _revision으로 동시 PUT: 하나만 200, 나머지 412(낙관적 동시성)
	rev := shared["_revision"].(float64)
	results := make(chan int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _ := nsxDo(t, e, http.MethodPut, base+"shared", fmt.Sprintf(`{"display_name":"put-%d","_revision":%v}`, i, rev))
			results <- code
		}(i)
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for c := range results {
		switch c {
		case 200:
			ok++
		case http.StatusPreconditionFailed:
			conflict++
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Fatalf("concurrent PUT: ok=%d conflict=%d", ok, conflict)
	}
}

// 반복 호출은 실제 제품의 멱등성 규칙을 따른다: NSX PATCH는 같은 결과(upsert), 이미 켜진 VM 전원 켜기는 InvalidPowerState.
// 추적: T-FR-BE-0204
func TestIdempotency(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if code, _ := nsxDo(t, e, http.MethodPatch, "/policy/api/v1/infra/segments/idem", `{"display_name":"idem"}`); code != 200 {
			t.Fatalf("PATCH #%d: %d", i, code)
		}
	}
	_, seg := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments/idem", "")
	if seg["display_name"] != "idem" {
		t.Fatalf("repeated PATCH: %v", seg)
	}

	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	for _, vm := range vms {
		if st, _ := vm.PowerState(ctx); st != types.VirtualMachinePowerStatePoweredOn {
			continue
		}
		tk, err := vm.PowerOn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tk.WaitForResult(ctx, nil)
		te, ok := err.(task.Error)
		if !ok {
			t.Fatalf("PowerOn on powered-on VM: want task error, got %v", err)
		}
		if _, ok := te.Fault().(*types.InvalidPowerState); !ok {
			t.Fatalf("PowerOn on powered-on VM: %T", te.Fault())
		}
		return
	}
	t.Fatal("no powered-on VM in fixture")
}

// 환경 간 ID Namespace 분리(FR-DM-0207): 같은 Seed라도 Namespace가 다르면 VM UUID·NSX unique_id가 겹치지 않고,
// 같은 Namespace는 초기화 뒤에도 같은 ID를 만든다(FR-DM-0210).
// 추적: T-FR-DM-0207 T-FR-DM-0210
func TestEnvironmentNamespace(t *testing.T) {
	ids := func(e *testenv.Env) (map[string]bool, string) {
		ctx := context.Background()
		c := e.Login("")
		f := find.NewFinder(c.Client, true)
		dc, _ := f.DefaultDatacenter(ctx)
		f.SetDatacenter(dc)
		vms, _ := f.VirtualMachineList(ctx, "*")
		refs := make([]types.ManagedObjectReference, 0, len(vms))
		for _, v := range vms {
			refs = append(refs, v.Reference())
		}
		var mvs []mo.VirtualMachine
		_ = property.DefaultCollector(c.Client).Retrieve(ctx, refs, []string{"config.instanceUuid", "config.uuid"}, &mvs)
		out := map[string]bool{}
		for _, m := range mvs {
			out[m.Config.InstanceUuid], out[m.Config.Uuid] = true, true
		}
		_, seg := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments/seg-01", "")
		return out, fmt.Sprint(seg["unique_id"])
	}
	a := testenv.Start(t, nil)
	aIDs, aSeg := ids(a)
	res, _ := a.AdminDo(http.MethodPost, "/v1/reset", nil)
	res.Body.Close()
	if again, seg := ids(a); seg != aSeg || len(again) != len(aIDs) {
		t.Fatalf("same namespace must reproduce IDs after reset")
	} else {
		for id := range again {
			if !aIDs[id] {
				t.Fatalf("UUID %s changed after reset", id)
			}
		}
	}
	// 같은 Seed, 다른 Namespace인 Fixture
	src, _ := os.ReadFile(filepath.Join(a.Repo, "30-implementation", "data", "fixtures", "default-small.yaml"))
	other := filepath.Join(t.TempDir(), "other.yaml")
	_ = os.WriteFile(other, []byte(strings.Replace(string(src), "namespace: default-small", "namespace: team-b", 1)), 0o644)
	b := testenv.Start(t, func(c *config.Config) { c.FixtureFile = other })
	bIDs, bSeg := ids(b)
	if bSeg == aSeg || len(bIDs) != len(aIDs) {
		t.Fatalf("namespace must separate NSX IDs: %s %s", aSeg, bSeg)
	}
	for id := range bIDs {
		if aIDs[id] {
			t.Fatalf("UUID %s shared across namespaces", id)
		}
	}
}

// 복구 규칙(FR-DM-0403, FR-DM-0402 Reconciliation): vCenter에서 NSX DVPG를 직접 지워도 한쪽만 남은 상태가
// 영구히 남지 않는다 — vcsim 변경 신호로 조정이 돌아 Segment의 DVPG를 다시 만든다.
// 추적: T-FR-DM-0403 T-FR-DM-0402
func TestReconcileRepairsDrift(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	find := func() (types.ManagedObjectReference, bool) {
		v, err := view.NewManager(c.Client).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"DistributedVirtualPortgroup"}, true)
		if err != nil {
			t.Fatal(err)
		}
		defer v.Destroy(ctx)
		var pgs []mo.DistributedVirtualPortgroup
		_ = v.Retrieve(ctx, []string{"DistributedVirtualPortgroup"}, []string{"config"}, &pgs)
		for _, pg := range pgs {
			if pg.Config.SegmentId == "/infra/segments/seg-03" {
				return pg.Self, true
			}
		}
		return types.ManagedObjectReference{}, false
	}
	ref, ok := find()
	if !ok {
		t.Fatal("fixture segment seg-03 must have an NSX DVPG")
	}
	tk, err := object.NewDistributedVirtualPortgroup(c.Client, ref).Destroy(ctx)
	if err == nil {
		err = tk.Wait(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "DVPG recreated by reconciliation", func() bool {
		r, ok := find()
		return ok && r != ref
	})
}
