package nsx

import "testing"

// T-FR-NSX-0232 지원 검색 문법(Lucene 부분집합)과 T-FR-NSX-0215 Group expression 규칙의 단위 시험.
func TestSearchQuerySyntax(t *testing.T) {
	seg := map[string]any{"resource_type": "Segment", "display_name": "web-01", "id": "web-01", "path": "/infra/segments/web-01",
		"tags": []any{map[string]any{"scope": "app", "tag": "Web"}}, "_revision": float64(3)}
	t1 := map[string]any{"resource_type": "Tier1", "display_name": "T1 Gateway", "id": "t1-a", "path": "/infra/tier-1s/t1-a"}
	cases := []struct {
		q          string
		seg, tier1 bool
	}{
		{"resource_type:Segment", true, false},
		{"resource_type:segment", true, false},
		{"display_name:web*", true, false},
		{"display_name:w?b-01", true, false},
		{"resource_type:Segment AND display_name:db*", false, false},
		{"resource_type:Segment OR resource_type:Tier1", true, true},
		{"resource_type:Segment Tier1", true, true}, // 이어 쓴 항은 OR, 'Tier1'은 필드 없는 값(resource_type 일치)
		{"resource_type:Tier1 || web", true, true},
		{"NOT resource_type:Segment", false, true},
		{"-resource_type:Segment", false, true},
		{"(resource_type:Segment OR resource_type:Tier1) AND !display_name:web*", false, true},
		{`display_name:"T1 Gateway"`, false, true},
		{`display_name:"T1*"`, false, false}, // 따옴표 안의 *는 문자 그대로
		{"tags.scope:app AND tags.tag:web", true, false},
		{`path:\/infra\/segments\/*`, true, false},
		{"_revision:3", true, false},
		{"gateway", false, true}, // 필드 없는 값: 단어 일치
		{"*", true, true},
	}
	for _, c := range cases {
		n, err := parseQuery(c.q)
		if err != nil {
			t.Fatalf("%q: %v", c.q, err)
		}
		if got := n.match(seg); got != c.seg {
			t.Errorf("%q on segment = %v, want %v", c.q, got, c.seg)
		}
		if got := n.match(t1); got != c.tier1 {
			t.Errorf("%q on tier1 = %v, want %v", c.q, got, c.tier1)
		}
	}
	for _, bad := range []string{"", "display_name:", "(a OR b", "a AND", "x:[1 TO 2]", "name~2", "_exists_:tags", `"open`} {
		if _, err := parseQuery(bad); err == nil {
			t.Errorf("%q: expected syntax error", bad)
		}
	}
}

func TestGroupExpressionRules(t *testing.T) {
	cond := func(mt string, op string) map[string]any {
		m := map[string]any{"resource_type": "Condition", "member_type": mt, "key": "Tag", "value": "a|b"}
		if op != "" {
			m["operator"] = op
		}
		return m
	}
	and := map[string]any{"resource_type": "ConjunctionOperator", "conjunction_operator": "AND"}
	or := map[string]any{"resource_type": "ConjunctionOperator", "conjunction_operator": "OR"}
	ok := [][]any{
		{cond("VirtualMachine", "EQUALS")},
		{cond("Segment", "")}, // Segment·SegmentPort는 operator 생략 가능
		{cond("VirtualMachine", "EQUALS"), or, map[string]any{"resource_type": "NestedExpression", "expressions": []any{cond("VirtualMachine", "EQUALS"), and, cond("VirtualMachine", "CONTAINS")}}},
	}
	for i, x := range ok {
		if msg := checkExpressions(x, false); msg != "" {
			t.Errorf("valid #%d rejected: %s", i, msg)
		}
	}
	bad := [][]any{
		{cond("VirtualMachine", "EQUALS"), and}, // 짝수 길이
		{cond("VirtualMachine", "EQUALS"), cond("VirtualMachine", "EQUALS"), cond("VirtualMachine", "EQUALS")}, // 홀수 위치에 조건
		{cond("VirtualMachine", "")}, // VM은 operator 필요
		{map[string]any{"resource_type": "NestedExpression", "expressions": []any{cond("VirtualMachine", "EQUALS"), or, cond("VirtualMachine", "EQUALS")}}},
		{map[string]any{"resource_type": "NestedExpression", "expressions": []any{cond("VirtualMachine", "EQUALS"), and, cond("Segment", "EQUALS")}}},
		{cond("VirtualMachine", "EQUALS"), or, cond("VirtualMachine", "EQUALS"), or, cond("VirtualMachine", "EQUALS"), or, cond("VirtualMachine", "EQUALS"), or, cond("VirtualMachine", "EQUALS"), or, cond("VirtualMachine", "EQUALS")},
	}
	for i, x := range bad {
		if msg := checkExpressions(x, false); msg == "" {
			t.Errorf("invalid #%d accepted", i)
		}
	}
}
