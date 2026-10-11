package nsxspec

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Handler는 스펙 기반 범용 CRUD 핸들러다(PoC-08·09·10·16·17). 저장은 메모리 맵이다.
//
// 계약 근거(NSX-T 3.2.1 Policy 스펙):
//   - Segment PATCH/PUT/DELETE 응답 코드에 412가 있다(409 없음).
//   - `_revision` 설명: PUT은 현재 _revision을 포함해야 하며 없거나 오래되면 거부된다.
//   - Realization 상태: GET /infra/realized-state/status?intent_path= → ConsolidatedRealizedStatus,
//     consolidated_status ∈ {SUCCESS, IN_PROGRESS, ERROR, UNKNOWN, UNINITIALIZED}.
//
// 가정(미검증, 프로파일 behavior-rules로 확정 필요):
//   - PATCH에 _revision이 있고 오래되면 412. 없으면 검사하지 않는다.
//   - 오류 본문 형식, 삭제된 intent의 Realization 조회 응답(404).
//   - H-API는 전체 원자성(하나라도 실패하면 아무것도 반영하지 않음, FR-NSX-0225).
type Handler struct {
	Specs []*Spec
	// RealizationDelay는 변경 후 SUCCESS로 보고하기까지의 지연이다(FR-NSX-0502·0503, FR-BE-0606).
	RealizationDelay time.Duration

	mu       sync.Mutex
	store    map[string]map[string]any // 키: basePath + 상대 경로
	realized map[string]time.Time      // 키: intent path(/infra/...) → SUCCESS 예정 시각
}

func NewHandler(specs ...*Spec) *Handler {
	return &Handler{Specs: specs, store: map[string]map[string]any{}, realized: map[string]time.Time{}}
}

func nsxError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// 오류 본문 형식은 가정(미검증)이다. 프로파일 errors.yaml로 대체 예정(FR-NSX-0416).
	_ = json.NewEncoder(w).Encode(map[string]any{"httpStatus": http.StatusText(status), "error_message": msg})
}

type httpErr struct {
	status int
	msg    string
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var spec *Spec
	var rel string
	for _, s := range h.Specs {
		if p, ok := strings.CutPrefix(r.URL.Path, s.BasePath); ok {
			spec, rel = s, p
			break
		}
	}
	if spec == nil {
		nsxError(w, http.StatusNotFound, "unknown base path")
		return
	}
	specPaths, ok := spec.Match(rel, r.URL.Query())
	if !ok {
		nsxError(w, http.StatusNotFound, "path not in active profile spec")
		return
	}
	bodyRef, ok := spec.Operation(specPaths, r.Method)
	if !ok {
		nsxError(w, http.StatusMethodNotAllowed, "method not in active profile spec")
		return
	}

	// Realization 상태 조회
	if r.Method == http.MethodGet && rel == "/infra/realized-state/status" {
		h.realizationStatus(w, r.URL.Query().Get("intent_path"))
		return
	}

	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			nsxError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		// Hierarchical API (PoC-16)
		if r.Method == http.MethodPatch && rel == "/infra" {
			if e := h.hierarchical(spec, body); e != nil {
				nsxError(w, e.status, e.msg)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if bodyRef != "" {
			if errs := spec.Compile(bodyRef).Validate(body, bodyRef, true); len(errs) > 0 {
				nsxError(w, http.StatusBadRequest, strings.Join(errs, "; "))
				return
			}
		}
		h.mu.Lock()
		e := h.upsertLocked(spec.BasePath, rel, body, r.Method == http.MethodPut)
		h.mu.Unlock()
		if e != nil {
			nsxError(w, e.status, e.msg)
			return
		}
		w.WriteHeader(http.StatusOK) // Segment PATCH/PUT: 200, 본문 없음
	case http.MethodGet:
		h.mu.Lock()
		obj, ok := h.store[spec.BasePath+rel]
		h.mu.Unlock()
		if !ok {
			nsxError(w, http.StatusNotFound, "object not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj)
	case http.MethodDelete:
		h.mu.Lock()
		delete(h.store, spec.BasePath+rel)
		delete(h.realized, rel)
		h.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		nsxError(w, http.StatusMethodNotAllowed, "not implemented in PoC")
	}
}

// upsertLocked는 PATCH(병합)·PUT(교체)를 _revision 규칙과 함께 적용한다. h.mu를 잡은 상태에서 호출한다.
func (h *Handler) upsertLocked(base, rel string, body map[string]any, put bool) *httpErr {
	key := base + rel
	cur, exists := h.store[key]
	if exists {
		rev, given := body["_revision"]
		if put && !given {
			return &httpErr{http.StatusPreconditionFailed, "PUT requires current _revision"}
		}
		if given && rev != cur["_revision"] {
			return &httpErr{http.StatusPreconditionFailed, "object was modified by somebody else (stale _revision)"}
		}
	}
	next := map[string]any{}
	if exists && !put {
		for k, v := range cur {
			next[k] = v
		}
	}
	for k, v := range body {
		next[k] = v
	}
	if exists {
		next["_create_time"] = cur["_create_time"]
		next["_revision"] = cur["_revision"].(float64) + 1
	} else {
		next["_create_time"] = float64(time.Now().UnixMilli())
		next["_revision"] = float64(0)
	}
	next["_last_modified_time"] = float64(time.Now().UnixMilli())
	next["path"] = rel
	if base == "/policy/api/v1" {
		h.realized[rel] = time.Now().Add(h.RealizationDelay)
	}
	h.store[key] = next
	return nil
}

func (h *Handler) realizationStatus(w http.ResponseWriter, intent string) {
	h.mu.Lock()
	at, ok := h.realized[intent]
	h.mu.Unlock()
	if !ok {
		nsxError(w, http.StatusNotFound, "intent not found") // 가정: 삭제·미존재 intent 응답(FR-NSX-0508 확인 필요)
		return
	}
	status := "SUCCESS"
	if time.Now().Before(at) {
		status = "IN_PROGRESS"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"intent_path":         intent,
		"consolidated_status": map[string]any{"consolidated_status": status},
	})
}

// hierarchical은 PATCH /infra의 children(Child<Type> 래퍼)을 검증한 뒤 한 번에 반영한다.
// 지원 범위(PoC): ChildSegment, ChildTier1. marked_for_delete=true는 삭제.
func (h *Handler) hierarchical(spec *Spec, body map[string]any) *httpErr {
	children, _ := body["children"].([]any)
	type op struct {
		rel    string
		obj    map[string]any
		delete bool
	}
	parents := map[string]string{"Segment": "/infra/segments/", "Tier1": "/infra/tier-1s/"}
	var ops []op
	for i, c := range children {
		child, _ := c.(map[string]any)
		wrapper, _ := child["resource_type"].(string)
		kind := strings.TrimPrefix(wrapper, "Child")
		prefix, ok := parents[kind]
		if !ok {
			return &httpErr{http.StatusBadRequest, "children[" + strconv.Itoa(i) + "]: unsupported in PoC: " + wrapper}
		}
		obj, _ := child[kind].(map[string]any)
		id, _ := obj["id"].(string)
		if obj == nil || id == "" {
			return &httpErr{http.StatusBadRequest, "children[" + strconv.Itoa(i) + "]: missing " + kind + ".id"}
		}
		del, _ := child["marked_for_delete"].(bool)
		if !del {
			// H-API 예외: marked_for_delete는 스펙상 읽기전용이지만 H-API에서는 삭제 지시로 쓰인다.
			payload := map[string]any{}
			for k, v := range obj {
				if k != "marked_for_delete" {
					payload[k] = v
				}
			}
			if errs := spec.Compile(kind).Validate(payload, kind, true); len(errs) > 0 {
				return &httpErr{http.StatusBadRequest, "children[" + strconv.Itoa(i) + "]: " + strings.Join(errs, "; ")}
			}
			obj = payload
		}
		ops = append(ops, op{rel: prefix + id, obj: obj, delete: del})
	}

	// 원자성: 스냅샷을 만들고 하나라도 실패하면 되돌린다.
	h.mu.Lock()
	defer h.mu.Unlock()
	snapStore := make(map[string]map[string]any, len(h.store))
	for k, v := range h.store {
		snapStore[k] = v
	}
	snapReal := make(map[string]time.Time, len(h.realized))
	for k, v := range h.realized {
		snapReal[k] = v
	}
	for _, o := range ops {
		if o.delete {
			delete(h.store, spec.BasePath+o.rel)
			delete(h.realized, o.rel)
			continue
		}
		if e := h.upsertLocked(spec.BasePath, o.rel, o.obj, false); e != nil {
			h.store, h.realized = snapStore, snapReal
			return e
		}
	}
	return nil
}
