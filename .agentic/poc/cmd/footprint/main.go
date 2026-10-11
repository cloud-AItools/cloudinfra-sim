// footprint는 PoC-19(Windows 네이티브 실행·기본 Fixture 리소스 예산 측정)용 실행파일이다.
// 기본 Fixture(default-small)의 vcsim과 SNI Gateway를 띄우고 첫 응답까지의 시간을 출력한 뒤 유휴 상태로 대기한다.
// 메모리·CPU는 외부(PowerShell Get-Process)에서 측정한다. PoC 코드이며 본 구현이 아니다.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"cloudinfra-sim/poc/nsxspec"

	"github.com/vmware/govmomi/simulator"
)

const domain = "cloudinfra-sim.test"

func main() {
	start := time.Now()
	bind := flag.String("bind", "127.0.0.2:443", "vCenter listen address")
	idle := flag.Duration("idle", 30*time.Second, "idle time before exit")
	nsxDir := flag.String("nsxspec", "", "NSX spec directory (nsx_api.json, nsx_policy_api.json); empty = vCenter only")
	flag.Parse()

	m := simulator.VPX()
	m.Datacenter, m.Cluster, m.ClusterHost, m.Host = 1, 1, 3, 0
	m.Datastore, m.Portgroup, m.Machine = 2, 4, 20
	if err := m.Create(); err != nil {
		fail(err)
	}
	s := m.Service.NewServer()
	defer s.Close()

	cert, pool := newCert()
	ln, err := net.Listen("tcp", *bind)
	if err != nil {
		fail(fmt.Errorf("bind %s: %w", *bind, err))
	}
	vc := m.Service.ServeMux
	var nsx http.Handler
	if *nsxDir != "" {
		var specs []*nsxspec.Spec
		for _, f := range []string{"nsx_api.json", "nsx_policy_api.json"} {
			s, err := nsxspec.Load(filepath.Join(*nsxDir, f))
			if err != nil {
				fail(err)
			}
			specs = append(specs, s)
		}
		nsx = nsxspec.NewHandler(specs...)
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.TLS.ServerName, "vcenter.") {
				vc.ServeHTTP(w, r)
				return
			}
			if nsx != nil && strings.HasPrefix(r.TLS.ServerName, "nsx.") {
				nsx.ServeHTTP(w, r)
				return
			}
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
		}),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	go func() { _ = srv.ServeTLS(ln, "", "") }()

	// 첫 API 응답: /sdk/vimServiceVersions.xml (NFR-0110의 '첫 API 응답 가능')
	tr :=&http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "vcenter." + domain}}
	tr.Dial = func(_, _ string) (net.Conn, error) { return net.Dial("tcp", ln.Addr().String()) }
	res, err := (&http.Client{Transport: tr}).Get("https://vcenter." + domain + "/sdk/vimServiceVersions.xml")
	if err != nil {
		fail(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	tr.CloseIdleConnections()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	nsxStatus := 0
	if nsx != nil {
		tr.TLSClientConfig.ServerName = "nsx." + domain
		res, err := (&http.Client{Transport: tr}).Get("https://nsx." + domain + "/policy/api/v1/infra/segments/none")
		if err != nil {
			fail(err)
		}
		nsxStatus = res.StatusCode // 404(객체 없음)면 경로 색인·라우팅 정상
		res.Body.Close()
		tr.CloseIdleConnections()
	}
	fmt.Printf("nsx_first_status=%d ", nsxStatus)
	fmt.Printf("READY pid=%d bind=%s first_response_ms=%d status=%d go_heap_inuse_mb=%.1f go_sys_mb=%.1f vms=%d\n",
		os.Getpid(), ln.Addr(), time.Since(start).Milliseconds(), res.StatusCode,
		float64(ms.HeapInuse)/1e6, float64(ms.Sys)/1e6, len(m.Map().All("VirtualMachine")))
	time.Sleep(*idle)
}

func newCert() (tls.Certificate, *x509.CertPool) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "cloudinfra-sim PoC"},
		DNSNames: []string{"*." + domain}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}
