// 수신 TLS 버전 계약(R-45 ③, 2026-10-11 결정): vCenter 7.0.x·NSX-T 3.2.x 프로파일은 tls.max_version 1.2.
// 실제 제품은 vSphere 8.0 U3·NSX 4.1.2부터 TLS 1.3을 쓴다(KB 377237·319144). TLS 1.3만 쓰는 클라이언트는 거부돼야 한다.
package contract

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"cloudinfra-sim/test/internal/testenv"
)

// T-NFR-0302 T-FR-API-0306: HTTPS 수신 버전 범위는 프로파일이 정한다.
func TestListenerTLSMaxVersion(t *testing.T) {
	env := testenv.Start(t, nil)
	if env.Sim.Profile.VCenter.TLS.MaxVersion != "1.2" || env.Sim.Profile.NSX.TLS.MaxVersion != "1.2" {
		t.Fatalf("프로파일 tls.max_version: vCenter %q NSX %q, 1.2 기대", env.Sim.Profile.VCenter.TLS.MaxVersion, env.Sim.Profile.NSX.TLS.MaxVersion)
	}
	for _, svc := range []string{"vcenter", "nsx"} {
		addr := env.Sim.Addrs[svc].String()
		name := svc + "." + testenv.Domain
		dial := func(min, max uint16) (*tls.Conn, error) {
			d := &net.Dialer{Timeout: 5 * time.Second}
			return tls.DialWithDialer(d, "tcp", addr, &tls.Config{RootCAs: env.Sim.Certs.CAPool, ServerName: name, MinVersion: min, MaxVersion: max})
		}
		// 기본 클라이언트(1.2 ~ 1.3 제시)는 1.2로 합의한다.
		c, err := dial(tls.VersionTLS12, 0)
		if err != nil {
			t.Fatalf("%s 기본 클라이언트 접속 실패: %v", svc, err)
		}
		if v := c.ConnectionState().Version; v != tls.VersionTLS12 {
			t.Errorf("%s 합의 버전 %#x, TLS 1.2 기대", svc, v)
		}
		c.Close()
		// TLS 1.3만 제시하면 핸드셰이크가 실패한다(실제 7.0·3.2와 같음).
		if c, err := dial(tls.VersionTLS13, tls.VersionTLS13); err == nil {
			c.Close()
			t.Errorf("%s: TLS 1.3 전용 클라이언트가 접속됐다", svc)
		}
		// 1.2 미만은 계속 거부한다(하한 1.2).
		if c, err := dial(tls.VersionTLS10, tls.VersionTLS11); err == nil {
			c.Close()
			t.Errorf("%s: TLS 1.1 클라이언트가 접속됐다", svc)
		}
	}
}
