// Phase 5 Snapshot/Restore·재기동 복원·불일치 검출(WP 5.2). 근거: vcsim 저장·복원 형식(govc object.save·Model.Load,
// SRC-GOVMOMI), PRD FR-DM-0209·0304·0306, NFR-0205 ~ 0208. 재처리 정책·제외 항목은 BEHAVIOR_SPEC §14의 가정이다.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

const segTZ = "/infra/sites/default/enforcement-points/default/transport-zones/tz-overlay"

func snapVC(t *testing.T, e *testenv.Env) lc {
	c := e.Login("")
	f, _ := inventory(t, c)
	return lc{t: t, ctx: context.Background(), c: c, f: f}
}

func (l lc) create(name string) types.ManagedObjectReference {
	l.t.Helper()
	dc, _ := l.f.DefaultDatacenter(l.ctx)
	folders, _ := dc.Folders(l.ctx)
	pool, _ := l.f.DefaultResourcePool(l.ctx)
	dss, _ := l.f.DatastoreList(l.ctx, "*")
	ds, _ := dss[0].ObjectName(l.ctx)
	tk, err := folders.VmFolder.CreateVM(l.ctx, types.VirtualMachineConfigSpec{Name: name, GuestId: "otherGuest64", NumCPUs: 1, MemoryMB: 256,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[" + ds + "]"}}, pool, nil)
	return l.run("create "+name, tk, err, "").(types.ManagedObjectReference)
}

func (l lc) exists(name string) bool {
	_, err := l.f.VirtualMachine(l.ctx, name)
	if _, nf := err.(*find.NotFoundError); nf {
		return false
	}
	if err != nil {
		l.t.Fatal(err)
	}
	return true
}

func segmentPortgroup(t *testing.T, c *govmomi.Client, path string) bool {
	t.Helper()
	ctx := context.Background()
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	nets, _ := f.NetworkList(ctx, "*")
	var refs []types.ManagedObjectReference
	for _, n := range nets {
		if n.Reference().Type == "DistributedVirtualPortgroup" {
			refs = append(refs, n.Reference())
		}
	}
	var pgs []mo.DistributedVirtualPortgroup
	if err := property.DefaultCollector(c.Client).Retrieve(ctx, refs, []string{"config"}, &pgs); err != nil {
		t.Fatal(err)
	}
	for _, pg := range pgs {
		if pg.Config.SegmentId == path {
			return true
		}
	}
	return false
}

// T-TR-SCN-0104 T-TR-SCN-0105 T-NFR-0206 T-FR-DM-0209 T-FR-DM-0304 T-FR-DM-0306
func TestSnapshotRestoreRoundTrip(t *testing.T) {
	e := testenv.Start(t, nil)
	l := snapVC(t, e)
	// 기준 상태: VM 생성·전원·스냅샷, NSX Segment·Group(태그 조건)·VM Tag, 스토리지 정책 없음
	ref := l.create("snap-vm")
	vm := object.NewVirtualMachine(l.c.Client, ref)
	tk, err := vm.PowerOn(l.ctx)
	l.run("power on", tk, err, "")
	tk, err = vm.CreateSnapshot(l.ctx, "s-before", "", false, false)
	l.run("vm snapshot", tk, err, "")
	mustNSX(t, e, "PATCH", p4+"/infra/segments/snap-seg", `{"display_name":"snap-seg","transport_zone_path":"`+segTZ+`"}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/groups/snap-grp", `{"display_name":"snap-grp","expression":[{"resource_type":"Condition","member_type":"VirtualMachine","key":"Tag","operator":"EQUALS","value":"app|snap"}]}`, 200)
	waitRealized(t, e, "/infra/segments/snap-seg")
	before := mustNSX(t, e, "GET", p4+"/infra/segments/snap-seg", "", 200)
	waitFor(t, "snap-seg portgroup", func() bool { return segmentPortgroup(t, l.c, "/infra/segments/snap-seg") })

	code, man := adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"s1"}`)
	if code != 201 || man["vcenter_objects"].(float64) < 100 || man["nsx_objects"].(float64) < 10 {
		t.Fatalf("create snapshot: %d %v", code, man)
	}
	files, _ := man["sha256"].(map[string]any)
	for _, f := range []string{"state.db", "nsx.json", "vcenter.json"} {
		if files[f] == nil {
			t.Fatalf("manifest missing %s: %v", f, files)
		}
	}
	// 변경: VM 전원 끄기·새 VM, Segment 삭제·새 Segment, Group 수정
	tk, err = vm.PowerOff(l.ctx)
	l.run("power off", tk, err, "")
	l.create("after-vm")
	mustNSX(t, e, "DELETE", p4+"/infra/segments/snap-seg", "", 200)
	mustNSX(t, e, "PATCH", p4+"/infra/segments/after-seg", `{"display_name":"after-seg","transport_zone_path":"`+segTZ+`"}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/groups/snap-grp", `{"description":"changed"}`, 200)

	code, res := adminJSON(t, e, http.MethodPost, "/v1/snapshots/s1/restore", "")
	if code != 200 || res["consistent"] != true {
		t.Fatalf("restore: %d %v", code, res)
	}
	// 기존 세션은 무효(Reset과 같음) → 다시 로그인
	if _, err := l.f.VirtualMachineList(l.ctx, "*"); err == nil {
		t.Fatal("old session must be invalid after restore")
	}
	l = snapVC(t, e)
	if !l.exists("snap-vm") || l.exists("after-vm") {
		t.Fatalf("vm set after restore: snap-vm=%v after-vm=%v", l.exists("snap-vm"), l.exists("after-vm"))
	}
	vm = l.vm("snap-vm")
	got := l.props(vm.Reference(), "runtime.powerState", "snapshot", "config.hardware.memoryMB")
	if got.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOn || got.Snapshot == nil || got.Snapshot.RootSnapshotList[0].Name != "s-before" {
		t.Fatalf("vm state after restore: %v %+v", got.Runtime.PowerState, got.Snapshot)
	}
	if vm.Reference() != ref {
		t.Fatalf("MoRef changed: %v → %v", ref, vm.Reference())
	}
	after := mustNSX(t, e, "GET", p4+"/infra/segments/snap-seg", "", 200)
	if after["_revision"] != before["_revision"] || after["unique_id"] != before["unique_id"] {
		t.Fatalf("segment after restore: %v vs %v", after, before)
	}
	mustNSX(t, e, "GET", p4+"/infra/segments/after-seg", "", 404)
	if g := mustNSX(t, e, "GET", p4+"/infra/domains/default/groups/snap-grp", "", 200); g["description"] != nil {
		t.Fatalf("group not restored: %v", g)
	}
	waitRealized(t, e, "/infra/segments/snap-seg")
	if !segmentPortgroup(t, l.c, "/infra/segments/snap-seg") || segmentPortgroup(t, l.c, "/infra/segments/after-seg") {
		t.Fatal("vCenter ↔ NSX relation not consistent after restore")
	}
	// 복원 뒤 작업이 이어진다: 전원·복제·스냅샷 복귀, NSX 수정(현재 revision), 새 Segment
	tk, err = vm.PowerOff(l.ctx)
	l.run("power off after restore", tk, err, "")
	tk, err = vm.RevertToCurrentSnapshot(l.ctx, true)
	l.run("revert vm snapshot", tk, err, "")
	dc, _ := l.f.DefaultDatacenter(l.ctx)
	folders, _ := dc.Folders(l.ctx)
	tk, err = vm.Clone(l.ctx, folders.VmFolder, "snap-clone", types.VirtualMachineCloneSpec{})
	l.run("clone after restore", tk, err, "")
	mustNSX(t, e, "PATCH", p4+"/infra/segments/snap-seg", fmt.Sprintf(`{"description":"x","_revision":%v}`, after["_revision"]), 200)
	mustNSX(t, e, "PATCH", p4+"/infra/segments/new-seg", `{"display_name":"new-seg","transport_zone_path":"`+segTZ+`"}`, 200)
	if code, c := adminJSON(t, e, http.MethodGet, "/v1/consistency", ""); code != 200 || c["consistent"] != true {
		t.Fatalf("consistency after work: %v", c)
	}
	// 같은 Snapshot으로 다시 복원할 수 있다(반복 재현, AC-10)
	if code, res := adminJSON(t, e, http.MethodPost, "/v1/snapshots/last/restore", ""); code != 200 || res["snapshot"] != "s1" || res["consistent"] != true {
		t.Fatalf("second restore: %d %v", code, res)
	}
	l = snapVC(t, e)
	if l.exists("snap-clone") {
		t.Fatal("clone must be gone after second restore")
	}
}

// T-NFR-0208 T-NFR-0207 T-TR-SCN-0104
func TestSnapshotRefusalsAndConsistency(t *testing.T) {
	e := testenv.Start(t, func(c *config.Config) { c.RetainSnapshots = 2 })
	if code, _ := adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"../x"}`); code != 400 {
		t.Fatalf("bad name: %d", code)
	}
	if code, _ := adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"_shutdown"}`); code != 400 {
		t.Fatalf("reserved name: %d", code)
	}
	if code, _ := adminJSON(t, e, http.MethodPost, "/v1/snapshots/none/restore", ""); code != 404 {
		t.Fatalf("missing snapshot: %d", code)
	}
	adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"a"}`)
	if code, _ := adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"a"}`); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}
	adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"b"}`)
	if code, r := adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"c"}`); code != 409 {
		t.Fatalf("retention limit (2): %d %v", code, r)
	}
	// 진행 중 Task가 있으면 만들지 않는다(재처리 정책: 진행 중 작업을 Snapshot에 담지 않음)
	l := snapVC(t, e)
	user := e.User("vcenter").Name
	addFault(t, e, `{"layer":"vcenter","match":{"soap_method":"CreateVM_Task","user":"`+user+`"},"action":{"type":"task_fail","fault":"InsufficientResourcesFault","delay_ms":1500},"count":1}`)
	adminJSON(t, e, http.MethodDelete, "/v1/snapshots/b", "")
	dc, _ := l.f.DefaultDatacenter(l.ctx)
	folders, _ := dc.Folders(l.ctx)
	pool, _ := l.f.DefaultResourcePool(l.ctx)
	tk, err := folders.VmFolder.CreateVM(l.ctx, types.VirtualMachineConfigSpec{Name: "busy", GuestId: "otherGuest64",
		Files: &types.VirtualMachineFileInfo{VmPathName: "[LocalDS_0]"}}, pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code, r := adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"busy"}`); code != 409 {
		t.Fatalf("running task: %d %v", code, r)
	}
	_ = tk.Wait(l.ctx)
	if code, _ := adminJSON(t, e, http.MethodPost, "/v1/snapshots", `{"name":"b"}`); code != 201 {
		t.Fatalf("after task: %d", code)
	}
	// 무결성: 파일이 바뀐 Snapshot은 복원하지 않는다(상태 불변)
	dir := filepath.Join(e.Sim.Cfg.DataDir, "snapshots", "b")
	if err := os.WriteFile(filepath.Join(dir, "nsx.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, r := adminJSON(t, e, http.MethodPost, "/v1/snapshots/b/restore", ""); code != 422 {
		t.Fatalf("corrupt snapshot: %d %v", code, r)
	}
	// 불일치 검출(NFR-0207): 저장소(원본)에서 객체를 직접 지우면 캐시와의 차이를 보고한다
	if code, c := adminJSON(t, e, http.MethodGet, "/v1/consistency", ""); code != 200 || c["consistent"] != true {
		t.Fatalf("baseline consistency: %v", c)
	}
	if _, err := e.Sim.Store.DB.Exec(`DELETE FROM nsx_resources WHERE key = '/policy/api/v1/infra/segments/seg-01'`); err != nil {
		t.Fatal(err)
	}
	_, c := adminJSON(t, e, http.MethodGet, "/v1/consistency", "")
	if c["consistent"] != false || fmt.Sprint(c["issues"]) != "[nsx cache-only /policy/api/v1/infra/segments/seg-01]" {
		t.Fatalf("inconsistency not detected: %v", c)
	}
	// 복원하면 다시 일관된 상태가 된다
	if code, r := adminJSON(t, e, http.MethodPost, "/v1/snapshots/a/restore", ""); code != 200 || r["consistent"] != true {
		t.Fatalf("restore a: %d %v", code, r)
	}
	if code, _ := adminJSON(t, e, http.MethodDelete, "/v1/snapshots/a", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
}

// T-NFR-0205 T-FR-BE-0208: full 프로필은 정상 종료 때 _shutdown Snapshot을 남기고, 선택하면 재기동 시 복원한다.
func TestRestartRecovery(t *testing.T) {
	dataDir := t.TempDir()
	full := func(restore string) func(*config.Config) {
		return func(c *config.Config) { c.DataDir, c.RunProfile, c.RestoreSnapshot = dataDir, "full", restore }
	}
	stop := func(e *testenv.Env) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		e.Sim.Shutdown(ctx)
	}
	e1 := testenv.Start(t, full(""))
	ref := snapVC(t, e1).create("survivor")
	mustNSX(t, e1, "PATCH", p4+"/infra/segments/survivor-seg", `{"display_name":"survivor-seg","transport_zone_path":"`+segTZ+`"}`, 200)
	stop(e1)
	if _, err := os.Stat(filepath.Join(dataDir, "snapshots", "_shutdown", "manifest.json")); err != nil {
		t.Fatalf("shutdown snapshot: %v", err)
	}
	// 복원 선택: 같은 MoRef·같은 NSX 객체로 이어서 시작한다
	e2 := testenv.Start(t, full("_shutdown"))
	l2 := snapVC(t, e2)
	if !l2.exists("survivor") || l2.vm("survivor").Reference() != ref {
		t.Fatal("restart restore: VM missing or MoRef changed")
	}
	mustNSX(t, e2, "GET", p4+"/infra/segments/survivor-seg", "", 200)
	if code, c := adminJSON(t, e2, http.MethodGet, "/v1/consistency", ""); code != 200 || c["consistent"] != true {
		t.Fatalf("consistency after restart restore: %v", c)
	}
	stop(e2)
	// 복원을 선택하지 않으면 Fixture 상태로 시작한다
	e3 := testenv.Start(t, full(""))
	if snapVC(t, e3).exists("survivor") {
		t.Fatal("fresh start must not restore")
	}
	mustNSX(t, e3, "GET", p4+"/infra/segments/survivor-seg", "", 404)
	stop(e3)
	// 없는 Snapshot을 지정하면 기동하지 않는다(조용히 Fixture로 시작하지 않음)
	if _, err := testenv.New(t, full("missing")); err == nil {
		t.Fatal("unknown --restore-snapshot must refuse start")
	}
}
