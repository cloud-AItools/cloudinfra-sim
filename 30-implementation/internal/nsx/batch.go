package nsx

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"cloudinfra-sim/internal/store"
)

// batch는 한 요청의 쓰기 묶음이다(NFR-0204, FR-DM-0305). 단일 리소스 쓰기, 내장 하위 객체(Security Policy의 rules,
// Service의 service_entries), Hierarchical API(FR-NSX-0223)가 같은 경로를 쓴다.
//
//   - 변경은 pending에 모으고, 조회는 pending → 캐시 순으로 본다(같은 요청 안의 참조를 순서와 무관하게 해석, FR-NSX-0224).
//   - 참조 유효성(FR-NSX-0410)과 삭제 참조 검사(FR-NSX-0415)는 모든 변경을 모은 뒤 commit에서 최종 상태로 판단한다.
//   - 저장소에는 한 트랜잭션으로 기록하고, 성공한 뒤에만 캐시를 바꾼다. 하나라도 실패하면 아무것도 반영하지 않는다
//     (H-API 원자성, FR-NSX-0225: 공식 근거 미확보 — 가정, BEHAVIOR_SPEC §11).
//
// 호출자는 e.mu를 잡고 있어야 한다.
type batch struct {
	e       *Engine
	pending map[string]map[string]any // 키 → 객체(nil = 삭제)
	order   []string
	touched []string // 참조 검사 대상(쓴 객체)
	deleted []string
	post    []func()
	noRev   bool // H-API에서 enforce_revision_check=false(기본): _revision 검사 생략
	user    string
}

func (e *Engine) newBatch(user string) *batch {
	if user == "" {
		user = "admin"
	}
	return &batch{e: e, pending: map[string]map[string]any{}, user: user}
}

func (b *batch) get(key string) (map[string]any, bool) {
	if o, ok := b.pending[key]; ok {
		return o, o != nil
	}
	o, ok := b.e.store[key]
	return o, ok
}

func (b *batch) set(key string, obj map[string]any) {
	if _, ok := b.pending[key]; !ok {
		b.order = append(b.order, key)
	}
	b.pending[key] = obj
}

// under는 prefix 아래의 객체 키다(pending 반영). depth < 0이면 모든 하위, 아니면 prefix 뒤 '/' 수가 depth인 키.
func (b *batch) under(prefix string, depth int) []string {
	seen := map[string]bool{}
	var out []string
	add := func(k string) {
		if seen[k] || !strings.HasPrefix(k, prefix) {
			return
		}
		seen[k] = true
		if depth >= 0 && strings.Count(strings.TrimPrefix(k, prefix), "/") != depth {
			return
		}
		if _, ok := b.get(k); ok {
			out = append(out, k)
		}
	}
	for k := range b.e.store {
		add(k)
	}
	for _, k := range b.order {
		add(k)
	}
	sort.Strings(out)
	return out
}

// systemFields는 시스템이 정하는 읽기전용 값이다. 갱신·교체해도 유지한다.
var systemFields = []string{"_create_time", "_create_user", "unique_id", "rule_id", "is_default", "_system_owned", "realization_id"}

// upsert는 PATCH·PUT과 _revision 규칙을 적용한다(FR-NSX-0403~0409).
//
// PATCH 의미(FR-NSX-0405): 부분 PATCH가 켜져 있으면 지정한 필드만 바꾸고(중첩 객체는 재귀 병합, 배열은 교체),
// 꺼져 있으면(기본값) 요청 본문을 객체 전체로 본다(지정하지 않은 사용자 필드는 남지 않는다). 근거: 스펙
// /system-config/nsx-partial-patch-config(기본 false)와 API Guide의 Partial PATCH 설명. 리소스별 예외는 미구현.
func (b *batch) upsert(base, rel string, body map[string]any, put, partial bool, defaults map[string]any) (int, string) {
	e := b.e
	key := base + rel
	cur, exists := b.get(key)
	if exists && !b.noRev {
		rev, given := body["_revision"]
		if put && !given {
			return http.StatusPreconditionFailed, "The object was modified by somebody else. _revision is required for PUT."
		}
		if given && rev != cur["_revision"] {
			return http.StatusPreconditionFailed, "The object was modified by somebody else."
		}
	}
	next := map[string]any{}
	if exists && !put && partial {
		next = deepMerge(cur, body)
	} else {
		for k, v := range body {
			next[k] = v
		}
		// 생성·교체 시 스펙 기본값 적용(FR-NSX-0107): admin_state=UP 등, display_name은 "Defaults to ID if not set".
		for k, v := range defaults {
			if _, ok := next[k]; !ok {
				next[k] = v
			}
		}
		if _, ok := next["display_name"]; !ok {
			next["display_name"] = rel[strings.LastIndex(rel, "/")+1:]
		}
	}
	now := float64(e.now().UnixMilli())
	id := rel[strings.LastIndex(rel, "/")+1:]
	if exists {
		for _, f := range systemFields {
			if v, ok := cur[f]; ok {
				next[f] = v
			}
		}
		r, _ := cur["_revision"].(float64)
		next["_revision"] = r + 1
	} else {
		next["_create_time"], next["_create_user"], next["_revision"] = now, b.user, float64(0)
		if base == policyBase {
			next["unique_id"] = newUUID() // Policy 리소스만 unique_id가 있다(Manager ManagedResource에는 없음)
		}
	}
	next["id"] = id
	next["_last_modified_time"], next["_last_modified_user"] = now, b.user
	delete(next, "marked_for_delete")
	delete(next, "children")
	if base == policyBase {
		next["path"], next["relative_path"] = rel, id
		next["parent_path"] = rel[:strings.LastIndex(rel, "/")]
		if status, msg := e.prepareLocked(b, rel, cur, exists, next); status != 0 {
			return status, msg
		}
		at := e.now().Add(e.delay)
		segment := len(e.onChange) > 0 && strings.HasPrefix(rel, "/infra/segments/") && strings.Count(rel, "/") == 3
		b.post = append(b.post, func() {
			e.realized[rel] = at
			delete(e.realFault, rel) // 다시 쓴 intent는 다시 실현한다
			if segment {
				e.sync[rel] = syncInfo{pending: true} // vCenter 동기화 전(FR-DM-0404)
			}
		})
	}
	b.set(key, next)
	b.touched = append(b.touched, key)
	return 0, ""
}

// remove는 객체를 지운다. cascade가 아니면 하위 객체가 남아 있을 때 거부한다(FR-NSX-0415, TR-E2E-0804; 상태 400은 가정).
// cascade는 내장 하위 객체(Security Policy 삭제 시 rules, 스펙 "Deletes the security policy along with all the rules")와
// H-API marked_for_delete(스펙: "This resource along with its all children in intent tree will be deleted")에 쓴다.
// 없는 객체 삭제는 성공으로 본다(가정, FR-NSX-0406 계약 확인 필요).
func (b *batch) remove(base, rel string, cascade bool) (int, string) {
	key := base + rel
	if _, ok := b.get(key); !ok {
		return 0, ""
	}
	kids := b.under(key+"/", -1)
	if len(kids) > 0 && !cascade {
		return http.StatusBadRequest, "Cannot delete " + rel + ": it has " + strconv.Itoa(len(kids)) + " child object(s) such as " + strings.TrimPrefix(kids[0], base)
	}
	for _, k := range append(kids, key) {
		if b.e.derived[k] {
			return http.StatusForbidden, "The object " + strings.TrimPrefix(k, base) + " is system owned and cannot be modified."
		}
		b.set(k, nil)
		b.deleted = append(b.deleted, k)
		r := strings.TrimPrefix(k, base)
		b.post = append(b.post, func() {
			delete(b.e.realized, r)
			delete(b.e.sync, r)
			delete(b.e.realFault, r)
		})
	}
	return 0, ""
}

// commit은 참조를 검사하고 한 트랜잭션으로 기록한 뒤 캐시를 바꾼다.
func (b *batch) commit() (int, string) {
	e := b.e
	for _, k := range b.touched {
		obj, ok := b.get(k)
		if !ok {
			continue
		}
		for _, r := range refsOf(obj) {
			if r.path == strings.TrimPrefix(k, policyBase) {
				continue
			}
			if _, exists := b.get(policyBase + r.path); !exists {
				// 가정: 상태 400, 메시지 형식(스펙에 상황별 error_code 없음)
				return http.StatusBadRequest, "Invalid " + r.field + " " + r.path + ". The referenced object does not exist."
			}
		}
	}
	if len(b.deleted) > 0 {
		gone := map[string]bool{}
		for _, k := range b.deleted {
			gone[strings.TrimPrefix(k, policyBase)] = true
		}
		check := func(k string, obj map[string]any) (int, string) {
			if obj == nil || !strings.HasPrefix(k, policyBase) {
				return 0, ""
			}
			for _, r := range refsOf(obj) {
				if gone[r.path] {
					return http.StatusBadRequest, "Cannot delete " + r.path + ": it is referenced by " + strings.TrimPrefix(k, policyBase) + "." + r.field
				}
			}
			return 0, ""
		}
		for k, obj := range e.store {
			if _, overridden := b.pending[k]; overridden {
				continue
			}
			if st, msg := check(k, obj); st != 0 {
				return st, msg
			}
		}
		for _, k := range b.order {
			if st, msg := check(k, b.pending[k]); st != 0 {
				return st, msg
			}
		}
	}
	if len(b.order) == 0 {
		return 0, ""
	}
	tx, err := e.db.BeginNSX(context.Background())
	if err != nil {
		return http.StatusInternalServerError, "storage error: " + err.Error()
	}
	for _, k := range b.order {
		if obj := b.pending[k]; obj != nil {
			err = tx.Put(k, obj)
		} else {
			err = tx.Delete(k)
		}
		if err != nil {
			_ = tx.Rollback()
			return http.StatusInternalServerError, "storage error: " + err.Error()
		}
	}
	if err := tx.Commit(); err != nil {
		return http.StatusInternalServerError, "storage error: " + err.Error()
	}
	for _, k := range b.order {
		if obj := b.pending[k]; obj != nil {
			e.store[k] = obj
		} else {
			delete(e.store, k)
		}
	}
	for _, f := range b.post {
		f()
	}
	return 0, ""
}

type ref struct{ field, path string }

// 참조 필드(FR-NSX-0410·0415). 값이 Policy 경로(/infra/...)인 것만 참조로 본다("ANY", IP 주소 등은 제외).
var (
	refScalar = []string{"connectivity_path", "transport_zone_path", "tier0_path", "tier1_path", "segment_path", "edge_cluster_path", "scheduler_path"}
	refArray  = []string{"source_groups", "destination_groups", "services", "scope", "edge_cluster_member_paths"}
)

func refsOf(obj map[string]any) []ref {
	var out []ref
	add := func(field string, v any) {
		if s, ok := v.(string); ok && strings.HasPrefix(s, "/infra/") {
			out = append(out, ref{field, s})
		}
	}
	for _, f := range refScalar {
		add(f, obj[f])
	}
	for _, f := range refArray {
		if a, ok := obj[f].([]any); ok {
			for _, v := range a {
				add(f, v)
			}
		}
	}
	var walk func(exprs []any)
	walk = func(exprs []any) {
		for _, x := range exprs {
			m, _ := x.(map[string]any)
			switch m["resource_type"] {
			case "PathExpression":
				if ps, ok := m["paths"].([]any); ok {
					for _, p := range ps {
						add("expression.paths", p)
					}
				}
			case "NestedExpression":
				sub, _ := m["expressions"].([]any)
				walk(sub)
			}
		}
	}
	if x, ok := obj["expression"].([]any); ok {
		walk(x)
	}
	// Tier-0/1 locale-services의 edge_cluster_path는 스칼라로 이미 포함된다.
	return out
}

func (e *Engine) persist(f func(*store.NSXTx) error) error {
	tx, err := e.db.BeginNSX(context.Background())
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
