package vcenter

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// Behavior Adapter (WP 2.11, FR-BE-0401 ~ FR-BE-0406).
//
// 요청 경로 안에서 동기로 사전조건·용량을 검사한다(FR-BE-0402). 위반이면 vcsim 대신 Adapter가 실패하는 Task를
// 만들고(같은 세션 Context, vcsim Task 수명주기 그대로), 응답은 정상 *_TaskResponse로 돌려준다 — 클라이언트에게는
// 실제 vCenter처럼 Task error로 보인다. vcsim 객체를 래퍼로 교체하지 않는다(vcsim 내부에 *VirtualMachine 타입 단언이
// 29곳 있어 안전하지 않음). 외부 프로세스에서 SOAP을 반복 호출해 상태를 바꾸지 않는다(FR-BE-0406).

// adapterMarker는 Registry.Handler가 돌려주는 표지 Fault의 Reason 접두어다. soap()이 이를 Task 응답으로 바꾼다.
const adapterMarker = "cloudinfra-sim-adapter-task:"

var markerRe = regexp.MustCompile(regexp.QuoteMeta(adapterMarker) + `(task-[0-9]+)`)

// hostCapacity는 Fixture의 호스트당 가상 용량이다(FR-BE-0701).
func (v *VCenter) hostCapacity() (cpu int32, memMB int64) {
	return int32(v.fix.VCenter.HostCPUCores), int64(v.fix.VCenter.HostMemoryGB) * 1024
}

func (v *VCenter) installAdapter(m *simulator.Model) {
	m.Map().Handler = func(ctx *simulator.Context, method *simulator.Method) (mo.Reference, types.BaseMethodFault) {
		noteSessionCall(ctx, method) // vcsim 세션 객체 락 누수 보완: 파괴·세션 종료(viewlocks.go, R-45 ④)
		if ctx.Context != nil && ctx.Context.Value(internalCall{}) != nil {
			return nil, nil // 시뮬레이터 내부 조회(Snapshot 저장 등): 세션·권한·Fault 검사 대상이 아니다
		}
		if strings.HasSuffix(method.Name, "_Task") {
			// 이 요청에서 만들어지는 Task의 info.name을 WSDL 메서드 이름으로 고친다(taskObserver.PutObject).
			ctx.Context = context.WithValue(ctx.Context, wsdlMethodKey{}, method.Name)
		}
		// 세션 유휴·강제 만료(WP 4.1) → NotAuthenticated, 그다음 vCenter 의미 계층 Fault(WP 4.3)
		if f := v.checkSOAPSession(ctx, method.Name); f != nil {
			return nil, f
		}
		if f := v.checkPermission(ctx, method); f != nil { // 권한(WP 4.2)
			return nil, f
		}
		if f := v.applyFault(ctx, method); f != nil {
			return nil, f
		}
		if f, handled := v.lifecycleCheck(ctx, method); handled { // VM 생명주기 사전조건·vcsim 보완(WP 4.11, lifecycle.go)
			return nil, f
		}
		if f, handled := unimplemented(ctx, method); handled { // vcsim 미구현 메서드(clusterusage.go)
			return nil, f
		}
		switch method.Name {
		case "RetrieveProperties", "RetrievePropertiesEx":
			if ctx.Session == nil {
				if obj, ok := anonymousTarget(method); !ok {
					return nil, &types.NotAuthenticated{NoPermission: types.NoPermission{Object: &obj, PrivilegeId: "System.View"}}
				}
			}
		case "CreateVM_Task":
			req, ok := method.Body.(*types.CreateVM_Task)
			if !ok {
				return nil, nil
			}
			if req.Config.Files != nil {
				if f := v.checkDatastoreSpace(ctx, req.Config.Files.VmPathName); f != nil {
					return nil, v.failTask(ctx, method.This, "createVm", f)
				}
			}
			if f := v.checkDuplicateName(ctx, method.This, req.Config.Name); f != nil {
				return nil, v.failTask(ctx, method.This, "createVm", f)
			}
			if f := v.checkCapacity(req.Config.NumCPUs, int64(req.Config.MemoryMB)); f != nil {
				return nil, v.failTask(ctx, method.This, "createVm", f)
			}
		case "ReconfigVM_Task":
			req, ok := method.Body.(*types.ReconfigVM_Task)
			if !ok {
				return nil, nil
			}
			vm, ok := ctx.Map.Get(method.This).(*simulator.VirtualMachine)
			if !ok {
				return nil, nil // 없는 객체: vcsim의 ManagedObjectNotFound
			}
			v.notePBMReconfig(ctx, vm, &req.Spec) // 디스크 정책: 요청 직전 디스크 목록(pbm.go)
			var f types.BaseMethodFault
			ctx.WithLock(vm, func() {
				if f = v.checkReconfigure(vm, &req.Spec); f == nil && req.Spec.VmOpNotificationToAppEnabled != nil {
					// vcsim이 반영하지 않는 ConfigSpec 필드(7.0.3.0 도입): ConfigInfo의 같은 이름 필드로 옮긴다(API 문서 의미).
					// 응답은 세션 버전 필터를 거치므로 7.0.3.0 미만 세션에는 나타나지 않는다(WP 4.7 버전 간 비교).
					vm.Config.VmOpNotificationToAppEnabled = req.Spec.VmOpNotificationToAppEnabled
				}
			})
			if f != nil {
				return nil, v.failTask(ctx, method.This, "reconfigVM", f)
			}
		}
		return nil, nil
	}
}

// anonymousTarget: 세션 없는 속성 조회는 ServiceInstance만 허용한다(가정). vcsim은 세션 없이 모든 객체의
// RetrieveProperties를 허용하지만, 실제 vCenter는 인벤토리 조회에 NotAuthenticated를 돌려준다.
// Fetch는 vcsim이 대상을 PropertyCollector로 바꿔 원래 객체를 알 수 없으므로 제한하지 않는다(R-24).
// 거부 시 첫 번째 비허용 객체를 돌려준다.
func anonymousTarget(method *simulator.Method) (types.ManagedObjectReference, bool) {
	var specs []types.PropertyFilterSpec
	switch b := method.Body.(type) {
	case *types.RetrieveProperties:
		specs = b.SpecSet
	case *types.RetrievePropertiesEx:
		specs = b.SpecSet
	}
	for _, s := range specs {
		for _, o := range s.ObjectSet {
			if o.Obj.Type != "ServiceInstance" || o.Skip != nil && *o.Skip || len(o.SelectSet) > 0 {
				return o.Obj, false
			}
		}
	}
	return types.ManagedObjectReference{}, true
}

// checkDuplicateName: 같은 폴더에 같은 이름의 개체가 있으면 DuplicateName(vSphere API CreateVM_Task 문서화 Fault).
// vcsim은 vmx 파일 충돌만 검사하고 폴더 내 이름 중복은 허용한다.
func (v *VCenter) checkDuplicateName(ctx *simulator.Context, folder types.ManagedObjectReference, name string) types.BaseMethodFault {
	f, ok := ctx.Map.Get(folder).(*simulator.Folder)
	if !ok || name == "" {
		return nil
	}
	var dup mo.Reference
	ctx.WithLock(f, func() { dup = ctx.Map.FindByName(name, f.ChildEntity) })
	if dup == nil {
		return nil
	}
	return &types.DuplicateName{Name: name, Object: dup.Reference()}
}

// checkCapacity: VM 하나가 호스트 가상 용량을 넘으면 InsufficientResourcesFault(behavior-rules capacity, 가정).
func (v *VCenter) checkCapacity(cpus int32, memMB int64) types.BaseMethodFault {
	maxCPU, maxMem := v.hostCapacity()
	if (maxCPU > 0 && cpus > maxCPU) || (maxMem > 0 && memMB > maxMem) {
		return &types.InsufficientResourcesFault{
			VimFault: types.VimFault{MethodFault: types.MethodFault{FaultMessage: []types.LocalizableMessage{{
				Key: "cloudinfra-sim.capacity", Message: fmt.Sprintf("requested %d vCPU / %d MB exceeds host capacity %d vCPU / %d MB", cpus, memMB, maxCPU, maxMem),
			}}}},
		}
	}
	return nil
}

// checkReconfigure: 용량, 전원이 켜진 VM의 CPU·메모리 변경 규칙(behavior-rules reconfigure_powered_on).
func (v *VCenter) checkReconfigure(vm *simulator.VirtualMachine, spec *types.VirtualMachineConfigSpec) types.BaseMethodFault {
	cpus, mem := vm.Config.Hardware.NumCPU, int64(vm.Config.Hardware.MemoryMB)
	newCPU, newMem := cpus, mem
	if spec.NumCPUs != 0 {
		newCPU = spec.NumCPUs
	}
	if spec.MemoryMB != 0 {
		newMem = spec.MemoryMB
	}
	if f := v.checkCapacity(newCPU, newMem); f != nil {
		return f
	}
	if vm.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOn {
		return nil
	}
	r := v.prof.VCRules.ReconfigurePoweredOn
	hotCPU := vm.Config.CpuHotAddEnabled != nil && *vm.Config.CpuHotAddEnabled
	hotMem := vm.Config.MemoryHotAddEnabled != nil && *vm.Config.MemoryHotAddEnabled
	violated := (newCPU > cpus && r.CPUIncreaseRequiresHotAdd && !hotCPU) ||
		(newCPU < cpus && r.CPUDecreaseRequiresPowerOff) ||
		(newMem > mem && r.MemIncreaseRequiresHotAdd && !hotMem) ||
		(newMem < mem && r.MemDecreaseRequiresPowerOff)
	if !violated {
		return nil
	}
	return &types.InvalidPowerState{
		InvalidState:   types.InvalidState{},
		RequestedState: types.VirtualMachinePowerStatePoweredOff,
		ExistingState:  types.VirtualMachinePowerStatePoweredOn,
	}
}

// failTask는 같은 Context로 실패하는 vcsim Task를 만들고 표지 Fault를 돌려준다.
func (v *VCenter) failTask(ctx *simulator.Context, this types.ManagedObjectReference, name string, fault types.BaseMethodFault) types.BaseMethodFault {
	obj := ctx.Map.Get(this)
	task := simulator.CreateTask(obj, name, func(*simulator.Task) (types.AnyType, types.BaseMethodFault) {
		return nil, fault
	})
	ref := task.Run(ctx)
	v.log.Info("adapter rejected request", "task", ref.Value, "target", this.Value, "operation", name, "fault", fmt.Sprintf("%T", fault))
	return &types.SystemError{Reason: adapterMarker + ref.Value}
}

// rewriteAdapterFault는 표지 Fault 응답을 정상 Task 응답으로 바꾼다. 해당 없으면 false.
func rewriteAdapterFault(w http.ResponseWriter, method string, body []byte) bool {
	m := markerRe.FindSubmatch(body)
	if m == nil {
		return false
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<soapenv:Envelope xmlns:soapenc="http://schemas.xmlsoap.org/soap/encoding/" xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`+
		`<soapenv:Body><%sResponse xmlns="urn:vim25"><returnval type="Task">%s</returnval></%sResponse></soapenv:Body></soapenv:Envelope>`,
		method, m[1], method)
	return true
}

// wsdlMethodKey는 요청 Context에 담는 SOAP 메서드 이름(예: CreateVM_Task)의 키다.
type wsdlMethodKey struct{}
