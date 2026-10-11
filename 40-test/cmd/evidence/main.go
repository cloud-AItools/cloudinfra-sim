// evidence는 `go test -json` 결과를 API별 검증 증거로 바꿔 verification.yaml을 생성한다(WP 1.13, FR-API-0302).
//
//	go test -json ./... > results.json
//	go run ./cmd/evidence -results results.json
//
// 증거는 구현 식별자(소스 다이제스트 또는 BUILD_COMMIT)와 프로파일 해시에 묶인다. 매핑된 테스트가 하나라도
// 실패·미실행이면 해당 API는 승격하지 않는다(미실행을 통과로 처리하지 않음).
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"cloudinfra-sim/internal/buildinfo"
	"cloudinfra-sim/internal/profile"
)

type event struct {
	Action  string
	Package string
	Test    string
}

func main() {
	results := flag.String("results", "results.json", "go test -json 출력")
	mapFile := flag.String("map", "evidence-map.yaml", "API → 테스트 매핑")
	impl := flag.String("impl", "../30-implementation", "구현 디렉터리")
	vc := flag.String("vcenter", "7.0.3.02500", "vCenter 프로파일")
	nsx := flag.String("nsx", "3.2.4.3", "NSX 프로파일")
	scopeOnly := flag.Bool("scope-only", false, "프로파일 execution_scope에 있는 API만 기록(추가 버전 프로파일: 계약 근거가 없는 API를 승격하지 않음)")
	flag.Parse()

	pass, fail := readResults(*results)
	var mapping map[string]map[string][]string
	b, err := os.ReadFile(*mapFile)
	must(err)
	must(yaml.Unmarshal(b, &mapping))
	commit, err := buildinfo.SourceDigest(*impl)
	must(err)
	root := filepath.Join(*impl, "data", "compatibility-profiles")
	for product, dir := range map[string]string{"vcenter": filepath.Join(root, "vcenter", *vc), "nsx": filepath.Join(root, "nsx", *nsx)} {
		apis := mapping[product]
		if *scopeOnly {
			apis = inScope(dir, apis)
		}
		write(product, dir, apis, pass, fail, commit)
	}
}

// inScope는 profile.yaml execution_scope에 있는 API만 남긴다.
func inScope(dir string, apis map[string][]string) map[string][]string {
	var p struct {
		Scope []string `yaml:"execution_scope"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "profile.yaml"))
	must(err)
	must(yaml.Unmarshal(b, &p))
	out := map[string][]string{}
	for _, id := range p.Scope {
		if t, ok := apis[id]; ok {
			out[id] = t
		}
	}
	return out
}

func readResults(path string) (pass, fail map[string]bool) {
	pass, fail = map[string]bool{}, map[string]bool{}
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e event
		if json.Unmarshal([]byte(line), &e) != nil || e.Test == "" || strings.Contains(e.Test, "/") {
			continue
		}
		key := e.Package[strings.LastIndex(e.Package, "/")+1:] + "/" + e.Test
		switch e.Action {
		case "pass":
			pass[key] = true
		case "fail":
			fail[key] = true
		}
	}
	must(sc.Err())
	return
}

func write(product, dir string, apis map[string][]string, pass, fail map[string]bool, commit string) {
	path := filepath.Join(dir, "verification.yaml")
	var v profile.Verification
	if b, err := os.ReadFile(path); err == nil {
		must(yaml.Unmarshal(b, &v))
	}
	if v.APIs == nil {
		v.APIs = map[string]profile.APIEvidence{}
	}
	ids := make([]string, 0, len(apis))
	for id := range apis {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		tests := apis[id]
		ok := len(tests) > 0
		var missing []string
		for _, t := range tests {
			if !pass[t] || fail[t] {
				ok = false
				missing = append(missing, t)
			}
		}
		cur := v.APIs[id]
		// 계약 근거(SRC-*)는 유지하고 시험 증거를 덧붙인다.
		var sources []string
		for _, ev := range cur.Evidence {
			if strings.HasPrefix(ev, "SRC-") {
				sources = append(sources, ev)
			}
		}
		if ok {
			cur.Status = profile.IntegrationTested.String()
			cur.Evidence = append(sources, tests...)
			cur.Note = ""
		} else {
			if cur.Status == profile.IntegrationTested.String() {
				cur.Status = profile.ContractDefined.String() // 증거 무효화
			}
			cur.Evidence = sources
			cur.Note = "미통과·미실행: " + strings.Join(missing, ", ")
		}
		v.APIs[id] = cur
		fmt.Printf("%-8s %-12s %s\n", product, id, cur.Status)
	}
	v.Commit = commit
	v.ProfileHash = profile.HashProfile(dir)
	v.GeneratedAt = time.Now().UTC().Format(time.RFC3339) + " (cmd/evidence)"
	out, err := yaml.Marshal(&v)
	must(err)
	header := "# API별 검증 상태(TR-COV-0201). cmd/evidence가 생성한다. 손으로 Integration Tested로 고치지 않는다.\n"
	must(os.WriteFile(path, append([]byte(header), out...), 0o644))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "evidence:", err)
		os.Exit(1)
	}
}
