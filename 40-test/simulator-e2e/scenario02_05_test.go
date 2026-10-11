// Scenario 02 ~ 05 — vCenter 인벤토리 동기화와 VM 생명주기 (PRD-2 TR-E2E-0201 ~ TR-E2E-0504, Phase 2a 종료 게이트).
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

type vcTarget struct {
	c      *govmomi.Client
	finder *find.Finder
	folder *object.Folder
	pool   *object.ResourcePool
	ds     *object.Datastore
}

func target(t *testing.T, e *testenv.Env) *vcTarget {
	t.Helper()
	ctx := context.Background()
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, err := f.DefaultDatacenter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.SetDatacenter(dc)
	folders, _ := dc.Folders(ctx)
	pool, err := f.DefaultResourcePool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dss, _ := f.DatastoreList(ctx, "*")
	return &vcTarget{c: c, finder: f, folder: folders.VmFolder, pool: pool, ds: dss[0]}
}

func (v *vcTarget) create(t *testing.T, name string, cpus int32, memMB int64) (types.ManagedObjectReference, error) {
	t.Helper()
	ctx := context.Background()
	dsName, _ := v.ds.ObjectName(ctx)
	tk, err := v.folder.CreateVM(ctx, types.VirtualMachineConfigSpec{
		Name: name, GuestId: string(types.VirtualMachineGuestOsIdentifierOtherGuest64), NumCPUs: cpus, MemoryMB: memMB,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"},
	}, v.pool, nil)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}
	ti, err := tk.WaitForResult(ctx, nil)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}
	return ti.Result.(types.ManagedObjectReference), nil
}

func taskFault(t *testing.T, err error) types.BaseMethodFault {
	t.Helper()
	var te task.Error
	if !errorsAs(err, &te) {
		t.Fatalf("want task error, got %v", err)
	}
	return te.Fault()
}

func errorsAs(err error, te *task.Error) bool {
	if e, ok := err.(task.Error); ok {
		*te = e
		return true
	}
	return false
}

func restVM(t *testing.T, e *testenv.Env, id string) (int, map[string]any) {
	t.Helper()
	u := e.User("vcenter")
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.SetBasicAuth(u.Name, u.Password)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var token string
	_ = json.NewDecoder(res.Body).Decode(&token)
	res.Body.Close()
	req, _ = http.NewRequest(http.MethodGet, "https://vcenter."+testenv.Domain+"/api/vcenter/vm/"+id, nil)
	req.Header.Set("vmware-api-session-id", token)
	res, err = e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out
}

// TR-E2E-0201 절차, 성공 기준 0202 ~ 0205.
// 추적: T-TR-E2E-0201 T-TR-E2E-0202 T-TR-E2E-0203 T-TR-E2E-0204 T-TR-E2E-0205 T-FR-BE-0201 T-FR-BE-0202 T-FR-DM-0205 T-FR-DM-0206 T-FR-VC-0509 T-FR-BE-0404
func TestScenario02_InventorySync(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	client := target(t, e)
	vms, _ := client.finder.VirtualMachineList(ctx, "*")
	before := len(vms)
	ref := vms[0].Reference()

	// ① 클라이언트가 변경 감시 시작
	pc, _ := property.DefaultCollector(client.c.Client).Create(ctx)
	defer pc.Destroy(ctx)
	f := new(property.WaitFilter).Add(ref, "VirtualMachine", []string{"config.hardware.numCPU"})
	if _, err := pc.CreateFilter(ctx, f.CreateFilter); err != nil {
		t.Fatal(err)
	}
	ver := ""
	wait := func() *types.UpdateSet {
		sec := int32(10)
		r, err := methods.WaitForUpdatesEx(ctx, client.c.Client, &types.WaitForUpdatesEx{This: pc.Reference(), Version: ver, Options: &types.WaitOptions{MaxWaitSeconds: &sec}})
		if err != nil {
			t.Fatal(err)
		}
		if r.Returnval != nil {
			ver = r.Returnval.Version
		}
		return r.Returnval
	}
	_ = wait() // 초기 상태

	// ② 다른 세션(시뮬레이터 측 변경 주체)이 CPU를 바꾼다(전원을 끈 뒤 변경 — 켜진 VM은 Hot-add 규칙 적용)
	changeErr := make(chan error, 1)
	go func() {
		other := e.Login("")
		vm := object.NewVirtualMachine(other.Client, ref)
		if tk, err := vm.PowerOff(ctx); err == nil {
			_ = tk.Wait(ctx)
		}
		tk, err := vm.Reconfigure(ctx, types.VirtualMachineConfigSpec{NumCPUs: 6})
		if err == nil {
			err = tk.Wait(ctx)
		}
		changeErr <- err
	}()
	// ③ 변경 감시로 수신 ④ 변경된 CPU 반영
	deadline := time.Now().Add(10 * time.Second)
	got := int32(0)
	for got != 6 && time.Now().Before(deadline) {
		us := wait()
		if us == nil {
			continue
		}
		for _, ch := range us.FilterSet[0].ObjectSet[0].ChangeSet {
			if ch.Name == "config.hardware.numCPU" {
				got = ch.Val.(int32)
			}
		}
	}
	if got != 6 {
		t.Fatalf("T-TR-E2E-0203: CPU change not observed (got %d, change error: %v)", got, <-changeErr)
	}
	// 재조회: 같은 ID, 바뀐 값, 중복 없음
	var mvm mo.VirtualMachine
	if err := client.c.RetrieveOne(ctx, ref, []string{"config.hardware.numCPU"}, &mvm); err != nil || mvm.Config.Hardware.NumCPU != 6 {
		t.Fatalf("T-FR-BE-0202: re-query %v %v", err, mvm.Config)
	}
	vms, _ = client.finder.VirtualMachineList(ctx, "*")
	if len(vms) != before {
		t.Fatalf("T-TR-E2E-0204: duplicate VMs %d -> %d", before, len(vms))
	}
	if mvm.Self != ref {
		t.Fatal("T-TR-E2E-0202: VM ID changed")
	}
}

// TR-E2E-0301 절차, 성공 기준 0302 ~ 0305.
// 추적: T-TR-E2E-0301 T-TR-E2E-0302 T-TR-E2E-0303 T-TR-E2E-0304 T-TR-E2E-0305 T-FR-VC-0401 T-FR-VC-0501~0506 T-FR-VC-0510 T-FR-BE-0203 T-FR-BE-0501 T-FR-BE-0503 T-FR-DM-0203 T-FR-BE-0101 T-FR-BE-0404
func TestScenario03_CreateVM(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	v := target(t, e)
	dsName, _ := v.ds.ObjectName(ctx)
	tk, err := v.folder.CreateVM(ctx, types.VirtualMachineConfigSpec{
		Name: "s03-vm", GuestId: string(types.VirtualMachineGuestOsIdentifierOtherGuest64), NumCPUs: 2, MemoryMB: 2048,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"},
	}, v.pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Reference().Type != "Task" {
		t.Fatalf("T-FR-VC-0501: %v", tk.Reference())
	}
	ti, err := tk.WaitForResult(ctx, nil)
	if err != nil || ti.State != types.TaskInfoStateSuccess || ti.StartTime == nil || ti.CompleteTime == nil {
		t.Fatalf("T-FR-VC-0502~0504: %v %+v", err, ti)
	}
	ref := ti.Result.(types.ManagedObjectReference)

	// 관계: VM → Host·ResourcePool·Datastore, 역방향 Host.vm·Datastore.vm
	var vm mo.VirtualMachine
	if err := v.c.RetrieveOne(ctx, ref, []string{"name", "runtime.host", "resourcePool", "datastore", "config.instanceUuid", "config.uuid"}, &vm); err != nil {
		t.Fatal(err)
	}
	if vm.Runtime.Host == nil || vm.ResourcePool == nil || len(vm.Datastore) == 0 {
		t.Fatalf("T-TR-E2E-0304: relations missing %+v", vm)
	}
	var host mo.HostSystem
	_ = v.c.RetrieveOne(ctx, *vm.Runtime.Host, []string{"vm"}, &host)
	var ds mo.Datastore
	_ = v.c.RetrieveOne(ctx, vm.Datastore[0], []string{"vm"}, &ds)
	if !containsRef(host.Vm, ref) || !containsRef(ds.Vm, ref) {
		t.Fatal("T-FR-BE-0203: reverse relations missing")
	}
	if vm.Config.InstanceUuid == "" || vm.Config.Uuid == "" || vm.Config.InstanceUuid == vm.Config.Uuid {
		t.Fatalf("T-FR-DM-0203: ids %q %q", vm.Config.InstanceUuid, vm.Config.Uuid)
	}
	// Task·인벤토리·REST 일치
	code, info := restVM(t, e, ref.Value)
	if code != 200 || info["name"] != "s03-vm" || info["cpu"].(map[string]any)["count"] != float64(2) {
		t.Fatalf("T-FR-VC-0510: REST view %d %v", code, info)
	}
	// 내부 작업 기록(FR-BE-0501): QUEUED→RUNNING→SUCCESS 전이가 저장소에 남는다
	time.Sleep(200 * time.Millisecond)
	var states []string
	rows, err := e.Sim.Store.DB.Query("SELECT to_state FROM operation_transitions WHERE op_id = ? ORDER BY seq", tk.Reference().Value)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		states = append(states, s)
	}
	rows.Close()
	if len(states) == 0 || states[len(states)-1] != "SUCCESS" {
		t.Fatalf("T-FR-BE-0501: transitions %v", states)
	}
	t.Logf("task %s transitions: %v", tk.Reference().Value, states)
}

func containsRef(list []types.ManagedObjectReference, r types.ManagedObjectReference) bool {
	for _, x := range list {
		if x == r {
			return true
		}
	}
	return false
}

// TR-E2E-0401 절차, 성공 기준 0402·0403.
// 추적: T-TR-E2E-0401 T-TR-E2E-0402 T-TR-E2E-0403 T-FR-VC-0404 T-FR-VC-0405 T-FR-VC-0406 T-FR-VC-0417 T-FR-VC-0418 T-FR-BE-0207 T-FR-BE-0401 T-FR-BE-0402 T-FR-BE-0701 T-FR-BE-0702 T-FR-BE-0101 T-FR-BE-0404
func TestScenario04_Reconfigure(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	v := target(t, e)
	ref, err := v.create(t, "s04-vm", 2, 2048)
	if err != nil {
		t.Fatal(err)
	}
	vm := object.NewVirtualMachine(v.c.Client, ref)
	reconfig := func(spec types.VirtualMachineConfigSpec) error {
		tk, err := vm.Reconfigure(ctx, spec)
		if err != nil {
			return err
		}
		return tk.Wait(ctx)
	}
	// 전원 꺼짐: 변경 성공, Config·Summary 반영
	if err := reconfig(types.VirtualMachineConfigSpec{NumCPUs: 4, MemoryMB: 4096}); err != nil {
		t.Fatalf("powered-off reconfigure: %v", err)
	}
	var m mo.VirtualMachine
	_ = v.c.RetrieveOne(ctx, ref, []string{"config.hardware", "summary.config"}, &m)
	if m.Config.Hardware.NumCPU != 4 || m.Config.Hardware.MemoryMB != 4096 || m.Summary.Config.NumCpu != 4 || m.Summary.Config.MemorySizeMB != 4096 {
		t.Fatalf("T-TR-E2E-0402: config/summary %+v %+v", m.Config.Hardware, m.Summary.Config)
	}
	// 용량 초과: InsufficientResourcesFault (Task error, Adapter)
	err = reconfig(types.VirtualMachineConfigSpec{NumCPUs: 128})
	if _, ok := taskFault(t, err).(*types.InsufficientResourcesFault); !ok {
		t.Fatalf("T-FR-BE-0701: want InsufficientResourcesFault, got %v", err)
	}
	// 전원 켜짐 + Hot-add 꺼짐: CPU 증가 거부(behavior-rules, 가정 Fault)
	tk, _ := vm.PowerOn(ctx)
	if err := tk.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	err = reconfig(types.VirtualMachineConfigSpec{NumCPUs: 6})
	if _, ok := taskFault(t, err).(*types.InvalidPowerState); !ok {
		t.Fatalf("T-FR-VC-0418: want InvalidPowerState, got %v", err)
	}
	// 거부된 Task와 리소스 상태는 구분된다: 값은 그대로 4
	_ = v.c.RetrieveOne(ctx, ref, []string{"config.hardware"}, &m)
	if m.Config.Hardware.NumCPU != 4 {
		t.Fatalf("T-FR-BE-0207: rejected task changed state: %d", m.Config.Hardware.NumCPU)
	}
	// Hot-add를 켠 뒤(전원 끔 상태에서 설정) 켜진 상태 증가는 허용
	tk, _ = vm.PowerOff(ctx)
	_ = tk.Wait(ctx)
	if err := reconfig(types.VirtualMachineConfigSpec{CpuHotAddEnabled: types.NewBool(true)}); err != nil {
		t.Fatal(err)
	}
	tk, _ = vm.PowerOn(ctx)
	_ = tk.Wait(ctx)
	if err := reconfig(types.VirtualMachineConfigSpec{NumCPUs: 6}); err != nil {
		t.Fatalf("hot-add increase: %v", err)
	}
	code, info := restVM(t, e, ref.Value)
	if code != 200 || info["cpu"].(map[string]any)["count"] != float64(6) || info["power_state"] != "POWERED_ON" {
		t.Fatalf("T-TR-E2E-0403: REST %d %v", code, info["cpu"])
	}
}

// TR-E2E-0501 절차, 성공 기준 0502 ~ 0504.
// 추적: T-TR-E2E-0501 T-TR-E2E-0502 T-TR-E2E-0503 T-TR-E2E-0504 T-FR-VC-0409 T-FR-BE-0206 T-FR-DM-0208 T-FR-FLT-0105
func TestScenario05_Delete(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	v := target(t, e)
	ref, err := v.create(t, "s05-vm", 1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	vm := object.NewVirtualMachine(v.c.Client, ref)
	var before mo.VirtualMachine
	_ = v.c.RetrieveOne(ctx, ref, []string{"runtime.host", "datastore", "network", "resourcePool", "parent"}, &before)
	tk, _ := vm.PowerOn(ctx)
	_ = tk.Wait(ctx)
	// ② 사전조건: 켜진 VM 삭제는 InvalidPowerState
	tk, err = vm.Destroy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := taskFault(t, tk.Wait(ctx)).(*types.InvalidPowerState); !ok {
		t.Fatal("T-FR-VC-0417: destroy of powered-on VM must fail with InvalidPowerState")
	}
	tk, _ = vm.PowerOff(ctx)
	_ = tk.Wait(ctx)
	tk, _ = vm.Destroy(ctx)
	if err := tk.Wait(ctx); err != nil {
		t.Fatalf("T-TR-E2E-0502: %v", err)
	}
	// 재조회: SOAP ManagedObjectNotFound, REST NOT_FOUND
	var gone mo.VirtualMachine
	err = v.c.RetrieveOne(ctx, ref, []string{"name"}, &gone)
	if err == nil || !soap.IsSoapFault(err) {
		t.Fatalf("T-TR-E2E-0503: re-query after delete: %v", err)
	}
	if _, ok := soap.ToSoapFault(err).VimFault().(types.ManagedObjectNotFound); !ok {
		t.Fatalf("T-TR-E2E-0503: want ManagedObjectNotFound, got %T", soap.ToSoapFault(err).VimFault())
	}
	if code, body := restVM(t, e, ref.Value); code != http.StatusNotFound || body["error_type"] != "NOT_FOUND" {
		t.Fatalf("T-TR-E2E-0503 REST: %d %v", code, body)
	}
	// 고아 참조 없음
	var host mo.HostSystem
	_ = v.c.RetrieveOne(ctx, *before.Runtime.Host, []string{"vm"}, &host)
	var ds mo.Datastore
	_ = v.c.RetrieveOne(ctx, before.Datastore[0], []string{"vm"}, &ds)
	var rp mo.ResourcePool
	_ = v.c.RetrieveOne(ctx, *before.ResourcePool, []string{"vm"}, &rp)
	var folder mo.Folder
	_ = v.c.RetrieveOne(ctx, *before.Parent, []string{"childEntity"}, &folder)
	if containsRef(host.Vm, ref) || containsRef(ds.Vm, ref) || containsRef(rp.Vm, ref) || containsRef(folder.ChildEntity, ref) {
		t.Fatal("T-TR-E2E-0504: orphan reference remains")
	}
	for _, n := range before.Network {
		var net mo.Network
		_ = v.c.RetrieveOne(ctx, n, []string{"vm"}, &net)
		if containsRef(net.Vm, ref) {
			t.Fatal("T-FR-BE-0206: network still references deleted VM")
		}
	}
	// 삭제 후 ID 재사용 없음(FR-DM-0208)
	ref2, err := v.create(t, "s05-vm", 1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if ref2 == ref {
		t.Fatal("T-FR-DM-0208: MoRef reused after delete")
	}
}
