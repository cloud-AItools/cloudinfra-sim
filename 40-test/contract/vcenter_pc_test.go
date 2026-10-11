// PropertyCollector·View 계약 테스트 (FR-VC-0507 ~ FR-VC-0509, FR-VC-0202; API VC-SOAP-004 ~ 007).
// 기대값 근거: SRC-VIM-TYPEINFO(메서드·타입 존재), vim25 계약(Ex 메서드의 token 기반 이어 받기, WaitOptions.maxWaitSeconds).
package contract

import (
	"context"
	"testing"
	"time"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-VC-0202 / T-FR-VC-0507: ContainerView + RetrievePropertiesEx(MaxObjects) + ContinueRetrievePropertiesEx로 전체를 받는다.
func TestContainerViewAndPagedRetrieve(t *testing.T) {
	e := testenv.Start(t, nil)
	c := e.Login("")
	ctx := context.Background()
	v, err := view.NewManager(c.Client).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		t.Fatalf("CreateContainerView: %v", err)
	}
	defer func() {
		if err := v.Destroy(ctx); err != nil {
			t.Errorf("DestroyView: %v", err)
		}
	}()
	spec := types.PropertyFilterSpec{
		ObjectSet: []types.ObjectSpec{{Obj: v.Reference(), Skip: types.NewBool(true), SelectSet: []types.BaseSelectionSpec{
			&types.TraversalSpec{Type: "ContainerView", Path: "view"},
		}}},
		PropSet: []types.PropertySpec{{Type: "VirtualMachine", PathSet: []string{"name"}}},
	}
	pc := property.DefaultCollector(c.Client)
	res, err := methods.RetrievePropertiesEx(ctx, c.Client, &types.RetrievePropertiesEx{
		This: pc.Reference(), SpecSet: []types.PropertyFilterSpec{spec}, Options: types.RetrieveOptions{MaxObjects: 7},
	})
	if err != nil {
		t.Fatalf("RetrievePropertiesEx: %v", err)
	}
	total, pages := len(res.Returnval.Objects), 1
	token := res.Returnval.Token
	for token != "" {
		next, err := methods.ContinueRetrievePropertiesEx(ctx, c.Client, &types.ContinueRetrievePropertiesEx{This: pc.Reference(), Token: token})
		if err != nil {
			t.Fatalf("ContinueRetrievePropertiesEx: %v", err)
		}
		total += len(next.Returnval.Objects)
		token = next.Returnval.Token
		pages++
	}
	t.Logf("retrieved %d VMs in %d pages (MaxObjects=7)", total, pages)
	if total != 20 || pages < 3 {
		t.Fatalf("paged retrieve: total=%d pages=%d", total, pages)
	}
}

// T-FR-VC-0508 / T-FR-VC-0509: CreateFilter → WaitForUpdatesEx 초기 결과 → 변경 없으면 maxWaitSeconds 후 빈 결과 →
// 변경 시 변경 속성 반환 → DestroyPropertyFilter. 버전 토큰은 vcsim이 '-' 고정이라 값 비교는 하지 않는다(R-03).
func TestFilterAndWaitForUpdatesEx(t *testing.T) {
	e := testenv.Start(t, nil)
	c := e.Login("")
	ctx := context.Background()
	vmRef := firstRef(t, e, "VirtualMachine")
	pc, err := property.DefaultCollector(c.Client).Create(ctx) // 세션 전용 PropertyCollector
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Destroy(ctx)
	filter := new(property.WaitFilter).Add(vmRef, "VirtualMachine", []string{"runtime.powerState"})
	f, err := pc.CreateFilter(ctx, filter.CreateFilter)
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ver := ""
	wait := func(sec int32) *types.UpdateSet {
		r, err := methods.WaitForUpdatesEx(ctx, c.Client, &types.WaitForUpdatesEx{This: pc.Reference(), Version: ver, Options: &types.WaitOptions{MaxWaitSeconds: &sec}})
		if err != nil {
			t.Fatalf("WaitForUpdatesEx: %v", err)
		}
		if r.Returnval != nil {
			ver = r.Returnval.Version
		}
		return r.Returnval
	}
	if us := wait(5); us == nil || len(us.FilterSet) != 1 {
		t.Fatal("initial update missing")
	}
	start := time.Now()
	if us := wait(1); us != nil {
		t.Fatalf("no-change wait must return empty, got %+v", us)
	}
	if d := time.Since(start); d < 900*time.Millisecond || d > 3*time.Second {
		t.Fatalf("maxWaitSeconds=1 honoured? took %v", d)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		c2 := e.Login("")
		tk, err := methods.PowerOffVM_Task(ctx, c2.Client, &types.PowerOffVM_Task{This: vmRef})
		if err == nil {
			_ = tk
		}
	}()
	us := wait(10)
	if us == nil || len(us.FilterSet[0].ObjectSet) == 0 {
		t.Fatal("change not delivered")
	}
	ch := us.FilterSet[0].ObjectSet[0].ChangeSet
	if len(ch) == 0 || ch[0].Name != "runtime.powerState" {
		t.Fatalf("unexpected change set %+v", ch)
	}
	if _, err := methods.DestroyPropertyFilter(ctx, c.Client, &types.DestroyPropertyFilter{This: f.Reference()}); err != nil {
		t.Fatalf("DestroyPropertyFilter: %v", err)
	}
}
