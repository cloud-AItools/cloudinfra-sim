package poc

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/vim25/types"
)

// 버전 인식 직렬화 PoC(PoC-13, R-11).
// vcsim은 SOAPAction 버전을 보지 않고 최신 타입 전체를 직렬화한다. 여기서는 vcsim을 수정하지 않고
// Gateway에서 응답 XML을 타입 메타데이터로 따라가며 세션 버전에 없는 필드·타입을 제거한다.

// vimVersion은 "7.0.3.0" 형식을 비교 가능한 정수 배열로 바꾼다.
func vimVersion(s string) [4]int {
	var v [4]int
	for i, p := range strings.SplitN(s, ".", 4) {
		v[i], _ = strconv.Atoi(p)
	}
	return v
}

func newer(a, b string) bool { // a > b
	va, vb := vimVersion(a), vimVersion(b)
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// typeMinVersion은 govmomi 생성 코드의 AddMinAPIVersionForType 호출을 읽는다.
// govmomi는 이 맵을 공개하지 않으므로 PoC에서는 소스에서 추출한다(본 구현에서는 코드 생성으로 대체).
func typeMinVersion(govmomiDir string) map[string]string {
	re := regexp.MustCompile(`minAPIVersionForType\["(\w+)"\] = "([\d.]+)"`)
	out := map[string]string{}
	files, _ := filepath.Glob(filepath.Join(govmomiDir, "vim25", "types", "*.go"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllSubmatch(b, -1) {
			out[string(m[1])] = string(m[2])
		}
	}
	return out
}

// versionMeta는 버전 메타데이터다. govmomi(필드 태그, 타입 맵)는 7.0.0.2 이후만 담고 있으므로
// pyVmomi에서 추출한 vim-typeinfo.json(타입·필드별 전체 도입 버전)으로 보완한다.
type versionMeta struct {
	govTypes map[string]string            // govmomi minAPIVersionForType
	pyTypes  map[string]string            // pyVmomi 타입 도입 버전
	pyFields map[string]map[string]string // pyVmomi [WSDL 타입명][필드명] 도입 버전
}

func (m *versionMeta) typeVersion(name string) (string, bool) {
	if v, ok := m.pyTypes[name]; ok {
		return v, true
	}
	v, ok := m.govTypes[name]
	return v, ok
}

// fieldVersion은 필드를 선언한 타입(owner) 기준으로 pyVmomi 버전을 찾고, 없으면 govmomi vim 태그를 쓴다.
func (m *versionMeta) fieldVersion(owner, field string, f reflect.StructField) string {
	if v, ok := m.pyFields[owner][field]; ok {
		return v
	}
	return f.Tag.Get("vim")
}

// loadPyVmomi는 extract_pyvmomi.py가 만든 JSON을 읽는다.
func loadPyVmomi(path string, m *versionMeta) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc struct {
		Types map[string]struct {
			Since  string            `json:"since"`
			Fields map[string]string `json:"fields"`
		} `json:"types"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	m.pyTypes, m.pyFields = map[string]string{}, map[string]map[string]string{}
	for name, t := range doc.Types {
		m.pyTypes[name] = t.Since
		m.pyFields[name] = t.Fields
	}
	return nil
}

// responseTypes는 PoC 범위의 메서드 응답 타입이다. 본 구현에서는 WSDL/메서드 목록에서 생성한다.
var responseTypes = map[string]reflect.Type{
	"RetrieveServiceContentResponse": reflect.TypeOf(types.RetrieveServiceContentResponse{}),
	"RetrievePropertiesExResponse":   reflect.TypeOf(types.RetrievePropertiesExResponse{}),
	"RetrievePropertiesResponse":     reflect.TypeOf(types.RetrievePropertiesResponse{}),
}

type filterStats struct {
	Fields  map[string]int // "Type.field@version"
	Types   map[string]int // xsi:type@version
	Unknown int            // 타입을 따라가지 못한 요소 수
}

// filterXML은 sessionVersion보다 새 필드·타입을 제거(strip=true)하거나 개수만 센다.
func filterXML(in []byte, sessionVersion string, meta *versionMeta, strip bool) ([]byte, filterStats) {
	st := filterStats{Fields: map[string]int{}, Types: map[string]int{}}
	tf := types.TypeFunc()
	dec := xml.NewDecoder(bytes.NewReader(in))
	var out bytes.Buffer
	type frame struct{ typ reflect.Type }
	var stack []frame
	skipDepth := 0

	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return in, st
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			var parent reflect.Type
			if len(stack) > 0 {
				parent = stack[len(stack)-1].typ
			}
			var typ reflect.Type
			if rt, ok := responseTypes[t.Name.Local]; ok {
				typ = rt
			} else if parent != nil {
				f, owner, ok := fieldByXMLName(parent, t.Name.Local)
				if ok {
					if v := meta.fieldVersion(owner.Name(), t.Name.Local, f); v != "" && newer(v, sessionVersion) {
						st.Fields[owner.Name()+"."+t.Name.Local+"@"+v]++
						if strip {
							skipDepth = 1
							continue
						}
					}
					typ = elemType(f.Type)
				} else {
					st.Unknown++
				}
			}
			for _, a := range t.Attr {
				if a.Name.Local == "type" && (a.Name.Space == "xsi" || strings.HasSuffix(a.Name.Space, "XMLSchema-instance")) {
					name := a.Value
					if i := strings.IndexByte(name, ':'); i >= 0 {
						name = name[i+1:]
					}
					if v, ok := meta.typeVersion(name); ok && newer(v, sessionVersion) {
						// 실제 제품은 하위 호환 규칙(상위 타입 대체 등)을 따른다. PoC는 탐지만 하고 제거한다.
						st.Types[name+"@"+v]++
						if strip {
							skipDepth = 1
						}
					}
					if rt, ok := tf(name); ok {
						typ = rt
					}
				}
			}
			if skipDepth > 0 {
				continue
			}
			stack = append(stack, frame{typ})
			writeStart(&out, t)
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			stack = stack[:len(stack)-1]
			out.WriteString("</" + qname(t.Name) + ">")
		case xml.CharData:
			if skipDepth == 0 {
				_ = xml.EscapeText(&out, t)
			}
		case xml.ProcInst:
			if skipDepth == 0 {
				out.WriteString("<?" + t.Target + " " + string(t.Inst) + "?>")
			}
		}
	}
	return out.Bytes(), st
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
		return nil // 인터페이스(Base*, AnyType)는 xsi:type으로 해석
	}
	return t
}

// fieldByXMLName은 XML 요소명에 해당하는 필드와 그 필드를 선언한 구조체 타입(임베딩 상위 타입 포함)을 찾는다.
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

// versionGateway는 요청의 SOAPAction(urn:vim25/<ver>)과 프로파일 최대 버전 중 낮은 쪽을 세션 버전으로 써서
// /sdk 응답을 필터링한다(FR-VC-0605, FR-VC-0606).
func versionGateway(next http.Handler, profileMax string, meta *versionMeta) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/sdk") || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		ver := sessionVersion(r, profileMax)
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, r)
		body, _ := filterXML(rec.Body.Bytes(), ver, meta, true)
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(rec.Code)
		_, _ = w.Write(body)
	})
}

// sessionVersion은 SOAPAction 네임스페이스 버전과 프로파일 최대 버전 중 낮은 쪽이다(FR-VC-0605).
func sessionVersion(r *http.Request, profileMax string) string {
	action := strings.Trim(r.Header.Get("SOAPAction"), `"`)
	if v, ok := strings.CutPrefix(action, "urn:vim25/"); ok && newer(profileMax, v) {
		return v
	}
	return profileMax
}
