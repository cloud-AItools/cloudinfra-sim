package sim

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/gateway"
	"cloudinfra-sim/internal/profile"
)

// API Coverage Report(WP 5.5: TR-SCN-0112, TR-COV-0101·0102, AC-12·AC-13).
//
// 실행 중인 프로파일 조합에 대해 API별 실행 범위 포함 여부·검증 상태·증거(시험 ID·계약 근거)·증거 구현 식별자와
// 일자, 그리고 실행 중 관측한 미지원 API 호출(FR-API-0403·0404)을 한 문서로 만든다. API 경로·요구사항 등 TR-COV-0102의
// 설계 열은 20-design/api-coverage/API_COVERAGE_MATRIX.md가 원본이며, 이 리포트는 그 행 ID로 연결된다.
// 검증 상한은 Integration Tested다(TR-COV-0409): 실행 범위 밖이거나 증거가 현재 구현과 맞지 않으면 '지원'으로 세지 않는다.

type CoverageAPI struct {
	ID       string   `json:"api"`
	Family   string   `json:"family"`
	Scope    string   `json:"scope"` // execution | out-of-scope
	Status   string   `json:"status"`
	Sources  []string `json:"contract_sources,omitempty"`
	Tests    []string `json:"tests,omitempty"`
	Note     string   `json:"note,omitempty"`
	Verified bool     `json:"counted_as_supported"`
}

type CoverageProduct struct {
	Product      string         `json:"product"`
	Version      string         `json:"version"`
	Build        string         `json:"build"`
	EvidenceID   string         `json:"evidence_commit"`
	EvidenceDate string         `json:"evidence_generated_at"`
	EvidenceOK   bool           `json:"evidence_matches_build"`
	Summary      map[string]int `json:"summary"`
	APIs         []CoverageAPI  `json:"apis"`
}

type CoverageReport struct {
	Generated        time.Time                 `json:"generated_at"`
	Build            map[string]string         `json:"build"`
	VerificationMode string                    `json:"verification_mode"`
	VerificationCap  string                    `json:"verification_cap"`
	Interop          string                    `json:"interop"`
	Products         []CoverageProduct         `json:"products"`
	Unsupported      []gateway.UnsupportedStat `json:"unsupported_calls"`
	UnsupportedOther int64                     `json:"unsupported_other_bucket"`
	Matrix           string                    `json:"design_matrix"`
}

func family(id string) string {
	switch {
	case strings.HasPrefix(id, "VC-SOAP"), strings.HasPrefix(id, "VC-NET"):
		return "SOAP (vim25)"
	case strings.HasPrefix(id, "VC-REST"):
		return "REST (/api)"
	case strings.HasPrefix(id, "VC-LOOKUP"):
		return "SOAP (Lookup Service)"
	case strings.HasPrefix(id, "VC-PBM"):
		return "SOAP (PBM)"
	case strings.HasPrefix(id, "NSX"):
		return "NSX Policy/Manager REST"
	}
	return "-"
}

// CoverageReport는 현재 조합의 Coverage Report다.
func (s *Simulator) CoverageReport() *CoverageReport {
	stats, other := s.GW.Unsup.Snapshot()
	r := &CoverageReport{
		Generated: time.Now().UTC(), Build: map[string]string{"kind": config.BuildKind, "commit": BuildCommit},
		VerificationMode: s.Cfg.VerificationMode, VerificationCap: "Integration Tested (승인 범위 자체 테스트, CMP·실환경 미검증)",
		Interop: s.Profile.Interop, Unsupported: stats, UnsupportedOther: other, Matrix: "20-design/api-coverage/API_COVERAGE_MATRIX.md",
	}
	build := BuildCommit
	product := func(name, version, b string, scope []string, v profile.Verification) CoverageProduct {
		p := CoverageProduct{Product: name, Version: version, Build: b, EvidenceID: v.Commit, EvidenceDate: v.GeneratedAt,
			EvidenceOK: build != "" && v.Commit == build, Summary: map[string]int{}}
		in := map[string]bool{}
		for _, id := range scope {
			in[id] = true
		}
		ids := append([]string{}, scope...)
		for id := range v.APIs {
			if !in[id] {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			ev := v.APIs[id]
			a := CoverageAPI{ID: id, Family: family(id), Scope: "out-of-scope", Status: ev.Status, Note: ev.Note}
			if a.Status == "" {
				a.Status = "Not Verified"
			}
			if in[id] {
				a.Scope = "execution"
			}
			for _, e := range ev.Evidence {
				if strings.HasPrefix(e, "SRC-") {
					a.Sources = append(a.Sources, e)
				} else {
					a.Tests = append(a.Tests, e)
				}
			}
			a.Verified = in[id] && a.Status == "Integration Tested"
			p.Summary[a.Scope+"/"+a.Status]++
			if a.Verified {
				p.Summary["counted_as_supported"]++
			}
			p.APIs = append(p.APIs, a)
		}
		return p
	}
	r.Products = []CoverageProduct{
		product("vCenter", s.Profile.VCenter.Version, s.Profile.VCenter.Build, s.Profile.VCenter.ExecutionScope, s.Profile.VCVerification),
		product("NSX", s.Profile.NSX.Version, s.Profile.NSX.Build, s.Profile.NSX.ExecutionScope, s.Profile.NSXVerification),
	}
	return r
}

// Markdown은 사람용 Coverage Report다.
func (r *CoverageReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# API Coverage Report\n\n- 생성: %s\n- 빌드: %s %s, 검증 모드 %s\n- 검증 상한: %s\n- 조합 상호운용성: %s\n- 설계 열(경로·요구사항·시나리오): `%s`\n\n",
		r.Generated.Format(time.RFC3339), r.Build["kind"], r.Build["commit"], r.VerificationMode, r.VerificationCap, r.Interop, r.Matrix)
	for _, p := range r.Products {
		fmt.Fprintf(&b, "## %s %s (build %s)\n\n증거: commit `%s`, 생성 %s, 현재 빌드와 일치 %v. 지원으로 세는 API: %d\n\n",
			p.Product, p.Version, p.Build, p.EvidenceID, p.EvidenceDate, p.EvidenceOK, p.Summary["counted_as_supported"])
		b.WriteString("| API | 계열 | 범위 | 상태 | 계약 근거 | 시험 | 비고 |\n|---|---|---|---|---|---|---|\n")
		for _, a := range p.APIs {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", a.ID, a.Family, a.Scope, a.Status,
				strings.Join(a.Sources, ", "), strings.Join(a.Tests, "<br>"), strings.ReplaceAll(a.Note, "|", "\\|"))
		}
		b.WriteString("\n")
	}
	b.WriteString("## 미지원 API 호출(관측, FR-API-0403·0404)\n\n")
	if len(r.Unsupported) == 0 {
		b.WriteString("없음\n")
	} else {
		b.WriteString("| 호출 | 횟수 | API 버전 | 처음 | 마지막 |\n|---|---|---|---|---|\n")
		for _, u := range r.Unsupported {
			fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n", strings.ReplaceAll(u.Key, "|", "\\|"), u.Count, strings.Join(u.Versions, ", "),
				u.FirstSeen.UTC().Format(time.RFC3339), u.LastSeen.UTC().Format(time.RFC3339))
		}
	}
	fmt.Fprintf(&b, "\n'기타' 버킷: %d건\n", r.UnsupportedOther)
	return b.String()
}

// Coverage는 Admin API 어댑터다(markdown이면 string).
func (s *Simulator) Coverage(markdown bool) any {
	r := s.CoverageReport()
	if markdown {
		return r.Markdown()
	}
	return r
}
