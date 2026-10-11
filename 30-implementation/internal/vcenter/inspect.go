package vcenter

import (
	"time"
)

// EventRecord는 Admin API 이벤트 조회(TR-SCN-0110) 항목이다. 시각은 가상 시각(FR-BE-0611)이다.
type EventRecord struct {
	Key     int32     `json:"key"`
	Type    string    `json:"type"`
	Created time.Time `json:"created_time"`
	User    string    `json:"user,omitempty"`
	Entity  string    `json:"entity,omitempty"` // MoRef 값
	Name    string    `json:"entity_name,omitempty"`
	Message string    `json:"message"`
}

// RecentEvents는 vCenter EventManager의 최근 이벤트다(새 것 먼저). typ은 이벤트 타입 이름 필터(예: VmPoweredOnEvent),
// entity는 MoRef 값 필터다. vcsim 링버퍼(최근 1,000건)만 본다 — 더 긴 기록은 저장소 보관본(SyncEvents, store.VCEvents).
// (이전에는 QueryEvents의 MaxCount를 주지 않아 vcsim 기본 페이지 10건만 돌려주었다.)
func (v *VCenter) RecentEvents(limit int, typ, entity string) ([]EventRecord, error) {
	evs, err := latestEvents(v.Model())
	if err != nil {
		return nil, err
	}
	out := []EventRecord{}
	for i := len(evs) - 1; i >= 0; i-- {
		r := v.recordOf(evs[i])
		if (typ != "" && r.Type != typ) || (entity != "" && r.Entity != entity) {
			continue
		}
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (v *VCenter) toVirtual(t time.Time) time.Time {
	if v.Clock != nil {
		return v.Clock.ToVirtual(t).UTC()
	}
	return t.UTC()
}
