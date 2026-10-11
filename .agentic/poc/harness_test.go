// Phase 0a PoC 공통 하네스.
// PRD-4 Phase 0a: 코드는 PoC 범위로 한정하며 본 구현(30-implementation/)이 아니다.
package poc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

const simDomain = "cloudinfra-sim.test"

// defaultSmall은 NFR-0108 기준 Fixture(default-small)의 vCenter 부분이다.
// vcsim VPX 모델은 Host를 클러스터 단위로 만들므로 Cluster 1, Host 3으로 맞춘다.
func defaultSmall() *simulator.Model {
	m := simulator.VPX()
	m.Datacenter = 1
	m.Cluster = 1
	m.ClusterHost = 3
	m.Host = 0
	m.Datastore = 2
	m.Portgroup = 4
	m.Machine = 20 // VPX 모델은 리소스 풀당 Machine 개수를 만든다
	return m
}

// newCert는 SAN *.<도메인>을 가진 자체 서명 인증서를 만든다(NFR-0313).
func newCert(t testing.TB) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cloudinfra-sim PoC CA"},
		DNSNames:              []string{"*." + simDomain},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// gateway는 PoC-12의 내장 Gateway다. 하나의 TLS 리스너에서 SNI로 vCenter와 NSX를 분기한다.
type gateway struct {
	addr  string
	pool  *x509.CertPool
	close func()
}

func startGateway(t testing.TB, bindIP string, routes map[string]http.Handler) *gateway {
	t.Helper()
	cert, pool := newCert(t)
	ln, err := net.Listen("tcp", net.JoinHostPort(bindIP, "0"))
	if err != nil {
		t.Fatalf("listen %s: %v", bindIP, err)
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sni := r.TLS.ServerName
			svc, _, _ := strings.Cut(sni, ".")
			h, ok := routes[svc]
			if !ok || !strings.HasSuffix(sni, "."+simDomain) {
				http.Error(w, "unknown virtual host: "+sni, http.StatusMisdirectedRequest)
				return
			}
			h.ServeHTTP(w, r)
		}),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	return &gateway{addr: ln.Addr().String(), pool: pool, close: func() { _ = srv.Close() }}
}

// transport는 *.cloudinfra-sim.test를 Gateway 주소로 해석한다(hosts 파일 대체).
func (g *gateway) transport() *http.Transport {
	d := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: g.pool},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return d.DialContext(ctx, network, g.addr)
		},
	}
}

// newVCenterHandler는 vcsim 모델을 만들고 /sdk 등 vCenter 경로의 http.Handler를 돌려준다.
// vcsim 자체 리스너는 사용하지 않고 ServeMux만 Gateway에 연결한다.
func newVCenterHandler(t testing.TB, m *simulator.Model) (*simulator.Model, http.Handler) {
	t.Helper()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	s := m.Service.NewServer()
	t.Cleanup(func() { s.Close(); m.Remove() })
	return m, m.Service.ServeMux
}

// login은 govmomi(PoC-20 후보 SDK)로 Gateway를 통해 vCenter에 로그인한다.
func login(t testing.TB, g *gateway) *govmomi.Client {
	t.Helper()
	ctx := context.Background()
	u, _ := url.Parse("https://vcenter." + simDomain + "/sdk")
	sc := soap.NewClient(u, false)
	// govmomi는 자체 DialTLSContext를 쓰므로 해제하고, 표준 TLS 경로(URL 호스트명 = SNI)를 사용한다.
	tr := sc.DefaultTransport()
	tr.TLSClientConfig = &tls.Config{RootCAs: g.pool}
	tr.DialTLSContext = nil
	tr.DialContext = g.transport().DialContext
	vc, err := vim25.NewClient(ctx, sc)
	if err != nil {
		t.Fatal(err)
	}
	c := &govmomi.Client{Client: vc, SessionManager: session.NewManager(vc)}
	if err := c.Login(ctx, simulator.DefaultLogin); err != nil {
		t.Fatal(err)
	}
	return c
}
