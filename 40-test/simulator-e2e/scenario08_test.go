// Scenario 08 — vCenter + NSX 통합 Workflow (PRD-2 TR-E2E-0801 ~ TR-E2E-0804, Phase 3 종료 게이트).
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatalf("timeout: %s", what)
}

// nsxPortgroup은 segmentId가 path인 NSX DVPG를 찾는다.
func nsxPortgroup(t *testing.T, v *vcTarget, path string) *mo.DistributedVirtualPortgroup {
	t.Helper()
	ctx := context.Background()
	refs, err := v.finder.NetworkList(ctx, "*")
	if err != nil {
		t.Fatal(err)
	}
	var pgRefs []types.ManagedObjectReference
	for _, r := range refs {
		if r.Reference().Type == "DistributedVirtualPortgroup" {
			pgRefs = append(pgRefs, r.Reference())
		}
	}
	var pgs []mo.DistributedVirtualPortgroup
	if err := property.DefaultCollector(v.c.Client).Retrieve(ctx, pgRefs, []string{"name", "config", "vm"}, &pgs); err != nil {
		t.Fatal(err)
	}
	for i := range pgs {
		if pgs[i].Config.SegmentId == path {
			return &pgs[i]
		}
	}
	return nil
}

func nsxJSON(t *testing.T, e *testenv.Env, path string) (int, map[string]any) {
	t.Helper()
	n := nsx{t, e}
	code, o, _ := n.do(http.MethodGet, path, "")
	return code, o
}

func results(o map[string]any) []map[string]any {
	var out []map[string]any
	if rs, ok := o["results"].([]any); ok {
		for _, r := range rs {
			out = append(out, r.(map[string]any))
		}
	}
	return out
}

// TR-E2E-0801 절차 ①~⑦, 성공 기준 0802 ~ 0804.
// 추적: T-TR-E2E-0801 T-TR-E2E-0802 T-TR-E2E-0803 T-TR-E2E-0804 T-FR-NSX-0603 T-FR-NSX-0604 T-FR-NSX-0606 T-FR-NSX-0607 T-FR-NSX-0608 T-FR-NSX-0226 T-FR-NSX-0227 T-FR-NSX-0228 T-FR-NSX-0602 T-FR-NSX-0601 T-FR-NSX-0609 T-FR-NSX-0605 T-FR-DM-0401 T-FR-DM-0404 T-FR-BE-0206 T-FR-BE-0403 T-TR-COV-0309
func TestScenario08_CrossSystemWorkflow(t *testing.T) {
	e := testenv.Start(t, nil)
	n := nsx{t, e}
	ctx := context.Background()
	const seg = "/infra/segments/app-net"

	// ① NSX Segment 생성(VDS에 연결된 Overlay TZ)
	if code, _, raw := n.do(http.MethodPatch, "/policy/api/v1"+seg, `{"display_name":"app-net","transport_zone_path":"/infra/sites/default/enforcement-points/default/transport-zones/tz-overlay","subnets":[{"gateway_address":"10.30.0.1/24"}]}`); code != 200 {
		t.Fatalf("① segment: %d %s", code, raw)
	}
	_, so := nsxJSON(t, e, "/policy/api/v1"+seg)
	if s := n.realization(seg); s != "SUCCESS" {
		t.Fatalf("① realization %s (network sync runs within the write)", s)
	}

	// ② Network 관계: vCenter NSX DVPG(backingType=nsx, segmentId=Policy Path, logicalSwitchUuid=unique_id)
	v := target(t, e)
	pg := nsxPortgroup(t, v, seg)
	if pg == nil || pg.Config.BackingType != "nsx" || pg.Config.LogicalSwitchUuid != so["unique_id"] || pg.Name != "app-net" {
		t.Fatalf("② NSX DVPG: %+v", pg)
	}
	// 매핑 대상이 아닌 TZ(Fixture에 VDS 미연결) Segment는 vCenter 네트워크가 되지 않는다(FR-NSX-0603)
	nsxRefs, _ := v.finder.NetworkList(ctx, "*")
	before := len(nsxRefs)

	// ③ VM 생성 ④ vNIC을 NSX DVPG에 연결
	vmRef, err := v.create(t, "app-vm", 1, 512)
	if err != nil {
		t.Fatal(err)
	}
	vm := object.NewVirtualMachine(v.c.Client, vmRef)
	backing, err := object.NewDistributedVirtualPortgroup(v.c.Client, pg.Self).EthernetCardBackingInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nic, err := object.EthernetCardTypes().CreateEthernetCard("vmxnet3", backing)
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.AddDevice(ctx, nic); err != nil {
		t.Fatalf("④ add vNIC: %v", err)
	}
	var mvm mo.VirtualMachine
	_ = vm.Properties(ctx, vmRef, []string{"config.instanceUuid", "network", "runtime.host"}, &mvm)
	inst := mvm.Config.InstanceUuid

	// ⑤ 양 시스템 조회: 관계 ID 일치(TR-E2E-0802). 파생은 비동기(동기화 전에는 아직 보이지 않음, FR-DM-0404)
	var fvm, vif, port map[string]any
	eventually(t, "NSX fabric VM·VIF·segment port", func() bool {
		_, l := nsxJSON(t, e, "/api/v1/fabric/virtual-machines?external_id="+inst)
		_, f := nsxJSON(t, e, "/api/v1/fabric/vifs?owner_vm_id="+inst)
		_, p := nsxJSON(t, e, "/policy/api/v1"+seg+"/ports")
		if len(results(l)) != 1 || len(results(p)) != 1 {
			return false
		}
		for _, x := range results(f) {
			if x["lport_attachment_id"] != nil {
				vif = x
			}
		}
		fvm, port = results(l)[0], results(p)[0]
		return vif != nil
	})
	att, _ := port["attachment"].(map[string]any)
	if vif["lport_attachment_id"] != att["id"] || fvm["external_id"] != inst || vif["owner_vm_id"] != inst || fvm["power_state"] != "VM_STOPPED" {
		t.Fatalf("⑤ relation IDs: vm=%v vif=%v port=%v", fvm, vif, port)
	}
	if !containsRef(mvm.Network, pg.Self) {
		t.Fatalf("⑤ vCenter VM.network must include NSX DVPG: %v", mvm.Network)
	}
	// 호스트 관계: Fabric VM host_id = Discovered Node external_id, cm_local_id = vCenter Host MoRef (FR-NSX-0602)
	_, dn := nsxJSON(t, e, "/api/v1/fabric/discovered-nodes?cm_local_id="+mvm.Runtime.Host.Value)
	if d := results(dn); len(d) != 1 || d[0]["external_id"] != fvm["host_id"] {
		t.Fatalf("⑤ discovered node: %v vm.host_id=%v", dn, fvm["host_id"])
	}
	// Compute Manager 상태(FR-NSX-0601)
	_, cms := nsxJSON(t, e, "/api/v1/fabric/compute-managers")
	cmID := results(cms)[0]["id"].(string)
	if code, st := nsxJSON(t, e, "/api/v1/fabric/compute-managers/"+cmID+"/status"); code != 200 || st["connection_status"] != "UP" {
		t.Fatalf("compute manager status: %d %v", code, st)
	}
	// 실현 VM 조회(FR-NSX-0228), NSX VM Tag(FR-NSX-0226·0609: NSX 소유, vCenter와 무관)
	if code, _, raw := n.do(http.MethodPost, "/api/v1/fabric/virtual-machines?action=add_tags", `{"external_id":"`+inst+`","tags":[{"scope":"app","tag":"web"}]}`); code != 204 {
		t.Fatalf("add_tags: %d %s", code, raw)
	}
	eventually(t, "realized VM with NSX tag", func() bool {
		_, rv := nsxJSON(t, e, "/policy/api/v1/infra/realized-state/enforcement-points/default/virtual-machines")
		for _, r := range results(rv) {
			if r["id"] == inst {
				tags, _ := r["tags"].([]any)
				return len(tags) == 1
			}
		}
		return false
	})
	// VM 관련 정책 참조(FR-NSX-0605): VM Tag 조건 Group의 유효 멤버에 VM이 들어간다
	if code, _, raw := n.do(http.MethodPatch, "/policy/api/v1/infra/domains/default/groups/web-vms", `{"expression":[{"resource_type":"Condition","member_type":"VirtualMachine","key":"Tag","operator":"EQUALS","value":"app|web"}]}`); code != 200 {
		t.Fatalf("group: %d %s", code, raw)
	}
	_, gm := nsxJSON(t, e, "/policy/api/v1/infra/domains/default/groups/web-vms/members/virtual-machines")
	if m := results(gm); len(m) != 1 || m[0]["id"] != inst {
		t.Fatalf("group members by VM tag: %v", gm)
	}
	n.do(http.MethodPatch, "/policy/api/v1/infra/domains/default/groups/name-vms", `{"expression":[{"resource_type":"Condition","member_type":"VirtualMachine","key":"Name","operator":"STARTSWITH","value":"app-"},{"resource_type":"ConjunctionOperator","conjunction_operator":"AND"},{"resource_type":"Condition","member_type":"VirtualMachine","key":"Tag","operator":"NOTEQUALS","value":"app|db"}]}`)
	_, gm = nsxJSON(t, e, "/policy/api/v1/infra/domains/default/groups/name-vms/members/virtual-machines")
	if m := results(gm); len(m) != 1 || m[0]["display_name"] != "app-vm" {
		t.Fatalf("group members by name AND not tag: %v", gm)
	}
	// 생성한 Segment Port는 시스템 소유: 사용자 변경 거부(가정)
	if code, _, _ := n.do(http.MethodDelete, "/policy/api/v1"+seg+"/ports/"+port["id"].(string), ""); code != http.StatusForbidden {
		t.Fatalf("system-owned port delete: %d", code)
	}
	// TR-E2E-0803: 일관성 — 조정이 고칠 것도 남은 불일치도 없다
	res, _ := e.AdminDo(http.MethodPost, "/v1/reconcile", nil)
	var rep map[string]any
	_ = json.NewDecoder(res.Body).Decode(&rep)
	res.Body.Close()
	if rep["created_portgroups"] != nil || rep["deleted_portgroups"] != nil || rep["errors"] != nil || rep["drift"] != nil {
		t.Fatalf("TR-E2E-0803 drift after workflow: %v", rep)
	}

	// TR-E2E-0804: VM이 연결된 Segment 삭제는 거부
	if code, o, _ := n.do(http.MethodDelete, "/policy/api/v1"+seg, ""); code != http.StatusBadRequest || !strings.Contains(fmt.Sprint(o["error_message"]), "child") {
		t.Fatalf("delete segment with port: %d %v", code, o)
	}
	// ⑥ VM 삭제 → 파생 객체 제거
	tk, _ := vm.Destroy(ctx)
	if err := tk.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	eventually(t, "derived objects removed", func() bool {
		_, p := nsxJSON(t, e, "/policy/api/v1"+seg+"/ports")
		_, l := nsxJSON(t, e, "/api/v1/fabric/virtual-machines?external_id="+inst)
		return len(results(p)) == 0 && len(results(l)) == 0
	})
	// ⑦ Segment 삭제 → vCenter DVPG 제거
	if code, _, raw := n.do(http.MethodDelete, "/policy/api/v1"+seg, ""); code != 200 {
		t.Fatalf("⑦ delete segment: %d %s", code, raw)
	}
	if pg := nsxPortgroup(t, v, seg); pg != nil {
		t.Fatalf("⑦ DVPG must be removed: %v", pg.Self)
	}
	after, _ := v.finder.NetworkList(ctx, "*")
	if len(after) != before-1 {
		t.Fatalf("network count %d -> %d", before, len(after))
	}
}

// Realization 확장: 의존 리소스 미실현 → 대기, 진행 중 재수정 → 최신 intent 기준, 동기화 실패 → ERROR와 오류 정보.
// 추적: T-FR-NSX-0504 T-FR-NSX-0505 T-FR-NSX-0507 T-FR-NSX-0509 T-FR-DM-0403 T-FR-MET-0202
func TestRealizationDependenciesAndFailure(t *testing.T) {
	e := testenv.Start(t, nil)
	n := nsx{t, e}
	delay := func(ms int) {
		res, _ := e.AdminDo(http.MethodPut, "/v1/nsx/realization-delay", strings.NewReader(fmt.Sprintf(`{"ms":%d}`, ms)))
		res.Body.Close()
	}
	// 의존(FR-NSX-0507): Tier-1이 실현되기 전에는 그것에 연결한 Segment도 IN_PROGRESS
	delay(600)
	n.do(http.MethodPatch, "/policy/api/v1/infra/tier-1s/t1-dep", `{}`)
	delay(0)
	n.do(http.MethodPatch, "/policy/api/v1/infra/segments/dep", `{"connectivity_path":"/infra/tier-1s/t1-dep"}`)
	if s := n.realization("/infra/segments/dep"); s != "IN_PROGRESS" {
		t.Fatalf("dependent segment: %s", s)
	}
	eventually(t, "dependency realized", func() bool { return n.realization("/infra/segments/dep") == "SUCCESS" })

	// 진행 중 재수정(FR-NSX-0505): 최신 intent의 지연이 끝나야 완료, intent_version 증가
	delay(500)
	n.do(http.MethodPatch, "/policy/api/v1/infra/segments/remod", `{}`)
	time.Sleep(300 * time.Millisecond)
	n.do(http.MethodPatch, "/policy/api/v1/infra/segments/remod", `{"description":"v2"}`)
	time.Sleep(300 * time.Millisecond) // 첫 intent만이라면 이미 완료됐을 시각
	_, st := nsxJSON(t, e, "/policy/api/v1/infra/realized-state/status?intent_path=/infra/segments/remod")
	if st["consolidated_status"].(map[string]any)["consolidated_status"] != "IN_PROGRESS" || st["intent_version"] != "1" {
		t.Fatalf("re-modification: %v", st)
	}
	eventually(t, "re-modified segment realized", func() bool { return n.realization("/infra/segments/remod") == "SUCCESS" })
	delay(0)

	// 동기화 실패(FR-NSX-0504): NSX 준비 VDS가 사라지면 매핑 대상 Segment는 ERROR와 오류 정보를 보고
	v := target(t, e)
	ctx := context.Background()
	vw, err := view.NewManager(v.c.Client).CreateContainerView(ctx, v.c.ServiceContent.RootFolder, []string{"DistributedVirtualSwitch"}, true)
	if err != nil {
		t.Fatal(err)
	}
	dvsList, err := vw.Find(ctx, []string{"DistributedVirtualSwitch"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range dvsList {
		{
			tk, err := object.NewDistributedVirtualSwitch(v.c.Client, r.Reference()).Destroy(ctx)
			if err == nil {
				err = tk.Wait(ctx)
			}
			if err != nil {
				t.Fatalf("destroy DVS: %v", err)
			}
		}
	}
	n.do(http.MethodPatch, "/policy/api/v1/infra/segments/no-vds", `{}`)
	if s := n.realization("/infra/segments/no-vds"); s != "ERROR" {
		t.Fatalf("segment without prepared VDS: %s", s)
	}
	_, re := nsxJSON(t, e, "/policy/api/v1/infra/realized-state/realized-entities?intent_path=/infra/segments/no-vds")
	r := results(re)
	if len(r) != 1 || r[0]["state"] != "ERROR" || !strings.Contains(fmt.Sprint(r[0]["publish_status_error"]), "VDS") {
		t.Fatalf("realized entity error info: %v", re)
	}
	// NSX 알람(FR-MET-0201 'NSX Policy Realization Failed'): OPEN, 원인 intent 경로·엔터티 표시
	_, al := nsxJSON(t, e, "/api/v1/alarms?status=OPEN")
	found := false
	for _, a := range results(al) {
		src, _ := a["alarm_source"].([]any)
		if len(src) == 1 && src[0] == "/infra/segments/no-vds" && a["severity"] != nil && a["entity_id"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("realization failure alarm: %v", al)
	}
	// 복구 절차(FR-DM-0403): 환경 초기화 후 다시 조정하면 정상
	res, _ := e.AdminDo(http.MethodPost, "/v1/reset", nil)
	res.Body.Close()
	if _, al := nsxJSON(t, e, "/api/v1/alarms"); len(results(al)) != 0 {
		t.Fatalf("alarms after reset: %v", al)
	}
	if s := n.realization("/infra/segments/seg-01"); s != "SUCCESS" {
		t.Fatalf("after reset: %s", s)
	}
}
