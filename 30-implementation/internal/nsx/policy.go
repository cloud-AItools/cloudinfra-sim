package nsx

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// Policy 보안·라우팅 리소스 행위(WP 4.9, FR-NSX-0212 ~ 0218, 0223 ~ 0225). 경로·스키마는 스펙 범용 핸들러가 다루고,
// 여기서는 스펙 설명에 적힌 행위를 더한다. 근거는 SRC-NSX-321의 오퍼레이션·필드 설명이며, 근거가 없는 부분은 가정이다
// (BEHAVIOR_SPEC §11).

// embedded는 부모 객체 본문에 하위 객체 목록을 함께 쓰는 리소스다.
//   - Security/Gateway Policy의 rules: PATCH는 나열한 Rule을 만들거나 바꾸고(스펙 "pass all the rules which you wish to
//     edit as embedded rules"), PUT은 Rule 전체를 바꾼다("This is a full replace. All the rules are replaced"). 삭제는
//     Rule도 함께 지운다("Deletes the security policy along with all the rules").
//   - Service의 service_entries: 같은 방식(가정 — Service 오퍼레이션 설명에 명시 없음).
//
// 단일 GET 응답에 하위 목록을 넣는다(가정: 목록 조회에는 넣지 않음).
type embeddedKind struct {
	re         *regexp.Regexp
	field, seg string
	def        string
	ordered    bool // sequence_number 순서
	countField string
}

var embeddedKinds = []embeddedKind{
	{re: regexp.MustCompile(`^/infra/domains/[^/]+/(security|gateway)-policies/[^/]+$`), field: "rules", seg: "rules", def: "Rule", ordered: true, countField: "rule_count"},
	{re: regexp.MustCompile(`^/infra/services/[^/]+$`), field: "service_entries", seg: "service-entries", def: "ServiceEntry"},
}

func embeddedOf(rel string) *embeddedKind {
	for i := range embeddedKinds {
		if embeddedKinds[i].re.MatchString(rel) {
			return &embeddedKinds[i]
		}
	}
	return nil
}

var (
	ruleRe   = regexp.MustCompile(`^/infra/domains/[^/]+/(security|gateway)-policies/[^/]+/rules/[^/]+$`)
	groupRe  = regexp.MustCompile(`^/infra/domains/[^/]+/groups/[^/]+$`)
	policyRe = regexp.MustCompile(`^/infra/domains/[^/]+/(security|gateway)-policies/[^/]+$`)
)

// applyWrite는 객체 하나(내장 하위 객체·H-API children 포함)를 batch에 쓴다.
func (e *Engine) applyWrite(b *batch, spec *Spec, base, rel string, body map[string]any, put, partial bool, defaults map[string]any) (int, string) {
	if base != policyBase {
		return b.upsert(base, rel, body, put, partial, defaults)
	}
	emb := embeddedOf(rel)
	var kids []any
	hasKids := false
	if emb != nil {
		if v, ok := body[emb.field]; ok {
			kids, _ = v.([]any)
			hasKids = true
			delete(body, emb.field)
		}
	}
	children, hasChildren := body["children"].([]any)
	delete(body, "children")
	if st, msg := b.upsert(base, rel, body, put, partial, defaults); st != 0 {
		return st, msg
	}
	if emb != nil && hasKids {
		listed := map[string]bool{}
		for i, k := range kids {
			km, _ := k.(map[string]any)
			id, _ := km["id"].(string)
			if id == "" {
				// 가정: 내장 하위 객체는 id가 필요하다(경로를 만들 수 없음). 상태 400·메시지는 미검증
				return http.StatusBadRequest, fmt.Sprintf("%s[%d].id: required for embedded %s", emb.field, i, emb.def)
			}
			childRel := rel + "/" + emb.seg + "/" + id
			listed[childRel] = true
			if st, msg := e.applyWrite(b, spec, base, childRel, km, false, false, defaultsFor(spec, emb.def, km)); st != 0 {
				return st, msg
			}
		}
		if put {
			for _, k := range b.under(base+rel+"/"+emb.seg+"/", 0) {
				if r := strings.TrimPrefix(k, base); !listed[r] {
					if st, msg := b.remove(base, r, true); st != 0 {
						return st, msg
					}
				}
			}
		}
	}
	if hasChildren {
		return e.hierarchical(b, spec, rel, children, partial)
	}
	return 0, ""
}

// defaultsFor는 정의(다형이면 resource_type의 하위 타입)의 최상위 기본값이다. resource_type을 생략한 생성 요청은
// 정의 이름으로 채운다(스펙 resource_type: "The type of this resource", 실제 제품도 응답에 항상 포함).
func defaultsFor(spec *Spec, def string, body map[string]any) map[string]any {
	sc := spec.Compile(def)
	name := def
	if sub := sc.subtype(body); sub != nil {
		sc, name = sub, sub.ref
	}
	out := sc.Defaults()
	if _, ok := sc.Properties["resource_type"]; ok {
		out["resource_type"] = name
	}
	return out
}

// hierarchical은 Hierarchical API children을 처리한다(FR-NSX-0223 ~ 0225, 스펙 PatchInfra 설명).
//   - Child<Type> 래퍼: 감싼 객체와 그 children을 만들거나 바꾼다. marked_for_delete=true면 하위 트리와 함께 지운다.
//   - ChildResourceReference: 대상(id·target_type)은 바꾸지 않고 children만 처리한다. 대상이 없으면 400(가정).
//   - 하위 경로는 스펙에서 '부모 경로 + /<컬렉션>/{id}' 형태이고 PATCH 본문이 해당 타입인 템플릿으로 정한다.
//   - 참조·삭제 검사는 요청 전체를 모은 뒤 최종 상태로 판단하므로 children 순서와 무관하다(FR-NSX-0224, 스펙 "System will
//     resolve the dependecies of nodes within the intent tree").
func (e *Engine) hierarchical(b *batch, spec *Spec, parent string, children []any, partial bool) (int, string) {
	for i, c := range children {
		w, _ := c.(map[string]any)
		rt, _ := w["resource_type"].(string)
		at := fmt.Sprintf("%s.children[%d]", parent, i)
		if rt == "ChildResourceReference" {
			id, _ := w["id"].(string)
			typ, _ := w["target_type"].(string)
			p := spec.childPath(parent, typ, id)
			if p == "" {
				return http.StatusBadRequest, at + ": target_type " + typ + " cannot be a child of " + parent
			}
			if _, ok := b.get(policyBase + p); !ok {
				return http.StatusBadRequest, at + ": referenced object " + p + " does not exist"
			}
			sub, _ := w["children"].([]any)
			if st, msg := e.hierarchical(b, spec, p, sub, partial); st != 0 {
				return st, msg
			}
			continue
		}
		obj, typ := wrapped(w, rt)
		if obj == nil {
			return http.StatusBadRequest, at + ": " + rt + " must contain the wrapped object"
		}
		id, _ := obj["id"].(string)
		if id == "" {
			return http.StatusBadRequest, at + ": " + typ + ".id: required"
		}
		p := spec.childPath(parent, typ, id)
		if p == "" {
			return http.StatusBadRequest, at + ": " + typ + " cannot be a child of " + parent
		}
		if del, _ := w["marked_for_delete"].(bool); del {
			if st, msg := b.remove(policyBase, p, true); st != 0 {
				return st, msg
			}
			continue
		}
		if b.e.derived[policyBase+p] {
			return http.StatusForbidden, "The object " + p + " is system owned and cannot be modified."
		}
		if st, msg := e.applyWrite(b, spec, policyBase, p, copyMap(obj), false, partial, defaultsFor(spec, typ, obj)); st != 0 {
			return st, msg
		}
	}
	return 0, ""
}

// wrapped는 Child<Type> 래퍼에서 감싼 객체와 타입을 꺼낸다. 속성 이름은 보통 <Type>(예: ChildSegment.Segment)이다.
func wrapped(w map[string]any, rt string) (map[string]any, string) {
	name := strings.TrimPrefix(rt, "Child")
	if o, ok := w[name].(map[string]any); ok {
		if t, _ := o["resource_type"].(string); t != "" {
			return o, t
		}
		return o, name
	}
	keys := make([]string, 0, len(w))
	for k := range w {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if o, ok := w[k].(map[string]any); ok {
			if t, _ := o["resource_type"].(string); t != "" {
				return o, t
			}
		}
	}
	return nil, ""
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// childPath는 parent 아래 typ 객체(id)의 경로다. 스펙에서 PATCH(없으면 PUT) 본문이 typ인 템플릿 중 부모 부분이 parent와
// 맞는 것을 찾는다. 없으면 "".
func (s *Spec) childPath(parent, typ, id string) string {
	s.mu.Lock()
	if s.byBody == nil {
		s.byBody = map[string][]string{}
		for p := range s.Paths {
			if strings.Contains(p, "?") {
				continue
			}
			for _, m := range []string{"patch", "put"} {
				if op, ok := s.operation(p, m); ok && op.BodyRef != "" {
					s.byBody[op.BodyRef] = append(s.byBody[op.BodyRef], p)
					break
				}
			}
		}
		for k := range s.byBody {
			sort.Strings(s.byBody[k])
		}
	}
	tmpls := s.byBody[typ]
	s.mu.Unlock()
	ps := split(parent)
	for _, t := range tmpls {
		ts := split(t)
		if len(ts) != len(ps)+2 || !strings.HasPrefix(ts[len(ts)-1], "{") {
			continue
		}
		ok := true
		for i, seg := range ps {
			if !strings.HasPrefix(ts[i], "{") && ts[i] != seg {
				ok = false
				break
			}
		}
		if ok {
			return parent + "/" + ts[len(ts)-2] + "/" + id
		}
	}
	return ""
}

// prepareLocked는 리소스별 검사와 시스템 값을 적용한다(upsert 안, 기록 전).
func (e *Engine) prepareLocked(b *batch, rel string, cur map[string]any, exists bool, next map[string]any) (int, string) {
	// 기본 Service는 바꾸거나 지울 수 없다(스펙 Service.is_default: "Such default services can't be modified/deleted").
	// 거부 상태 403·메시지는 가정이다.
	if exists && cur["is_default"] == true && strings.HasPrefix(rel, "/infra/services/") {
		return http.StatusForbidden, "The default service " + rel + " cannot be modified or deleted."
	}
	switch {
	case ruleRe.MatchString(rel):
		if !exists {
			e.ruleSeq++
			next["rule_id"] = float64(e.ruleSeq) // 시스템이 정하는 4바이트 양수(스펙 rule_id). 시작 값 1000은 가정
		}
		if _, ok := next["sequence_number"]; !ok {
			next["sequence_number"] = float64(0) // 스펙: 지정하지 않으면 0
		}
		for _, f := range []string{"source_groups", "destination_groups", "services"} {
			if st, msg := checkAny(next, f); st != 0 {
				return st, msg
			}
		}
	case groupRe.MatchString(rel):
		if x, ok := next["expression"].([]any); ok {
			if msg := checkExpressions(x, false); msg != "" {
				return http.StatusBadRequest, "Invalid expression for group " + rel + ": " + msg
			}
		}
	}
	return 0, ""
}

// checkAny: "ANY"(대소문자 무관)는 배열의 유일한 원소여야 한다(스펙 source_groups·destination_groups·services 설명).
func checkAny(obj map[string]any, field string) (int, string) {
	a, _ := obj[field].([]any)
	for _, v := range a {
		if s, _ := v.(string); strings.EqualFold(s, "ANY") && len(a) > 1 {
			return http.StatusBadRequest, field + `: "ANY" must be the only element when used.`
		}
	}
	return 0, ""
}

// checkExpressions는 Group.expression 규칙(스펙 설명)을 검사한다: 비어 있지 않으면 홀수 길이, 짝수 위치는 조건·홀수 위치는
// ConjunctionOperator, Condition+NestedExpression 5개 이하, 경로·IP 등 500개 이하. NestedExpression은 Condition과 AND만,
// 같은 member_type이어야 한다. member_type이 Segment·SegmentPort가 아니면 Condition.operator가 필요하다.
func checkExpressions(x []any, nested bool) string {
	if len(x) == 0 {
		return ""
	}
	if len(x)%2 == 0 {
		return "the expression list must be of odd size"
	}
	conds, members := 0, 0
	memberType := ""
	for i, v := range x {
		m, _ := v.(map[string]any)
		rt, _ := m["resource_type"].(string)
		if i%2 == 1 {
			if rt != "ConjunctionOperator" {
				return fmt.Sprintf("expression[%d] must be a ConjunctionOperator", i)
			}
			if nested && m["conjunction_operator"] != "AND" {
				return "only AND is allowed inside a NestedExpression"
			}
			continue
		}
		if rt == "ConjunctionOperator" {
			return fmt.Sprintf("expression[%d] must not be a ConjunctionOperator", i)
		}
		if nested && rt != "Condition" {
			return "only Condition is allowed inside a NestedExpression"
		}
		switch rt {
		case "Condition":
			conds++
			mt, _ := m["member_type"].(string)
			if nested {
				if memberType != "" && mt != memberType {
					return "a NestedExpression must be homogeneous in member_type"
				}
				memberType = mt
			}
			if op, _ := m["operator"].(string); op == "" && mt != "Segment" && mt != "SegmentPort" {
				return fmt.Sprintf("expression[%d].operator is required for member_type %s", i, mt)
			}
		case "NestedExpression":
			conds++
			sub, _ := m["expressions"].([]any)
			if len(sub) == 0 {
				return "a NestedExpression must not be empty"
			}
			if msg := checkExpressions(sub, true); msg != "" {
				return msg
			}
		case "IPAddressExpression":
			a, _ := m["ip_addresses"].([]any)
			members += len(a)
		case "MACAddressExpression":
			a, _ := m["mac_addresses"].([]any)
			members += len(a)
		case "ExternalIDExpression":
			a, _ := m["external_ids"].([]any)
			members += len(a)
		case "PathExpression":
			a, _ := m["paths"].([]any)
			members += len(a)
		}
	}
	if conds > 5 {
		return "at most 5 Condition and NestedExpression are allowed"
	}
	if members > 500 {
		return "at most 500 IP/MAC addresses, external IDs and paths are allowed"
	}
	return ""
}

// decorate는 조회 응답을 만든다: 내장 하위 목록(단일 GET)과 rule_count.
func (e *Engine) decorateLocked(rel string, obj map[string]any, single bool) map[string]any {
	emb := embeddedOf(rel)
	if emb == nil {
		return obj
	}
	kids := e.childrenLocked(rel, emb)
	out := copyMap(obj)
	if emb.countField != "" {
		out[emb.countField] = float64(len(kids))
	}
	if single {
		out[emb.field] = kids
	}
	return out
}

func (e *Engine) childrenLocked(rel string, emb *embeddedKind) []map[string]any {
	kids := []map[string]any{}
	for _, k := range sortedKeys(e.store, policyBase+rel+"/"+emb.seg+"/", 0) {
		kids = append(kids, e.store[k])
	}
	if emb.ordered {
		sortBySequence(kids)
	}
	return kids
}

func seqOf(o map[string]any) float64 { f, _ := o["sequence_number"].(float64); return f }

func sortBySequence(items []map[string]any) {
	sort.SliceStable(items, func(i, j int) bool { return seqOf(items[i]) < seqOf(items[j]) })
}

// revise는 POST …?action=revise다(ReviseSecurityPolicies·ReviseSecurityRule): 본문으로 객체를 갱신하고 operation
// (insert_top 기본·insert_bottom·insert_after·insert_before, anchor_path)에 따라 순서를 바꾼다.
// 순서는 sequence_number로 표현한다. 필요한 위치에 빈 번호가 없으면 형제 전체를 10 간격으로 다시 매긴다(가정: 번호
// 체계와 형제의 _revision 증가 여부는 미검증). Policy의 형제는 같은 Domain·같은 category의 Policy다.
func (e *Engine) reviseLocked(b *batch, spec *Spec, rel string, body map[string]any, op, anchor string) (int, string) {
	switch op {
	case "":
		op = "insert_top"
	case "insert_top", "insert_bottom":
	case "insert_after", "insert_before":
		if _, ok := b.get(policyBase + anchor); !ok || anchor == rel {
			return http.StatusBadRequest, "Invalid anchor_path " + anchor
		}
	default:
		return http.StatusBadRequest, "Invalid operation " + op
	}
	def := "Rule"
	if policyRe.MatchString(rel) {
		def = "SecurityPolicy"
	}
	if st, msg := e.applyWrite(b, spec, policyBase, rel, body, false, e.partialPatch, defaultsFor(spec, def, body)); st != 0 {
		return st, msg
	}
	self, _ := b.get(policyBase + rel)
	collection := rel[:strings.LastIndex(rel, "/")+1]
	var sibs []map[string]any
	for _, k := range b.under(policyBase+collection, 0) {
		o, _ := b.get(k)
		if k == policyBase+rel || (def == "SecurityPolicy" && o["category"] != self["category"]) {
			continue
		}
		sibs = append(sibs, o)
	}
	sortBySequence(sibs)
	pos := 0
	switch op {
	case "insert_bottom":
		pos = len(sibs)
	case "insert_after", "insert_before":
		pos = -1
		for i, s := range sibs {
			if s["path"] == anchor {
				pos = i
				if op == "insert_after" {
					pos++
				}
			}
		}
		if pos < 0 {
			return http.StatusBadRequest, "anchor_path " + anchor + " is not a sibling of " + rel
		}
	}
	hasLo, hasHi := pos > 0, pos < len(sibs)
	var lo, hi float64
	if hasLo {
		lo = seqOf(sibs[pos-1])
	}
	if hasHi {
		hi = seqOf(sibs[pos])
	}
	switch {
	case !hasLo && !hasHi:
		return 0, ""
	case !hasLo && hi >= 1:
		self["sequence_number"] = hi - 1
	case hasLo && !hasHi:
		self["sequence_number"] = lo + 1
	case hasLo && hi-lo > 1:
		self["sequence_number"] = lo + 1
	default:
		order := append(append(append([]map[string]any{}, sibs[:pos]...), self), sibs[pos:]...)
		for i, o := range order {
			n := o
			if o["path"] != self["path"] {
				n = copyMap(o)
				r, _ := o["_revision"].(float64)
				n["_revision"] = r + 1
			}
			n["sequence_number"] = float64((i + 1) * 10)
			p, _ := n["path"].(string)
			b.set(policyBase+p, n)
		}
	}
	return 0, ""
}
