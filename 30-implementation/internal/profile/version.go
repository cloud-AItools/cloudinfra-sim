package profile

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// VersionMeta는 vim25 타입·필드·메서드의 도입 버전이다. 원천은 pyVmomi 추출본(`vim-typeinfo.json`)이며
// 없는 필드는 govmomi `vim` 구조체 태그로 보완한다(PoC-13).
type VersionMeta struct {
	Types   map[string]string            // WSDL 타입명 → 도입 버전
	Fields  map[string]map[string]string // WSDL 타입명 → 필드 → 도입 버전
	Methods map[string]string            // 메서드명 → 가장 이른 도입 버전(타입 무관)
	Privs   map[string]map[string]string // WSDL 타입명 → 메서드 → 권한 ID(RBAC, FR-API-0211)
	Parent  map[string]string            // WSDL 타입명 → 부모 WSDL 타입명(관리 객체 권한 상속 조회)
	vmodl   map[string]string            // vmodl 이름 → WSDL 이름
}

func LoadVersionMeta(path string) (*VersionMeta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Types map[string]struct {
			Since   string            `json:"since"`
			Fields  map[string]string `json:"fields"`
			Methods map[string]string `json:"methods"`
			Privs   map[string]string `json:"privileges"`
			Name    string            `json:"name"`
			ParentV string            `json:"parent"`
		} `json:"types"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	m := &VersionMeta{Types: map[string]string{}, Fields: map[string]map[string]string{}, Methods: map[string]string{},
		Privs: map[string]map[string]string{}, Parent: map[string]string{}, vmodl: map[string]string{}}
	for name, t := range doc.Types {
		m.vmodl[t.Name] = name
	}
	for name, t := range doc.Types {
		m.Types[name] = t.Since
		m.Fields[name] = t.Fields
		m.Privs[name] = t.Privs
		m.Parent[name] = m.vmodl[t.ParentV]
		for meth, v := range t.Methods {
			if cur, ok := m.Methods[meth]; !ok || Newer(cur, v) {
				m.Methods[meth] = v
			}
		}
	}
	return m, nil
}

// Newer는 a가 b보다 새 버전인지 비교한다("7.0.3.0" 형식, 숫자가 아닌 부분은 0).
func Newer(a, b string) bool {
	va, vb := parse(a), parse(b)
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

func parse(s string) [4]int {
	var v [4]int
	for i, p := range strings.SplitN(s, ".", 4) {
		v[i], _ = strconv.Atoi(p)
	}
	return v
}

// SessionVersion은 SOAPAction의 urn:vim25/<ver>와 프로파일 최대 버전 중 낮은 쪽이다(FR-VC-0605).
func SessionVersion(r *http.Request, profileMax string) string {
	action := strings.Trim(r.Header.Get("SOAPAction"), `"`)
	if v, ok := strings.CutPrefix(action, "urn:vim25/"); ok && v != "" && Newer(profileMax, v) {
		return v
	}
	return profileMax
}

// MethodSince는 메서드 도입 버전이다. 모르면 ok=false.
func (m *VersionMeta) MethodSince(method string) (string, bool) {
	v, ok := m.Methods[method]
	return v, ok
}

func (m *VersionMeta) fieldVersion(owner, field string, f reflect.StructField) string {
	if v, ok := m.Fields[owner][field]; ok {
		return v
	}
	return f.Tag.Get("vim")
}

// FilterStats는 필터가 제거한 항목 수다(로그·시험용).
type FilterStats struct {
	Fields, Types int
}

// FilterSOAP은 SOAP 응답 XML에서 sessionVersion보다 새 필드·타입을 제거한다(FR-VC-0606).
// 응답 요소를 govmomi Go 타입으로 따라가며 판정한다. 해석할 수 없는 응답은 원본을 그대로 돌려준다.
func (m *VersionMeta) FilterSOAP(in []byte, sessionVersion string) ([]byte, FilterStats) {
	var st FilterStats
	tf := types.TypeFunc()
	dec := xml.NewDecoder(bytes.NewReader(in))
	var out bytes.Buffer
	out.Grow(len(in))
	var stack []reflect.Type
	skip := 0
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return in, FilterStats{}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 {
				skip++
				continue
			}
			var parent, typ reflect.Type
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if strings.HasSuffix(t.Name.Local, "Response") && parent == nil {
				typ, _ = tf(t.Name.Local)
				if typ == nil {
					typ = responseType(t.Name.Local)
				}
			} else if parent != nil {
				if f, owner, ok := fieldByXMLName(parent, t.Name.Local); ok {
					if v := m.fieldVersion(owner.Name(), t.Name.Local, f); v != "" && Newer(v, sessionVersion) {
						st.Fields++
						skip = 1
						continue
					}
					typ = elemType(f.Type)
				}
			}
			for _, a := range t.Attr {
				if a.Name.Local != "type" || !(a.Name.Space == "xsi" || strings.HasSuffix(a.Name.Space, "XMLSchema-instance")) {
					continue
				}
				name := a.Value
				if i := strings.IndexByte(name, ':'); i >= 0 {
					name = name[i+1:]
				}
				if v, ok := m.Types[name]; ok && Newer(v, sessionVersion) {
					st.Types++ // 하위 호환 타입 대체 규칙은 미구현: 제거한다(R-11)
					skip = 1
				}
				if rt, ok := tf(name); ok {
					typ = rt
				}
			}
			if skip > 0 {
				continue
			}
			stack = append(stack, typ)
			writeStart(&out, t)
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			out.WriteString("</" + qname(t.Name) + ">")
		case xml.CharData:
			if skip == 0 {
				escapeText(&out, t)
			}
		case xml.ProcInst:
			if skip == 0 {
				out.WriteString("<?" + t.Target + " " + string(t.Inst) + "?>")
			}
		}
	}
	res, n := m.filterPaths(out.Bytes(), sessionVersion)
	st.Fields += n
	return res, st
}

// responseType은 types 패키지의 <Method>Response 구조체를 찾는다(TypeFunc에는 등록되지 않음).
func responseType(name string) reflect.Type {
	return responseTypes[name]
}

var responseTypes = map[string]reflect.Type{}

// RegisterResponseTypes는 필터가 따라갈 응답 타입을 등록한다.
func RegisterResponseTypes(vals ...any) {
	for _, v := range vals {
		t := reflect.TypeOf(v)
		responseTypes[t.Name()] = t
	}
}

func init() {
	RegisterResponseTypes(
		types.RetrieveServiceContentResponse{}, types.RetrievePropertiesExResponse{}, types.RetrievePropertiesResponse{},
		types.ContinueRetrievePropertiesExResponse{}, types.WaitForUpdatesExResponse{}, types.WaitForUpdatesResponse{},
		types.CheckForUpdatesResponse{}, types.LoginResponse{}, types.CreateContainerViewResponse{},
		types.FindByUuidResponse{}, types.FindByInventoryPathResponse{}, types.FindAllByUuidResponse{},
		// 시각을 담는 응답(Virtual Clock 시각 변환, FR-BE-0611)과 이벤트·Task·성능 조회
		types.CurrentTimeResponse{}, types.QueryEventsResponse{}, types.ReadNextEventsResponse{}, types.ReadPreviousEventsResponse{},
		types.ReadNextTasksResponse{}, types.ReadPreviousTasksResponse{}, types.QueryPerfResponse{}, types.SessionIsActiveResponse{},
	)
}

func qname(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func writeStart(w *bytes.Buffer, t xml.StartElement) {
	w.WriteString("<" + qname(t.Name))
	for _, a := range t.Attr {
		w.WriteString(" " + qname(a.Name) + `="`)
		_ = xml.EscapeText(w, []byte(a.Value))
		w.WriteString(`"`)
	}
	w.WriteString(">")
}

func elemType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	return t
}

func fieldByXMLName(t reflect.Type, name string) (reflect.StructField, reflect.Type, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("xml"), ",")
		if f.Anonymous && tag == "" {
			if et := elemType(f.Type); et != nil {
				if sf, owner, ok := fieldByXMLName(et, name); ok {
					return sf, owner, true
				}
			}
			continue
		}
		if tag == name {
			return f, t, true
		}
	}
	return reflect.StructField{}, nil, false
}

// SOAPMethod는 SOAP 요청 본문에서 메서드 이름(Body의 첫 요소)을 읽는다.
func SOAPMethod(body []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(body))
	inBody := false
	for {
		tok, err := dec.RawToken()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			if inBody {
				return se.Name.Local
			}
			if se.Name.Local == "Body" {
				inBody = true
			}
		}
	}
}

// 속성 경로로 요청한 값(RetrievePropertiesEx의 propSet, WaitForUpdatesEx의 changeSet)은 <name>에 경로가, <val>에
// 값이 들어가므로 구조 필터가 필드로 보지 못한다. 경로의 각 단계를 관리 객체 타입부터 따라가며 세션 버전보다 새로
// 도입된 단계가 있으면 그 propSet·changeSet을 뺀다(가정: 해당 버전에 없는 속성은 노출하지 않음, R-11).
var (
	pathTokRe = regexp.MustCompile(`<obj type="([A-Za-z0-9_]+)"|<(propSet|changeSet)><name>([^<]+)</name>`)
	moTypes   sync.Map // 관리 객체 타입명 → reflect.Type(nil 포함)
)

func (m *VersionMeta) filterPaths(in []byte, sessionVersion string) ([]byte, int) {
	idx := pathTokRe.FindAllSubmatchIndex(in, -1)
	if len(idx) == 0 {
		return in, 0
	}
	var out bytes.Buffer
	last, removed := 0, 0
	moType := ""
	for _, x := range idx {
		if x[0] < last {
			continue // 이미 뺀 블록 안
		}
		if x[2] >= 0 {
			moType = string(in[x[2]:x[3]])
			continue
		}
		elem, path := string(in[x[4]:x[5]]), string(in[x[6]:x[7]])
		if moType == "" || !m.pathNewer(moType, path, sessionVersion) {
			continue
		}
		end := bytes.Index(in[x[0]:], []byte("</"+elem+">"))
		if end < 0 {
			continue
		}
		out.Write(in[last:x[0]])
		last = x[0] + end + len("</"+elem+">")
		removed++
	}
	if removed == 0 {
		return in, 0
	}
	out.Write(in[last:])
	return out.Bytes(), removed
}

func managedType(name string) reflect.Type {
	if v, ok := moTypes.Load(name); ok {
		t, _ := v.(reflect.Type)
		return t
	}
	var t reflect.Type
	if v, err := mo.ObjectContentToType(types.ObjectContent{Obj: types.ManagedObjectReference{Type: name}}); err == nil {
		t = reflect.TypeOf(v)
	}
	moTypes.Store(name, t)
	return t
}

// pathNewer는 관리 객체 typ의 속성 경로(예: config.backingType, config.hardware.device[2000])에 세션 버전보다
// 새로 도입된 단계가 있는지 본다. 해석할 수 없는 단계(인터페이스 타입 등)를 만나면 거기서 멈춘다(유지).
func (m *VersionMeta) pathNewer(typ, path, sessionVersion string) bool {
	cur := managedType(typ)
	if cur == nil {
		return false
	}
	for i, seg := range strings.Split(path, ".") {
		if j := strings.IndexByte(seg, '['); j >= 0 {
			seg = seg[:j]
		}
		var f reflect.StructField
		var owner reflect.Type
		var ok bool
		if i == 0 {
			f, owner, ok = fieldByTag(cur, "json", seg) // mo 구조체는 json 태그
		} else {
			f, owner, ok = fieldByXMLName(cur, seg)
		}
		if !ok {
			return false
		}
		if v := m.fieldVersion(owner.Name(), seg, f); v != "" && Newer(v, sessionVersion) {
			return true
		}
		if cur = elemType(f.Type); cur == nil {
			return false
		}
	}
	return false
}

func fieldByTag(t reflect.Type, key, name string) (reflect.StructField, reflect.Type, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get(key), ",")
		if f.Anonymous && (tag == "" || tag == "-") {
			if et := elemType(f.Type); et != nil {
				if sf, owner, ok := fieldByTag(et, key, name); ok {
					return sf, owner, true
				}
			}
			continue
		}
		if tag == name {
			return f, t, true
		}
	}
	return reflect.StructField{}, nil, false
}

// escapeText는 문자 데이터에서 &·<·>만 이스케이프한다. xml.EscapeText는 줄바꿈을 &#xA;로 바꾸는데,
// 루트 요소 밖(XML 선언 뒤 줄바꿈)에 문자 참조가 생기면 문서가 well-formed가 아니게 되어 expat 기반
// 파서(pyVmomi)가 거부한다(govmomi는 허용). 원본 응답의 공백·줄바꿈을 그대로 유지한다.
var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeText(w *bytes.Buffer, b []byte) {
	_, _ = textEscaper.WriteString(w, string(b))
}

// privilegeOverrides는 pyVmomi 타입 정보에 권한이 비어 있는(타입마다 다른) 메서드의 권한이다.
// 근거: vSphere API 문서의 Destroy_Task·Rename_Task 권한(타입별), 가정으로 기록한다(BEHAVIOR_SPEC §9).
var privilegeOverrides = map[string]map[string]string{
	// ReconfigVM_Task의 실제 권한은 바꾸는 항목마다 다르다(CPUCount·Memory 등). 대표값으로 Config.Settings를 쓴다.
	"VirtualMachine": {"Destroy_Task": "VirtualMachine.Inventory.Delete", "Rename_Task": "VirtualMachine.Config.Rename",
		"ReconfigVM_Task": "VirtualMachine.Config.Settings"},
	"Folder":                      {"Destroy_Task": "Folder.Delete", "Rename_Task": "Folder.Rename"},
	"DistributedVirtualPortgroup": {"Destroy_Task": "DVPortgroup.Delete", "Rename_Task": "DVPortgroup.Modify"},
	"HostSystem":                  {"Destroy_Task": "Host.Inventory.RemoveHostFromCluster", "Rename_Task": "Host.Inventory.RenameCluster"},
}

// Privilege는 관리 객체 타입 objType에서 메서드 method에 필요한 권한 ID다(부모 타입 상속). 모르면 "".
func (m *VersionMeta) Privilege(objType, method string) string {
	if p := privilegeOverrides[objType][method]; p != "" {
		return p
	}
	for t := objType; t != ""; t = m.Parent[t] {
		if p := m.Privs[t][method]; p != "" {
			return p
		}
	}
	return ""
}
