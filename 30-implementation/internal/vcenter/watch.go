package vcenter

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// 교차 시스템 관계(WP 3.1 ~ 3.3)가 쓰는 vCenter 쪽 진입점.
//
// InternalClient는 vcsim Service.RoundTrip으로 같은 프로세스 안에서 SOAP 메서드를 호출하는 govmomi 클라이언트다.
// 네트워크·외부 프로세스를 거치지 않고(FR-BE-0406) 내부 세션으로 실행되며, 요청은 Registry.Handler(Adapter 규칙,
// Task info.name 보정)를 그대로 거친다. Gateway를 거치지 않으므로 버전 필터·호출 기록은 적용되지 않는다.
func (v *VCenter) InternalClient(ctx context.Context) (*vim25.Client, error) {
	m := v.Model()
	if m == nil {
		return nil, fmt.Errorf("vCenter model closed")
	}
	return vim25.NewClient(ctx, m.Service)
}

// OnChange는 VM·포트그룹·호스트·VDS 변경 때 바뀐 객체의 MoRef로 부를 함수를 등록한다. vcsim 변경 핸들러에서
// 동기로 불리므로 fn은 다른 락을 기다리거나 막히면 안 된다(표시·신호만, R-15). Reset 뒤에도 유지된다.
// 관계 엔진은 MoRef로 증분 조정 대상(변경 VM)을 고른다(R-45).
func (v *VCenter) OnChange(fn func(types.ManagedObjectReference)) {
	v.mu.Lock()
	v.onChange = append(v.onChange, fn)
	m := v.model
	v.mu.Unlock()
	if m != nil {
		m.Map().AddHandler(&changeWatcher{fns: []func(types.ManagedObjectReference){fn}})
	}
}

// changeWatcher는 관계 대상 객체의 변경을 알린다(Outbox 신호).
type changeWatcher struct {
	fns []func(types.ManagedObjectReference)
}

func (w *changeWatcher) Reference() types.ManagedObjectReference {
	return types.ManagedObjectReference{Type: "CloudinfraSimChangeWatcher", Value: "change-watcher"}
}

func relevant(ref types.ManagedObjectReference) bool {
	switch ref.Type {
	case "VirtualMachine", "DistributedVirtualPortgroup", "HostSystem", "VmwareDistributedVirtualSwitch":
		return true
	}
	return false
}

func (w *changeWatcher) notify(ref types.ManagedObjectReference) {
	if relevant(ref) {
		for _, f := range w.fns {
			f(ref)
		}
	}
}

func (w *changeWatcher) PutObject(_ *simulator.Context, obj mo.Reference) { w.notify(obj.Reference()) }
func (w *changeWatcher) UpdateObject(_ *simulator.Context, obj mo.Reference, _ []types.PropertyChange) {
	w.notify(obj.Reference())
}
func (w *changeWatcher) RemoveObject(_ *simulator.Context, ref types.ManagedObjectReference) {
	w.notify(ref)
}
