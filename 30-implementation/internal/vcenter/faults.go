package vcenter

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/fault"
)

// Phase 4a: 세션 유휴 만료(WP 4.1)와 vCenter 의미 계층 Fault Injection(WP 4.3·4.5).

func init() {
	baseFault := reflect.TypeOf((*types.BaseMethodFault)(nil)).Elem()
	fault.FaultTypeOK = func(name string) bool {
		t, ok := types.TypeFunc()(name)
		return ok && reflect.PointerTo(t).Implements(baseFault)
	}
}

// sessionTracker는 세션별 마지막 사용(가상 시각)과 강제 만료를 기록한다. 키: "soap:<세션 키>", "rest:<토큰>".
type sessionTracker struct {
	mu     sync.Mutex
	last   map[string]time.Time
	forced map[string]bool
}

func newSessionTracker() *sessionTracker {
	return &sessionTracker{last: map[string]time.Time{}, forced: map[string]bool{}}
}

// check는 만료면 true(기록 삭제), 아니면 마지막 사용을 갱신한다. timeout 0은 유휴 만료 없음.
func (t *sessionTracker) check(key string, now time.Time, timeout time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.forced[key] {
		delete(t.forced, key)
		delete(t.last, key)
		return true
	}
	if l, ok := t.last[key]; ok && timeout > 0 && now.Sub(l) > timeout {
		delete(t.last, key)
		return true
	}
	t.last[key] = now
	return false
}

func (t *sessionTracker) expire(key string) {
	t.mu.Lock()
	t.forced[key] = true
	t.mu.Unlock()
}

func (t *sessionTracker) forget(key string) {
	t.mu.Lock()
	delete(t.last, key)
	delete(t.forced, key)
	t.mu.Unlock()
}

// ExpireSession은 세션을 다음 요청에서 만료시킨다(세션 계층 Fault). kind: soap | rest.
func (v *VCenter) ExpireSession(kind, id string) { v.sess.expire(kind + ":" + id) }

// checkSOAPSession은 SOAP 세션의 유휴 만료·강제 만료를 판정하고, 만료면 vcsim 세션을 지운다(FR-API-0204, FR-BE-0607).
func (v *VCenter) checkSOAPSession(ctx *simulator.Context, method string) types.BaseMethodFault {
	s := ctx.Session
	if s == nil || s.UserName == "" { // 미인증 또는 내부 세션(Service.RoundTrip)
		return nil
	}
	key := "soap:" + s.Key
	if method == "Logout" {
		v.sess.forget(key)
		return nil
	}
	timeout := time.Duration(v.prof.VCRules.Session.IdleTimeoutSec) * time.Second
	if !v.sess.check(key, v.Now(), timeout) {
		return nil
	}
	sm := ctx.Map.SessionManager()
	ictx := &simulator.Context{Map: ctx.Map, Context: context.Background(), Session: &simulator.Session{}}
	ictx.WithLock(sm, func() { sm.TerminateSession(ictx, &types.TerminateSession{SessionId: []string{s.Key}}) })
	releaseSession(s.Key) // 만료 세션의 객체 락 항목 회수(viewlocks.go — 내부 호출이라 어댑터 훅을 거치지 않는다)
	v.log.Info("session expired", "user", s.UserName, "method", method)
	this := sm.Reference()
	return &types.NotAuthenticated{NoPermission: types.NoPermission{Object: &this, PrivilegeId: "System.View"}}
}

// restSessionOK는 /api 세션 토큰의 만료를 판정한다. 만료면 vAPI 세션 저장소에서도 지운다.
func (v *VCenter) restSessionOK(m *simulator.Model, id string) bool {
	if sessionInfo(m, id) == nil {
		return false
	}
	timeout := time.Duration(v.prof.VCRules.Session.RESTIdleTimeoutSec) * time.Second
	if !v.sess.check("rest:"+id, v.Now(), timeout) {
		return true
	}
	deleteRESTSession(m, id)
	return false
}

// applyFault는 vCenter 의미 계층 규칙을 적용한다. 대체 결과(표지 Fault 또는 SOAP Fault)가 있으면 돌려준다.
func (v *VCenter) applyFault(ctx *simulator.Context, method *simulator.Method) types.BaseMethodFault {
	if v.Faults == nil || ctx.Session == nil || ctx.Session.UserName == "" {
		return nil
	}
	rule, ok := v.Faults.Apply(fault.LayerVCenter, fault.Request{Product: "vcenter", SOAPMethod: method.Name,
		Resource: method.This.Value, User: ctx.Session.UserName, Now: v.Now()})
	if !ok {
		return nil
	}
	v.log.Warn("fault injected", "rule", rule.ID, "layer", rule.Layer, "action", rule.Action.Type, "method", method.Name,
		"target", method.This.Value, "user", ctx.Session.UserName, "fault", rule.Action.Fault, "delay_ms", rule.Action.DelayMS)
	isTask := strings.HasSuffix(method.Name, "_Task")
	switch rule.Action.Type {
	case fault.ActionSOAPFault:
		return newFault(rule.Action.Fault)
	case fault.ActionTaskFail:
		if !isTask {
			return newFault(rule.Action.Fault)
		}
		return v.failTaskAfter(ctx, method.This, taskName(method.Name), newFault(rule.Action.Fault), time.Duration(rule.Action.DelayMS)*time.Millisecond)
	case fault.ActionTaskDelay:
		if isTask {
			return v.delayedTask(ctx, method, time.Duration(rule.Action.DelayMS)*time.Millisecond)
		}
	}
	return nil
}

// newFault는 이름으로 vim25 Fault 값을 만든다(필드는 비움). Validate가 이름을 검사했다.
func newFault(name string) types.BaseMethodFault {
	t, _ := types.TypeFunc()(name)
	f, _ := reflect.New(t).Interface().(types.BaseMethodFault)
	if f == nil {
		return &types.SystemError{Reason: "fault " + name}
	}
	return f
}

// taskName은 vcsim 내부 Task 이름 관례(PowerOnVM_Task → powerOnVM)다. info.name은 WSDL 이름으로 보정된다.
func taskName(method string) string {
	n := strings.TrimSuffix(method, "_Task")
	return strings.ToLower(n[:1]) + n[1:]
}

// failTaskAfter는 실행 중(running)으로 delay(가상 시간)를 보낸 뒤 fault로 끝나는 Task를 만든다(TR-E2E-0901 ②).
func (v *VCenter) failTaskAfter(ctx *simulator.Context, this types.ManagedObjectReference, name string, f types.BaseMethodFault, delay time.Duration) types.BaseMethodFault {
	obj := ctx.Map.Get(this)
	task := simulator.CreateTask(obj, name, func(*simulator.Task) (types.AnyType, types.BaseMethodFault) {
		if delay > 0 {
			v.sleep(delay)
		}
		return nil, f
	})
	var ref types.ManagedObjectReference
	if delay > 0 {
		ref = runDetached(ctx, task, this)
	} else {
		ref = task.Run(ctx)
	}
	return &types.SystemError{Reason: adapterMarker + ref.Value}
}

// delayedTask는 Task 완료 지연(FR-BE-0605): 가상 시간 delay 뒤 원래 vcsim 메서드를 같은 Context로 실행하고
// 그 결과·오류로 끝나는 Task를 돌려준다. 안쪽 vcsim Task는 끝나면 레지스트리·recentTask에서 지운다(중복 노출 방지).
func (v *VCenter) delayedTask(ctx *simulator.Context, method *simulator.Method, delay time.Duration) types.BaseMethodFault {
	obj := ctx.Map.Get(method.This)
	if obj == nil {
		return nil
	}
	name := strings.TrimSuffix(method.Name, "_Task") + "Task"
	fn := reflect.ValueOf(obj).MethodByName(name)
	if !fn.IsValid() {
		return nil
	}
	task := simulator.CreateTask(obj, taskName(method.Name), func(t *simulator.Task) (types.AnyType, types.BaseMethodFault) {
		v.sleep(delay)
		var res soap.HasFault
		ctx.WithLock(obj, func() {
			res = fn.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(method.Body)})[0].Interface().(soap.HasFault)
		})
		if f := res.Fault(); f != nil {
			if mf, ok := f.Detail.Fault.(types.BaseMethodFault); ok {
				return nil, mf
			}
			return nil, &types.SystemError{Reason: f.String}
		}
		inner, ok := innerTask(res)
		if !ok {
			return nil, nil
		}
		it, ok := ctx.Map.Get(inner).(*simulator.Task)
		if !ok {
			return nil, nil
		}
		it.Wait()
		var info types.TaskInfo
		ctx.WithLock(it, func() { info = it.Info })
		if v.tasks != nil {
			ent := types.ManagedObjectReference{}
			if info.Entity != nil {
				ent = *info.Entity
			}
			v.tasks.purge(inner, ent)
		}
		if info.Error != nil {
			return nil, info.Error.Fault
		}
		return info.Result, nil
	})
	ref := runDetached(ctx, task, method.This)
	return &types.SystemError{Reason: adapterMarker + ref.Value}
}

// innerTask는 *_TaskBody 응답에서 Task MoRef를 꺼낸다.
func innerTask(res soap.HasFault) (types.ManagedObjectReference, bool) {
	rv := reflect.ValueOf(res).Elem().FieldByName("Res")
	if !rv.IsValid() || rv.IsNil() {
		return types.ManagedObjectReference{}, false
	}
	ret := rv.Elem().FieldByName("Returnval")
	if r, ok := ret.Interface().(types.ManagedObjectReference); ok {
		return r, true
	}
	return types.ManagedObjectReference{}, false
}

// sleep은 가상 시간으로 기다린다(시계 없으면 실제 시간).
func (v *VCenter) sleep(d time.Duration) {
	if v.Clock != nil {
		v.Clock.Sleep(d, nil)
		return
	}
	time.Sleep(d)
}

// checkDatastoreSpace: 여유 공간 0인 데이터스토어에 VM을 만들면 NoDiskSpace(Datastore 용량 부족 Fault, FR-FLT-0112).
func (v *VCenter) checkDatastoreSpace(ctx *simulator.Context, path string) types.BaseMethodFault {
	name, _, _ := strings.Cut(strings.TrimPrefix(path, "["), "]")
	for _, obj := range ctx.Map.AllReference("Datastore") {
		ds, ok := obj.(*simulator.Datastore)
		if !ok {
			continue
		}
		var full bool
		ctx.WithLock(ds, func() { full = ds.Name == name && ds.Summary.FreeSpace <= 0 })
		if full {
			return &types.NoDiskSpace{FileFault: types.FileFault{File: path}, Datastore: name}
		}
	}
	return nil
}

// ApplyStateFaults는 인프라 상태 규칙의 추가·제거를 반영한다(FR-FLT-0110·0111·0112). 규칙이 있는 동안 상태를 유지하고
// 제거하면 되돌린다. 같은 프로세스 클라이언트(Service.RoundTrip)로 vcsim 메서드를 호출하거나 데이터스토어 값을 바꾼다.
func (v *VCenter) ApplyStateFaults(added, removed []fault.Rule) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := v.InternalClient(ctx)
	if err != nil {
		return
	}
	m := v.Model()
	wait := func(t *object.Task, err error) {
		if err == nil {
			_ = t.Wait(ctx)
		}
		if err != nil {
			v.log.Warn("state fault apply failed", "err", err)
		}
	}
	for i, list := range [][]fault.Rule{removed, added} {
		on := i == 1
		for _, r := range list {
			if r.Layer != fault.LayerVCenter || !fault.StateActions[r.Action.Type] {
				continue
			}
			switch r.Action.Type {
			case fault.ActionHostDisconnect:
				h := object.NewHostSystem(c, types.ManagedObjectReference{Type: "HostSystem", Value: r.Match.Resource})
				if on {
					wait(h.Disconnect(ctx))
				} else {
					wait(h.Reconnect(ctx, nil, nil))
				}
			case fault.ActionHostMaintenance:
				h := object.NewHostSystem(c, types.ManagedObjectReference{Type: "HostSystem", Value: r.Match.Resource})
				if on {
					wait(h.EnterMaintenanceMode(ctx, 0, false, nil))
				} else {
					wait(h.ExitMaintenanceMode(ctx, 0))
				}
			case fault.ActionDatastoreFull:
				v.setDatastoreFull(m, r.Match.Resource, on)
			}
			v.log.Warn("fault state applied", "rule", r.ID, "action", r.Action.Type, "resource", r.Match.Resource, "on", on)
		}
	}
}

func (v *VCenter) setDatastoreFull(m *simulator.Model, ref string, full bool) {
	ds, ok := m.Map().Get(types.ManagedObjectReference{Type: "Datastore", Value: ref}).(*simulator.Datastore)
	if !ok {
		return
	}
	ctx := &simulator.Context{Map: m.Map(), Context: context.Background(), Session: &simulator.Session{}}
	ctx.WithLock(ds, func() {
		v.dsMu.Lock()
		defer v.dsMu.Unlock()
		if full {
			if _, saved := v.dsFree[ref]; !saved {
				v.dsFree[ref] = ds.Summary.FreeSpace
			}
			ds.Summary.FreeSpace = 0
		} else if orig, saved := v.dsFree[ref]; saved {
			ds.Summary.FreeSpace = orig
			delete(v.dsFree, ref)
		}
		if info, ok := ds.Info.(*types.VmfsDatastoreInfo); ok {
			info.FreeSpace = ds.Summary.FreeSpace
		} else if info, ok := ds.Info.(*types.LocalDatastoreInfo); ok {
			info.FreeSpace = ds.Summary.FreeSpace
		}
		ctx.Map.Update(ctx, ds, []types.PropertyChange{{Name: "summary.freeSpace", Val: ds.Summary.FreeSpace}})
	})
	v.evaluateDatastore(m, ds) // 사용률 임계 알람(FR-MET-0201)
}

var _ = fmt.Sprint
var _ mo.Reference

var lockSeq atomic.Int64

// runDetached는 Task를 실행하되 vcsim이 실행 동안 잡는 엔터티 락(lock handoff)을 전용 락 객체로 돌린다.
// 지연·대기하는 Task가 대상 엔터티 락을 쥔 채 기다리면 대상 조회와 안쪽 Task가 막힌다(교착). 실행을 시작한 뒤
// 클라이언트에 보이는 info.entity를 원래 엔터티로 되돌린다.
func runDetached(ctx *simulator.Context, t *simulator.Task, entity types.ManagedObjectReference) types.ManagedObjectReference {
	lock := types.ManagedObjectReference{Type: "CloudinfraSimTaskLock", Value: fmt.Sprintf("lock-%d", lockSeq.Add(1))}
	t.Info.Entity = &lock
	ref := t.Run(ctx)
	e := entity
	ctx.Map.AtomicUpdate(ctx, t, []types.PropertyChange{{Name: "info.entity", Val: e}})
	return ref
}
