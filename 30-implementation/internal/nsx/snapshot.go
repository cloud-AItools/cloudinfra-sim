package nsx

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Snapshot·Restore와 상태 불일치 검출(WP 5.2, FR-DM-0209·0306, NFR-0206·0207).
//
// 원본 상태는 저장소(SQLite nsx_resources)이고 Snapshot은 시뮬레이터가 저장소를 통째로 백업한다(store.Backup).
// 여기서는 저장소 밖의 엔진 상태(Realization 예정 시각, rule_id 번호, 시스템 설정, 알람)만 담는다. 복원 후 엔진의
// 읽기 캐시는 저장소에서 다시 읽고, vCenter 파생 객체(Fabric VM·VIF·Logical Switch 등)는 관계 조정이 다시 만든다.
// 세션은 모두 무효가 된다. Fault로 주입한 Realization 실패는 Fault Rule과 함께 지운다(복원은 Fault Rule을 비운다).

type engineState struct {
	Realized     map[string]time.Time      `json:"realized"` // intent path → SUCCESS 예정 가상 시각(0 = 이미 완료)
	RuleSeq      int                       `json:"rule_seq"`
	PartialPatch bool                      `json:"partial_patch"`
	Alarms       map[string]map[string]any `json:"alarms,omitempty"`
}

// ExportState는 저장소 밖의 엔진 상태다. 호출자는 교차 락을 쥐고 있어야 한다.
func (e *Engine) ExportState() ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := engineState{Realized: map[string]time.Time{}, RuleSeq: e.ruleSeq, PartialPatch: e.partialPatch, Alarms: e.alarms}
	for k, t := range e.realized {
		if _, failed := e.realFault[k]; failed {
			continue
		}
		st.Realized[k] = t
	}
	return json.MarshalIndent(st, "", "  ")
}

// ImportState는 저장소(이미 복원됨)에서 캐시를 다시 읽고 엔진 상태를 되돌린다.
func (e *Engine) ImportState(b []byte) error {
	var st engineState
	if err := json.Unmarshal(b, &st); err != nil {
		return fmt.Errorf("NSX 엔진 상태: %w", err)
	}
	rows, err := e.db.LoadNSX(context.Background())
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store = rows
	e.realized = st.Realized
	if e.realized == nil {
		e.realized = map[string]time.Time{}
	}
	e.ruleSeq, e.partialPatch = st.RuleSeq, st.PartialPatch
	e.alarms = st.Alarms
	if e.alarms == nil {
		e.alarms = map[string]map[string]any{}
	}
	e.sync, e.derived, e.realFault = map[string]syncInfo{}, map[string]bool{}, map[string]realFault{}
	e.sessions = map[string]session{}
	return nil
}

// Consistency는 상태 불일치를 찾는다(NFR-0207): ① 읽기 캐시와 저장소(원본)의 차이 ② 존재하지 않는 객체를 가리키는
// Policy 참조. 반환값이 비어 있으면 일관된 상태다.
func (e *Engine) Consistency() []string {
	rows, err := e.db.LoadNSX(context.Background())
	if err != nil {
		return []string{"nsx store: " + err.Error()}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for k, obj := range e.store {
		if e.derived[k] {
			continue
		}
		db, ok := rows[k]
		if !ok {
			out = append(out, "nsx cache-only "+k)
			continue
		}
		if !jsonEqual(obj, db) {
			out = append(out, "nsx cache differs from store "+k)
		}
	}
	for k := range rows {
		if _, ok := e.store[k]; !ok {
			out = append(out, "nsx store-only "+k)
		}
	}
	for k, obj := range e.store {
		if !strings.HasPrefix(k, policyBase) {
			continue
		}
		for _, r := range refsOf(obj) {
			if r.path == strings.TrimPrefix(k, policyBase) {
				continue
			}
			if _, ok := e.store[policyBase+r.path]; !ok {
				out = append(out, fmt.Sprintf("nsx dangling reference %s.%s → %s", strings.TrimPrefix(k, policyBase), r.field, r.path))
			}
		}
	}
	sort.Strings(out)
	return out
}

// jsonEqual은 JSON 왕복 기준으로 비교한다(캐시의 숫자 타입이 저장소에서 float64로 읽히는 차이를 무시).
func jsonEqual(a, b map[string]any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	var x, y any
	_ = json.Unmarshal(ja, &x)
	_ = json.Unmarshal(jb, &y)
	return reflect.DeepEqual(x, y)
}

// AlarmList는 현재 NSX 알람(Realization 실패 등)이다(Admin API 이벤트 조회, TR-SCN-0110).
func (e *Engine) AlarmList() []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	keys := make([]string, 0, len(e.alarms))
	for k := range e.alarms {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []map[string]any{}
	for _, k := range keys {
		a := map[string]any{"intent_path": k}
		for f, v := range e.alarms[k] {
			a[f] = v
		}
		out = append(out, a)
	}
	return out
}
