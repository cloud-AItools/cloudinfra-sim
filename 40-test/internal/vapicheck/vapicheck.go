// Package vapicheck는 REST(/api) 응답을 SDK 바인딩에서 추출한 계약(vapi-contract.json)으로 검사한다.
// 근거: SRC-VAPI-SDK-7032 (vsphere-automation-sdk-python v7.0.3.2, 추출 .agentic/poc/tools/extract_vapi.py).
// 계약 파일은 저장소 밖에 둔다(D11 처리, 원본 파일 머리말의 저작권 표기 확인 전). 경로: VAPI_CONTRACT 환경변수.
package vapicheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type Type struct {
	Kind     string `json:"kind"`
	Optional bool   `json:"optional"`
	Item     *Type  `json:"item"`
	Key      *Type  `json:"key"`
	Value    *Type  `json:"value"`
	Ref      string `json:"type"`
}

type Operation struct {
	Method string   `json:"method"`
	URL    string   `json:"url"`
	Output *Type    `json:"output"`
	Errors []string `json:"errors"`
}

type Contract struct {
	Structs    map[string]map[string]*Type `json:"structs"`
	Enums      map[string][]string         `json:"enums"`
	Operations map[string]Operation        `json:"operations"`
	HTTPStatus map[string]int              `json:"http_status"`
}

// DefaultPath는 계약 파일의 기본 위치다.
func DefaultPath() string {
	if p := os.Getenv("VAPI_CONTRACT"); p != "" {
		return p
	}
	return "D:/AISCP/tools/specs/vapi-bindings-7.0.3.2/vapi-contract.json"
}

func Load(path string) (*Contract, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Contract
	return &c, json.Unmarshal(b, &c)
}

// CheckOperation은 오퍼레이션 출력 계약으로 응답 본문을 검사한다.
func (c *Contract) CheckOperation(op string, body []byte) []string {
	o, ok := c.Operations[op]
	if !ok {
		return []string{"unknown operation " + op}
	}
	return c.CheckBody(o.Output, body)
}

func (c *Contract) CheckBody(t *Type, body []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return []string{"invalid JSON: " + err.Error()}
	}
	var errs []string
	c.check("$", v, t, &errs)
	return errs
}

// CheckError는 오류 응답이 선언된 오류인지, 상태 코드가 vAPI 매핑과 같은지, 본문이 Error 구조인지 검사한다.
func (c *Contract) CheckError(op, errType string, status int, body []byte) []string {
	var errs []string
	id := "com.vmware.vapi.std.errors." + strings.ToLower(errType)
	o := c.Operations[op]
	declared := false
	for _, e := range o.Errors {
		if e == id {
			declared = true
		}
	}
	if !declared {
		errs = append(errs, fmt.Sprintf("%s is not declared for %s (declared: %v)", id, op, o.Errors))
	}
	if want := c.HTTPStatus[id]; want != status {
		errs = append(errs, fmt.Sprintf("status %d, vAPI mapping requires %d for %s", status, want, id))
	}
	errs = append(errs, c.CheckBody(&Type{Kind: "ref", Ref: "com.vmware.vapi.std.errors.error"}, body)...)
	var e struct {
		ErrorType string `json:"error_type"`
	}
	_ = json.Unmarshal(body, &e)
	if e.ErrorType != errType {
		errs = append(errs, fmt.Sprintf("error_type %q, want %q", e.ErrorType, errType))
	}
	return errs
}

func (c *Contract) check(path string, v any, t *Type, errs *[]string) {
	add := func(format string, a ...any) { *errs = append(*errs, path+": "+fmt.Sprintf(format, a...)) }
	if t == nil {
		return
	}
	if v == nil {
		if !t.Optional {
			add("null for required %s", t.Kind)
		}
		return
	}
	switch t.Kind {
	case "string", "secret", "id", "uri":
		if _, ok := v.(string); !ok {
			add("want string, got %T", v)
		}
	case "datetime":
		s, ok := v.(string)
		if !ok {
			add("want datetime string, got %T", v)
		} else if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			add("datetime %q is not RFC 3339", s)
		}
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			add("want integer, got %T", v)
		} else if _, err := n.Int64(); err != nil {
			add("want integer, got %s", n)
		}
	case "number":
		if _, ok := v.(json.Number); !ok {
			add("want number, got %T", v)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			add("want boolean, got %T", v)
		}
	case "list", "set":
		a, ok := v.([]any)
		if !ok {
			add("want array, got %T", v)
			return
		}
		for i, x := range a {
			c.check(fmt.Sprintf("%s[%d]", path, i), x, t.Item, errs)
		}
	case "map":
		m, ok := v.(map[string]any)
		if !ok {
			add("want object(map), got %T", v)
			return
		}
		for k, x := range m {
			c.check(path+"."+k, x, t.Value, errs)
		}
	case "ref":
		if vals, ok := c.Enums[t.Ref]; ok {
			s, ok := v.(string)
			if !ok {
				add("want enum string, got %T", v)
				return
			}
			for _, x := range vals {
				if x == s {
					return
				}
			}
			add("%q not in enum %s", s, t.Ref)
			return
		}
		fields, ok := c.Structs[t.Ref]
		if !ok {
			return // 계약에 없는 참조(비 MVP 모듈): 검사하지 않음
		}
		m, ok := v.(map[string]any)
		if !ok {
			add("want object %s, got %T", t.Ref, v)
			return
		}
		names := make([]string, 0, len(fields))
		for n := range fields {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			ft := fields[n]
			x, present := m[n]
			if !present {
				if !ft.Optional {
					add("required field %q of %s missing", n, t.Ref)
				}
				continue
			}
			c.check(path+"."+n, x, ft, errs)
		}
		for k := range m {
			if _, ok := fields[k]; !ok {
				add("unknown field %q for %s", k, t.Ref)
			}
		}
	}
}
