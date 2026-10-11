// 선정 SDK(govmomi v0.56.0) 자체 계약 테스트 + 스키마 대조 검사기 (PRD-2 TR-COV-0304, TR-COV-0301, AC-17, AC-16 MVP 범위).
// MVP 실행 범위의 SOAP 호출(VC-SOAP-001 ~ 013)을 govmomi로 수행하고, 오가는 모든 응답을 공식 계약
// (SRC-VIM-TYPEINFO)에 대조한다: 네임스페이스, xsi:type, 속성 이름·선언 순서·도입 버전·필수 여부, 열거값, 기본형 형식,
// MethodName·TypeName, 관리 객체 참조, 선언 Fault. 기준 세션(7.0.3.0)과 협상 버전을 낮춘 세션(6.7.3)에서 모두 수행한다.
package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
	"cloudinfra-sim/test/internal/vimcheck"
)

func loadVimContract(t *testing.T, e *testenv.Env) *vimcheck.Contract {
	t.Helper()
	c, err := vimcheck.Load(filepath.Join(e.Sim.Profile.VCenterDir, e.Sim.Profile.VCenter.TypeInfo))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// mvpWorkload는 MVP 실행 범위의 SOAP 메서드를 정상·오류 경로로 호출한다.
func mvpWorkload(t *testing.T, e *testenv.Env, c *govmomi.Client, name string) {
	t.Helper()
	ctx := context.Background()
	pc := property.DefaultCollector(c.Client)
	// 조회: 관리 객체 종류별 전체 속성(VC-SOAP-004 ~ 006)
	m := view.NewManager(c.Client)
	for _, kind := range []string{"Datacenter", "Folder", "ClusterComputeResource", "HostSystem", "ResourcePool", "Datastore",
		"Network", "DistributedVirtualPortgroup", "VmwareDistributedVirtualSwitch", "VirtualMachine"} {
		v, err := m.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{kind}, true)
		if err != nil {
			t.Fatal(err)
		}
		refs, err := v.Find(ctx, []string{kind}, nil)
		if err != nil {
			t.Fatalf("%s list: %v", kind, err)
		}
		if len(refs) > 2 && kind != "DistributedVirtualPortgroup" { // DVPG는 전부(NSX DVPG 포함, Phase 3)
			refs = refs[:2]
		}
		if len(refs) > 0 {
			var content []types.ObjectContent
			if err := pc.Retrieve(ctx, refs, nil, &content); err != nil {
				t.Fatalf("%s retrieve: %v", kind, err)
			}
		}
		_ = v.Destroy(ctx)
	}
	// ServiceContent·세션·시각(VC-SOAP-001·003)
	_, _ = methods.GetCurrentTime(ctx, c.Client)
	_, _ = c.SessionManager.SessionIsActive(ctx)

	// VM 생명주기와 Task(VC-SOAP-008 ~ 012)
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	dss, _ := f.DatastoreList(ctx, "*")
	dsName, _ := dss[0].ObjectName(ctx)
	spec := types.VirtualMachineConfigSpec{Name: name, GuestId: "otherGuest64", NumCPUs: 1, MemoryMB: 512,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"}}
	tk, err := folders.VmFolder.CreateVM(ctx, spec, pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	ti, err := tk.WaitForResult(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	vm := object.NewVirtualMachine(c.Client, ti.Result.(types.ManagedObjectReference))
	var content []types.ObjectContent
	_ = pc.Retrieve(ctx, []types.ManagedObjectReference{vm.Reference(), tk.Reference()}, nil, &content)
	// 실패 Task(Adapter, DuplicateName·InsufficientResources·InvalidPowerState)
	for _, s := range []types.VirtualMachineConfigSpec{spec, {Name: name + "-big", GuestId: "otherGuest64", NumCPUs: 4096, MemoryMB: 512,
		Files: spec.Files}} {
		if tk, err := folders.VmFolder.CreateVM(ctx, s, pool, nil); err == nil {
			_, _ = tk.WaitForResult(ctx, nil)
			_ = pc.Retrieve(ctx, []types.ManagedObjectReference{tk.Reference()}, []string{"info"}, &content)
		}
	}
	if tk, err := vm.Reconfigure(ctx, types.VirtualMachineConfigSpec{NumCPUs: 2, MemoryMB: 1024}); err == nil {
		_ = tk.Wait(ctx)
	}
	// 변경 감시(VC-SOAP-007): 전원 켜기 전후 runtime 변경
	wctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	go func() {
		time.Sleep(200 * time.Millisecond)
		if tk, err := vm.PowerOn(ctx); err == nil {
			_ = tk.Wait(ctx)
		}
	}()
	_ = property.Wait(wctx, property.DefaultCollector(c.Client), vm.Reference(), []string{"runtime.powerState", "summary.runtime", "config.hardware"},
		func(ch []types.PropertyChange) bool {
			for _, p := range ch {
				if p.Name == "runtime.powerState" && p.Val == types.VirtualMachinePowerStatePoweredOn {
					return true
				}
			}
			return false
		})
	cancel()
	if tk, err := vm.Destroy(ctx); err == nil { // 켜진 VM 삭제 → InvalidPowerState
		_ = tk.Wait(ctx)
	}
	if tk, err := vm.PowerOff(ctx); err == nil {
		_ = tk.Wait(ctx)
	}
	if tk, err := vm.Destroy(ctx); err == nil {
		_ = tk.Wait(ctx)
	}
	// 성능 메트릭(WP 3.7): 호스트·VM QueryPerf
	if counters, err := performance.NewManager(c.Client).CounterInfoByName(ctx); err == nil {
		hosts, _ := f.HostSystemList(ctx, "*")
		ids := []types.PerfMetricId{{CounterId: counters["cpu.usagemhz.average"].Key}, {CounterId: counters["mem.usage.average"].Key}}
		_, _ = methods.QueryPerf(ctx, c.Client, &types.QueryPerf{This: *c.ServiceContent.PerfManager, QuerySpec: []types.PerfQuerySpec{
			{Entity: hosts[0].Reference(), MetricId: ids, IntervalId: 20, MaxSample: 3}, {Entity: vm.Reference(), MetricId: ids, IntervalId: 20, MaxSample: 3}}})
	}
	// 오류: 없는 객체(ManagedObjectNotFound), 이벤트 조회
	_, _ = vm.PowerOn(ctx)
	_, _ = event.NewManager(c.Client).QueryEvents(ctx, types.EventFilterSpec{MaxCount: 20})
	var tm []types.ObjectContent
	_ = pc.Retrieve(ctx, []types.ManagedObjectReference{*c.ServiceContent.TaskManager}, []string{"recentTask"}, &tm)
}

// T-TR-COV-0304 T-TR-COV-0301 T-FR-VC-0606 T-FR-API-0101 T-FR-API-0102
// 추적: T-TR-COV-0403 T-NFR-ARC-0109
func TestSOAPSchemaContract(t *testing.T) {
	e := testenv.Start(t, nil)
	vc := loadVimContract(t, e)
	for _, session := range []string{"7.0.3.0", "6.7.3"} {
		t.Run(session, func(t *testing.T) {
			c, rec := e.RecordingLogin(session)
			mvpWorkload(t, e, c, "schema-"+strings.ReplaceAll(session, ".", ""))
			_ = c.Logout(context.Background())
			byRule := map[string][]string{}
			methodsSeen := map[string]int{}
			for _, x := range rec.Exchanges {
				methodsSeen[x.Method]++
				for _, f := range vc.CheckResponse(x.Method, x.SessionVersion, x.Response) {
					key := f.Rule + " " + normalizePath(f.Path)
					byRule[key] = append(byRule[key], x.Method+": "+f.Detail)
				}
			}
			var ms []string
			for m, n := range methodsSeen {
				ms = append(ms, fmt.Sprintf("%s×%d", m, n))
			}
			sort.Strings(ms)
			t.Logf("checked %d responses: %s", len(rec.Exchanges), strings.Join(ms, " "))
			if len(byRule) > 0 {
				var keys []string
				for k := range byRule {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				var b strings.Builder
				for _, k := range keys {
					fmt.Fprintf(&b, "  %s (%d) e.g. %s\n", k, len(byRule[k]), byRule[k][0])
				}
				t.Errorf("contract violations (%d kinds):\n%s", len(keys), b.String())
			}
		})
	}
}

// normalizePath는 배열 색인을 지워 같은 종류의 위반을 묶는다.
func normalizePath(p string) string {
	var b strings.Builder
	skip := false
	for _, r := range p {
		switch {
		case r == '[':
			skip = true
		case r == ']':
			skip = false
		case !skip:
			b.WriteRune(r)
		}
	}
	return b.String()
}
