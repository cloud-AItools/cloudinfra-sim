// vCenter 이벤트 보관(R-44, 2026-10-11): vcsim 링버퍼(1,000건)를 넘는 이벤트를 저장소에 보관하고(lite 1,000건, full
// 100,000건·7일), Snapshot 복원·Fixture 캐시 적재 뒤에도 이벤트 기록(생성 이벤트)이 같은 키로 남는다.
package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/store"
	"cloudinfra-sim/internal/vcenter"
	"cloudinfra-sim/test/internal/testenv"
)

func postUserEvents(t *testing.T, e *testenv.Env, n int, tag string) {
	t.Helper()
	c := e.Login(testenv.VCenterVersion())
	m := event.NewManager(c.Client)
	ctx := context.Background()
	for i := range n {
		ev := &types.GeneralUserEvent{GeneralEvent: types.GeneralEvent{Message: fmt.Sprintf("%s-%d", tag, i)}}
		if err := m.PostEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
}

// archived는 Admin 이벤트 조회(보관본)의 vCenter 이벤트다.
func archived(t *testing.T, e *testenv.Env, limit int, typ string) ([]store.VCEvent, map[string]any) {
	t.Helper()
	res, err := e.Sim.Events(limit, typ, "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	var out struct {
		VCenter   []store.VCEvent `json:"vcenter"`
		Retention map[string]any  `json:"retention"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.VCenter, out.Retention
}

// T-NFR-0503 T-TR-SCN-0110 T-FR-MET-0203
func TestVCenterEventArchive(t *testing.T) {
	// full: 링버퍼를 넘는 이벤트를 보관한다
	e := testenv.Start(t, func(c *config.Config) { c.RunProfile = "full" })
	postUserEvents(t, e, 800, "a")
	if got, _ := archived(t, e, 5000, "GeneralUserEvent"); len(got) != 800 {
		t.Fatalf("1차 보관 %d건, 기대 800", len(got))
	}
	postUserEvents(t, e, 800, "b")
	got, ret := archived(t, e, 5000, "GeneralUserEvent")
	if len(got) != 1600 {
		t.Fatalf("보관 %d건, 기대 1600(vcsim 링버퍼는 1,000건)", len(got))
	}
	if got[0].Message != "b-799" || got[len(got)-1].Message != "a-0" {
		t.Fatalf("최근 순이 아니다: %s … %s", got[0].Message, got[len(got)-1].Message)
	}
	if fmt.Sprint(ret["vcenter_events"]) != "100000" || fmt.Sprint(ret["vcenter_events_dropped"]) != "0" {
		t.Fatalf("retention %v", ret)
	}
	ring, err := e.Sim.VC.RecentEvents(5000, "GeneralUserEvent", "")
	if err != nil || len(ring) > 1001 {
		t.Fatalf("vcsim 기록 %d건(%v) — 링버퍼 1,000건 기대", len(ring), err)
	}
	// Reset: 새 환경의 이벤트만 남는다
	if err := e.Sim.Reset(); err != nil {
		t.Fatal(err)
	}
	if got, _ := archived(t, e, 5000, "GeneralUserEvent"); len(got) != 0 {
		t.Fatalf("Reset 뒤 이전 환경 이벤트 %d건이 남았다", len(got))
	}
	if got, _ := archived(t, e, 5000, "VmCreatedEvent"); len(got) == 0 {
		t.Fatal("Reset 뒤 새 환경의 생성 이벤트가 보관되지 않았다")
	}

	// lite: 보관 한도 1,000건
	l := testenv.Start(t, nil)
	postUserEvents(t, l, 700, "x")
	archived(t, l, 10, "")
	postUserEvents(t, l, 700, "y")
	all, ret := archived(t, l, 100000, "")
	if len(all) != 1000 || fmt.Sprint(ret["vcenter_events"]) != "1000" {
		t.Fatalf("lite 보관 %d건(한도 %v), 기대 1000", len(all), ret["vcenter_events"])
	}
}

// T-FR-DM-0304 T-TR-SCN-0105: Snapshot 복원 뒤 vCenter 이벤트 기록(키·타입·대상)이 복원 전과 같고 보관본이 중복되지 않는다.
func TestEventsSurviveRestore(t *testing.T) {
	e := testenv.Start(t, nil)
	before, err := e.Sim.VC.RecentEvents(1000, "", "")
	if err != nil || len(before) == 0 {
		t.Fatalf("events: %d %v", len(before), err)
	}
	if _, err := e.Sim.CreateSnapshot("ev"); err != nil {
		t.Fatal(err)
	}
	archivedBefore, _ := archived(t, e, 100000, "")
	if _, err := e.Sim.Restore("ev"); err != nil {
		t.Fatal(err)
	}
	after, err := e.Sim.VC.RecentEvents(1000, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(slim(before)) != fmt.Sprint(slim(after)) {
		t.Fatalf("복원 뒤 이벤트 기록이 다르다: %d건 → %d건", len(before), len(after))
	}
	if archivedAfter, _ := archived(t, e, 100000, ""); len(archivedAfter) != len(archivedBefore) {
		t.Fatalf("복원 뒤 보관본 %d건, 복원 전 %d건(중복 보관)", len(archivedAfter), len(archivedBefore))
	}
}

// vcEvent는 생성 시각을 뺀 이벤트 요약이다(복원·적재 때 시각은 '지금'으로 다시 붙는다).
type vcEvent struct {
	Key                         int32
	Type, Entity, User, Message string
}

func slim(rs []vcenter.EventRecord) []vcEvent {
	out := make([]vcEvent, len(rs))
	for i, r := range rs {
		out[i] = vcEvent{r.Key, r.Type, r.Entity, r.User, r.Message}
	}
	return out
}
