package vcenter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vmware/govmomi/alarm"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
)

// 기본 알람 'Datastore usage on disk'(FR-MET-0201 Datastore Usage Threshold, FR-MET-0202 계약 기준).
// 알람 정의는 실제 vCenter 기본 알람처럼 메트릭 식(데이터스토어 사용률 75% 경고·85% 위험)으로 만들고, 평가는
// 시뮬레이터가 한다(vcsim 알람 엔진은 EventEx 식만 평가). 상태가 바뀌면 데이터스토어 triggeredAlarmState·
// overallStatus를 고치고 AlarmStatusChangedEvent를 남긴다. 임계값·카운터 선택은 가정이다.

const dsUsageAlarm = "Datastore usage on disk"

type dsAlarm struct {
	mu     sync.Mutex
	ref    *types.ManagedObjectReference
	status map[string]types.ManagedEntityStatus // datastore MoRef → 현재 상태
}

func (v *VCenter) installDatastoreAlarm(m *simulator.Model) {
	v.dsAlarm = &dsAlarm{status: map[string]types.ManagedEntityStatus{}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := vim25.NewClient(ctx, m.Service) // Reset 중: 새 모델(아직 v.model로 바꾸기 전)
	if err != nil {
		return
	}
	am, err := alarm.GetManager(c)
	if err != nil {
		return
	}
	counter := int32(0)
	if pm, ok := m.Map().Get(*m.ServiceContent.PerfManager).(*perfManager); ok {
		for _, pc := range pm.PerfCounter {
			if counterName(pc) == "disk.used.latest" {
				counter = pc.Key
			}
		}
	}
	spec := &types.AlarmSpec{
		Name:        dsUsageAlarm,
		Description: "Default alarm to monitor datastore disk usage",
		Enabled:     true,
		Expression: &types.OrAlarmExpression{Expression: []types.BaseAlarmExpression{&types.MetricAlarmExpression{
			Operator: types.MetricAlarmOperatorIsAbove, Type: "Datastore",
			Metric: types.PerfMetricId{CounterId: counter}, Yellow: 7500, Red: 8500,
		}}},
	}
	// Snapshot에서 적재한 모델에는 알람이 이미 있다(snapshot.go): 같은 이름을 다시 만들지 않는다.
	if existing, err := am.GetAlarm(ctx, nil); err == nil {
		for _, a := range existing {
			if a.Info.Name == dsUsageAlarm {
				ref := a.Self
				v.dsAlarm.ref = &ref
				return
			}
		}
	}
	ref, err := am.CreateAlarm(ctx, object.NewFolder(c, c.ServiceContent.RootFolder), spec)
	if err != nil {
		v.log.Warn("datastore alarm create failed", "err", err)
		return
	}
	v.dsAlarm.ref = ref
}

// evaluateDatastore는 사용률로 알람 상태를 다시 계산하고 바뀌었으면 반영한다.
func (v *VCenter) evaluateDatastore(m *simulator.Model, ds *simulator.Datastore) {
	a := v.dsAlarm
	if a == nil || a.ref == nil {
		return
	}
	ctx := &simulator.Context{Map: m.Map(), Context: context.Background(), Session: &simulator.Session{}}
	var capacity, free int64
	var name string
	ctx.WithLock(ds, func() { capacity, free, name = ds.Summary.Capacity, ds.Summary.FreeSpace, ds.Name })
	if capacity <= 0 {
		return
	}
	usage := (capacity - free) * 10000 / capacity
	to := types.ManagedEntityStatusGreen
	switch {
	case usage >= 8500:
		to = types.ManagedEntityStatusRed
	case usage >= 7500:
		to = types.ManagedEntityStatusYellow
	}
	a.mu.Lock()
	from, ok := a.status[ds.Self.Value]
	if !ok {
		from = types.ManagedEntityStatusGreen
	}
	a.status[ds.Self.Value] = to
	a.mu.Unlock()
	if from == to {
		return
	}
	key := fmt.Sprintf("%s.%s", a.ref.Value, ds.Self.Value)
	ctx.WithLock(ds, func() {
		states := ds.TriggeredAlarmState[:0:0]
		for _, s := range ds.TriggeredAlarmState {
			if s.Key != key {
				states = append(states, s)
			}
		}
		if to != types.ManagedEntityStatusGreen {
			states = append(states, types.AlarmState{Key: key, Entity: ds.Self, Alarm: *a.ref, OverallStatus: to,
				Time: time.Now(), Acknowledged: types.NewBool(false)})
		}
		ctx.Map.Update(ctx, ds, []types.PropertyChange{
			{Name: "triggeredAlarmState", Val: states}, {Name: "overallStatus", Val: to},
		})
	})
	ev := &types.AlarmStatusChangedEvent{
		AlarmEvent: types.AlarmEvent{Alarm: types.AlarmEventArgument{EntityEventArgument: types.EntityEventArgument{Name: dsUsageAlarm}, Alarm: *a.ref}},
		Source:     types.ManagedEntityEventArgument{EntityEventArgument: types.EntityEventArgument{Name: "Datacenters"}, Entity: m.ServiceContent.RootFolder},
		Entity:     types.ManagedEntityEventArgument{EntityEventArgument: types.EntityEventArgument{Name: name}, Entity: ds.Self},
		From:       string(from), To: string(to),
	}
	ev.FullFormattedMessage = fmt.Sprintf("Alarm '%s' on %s changed from %s to %s", dsUsageAlarm, name, from, to)
	em := m.Map().EventManager()
	ctx.WithLock(em, func() { em.PostEvent(ctx, &types.PostEvent{This: em.Self, EventToPost: ev}) })
	v.log.Info("datastore usage alarm", "datastore", name, "usage_pct", float64(usage)/100, "from", from, "to", to)
}
