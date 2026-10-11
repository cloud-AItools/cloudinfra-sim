package vcenter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// VM 생명주기 확장(WP 4.11, FR-VC-0402 ~ 0416)의 사전조건·vcsim 보완. 메서드 자체는 vcsim 구현을 쓰고, 공식 계약(선언 Fault,
// API 설명)과 다른 부분만 Adapter에서 고친다. 근거가 없는 Fault 선택은 가정이다(BEHAVIOR_SPEC §12).
//
//   - 템플릿: 전원·재설정·일시 중단·스냅샷·재구성·게스트 사용자 지정은 거부한다(NotSupported, 가정 — 실제 제품에서 템플릿은
//     VM으로 변환해야 이 작업을 할 수 있다). vcsim은 템플릿 전원 켜기를 허용한다.
//   - ResetVM_Task: 전원이 켜진 VM만(InvalidPowerState requested=poweredOn). vcsim은 끄기부터 시도해 requested=poweredOff로 실패한다.
//   - CloneVM_Task: 결과 VM의 vCPU·메모리 용량, 대상 데이터스토어 여유 공간 검사(CreateVM_Task와 같은 규칙). vcsim은 내부
//     CreateVM 호출이라 Adapter 검사를 거치지 않는다.
//   - 스냅샷 복귀: 스냅샷 시점의 하드웨어 구성과 전원 상태로 되돌린다(API: RevertToSnapshot_Task suppressPowerOn 설명, 스냅샷
//     트리 state = 스냅샷 시점 전원 상태). vcsim은 currentSnapshot만 바꾼다.
//   - Rename_Task: 같은 폴더의 이름 중복은 DuplicateName, 빈 이름은 InvalidName(선언 Fault). vcsim은 InvalidArgument.
//   - MarkAsTemplate·MarkAsVirtualMachine: vcsim이 속성 변경을 알리지 않아 WaitForUpdatesEx·PropertyCollector가 바뀐 값을
//     놓친다 → 성공 응답 뒤 config.template·summary.config.template·resourcePool 변경을 알린다. MarkAsVirtualMachine의 pool은
//     ResourcePool이어야 한다(InvalidArgument, 가정).

var templateBlocked = map[string]bool{
	"PowerOnVM_Task": true, "ResetVM_Task": true, "SuspendVM_Task": true, "CreateSnapshot_Task": true, "CreateSnapshotEx_Task": true,
	"RevertToCurrentSnapshot_Task": true, "ReconfigVM_Task": true, "CustomizeVM_Task": true,
}

// lifecycleCheck는 Adapter Handler에서 부른다. 처리했으면(거부 또는 대체 Task) fault를 돌려준다.
func (v *VCenter) lifecycleCheck(ctx *simulator.Context, method *simulator.Method) (types.BaseMethodFault, bool) {
	vm, isVM := ctx.Map.Get(method.This).(*simulator.VirtualMachine)
	if isVM && templateBlocked[method.Name] {
		var tmpl bool
		ctx.WithLock(vm, func() { tmpl = vm.Config != nil && vm.Config.Template })
		if tmpl {
			return v.failTask(ctx, method.This, taskName(method.Name), &types.NotSupported{}), true
		}
	}
	switch method.Name {
	case "ResetVM_Task":
		if !isVM {
			return nil, false
		}
		var st types.VirtualMachinePowerState
		ctx.WithLock(vm, func() { st = vm.Runtime.PowerState })
		if st != types.VirtualMachinePowerStatePoweredOn {
			return v.failTask(ctx, method.This, "reset", &types.InvalidPowerState{RequestedState: types.VirtualMachinePowerStatePoweredOn, ExistingState: st}), true
		}
	case "CloneVM_Task":
		req, ok := method.Body.(*types.CloneVM_Task)
		if !ok || !isVM {
			return nil, false
		}
		var cpus int32
		var mem int64
		var ds string
		ctx.WithLock(vm, func() {
			cpus, mem = vm.Config.Hardware.NumCPU, int64(vm.Config.Hardware.MemoryMB)
			ds = vm.Config.Files.VmPathName
		})
		if c := req.Spec.Config; c != nil {
			if c.NumCPUs != 0 {
				cpus = c.NumCPUs
			}
			if c.MemoryMB != 0 {
				mem = c.MemoryMB
			}
		}
		if l := req.Spec.Location.Datastore; l != nil {
			if d, ok := ctx.Map.Get(*l).(*simulator.Datastore); ok {
				ds = "[" + d.Name + "]"
			}
		}
		if f := v.checkCapacity(cpus, mem); f != nil {
			return v.failTask(ctx, method.This, "cloneVm", f), true
		}
		if f := v.checkDatastoreSpace(ctx, ds); f != nil {
			return v.failTask(ctx, method.This, "cloneVm", f), true
		}
	case "MarkAsVirtualMachine":
		req, ok := method.Body.(*types.MarkAsVirtualMachine)
		if !ok {
			return nil, false
		}
		if _, ok := ctx.Map.Get(req.Pool).(*simulator.ResourcePool); !ok {
			return &types.InvalidArgument{InvalidProperty: "pool"}, true
		}
	case "RevertToCurrentSnapshot_Task", "RevertToSnapshot_Task":
		return v.revertSnapshot(ctx, method)
	case "RegisterVM_Task":
		if req, ok := method.Body.(*types.RegisterVM_Task); ok {
			v.prepareRegister(ctx, req)
		}
	case "Rename_Task":
		// ManagedEntity.Rename_Task 선언 Fault: DuplicateName(같은 폴더에 같은 이름), InvalidName. vcsim은 InvalidArgument.
		req, ok := method.Body.(*types.Rename_Task)
		if !ok {
			return nil, false
		}
		ent, ok := ctx.Map.Get(method.This).(mo.Entity)
		if !ok {
			return nil, false
		}
		if req.NewName == "" || len(req.NewName) > 80 {
			return v.failTask(ctx, method.This, "rename", &types.InvalidName{Name: req.NewName}), true // 길이 한도 80은 가정
		}
		var parent *types.ManagedObjectReference
		ctx.WithLock(ent, func() { parent = ent.Entity().Parent })
		if parent == nil {
			return nil, false
		}
		if f, ok := ctx.Map.Get(*parent).(*simulator.Folder); ok {
			var dup mo.Reference
			ctx.WithLock(f, func() { dup = ctx.Map.FindByName(req.NewName, f.ChildEntity) })
			if dup != nil && dup.Reference() != method.This {
				return v.failTask(ctx, method.This, "rename", &types.DuplicateName{Name: req.NewName, Object: dup.Reference()}), true
			}
		}
	}
	return nil, false
}

// revertSnapshot은 스냅샷 복귀 Task를 Adapter가 직접 만든다(vcsim Task 수명주기·Context 그대로).
func (v *VCenter) revertSnapshot(ctx *simulator.Context, method *simulator.Method) (types.BaseMethodFault, bool) {
	var vm *simulator.VirtualMachine
	var snapRef types.ManagedObjectReference
	suppress := false
	switch req := method.Body.(type) {
	case *types.RevertToCurrentSnapshot_Task:
		vm, _ = ctx.Map.Get(method.This).(*simulator.VirtualMachine)
		if vm == nil {
			return nil, false
		}
		var cur *types.ManagedObjectReference
		ctx.WithLock(vm, func() {
			if vm.Snapshot != nil {
				cur = vm.Snapshot.CurrentSnapshot
			}
		})
		if cur == nil {
			return nil, false // vcsim: NotFound(선언 Fault)
		}
		snapRef, suppress = *cur, req.SuppressPowerOn != nil && *req.SuppressPowerOn
	case *types.RevertToSnapshot_Task:
		s, ok := ctx.Map.Get(method.This).(*simulator.VirtualMachineSnapshot)
		if !ok {
			return nil, false
		}
		vm, _ = ctx.Map.Get(s.Vm).(*simulator.VirtualMachine)
		if vm == nil {
			return nil, false
		}
		snapRef, suppress = s.Self, req.SuppressPowerOn != nil && *req.SuppressPowerOn
	default:
		return nil, false
	}
	snap, ok := ctx.Map.Get(snapRef).(*simulator.VirtualMachineSnapshot)
	if !ok {
		return nil, false
	}
	var tmpl bool
	ctx.WithLock(vm, func() { tmpl = vm.Config.Template })
	if tmpl {
		return v.failTask(ctx, method.This, "revertSnapshot", &types.NotSupported{}), true
	}
	task := simulator.CreateTask(vm, "revertSnapshot", func(*simulator.Task) (types.AnyType, types.BaseMethodFault) {
		var target, cur types.VirtualMachinePowerState
		ctx.WithLock(vm, func() {
			if tree := findTree(vm.Snapshot.RootSnapshotList, snapRef); tree != nil {
				target = tree.State
			}
			cur = vm.Runtime.PowerState
			hw := snap.Config.Hardware
			hw.Device = append([]types.BaseVirtualDevice(nil), snap.Config.Hardware.Device...)
			sum := vm.Summary.Config
			sum.NumCpu, sum.MemorySizeMB = hw.NumCPU, hw.MemoryMB
			ctx.Map.Update(ctx, vm, []types.PropertyChange{
				{Name: "config.hardware", Val: hw},
				{Name: "summary.config", Val: sum},
				{Name: "snapshot.currentSnapshot", Val: snapRef},
			})
		})
		if suppress && target == types.VirtualMachinePowerStatePoweredOn {
			target = types.VirtualMachinePowerStatePoweredOff
		}
		if target == "" || target == cur {
			return nil, nil
		}
		// 전원 전이는 vcsim 전원 Task로 한다(이벤트·runtime 갱신 포함). 일시 중단 상태로의 복귀는 켠 뒤 일시 중단한다.
		step := func(st types.VirtualMachinePowerState) types.BaseMethodFault {
			var res any
			switch st {
			case types.VirtualMachinePowerStatePoweredOn:
				res = vm.PowerOnVMTask(ctx, &types.PowerOnVM_Task{This: vm.Self})
			case types.VirtualMachinePowerStatePoweredOff:
				res = vm.PowerOffVMTask(ctx, &types.PowerOffVM_Task{This: vm.Self})
			case types.VirtualMachinePowerStateSuspended:
				res = vm.SuspendVMTask(ctx, &types.SuspendVM_Task{This: vm.Self})
			}
			var ref types.ManagedObjectReference
			switch b := res.(type) {
			case *methods.PowerOnVM_TaskBody:
				ref = b.Res.Returnval
			case *methods.PowerOffVM_TaskBody:
				ref = b.Res.Returnval
			case *methods.SuspendVM_TaskBody:
				ref = b.Res.Returnval
			}
			t, ok := ctx.Map.Get(ref).(*simulator.Task)
			if !ok {
				return &types.SystemError{Reason: fmt.Sprintf("power transition to %s failed", st)}
			}
			t.Wait()
			if t.Info.Error != nil {
				return t.Info.Error.Fault
			}
			return nil
		}
		switch {
		case target == types.VirtualMachinePowerStateSuspended && cur != types.VirtualMachinePowerStatePoweredOn:
			if cur == types.VirtualMachinePowerStatePoweredOff {
				if f := step(types.VirtualMachinePowerStatePoweredOn); f != nil {
					return nil, f
				}
			}
			return nil, step(types.VirtualMachinePowerStateSuspended)
		case target == types.VirtualMachinePowerStatePoweredOn && cur == types.VirtualMachinePowerStateSuspended:
			return nil, step(types.VirtualMachinePowerStatePoweredOn)
		default:
			return nil, step(target)
		}
	})
	ref := task.Run(ctx)
	return &types.SystemError{Reason: adapterMarker + ref.Value}, true
}

// prepareRegister는 vcsim RegisterVM의 공백(R-36)을 메운다: vcsim은 등록 이름으로 '<이름>.nvram'을 열어 다른 이름으로
// 등록하면 NotFound다. 실제 제품의 name은 인벤토리 표시 이름일 뿐이므로, VM 폴더에 nvram이 하나 있고 새 이름의 nvram이
// 없으면 그 파일을 새 이름으로 복사해 vcsim 등록 경로가 열 수 있게 한다(로컬 데이터스토어만).
func (v *VCenter) prepareRegister(ctx *simulator.Context, req *types.RegisterVM_Task) {
	var p object.DatastorePath
	if req.Name == "" || !p.FromString(req.Path) {
		return
	}
	for _, obj := range ctx.Map.AllReference("Datastore") {
		ds, ok := obj.(*simulator.Datastore)
		if !ok || ds.Name != p.Datastore || ds.Summary.Url == "" {
			continue
		}
		dir := filepath.Dir(filepath.Join(filepath.FromSlash(ds.Summary.Url), filepath.FromSlash(p.Path)))
		want := filepath.Join(dir, req.Name+".nvram")
		if _, err := os.Stat(want); err == nil {
			return
		}
		found, _ := filepath.Glob(filepath.Join(dir, "*.nvram"))
		if len(found) != 1 {
			return
		}
		if b, err := os.ReadFile(found[0]); err == nil {
			_ = os.WriteFile(want, b, 0o600)
		}
		return
	}
}

func findTree(trees []types.VirtualMachineSnapshotTree, ref types.ManagedObjectReference) *types.VirtualMachineSnapshotTree {
	for i := range trees {
		if trees[i].Snapshot == ref {
			return &trees[i]
		}
		if t := findTree(trees[i].ChildSnapshotList, ref); t != nil {
			return t
		}
	}
	return nil
}

var thisRe = regexp.MustCompile(`<_this[^>]*type="VirtualMachine"[^>]*>([^<]+)</_this>`)

// afterTemplateChange는 MarkAsTemplate·MarkAsVirtualMachine 성공 뒤 바뀐 속성을 알린다.
func (v *VCenter) afterTemplateChange(m *simulator.Model, reqBody []byte) {
	sm := thisRe.FindSubmatch(reqBody)
	if sm == nil {
		return
	}
	vm, ok := m.Map().Get(types.ManagedObjectReference{Type: "VirtualMachine", Value: string(sm[1])}).(*simulator.VirtualMachine)
	if !ok {
		return
	}
	ctx := &simulator.Context{Map: m.Map(), Context: context.Background(), Session: &simulator.Session{}}
	ctx.WithLock(vm, func() {
		ch := []types.PropertyChange{
			{Name: "config.template", Val: vm.Config.Template},
			{Name: "summary.config.template", Val: vm.Summary.Config.Template},
		}
		if vm.ResourcePool != nil {
			ch = append(ch, types.PropertyChange{Name: "resourcePool", Val: *vm.ResourcePool})
		} else {
			ch = append(ch, types.PropertyChange{Name: "resourcePool", Val: nil})
		}
		ctx.Map.Update(ctx, vm, ch)
	})
}
