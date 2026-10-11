// Package nsxcheck은 NSX 응답 본문을 공식 OpenAPI 스펙(SRC-NSX-321: NSX-T 3.2.1 nsx_api.json·nsx_policy_api.json)의
// 응답 스키마에 대조한다(PRD-2 TR-COV-0301, FR-API-0101·0102). 구현의 요청 검증기와 독립적으로 스펙 파일을 직접 읽는다.
//
// 검사: 타입(object·array·string·integer·number·boolean), enum, 모든 단계의 required, 스키마에 없는 속성(임의 추가·
// 비표준화 탐지), $ref·allOf 해석. 오류 응답(4xx·5xx)은 스펙에 상태별 스키마가 없으므로 ApiError 정의에 대조한다.
package nsxcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Spec struct {
	BasePath    string                                `json:"basePath"`
	Paths       map[string]map[string]json.RawMessage `json:"paths"`
	Definitions map[string]json.RawMessage            `json:"definitions"`
	resolved    map[string]*schema
}

type schema struct {
	Type          string
	Enum          []any
	Required      []string
	Properties    map[string]*schema
	Items         *schema
	Additional    bool   // additionalProperties 허용(맵 형식)
	Discriminator string // 다형 타입: 이 속성 값이 실제 정의 이름
}

func Load(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &Spec{resolved: map[string]*schema{}}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	return s, nil
}

type raw struct {
	Ref        string                     `json:"$ref"`
	Type       string                     `json:"type"`
	Enum       []any                      `json:"enum"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
	Items      json.RawMessage            `json:"items"`
	AllOf      []json.RawMessage          `json:"allOf"`
	AddProps   json.RawMessage            `json:"additionalProperties"`
	Discrim    string                     `json:"discriminator"`
}

func (s *Spec) def(name string) *schema {
	if sc, ok := s.resolved[name]; ok {
		return sc
	}
	sc := &schema{Properties: map[string]*schema{}}
	s.resolved[name] = sc // 재귀 대비 선등록
	if r, ok := s.Definitions[name]; ok {
		s.merge(sc, r)
	}
	return sc
}

func (s *Spec) merge(sc *schema, r json.RawMessage) {
	var x raw
	if json.Unmarshal(r, &x) != nil {
		return
	}
	if x.Ref != "" {
		d := s.def(strings.TrimPrefix(x.Ref, "#/definitions/"))
		if sc.Type == "" {
			sc.Type = d.Type
		}
		sc.Enum = append(sc.Enum, d.Enum...)
		sc.Required = append(sc.Required, d.Required...)
		for k, v := range d.Properties {
			sc.Properties[k] = v
		}
		if sc.Items == nil {
			sc.Items = d.Items
		}
		sc.Additional = sc.Additional || d.Additional
		if sc.Discriminator == "" {
			sc.Discriminator = d.Discriminator
		}
	}
	for _, a := range x.AllOf {
		s.merge(sc, a)
	}
	if x.Discrim != "" {
		sc.Discriminator = x.Discrim
	}
	if x.Type != "" {
		sc.Type = x.Type
	}
	sc.Enum = append(sc.Enum, x.Enum...)
	sc.Required = append(sc.Required, x.Required...)
	for k, v := range x.Properties {
		p := &schema{Properties: map[string]*schema{}}
		s.merge(p, v)
		sc.Properties[k] = p
	}
	if len(x.Items) > 0 {
		sc.Items = &schema{Properties: map[string]*schema{}}
		s.merge(sc.Items, x.Items)
	}
	if len(x.AddProps) > 0 && string(x.AddProps) != "false" {
		sc.Additional = true
	}
	if sc.Type == "" && len(sc.Properties) > 0 {
		sc.Type = "object"
	}
}

// ResponseSchema는 경로 템플릿(basePath 제외)·메서드·상태의 응답 스키마 정의다. 본문이 없으면 nil.
func (s *Spec) responseSchema(template, method string, status int) (*schema, error) {
	ops, ok := s.Paths[template]
	if !ok {
		return nil, fmt.Errorf("path %s not in spec", template)
	}
	var op struct {
		Responses map[string]struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(ops[strings.ToLower(method)], &op); err != nil {
		return nil, fmt.Errorf("%s %s not in spec", method, template)
	}
	r, ok := op.Responses[strconv.Itoa(status)]
	if !ok {
		return nil, fmt.Errorf("status %d not declared for %s %s", status, method, template)
	}
	if status >= 400 {
		return s.def("ApiError"), nil
	}
	if len(r.Schema) == 0 {
		return nil, nil
	}
	sc := &schema{Properties: map[string]*schema{}}
	s.merge(sc, r.Schema)
	return sc, nil
}

// Check는 응답 하나를 검사해 위반 목록을 돌려준다. template은 스펙 경로(예: /infra/segments/{segment-id}).
func (s *Spec) Check(method, template string, status int, body []byte) []string {
	sc, err := s.responseSchema(template, method, status)
	if err != nil {
		return []string{err.Error()}
	}
	if sc == nil {
		if len(strings.TrimSpace(string(body))) > 0 {
			return []string{"spec declares no response body, got one"}
		}
		return nil
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return []string{"invalid JSON: " + err.Error()}
	}
	return s.validate(sc, v, "$")
}

func (s *Spec) validate(sc *schema, v any, path string) []string {
	// 다형 타입(discriminator): 값의 판별 속성이 가리키는 하위 정의로 검사한다.
	if m, ok := v.(map[string]any); ok && sc.Discriminator != "" {
		if name, ok := m[sc.Discriminator].(string); ok {
			if _, ok := s.Definitions[name]; ok {
				if sub := s.def(name); sub != sc {
					sc = sub
				}
			} else {
				return []string{path + "." + sc.Discriminator + ": unknown subtype " + name}
			}
		}
	}
	var errs []string
	switch sc.Type {
	case "object", "":
		m, ok := v.(map[string]any)
		if !ok {
			if sc.Type == "" {
				return nil // 스키마 형식 미지정
			}
			return []string{path + ": expected object"}
		}
		for _, r := range sc.Required {
			if _, ok := m[r]; !ok {
				errs = append(errs, path+"."+r+": required")
			}
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p, ok := sc.Properties[k]
			if !ok {
				if !sc.Additional && len(sc.Properties) > 0 {
					errs = append(errs, path+"."+k+": not in schema")
				}
				continue
			}
			errs = append(errs, s.validate(p, m[k], path+"."+k)...)
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return []string{path + ": expected array"}
		}
		if sc.Items != nil {
			for i, e := range a {
				errs = append(errs, s.validate(sc.Items, e, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			return []string{path + ": expected string"}
		}
		if len(sc.Enum) > 0 && !contains(sc.Enum, s) {
			errs = append(errs, fmt.Sprintf("%s: %q not in enum", path, s))
		}
	case "integer":
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			return []string{path + ": expected integer"}
		}
	case "number":
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

func contains(xs []any, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
