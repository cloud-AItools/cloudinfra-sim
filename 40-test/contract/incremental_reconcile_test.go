// 증분 관계 조정(R-45). 검증 모드(SIM_RELATIONS_VERIFY)로 매 조정마다 전체 계산과 대조하면서, 관계 입력을 바꾸는
// 작업(VM 생성·복제·삭제·이름·전원·NIC 연결, 이동, NSX VM Tag, Segment 생성·이름 변경·삭제, 호스트 상태 Fault, Snapshot
// 복원)을 섞어 실행한다. 차이가 한 번이라도 나면 testenv 정리 단계가 실패시킨다.
package contract

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-DM-0402 T-FR-DM-0403 T-FR-NSX-0602 T-FR-NSX-0604 T-FR-NSX-0608
func TestIncrementalReconcile(t *testing.T) {
	t.Setenv("SIM_RELATIONS_VERIFY", "1")
	e := testenv.Start(t, nil)
	if !e.Sim.Rel.Verify {
		t.Fatal("verify mode not enabled")
	}
	l := snapVC(t, e)
	last := func() (string, int, int) {
		r, _ := e.Sim.Rel.Last()
		if len(r.Errors) > 0 {
			t.Fatalf("reconcile errors: %v", r.Errors)
		}
		return r.Mode, r.Refreshed, r.VMs
	}
	// Segment 쓰기(VM 변경 없음): 증분, 다시 조회한 VM 없음
	mustNSX(t, e, "PATCH", p4+"/infra/segments/inc-a", `{"display_name":"inc-a","transport_zone_path":"`+segTZ+`"}`, 200)
	waitFor(t, "inc-a portgroup", func() bool { return segmentPortgroup(t, l.c, "/infra/segments/inc-a") })
	mustNSX(t, e, "PATCH", p4+"/infra/segments/inc-a", `{"description":"x"}`, 200)
	if mode, refreshed, vms := last(); mode != "incremental" || refreshed != 0 || vms != 20 {
		t.Fatalf("segment write: mode=%s refreshed=%d vms=%d", mode, refreshed, vms)
	}
	// VM NIC을 Segment에 연결 → 그 VM만 다시 조회, Segment Port 생성
	attachVMToSegment(t, e, "/infra/segments/inc-a")
	waitFor(t, "segment port", func() bool {
		_, o := nsxDo(t, e, "GET", p4+"/infra/segments/inc-a/ports", "")
		return len(results(o)) == 1
	})
	// VM 생명주기 혼합
	ref := l.create("inc-vm")
	vm := object.NewVirtualMachine(l.c.Client, ref)
	tk, err := vm.PowerOn(l.ctx)
	l.run("power on", tk, err, "")
	tk, err = vm.Rename(l.ctx, "inc-vm-renamed")
	l.run("rename", tk, err, "")
	dc, _ := l.f.DefaultDatacenter(l.ctx)
	folders, _ := dc.Folders(l.ctx)
	tk, err = vm.Clone(l.ctx, folders.VmFolder, "inc-clone", types.VirtualMachineCloneSpec{})
	l.run("clone", tk, err, "")
	hosts, _ := l.f.HostSystemList(l.ctx, "*")
	target := hosts[len(hosts)-1].Reference()
	tk, err = vm.Relocate(l.ctx, types.VirtualMachineRelocateSpec{Host: &target}, types.VirtualMachineMovePriorityDefaultPriority)
	l.run("relocate", tk, err, "")
	tk, err = vm.PowerOff(l.ctx)
	l.run("power off", tk, err, "")
	tk, err = vm.Destroy(l.ctx)
	l.run("destroy", tk, err, "")
	// NSX VM Tag(Fabric VM tags는 조립 시 반영)
	_, fv := nsxDo(t, e, "GET", "/api/v1/fabric/virtual-machines", "")
	ext := results(fv)[0]["external_id"]
	mustNSX(t, e, "POST", "/api/v1/fabric/virtual-machines?action=update_tags", fmt.Sprintf(`{"external_id":%q,"tags":[{"scope":"app","tag":"inc"}]}`, ext), 204)
	// Segment 이름 변경·새 Segment·삭제
	mustNSX(t, e, "PATCH", p4+"/infra/segments/inc-a", `{"display_name":"inc-a2"}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/segments/inc-b", `{"display_name":"inc-b","transport_zone_path":"`+segTZ+`"}`, 200)
	mustNSX(t, e, "DELETE", p4+"/infra/segments/inc-b", "", 200)
	// 호스트 상태 Fault(유지보수) → Transport Node 상태
	addFault(t, e, `{"layer":"vcenter","match":{"resource":"`+hosts[0].Reference().Value+`"},"action":{"type":"host_maintenance"}}`)
	adminJSON(t, e, http.MethodDelete, "/v1/faults", "")
	// 비동기 조정이 끝날 때까지 기다린 뒤 관리자 조정(전체)과 결과 일치 확인
	time.Sleep(300 * time.Millisecond)
	e.Sim.GW.Lock.Lock()
	inc := e.Sim.Rel.Reconcile("vcenter-change")
	full := e.Sim.Rel.Reconcile("admin")
	e.Sim.GW.Lock.Unlock()
	if inc.Mode != "incremental" || full.Mode != "full" || inc.VMs != full.VMs || inc.VIFs != full.VIFs || inc.Ports != full.Ports {
		t.Fatalf("incremental %+v vs full %+v", inc, full)
	}
	// Snapshot 복원은 모델을 바꾸므로 전체 계산
	adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"inc"}`)
	if code, r := adminJSON(t, e, http.MethodPost, "/v1/snapshots/inc/restore", ""); code != 200 || r["consistent"] != true {
		t.Fatalf("restore: %d %v", code, r)
	}
	if mode, _, _ := last(); mode != "full" {
		t.Fatalf("after restore mode %s", mode)
	}
	mustNSX(t, e, "PATCH", p4+"/infra/segments/inc-a", `{"description":"after restore"}`, 200)
	if mode, refreshed, _ := last(); mode != "incremental" || refreshed != 0 {
		t.Fatalf("after restore write: %s %d", mode, refreshed)
	}
}
