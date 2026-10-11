// Package config는 실행 설정(명령행 옵션과 환경변수)을 해석한다.
// 관련: FR-API-0301(버전 입력), FR-API-0302(검증 모드), NFR-0313(도메인), NFR-0304(Credential), NFR-0114(lite).
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildKind는 빌드 종류다. 배포 패키지는 ldflags로 "release"를 넣는다(FR-API-0302 기본값 결정).
var BuildKind = "development"

type Config struct {
	VCenterVersion   string // 예: 7.0.3.02500
	NSXVersion       string // 예: 3.2.4.3
	Domain           string // 기본 cloudinfra-sim.test
	VCenterAddr      string // 기본 127.0.0.2:443 (vCenter·ESXi)
	NSXAddr          string // 기본 127.0.0.3:443
	AdminAddr        string // 기본 127.0.0.1:9000
	DataDir          string // 인증서·자격 증명·런타임 데이터
	ProfilesDir      string // 호환성 프로파일 루트
	FixtureFile      string // Seed Fixture
	ScenariosDir     string // 선언형 시나리오(Admin API Runner, WP 5.1)
	VerificationMode string // release | development
	RunProfile       string // lite | full
	LogFormat        string // text | json
	LogLevel         string // debug | info | warn | error

	// 보존 한도 조정(NFR-0508). 0이면 실행 프로파일 기본값(RESOURCE_BUDGET §4), 음수·상한 초과는 거부(무제한 금지).
	RetainTasks       int // 완료 Task 객체(lite 200, full 10,000)
	RetainTransitions int // Operation Transition(lite 5,000, full 500,000)
	RetainOperations  int // 완료 Operation(lite 5,000, full 500,000)
	RetainTrace       int // API Trace 메타데이터(lite 10,000)

	// Phase 5(WP 5.2·5.9)
	RetainSnapshots int    // 보존 Snapshot 수(lite 5, full 20)
	RetainDays      int    // full 기간 한도(일, RESOURCE_BUDGET §4: 7). lite는 기간 한도 없음(건수만)
	RetainTraceMB   int    // full Trace 본문 총용량(MB, 2,048)
	RestoreSnapshot string // 기동 시 복원할 Snapshot 이름(FR-BE-0208). "last"는 가장 최근, 비면 Fixture로 시작
	// FixtureCache는 vCenter Fixture 인벤토리 캐시다(R-45 ②): auto(VM FixtureCacheMinVMs개 이상) | on | off
	FixtureCache string
}

// FixtureCacheMinVMs는 auto에서 캐시를 쓰는 최소 VM 수다(작은 Fixture는 새로 만드는 편이 빠르다).
const FixtureCacheMinVMs = 200

// Parse는 명령행 인자와 환경변수(SIM_*)를 읽는다. 명령행이 환경변수보다 우선한다.
func Parse(args []string, getenv func(string) string) (*Config, error) {
	env := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	fs := flag.NewFlagSet("cloudinfra-sim", flag.ContinueOnError)
	c := &Config{}
	fs.StringVar(&c.VCenterVersion, "vcenter-version", env("SIM_VCENTER_VERSION", ""), "vCenter 프로파일 버전 (예: 7.0.3.02500)")
	fs.StringVar(&c.NSXVersion, "nsx-version", env("SIM_NSX_VERSION", ""), "NSX 프로파일 버전 (예: 3.2.4.3)")
	fs.StringVar(&c.Domain, "domain", env("SIM_DOMAIN", "cloudinfra-sim.test"), "가상 호스트 도메인")
	fs.StringVar(&c.VCenterAddr, "vcenter-addr", env("SIM_VCENTER_ADDR", "127.0.0.2:443"), "vCenter·ESXi 수신 주소")
	fs.StringVar(&c.NSXAddr, "nsx-addr", env("SIM_NSX_ADDR", "127.0.0.3:443"), "NSX 수신 주소")
	fs.StringVar(&c.AdminAddr, "admin-addr", env("SIM_ADMIN_ADDR", "127.0.0.1:9000"), "Admin API 수신 주소(루프백)")
	fs.StringVar(&c.DataDir, "data-dir", env("SIM_DATA_DIR", defaultDataDir()), "데이터 디렉터리")
	fs.StringVar(&c.ProfilesDir, "profiles-dir", env("SIM_PROFILES_DIR", "data/compatibility-profiles"), "호환성 프로파일 디렉터리")
	fs.StringVar(&c.FixtureFile, "fixture", env("SIM_FIXTURE", "data/fixtures/default-small.yaml"), "Seed Fixture 파일")
	fs.StringVar(&c.ScenariosDir, "scenarios-dir", env("SIM_SCENARIOS_DIR", "data/scenarios"), "선언형 시나리오 디렉터리")
	fs.StringVar(&c.VerificationMode, "verification-mode", env("SIM_VERIFICATION_MODE", defaultMode()), "release | development")
	fs.StringVar(&c.RunProfile, "run-profile", env("SIM_RUN_PROFILE", "lite"), "lite | full")
	fs.StringVar(&c.LogFormat, "log-format", env("SIM_LOG_FORMAT", "text"), "text | json")
	fs.StringVar(&c.LogLevel, "log-level", env("SIM_LOG_LEVEL", "info"), "debug | info | warn | error")
	fs.IntVar(&c.RetainTasks, "retain-tasks", envInt(getenv, "SIM_RETAIN_TASKS"), "완료 Task 보존 수(0=기본)")
	fs.IntVar(&c.RetainTransitions, "retain-transitions", envInt(getenv, "SIM_RETAIN_TRANSITIONS"), "상태 전이 보존 수(0=기본)")
	fs.IntVar(&c.RetainOperations, "retain-operations", envInt(getenv, "SIM_RETAIN_OPERATIONS"), "완료 Operation 보존 수(0=기본)")
	fs.IntVar(&c.RetainTrace, "retain-trace", envInt(getenv, "SIM_RETAIN_TRACE"), "API Trace 보존 수(0=기본)")
	fs.IntVar(&c.RetainSnapshots, "retain-snapshots", envInt(getenv, "SIM_RETAIN_SNAPSHOTS"), "보존 Snapshot 수(0=기본)")
	fs.IntVar(&c.RetainDays, "retain-days", envInt(getenv, "SIM_RETAIN_DAYS"), "full 기간 보존 한도(일, 0=기본 7)")
	fs.IntVar(&c.RetainTraceMB, "retain-trace-mb", envInt(getenv, "SIM_RETAIN_TRACE_MB"), "full Trace 본문 총용량(MB, 0=기본 2048)")
	fs.StringVar(&c.RestoreSnapshot, "restore-snapshot", env("SIM_RESTORE_SNAPSHOT", ""), "기동 시 복원할 Snapshot 이름 또는 last(비면 Fixture로 시작)")
	fs.StringVar(&c.FixtureCache, "fixture-cache", env("SIM_FIXTURE_CACHE", "auto"), "vCenter Fixture 인벤토리 캐시: auto(VM 200개 이상) | on | off")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return c, c.validate()
}

func defaultMode() string {
	if BuildKind == "release" {
		return "release"
	}
	return "development"
}

func defaultDataDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "cloudinfra-sim")
	}
	return "sim-data"
}

func (c *Config) validate() error {
	var errs []string
	if c.VCenterVersion == "" {
		errs = append(errs, "--vcenter-version(SIM_VCENTER_VERSION)이 필요하다")
	}
	if c.NSXVersion == "" {
		errs = append(errs, "--nsx-version(SIM_NSX_VERSION)이 필요하다")
	}
	if c.VerificationMode != "release" && c.VerificationMode != "development" {
		errs = append(errs, "--verification-mode는 release 또는 development")
	}
	if c.RunProfile != "lite" && c.RunProfile != "full" {
		errs = append(errs, "--run-profile은 lite 또는 full")
	}
	for _, r := range []struct {
		name   string
		v, max int
	}{{"--retain-tasks", c.RetainTasks, MaxRetainTasks}, {"--retain-transitions", c.RetainTransitions, MaxRetainRecords},
		{"--retain-operations", c.RetainOperations, MaxRetainRecords}, {"--retain-trace", c.RetainTrace, MaxRetainRecords},
		{"--retain-snapshots", c.RetainSnapshots, MaxRetainSnapshots}, {"--retain-days", c.RetainDays, MaxRetainDays}, {"--retain-trace-mb", c.RetainTraceMB, MaxRetainTraceMB}} {
		if r.v < 0 || r.v > r.max {
			errs = append(errs, fmt.Sprintf("%s는 0(기본) 또는 1~%d(무제한 보존 금지, NFR-0508)", r.name, r.max))
		}
	}
	if c.FixtureCache != "auto" && c.FixtureCache != "on" && c.FixtureCache != "off" {
		errs = append(errs, "--fixture-cache는 auto, on, off")
	}
	if err := checkDomain(c.Domain); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// checkDomain은 예약 TLD 원칙을 적용한다(NFR-0314): .test 기본, 팀 공용 내부 DNS는 .internal 허용.
func checkDomain(d string) error {
	d = strings.ToLower(strings.TrimSuffix(d, "."))
	switch {
	case strings.HasSuffix(d, ".test"), strings.HasSuffix(d, ".internal"):
		return nil
	default:
		return fmt.Errorf("도메인 %q: 예약 TLD(.test 또는 .internal)만 허용한다(NFR-0314)", d)
	}
}

// Host는 서비스 호스트 이름을 만든다(NFR-0313): <서비스>.<도메인>.
func (c *Config) Host(service string) string { return service + "." + c.Domain }

// 보존 한도 상한(가정: full 기본값의 10배, RESOURCE_BUDGET §4). 무제한 보존을 막는다(NFR-0508).
const (
	MaxRetainTasks   = 100000
	MaxRetainRecords = 5000000
	// Phase 5: Snapshot 수·기간·Trace 용량 상한(가정, full 기본값의 5~10배)
	MaxRetainSnapshots = 200
	MaxRetainDays      = 90
	MaxRetainTraceMB   = 20480
)

// Pick은 설정값 v가 0이면 기본값 def를 쓴다.
func Pick(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func envInt(getenv func(string) string, key string) int {
	s := getenv(key)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1 // 숫자가 아니면 검증에서 거부
	}
	return n
}
