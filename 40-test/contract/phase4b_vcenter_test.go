// Phase 4b vCenter 생명주기 확장 계약(WP 4.11, FR-VC-0402 ~ 0416, FR-VC-0417·0418). 근거: SRC-VIM-TYPEINFO(메서드·선언 Fault),
// vSphere API 설명. 모든 SOAP 응답을 세션 버전 계약(vimcheck)으로 대조한다. 가정 항목은 BEHAVIOR_SPEC §12.
package contract

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

type lc struct {
	t   *testing.T
	ctx context.Context
	c   *govmomi.Client
	f   *find.Finder
}

func (l lc) vm(name string) *object.VirtualMachine {
	l.t.Helper()
	vm, err := l.f.VirtualMachine(l.ctx, name)
	if err != nil {
		l.t.Fatalf("vm %s: %v", name, err)
	}
	return vm
}

func (l lc) props(ref types.ManagedObjectReference, ps ...string) mo.VirtualMachine {
	l.t.Helper()
	var out mo.VirtualMachine
	if err := property.DefaultCollector(l.c.Client).RetrieveOne(l.ctx, ref, ps, &out); err != nil {
		l.t.Fatal(err)
	}
	return out
}

// run은 Task를 끝까지 기다린다. want가 비면 성공, 아니면 그 Fault 타입(%T 끝부분)이어야 한다. 결과를 돌려준다.
func (l lc) run(what string, tk *object.Task, err error, want string) types.AnyType {
	l.t.Helper()
	if err != nil {
		if want != "" && strings.HasSuffix(fmt.Sprintf("%T", soapFaultOf(err)), "."+want) {
			return nil
		}
		l.t.Fatalf("%s: call error %v", what, err)
	}
	info, err := tk.WaitForResult(l.ctx, nil)
	if want == "" {
		if err != nil {
			l.t.Fatalf("%s: %v", what, err)
		}
		return info.Result
	}
	if err == nil {
		l.t.Fatalf("%s: want %s, succeeded", what, want)
	}
	got := ""
	if info != nil && info.Error != nil {
		got = fmt.Sprintf("%T", info.Error.Fault)
	}
	if !strings.HasSuffix(got, "."+want) {
		l.t.Fatalf("%s: want %s, got %s (%v)", what, want, got, err)
	}
	return nil
}

func (l lc) syncFault(what string, err error, want string) {
	l.t.Helper()
	if got := fmt.Sprintf("%T", soapFaultOf(err)); err == nil || !strings.HasSuffix(got, "."+want) {
		l.t.Fatalf("%s: want %s, got %s (%v)", what, want, got, err)
	}
}

// T-FR-VC-0402 T-FR-VC-0403 T-FR-VC-0407 T-FR-VC-0408 T-FR-VC-0410 T-FR-VC-0411 T-FR-VC-0412 T-FR-VC-0413 T-FR-VC-0414 T-FR-VC-0415 T-FR-VC-0416 T-FR-VC-0417 T-FR-VC-0418
func TestVMLifecycleExtended(t *testing.T) {
	e := testenv.Start(t, nil)
	contract := loadVimContract(t, e)
	c, rec := e.RecordingLogin("")
	ctx := context.Background()
	f := find.NewFinder(c.Client, true)
	dc, err := f.DefaultDatacenter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.SetDatacenter(dc)
	l := lc{t, ctx, c, f}
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	vms, _ := f.VirtualMachineList(ctx, "*")
	src := vms[0]
	srcName, _ := src.ObjectName(ctx)
	if p := l.props(src.Reference(), "runtime.powerState"); p.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOn {
		t.Fatalf("fixture VM must be powered on: %s", p.Runtime.PowerState)
	}

	// --- Clone(FR-VC-0402): 켜진 VM 복제, 사양 덮어쓰기, 이름 중복, 용량 초과 ---
	tk, err := src.Clone(ctx, folders.VmFolder, "lc-clone", types.VirtualMachineCloneSpec{Location: types.VirtualMachineRelocateSpec{Pool: types.NewReference(pool.Reference())},
		Config: &types.VirtualMachineConfigSpec{NumCPUs: 2, MemoryMB: 2048}})
	ref := l.run("clone", tk, err, "")
	clone := object.NewVirtualMachine(c.Client, ref.(types.ManagedObjectReference))
	cp := l.props(clone.Reference(), "config.hardware", "runtime.powerState", "config.template", "config.instanceUuid")
	if cp.Config.Hardware.NumCPU != 2 || cp.Config.Hardware.MemoryMB != 2048 || cp.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOff {
		t.Fatalf("clone result: cpu %d mem %d power %s", cp.Config.Hardware.NumCPU, cp.Config.Hardware.MemoryMB, cp.Runtime.PowerState)
	}
	tk, err = src.Clone(ctx, folders.VmFolder, "lc-clone", types.VirtualMachineCloneSpec{Location: types.VirtualMachineRelocateSpec{Pool: types.NewReference(pool.Reference())}})
	l.run("clone duplicate", tk, err, "DuplicateName")
	tk, err = src.Clone(ctx, folders.VmFolder, "lc-big", types.VirtualMachineCloneSpec{Location: types.VirtualMachineRelocateSpec{Pool: types.NewReference(pool.Reference())},
		Config: &types.VirtualMachineConfigSpec{NumCPUs: 4096}})
	l.run("clone over capacity", tk, err, "InsufficientResourcesFault")

	// --- Template(FR-VC-0415): 켜진 VM 변환 거부, 변환, 변경 알림, 템플릿 작업 거부, 템플릿에서 복제, VM으로 되돌리기 ---
	l.syncFault("mark template powered on", src.MarkAsTemplate(ctx), "InvalidPowerState")
	pc := property.DefaultCollector(c.Client)
	seenTemplate := make(chan bool, 1)
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go func() {
		_ = property.Wait(wctx, pc, clone.Reference(), []string{"config.template"}, func(ch []types.PropertyChange) bool {
			for _, p := range ch {
				if p.Val == true {
					seenTemplate <- true
					return true
				}
			}
			return false
		})
	}()
	time.Sleep(200 * time.Millisecond)
	if err := clone.MarkAsTemplate(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-seenTemplate:
	case <-wctx.Done():
		t.Fatal("WaitForUpdatesEx did not report config.template change")
	}
	tp := l.props(clone.Reference(), "config.template", "summary.config.template", "resourcePool")
	if !tp.Config.Template || !tp.Summary.Config.Template || tp.ResourcePool != nil {
		t.Fatalf("template props: %v %v %v", tp.Config.Template, tp.Summary.Config.Template, tp.ResourcePool)
	}
	tk, err = clone.PowerOn(ctx)
	l.run("power on template", tk, err, "NotSupported")
	tk, err = clone.CreateSnapshot(ctx, "s", "", false, false)
	l.run("snapshot template", tk, err, "NotSupported")
	tk, err = clone.Clone(ctx, folders.VmFolder, "lc-from-tmpl", types.VirtualMachineCloneSpec{Location: types.VirtualMachineRelocateSpec{Pool: types.NewReference(pool.Reference())}, PowerOn: true})
	fromTmpl := object.NewVirtualMachine(c.Client, l.run("clone from template", tk, err, "").(types.ManagedObjectReference))
	if p := l.props(fromTmpl.Reference(), "runtime.powerState", "config.template"); p.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOn || p.Config.Template {
		t.Fatalf("clone from template: %s template=%v", p.Runtime.PowerState, p.Config.Template)
	}
	l.syncFault("mark vm bad pool", clone.MarkAsVirtualMachine(ctx, *object.NewResourcePool(c.Client, folders.VmFolder.Reference()), nil), "InvalidArgument")
	if err := clone.MarkAsVirtualMachine(ctx, *pool, nil); err != nil {
		t.Fatal(err)
	}
	if p := l.props(clone.Reference(), "config.template", "resourcePool"); p.Config.Template || p.ResourcePool == nil {
		t.Fatalf("mark as vm: template=%v pool=%v", p.Config.Template, p.ResourcePool)
	}

	// --- Reset·Suspend(FR-VC-0407·0408): 전원 상태 사전조건 ---
	tk, err = clone.Reset(ctx)
	l.run("reset powered off", tk, err, "InvalidPowerState")
	tk, err = clone.Suspend(ctx)
	l.run("suspend powered off", tk, err, "InvalidPowerState")
	tk, err = fromTmpl.Reset(ctx)
	l.run("reset", tk, err, "")
	tk, err = fromTmpl.Suspend(ctx)
	l.run("suspend", tk, err, "")
	if p := l.props(fromTmpl.Reference(), "runtime.powerState"); p.Runtime.PowerState != types.VirtualMachinePowerStateSuspended {
		t.Fatalf("suspend: %s", p.Runtime.PowerState)
	}
	tk, err = fromTmpl.PowerOn(ctx)
	l.run("resume", tk, err, "")

	// --- Rename·Move(FR-VC-0410·0411) ---
	tk, err = clone.Rename(ctx, "lc-renamed")
	l.run("rename", tk, err, "")
	tk, err = clone.Rename(ctx, srcName)
	l.run("rename duplicate", tk, err, "DuplicateName")
	sub, err := folders.VmFolder.CreateFolder(ctx, "lc-folder")
	if err != nil {
		t.Fatal(err)
	}
	tk, err = sub.MoveInto(ctx, []types.ManagedObjectReference{clone.Reference()})
	l.run("move into folder", tk, err, "")
	if p := l.props(clone.Reference(), "parent", "name"); p.Parent == nil || *p.Parent != sub.Reference() || p.Name != "lc-renamed" {
		t.Fatalf("move/rename: parent %v name %s", p.Parent, p.Name)
	}
	hosts, _ := f.HostSystemList(ctx, "*")
	var other *object.HostSystem
	cur := l.props(fromTmpl.Reference(), "runtime.host").Runtime.Host
	for _, h := range hosts {
		if h.Reference() != *cur {
			other = h
			break
		}
	}
	tk, err = fromTmpl.Relocate(ctx, types.VirtualMachineRelocateSpec{Host: types.NewReference(other.Reference())}, "")
	l.run("relocate", tk, err, "")
	if p := l.props(fromTmpl.Reference(), "runtime.host"); *p.Runtime.Host != other.Reference() {
		t.Fatalf("relocate: host %v", p.Runtime.Host)
	}

	// --- Snapshot(FR-VC-0412 ~ 0414): 메모리 없는 스냅샷 → 복귀 시 꺼짐·하드웨어 복원, 메모리 포함 → 켜짐 유지 ---
	vm := fromTmpl
	baseCPU := l.props(vm.Reference(), "config.hardware").Config.Hardware.NumCPU
	tk, err = vm.CreateSnapshot(ctx, "base", "", false, false)
	s1 := l.run("snapshot base", tk, err, "").(types.ManagedObjectReference)
	tk, err = vm.Reconfigure(ctx, types.VirtualMachineConfigSpec{NumCPUs: 4, MemoryMB: 4096, CpuHotAddEnabled: types.NewBool(true)})
	if err == nil {
		// 켜진 VM의 CPU 증가는 Hot-add가 필요(기존 규칙): 먼저 끄고 바꾼 뒤 다시 켠다
		_ = tk.Wait(ctx)
	}
	tk, _ = vm.PowerOff(ctx)
	_ = tk.Wait(ctx)
	tk, err = vm.Reconfigure(ctx, types.VirtualMachineConfigSpec{NumCPUs: 4, MemoryMB: 4096})
	l.run("reconfigure", tk, err, "")
	tk, _ = vm.PowerOn(ctx)
	_ = tk.Wait(ctx)
	tk, err = vm.CreateSnapshot(ctx, "with-memory", "", true, false)
	s2 := l.run("snapshot with memory", tk, err, "").(types.ManagedObjectReference)
	snap := l.props(vm.Reference(), "snapshot")
	if snap.Snapshot == nil || len(snap.Snapshot.RootSnapshotList) != 1 || len(snap.Snapshot.RootSnapshotList[0].ChildSnapshotList) != 1 ||
		snap.Snapshot.RootSnapshotList[0].State != types.VirtualMachinePowerStatePoweredOff || *snap.Snapshot.CurrentSnapshot != s2 {
		t.Fatalf("snapshot tree: %+v", snap.Snapshot)
	}
	tk, err = object.NewVirtualMachine(c.Client, vm.Reference()).RevertToSnapshot(ctx, s1.Value, false)
	l.run("revert to base", tk, err, "")
	p := l.props(vm.Reference(), "runtime.powerState", "config.hardware", "snapshot.currentSnapshot", "summary.config")
	if p.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOff || p.Config.Hardware.NumCPU != baseCPU || p.Summary.Config.NumCpu != baseCPU {
		t.Fatalf("revert to base: power %s cpu %d/%d", p.Runtime.PowerState, p.Config.Hardware.NumCPU, p.Summary.Config.NumCpu)
	}
	tk, err = vm.RevertToCurrentSnapshot(ctx, true) // suppressPowerOn: base는 꺼진 상태라 그대로
	l.run("revert current suppress", tk, err, "")
	// with-memory 스냅샷으로 복귀(스냅샷 객체 메서드) → 켜짐, CPU 4
	res, err := methods.RevertToSnapshot_Task(ctx, c.Client, &types.RevertToSnapshot_Task{This: s2})
	if err != nil {
		t.Fatal(err)
	}
	l.run("revert to with-memory", object.NewTask(c.Client, res.Returnval), nil, "")
	p = l.props(vm.Reference(), "runtime.powerState", "config.hardware")
	if p.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOn || p.Config.Hardware.NumCPU != 4 {
		t.Fatalf("revert with memory: power %s cpu %d", p.Runtime.PowerState, p.Config.Hardware.NumCPU)
	}
	tk, err = vm.RemoveSnapshot(ctx, "with-memory", false, nil)
	l.run("remove snapshot", tk, err, "")
	tk, err = vm.RemoveAllSnapshot(ctx, nil)
	l.run("remove all", tk, err, "")
	if p := l.props(vm.Reference(), "snapshot"); p.Snapshot != nil {
		t.Fatalf("snapshots left: %+v", p.Snapshot)
	}

	// --- Guest Customization(FR-VC-0416): 켜진 VM 거부, 대기 → 전원 켜기 시 적용·이벤트 ---
	spec := types.CustomizationSpec{
		Identity:         &types.CustomizationLinuxPrep{HostName: &types.CustomizationFixedName{Name: "lc-host"}, Domain: "example.test"},
		GlobalIPSettings: types.CustomizationGlobalIPSettings{},
		NicSettingMap:    []types.CustomizationAdapterMapping{{Adapter: types.CustomizationIPSettings{Ip: &types.CustomizationFixedIp{IpAddress: "10.20.0.5"}}}},
	}
	tk, err = vm.Customize(ctx, spec)
	l.run("customize powered on", tk, err, "InvalidPowerState")
	tk, _ = vm.PowerOff(ctx)
	_ = tk.Wait(ctx)
	tk, err = vm.Customize(ctx, spec)
	l.run("customize", tk, err, "")
	if p := l.props(vm.Reference(), "config.tools"); p.Config.Tools == nil || p.Config.Tools.PendingCustomization == "" {
		t.Fatal("pending customization not set")
	}
	tk, _ = vm.PowerOn(ctx)
	_ = tk.Wait(ctx)
	g := l.props(vm.Reference(), "guest", "config.tools")
	if g.Guest.HostName != "lc-host" || g.Guest.IpAddress != "10.20.0.5" || g.Config.Tools.PendingCustomization != "" {
		t.Fatalf("customization result: host %q ip %q pending %q", g.Guest.HostName, g.Guest.IpAddress, g.Config.Tools.PendingCustomization)
	}
	evs, err := event.NewManager(c.Client).QueryEvents(ctx, types.EventFilterSpec{Entity: &types.EventFilterSpecByEntity{Entity: vm.Reference(), Recursion: types.EventFilterSpecRecursionOptionSelf}})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, ev := range evs {
		kinds = append(kinds, fmt.Sprintf("%T", ev))
	}
	if !strings.Contains(strings.Join(kinds, ","), "CustomizationStartedEvent") {
		t.Fatalf("customization events: %v", kinds)
	}

	// --- Register(FR-VC-0403): 켜진 VM 등록 해제 거부, 등록 해제 → 다시 등록 ---
	l.syncFault("unregister powered on", vm.Unregister(ctx), "InvalidPowerState")
	tk, _ = vm.PowerOff(ctx)
	_ = tk.Wait(ctx)
	path := l.props(vm.Reference(), "config.files").Config.Files.VmPathName
	if err := vm.Unregister(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.VirtualMachine(ctx, "lc-from-tmpl"); err == nil {
		t.Fatal("unregistered VM still in inventory")
	}
	tk, err = folders.VmFolder.RegisterVM(ctx, path, "lc-registered", false, pool, nil)
	reg := l.run("register", tk, err, "").(types.ManagedObjectReference)
	if p := l.props(reg, "name", "config.files"); p.Name != "lc-registered" || p.Config.Files.VmPathName != path {
		t.Fatalf("registered: %s %s", p.Name, p.Config.Files.VmPathName)
	}
	tk, err = folders.VmFolder.RegisterVM(ctx, path, "lc-registered", false, pool, nil)
	l.run("register already registered", tk, err, "AlreadyExists") // 선언 Fault: 이미 등록된 VM

	// 모든 응답이 세션 버전 계약을 지킨다(선언 Fault·필수·순서·버전)
	var findings []string
	for _, x := range rec.Exchanges {
		for _, fd := range contract.CheckResponse(x.Method, x.SessionVersion, x.Response) {
			findings = append(findings, x.Method+" "+fd.String())
		}
	}
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Fatalf("contract findings (%d):\n  %s", len(findings), strings.Join(findings, "\n  "))
	}
}
