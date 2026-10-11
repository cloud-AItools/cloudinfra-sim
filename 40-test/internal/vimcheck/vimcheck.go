// Package vimcheck은 vSphere SOAP 응답을 공식 계약(SRC-VIM-TYPEINFO: pyVmomi 9.1.1.0 타입 정보에서 추출한
// 타입·속성 선언 순서·선택 여부·도입 버전·열거값·메서드 반환형·선언 Fault)에 대조하는 스키마 대조 검사기다
// (PRD-2 TR-COV-0301·TR-COV-0304, WP 2.9·2.12). 선정 SDK(govmomi)는 관대하게 역직렬화하므로 SDK 통과만으로는
// 놓치는 위반(요소 순서, 필수 속성 누락, 잘못된 열거값, 세션 버전 밖 타입·속성, 선언되지 않은 Fault)을 찾는다.
//
// 검사 범위: 네임스페이스(urn:vim25), 응답 요소 이름, xsi:type의 존재·도입 버전·대입 가능성, 데이터 객체의 속성 이름·
// 선언 순서·도입 버전·필수 여부·반복 여부, 열거값, 기본형(boolean·정수·dateTime) 형식, MethodName·TypeName 값,
// 관리 객체 참조의 type 속성, anyType의 xsi:type 필수, Fault 상세의 선언 여부(RuntimeFault 계열은 모든 메서드에 허용).
package vimcheck

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Prop struct {
	Name, Type, Since string
	Optional, Link    bool // Link: F_LINK — 대상 객체 대신 key 문자열로 직렬화
}

type Type struct {
	Kind, Name, Parent, Since string
	Props                     []Prop
	Values                    []string
	Results                   map[string][2]string
	Faults                    map[string][]string
}

type Contract struct {
	types   map[string]*Type // WSDL 이름 → 타입
	byVmodl map[string]string
	methods map[string]*Type // WSDL 메서드 이름 → 소유 관리 타입
}

// Load는 프로파일의 vim-typeinfo.json을 읽는다.
func Load(path string) (*Contract, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Types map[string]struct {
			Kind, Name, Parent, Since string
			Props                     [][]any `json:"props"`
			Values                    []string
			Results                   map[string][2]string
			Faults                    map[string][]string
		}
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	c := &Contract{types: map[string]*Type{}, byVmodl: map[string]string{}, methods: map[string]*Type{}}
	for w, t := range doc.Types {
		nt := &Type{Kind: t.Kind, Name: t.Name, Parent: t.Parent, Since: t.Since, Values: t.Values, Results: t.Results, Faults: t.Faults}
		for _, p := range t.Props {
			if len(p) < 5 {
				return nil, fmt.Errorf("type %s: props entry %v", w, p)
			}
			nt.Props = append(nt.Props, Prop{Name: p[0].(string), Type: p[1].(string), Since: p[2].(string), Optional: p[3] == true, Link: p[4] == true})
		}
		c.add(w, nt)
	}
	// pyVmomi가 코드로 만드는 기본 타입(VmomiSupport). 속성은 vim WSDL 선언을 따른다.
	c.add("MethodFault", &Type{Kind: "data", Name: "vmodl.MethodFault", Parent: "vmodl.DynamicData", Since: "1.0", Props: []Prop{
		{"faultCause", "vmodl.MethodFault", "1.0", true, false}, {"faultMessage", "vmodl.LocalizableMessage[]", "1.0", true, false}}})
	c.add("RuntimeFault", &Type{Kind: "data", Name: "vmodl.RuntimeFault", Parent: "vmodl.MethodFault", Since: "1.0"})
	c.add("LocalizedMethodFault", &Type{Kind: "data", Name: "vmodl.LocalizedMethodFault", Parent: "vmodl.DynamicData", Since: "1.0", Props: []Prop{
		{"fault", faultObject, "1.0", false, false}, {"localizedMessage", "string", "1.0", true, false}}})
	return c, nil
}

func (c *Contract) add(wsdl string, t *Type) {
	c.types[wsdl] = t
	c.byVmodl[t.Name] = wsdl
	for m := range t.Results {
		if _, dup := c.methods[m]; !dup {
			c.methods[m] = t
		}
	}
}

// Finding은 계약 위반 하나다.
type Finding struct {
	Path, Rule, Detail string
}

func (f Finding) String() string { return f.Rule + " " + f.Path + ": " + f.Detail }

type node struct {
	name     xml.Name
	attr     []xml.Attr
	children []*node
	text     string
}

func (n *node) xsiType() string {
	for _, a := range n.attr {
		if a.Name.Local == "type" && (a.Name.Space == "xsi" || strings.HasSuffix(a.Name.Space, "XMLSchema-instance")) {
			v := a.Value
			if i := strings.IndexByte(v, ':'); i >= 0 {
				v = v[i+1:]
			}
			return v
		}
	}
	return ""
}

func (n *node) plainAttr(name string) (string, bool) {
	for _, a := range n.attr {
		if a.Name.Local == name && a.Name.Space == "" {
			return a.Value, true
		}
	}
	return "", false
}

func parse(b []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	root := &node{}
	stack := []*node{root}
	for {
		tok, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name, attr: t.Attr}
			p := stack[len(stack)-1]
			p.children = append(p.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			stack[len(stack)-1].text += string(t)
		}
	}
	return root, nil
}

type checker struct {
	c        *Contract
	version  string
	findings []Finding
	fields   map[string]string // 응답에 나온 데이터 속성 "<WSDL 타입>.<속성>" → since (버전 간 비교, WP 4.7)
}

func (k *checker) add(path, rule, format string, a ...any) {
	k.findings = append(k.findings, Finding{Path: path, Rule: rule, Detail: fmt.Sprintf(format, a...)})
}

// CheckResponse는 SOAP 메서드 method의 응답 본문을 세션 버전 sessionVersion 기준으로 검사한다.
func (c *Contract) CheckResponse(method, sessionVersion string, body []byte) []Finding {
	f, _ := c.CheckResponseFields(method, sessionVersion, body)
	return f
}

// CheckResponseFields는 CheckResponse와 같고, 응답에 나온 데이터 속성("<WSDL 타입>.<속성>" → 도입 버전)도 돌려준다.
func (c *Contract) CheckResponseFields(method, sessionVersion string, body []byte) ([]Finding, map[string]string) {
	k := &checker{c: c, version: sessionVersion, fields: map[string]string{}}
	return k.check(method, body), k.fields
}

func (k *checker) check(method string, body []byte) []Finding {
	c := k.c
	root, err := parse(body)
	if err != nil {
		k.add("/", "well-formed", "%v", err)
		return k.findings
	}
	var bodyEl *node
	for _, env := range root.children {
		for _, ch := range env.children {
			if ch.name.Local == "Body" {
				bodyEl = ch
			}
		}
	}
	if bodyEl == nil || len(bodyEl.children) != 1 {
		k.add("/", "envelope", "Body with exactly one child expected")
		return k.findings
	}
	el := bodyEl.children[0]
	owner := c.methods[method]
	if el.name.Local == "Fault" {
		k.checkFault(el, method, owner)
		return k.findings
	}
	if el.name.Space != "urn:vim25" {
		k.add(el.name.Local, "namespace", "response namespace %q, want urn:vim25", el.name.Space)
	}
	if el.name.Local != method+"Response" {
		k.add(el.name.Local, "response-name", "want %sResponse", method)
	}
	if owner == nil {
		k.add(method, "method", "method not in contract")
		return k.findings
	}
	res := owner.Results[method][0]
	for i, rv := range el.children {
		if rv.name.Local != "returnval" {
			k.add(rv.name.Local, "response-element", "only returnval allowed")
			continue
		}
		if res == "void" || res == "" {
			k.add("returnval", "response-element", "void method returned a value")
			continue
		}
		if !strings.HasSuffix(res, "[]") && i > 0 {
			k.add("returnval", "multiplicity", "single-valued result repeated")
		}
		k.value(rv, strings.TrimSuffix(res, "[]"), method+"/returnval")
	}
	return k.findings
}

func (k *checker) checkFault(el *node, method string, owner *Type) {
	var detail *node
	for _, ch := range el.children {
		if ch.name.Local == "detail" {
			detail = ch
		}
	}
	if detail == nil || len(detail.children) == 0 {
		k.add("Fault", "fault-detail", "no fault detail")
		return
	}
	f := detail.children[0]
	typ := f.xsiType()
	if typ == "" {
		typ = strings.TrimSuffix(f.name.Local, "Fault")
		if _, ok := k.c.types[typ]; !ok {
			typ = f.name.Local
		}
	}
	t, ok := k.c.types[typ]
	if !ok {
		k.add("Fault/"+typ, "fault-type", "unknown fault type")
		return
	}
	if !k.c.assignable(t.Name, "vmodl.RuntimeFault") && owner != nil {
		declared := false
		for _, d := range owner.Faults[method] {
			if k.c.assignable(t.Name, d) {
				declared = true
			}
		}
		if !declared {
			k.add("Fault/"+typ, "fault-declared", "%s is not declared by %s and is not a RuntimeFault", typ, method)
		}
	}
	k.data(f, t, "Fault/"+typ)
}

// assignable: vmodl 타입 a가 b이거나 b의 하위 타입인가.
func (c *Contract) assignable(a, b string) bool {
	for a != "" {
		if a == b {
			return true
		}
		w, ok := c.byVmodl[a]
		if !ok {
			return false
		}
		a = c.types[w].Parent
	}
	return false
}

var xsdPrimitive = map[string]string{"string": "string", "boolean": "boolean", "int": "int", "long": "long", "short": "short",
	"byte": "byte", "float": "float", "double": "double", "dateTime": "vmodl.DateTime", "base64Binary": "vmodl.Binary", "anyURI": "string"}

var intRe = regexp.MustCompile(`^-?[0-9]+$`)

// faultObject는 LocalizedMethodFault.fault처럼 실제 Fault 객체(xsi:type 필수)가 오는 자리다.
const faultObject = "vmodl.MethodFault#object"

// value는 선언 타입 decl(vmodl 이름 또는 기본형)인 요소 n을 검사한다.
func (k *checker) value(n *node, decl, path string) {
	// vmodl.MethodFault 형식 데이터 속성(예: TaskInfo.error)은 WSDL에서 LocalizedMethodFault로 직렬화된다.
	if decl == "vmodl.MethodFault" && n.xsiType() == "" {
		decl = "vmodl.LocalizedMethodFault"
	}
	if decl == faultObject {
		if n.xsiType() == "" {
			k.add(path, "xsi-type", "fault object without xsi:type")
			return
		}
		decl = "vmodl.MethodFault"
	}
	if xt := n.xsiType(); xt != "" {
		actual, ok := k.resolveXSI(xt)
		if !ok {
			k.add(path, "xsi-type", "unknown xsi:type %s", xt)
			return
		}
		if w, ok := k.c.byVmodl[actual]; ok {
			if t := k.c.types[w]; newer(t.Since, k.version) {
				k.add(path, "type-version", "xsi:type %s since %s > session %s", xt, t.Since, k.version)
			}
		}
		if decl != "anyType" && decl != actual && !strings.HasPrefix(actual, "ArrayOf") && !k.c.assignable(actual, decl) &&
			!(decl == "vmodl.ManagedObject" && actual == "ManagedObjectReference") {
			k.add(path, "xsi-type", "%s not assignable to %s", actual, decl)
		}
		decl = actual
	} else if decl == "anyType" {
		k.add(path, "xsi-type", "anyType value without xsi:type")
		return
	}
	if el, ok := strings.CutPrefix(decl, "ArrayOf"); ok {
		item := el
		if p, ok := xsdPrimitive[strings.ToLower(el[:1])+el[1:]]; ok {
			item = p
		} else if w, ok := k.c.types[el]; ok {
			item = w.Name
		} else if el == "ManagedObjectReference" {
			item = "vmodl.ManagedObject"
		}
		for i, ch := range n.children {
			k.value(ch, item, fmt.Sprintf("%s[%d]", path, i))
		}
		return
	}
	text := strings.TrimSpace(n.text)
	switch decl {
	case "string", "vmodl.PropertyPath", "vmodl.Binary":
		return
	case "boolean":
		if text != "true" && text != "false" && text != "1" && text != "0" {
			k.add(path, "boolean", "%q", text)
		}
		return
	case "int", "long", "short", "byte":
		if !intRe.MatchString(text) {
			k.add(path, "integer", "%q", text)
		}
		return
	case "float", "double":
		if _, err := strconv.ParseFloat(text, 64); err != nil {
			k.add(path, "float", "%q", text)
		}
		return
	case "vmodl.DateTime":
		if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
			k.add(path, "dateTime", "%q", text)
		}
		return
	case "vmodl.MethodName":
		if _, ok := k.c.methods[text]; !ok {
			k.add(path, "method-name", "%q is not a WSDL method name", text)
		}
		return
	case "vmodl.TypeName":
		if _, ok := k.c.types[text]; !ok && text != "ManagedObjectReference" {
			k.add(path, "type-name", "%q is not a known type", text)
		}
		return
	case "vmodl.ManagedObject", "ManagedObjectReference":
		k.moref(n, "", path)
		return
	}
	w, ok := k.c.byVmodl[decl]
	if !ok {
		k.add(path, "type", "declared type %s not in contract", decl)
		return
	}
	t := k.c.types[w]
	switch t.Kind {
	case "enum":
		for _, v := range t.Values {
			if v == text {
				return
			}
		}
		k.add(path, "enum", "%q is not a %s value", text, w)
	case "managed":
		k.moref(n, t.Name, path)
	default:
		k.data(n, t, path)
	}
}

func (k *checker) resolveXSI(x string) (string, bool) {
	if p, ok := xsdPrimitive[x]; ok {
		return p, true
	}
	if x == "ManagedObjectReference" {
		return x, true
	}
	if strings.HasPrefix(x, "ArrayOf") {
		return x, true
	}
	if t, ok := k.c.types[x]; ok {
		return t.Name, true
	}
	return "", false
}

func (k *checker) moref(n *node, decl, path string) {
	typ, ok := n.plainAttr("type")
	if !ok || strings.TrimSpace(n.text) == "" {
		k.add(path, "moref", "managed object reference needs type attribute and value")
		return
	}
	t, ok := k.c.types[typ]
	if !ok || t.Kind != "managed" {
		k.add(path, "moref", "type %q is not a managed object type", typ)
		return
	}
	if decl != "" && !k.c.assignable(t.Name, decl) {
		k.add(path, "moref", "%s not assignable to %s", typ, decl)
	}
}

// props는 부모부터 선언 순서대로 이어 붙인 속성 목록이다(WSDL sequence 순서).
func (c *Contract) props(t *Type) []Prop {
	var chain []*Type
	for cur := t; cur != nil; {
		chain = append(chain, cur)
		w, ok := c.byVmodl[cur.Parent]
		if !ok {
			break
		}
		cur = c.types[w]
	}
	var out []Prop
	for i := len(chain) - 1; i >= 0; i-- {
		out = append(out, chain[i].Props...)
	}
	return out
}

func (k *checker) data(n *node, t *Type, path string) {
	props := k.c.props(t)
	idx := map[string]int{}
	for i, p := range props {
		idx[p.Name] = i
	}
	seen := map[string]int{}
	last := -1
	for _, ch := range n.children {
		name := ch.name.Local
		i, ok := idx[name]
		if !ok {
			k.add(path+"/"+name, "unknown-property", "not a property of %s", k.c.byVmodl[t.Name])
			continue
		}
		p := props[i]
		k.fields[k.c.byVmodl[t.Name]+"."+name] = p.Since
		if i < last {
			k.add(path+"/"+name, "order", "out of declaration order in %s", k.c.byVmodl[t.Name])
		}
		last = i
		seen[name]++
		if seen[name] > 1 && !strings.HasSuffix(p.Type, "[]") {
			k.add(path+"/"+name, "multiplicity", "single-valued property repeated")
		}
		if newer(p.Since, k.version) {
			k.add(path+"/"+name, "property-version", "since %s > session %s", p.Since, k.version)
		}
		decl := strings.TrimSuffix(p.Type, "[]")
		if p.Link {
			decl = "string"
		}
		k.value(ch, decl, path+"/"+name)
	}
	for _, p := range props {
		if !p.Optional && seen[p.Name] == 0 && !newer(p.Since, k.version) {
			k.add(path+"/"+p.Name, "required", "required property of %s missing", k.c.byVmodl[t.Name])
		}
	}
}

func newer(a, b string) bool {
	pa, pb := parts(a), parts(b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parts(s string) [4]int {
	var out [4]int
	for i, f := range strings.SplitN(s, ".", 4) {
		n, _ := strconv.Atoi(f)
		out[i] = n
	}
	return out
}
