// 결정론적 VM 호스트 배치(FR-DM-0207): 같은 Fixture로 새로 만들면 VM 배치가 매번 같고, 클러스터 호스트에 라운드 로빈으로
// 나뉘며, 생성 이벤트의 호스트가 실제 배치와 같다(vcenter/placement.go).
package contract

import (
	"context"
	"fmt"
	"testing"

	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-DM-0207 T-TR-SCN-0201
func TestDeterministicVMPlacement(t *testing.T) {
	off := func(c *config.Config) { c.FixtureCache = "off" }
	a := testenv.Start(t, off)
	b := testenv.Start(t, off)
	va, _ := vmState(t, a)
	vb, _ := vmState(t, b)
	if d := firstDiff(va, vb); d != "" {
		t.Fatalf("두 번 새로 만든 인벤토리의 VM 배치가 다르다: %s", d)
	}
	// Reset으로 다시 만들어도 같다
	if err := a.Sim.Reset(); err != nil {
		t.Fatal(err)
	}
	if vr, _ := vmState(t, a); firstDiff(vr, va) != "" {
		t.Fatalf("Reset 뒤 배치가 다르다: %s", firstDiff(vr, va))
	}

	// 라운드 로빈: 풀 안 i번째 VM(이름 끝 번호) → 클러스터 호스트 목록의 i mod n번째
	l := snapVC(t, a)
	ctx := context.Background()
	v, err := view.NewManager(l.c.Client).CreateContainerView(ctx, l.c.ServiceContent.RootFolder, []string{"ClusterComputeResource"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Destroy(ctx)
	var cls []mo.ClusterComputeResource
	if err := v.Retrieve(ctx, []string{"ClusterComputeResource"}, []string{"name", "host"}, &cls); err != nil || len(cls) == 0 {
		t.Fatalf("clusters: %v %d", err, len(cls))
	}
	hosts := cls[0].Host
	vw, _ := view.NewManager(l.c.Client).CreateContainerView(ctx, l.c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	defer vw.Destroy(ctx)
	var vms []mo.VirtualMachine
	if err := vw.Retrieve(ctx, []string{"VirtualMachine"}, []string{"name", "runtime.host"}, &vms); err != nil {
		t.Fatal(err)
	}
	perHost := map[string]int{}
	for _, vm := range vms {
		var i int
		if _, err := fmt.Sscanf(vm.Name, cls[0].Name+"_RP0_VM%d", &i); err != nil {
			continue // Fixture VM이 아닌 것(시험이 만든 VM)
		}
		if want := hosts[i%len(hosts)]; *vm.Runtime.Host != want {
			t.Errorf("%s: host %s, want %s", vm.Name, vm.Runtime.Host.Value, want.Value)
		}
		perHost[vm.Runtime.Host.Value]++
	}
	if len(perHost) != len(hosts) {
		t.Errorf("VM이 모든 호스트에 나뉘지 않았다: %v", perHost)
	}

	// 생성 이벤트의 호스트 = 실제 배치(배치를 나중에 옮기지 않았음)
	evs, err := event.NewManager(l.c.Client).QueryEvents(ctx, types.EventFilterSpec{EventTypeId: []string{"VmCreatedEvent"}, MaxCount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	where := map[string]string{}
	for _, vm := range vms {
		where[vm.Self.Value] = vm.Runtime.Host.Value
	}
	checked := 0
	for _, e := range evs {
		ev := e.GetEvent()
		if ev.Vm == nil || ev.Host == nil {
			continue
		}
		if h, ok := where[ev.Vm.Vm.Value]; ok {
			checked++
			if ev.Host.Host.Value != h {
				t.Errorf("%s 생성 이벤트 host %s, 실제 %s", ev.Vm.Vm.Value, ev.Host.Host.Value, h)
			}
		}
	}
	if checked == 0 {
		t.Error("VmCreatedEvent를 찾지 못했다")
	}
}
