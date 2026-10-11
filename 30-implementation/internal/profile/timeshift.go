package profile

import (
	"bytes"
	"encoding/xml"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/vmware/govmomi/vim25/types"
)

var timeType = reflect.TypeOf(time.Time{})

// ShiftTimes는 SOAP 응답의 xsd:dateTime 값을 fn으로 바꾼다(Virtual Clock, FR-BE-0611·R-12).
// vcsim은 Task·Event·세션 시각에 실제 시계를 쓰고 교체 지점이 없으므로, 응답에서 가상 시각으로 변환한다.
// 응답 요소를 govmomi Go 타입으로 따라가 time.Time 필드와 xsi:type="xsd:dateTime" 값만 바꾼다(문자열 등은 그대로).
// 해석할 수 없는 응답은 원본을 돌려준다.
func ShiftTimes(in []byte, fn func(time.Time) time.Time) []byte {
	tf := types.TypeFunc()
	dec := xml.NewDecoder(bytes.NewReader(in))
	var out bytes.Buffer
	out.Grow(len(in))
	var stack []reflect.Type
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return in
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var parent, typ reflect.Type
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if strings.HasSuffix(t.Name.Local, "Response") && parent == nil {
				typ, _ = tf(t.Name.Local)
				if typ == nil {
					typ = responseType(t.Name.Local)
				}
			} else if parent != nil && parent != timeType {
				if f, _, ok := fieldByXMLName(parent, t.Name.Local); ok {
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
				if name == "dateTime" {
					typ = timeType
				} else if rt, ok := tf(name); ok {
					typ = rt
				}
			}
			stack = append(stack, typ)
			writeStart(&out, t)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			out.WriteString("</" + qname(t.Name) + ">")
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == timeType {
				if ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(t))); err == nil {
					out.WriteString(fn(ts).UTC().Format(time.RFC3339Nano))
					continue
				}
			}
			escapeText(&out, t)
		case xml.ProcInst:
			out.WriteString("<?" + t.Target + " " + string(t.Inst) + "?>")
		}
	}
	return out.Bytes()
}
