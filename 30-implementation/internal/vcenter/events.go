package vcenter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unsafe"

	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"

	"cloudinfra-sim/internal/store"
)

// vCenter 이벤트 보관(R-44, 2026-10-11). vcsim EventManager는 최근 1,000건(maxPageSize)만 메모리에 둔다.
//  ① 보관: SyncEvents가 vcsim 이벤트를 시뮬레이터 저장소(vcenter_events)로 복제한다(실행 프로필 한도 lite 1,000건,
//     full 100,000건·7일). 이벤트 키가 바뀌었을 때만 조회한다. 호출자(sim)는 교차 락을 쥔다 — Reset·복원과 겹치지 않는다.
//  ② 저장·재현: Snapshot·Fixture 캐시는 이벤트 기록(최근 1,000건)을 events.xml로 저장하고, 적재할 때 같은 순서·같은
//     키로 다시 게시한다(vcsim Model.Load는 이벤트를 복원하지 않는다). 생성 시각은 적재 시각(Create와 같은 '지금').
// 한계: SOAP 클라이언트의 QueryEvents·EventHistoryCollector는 여전히 vcsim의 최근 1,000건을 본다(실제 vCenter도 페이지당
// 최대 1,000건). 두 동기화 사이에 1,000건 넘게 생기면 그 사이 일부를 놓친다(Dropped로 센다).

const eventsFile = "events.xml"

// recordOf는 vcsim 이벤트를 보관·조회 형식으로 바꾼다.
func (v *VCenter) recordOf(ev types.BaseEvent) EventRecord {
	e := ev.GetEvent()
	r := EventRecord{Key: e.Key, Type: strings.TrimPrefix(fmt.Sprintf("%T", ev), "*types."), Created: v.toVirtual(e.CreatedTime),
		User: e.UserName, Message: e.FullFormattedMessage}
	if r.Message == "" { // 사용자 이벤트(GeneralUserEvent 등)는 서식 문장 없이 message에만 내용이 있다
		if g, ok := ev.(types.BaseGeneralEvent); ok {
			r.Message = g.GetGeneralEvent().Message
		}
	}
	switch {
	case e.Vm != nil:
		r.Entity, r.Name = e.Vm.Vm.Value, e.Vm.Name
	case e.Host != nil:
		r.Entity, r.Name = e.Host.Host.Value, e.Host.Name
	case e.Ds != nil:
		r.Entity, r.Name = e.Ds.Datastore.Value, e.Ds.Name
	}
	if x, ok := ev.(*types.AlarmStatusChangedEvent); ok && r.Entity == "" {
		r.Entity, r.Name = x.Entity.Entity.Value, x.Entity.Name
	}
	return r
}

// latestEvents는 vcsim 이벤트 기록(오래된 순 = 키 오름차순, 최대 1,000건)이다. QueryEvents는 최근 것을 먼저 준다.
func latestEvents(m *simulator.Model) ([]types.BaseEvent, error) {
	ctx := context.WithValue(context.Background(), internalCall{}, true)
	c, err := vim25.NewClient(ctx, m.Service)
	if err != nil {
		return nil, err
	}
	evs, err := event.NewManager(c).QueryEvents(ctx, types.EventFilterSpec{MaxCount: 1000})
	if err != nil {
		return nil, err
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].GetEvent().Key < evs[j].GetEvent().Key })
	return evs, nil
}

// eventKeyField는 vcsim EventManager의 마지막 이벤트 키(비공개 필드 key)다. 공개 API가 없어 reflect로 접근한다(govmomi v0.56.0).
func eventKeyField(em *simulator.EventManager) *int32 {
	f := reflect.ValueOf(em).Elem().FieldByName("key")
	if !f.IsValid() || f.Kind() != reflect.Int32 || !f.CanAddr() {
		return nil
	}
	return (*int32)(unsafe.Pointer(f.UnsafeAddr()))
}

func lastEventKey(m *simulator.Model) int32 {
	if p := eventKeyField(m.Map().EventManager()); p != nil {
		return *p
	}
	return -1 // 알 수 없음: 매번 조회
}

// SyncEvents는 아직 보관하지 않은 vcsim 이벤트를 저장소에 넣는다. 호출자는 교차 락을 쥐어야 한다.
func (v *VCenter) SyncEvents(ctx context.Context) error {
	m := v.Model()
	if m == nil || v.db == nil {
		return nil
	}
	v.evMu.Lock()
	defer v.evMu.Unlock()
	epoch := v.epoch.Load()
	if epoch != v.evEpoch {
		v.evEpoch, v.evLast = epoch, 0
	}
	if k := lastEventKey(m); k >= 0 && k <= v.evLast {
		return nil
	}
	evs, err := latestEvents(m)
	if err != nil {
		return err
	}
	var recs []store.VCEvent
	last := v.evLast
	for _, ev := range evs {
		e := ev.GetEvent()
		if e.Key <= v.evLast {
			continue
		}
		if len(recs) == 0 && v.evLast > 0 && e.Key > v.evLast+1 {
			v.evDropped += int64(e.Key - v.evLast - 1) // 동기화 사이에 링버퍼를 넘었다
		}
		r := v.recordOf(ev)
		recs = append(recs, store.VCEvent{Epoch: epoch, Key: e.Key, At: r.Created, Type: r.Type, User: r.User, Entity: r.Entity, EntityName: r.Name, Message: r.Message})
		last = max(last, e.Key)
	}
	if err := v.db.RecordVCEvents(ctx, recs); err != nil {
		return err
	}
	v.evLast = last
	return nil
}

// ResetEventArchive는 보관 이벤트를 지우고 현재 모델의 이벤트를 처음부터 다시 보관하게 한다(환경 초기화).
func (v *VCenter) ResetEventArchive(ctx context.Context) error {
	v.evMu.Lock()
	defer v.evMu.Unlock()
	v.evEpoch, v.evLast, v.evDropped = v.epoch.Load(), 0, 0
	if v.db == nil {
		return nil
	}
	return v.db.ClearVCEvents(ctx)
}

// MarkEventsArchived는 현재 모델의 이벤트를 이미 보관한 것으로 둔다(Snapshot 복원: 복원한 저장소에 Snapshot 시점까지의
// 보관 이벤트가 있고, 모델은 같은 이벤트를 다시 게시해 둔 상태다).
func (v *VCenter) MarkEventsArchived() {
	v.evMu.Lock()
	defer v.evMu.Unlock()
	v.evEpoch, v.evLast = v.epoch.Load(), max(lastEventKey(v.Model()), 0)
}

// EventsDropped는 동기화 사이에 링버퍼를 넘어 보관하지 못한 이벤트 수다.
func (v *VCenter) EventsDropped() int64 {
	v.evMu.Lock()
	defer v.evMu.Unlock()
	return v.evDropped
}

// exportEvents는 이벤트 기록을 dir/events.xml로 저장한다(Snapshot·Fixture 캐시).
func exportEvents(m *simulator.Model, dir string) error {
	evs, err := latestEvents(m)
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, eventsFile))
	if err != nil {
		return err
	}
	if err := xml.NewEncoder(f).Encode(types.ArrayOfEvent{Event: evs}); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// importEvents는 저장한 이벤트를 같은 순서·같은 키로 다시 게시한다(활성화 전 모델). 파일이 없으면(이전 저장본) 건너뛴다.
func importEvents(m *simulator.Model, dir string) error {
	f, err := os.Open(filepath.Join(dir, eventsFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	d.TypeFunc = types.TypeFunc()
	var arr types.ArrayOfEvent
	if err := d.Decode(&arr); err != nil {
		return fmt.Errorf("이벤트 기록: %w", err)
	}
	if len(arr.Event) == 0 {
		return nil
	}
	em := m.Map().EventManager()
	if p := eventKeyField(em); p != nil {
		*p = arr.Event[0].GetEvent().Key - 1 // 다시 게시하면 원래 키가 된다(PostEvent가 key++)
	}
	for _, ev := range arr.Event {
		user := ev.GetEvent().UserName
		ctx := &simulator.Context{Map: m.Map(), Context: context.Background(),
			Session: &simulator.Session{UserSession: types.UserSession{UserName: user}, Map: m.Map()}}
		ctx.WithLock(em, func() { em.PostEvent(ctx, &types.PostEvent{This: em.Self, EventToPost: ev}) })
	}
	return nil
}
