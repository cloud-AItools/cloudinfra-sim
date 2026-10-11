// Package profile은 버전 프로파일을 로드하고 기동 검증(FR-API-0302)과 버전 인식 직렬화(FR-VC-0604~0608)를 제공한다.
package profile

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Status는 API 구현·검증 상태다(TR-COV-0201). 순서가 곧 단계다.
type Status int

const (
	NotAnalyzed Status = iota
	Unsupported
	ContractDefined
	Implemented
	UnitTested
	IntegrationTested
)

var statusNames = []string{"Not Analyzed", "Unsupported", "Contract Defined", "Implemented", "Unit Tested", "Integration Tested"}

func (s Status) String() string { return statusNames[s] }

func ParseStatus(s string) (Status, error) {
	for i, n := range statusNames {
		if strings.EqualFold(n, s) {
			return Status(i), nil
		}
	}
	return NotAnalyzed, fmt.Errorf("알 수 없는 검증 상태 %q", s)
}

// SpecSource는 계약 근거의 출처 기록이다(FR-API-0307).
type SpecSource struct {
	ID      string `yaml:"id"`
	File    string `yaml:"file,omitempty"`
	Version string `yaml:"version"`
	SHA256  string `yaml:"sha256,omitempty"`
	Origin  string `yaml:"origin"`
	Note    string `yaml:"note,omitempty"`
}

// VCenter는 vCenter 프로파일(`profile.yaml`)이다.
type VCenter struct {
	Product          string            `yaml:"product"`
	Version          string            `yaml:"version"`
	Build            string            `yaml:"build"`
	APIVersion       string            `yaml:"api_version"`
	PriorVersions    []string          `yaml:"prior_versions"`
	About            map[string]string `yaml:"about"`
	ApplianceVersion map[string]string `yaml:"appliance_version"`
	TypeInfo         string            `yaml:"typeinfo"`
	SpecSources      []SpecSource      `yaml:"spec_sources"`
	ExecutionScope   []string          `yaml:"execution_scope"`
	TLS              TLS               `yaml:"tls"`
}

// TLS는 제품이 받아들이는 TLS 버전 범위다(R-45 ③, 2026-10-11 결정). 비우면 Gateway 기본(1.2 ~ 1.3).
type TLS struct {
	MaxVersion string `yaml:"max_version"` // "1.2" | "1.3"
	Basis      string `yaml:"basis"`
}

// MaxVersionID는 MaxVersion을 crypto/tls 상수로 바꾼다. 비어 있으면 0(제한 없음).
func (t TLS) MaxVersionID() (uint16, error) {
	switch t.MaxVersion {
	case "":
		return 0, nil
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	}
	return 0, fmt.Errorf("tls.max_version %q는 1.2 또는 1.3이어야 한다", t.MaxVersion)
}

// VCenterRules는 vCenter 버전별 동작 규칙(behavior-rules.yaml)이다(FR-VC-0418).
type VCenterRules struct {
	Capacity struct {
		Fault string `yaml:"fault"`
		Basis string `yaml:"basis"`
	} `yaml:"capacity"`
	ReconfigurePoweredOn struct {
		CPUIncreaseRequiresHotAdd   bool   `yaml:"cpu_increase_requires_hot_add"`
		CPUDecreaseRequiresPowerOff bool   `yaml:"cpu_decrease_requires_power_off"`
		MemIncreaseRequiresHotAdd   bool   `yaml:"memory_increase_requires_hot_add"`
		MemDecreaseRequiresPowerOff bool   `yaml:"memory_decrease_requires_power_off"`
		Fault                       string `yaml:"fault"`
		Basis                       string `yaml:"basis"`
	} `yaml:"reconfigure_powered_on"`
	Session struct {
		IdleTimeoutSec     int    `yaml:"idle_timeout_sec"`      // SOAP 세션 유휴 만료(FR-API-0204)
		RESTIdleTimeoutSec int    `yaml:"rest_idle_timeout_sec"` // /api 세션 유휴 만료
		Basis              string `yaml:"basis"`
	} `yaml:"session"`
}

// NSX는 NSX 프로파일이다.
type NSX struct {
	Product        string            `yaml:"product"`
	Version        string            `yaml:"version"`
	Build          string            `yaml:"build"`
	NodeVersion    map[string]string `yaml:"node_version"`
	SpecFiles      map[string]string `yaml:"spec_files"` // manager, policy
	SpecSources    []SpecSource      `yaml:"spec_sources"`
	ExecutionScope []string          `yaml:"execution_scope"`
	TLS            TLS               `yaml:"tls"`
	Behavior       struct {
		RealizationDelayMS int `yaml:"realization_delay_ms"`
		SessionTimeoutSec  int `yaml:"session_timeout_sec"` // JSESSIONID 유휴 만료(FR-API-0209)
	} `yaml:"behavior"`
}

type Verification struct {
	Commit      string                 `yaml:"commit"`
	ProfileHash string                 `yaml:"profile_hash"`
	GeneratedAt string                 `yaml:"generated_at"`
	APIs        map[string]APIEvidence `yaml:"apis"`
}

type APIEvidence struct {
	Status   string   `yaml:"status" json:"status"`
	Evidence []string `yaml:"evidence,omitempty,flow" json:"evidence,omitempty"`
	Note     string   `yaml:"note,omitempty" json:"note,omitempty"`
}

// Set은 활성 프로파일 묶음이다.
type Set struct {
	VCenterDir, NSXDir string
	VCenter            VCenter
	VCRules            VCenterRules
	NSX                NSX
	VCVerification     Verification
	NSXVerification    Verification
	Meta               *VersionMeta
	VCHash, NSXHash    string
	Interop            string // compatibility-matrix의 상호운용성 확인 상태(verified | unverified)
}

type matrix struct {
	Combinations []struct {
		VCenter string `yaml:"vcenter"`
		NSX     string `yaml:"nsx"`
		Interop string `yaml:"interop"`
	} `yaml:"combinations"`
}

// Load는 프로파일을 읽고 스키마·조합을 검증한다. 미등록·잘못된 스키마·허용되지 않은 조합은 항상 거부한다.
func Load(root, vcVersion, nsxVersion string) (*Set, error) {
	s := &Set{VCenterDir: filepath.Join(root, "vcenter", vcVersion), NSXDir: filepath.Join(root, "nsx", nsxVersion)}
	if err := readYAML(filepath.Join(s.VCenterDir, "profile.yaml"), &s.VCenter); err != nil {
		return nil, fmt.Errorf("vCenter 프로파일 %s: %w (사용 가능: %s)", vcVersion, err, available(root, "vcenter"))
	}
	if err := readYAML(filepath.Join(s.NSXDir, "profile.yaml"), &s.NSX); err != nil {
		return nil, fmt.Errorf("NSX 프로파일 %s: %w (사용 가능: %s)", nsxVersion, err, available(root, "nsx"))
	}
	var m matrix
	if err := readYAML(filepath.Join(root, "compatibility-matrix.yaml"), &m); err != nil {
		return nil, fmt.Errorf("compatibility-matrix.yaml: %w", err)
	}
	allowed := false
	for _, c := range m.Combinations {
		if c.VCenter == vcVersion && c.NSX == nsxVersion {
			allowed = true
			s.Interop = c.Interop
		}
	}
	if !allowed {
		return nil, fmt.Errorf("허용되지 않은 조합: vCenter %s × NSX %s (compatibility-matrix.yaml)", vcVersion, nsxVersion)
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := readYAML(filepath.Join(s.VCenterDir, "behavior-rules.yaml"), &s.VCRules); err != nil {
		return nil, fmt.Errorf("vCenter behavior-rules.yaml: %w", err)
	}
	_ = readYAML(filepath.Join(s.VCenterDir, "verification.yaml"), &s.VCVerification)
	_ = readYAML(filepath.Join(s.NSXDir, "verification.yaml"), &s.NSXVerification)
	s.VCHash = hashFiles(s.VCenterDir, "profile.yaml")
	s.NSXHash = hashFiles(s.NSXDir, "profile.yaml")
	meta, err := LoadVersionMeta(filepath.Join(s.VCenterDir, s.VCenter.TypeInfo))
	if err != nil {
		return nil, fmt.Errorf("버전 메타데이터: %w", err)
	}
	s.Meta = meta
	return s, nil
}

func (s *Set) validate() error {
	var errs []string
	v := s.VCenter
	if v.Version == "" || v.Build == "" || v.APIVersion == "" || v.TypeInfo == "" {
		errs = append(errs, "vCenter profile.yaml: version·build·api_version·typeinfo 필수")
	}
	if len(v.ExecutionScope) == 0 || len(s.NSX.ExecutionScope) == 0 {
		errs = append(errs, "execution_scope는 비울 수 없다(FR-API-0302)")
	}
	if s.NSX.SpecFiles["manager"] == "" || s.NSX.SpecFiles["policy"] == "" {
		errs = append(errs, "NSX profile.yaml: spec_files.manager·policy 필수")
	}
	for name, t := range map[string]TLS{"vCenter": v.TLS, "NSX": s.NSX.TLS} {
		if _, err := t.MaxVersionID(); err != nil {
			errs = append(errs, name+" profile.yaml: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("프로파일 스키마 오류: %s", strings.Join(errs, "; "))
	}
	return nil
}

// CheckMode는 기동 검증 모드의 최소 조건을 검사한다(FR-API-0302, VERSION_PROFILE_SPEC §5).
// 미달이면 API별 사유 목록을 담은 오류를 돌려준다.
func (s *Set) CheckMode(mode, commit string) error {
	type check struct {
		product string
		scope   []string
		ver     Verification
		hash    string
	}
	var problems []string
	// 상호운용성 매트릭스로 확인하지 않은 조합은 release로 기동하지 않는다(FR-API-0305, Phase 4b).
	if mode == "release" && s.Interop != "verified" {
		problems = append(problems, fmt.Sprintf("조합 vCenter %s × NSX %s: 상호운용성 미확인(compatibility-matrix interop=%q)", s.VCenter.Version, s.NSX.Version, s.Interop))
	}
	for _, c := range []check{
		{"vCenter", s.VCenter.ExecutionScope, s.VCVerification, s.VCHash},
		{"NSX", s.NSX.ExecutionScope, s.NSXVerification, s.NSXHash},
	} {
		for _, id := range c.scope {
			ev, ok := c.ver.APIs[id]
			st := NotAnalyzed
			if ok {
				var err error
				if st, err = ParseStatus(ev.Status); err != nil {
					problems = append(problems, fmt.Sprintf("%s %s: %v", c.product, id, err))
					continue
				}
			}
			switch mode {
			case "development":
				if st < ContractDefined {
					problems = append(problems, fmt.Sprintf("%s %s: %s (development 최소 Contract Defined)", c.product, id, st))
				}
			case "release":
				if st < IntegrationTested {
					problems = append(problems, fmt.Sprintf("%s %s: %s (release 최소 Integration Tested)", c.product, id, st))
				} else if c.ver.Commit != commit || c.ver.ProfileHash != c.hash {
					problems = append(problems, fmt.Sprintf("%s %s: 증거가 현재 구현(%s)·프로파일과 맞지 않아 무효", c.product, id, short(commit)))
				}
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s 모드 기동 조건 미달 %d건:\n  %s", mode, len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	return dec.Decode(v)
}

func available(root, product string) string {
	entries, _ := os.ReadDir(filepath.Join(root, product))
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		return "없음"
	}
	return strings.Join(out, ", ")
}

// HashProfile은 프로파일 식별자(profile.yaml의 SHA-256)다. 증거 무효화 판정에 쓴다.
func HashProfile(dir string) string { return hashFiles(dir, "profile.yaml") }

func hashFiles(dir string, names ...string) string {
	h := sha256.New()
	for _, n := range names {
		b, _ := os.ReadFile(filepath.Join(dir, n))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}
