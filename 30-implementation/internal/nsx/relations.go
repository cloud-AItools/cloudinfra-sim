package nsx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"cloudinfra-sim/internal/fault"
	"cloudinfra-sim/internal/store"
)

// 교차 시스템 관계(WP 3.1 ~ 3.4)를 위한 NSX 엔진 API. 관계 엔진(internal/relations)이 쓴다.
//
// - Segment 변경은 OnChange로 알린다(쓰기 성공 후, 락 밖에서 호출).
// - vCenter 동기화(NSX DVPG)가 끝나기 전 Segment의 Realization은 IN_PROGRESS, 실패하면 ERROR다(FR-DM-0404, FR-NSX-0504).
// - vCenter에서 파생한 객체(Fabric VM·VIF·Segment Port·Discovered Node)는 시스템 소유(_system_owned)이며 저장소에
//   기록하지 않고 조정(reconcile) 때마다 다시 만든다(원본은 vcsim, FR-BE-0302). 사용자 쓰기는 거부한다.
// - Fabric VM Tag는 NSX가 소유하는 원본 상태이므로 저장소에 기록한다(FR-NSX-0609).

// SegmentInfo는 관계 엔진에 넘기는 Segment 요약이다.
type SegmentInfo struct {
	Path, ID, Name, UniqueID string
	TZPath                   string // 명시한 transport_zone_path(비면 기본 Overlay TZ 사용, 가정)
}

type syncInfo struct {
	pending bool
	err     string
}

// OnChange는 Policy 쓰기·삭제 성공 뒤 부를 함수를 등록한다. 등록하면 Segment 쓰기는 동기화 대기 상태가 된다.
func (e *Engine) OnChange(fn func(cause string)) {
	e.mu.Lock()
	e.onChange = append(e.onChange, fn)
	e.mu.Unlock()
}

func (e *Engine) changed(cause string) {
	e.mu.Lock()
	fns := append([]func(string){}, e.onChange...)
	e.mu.Unlock()
	for _, f := range fns {
		f(cause)
	}
}

// Segments는 현재 Segment 목록이다(경로 순).
func (e *Engine) Segments() []SegmentInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []SegmentInfo
	for _, k := range sortedKeys(e.store, policyBase+"/infra/segments/", 0) {
		o := e.store[k]
		s := SegmentInfo{Path: strings.TrimPrefix(k, policyBase)}
		s.ID, _ = o["id"].(string)
		s.Name, _ = o["display_name"].(string)
		s.UniqueID, _ = o["unique_id"].(string)
		s.TZPath, _ = o["transport_zone_path"].(string)
		out = append(out, s)
	}
	return out
}

// TransportZones는 Policy TZ 경로 → 유형(OVERLAY·VLAN)이다.
func (e *Engine) TransportZones() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]string{}
	for _, k := range sortedKeys(e.store, policyBase+"/infra/sites/default/enforcement-points/default/transport-zones/", 0) {
		t, _ := e.store[k]["tz_type"].(string)
		out[strings.TrimPrefix(k, policyBase)] = strings.TrimSuffix(t, "_BACKED")
	}
	return out
}

// TransportZoneIDs는 Policy TZ ID(Fixture id) → Manager TZ UUID(realization_id)다.
func (e *Engine) TransportZoneIDs() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tzIDsLocked()
}

func (e *Engine) tzIDsLocked() map[string]string {
	out := map[string]string{}
	for _, k := range sortedKeys(e.store, policyBase+"/infra/sites/default/enforcement-points/default/transport-zones/", 0) {
		o := e.store[k]
		id, _ := o["id"].(string)
		out[id], _ = o["realization_id"].(string)
	}
	return out
}

// ComputeManagerID는 등록된 vCenter Compute Manager ID다.
func (e *Engine) ComputeManagerID() string { return uuidFrom(e.fix.Namespace, e.fix.Seed, "cm") }

// SetSegmentSync는 Segment의 vCenter 동기화 결과다. errMsg가 비면 완료, 아니면 실패(Realization ERROR).
func (e *Engine) SetSegmentSync(path, errMsg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.store[policyBase+path]; ok {
		e.sync[path] = syncInfo{err: errMsg}
		if f, ok := e.realFault[path]; !(ok && !f.hang && errMsg == "") { // Fault로 주입한 실패 알람은 동기화 성공으로 닫지 않는다
			e.alarmLocked(path, errMsg)
		}
	}
}

// ReplaceDerived는 vCenter 파생 객체 전체를 바꾼다. 키는 basePath + 상대 경로다.
func (e *Engine) ReplaceDerived(objs map[string]map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k := range e.derived {
		delete(e.store, k)
	}
	e.derived = map[string]bool{}
	now := float64(e.now().UnixMilli())
	for k, o := range objs {
		if !strings.HasPrefix(k, policyBase) {
			// Manager 인벤토리 형식(DiscoveredResource 계열)은 Policy 공통 메타데이터가 없다(스펙). 동기화 시각만 둔다.
			if strings.HasPrefix(k, managerBase+"/fabric/") {
				o["_last_sync_time"] = now
			}
			e.store[k] = o
			e.derived[k] = true
			continue
		}
		o["_system_owned"] = true
		if _, ok := o["_create_time"]; !ok {
			o["_create_time"], o["_last_modified_time"] = now, now
			o["_create_user"], o["_last_modified_user"] = "system", "system"
			o["_revision"] = float64(0)
		}
		if strings.HasPrefix(k, policyBase) {
			rel := strings.TrimPrefix(k, policyBase)
			o["path"], o["relative_path"] = rel, o["id"]
			o["parent_path"] = rel[:strings.LastIndex(rel, "/")]
		}
		e.store[k] = o
		e.derived[k] = true
	}
	e.deriveLogicalLocked(now)
}

// deriveLogicalLocked는 Policy Segment·Segment Port가 Manager API에 실현된 Logical Switch·Logical Port다(FR-NSX-0207).
// Policy가 만든 Manager 객체는 보호(_protection REQUIRE_OVERRIDE)·생성자 nsx_policy이며 사용자 쓰기는 거부한다(시스템 소유로
// 처리, 거부 상태 403은 가정). ID는 Segment·Port의 unique_id(Realization realized-entity ID와 같음). Overlay Segment의
// replication_mode MTEP, switch_type DEFAULT, tags의 policyPath 표기는 가정이다.
func (e *Engine) deriveLogicalLocked(now float64) {
	tzIDs := e.tzIDsLocked()
	defaultTZ, tzType := "", map[string]string{}
	for _, tz := range e.fix.NSX.TransportZones {
		tzType[tz.ID] = tz.TransportType
		if defaultTZ == "" && tz.TransportType == "OVERLAY" {
			defaultTZ = tz.ID
		}
	}
	meta := func(o map[string]any, path string) map[string]any {
		o["_create_user"], o["_last_modified_user"] = "nsx_policy", "nsx_policy"
		o["_create_time"], o["_last_modified_time"], o["_revision"] = now, now, float64(0)
		o["_protection"] = "REQUIRE_OVERRIDE"
		o["tags"] = []any{map[string]any{"scope": "policyPath", "tag": path}}
		return o
	}
	lsOf := map[string]string{}
	for _, k := range sortedKeys(e.store, policyBase+"/infra/segments/", 0) {
		seg := e.store[k]
		path := strings.TrimPrefix(k, policyBase)
		uid, _ := seg["unique_id"].(string)
		tz := defaultTZ
		if p, _ := seg["transport_zone_path"].(string); p != "" {
			tz = p[strings.LastIndex(p, "/")+1:]
		}
		if uid == "" || tzIDs[tz] == "" {
			continue
		}
		lsOf[path] = uid
		admin, _ := seg["admin_state"].(string)
		if admin == "" {
			admin = "UP"
		}
		ls := meta(map[string]any{"id": uid, "resource_type": "LogicalSwitch", "display_name": seg["display_name"],
			"transport_zone_id": tzIDs[tz], "admin_state": admin, "switch_type": "DEFAULT"}, path)
		if tzType[tz] == "OVERLAY" {
			ls["replication_mode"] = "MTEP"
		}
		key := managerBase + "/logical-switches/" + uid
		e.store[key], e.derived[key] = ls, true
	}
	for _, k := range sortedKeys(e.store, policyBase+"/infra/segments/", 2) {
		if !strings.Contains(k, "/ports/") {
			continue
		}
		port := e.store[k]
		path := strings.TrimPrefix(k, policyBase)
		ls := lsOf[path[:strings.Index(path, "/ports/")]]
		uid, _ := port["unique_id"].(string)
		if ls == "" || uid == "" {
			continue
		}
		admin, _ := port["admin_state"].(string)
		if admin == "" {
			admin = "UP"
		}
		lp := meta(map[string]any{"id": uid, "resource_type": "LogicalPort", "display_name": port["display_name"],
			"logical_switch_id": ls, "admin_state": admin}, path)
		if att, ok := port["attachment"].(map[string]any); ok {
			if id, _ := att["id"].(string); id != "" {
				lp["attachment"] = map[string]any{"attachment_type": "VIF", "id": id}
			}
		}
		key := managerBase + "/logical-ports/" + uid
		e.store[key], e.derived[key] = lp, true
	}
}

// VMTags는 NSX 소유 Fabric VM Tag다(external_id → tags).
func (e *Engine) VMTags() map[string][]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string][]any{}
	for k, o := range e.store {
		if ext, ok := strings.CutPrefix(k, managerBase+vmTagPrefix); ok {
			t, _ := o["tags"].([]any)
			out[ext] = t
		}
	}
	return out
}

const vmTagPrefix = "/fabric/vm-tags/" // 내부 저장 키(스펙 경로 아님, 목록 조회에 나오지 않음)

// vmTagAction은 POST /api/v1/fabric/virtual-machines?action=update_tags|add_tags|remove_tags다(FR-NSX-0226·0609).
func (e *Engine) vmTagAction(w http.ResponseWriter, r *http.Request, spec *Spec, bodyRef, action string) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "common-services", "Invalid JSON in request body")
		return
	}
	if bodyRef != "" {
		if errs := spec.Compile(bodyRef).Validate(body, bodyRef, true); len(errs) > 0 {
			apiError(w, http.StatusBadRequest, "Inventory", strings.Join(errs, "; "))
			return
		}
	}
	ext, _ := body["external_id"].(string)
	tags, _ := body["tags"].([]any)
	e.mu.Lock()
	if _, ok := e.store[managerBase+"/fabric/virtual-machines/"+ext]; !ok {
		e.mu.Unlock()
		apiError(w, http.StatusNotFound, "Inventory", "Virtual machine with external_id "+ext+" not found.") // 가정
		return
	}
	key := managerBase + vmTagPrefix + ext
	cur, _ := e.store[key]["tags"].([]any)
	next := tagOp(action, cur, tags)
	obj := map[string]any{"tags": next}
	err := e.persist(func(tx *store.NSXTx) error { return tx.Put(key, obj) })
	if err == nil {
		e.store[key] = obj
	}
	e.mu.Unlock()
	if err != nil {
		apiError(w, http.StatusInternalServerError, "Inventory", "storage error")
		return
	}
	e.changed("cid=" + r.Header.Get("X-Request-ID")) // 파생 Fabric VM의 tags를 다시 만든다
	w.WriteHeader(http.StatusNoContent)
}

func tagKey(t any) string {
	m, _ := t.(map[string]any)
	s, _ := m["scope"].(string)
	g, _ := m["tag"].(string)
	return s + "\x00" + g
}

// tagOp: update는 교체, add는 (scope, tag) 기준 추가, remove는 같은 (scope, tag) 제거.
func tagOp(action string, cur, tags []any) []any {
	switch action {
	case "update_tags":
		return append([]any{}, tags...)
	case "add_tags":
		out := append([]any{}, cur...)
		seen := map[string]bool{}
		for _, t := range cur {
			seen[tagKey(t)] = true
		}
		for _, t := range tags {
			if !seen[tagKey(t)] {
				out = append(out, t)
				seen[tagKey(t)] = true
			}
		}
		return out
	default: // remove_tags
		drop := map[string]bool{}
		for _, t := range tags {
			drop[tagKey(t)] = true
		}
		out := []any{}
		for _, t := range cur {
			if !drop[tagKey(t)] {
				out = append(out, t)
			}
		}
		return out
	}
}

// realizationLocked는 intent의 상태다: 동기화 실패 → ERROR, 지연·동기화 대기·의존 리소스 미실현 → IN_PROGRESS.
// 의존(FR-NSX-0507, 가정): connectivity_path·tier0_path·tier1_path·segment_path가 가리키는 intent가 SUCCESS여야 한다.
func (e *Engine) realizationLocked(path string, seen map[string]bool) (status, errMsg string, ok bool) {
	at, exists := e.realized[path]
	if !exists {
		return "", "", false
	}
	if f, ok := e.realFault[path]; ok {
		if f.hang {
			return "IN_PROGRESS", "", true // 실현이 끝나지 않는다(클라이언트 Timeout 재현)
		}
		return "ERROR", f.msg, true
	}
	if s := e.sync[path]; s.err != "" {
		return "ERROR", s.err, true
	} else if s.pending || e.now().Before(at) {
		return "IN_PROGRESS", "", true
	}
	if seen[path] {
		return "SUCCESS", "", true
	}
	seen[path] = true
	obj := e.store[policyBase+path]
	for _, f := range []string{"connectivity_path", "tier0_path", "tier1_path", "segment_path"} {
		dep, _ := obj[f].(string)
		if dep == "" {
			continue
		}
		if st, _, ok := e.realizationLocked(dep, seen); !ok || st != "SUCCESS" {
			return "IN_PROGRESS", "", true
		}
	}
	return "SUCCESS", "", true
}

func publishOf(status string) string {
	switch status {
	case "SUCCESS":
		return "REALIZED"
	case "ERROR":
		return "ERROR"
	}
	return "UNREALIZED"
}

// realizedEntities는 GET /infra/realized-state/realized-entities?intent_path=다(FR-NSX-0509, 오류 정보 FR-NSX-0504).
func (e *Engine) realizedEntities(w http.ResponseWriter, intent string) {
	e.mu.Lock()
	status, errMsg, ok := e.realizationLocked(intent, map[string]bool{})
	obj := e.store[policyBase+intent]
	e.mu.Unlock()
	if !ok {
		apiError(w, http.StatusNotFound, "Policy", "The path="+intent+" is invalid.")
		return
	}
	id, _ := obj["unique_id"].(string)
	ent := map[string]any{
		"resource_type": "GenericPolicyRealizedResource", "id": id, "display_name": obj["display_name"],
		"path": intent + "/realized-entity", "intent_paths": []string{intent}, "entity_type": "RealizedLogicalSwitch",
		"enforcement_point_path": "/infra/sites/default/enforcement-points/default", "site_path": "/infra/sites/default",
		"state": publishOf(status), "publish_status": publishOf(status), "realization_specific_identifier": id,
		"runtime_status": map[string]string{"SUCCESS": "UP", "ERROR": "DOWN"}[status],
	}
	if errMsg != "" {
		ent["publish_status_error"] = errMsg
		ent["alarms"] = []map[string]any{{"message": errMsg, "source_reference": intent, "error_details": map[string]any{"error_message": errMsg}}}
	}
	if ent["runtime_status"] == "" {
		ent["runtime_status"] = "UNKNOWN"
	}
	writeJSON(w, http.StatusOK, map[string]any{"result_count": 1, "results": []any{ent}})
}

// realizedVMs는 GET /infra/realized-state/enforcement-points/{ep}/virtual-machines다(FR-NSX-0228).
func (e *Engine) realizedVMs(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	var items []map[string]any
	for _, k := range sortedKeys(e.store, managerBase+"/fabric/virtual-machines/", 0) {
		vm := e.store[k]
		items = append(items, realizedVM(vm))
	}
	e.mu.Unlock()
	writeJSON(w, http.StatusOK, page(items, r))
}

// computeManagerStatus는 GET /api/v1/fabric/compute-managers/{id}/status다(FR-NSX-0601).
func (e *Engine) computeManagerStatus(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"connection_status": "UP", "registration_status": "REGISTERED",
		"version": e.prof.VCenter.Version, "last_sync_time": e.now().UnixMilli(),
	})
}

// filterItems는 목록 조회의 같음 조건 필터다(FR-NSX-0413): 항목 최상위 문자열 필드와 같은 이름의 쿼리 매개변수.
var listParams = map[string]bool{"cursor": true, "page_size": true, "sort_by": true, "sort_ascending": true, "included_fields": true,
	"include_mark_for_delete_objects": true, "intent_path": true}

func filterItems(items []map[string]any, q map[string][]string) []map[string]any {
	keys := make([]string, 0, len(q))
	for k := range q {
		if !listParams[k] {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return items
	}
	sort.Strings(keys)
	var out []map[string]any
	for _, it := range items {
		match := true
		for _, k := range keys {
			if v, ok := it[k].(string); ok && v != q[k][0] {
				match = false
			}
		}
		if match {
			out = append(out, it)
		}
	}
	return out
}

// 알람(FR-MET-0201 'NSX Policy Realization Failed', FR-MET-0202 NSX Alarm API 계약): 동기화 실패로 Realization이
// ERROR가 되면 OPEN 알람을 만들고, 회복하면 RESOLVED로 바꾼다. feature_name·event_type 값은 가정이다.
func (e *Engine) alarmLocked(path, errMsg string) {
	now := e.now().UnixMilli()
	a, exists := e.alarms[path]
	if errMsg != "" {
		if exists && a["status"] == "OPEN" {
			a["last_reported_time"], a["description"] = now, errMsg
			return
		}
		obj := e.store[policyBase+path]
		e.alarms[path] = map[string]any{
			"id": uuidFrom(e.fix.Namespace, e.fix.Seed, "alarm"+path+fmt.Sprint(now)), "resource_type": "Alarm",
			"feature_name": "realization", "feature_display_name": "Realization",
			"event_type": "realization_failure", "event_type_display_name": "Realization Failure",
			"summary": "Policy intent " + path + " failed to realize.", "description": errMsg,
			"recommended_action": "Check the vCenter host switch prepared for NSX and the transport zone of the segment.",
			"status":             "OPEN", "severity": "HIGH", "entity_id": obj["unique_id"], "entity_resource_type": "Segment",
			"alarm_source_type": "INTENT_PATH", "alarm_source": []string{path},
			"node_id": uuidFrom(e.fix.Namespace, e.fix.Seed, "cluster"), "node_resource_type": "ClusterNodeConfig", "last_reported_time": now,
		}
		return
	}
	if exists && a["status"] == "OPEN" {
		a["status"], a["resolved_time"], a["resolved_by"] = "RESOLVED", now, "system"
	}
}

// alarmsView는 GET /api/v1/alarms(목록)·/alarms/{id}다.
func (e *Engine) alarmsView(w http.ResponseWriter, r *http.Request, rel string) {
	e.mu.Lock()
	var items []map[string]any
	for _, a := range e.alarms {
		items = append(items, a)
	}
	e.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["id"]) < fmt.Sprint(items[j]["id"]) })
	if id, ok := strings.CutPrefix(rel, "/alarms/"); ok {
		for _, a := range items {
			if a["id"] == id {
				writeJSON(w, http.StatusOK, a)
				return
			}
		}
		apiError(w, http.StatusNotFound, "Alarms", "Alarm "+id+" not found.")
		return
	}
	writeJSON(w, http.StatusOK, page(filterItems(items, r.URL.Query()), r))
}

// realizedVM은 Fabric VM에서 RealizedVirtualMachine을 만든다(state 필수, tags는 있을 때만).
func realizedVM(vm map[string]any) map[string]any {
	ext, _ := vm["external_id"].(string)
	out := map[string]any{
		"resource_type": "RealizedVirtualMachine", "id": ext, "display_name": vm["display_name"], "state": "REALIZED",
		"host_id": vm["host_id"], "local_id_on_host": vm["local_id_on_host"], "power_state": vm["power_state"],
		"compute_ids": vm["compute_ids"], "path": "/infra/realized-state/enforcement-points/default/virtual-machines/" + ext,
	}
	if t, ok := vm["tags"].([]any); ok && len(t) > 0 {
		out["tags"] = t
	}
	return out
}

// ExpireSession은 JSESSIONID 세션을 즉시 만료시킨다(세션 계층 Fault, FR-FLT-0203).
func (e *Engine) ExpireSession(id string) {
	e.mu.Lock()
	delete(e.sessions, id)
	e.mu.Unlock()
}

type userKey struct{}

// realFault는 Fault로 주입한 Realization 상태다: 실패(ERROR, 메시지) 또는 정지(IN_PROGRESS 유지).
type realFault struct {
	hang bool
	msg  string
}

func (e *Engine) faultLog(r *http.Request, rule fault.Rule, rel string) {
	if e.Log != nil {
		e.Log.Warn("fault injected", "cid", r.Header.Get("X-Request-ID"), "rule", rule.ID, "layer", rule.Layer, "action", rule.Action.Type, "intent", rel)
	}
}
