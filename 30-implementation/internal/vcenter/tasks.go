package vcenter

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/store"
)

// taskObserver는 vcsim Task의 생성·상태 변경을 내부 작업 모델로 기록한다(WP 2.7, FR-BE-0501 ~ 0503, NFR-0402·0403).
//
// 락 규칙(R-15, ARCHITECTURE §4): vcsim 변경 핸들러는 요청 고루틴 안에서 객체 락을 쥔 채 동기로 호출되므로,
// 여기서는 값만 복사해 채널(Outbox)에 넣고 어떤 락도 잡지 않는다. 기록·로그·보존 정리는 소비자 고루틴이 한다.
type taskObserver struct {
	model   *simulator.Model
	db      *store.Store
	log     *slog.Logger
	keep    int // 보존할 완료 Task 객체 수(RESOURCE_BUDGET §4 NFR-0505: lite 200)
	events  chan taskEvent
	dropped atomic.Int64
	done    chan struct{}
	wg      sync.WaitGroup
}

type taskEvent struct {
	ref, entityRef      types.ManagedObjectReference
	entity, name, descr string
	state               types.TaskInfoState
	progress            int32
	queued, start, end  time.Time
	errMsg              string
}

func newTaskObserver(m *simulator.Model, db *store.Store, log *slog.Logger, keep int) *taskObserver {
	o := &taskObserver{model: m, db: db, log: log, keep: keep, events: make(chan taskEvent, 4096), done: make(chan struct{})}
	m.Map().AddHandler(o)
	o.wg.Add(1)
	go o.consume()
	return o
}

func (o *taskObserver) Reference() types.ManagedObjectReference {
	return types.ManagedObjectReference{Type: "CloudinfraSimTaskObserver", Value: "task-observer"}
}

// taskOf는 변경 핸들러가 받는 객체에서 mo.Task를 꺼낸다. vcsim은 PutObject에는 *simulator.Task를,
// Update(AtomicUpdate)에는 내장된 *mo.Task를 넘긴다.
func taskOf(obj mo.Reference) *mo.Task {
	switch x := obj.(type) {
	case *simulator.Task:
		return &x.Task
	case *mo.Task:
		return x
	}
	return nil
}

func (o *taskObserver) snapshot(t *mo.Task) {
	ev := taskEvent{ref: t.Self, name: t.Info.Name, descr: t.Info.DescriptionId, state: t.Info.State, queued: t.Info.QueueTime}
	if t.Info.Entity != nil {
		ev.entity, ev.entityRef = t.Info.Entity.Value, *t.Info.Entity
	}
	ev.progress = t.Info.Progress
	if t.Info.StartTime != nil {
		ev.start = *t.Info.StartTime
	}
	if t.Info.CompleteTime != nil {
		ev.end = *t.Info.CompleteTime
	}
	if t.Info.Error != nil {
		ev.errMsg = t.Info.Error.LocalizedMessage
		if ev.errMsg == "" && t.Info.Error.Fault != nil {
			ev.errMsg = typeName(t.Info.Error.Fault)
		}
	}
	select {
	case o.events <- ev:
	default:
		o.dropped.Add(1) // 소비자가 밀리면 기록을 버린다(요청 경로를 막지 않음)
	}
}

func typeName(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimPrefix(fmt.Sprintf("%T", v), "*types.")
}

// RegisterObject 구현(vcsim 변경 핸들러)
func (o *taskObserver) PutObject(_ *simulator.Context, obj mo.Reference) {
	if vm, ok := obj.(*simulator.VirtualMachine); ok {
		fixVMRuntime(vm) // 새로 등록되는 VM(CreateVM 등): 클라이언트에 보이기 전 등록 시점에 보정(R-27)
	}
	if c, ok := obj.(*simulator.ClusterComputeResource); ok {
		fixClusterConfig(c) // CreateClusterEx로 만든 클러스터
	}
	if t := taskOf(obj); t != nil {
		o.snapshot(t)
	}
}

func (o *taskObserver) UpdateObject(ctx *simulator.Context, obj mo.Reference, _ []types.PropertyChange) {
	if t := taskOf(obj); t != nil {
		fixTaskName(ctx, t)
		o.snapshot(t)
	}
}

// fixTaskName: vcsim은 info.name을 내부 이름(예: CreateVm)으로 채우지만 형식은 vmodl.MethodName이므로 요청의
// WSDL 메서드 이름(예: CreateVM_Task)으로 고친다(pyVmomi는 이 값을 메서드로 해석, R-26). vcsim은 Task 등록(Put)에는
// 새 Context를 넘기고, 첫 상태 변경(running)은 실행 고루틴 시작 전 요청 고루틴에서 요청 Context로 동기 호출한다.
func fixTaskName(ctx *simulator.Context, t *mo.Task) {
	if ctx == nil || ctx.Context == nil {
		return
	}
	// info.reason: 요청한 세션 사용자(vcsim은 "vcsim"으로 고정, FR-MET-0203 Initiator·FR-BE-0209)
	if ctx.Session != nil && ctx.Session.UserName != "" {
		if r, ok := t.Info.Reason.(*types.TaskReasonUser); !ok || r.UserName != ctx.Session.UserName {
			t.Info.Reason = &types.TaskReasonUser{UserName: ctx.Session.UserName}
		}
	}
	if name, ok := ctx.Context.Value(wsdlMethodKey{}).(string); ok && t.Info.Name != name {
		t.Info.Name = name
	}
	fixEntityName(ctx, t)
}

// fixEntityName: vcsim은 info.entityName을 엔터티 이름 대신 MoRef 값(예: vm-60)으로 채운다(R-32). API 문서상 이 속성은
// 엔터티의 이름이므로, MoRef 값과 같을 때만 엔터티의 현재 이름으로 고친다(이미 이름이면 그대로).
func fixEntityName(ctx *simulator.Context, t *mo.Task) {
	if t.Info.Entity == nil || t.Info.EntityName != t.Info.Entity.Value || ctx.Map == nil {
		return
	}
	if e, ok := ctx.Map.Get(*t.Info.Entity).(mo.Entity); ok {
		if name := e.Entity().Name; name != "" {
			t.Info.EntityName = name
		}
	}
}

func (o *taskObserver) RemoveObject(*simulator.Context, types.ManagedObjectReference) {}

var stateName = map[types.TaskInfoState]string{
	types.TaskInfoStateQueued: "QUEUED", types.TaskInfoStateRunning: "RUNNING",
	types.TaskInfoStateSuccess: "SUCCESS", types.TaskInfoStateError: "ERROR",
}

func (o *taskObserver) consume() {
	defer o.wg.Done()
	last := map[string]string{}
	var completed []taskEvent
	ctx := context.Background()
	for {
		select {
		case <-o.done:
			return
		case ev := <-o.events:
			state := stateName[ev.state]
			from := last[ev.ref.Value]
			if from == state {
				continue
			}
			last[ev.ref.Value] = state
			op := store.Operation{ID: ev.ref.Value, Target: ev.entity, Type: ev.descr, State: state, Progress: int(ev.progress),
				StartedAt: ev.start, CompletedAt: ev.end, Error: ev.errMsg}
			if err := o.db.RecordTransition(ctx, op, from); err != nil {
				o.log.Warn("operation record failed", "task", ev.ref.Value, "err", err)
			}
			// NFR-0403 상태 전이 로그
			o.log.Debug("task transition", "task", ev.ref.Value, "resource", ev.entity, "operation", ev.descr, "from", from, "to", state)
			if state == "SUCCESS" || state == "ERROR" {
				// NFR-0402 작업 추적: Task ID, Resource ID, 소요 시간
				d := time.Duration(0)
				if !ev.start.IsZero() && !ev.end.IsZero() {
					d = ev.end.Sub(ev.start)
				}
				o.log.Info("task completed", "task", ev.ref.Value, "resource", ev.entity, "operation", ev.descr,
					"state", state, "duration_ms", d.Milliseconds(), "error", ev.errMsg)
				delete(last, ev.ref.Value)
				completed = append(completed, ev)
				o.postEvents(ev, state)
				if state == "SUCCESS" && ev.descr == "VirtualMachine.reconfigVm" && ev.entity != "" {
					o.publishReconfigure(ev.entity)
				}
				// NFR-0505: 완료된 Task 객체를 보존 한도까지만 레지스트리에 남긴다(vcsim은 지우지 않음, R-22).
				for len(completed) > o.keep {
					old := completed[0]
					completed = completed[1:]
					o.purge(old.ref, old.entityRef)
				}
			}
		}
	}
}

// purge는 Task 객체를 레지스트리에서 지우고 TaskManager·대상 엔터티의 recentTask에서도 뺀다.
// 지우지 않으면 recentTask가 조회할 수 없는 Task를 가리킨다(실제 vCenter에는 없는 상태).
func (o *taskObserver) purge(task, entity types.ManagedObjectReference) {
	ctx := o.model.Service.Context
	m := o.model.Map()
	drop := func(obj mo.Reference, recent *[]types.ManagedObjectReference) {
		ctx.WithLock(obj, func() {
			if i := slices.Index(*recent, task); i >= 0 {
				next := slices.Delete(slices.Clone(*recent), i, i+1)
				ctx.Update(obj, []types.PropertyChange{{Name: "recentTask", Val: next}})
			}
		})
	}
	if si, ok := m.Get(vim25.ServiceInstance).(*simulator.ServiceInstance); ok && si.Content.TaskManager != nil {
		tm, _ := m.Get(*si.Content.TaskManager).(*simulator.TaskManager)
		if tm != nil {
			drop(tm, &tm.RecentTask)
		}
	}
	if e, ok := m.Get(entity).(mo.Entity); ok && entity.Value != "" {
		drop(e, &e.Entity().RecentTask)
	}
	m.Remove(ctx, task)
}

func (o *taskObserver) Close() {
	o.model.Map().RemoveHandler(o)
	close(o.done)
	o.wg.Wait()
}

// publishReconfigure는 vcsim이 PropertyCollector에 알리지 않는 재구성 결과를 다시 알린다(R-23).
// vcsim의 VirtualMachine.apply는 CPU·메모리·Hot-add 필드를 ctx.Update 없이 직접 바꾸므로 WaitForUpdatesEx가
// 변경을 받지 못한다. 요청 경로 밖(소비자 고루틴)에서 VM 객체 락만 잡고 현재 값을 같은 값으로 다시 할당해 통지한다.
func (o *taskObserver) publishReconfigure(vmID string) {
	ref := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmID}
	obj, ok := o.model.Map().Get(ref).(*simulator.VirtualMachine)
	if !ok {
		return
	}
	ctx := o.model.Service.Context
	ctx.WithLock(obj, func() {
		c := obj.Config
		if c == nil {
			return
		}
		changes := []types.PropertyChange{
			{Name: "config.hardware.numCPU", Val: c.Hardware.NumCPU},
			{Name: "config.hardware.memoryMB", Val: c.Hardware.MemoryMB},
			{Name: "summary.config.numCpu", Val: obj.Summary.Config.NumCpu},
			{Name: "summary.config.memorySizeMB", Val: obj.Summary.Config.MemorySizeMB},
		}
		if c.Hardware.NumCoresPerSocket != nil {
			changes = append(changes, types.PropertyChange{Name: "config.hardware.numCoresPerSocket", Val: *c.Hardware.NumCoresPerSocket})
		}
		for name, p := range map[string]*bool{"config.cpuHotAddEnabled": c.CpuHotAddEnabled, "config.cpuHotRemoveEnabled": c.CpuHotRemoveEnabled, "config.memoryHotAddEnabled": c.MemoryHotAddEnabled} {
			if p != nil {
				changes = append(changes, types.PropertyChange{Name: name, Val: *p})
			}
		}
		ctx.Update(obj, changes)
	})
}

// postEvents는 vcsim이 만들지 않는 이벤트를 추가한다(FR-MET-0201·0202, WP 3.5):
//   - Task 실패 → TaskEvent(info.state=error, 이벤트 종류 'Task Failed')
//   - 호스트 연결 끊기·다시 연결 Task 성공 → HostDisconnectedEvent·HostConnectedEvent
//
// 소비자 고루틴에서 같은 프로세스의 EventManager.PostEvent(Service.RoundTrip, 내부 세션)로 게시한다.
func (o *taskObserver) postEvents(ev taskEvent, state string) {
	var post types.BaseEvent
	m := o.model.Map()
	switch {
	case state == "ERROR":
		t, ok := m.Get(ev.ref).(*simulator.Task)
		if !ok {
			return
		}
		var info types.TaskInfo
		ctx := o.model.Service.Context
		ctx.WithLock(t, func() { info = t.Info })
		te := &types.TaskEvent{Info: info}
		if ev.entityRef.Type == "VirtualMachine" {
			if vm, ok := m.Get(ev.entityRef).(*simulator.VirtualMachine); ok {
				te.Vm = &types.VmEventArgument{EntityEventArgument: types.EntityEventArgument{Name: vm.Name}, Vm: ev.entityRef}
			}
		}
		te.FullFormattedMessage = fmt.Sprintf("Task: %s failed: %s", info.Name, ev.errMsg)
		post = te
	case state == "SUCCESS" && (ev.descr == "HostSystem.disconnectHost" || ev.descr == "HostSystem.reconnectHost"):
		h, ok := m.Get(ev.entityRef).(*simulator.HostSystem)
		if !ok {
			return
		}
		arg := &types.HostEventArgument{EntityEventArgument: types.EntityEventArgument{Name: h.Name}, Host: ev.entityRef}
		if ev.descr == "HostSystem.disconnectHost" {
			post = &types.HostDisconnectedEvent{HostEvent: types.HostEvent{Event: types.Event{Host: arg}}, Reason: string(types.HostDisconnectedEventReasonCodeUserRequest)}
			post.GetEvent().FullFormattedMessage = "Host " + h.Name + " is disconnected"
		} else {
			post = &types.HostConnectedEvent{HostEvent: types.HostEvent{Event: types.Event{Host: arg}}}
			post.GetEvent().FullFormattedMessage = "Connected to " + h.Name
		}
	default:
		return
	}
	// 게시자: Task를 요청한 사용자(info.reason, FR-MET-0203 Initiator). 요청 사용자 세션 Context로 EventManager에 직접 게시한다.
	user := ""
	if t, ok := m.Get(ev.ref).(*simulator.Task); ok {
		o.model.Service.Context.WithLock(t, func() {
			if r, ok := t.Info.Reason.(*types.TaskReasonUser); ok {
				user = r.UserName
			}
		})
	}
	em := m.EventManager()
	ctx := &simulator.Context{Map: m, Context: context.Background(), Session: &simulator.Session{UserSession: types.UserSession{UserName: user}, Map: m}}
	ctx.WithLock(em, func() { em.PostEvent(ctx, &types.PostEvent{This: em.Self, EventToPost: post}) })
}
