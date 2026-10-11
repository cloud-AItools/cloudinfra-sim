package vcenter

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/vmware/govmomi/simulator"
)

// Content Library /api(FR-VC-0206, WP 4.11). vcsim vAPI 시뮬레이터의 구형 /rest Content Library 구현을 재사용하고
// (NFR-ARC-0104) /api 요청·응답 형식으로 바꾼다(R-17과 같은 방식, PoC-02).
//
// 계약 근거(SRC-VAPI-SDK-7032, Phase 4b 추출 확장): content_client·content/library_client의 OperationRestMetadata
// — URL 템플릿, request_body_parameter(본문 = 해당 매개변수 구조 그대로), dispatch_parameters(?action=find), 출력 타입.
// vapi_runtime 2.34.0: 비구조 출력은 /api에서 감싸지 않는다, DELETE 성공 → 204, 그 밖의 성공 기본 200.
// 가정: create 성공 201(세션 생성과 같은 가정), 구형 → /api 오류 변환(type 마지막 요소를 error_type으로).
//
// 범위: library(list·get·find·update), local-library(list·get·create·update·delete), library/item(list·get·find·create·
// update·delete). 구독 라이브러리·파일 업로드(update-session)·OVF 배포(/api/vcenter/ovf)는 범위 밖이다.

type contentOp struct {
	method, pattern string // /api 패턴(Go ServeMux)
	legacy          string // 구형 서비스 경로(/rest 뒤)
	idVar           string // 경로 변수 → 구형 /id:<값>
	body            string // request_body_parameter
	action          string // dispatch action
	create          bool
	model           map[string]bool // 출력 구조의 계약 필드(나머지는 지운다)
}

// 계약 필드(SRC-VAPI-SDK-7032 com.vmware.content.LibraryModel·library.ItemModel). vcsim은 내부 필드(Item·Subs·VMTX)와
// 7.0.3 계약에 없는 필드(state_info 등)를 함께 직렬화한다(R-38) → 계약 필드만 남긴다.
var (
	libraryModel = fieldSet("creation_time", "description", "id", "last_modified_time", "last_sync_time", "name", "optimization_info",
		"publish_info", "security_policy_id", "server_guid", "storage_backings", "subscription_info", "type", "unset_security_policy_id", "version")
	itemModel = fieldSet("cached", "certificate_verification_info", "content_version", "creation_time", "description", "id",
		"last_modified_time", "last_sync_time", "library_id", "metadata_version", "name", "security_compliance", "size", "source_id", "type", "version")
)

func fieldSet(fs ...string) map[string]bool {
	m := map[string]bool{}
	for _, f := range fs {
		m[f] = true
	}
	return m
}

var contentOps = []contentOp{
	{method: "GET", pattern: "/api/content/library", legacy: "/com/vmware/content/library"},
	{method: "GET", pattern: "/api/content/library/{id}", legacy: "/com/vmware/content/library", idVar: "id", model: libraryModel},
	{method: "POST", pattern: "/api/content/library", legacy: "/com/vmware/content/library", body: "spec", action: "find"},
	{method: "PATCH", pattern: "/api/content/library/{id}", legacy: "/com/vmware/content/library", idVar: "id", body: "update_spec"},
	{method: "GET", pattern: "/api/content/local-library", legacy: "/com/vmware/content/local-library"},
	{method: "GET", pattern: "/api/content/local-library/{id}", legacy: "/com/vmware/content/local-library", idVar: "id", model: libraryModel},
	{method: "POST", pattern: "/api/content/local-library", legacy: "/com/vmware/content/local-library", body: "create_spec", create: true},
	{method: "PATCH", pattern: "/api/content/local-library/{id}", legacy: "/com/vmware/content/local-library", idVar: "id", body: "update_spec"},
	{method: "DELETE", pattern: "/api/content/local-library/{id}", legacy: "/com/vmware/content/local-library", idVar: "id"},
	{method: "GET", pattern: "/api/content/library/item", legacy: "/com/vmware/content/library/item"},
	{method: "GET", pattern: "/api/content/library/item/{id}", legacy: "/com/vmware/content/library/item", idVar: "id", model: itemModel},
	{method: "POST", pattern: "/api/content/library/item", legacy: "/com/vmware/content/library/item", body: "create_spec", create: true},
	{method: "POST", pattern: "/api/content/library/item", legacy: "/com/vmware/content/library/item", body: "spec", action: "find"},
	{method: "PATCH", pattern: "/api/content/library/item/{id}", legacy: "/com/vmware/content/library/item", idVar: "id", body: "update_spec"},
	{method: "DELETE", pattern: "/api/content/library/item/{id}", legacy: "/com/vmware/content/library/item", idVar: "id"},
}

// contentRoutes는 Content Library /api 경로를 등록한다. 같은 메서드·패턴에 action 분기가 있으면 한 핸들러에서 고른다.
func (v *VCenter) contentRoutes(mux *http.ServeMux, m *simulator.Model, authed func(func(http.ResponseWriter, *http.Request)) http.HandlerFunc) {
	byKey := map[string][]contentOp{}
	var keys []string
	for _, op := range contentOps {
		k := op.method + " " + op.pattern
		if _, ok := byKey[k]; !ok {
			keys = append(keys, k)
		}
		byKey[k] = append(byKey[k], op)
	}
	for _, k := range keys {
		ops := byKey[k]
		mux.HandleFunc(k, authed(func(w http.ResponseWriter, r *http.Request) {
			action := r.URL.Query().Get("action")
			for _, op := range ops {
				if op.action == action {
					v.contentCall(w, r, m, op)
					return
				}
			}
			restError(w, "OPERATION_NOT_FOUND", "vapi.rest.operation.not_found", "Operation not found: "+r.Method+" "+r.URL.Path)
		}))
	}
}

func (v *VCenter) contentCall(w http.ResponseWriter, r *http.Request, m *simulator.Model, op contentOp) {
	path := "/rest" + op.legacy
	if op.idVar != "" {
		path += "/id:" + url.PathEscape(r.PathValue(op.idVar))
	}
	q := url.Values{}
	for k, vals := range r.URL.Query() {
		if k != "action" && k != "client_token" {
			q[k] = vals
		}
	}
	if op.action != "" {
		q.Set("~action", op.action)
	}
	var body io.Reader
	if op.body != "" {
		var in any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			restError(w, "INVALID_ARGUMENT", "vapi.bindings.typeconverter.invalid.json", "Invalid JSON in request body: "+err.Error())
			return
		}
		wrapped := map[string]any{op.body: in}
		if t := r.URL.Query().Get("client_token"); t != "" {
			wrapped["client_token"] = t
		}
		b, _ := json.Marshal(wrapped)
		body = bytes.NewReader(b)
	}
	u := path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req := httptest.NewRequest(r.Method, u, body)
	req.Header.Set("vmware-api-session-id", r.Header.Get("vmware-api-session-id"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	m.Service.ServeMux.ServeHTTP(rec, req)
	if rec.Code/100 != 2 {
		legacyToAPIError(w, rec)
		return
	}
	var out struct {
		Value json.RawMessage `json:"value"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	switch {
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case len(out.Value) == 0:
		w.WriteHeader(http.StatusOK) // void(update)
	default:
		code := http.StatusOK
		if op.create {
			code = http.StatusCreated
		}
		val := out.Value
		if op.model != nil {
			val = v.contentModel(val, op.model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(val)
	}
}

// contentModel은 계약 필드만 남기고 *_time 값을 가상 시각(FR-BE-0611)의 UTC 밀리초 RFC 3339로 바꾼다.
func (v *VCenter) contentModel(raw json.RawMessage, allowed map[string]bool) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	for k, val := range obj {
		if !allowed[k] {
			delete(obj, k)
			continue
		}
		if s, ok := val.(string); ok && strings.HasSuffix(k, "_time") {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				if v.Clock != nil {
					t = v.Clock.ToVirtual(t)
				}
				obj[k] = t.UTC().Format("2006-01-02T15:04:05.000Z")
			}
		}
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return b
}

// legacyToAPIError는 구형 오류({"type":"com.vmware.vapi.std.errors.not_found","value":{...}})를 /api 오류로 바꾼다.
func legacyToAPIError(w http.ResponseWriter, rec *httptest.ResponseRecorder) {
	var e struct {
		Type  string `json:"type"`
		Value struct {
			Messages []any `json:"messages"`
		} `json:"value"`
	}
	errType := "NOT_FOUND"
	if json.Unmarshal(rec.Body.Bytes(), &e) == nil && e.Type != "" {
		errType = strings.ToUpper(e.Type[strings.LastIndex(e.Type, ".")+1:])
	} else if rec.Code != http.StatusNotFound {
		errType = "INTERNAL_SERVER_ERROR"
	}
	if _, ok := errorStatus[errType]; !ok {
		errType = "INTERNAL_SERVER_ERROR"
	}
	msg := strings.ToLower(strings.ReplaceAll(errType, "_", " "))
	restError(w, errType, "com.vmware.vapi.std.errors."+strings.ToLower(errType), "The operation failed: "+msg+".")
}
