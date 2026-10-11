// Event·PropertyCollector 개선(WP 4.6, FR-VC-0509, FR-MET-0202, FR-BE-0611). 근거: SRC-VIM-TYPEINFO(WaitForUpdatesEx 선언
// Fault InvalidCollectorVersion, UpdateSet.version·truncated, WaitOptions.maxObjectUpdates), vSphere API 설명.
package contract

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-VC-0509: 데이터 버전 발급·검증, maxObjectUpdates 부분 응답(truncated)과 이어 받기, 변경 감시.
func TestPropertyCollectorVersions(t *testing.T) {
	e := testenv.Start(t, nil)
	contract := loadVimContract(t, e)
	c, rec := e.RecordingLogin("")
	ctx := context.Background()
	pc, err := property.DefaultCollector(c.Client).Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Destroy(ctx)
	vv, err := view.NewManager(c.Client).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Destroy(ctx)
	ts := types.TraversalSpec{Type: "ContainerView", Path: "view"}
	spec := types.PropertyFilterSpec{
		ObjectSet: []types.ObjectSpec{{Obj: vv.Reference(), Skip: types.NewBool(true), SelectSet: []types.BaseSelectionSpec{&ts}}},
		PropSet:   []types.PropertySpec{{Type: "VirtualMachine", PathSet: []string{"name", "runtime.powerState"}}},
	}
	if _, err := pc.CreateFilter(ctx, types.CreateFilter{Spec: spec, PartialUpdates: false}); err != nil {
		t.Fatal(err)
	}
	wait := func(version string, maxObj int32) (*types.UpdateSet, error) {
		zero := int32(0)
		res, err := methods.WaitForUpdatesEx(ctx, c.Client, &types.WaitForUpdatesEx{This: pc.Reference(), Version: version,
			Options: &types.WaitOptions{MaxWaitSeconds: &zero, MaxObjectUpdates: maxObj}})
		if err != nil {
			return nil, err
		}
		return res.Returnval, nil
	}
	// 초기(빈 버전): 전체 상태를 maxObjectUpdates 단위로 나눠 받는다(Fixture VM 20개, 8개씩)
	set, err := wait("", 8)
	if err != nil || set == nil {
		t.Fatalf("initial: %v", err)
	}
	seen := 0
	versions := []string{}
	for {
		n := 0
		for _, fs := range set.FilterSet {
			n += len(fs.ObjectSet)
		}
		seen += n
		versions = append(versions, set.Version)
		if n > 8 {
			t.Fatalf("maxObjectUpdates exceeded: %d", n)
		}
		if set.Truncated == nil || !*set.Truncated {
			break
		}
		if set, err = wait(set.Version, 8); err != nil || set == nil {
			t.Fatalf("continue truncated: %v %v", set, err)
		}
	}
	if seen != 20 || len(versions) != 3 {
		t.Fatalf("paged initial updates: %d objects in %d sets (%v)", seen, len(versions), versions)
	}
	if strings.Join(versions, ",") != "1,2,3" {
		t.Fatalf("versions must increase per collector: %v", versions)
	}
	// 변경 없음(maxWaitSeconds=0): null 결과
	if set, err := wait(versions[2], 0); err != nil || set != nil {
		t.Fatalf("no change: %v %v", set, err)
	}
	// 발급하지 않은 버전 → InvalidCollectorVersion(선언 Fault)
	_, err = wait("999", 0)
	if _, ok := soapFaultOf(err).(types.InvalidCollectorVersion); !ok {
		t.Fatalf("unknown version: want InvalidCollectorVersion, got %T %v", soapFaultOf(err), err)
	}
	// 변경 감시: VM 전원 끄기 → 다음 버전에 runtime.powerState 변경
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	tk, _ := vms[0].PowerOff(ctx)
	_ = tk.Wait(ctx)
	ten := int32(10)
	res, err := methods.WaitForUpdatesEx(ctx, c.Client, &types.WaitForUpdatesEx{This: pc.Reference(), Version: versions[2], Options: &types.WaitOptions{MaxWaitSeconds: &ten}})
	if err != nil || res.Returnval == nil {
		t.Fatalf("change: %v", err)
	}
	if res.Returnval.Version != "4" {
		t.Fatalf("next version: %q", res.Returnval.Version)
	}
	found := false
	for _, fs := range res.Returnval.FilterSet {
		for _, o := range fs.ObjectSet {
			for _, ch := range o.ChangeSet {
				if o.Obj == vms[0].Reference() && ch.Name == "runtime.powerState" && ch.Val == types.VirtualMachinePowerStatePoweredOff {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("power state change not reported: %+v", res.Returnval)
	}
	// 다른 세션의 버전은 이 세션에서 유효하지 않다
	other := e.Login("")
	pc2, _ := property.DefaultCollector(other.Client).Create(ctx)
	defer pc2.Destroy(ctx)
	zero := int32(0)
	if _, err := methods.WaitForUpdatesEx(ctx, other.Client, &types.WaitForUpdatesEx{This: pc2.Reference(), Version: "1", Options: &types.WaitOptions{MaxWaitSeconds: &zero}}); err == nil {
		t.Fatal("version from another collector must be rejected")
	}
	for _, x := range rec.Exchanges {
		for _, fd := range contract.CheckResponse(x.Method, x.SessionVersion, x.Response) {
			t.Errorf("%s %s", x.Method, fd)
		}
	}
}

// T-FR-BE-0611 T-FR-MET-0202: 이벤트 시간 필터는 가상 시각으로 해석한다(가속·진행 후에도 경계가 맞다).
func TestEventTimeFilterVirtualClock(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	adminJSON(t, e, http.MethodPut, "/v1/clock", `{"mode":"test","frozen":true}`)
	adminJSON(t, e, http.MethodPut, "/v1/clock", `{"advance_ms":86400000}`) // 가상 하루 뒤
	_, st := adminJSON(t, e, http.MethodGet, "/v1/clock", "")
	vnow, err := time.Parse(time.RFC3339Nano, st["virtual_time"].(string))
	if err != nil {
		t.Fatal(err)
	}
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	tk, _ := vms[1].PowerOff(ctx)
	_ = tk.Wait(ctx)
	em := event.NewManager(c.Client)
	q := func(begin, end time.Time) []types.BaseEvent {
		t.Helper()
		evs, err := em.QueryEvents(ctx, types.EventFilterSpec{
			Entity:      &types.EventFilterSpecByEntity{Entity: vms[1].Reference(), Recursion: types.EventFilterSpecRecursionOptionSelf},
			Time:        &types.EventFilterSpecByTime{BeginTime: &begin, EndTime: &end},
			EventTypeId: []string{"VmPoweredOffEvent"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return evs
	}
	// 가상 시각 범위에 들어간다(정지된 가상 시각 vnow)
	got := q(vnow.Add(-time.Minute), vnow.Add(time.Minute))
	if len(got) != 1 {
		t.Fatalf("event in virtual window: %d", len(got))
	}
	if ct := got[0].GetEvent().CreatedTime; ct.Before(vnow.Add(-time.Second)) || ct.After(vnow.Add(time.Second)) {
		t.Fatalf("event createdTime %v not virtual %v", ct, vnow)
	}
	// 실제 시각(가상보다 하루 이전) 범위에는 없다
	if got := q(vnow.Add(-25*time.Hour), vnow.Add(-23*time.Hour)); len(got) != 0 {
		t.Fatalf("event must not match the real-time window: %d", len(got))
	}
	// 이벤트 컬렉터도 같은 규칙
	coll, err := em.CreateCollectorForEvents(ctx, types.EventFilterSpec{
		Entity: &types.EventFilterSpecByEntity{Entity: vms[1].Reference(), Recursion: types.EventFilterSpecRecursionOptionSelf},
		Time:   &types.EventFilterSpecByTime{BeginTime: types.NewTime(vnow.Add(-time.Minute))}, EventTypeId: []string{"VmPoweredOffEvent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	evs, err := coll.ReadNextEvents(ctx, 10)
	if err != nil || len(evs) != 1 {
		t.Fatalf("collector: %d %v", len(evs), err)
	}

}
