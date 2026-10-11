package nsx

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// 검색 API(FR-NSX-0221, 0232, 0413): GET /policy/api/v1/search/query, GET /api/v1/search/query (스펙 QuerySearch,
// 응답 SearchResponse). 스펙은 query 문법을 정하지 않으므로(“Search query”) 지원 문법은 Lucene query string의 부분집합으로
// 정한다(API_COVERAGE_MATRIX §6.2, 가정):
//
//	field:value          필드 값 일치(대소문자 무관). 점 표기로 중첩·배열 원소 필드(tags.scope, tags.tag)
//	value                필드 없는 값: 최상위 문자열 필드 또는 tags.tag가 일치하거나 그 값의 단어 하나와 일치
//	*, ?                 와일드카드(여러 문자, 한 문자)
//	"quoted value"       공백·특수문자 포함 값. 백슬래시 이스케이프(\/ 등)
//	AND, OR, NOT, &&, ||, !, -term, ( )   불리언 결합. 연산자 없이 이어 쓴 항은 OR(Lucene 기본값)
//
// 범위([a TO b]), 퍼지(~), 부스트(^), 필드 그룹(field:(a b)), _exists_는 지원하지 않는다(400).
// 대상: Policy는 /infra 아래 객체(시스템 소유 포함), Manager는 Manager 객체. sort_by·sort_ascending·cursor·page_size를 따른다.

func (e *Engine) search(w http.ResponseWriter, r *http.Request, base string) {
	q := r.URL.Query()
	expr, err := parseQuery(q.Get("query"))
	if err != nil {
		apiError(w, http.StatusBadRequest, "Search", "Invalid search query: "+err.Error())
		return
	}
	e.mu.Lock()
	var items []map[string]any
	keys := make([]string, 0, len(e.store))
	for k := range e.store {
		if strings.HasPrefix(k, base+"/") && !(base == managerBase && strings.HasPrefix(k, policyBase)) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if base == managerBase && strings.HasPrefix(k, managerBase+vmTagPrefix) {
			continue
		}
		o := e.store[k]
		if base == policyBase {
			o = e.decorateLocked(strings.TrimPrefix(k, base), o, false)
		}
		if expr.match(o) {
			items = append(items, o)
		}
	}
	e.mu.Unlock()
	if by := q.Get("sort_by"); by != "" {
		asc := q.Get("sort_ascending") != "false"
		sort.SliceStable(items, func(i, j int) bool {
			a, b := fmt.Sprint(items[i][by]), fmt.Sprint(items[j][by])
			if asc {
				return a < b
			}
			return a > b
		})
	}
	out := page(items, r)
	if by := q.Get("sort_by"); by != "" {
		out["sort_by"], out["sort_ascending"] = by, q.Get("sort_ascending") != "false"
	} else {
		delete(out, "sort_by")
		delete(out, "sort_ascending")
	}
	writeJSON(w, http.StatusOK, out)
}

// node 종류
type qnode struct {
	op         string // "and", "or", "not", "term"
	kids       []*qnode
	field, val string
	quoted     bool
}

func (n *qnode) match(o map[string]any) bool {
	switch n.op {
	case "and":
		for _, k := range n.kids {
			if !k.match(o) {
				return false
			}
		}
		return true
	case "or":
		for _, k := range n.kids {
			if k.match(o) {
				return true
			}
		}
		return false
	case "not":
		return !n.kids[0].match(o)
	case "all":
		return true
	}
	if n.field != "" {
		for _, v := range fieldValues(o, strings.Split(n.field, ".")) {
			if (n.quoted && strings.EqualFold(n.val, v)) || (!n.quoted && valueMatch(n.val, v)) {
				return true
			}
		}
		return false
	}
	var cands []string
	for _, v := range o {
		if s, ok := v.(string); ok {
			cands = append(cands, s)
		}
	}
	cands = append(cands, fieldValues(o, []string{"tags", "tag"})...)
	for _, c := range cands {
		if valueMatch(n.val, c) {
			return true
		}
		if !n.quoted {
			for _, tok := range strings.FieldsFunc(c, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
				if valueMatch(n.val, tok) {
					return true
				}
			}
		}
	}
	return false
}

// fieldValues는 점 표기 필드의 문자열 값들이다(배열은 원소마다, 숫자·불리언은 문자열로).
func fieldValues(v any, segs []string) []string {
	switch x := v.(type) {
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, fieldValues(e, segs)...)
		}
		return out
	case map[string]any:
		if len(segs) == 0 {
			return nil
		}
		return fieldValues(x[segs[0]], segs[1:])
	case nil:
		return nil
	default:
		if len(segs) > 0 {
			return nil
		}
		if f, ok := x.(float64); ok && f == float64(int64(f)) {
			return []string{fmt.Sprint(int64(f))}
		}
		return []string{fmt.Sprint(x)}
	}
}

func valueMatch(pattern, s string) bool {
	return glob([]rune(strings.ToLower(pattern)), []rune(strings.ToLower(s)))
}

// glob은 *(0개 이상)·?(1개) 와일드카드 비교다.
func glob(p, s []rune) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for i := 0; i <= len(s); i++ {
				if glob(p[1:], s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
		default:
			if len(s) == 0 || s[0] != p[0] {
				return false
			}
		}
		p, s = p[1:], s[1:]
	}
	return len(s) == 0
}

// --- 파서 ---

type qtok struct {
	kind   string // "word", "lp", "rp", "and", "or", "not"
	field  string // field:value의 필드(이스케이프·따옴표 밖의 첫 ':' 앞)
	val    string
	quoted bool
}

func lex(s string) ([]qtok, error) {
	var out []qtok
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '(':
			out = append(out, qtok{kind: "lp"})
			i++
		case r == ')':
			out = append(out, qtok{kind: "rp"})
			i++
		case r == '&' && i+1 < len(rs) && rs[i+1] == '&':
			out = append(out, qtok{kind: "and"})
			i += 2
		case r == '|' && i+1 < len(rs) && rs[i+1] == '|':
			out = append(out, qtok{kind: "or"})
			i += 2
		case r == '!' || (r == '-' && (i == 0 || unicode.IsSpace(rs[i-1]) || rs[i-1] == '(')):
			out = append(out, qtok{kind: "not"})
			i++
		default:
			var b strings.Builder
			quoted := false
			field := ""
			for i < len(rs) && !unicode.IsSpace(rs[i]) && rs[i] != '(' && rs[i] != ')' {
				c := rs[i]
				switch {
				case c == '\\' && i+1 < len(rs):
					b.WriteRune(rs[i+1])
					i += 2
					continue
				case c == '"':
					j := i + 1
					for j < len(rs) && rs[j] != '"' {
						if rs[j] == '\\' && j+1 < len(rs) {
							j++
						}
						b.WriteRune(rs[j])
						j++
					}
					if j >= len(rs) {
						return nil, fmt.Errorf("unterminated quote")
					}
					quoted = true
					i = j + 1
					continue
				case strings.ContainsRune("[]{}~^", c):
					return nil, fmt.Errorf("unsupported syntax %q", string(c))
				case c == ':' && field == "" && !quoted && b.Len() > 0:
					field = b.String()
					b.Reset()
					i++
					continue
				}
				b.WriteRune(c)
				i++
			}
			w := b.String()
			switch {
			case field != "":
				out = append(out, qtok{kind: "word", field: field, val: w, quoted: quoted})
			case !quoted && w == "AND":
				out = append(out, qtok{kind: "and"})
			case !quoted && w == "OR":
				out = append(out, qtok{kind: "or"})
			case !quoted && w == "NOT":
				out = append(out, qtok{kind: "not"})
			default:
				out = append(out, qtok{kind: "word", val: w, quoted: quoted})
			}
		}
	}
	return out, nil
}

type qparser struct {
	toks []qtok
	pos  int
}

func parseQuery(s string) (*qnode, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("query is required")
	}
	toks, err := lex(s)
	if err != nil {
		return nil, err
	}
	p := &qparser{toks: toks}
	n, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("unexpected token at position %d", p.pos)
	}
	return n, nil
}

func (p *qparser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos].kind
	}
	return ""
}

func (p *qparser) or() (*qnode, error) {
	n, err := p.and()
	if err != nil {
		return nil, err
	}
	kids := []*qnode{n}
	for {
		k := p.peek()
		if k == "or" {
			p.pos++
		} else if k != "word" && k != "lp" && k != "not" {
			break // 이어 쓴 항은 OR(Lucene 기본 연산자)
		}
		m, err := p.and()
		if err != nil {
			return nil, err
		}
		kids = append(kids, m)
	}
	if len(kids) == 1 {
		return n, nil
	}
	return &qnode{op: "or", kids: kids}, nil
}

func (p *qparser) and() (*qnode, error) {
	n, err := p.unary()
	if err != nil {
		return nil, err
	}
	kids := []*qnode{n}
	for p.peek() == "and" {
		p.pos++
		m, err := p.unary()
		if err != nil {
			return nil, err
		}
		kids = append(kids, m)
	}
	if len(kids) == 1 {
		return n, nil
	}
	return &qnode{op: "and", kids: kids}, nil
}

func (p *qparser) unary() (*qnode, error) {
	if p.peek() == "not" {
		p.pos++
		n, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &qnode{op: "not", kids: []*qnode{n}}, nil
	}
	switch p.peek() {
	case "lp":
		p.pos++
		n, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.peek() != "rp" {
			return nil, fmt.Errorf("missing )")
		}
		p.pos++
		return n, nil
	case "word":
		t := p.toks[p.pos]
		p.pos++
		if t.val == "*" && !t.quoted {
			return &qnode{op: "all"}, nil
		}
		if t.field != "" {
			if t.val == "" && !t.quoted {
				return nil, fmt.Errorf("missing value for field %s", t.field)
			}
			if t.field == "_exists_" {
				return nil, fmt.Errorf("unsupported syntax _exists_")
			}
			return &qnode{op: "term", field: t.field, val: t.val, quoted: t.quoted}, nil
		}
		return &qnode{op: "term", val: t.val, quoted: t.quoted}, nil
	}
	return nil, fmt.Errorf("unexpected end of query")
}
