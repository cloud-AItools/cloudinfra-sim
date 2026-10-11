package config

import "testing"

// T-NFR-0313 / T-NFR-0314: 도메인은 옵션·환경변수로 바꿀 수 있고 예약 TLD(.test, .internal)만 허용한다.
func TestDomainRules(t *testing.T) {
	base := []string{"--vcenter-version", "7.0.3.02500", "--nsx-version", "3.2.4.3"}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for _, tc := range []struct {
		args []string
		env  map[string]string
		ok   bool
		host string
	}{
		{nil, nil, true, "vcenter.cloudinfra-sim.test"},
		{[]string{"--domain", "lab.internal"}, nil, true, "vcenter.lab.internal"},
		{nil, map[string]string{"SIM_DOMAIN": "team.test"}, true, "vcenter.team.test"},
		{[]string{"--domain", "lab.local"}, nil, false, ""},
		{[]string{"--domain", "sim.dev"}, nil, false, ""},
		{[]string{"--domain", "corp.lab"}, nil, false, ""},
	} {
		c, err := Parse(append(append([]string{}, base...), tc.args...), env(tc.env))
		if (err == nil) != tc.ok {
			t.Fatalf("%v %v: ok=%v err=%v", tc.args, tc.env, tc.ok, err)
		}
		if tc.ok && c.Host("vcenter") != tc.host {
			t.Fatalf("host %s, want %s", c.Host("vcenter"), tc.host)
		}
	}
}

// T-FR-API-0301 / T-FR-API-0302: 버전 입력 필수, 검증 모드 기본값은 빌드 종류를 따른다.
func TestVersionAndModeDefaults(t *testing.T) {
	if _, err := Parse(nil, func(string) string { return "" }); err == nil {
		t.Fatal("versions must be required")
	}
	c, err := Parse(nil, func(k string) string {
		return map[string]string{"SIM_VCENTER_VERSION": "7.0.3.02500", "SIM_NSX_VERSION": "3.2.4.3"}[k]
	})
	// T-NFR-0301 T-NFR-ARC-0309: 기본 바인딩은 루프백(토폴로지 ①, D6)
	if c.VCenterAddr != "127.0.0.2:443" || c.NSXAddr != "127.0.0.3:443" || c.AdminAddr != "127.0.0.1:9000" {
		t.Fatalf("default bindings: %s %s %s", c.VCenterAddr, c.NSXAddr, c.AdminAddr)
	}
	if err != nil || c.VerificationMode != "development" {
		t.Fatalf("development build default: %v %v", c, err)
	}
	BuildKind = "release"
	defer func() { BuildKind = "development" }()
	c, _ = Parse([]string{"--vcenter-version", "x", "--nsx-version", "y"}, func(string) string { return "" })
	if c.VerificationMode != "release" {
		t.Fatalf("release build default: %s", c.VerificationMode)
	}
}

// T-NFR-0508: 보존 한도는 설정으로 조정하되 무제한(음수·상한 초과·숫자 아님)은 거부한다. 0은 실행 프로파일 기본값.
func TestRetentionLimits(t *testing.T) {
	base := []string{"--vcenter-version", "x", "--nsx-version", "y"}
	none := func(string) string { return "" }
	for _, tc := range []struct {
		args []string
		env  map[string]string
		ok   bool
	}{
		{nil, nil, true},
		{[]string{"--retain-tasks", "50", "--retain-trace", "100"}, nil, true},
		{[]string{"--retain-tasks", "-1"}, nil, false},
		{[]string{"--retain-tasks", "100001"}, nil, false},
		{[]string{"--retain-transitions", "5000001"}, nil, false},
		{nil, map[string]string{"SIM_RETAIN_OPERATIONS": "lots"}, false},
		{nil, map[string]string{"SIM_RETAIN_OPERATIONS": "700"}, true},
	} {
		get := none
		if tc.env != nil {
			get = func(k string) string { return tc.env[k] }
		}
		c, err := Parse(append(append([]string{}, base...), tc.args...), get)
		if (err == nil) != tc.ok {
			t.Fatalf("%v %v: err=%v want ok=%v", tc.args, tc.env, err, tc.ok)
		}
		if tc.ok && tc.env != nil && c.RetainOperations != 700 {
			t.Fatalf("env retain-operations: %d", c.RetainOperations)
		}
	}
	if Pick(0, 200) != 200 || Pick(7, 200) != 7 {
		t.Fatal("Pick")
	}
}
