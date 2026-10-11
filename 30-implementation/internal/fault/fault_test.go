package fault

import (
	"testing"
	"time"
)

// T-FR-FLT-0205 T-FR-FLT-0125 T-FR-FLT-0202 T-FR-FLT-0204: 규칙 검증, 계층·조건 일치, N회 후 복구, Seed 결정론.
func TestRuleModel(t *testing.T) {
	s := NewStore()
	for _, bad := range []Rule{
		{Action: Action{Type: "explode"}},
		{Action: Action{Type: ActionDelay}},
		{Action: Action{Type: ActionStatus, Status: 200}},
		{Layer: "nsx", Action: Action{Type: ActionUnavailable}},
		{Layer: "vcenter", Action: Action{Type: ActionTaskFail}},
		{Layer: "vcenter", Action: Action{Type: ActionDatastoreFull}},
		{Layer: "bogus", Action: Action{Type: ActionExpire}},
		{Action: Action{Type: ActionUnavailable}, Count: -1},
		{Action: Action{Type: ActionUnavailable}, Probability: 2},
	} {
		if _, err := s.Add(bad); err == nil {
			t.Fatalf("invalid rule accepted: %+v", bad)
		}
	}
	r, err := s.Add(Rule{Match: Match{Product: "nsx", PathPrefix: "/policy/"}, Action: Action{Type: ActionUnavailable}, Count: 2})
	if err != nil || r.Action.Status != 503 || r.Layer != LayerTransport {
		t.Fatalf("add: %+v %v", r, err)
	}
	q := Request{Product: "nsx", Method: "GET", Path: "/policy/api/v1/infra"}
	for i := 0; i < 2; i++ {
		if _, ok := s.Apply(LayerTransport, q); !ok {
			t.Fatalf("hit %d must apply", i)
		}
	}
	if _, ok := s.Apply(LayerTransport, q); ok {
		t.Fatal("must recover after count")
	}
	if _, ok := s.Apply(LayerVCenter, q); ok {
		t.Fatal("other layer must not match")
	}
	if !s.Remove(r.ID) || s.Remove(r.ID) || len(s.List()) != 0 {
		t.Fatal("remove")
	}

	// 사용자·리소스·N번째 호출
	s.Add(Rule{Layer: LayerVCenter, Match: Match{SOAPMethod: "PowerOnVM_Task", Resource: "vm-1", User: "alice", Nth: 2}, Action: Action{Type: ActionTaskFail, Fault: "InvalidPowerState"}, Count: 1})
	call := Request{Product: "vcenter", SOAPMethod: "PowerOnVM_Task", Resource: "vm-1", User: "alice"}
	if _, ok := s.Apply(LayerVCenter, Request{Product: "vcenter", SOAPMethod: "PowerOnVM_Task", Resource: "vm-1", User: "bob"}); ok {
		t.Fatal("user mismatch")
	}
	if _, ok := s.Apply(LayerVCenter, call); ok {
		t.Fatal("1st call must pass (nth=2)")
	}
	if _, ok := s.Apply(LayerVCenter, call); !ok {
		t.Fatal("2nd call must fail")
	}
	if _, ok := s.Apply(LayerVCenter, call); ok {
		t.Fatal("count 1 exhausted")
	}
	s.Clear()

	// 호출 순서(after)·가상 시각 범위
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	nb := at.Add(time.Hour)
	s.Add(Rule{Layer: LayerNSX, Match: Match{After: "PATCH /policy/api/v1/infra/tier-1s", NotBefore: &nb}, Action: Action{Type: ActionRealizationFail}})
	nq := Request{Product: "nsx", Method: "PATCH", Path: "/policy/api/v1/infra/segments/x", Now: at.Add(2 * time.Hour)}
	if _, ok := s.Apply(LayerNSX, nq); ok {
		t.Fatal("not armed before 'after' call")
	}
	s.Observe(Request{Method: "PATCH", Path: "/policy/api/v1/infra/tier-1s/t1"})
	if _, ok := s.Apply(LayerNSX, Request{Product: "nsx", Now: at}); ok {
		t.Fatal("before not_before")
	}
	if _, ok := s.Apply(LayerNSX, nq); !ok {
		t.Fatal("armed and in time window")
	}
	s.Clear()

	// 인프라 상태 동작은 요청마다 적용하지 않는다
	s.Add(Rule{Layer: LayerVCenter, Match: Match{Resource: "datastore-1"}, Action: Action{Type: ActionDatastoreFull}})
	if _, ok := s.Apply(LayerVCenter, Request{Resource: "datastore-1"}); ok || len(s.Active(LayerVCenter, ActionDatastoreFull)) != 1 {
		t.Fatal("state action")
	}

	// 같은 Seed → 같은 적용 순서
	seq := func(seed int64) string {
		st := NewStore()
		_, _ = st.Add(Rule{Action: Action{Type: ActionStatus, Status: 500}, Probability: 0.5, Seed: seed})
		out := ""
		for i := 0; i < 32; i++ {
			if _, ok := st.Apply(LayerTransport, Request{Product: "vcenter"}); ok {
				out += "1"
			} else {
				out += "0"
			}
		}
		return out
	}
	if a, b := seq(7), seq(7); a != b || a == seq(8) {
		t.Fatalf("seed determinism: %s %s %s", a, b, seq(8))
	}
}
