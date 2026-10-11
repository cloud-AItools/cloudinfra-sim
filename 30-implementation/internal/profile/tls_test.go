package profile

import (
	"crypto/tls"
	"testing"
)

func TestTLSMaxVersionID(t *testing.T) {
	for in, want := range map[string]uint16{"": 0, "1.2": tls.VersionTLS12, "1.3": tls.VersionTLS13} {
		if got, err := (TLS{MaxVersion: in}).MaxVersionID(); err != nil || got != want {
			t.Errorf("%q → %#x, %v; %#x 기대", in, got, err, want)
		}
	}
	for _, bad := range []string{"1.1", "TLS1.2", "1.4"} {
		if _, err := (TLS{MaxVersion: bad}).MaxVersionID(); err == nil {
			t.Errorf("%q를 받아들였다", bad)
		}
	}
	s := &Set{VCenter: VCenter{Version: "7", Build: "1", APIVersion: "7", TypeInfo: "x", ExecutionScope: []string{"a"}, TLS: TLS{MaxVersion: "1.1"}},
		NSX: NSX{ExecutionScope: []string{"a"}, SpecFiles: map[string]string{"manager": "m", "policy": "p"}}}
	if err := s.validate(); err == nil {
		t.Error("잘못된 tls.max_version으로 스키마 검증을 통과했다")
	}
}
