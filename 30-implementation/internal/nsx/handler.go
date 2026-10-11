package nsx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cloudinfra-sim/internal/fault"
	"cloudinfra-sim/internal/rbac"
)

// ServeHTTP는 인증 → 경로 판정(활성 스펙) → 행위 처리 순서로 NSX 요청을 처리한다.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 세션 생성·삭제(스펙 밖 경로, FR-API-0207). 계약 근거 미확보 — execution_scope 밖(NSX-001B).
	switch r.URL.Path {
	case "/api/session/create":
		e.sessionCreate(w, r)
		return
	case "/api/session/destroy":
		e.sessionDestroy(w, r)
		return
	}
	user, ok := e.authenticate(w, r)
	if !ok {
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), userKey{}, user)) // Fault 조건 match.user
	// 권한(FR-API-0211·0212): auditor는 조회만. 가정: 403 ApiError(메시지 형식 미검증)
	if e.Role != nil {
		if role, _ := e.Role("nsx", user); !rbac.NSXAllows(role, r.Method) {
			apiError(w, http.StatusForbidden, "common-services", "The user does not have permission to perform this operation.")
			return
		}
	}

	// 활성 프로파일 스펙 제공(FR-NSX-0104). 경로는 스펙 밖이며 API Guide 근거 확보 전까지 가정이다.
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/spec/openapi/") {
		e.serveSpec(w, strings.TrimPrefix(r.URL.Path, "/api/v1/spec/openapi/"))
		return
	}

	var spec *Spec
	var base, rel string
	for _, s := range e.specs {
		if p, ok := strings.CutPrefix(r.URL.Path, s.BasePath); ok && (p == "" || strings.HasPrefix(p, "/")) {
			spec, base, rel = s, s.BasePath, p
			break
		}
	}
	if spec == nil {
		apiError(w, http.StatusNotFound, "common-services", "The requested URI: "+r.URL.Path+" could not be found.")
		return
	}
	paths, ok := spec.Match(rel, r.URL.Query())
	if !ok {
		apiError(w, http.StatusNotFound, "common-services", "The requested URI: "+r.URL.Path+" could not be found.")
		return
	}
	op, ok := spec.Op(paths, r.Method)
	bodyRef := op.BodyRef
	if !ok {
		apiError(w, http.StatusMethodNotAllowed, "common-services", "Method "+r.Method+" is not supported for "+r.URL.Path)
		return
	}

	// 고정 응답 뷰(Manager API 조회)
	if r.Method == http.MethodGet && base == managerBase {
		switch rel {
		case "/node/version":
			writeJSON(w, http.StatusOK, e.prof.NSX.NodeVersion)
			return
		case "/cluster/status":
			writeJSON(w, http.StatusOK, map[string]any{
				"cluster_id":              uuidFrom(e.fix.Namespace, e.fix.Seed, "cluster"),
				"mgmt_cluster_status":     map[string]any{"status": "STABLE"},
				"control_cluster_status":  map[string]any{"status": "STABLE"},
				"detailed_cluster_status": map[string]any{"overall_status": "STABLE"},
			})
			return
		}
	}
	// 교차 시스템 관계 뷰·동작(WP 3.1·3.3·3.4)
	if base == managerBase && r.Method == http.MethodGet && strings.HasPrefix(rel, "/fabric/compute-managers/") && strings.HasSuffix(rel, "/status") {
		if strings.TrimSuffix(strings.TrimPrefix(rel, "/fabric/compute-managers/"), "/status") != e.ComputeManagerID() {
			apiError(w, http.StatusNotFound, "Inventory", "Compute manager not found.")
			return
		}
		e.computeManagerStatus(w)
		return
	}
	if base == managerBase && r.Method == http.MethodGet && (rel == "/alarms" || strings.HasPrefix(rel, "/alarms/")) {
		e.alarmsView(w, r, rel)
		return
	}
	if base == managerBase && r.Method == http.MethodPost && rel == "/fabric/virtual-machines" {
		if a := r.URL.Query().Get("action"); a == "update_tags" || a == "add_tags" || a == "remove_tags" {
			e.vmTagAction(w, r, spec, bodyRef, a)
			return
		}
	}
	if r.Method == http.MethodGet && rel == "/search/query" {
		e.search(w, r, base)
		return
	}
	if rel == "/search/dsl" {
		apiError(w, http.StatusNotImplemented, "Search", "DSL search is not supported by cloudinfra-sim.") // FR-NSX-0232 지원 문법 밖
		return
	}
	if m := groupMembersRe.FindStringSubmatch(rel); base == policyBase && r.Method == http.MethodGet && m != nil {
		e.groupMembers(w, r, m[1], m[2])
		return
	}
	if base == policyBase && r.Method == http.MethodGet && rel == "/infra/realized-state/realized-entities" {
		e.realizedEntities(w, r.URL.Query().Get("intent_path"))
		return
	}
	if base == policyBase && r.Method == http.MethodGet && strings.HasPrefix(rel, "/infra/realized-state/enforcement-points/") && strings.HasSuffix(rel, "/virtual-machines") {
		e.realizedVMs(w, r)
		return
	}
	if r.Method != http.MethodGet {
		e.mu.Lock()
		sys := e.derived[base+rel]
		e.mu.Unlock()
		if sys {
			// 가정: vCenter에서 파생한 시스템 소유 객체는 사용자가 바꿀 수 없다(상태·메시지 형식 미검증)
			apiError(w, http.StatusForbidden, "Policy", "The object "+rel+" is system owned and cannot be modified.")
			return
		}
	}
	if base == policyBase && rel == "/system-config/nsx-partial-patch-config" {
		e.partialPatchConfig(w, r, spec, bodyRef)
		return
	}
	if r.Method == http.MethodGet && base == policyBase {
		switch rel {
		case "/infra/realized-state/status":
			e.realizationStatus(w, r.URL.Query().Get("intent_path"))
			return
		case "/infra":
			writeJSON(w, http.StatusOK, map[string]any{"id": "infra", "resource_type": "Infra", "path": "/infra", "display_name": "infra", "_revision": 0})
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		e.get(w, r, base, rel, paths)
	case http.MethodPatch, http.MethodPut, http.MethodPost:
		e.write(w, r, spec, base, rel, op, paths)
	case http.MethodDelete:
		u, _ := r.Context().Value(userKey{}).(string)
		e.mu.Lock()
		b := e.newBatch(u)
		var status int
		var msg string
		if cur, ok := e.store[base+rel]; ok && cur["is_default"] == true && strings.HasPrefix(rel, "/infra/services/") {
			status, msg = http.StatusForbidden, "The default service "+rel+" cannot be modified or deleted." // 스펙 Service.is_default, 상태 가정
		} else {
			status, msg = b.remove(base, rel, base == policyBase && embeddedOf(rel) != nil)
		}
		if status == 0 {
			status, msg = b.commit()
		}
		e.mu.Unlock()
		if status == 0 && base == policyBase && relationInput(rel) {
			e.changed("cid=" + r.Header.Get("X-Request-ID"))
		}
		if status != 0 {
			apiError(w, status, "Policy", msg)
			return
		}
		w.WriteHeader(http.StatusOK) // 없는 객체 삭제도 200(가정, FR-NSX-0406 계약 확인 필요)
	default:
		apiError(w, http.StatusMethodNotAllowed, "common-services", "unsupported method")
	}
}

// authenticate는 Basic 인증(스펙 securityDefinitions) 또는 세션 쿠키 + XSRF를 확인한다(FR-API-0206~0208).
func (e *Engine) authenticate(w http.ResponseWriter, r *http.Request) (string, bool) {
	if u, p, ok := r.BasicAuth(); ok {
		if e.auth("nsx", u, p) {
			return u, true
		}
		apiError(w, http.StatusForbidden, "common-services", "The credentials were incorrect or the account specified has been locked.")
		return "", false
	}
	if c, err := r.Cookie("JSESSIONID"); err == nil {
		e.mu.Lock()
		s, ok := e.sessions[c.Value]
		// 유휴 만료(가상 시간, 프로파일 session_timeout_sec). 만료된 세션은 지운다(FR-API-0209, FR-BE-0607).
		if ok {
			if to := time.Duration(e.prof.NSX.Behavior.SessionTimeoutSec) * time.Second; to > 0 && e.now().Sub(s.last) > to {
				delete(e.sessions, c.Value)
				ok = false
			} else {
				s.last = e.now()
				e.sessions[c.Value] = s
			}
		}
		e.mu.Unlock()
		if ok {
			if r.Method != http.MethodGet && r.Header.Get("X-XSRF-TOKEN") != s.xsrf {
				apiError(w, http.StatusForbidden, "common-services", "XSRF token mismatch")
				return "", false
			}
			return s.user, true
		}
	}
	// 스펙 응답 목록에 401이 없으므로 미인증도 403으로 응답한다(BEHAVIOR_SPEC §6).
	apiError(w, http.StatusForbidden, "common-services", "The credentials were incorrect or the account specified has been locked.")
	return "", false
}

func (e *Engine) sessionCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusMethodNotAllowed, "common-services", "POST only")
		return
	}
	_ = r.ParseForm()
	user, pass := r.PostForm.Get("j_username"), r.PostForm.Get("j_password")
	if !e.auth("nsx", user, pass) {
		apiError(w, http.StatusForbidden, "common-services", "The credentials were incorrect or the account specified has been locked.")
		return
	}
	id, xsrf := newID(), newID()
	e.mu.Lock()
	e.sessions[id] = session{user: user, xsrf: xsrf, last: e.now()}
	e.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: id, Path: "/", Secure: true, HttpOnly: true})
	w.Header().Set("X-XSRF-TOKEN", xsrf)
	w.WriteHeader(http.StatusOK)
}

func (e *Engine) sessionDestroy(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("JSESSIONID"); err == nil {
		e.mu.Lock()
		delete(e.sessions, c.Value)
		e.mu.Unlock()
	}
	w.WriteHeader(http.StatusOK)
}

func (e *Engine) get(w http.ResponseWriter, r *http.Request, base, rel string, templates []string) {
	e.mu.Lock()
	obj, ok := e.store[base+rel]
	var items []map[string]any
	if ok && base == policyBase {
		obj = e.decorateLocked(rel, obj, true)
	}
	if !ok {
		// 목록 조회: 바로 아래 단계의 객체를 모은다.
		for _, k := range sortedKeys(e.store, base+rel+"/", 0) {
			it := e.store[k]
			if base == policyBase {
				it = e.decorateLocked(strings.TrimPrefix(k, base), it, false)
			}
			items = append(items, it)
		}
		if base == policyBase && strings.HasSuffix(rel, "/rules") {
			sortBySequence(items) // Rule 목록은 sequence_number 순서(가정)
		}
	}
	e.mu.Unlock()
	if ok {
		writeJSON(w, http.StatusOK, obj)
		return
	}
	if !isCollection(templates) {
		apiError(w, http.StatusNotFound, "Policy", "The path="+rel+" is invalid.")
		return
	}
	writeJSON(w, http.StatusOK, page(filterItems(items, r.URL.Query()), r))
}

// isCollection은 매칭된 스펙 템플릿의 마지막 세그먼트가 매개변수가 아니면 목록 경로로 본다.
func isCollection(templates []string) bool {
	for _, t := range templates {
		p, _, _ := strings.Cut(t, "?")
		if !strings.HasPrefix(p[strings.LastIndex(p, "/")+1:], "{") {
			return true
		}
	}
	return false
}

// page는 cursor·page_size 기반 페이지 조회다(FR-NSX-0414). cursor는 다음 시작 위치의 문자열이다.
func page(items []map[string]any, r *http.Request) map[string]any {
	q := r.URL.Query()
	start, size := cursorOf(q.Get("cursor"), q.Get("page_size"), len(items))
	end := min(start+size, len(items))
	out := map[string]any{"result_count": len(items), "results": nonNil(items[start:end]), "sort_by": "display_name", "sort_ascending": true}
	if end < len(items) {
		out["cursor"] = strconv.Itoa(end)
	}
	return out
}

// cursorOf는 cursor(다음 시작 위치 문자열)와 page_size(기본·최대 1000)를 해석한다.
func cursorOf(cursor, pageSize string, n int) (start, size int) {
	start, _ = strconv.Atoi(cursor)
	size, err := strconv.Atoi(pageSize)
	if err != nil || size <= 0 || size > 1000 {
		size = 1000
	}
	if start < 0 || start > n {
		start = n
	}
	return start, size
}

func nonNil(x []map[string]any) []map[string]any {
	if x == nil {
		return []map[string]any{}
	}
	return x
}

func (e *Engine) write(w http.ResponseWriter, r *http.Request, spec *Spec, base, rel string, op OpInfo, templates []string) {
	q := r.URL.Query()
	action := ""
	if _, cond, ok := strings.Cut(op.Path, "?"); ok {
		v, _ := url.ParseQuery(cond)
		action = v.Get("action")
	}
	if r.Method == http.MethodPost && action != "" && !(action == "revise" && base == policyBase) {
		// 스펙에 있지만 시뮬레이터가 행위를 구현하지 않은 동작(FR-API-0401): 객체로 저장하지 않고 501로 알린다(가정).
		apiError(w, http.StatusNotImplemented, "common-services", "Action "+action+" on "+r.URL.Path+" is not supported by cloudinfra-sim.")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "common-services", "Invalid JSON in request body")
		return
	}
	var defaults map[string]any
	if op.BodyRef != "" {
		sc := spec.Compile(op.BodyRef)
		if errs := sc.Validate(body, op.BodyRef, true); len(errs) > 0 {
			apiError(w, http.StatusBadRequest, "Policy", strings.Join(errs, "; "))
			return
		}
		defaults = defaultsFor(spec, op.BodyRef, body)
	}
	u, _ := r.Context().Value(userKey{}).(string)
	e.mu.Lock()
	partial := e.partialPatch
	if h := r.Header.Get("nsx-enable-partial-patch"); h != "" {
		partial = strings.EqualFold(h, "true") // 요청 헤더가 시스템 설정보다 우선(API Guide, PRD-1 §4.4)
	}
	// NSX 의미 계층 Fault(FR-FLT-0203·0204): Revision 충돌 강제, Realization 실패·정지(Timeout 재현, FR-NSX-0506)
	var rule fault.Rule
	var injected bool
	if e.Faults != nil {
		rule, injected = e.Faults.Apply(fault.LayerNSX, fault.Request{Product: "nsx", Method: r.Method, Path: r.URL.Path, Resource: rel, User: u, Now: e.now()})
	}
	if injected && rule.Action.Type == fault.ActionRevisionConfict {
		e.mu.Unlock()
		e.faultLog(r, rule, rel)
		apiError(w, http.StatusPreconditionFailed, "Policy", "The object was modified by somebody else.")
		return
	}
	b := e.newBatch(u)
	var status int
	var msg string
	switch {
	case action == "revise":
		status, msg = e.reviseLocked(b, spec, rel, body, q.Get("operation"), q.Get("anchor_path"))
	case r.Method == http.MethodPost && base == managerBase && isCollection(templates):
		// Manager API 생성(POST 컬렉션): 서버가 ID를 정한다(스펙 응답 201 + 객체).
		rel += "/" + newUUID()
		status, msg = e.applyWrite(b, spec, base, rel, body, false, false, defaults)
	case base == policyBase && rel == "/infra":
		// Hierarchical API(FR-NSX-0223): PATCH만 children을 처리한다(스펙: PUT은 "does not allow hierarchical updates").
		// _revision 검사는 enforce_revision_check=true일 때만 한다(스펙 쿼리 매개변수). Infra 자체 속성은 저장하지 않는다(가정).
		if r.Method == http.MethodPatch {
			b.noRev = !strings.EqualFold(q.Get("enforce_revision_check"), "true")
			children, _ := body["children"].([]any)
			status, msg = e.hierarchical(b, spec, "/infra", children, partial)
		}
	default:
		status, msg = e.applyWrite(b, spec, base, rel, body, r.Method == http.MethodPut, partial, defaults)
	}
	if status == 0 {
		status, msg = b.commit()
	}
	if status == 0 && base == policyBase && injected {
		e.realFault[rel] = realFault{hang: rule.Action.Type == fault.ActionRealizationHang, msg: rule.Action.Message}
		if !e.realFault[rel].hang {
			m := rule.Action.Message
			if m == "" {
				m = "Realization of " + rel + " failed on enforcement point default."
			}
			e.realFault[rel] = realFault{msg: m}
			e.alarmLocked(rel, m)
		}
	}
	var out map[string]any
	if status == 0 && op.RespRef != "" && r.Method != http.MethodPatch {
		if base == policyBase && rel == "/infra" {
			out = infraObject()
		} else if o, ok := e.store[base+rel]; ok {
			out = o
			if base == policyBase {
				out = e.decorateLocked(rel, o, true)
			}
		}
	}
	e.mu.Unlock()
	if injected {
		e.faultLog(r, rule, rel)
	}
	if status != 0 {
		apiError(w, status, "Policy", msg)
		return
	}
	if base == policyBase && relationInput(rel) {
		e.changed("cid=" + r.Header.Get("X-Request-ID")) // 관계 엔진: vCenter 네트워크 동기화(Segment) 등
	}
	// 성공 응답은 스펙 계약을 따른다: PATCH 200 본문 없음(Segment·Policy 공통), PUT·revise 200 + 객체, Manager POST 201 + 객체.
	if out != nil {
		writeJSON(w, op.RespCode, out)
		return
	}
	w.WriteHeader(op.RespCode)
}

func infraObject() map[string]any {
	return map[string]any{"id": "infra", "resource_type": "Infra", "path": "/infra", "display_name": "infra", "_revision": 0}
}

// deepMerge는 부분 PATCH 병합이다. 중첩 객체는 재귀 병합하고 배열·스칼라는 교체한다.
func deepMerge(cur, patch map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range cur {
		out[k] = v
	}
	for k, v := range patch {
		pm, pok := v.(map[string]any)
		cm, cok := out[k].(map[string]any)
		if pok && cok {
			out[k] = deepMerge(cm, pm)
		} else {
			out[k] = v
		}
	}
	return out
}

// partialPatchConfig는 GET/PATCH /policy/api/v1/system-config/nsx-partial-patch-config(스펙)다.
func (e *Engine) partialPatchConfig(w http.ResponseWriter, r *http.Request, spec *Spec, bodyRef string) {
	switch r.Method {
	case http.MethodGet:
		e.mu.Lock()
		v := e.partialPatch
		e.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"enable_partial_patch": v})
	case http.MethodPatch:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiError(w, http.StatusBadRequest, "Policy", "Invalid JSON in request body")
			return
		}
		if bodyRef != "" {
			if errs := spec.Compile(bodyRef).Validate(body, bodyRef, true); len(errs) > 0 {
				apiError(w, http.StatusBadRequest, "Policy", strings.Join(errs, "; "))
				return
			}
		}
		v, ok := body["enable_partial_patch"].(bool)
		if !ok {
			apiError(w, http.StatusBadRequest, "Policy", "Only boolean value is allowed for enable_partial_patch")
			return
		}
		e.mu.Lock()
		e.partialPatch = v
		e.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		apiError(w, http.StatusMethodNotAllowed, "Policy", "unsupported method")
	}
}

// serveSpec은 활성 프로파일의 스펙 파일을 그대로 제공한다(FR-NSX-0104).
func (e *Engine) serveSpec(w http.ResponseWriter, name string) {
	var file string
	switch name {
	case e.prof.NSX.SpecFiles["manager"], e.prof.NSX.SpecFiles["policy"]:
		file = name
	default:
		apiError(w, http.StatusNotFound, "common-services", "The requested URI could not be found.")
		return
	}
	b, err := os.ReadFile(filepath.Join(e.specDir, file))
	if err != nil {
		apiError(w, http.StatusNotFound, "common-services", "spec not available")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func newUUID() string {
	b := newID()
	return b[0:8] + "-" + b[8:12] + "-" + b[12:16] + "-" + b[16:20] + "-" + b[20:32]
}

func (e *Engine) realizationStatus(w http.ResponseWriter, intent string) {
	e.mu.Lock()
	status, _, ok := e.realizationLocked(intent, map[string]bool{})
	rev := e.store[policyBase+intent]["_revision"]
	e.mu.Unlock()
	if !ok {
		apiError(w, http.StatusNotFound, "Policy", "The path="+intent+" is invalid.") // 가정(FR-NSX-0508)
		return
	}
	// publish_status는 스펙 필수(ConsolidatedRealizedStatus). consolidated_status와의 대응은 가정(BEHAVIOR_SPEC §4.1).
	// intent_version: 실현 대상 intent의 _revision(재수정 시 증가, FR-NSX-0505).
	out := map[string]any{
		"intent_path":         intent,
		"publish_status":      publishOf(status),
		"consolidated_status": map[string]any{"consolidated_status": status},
	}
	if f, ok := rev.(float64); ok {
		out["intent_version"] = strconv.Itoa(int(f))
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// relationInput은 관계 조정의 입력이 되는 Policy 경로인지다: Segment(vCenter DVPG·Logical Switch·Port 파생),
// Transport Zone(사이트 아래), H-API 루트(/infra, 하위에 무엇이든 담을 수 있음). 그 밖의 쓰기(Group·Service·
// Security Policy·Tier-0/1 등)는 vCenter 쪽과 파생 객체를 바꾸지 않으므로 조정을 건너뛴다. 조정은 쓰기마다 교차 락
// 안에서 인벤토리를 훑으므로(VM 수에 비례) 불필요한 조정이 쓰기 처리량을 제한했다(Phase 5 벤치마크, R-09).
func relationInput(rel string) bool {
	return rel == "/infra" || strings.HasPrefix(rel, "/infra/segments") || strings.HasPrefix(rel, "/infra/sites/")
}
