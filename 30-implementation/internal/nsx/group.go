package nsx

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Group 유효 멤버 계산(FR-NSX-0229 태그 기반 멤버십, FR-NSX-0605 VM 관련 정책 참조).
//
// GET /infra/domains/{d}/groups/{g}/members/{virtual-machines|segments|segment-ports|ip-addresses}.
// 후보: VirtualMachine = Fabric VM(vCenter 파생, NSX 소유 VM Tag 반영), Segment = /infra/segments/*, SegmentPort =
// /infra/segments/*/ports/*(시스템 소유 포함). expression은 왼쪽부터 ConjunctionOperator로 결합한다.
//   - Condition: member_type이 후보 유형과 같아야 한다. key Name·Tag, operator EQUALS·NOTEQUALS·CONTAINS·STARTSWITH·
//     ENDSWITH(없으면 EQUALS), scope_operator EQUALS·NOTEQUALS. 스펙: "Evaluation of the condition expression will be
//     case insensitive". Tag 값은 "scope|tag" 또는 "tag"(scope 무시)이며 "|tag"는 scope를 보지 않는다.
//   - ExternalIDExpression(VirtualMachine): external_id 일치. PathExpression: 후보 경로 일치, 또는 경로가 Group이면 그
//     Group의 멤버(중첩 Group). NestedExpression: 내부 Condition의 AND.
//   - ip-addresses: IPAddressExpression의 주소와 PathExpression으로 포함한 Group의 주소(합집합, 가정).
// NOTIN, OSName·ComputerName 등 게스트 정보 key, MACAddressExpression·IdentityGroupExpression은 평가하지 않는다(미지원).

var groupMembersRe = regexp.MustCompile(`^(/infra/domains/[^/]+/groups/[^/]+)/members/(virtual-machines|segments|segment-ports|ip-addresses)$`)

type candidate struct {
	typ, name, path, ext string
	tags                 []any
	obj                  map[string]any
}

func (e *Engine) groupMembers(w http.ResponseWriter, r *http.Request, groupRel, kind string) {
	e.mu.Lock()
	g, ok := e.store[policyBase+groupRel]
	if !ok {
		e.mu.Unlock()
		apiError(w, http.StatusNotFound, "Policy", "The path="+groupRel+" is invalid.")
		return
	}
	var items []map[string]any
	var ips []string
	switch kind {
	case "ip-addresses":
		ips = e.groupIPsLocked(g, map[string]bool{groupRel: true})
	default:
		typ := map[string]string{"virtual-machines": "VirtualMachine", "segments": "Segment", "segment-ports": "SegmentPort"}[kind]
		for _, c := range e.candidatesLocked(typ) {
			if !e.evalExprsLocked(exprsOf(g), c, map[string]bool{groupRel: true}) {
				continue
			}
			if typ == "VirtualMachine" {
				items = append(items, realizedVM(c.obj))
			} else {
				items = append(items, map[string]any{"path": c.path, "display_name": c.name, "id": c.path[strings.LastIndex(c.path, "/")+1:]})
			}
		}
	}
	e.mu.Unlock()
	if kind == "ip-addresses" {
		// IP 목록은 문자열 배열이다(PolicyGroupIPMembersListResult). 페이지 계산은 같은 규칙을 쓴다.
		q := r.URL.Query()
		lo, size := cursorOf(q.Get("cursor"), q.Get("page_size"), len(ips))
		hi := min(lo+size, len(ips))
		out := map[string]any{"result_count": len(ips), "results": append([]string{}, ips[lo:hi]...)}
		if hi < len(ips) {
			out["cursor"] = strconv.Itoa(hi)
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, page(items, r))
}

func exprsOf(g map[string]any) []any { x, _ := g["expression"].([]any); return x }

func (e *Engine) candidatesLocked(typ string) []candidate {
	var out []candidate
	switch typ {
	case "VirtualMachine":
		for _, k := range sortedKeys(e.store, managerBase+"/fabric/virtual-machines/", 0) {
			vm := e.store[k]
			c := candidate{typ: typ, obj: vm}
			c.name, _ = vm["display_name"].(string)
			c.ext, _ = vm["external_id"].(string)
			c.tags, _ = vm["tags"].([]any)
			out = append(out, c)
		}
	case "Segment":
		for _, k := range sortedKeys(e.store, policyBase+"/infra/segments/", 0) {
			out = append(out, policyCandidate(typ, k, e.store[k]))
		}
	case "SegmentPort":
		for _, k := range sortedKeys(e.store, policyBase+"/infra/segments/", 2) {
			if strings.Contains(k, "/ports/") {
				out = append(out, policyCandidate(typ, k, e.store[k]))
			}
		}
	}
	return out
}

func policyCandidate(typ, key string, o map[string]any) candidate {
	c := candidate{typ: typ, obj: o, path: strings.TrimPrefix(key, policyBase)}
	c.name, _ = o["display_name"].(string)
	c.tags, _ = o["tags"].([]any)
	return c
}

func (e *Engine) evalExprsLocked(exprs []any, c candidate, seen map[string]bool) bool {
	if len(exprs) == 0 {
		return false
	}
	result, op, first := false, "OR", true
	for _, x := range exprs {
		m, _ := x.(map[string]any)
		if m["resource_type"] == "ConjunctionOperator" {
			if s, _ := m["conjunction_operator"].(string); s != "" {
				op = s
			}
			continue
		}
		v := e.evalOneLocked(m, c, seen)
		switch {
		case first:
			result = v
		case op == "AND":
			result = result && v
		default:
			result = result || v
		}
		first, op = false, "OR"
	}
	return result
}

func (e *Engine) evalOneLocked(m map[string]any, c candidate, seen map[string]bool) bool {
	switch m["resource_type"] {
	case "Condition":
		if mt, _ := m["member_type"].(string); mt != c.typ {
			return false
		}
		return evalCondition(m, c)
	case "NestedExpression":
		sub, _ := m["expressions"].([]any)
		return e.evalExprsLocked(sub, c, seen)
	case "ExternalIDExpression":
		if mt, _ := m["member_type"].(string); mt != c.typ || c.ext == "" {
			return false
		}
		ids, _ := m["external_ids"].([]any)
		for _, id := range ids {
			if id == c.ext {
				return true
			}
		}
	case "PathExpression":
		ps, _ := m["paths"].([]any)
		for _, p := range ps {
			s, _ := p.(string)
			if c.path != "" && s == c.path {
				return true
			}
			if groupRe.MatchString(s) && !seen[s] {
				if g, ok := e.store[policyBase+s]; ok {
					seen[s] = true
					hit := e.evalExprsLocked(exprsOf(g), c, seen)
					delete(seen, s)
					if hit {
						return true
					}
				}
			}
		}
	}
	return false
}

func evalCondition(m map[string]any, c candidate) bool {
	op, _ := m["operator"].(string)
	if op == "" {
		op = "EQUALS"
	}
	val, _ := m["value"].(string)
	switch m["key"] {
	case "Name":
		return strMatch(op, c.name, val)
	case "Tag":
		scope, tag, scoped := strings.Cut(val, "|")
		if !scoped {
			tag, scope = val, ""
		}
		scopeOp, _ := m["scope_operator"].(string)
		hit := false
		for _, t := range c.tags {
			tm, _ := t.(map[string]any)
			ts, _ := tm["scope"].(string)
			tt, _ := tm["tag"].(string)
			scopeOK := !scoped || scope == "" || strings.EqualFold(ts, scope)
			if scopeOp == "NOTEQUALS" && scoped && scope != "" {
				scopeOK = !strings.EqualFold(ts, scope)
			}
			tagOK := tag == "" || strMatch(strings.Replace(op, "NOTEQUALS", "EQUALS", 1), tt, tag)
			if scopeOK && tagOK {
				hit = true
			}
		}
		if op == "NOTEQUALS" {
			return !hit
		}
		return hit
	}
	return false
}

func strMatch(op, s, v string) bool {
	s, v = strings.ToLower(s), strings.ToLower(v)
	switch op {
	case "EQUALS":
		return s == v
	case "NOTEQUALS":
		return s != v
	case "CONTAINS":
		return strings.Contains(s, v)
	case "STARTSWITH":
		return strings.HasPrefix(s, v)
	case "ENDSWITH":
		return strings.HasSuffix(s, v)
	}
	return false
}

func (e *Engine) groupIPsLocked(g map[string]any, seen map[string]bool) []string {
	set := map[string]bool{}
	for _, x := range exprsOf(g) {
		m, _ := x.(map[string]any)
		switch m["resource_type"] {
		case "IPAddressExpression":
			a, _ := m["ip_addresses"].([]any)
			for _, ip := range a {
				if s, ok := ip.(string); ok {
					set[s] = true
				}
			}
		case "PathExpression":
			ps, _ := m["paths"].([]any)
			for _, p := range ps {
				s, _ := p.(string)
				if sub, ok := e.store[policyBase+s]; ok && groupRe.MatchString(s) && !seen[s] {
					seen[s] = true
					for _, ip := range e.groupIPsLocked(sub, seen) {
						set[ip] = true
					}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for ip := range set {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}
