// Package nsx는 NSX Manager·Policy API 엔진이다. 스펙 기반 범용 핸들러(FR-NSX-0105 ~ FR-NSX-0107)와 행위 리소스를 제공한다.
// 활성 프로파일의 스펙만 로드하고(경로 색인), 스키마는 실제 호출 시점에 지연 컴파일·캐시한다(PoC-17에서 이관).
package nsx

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Spec은 Swagger 2.0 스펙 하나(Manager 또는 Policy)다. 경로·정의는 RawMessage로 보관한다.
type Spec struct {
	BasePath    string                     `json:"basePath"`
	Paths       map[string]json.RawMessage `json:"paths"`
	Definitions map[string]json.RawMessage `json:"definitions"`

	root    *node
	mu      sync.Mutex
	schemas map[string]*Schema  // 지연 컴파일 캐시
	byBody  map[string][]string // 요청 본문 타입 → 스펙 경로(H-API 하위 경로 계산)
}

// node는 경로 세그먼트 트리다. 같은 모양(매개변수 이름만 다른) 템플릿과 쿼리 조건부 템플릿
// (NSX 스펙의 '/x/{id}?action=start' 형식)을 한 노드에 모은다.
type node struct {
	lit   map[string]*node
	param *node
	paths map[string][]string // 쿼리 조건("" = 조건 없음, "action=start") → 스펙 경로 키 목록
}

// Load는 스펙 파일을 읽고 경로 색인만 만든다.
func Load(file string) (*Spec, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	s := &Spec{schemas: map[string]*Schema{}}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	s.root = &node{}
	for p := range s.Paths {
		n := s.root
		pathPart, query, _ := strings.Cut(p, "?")
		for _, seg := range split(pathPart) {
			if strings.HasPrefix(seg, "{") {
				if n.param == nil {
					n.param = &node{}
				}
				n = n.param
				continue
			}
			if n.lit == nil {
				n.lit = map[string]*node{}
			}
			c, ok := n.lit[seg]
			if !ok {
				c = &node{}
				n.lit[seg] = c
			}
			n = c
		}
		if n.paths == nil {
			n.paths = map[string][]string{}
		}
		n.paths[query] = append(n.paths[query], p)
	}
	return s, nil
}

func split(p string) []string { return strings.Split(strings.Trim(p, "/"), "/") }

// Match는 요청 경로(basePath 제외)와 쿼리에 해당하는 스펙 경로 키 목록을 찾는다.
// 리터럴 세그먼트를 매개변수보다 우선하고, 쿼리 조건이 일치하는 템플릿을 조건 없는 템플릿보다 우선한다.
func (s *Spec) Match(p string, q url.Values) ([]string, bool) {
	var walk func(n *node, segs []string) *node
	walk = func(n *node, segs []string) *node {
		if len(segs) == 0 {
			if len(n.paths) > 0 {
				return n
			}
			return nil
		}
		if c, ok := n.lit[segs[0]]; ok {
			if r := walk(c, segs[1:]); r != nil {
				return r
			}
		}
		if n.param != nil {
			return walk(n.param, segs[1:])
		}
		return nil
	}
	n := walk(s.root, split(p))
	if n == nil {
		return nil, false
	}
	// 조건(action=x&resource_type=y 등)을 모두 만족하는 템플릿 중 조건이 가장 많은 것을 고른다.
	var best []string
	bestN := -1
	for cond, keys := range n.paths {
		want, _ := url.ParseQuery(cond)
		match := true
		for k, vs := range want {
			if q.Get(k) != vs[0] {
				match = false
				break
			}
		}
		if match && len(want) > bestN {
			best, bestN = keys, len(want)
		}
	}
	return best, bestN >= 0
}

// OpInfo는 오퍼레이션 계약 중 행위에 필요한 부분이다: 요청 본문 스키마, 성공 응답 코드·본문 스키마, 스펙 경로 키.
type OpInfo struct {
	Path     string // 스펙 경로 키(쿼리 조건 포함, 예: /x/{id}?action=revise)
	BodyRef  string
	RespCode int    // 가장 작은 2xx 응답 코드(없으면 200)
	RespRef  string // 성공 응답 본문 스키마(없으면 본문 없음)
}

// Operation은 경로·메서드의 요청 본문 스키마 참조만 읽는다.
func (s *Spec) Operation(paths []string, method string) (bodyRef string, ok bool) {
	op, ok := s.Op(paths, method)
	return op.BodyRef, ok
}

// Op는 경로·메서드의 오퍼레이션 계약이다.
func (s *Spec) Op(paths []string, method string) (OpInfo, bool) {
	for _, p := range paths { // 같은 모양 템플릿에 메서드가 나뉘어 있을 수 있다
		if op, ok := s.operation(p, method); ok {
			return op, true
		}
	}
	return OpInfo{}, false
}

func (s *Spec) operation(path, method string) (OpInfo, bool) {
	var ops map[string]struct {
		Parameters []struct {
			In     string `json:"in"`
			Schema struct {
				Ref string `json:"$ref"`
			} `json:"schema"`
		} `json:"parameters"`
		Responses map[string]struct {
			Schema struct {
				Ref string `json:"$ref"`
			} `json:"schema"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(s.Paths[path], &ops); err != nil {
		return OpInfo{}, false
	}
	op, ok := ops[strings.ToLower(method)]
	if !ok {
		return OpInfo{}, false
	}
	out := OpInfo{Path: path, RespCode: 200}
	for _, p := range op.Parameters {
		if p.In == "body" {
			out.BodyRef = strings.TrimPrefix(p.Schema.Ref, "#/definitions/")
		}
	}
	best := 0
	for code, r := range op.Responses {
		c, err := strconv.Atoi(code)
		if err != nil || c < 200 || c > 299 || (best != 0 && c > best) {
			continue
		}
		best = c
		out.RespCode, out.RespRef = c, strings.TrimPrefix(r.Schema.Ref, "#/definitions/")
	}
	return out, true
}

// Schema는 검증에 필요한 부분만 컴파일한 스키마다.
type Schema struct {
	Type          string
	Enum          []any
	Required      []string
	ReadOnly      bool
	Default       any // 스펙 default(FR-NSX-0107)
	Properties    map[string]*Schema
	Items         *Schema
	MaxItems      int
	Discriminator string // 다형 타입(resource_type 등): 값이 정의 이름이면 그 하위 타입으로 검사한다
	ref           string
	spec          *Spec
}

type rawSchema struct {
	Ref           string                     `json:"$ref"`
	Type          string                     `json:"type"`
	Enum          []any                      `json:"enum"`
	Required      []string                   `json:"required"`
	ReadOnly      bool                       `json:"readonly"`
	ReadOnly2     bool                       `json:"readOnly"`
	Default       any                        `json:"default"`
	Properties    map[string]json.RawMessage `json:"properties"`
	Items         json.RawMessage            `json:"items"`
	AllOf         []json.RawMessage          `json:"allOf"`
	MaxItems      int                        `json:"maxItems"`
	Discriminator string                     `json:"discriminator"`
}

// Compile은 정의 이름의 스키마를 컴파일한다(캐시). 재귀 참조는 이름으로 끊는다.
func (s *Spec) Compile(name string) *Schema {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compileLocked(name)
}

func (s *Spec) compileLocked(name string) *Schema {
	if sc, ok := s.schemas[name]; ok {
		return sc
	}
	sc := &Schema{Properties: map[string]*Schema{}, ref: name, spec: s}
	s.schemas[name] = sc // 재귀 참조 대비 선등록
	raw, ok := s.Definitions[name]
	if ok {
		s.merge(sc, raw)
	}
	return sc
}

func (s *Spec) merge(sc *Schema, raw json.RawMessage) {
	var r rawSchema
	if json.Unmarshal(raw, &r) != nil {
		return
	}
	if r.Ref != "" {
		base := s.compileLocked(strings.TrimPrefix(r.Ref, "#/definitions/"))
		mergeFrom(sc, base)
		return
	}
	for _, a := range r.AllOf { // NSX 스펙은 allOf로 상속을 표현한다
		s.merge(sc, a)
	}
	if r.Type != "" {
		sc.Type = r.Type
	}
	sc.Enum = append(sc.Enum, r.Enum...)
	sc.Required = append(sc.Required, r.Required...)
	sc.ReadOnly = sc.ReadOnly || r.ReadOnly || r.ReadOnly2
	if r.Default != nil {
		sc.Default = r.Default
	}
	if r.MaxItems > 0 {
		sc.MaxItems = r.MaxItems
	}
	if r.Discriminator != "" {
		sc.Discriminator = r.Discriminator
	}
	for k, v := range r.Properties {
		ps := &Schema{Properties: map[string]*Schema{}, spec: s}
		s.merge(ps, v)
		sc.Properties[k] = ps
	}
	if len(r.Items) > 0 {
		sc.Items = &Schema{Properties: map[string]*Schema{}, spec: s}
		s.merge(sc.Items, r.Items)
	}
}

func mergeFrom(dst, src *Schema) {
	if src.Type != "" {
		dst.Type = src.Type
	}
	dst.Enum = append(dst.Enum, src.Enum...)
	dst.Required = append(dst.Required, src.Required...)
	dst.ReadOnly = dst.ReadOnly || src.ReadOnly
	if src.Default != nil {
		dst.Default = src.Default
	}
	for k, v := range src.Properties {
		dst.Properties[k] = v
	}
	if src.Items != nil {
		dst.Items = src.Items
	}
	if src.MaxItems > 0 {
		dst.MaxItems = src.MaxItems
	}
	if src.Discriminator != "" {
		dst.Discriminator = src.Discriminator
	}
	if dst.Type == "" && len(src.Properties) > 0 {
		dst.Type = "object"
	}
}

// CompiledCount는 지금까지 컴파일된 정의 수다.
func (s *Spec) CompiledCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.schemas)
}

// Validate는 요청 본문을 검사한다: 타입, enum, 필수 필드(최상위·다형 하위 타입), 읽기전용 필드 지정, maxItems.
// 다형 타입(discriminator)은 값의 resource_type 등이 스펙 정의 이름이면 그 하위 타입 스키마로 검사한다(예: Expression →
// Condition의 필수 key·value·member_type, ServiceEntry → L4PortSetServiceEntry의 l4_protocol).
func (sc *Schema) Validate(v any, path string, top bool) []string {
	if sub := sc.subtype(v); sub != nil {
		return sub.validate(v, path, true, !top)
	}
	return sc.validate(v, path, top, !top)
}

func (sc *Schema) subtype(v any) *Schema {
	m, ok := v.(map[string]any)
	if !ok || sc.Discriminator == "" || sc.spec == nil {
		return nil
	}
	name, _ := m[sc.Discriminator].(string)
	if name == "" || name == sc.ref {
		return nil
	}
	if _, ok := sc.spec.Definitions[name]; !ok {
		return nil
	}
	return sc.spec.Compile(name)
}

func (sc *Schema) validate(v any, path string, required, nested bool) []string {
	var errs []string
	if sc.ReadOnly && nested {
		return []string{path + ": read-only field must not be set"}
	}
	switch sc.Type {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return []string{path + ": expected object"}
		}
		if required {
			for _, r := range sc.Required {
				if _, ok := m[r]; !ok {
					errs = append(errs, path+"."+r+": required")
				}
			}
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if ps, ok := sc.Properties[k]; ok {
				errs = append(errs, ps.Validate(m[k], path+"."+k, false)...)
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return []string{path + ": expected array"}
		}
		if sc.MaxItems > 0 && len(a) > sc.MaxItems {
			errs = append(errs, fmt.Sprintf("%s: at most %d items allowed", path, sc.MaxItems))
		}
		if sc.Items != nil {
			for i, e := range a {
				errs = append(errs, sc.Items.Validate(e, path+"["+strconv.Itoa(i)+"]", false)...)
			}
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			return []string{path + ": expected string"}
		}
		if len(sc.Enum) > 0 {
			found := false
			for _, e := range sc.Enum {
				if e == s {
					found = true
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("%s: %q not in enum", path, s))
			}
		}
	case "integer", "number":
		if _, ok := v.(float64); !ok {
			return []string{path + ": expected number"}
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return []string{path + ": expected boolean"}
		}
	}
	return errs
}

// Defaults는 최상위 속성의 스펙 기본값이다(읽기전용 제외).
func (sc *Schema) Defaults() map[string]any {
	out := map[string]any{}
	for k, p := range sc.Properties {
		if p.Default != nil && !p.ReadOnly {
			out[k] = p.Default
		}
	}
	return out
}
