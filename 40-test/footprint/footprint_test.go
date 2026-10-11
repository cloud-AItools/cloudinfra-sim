// 리소스 예산 측정 (WP 1.10, NFR-0107 ~ NFR-0113, AC-18). 기준: RESOURCE_BUDGET.md §1·§2 (D4 확정).
// 실제 실행파일을 빌드해 별도 프로세스로 띄우고 측정한다. 실행: go test ./footprint -run Footprint -v
package footprint

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloudinfra-sim/test/internal/testenv"
)

const (
	budgetMemoryMB  = 300
	budgetStartup   = 5 * time.Second
	budgetIdleCPU   = 50 * time.Millisecond // 10초 유휴 동안
	budgetInstallMB = 200
)

func freePort(t *testing.T, ip string) string {
	t.Helper()
	ln, err := net.Listen("tcp", ip+":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// 추적: T-NFR-0104 T-NFR-0105 T-NFR-0107 T-NFR-0113 T-NFR-0114 T-NFR-ARC-0108
func TestFootprint(t *testing.T) {
	if testing.Short() {
		t.Skip("footprint는 실행파일을 빌드한다")
	}
	repo := testenv.Repo()
	impl := filepath.Join(repo, "30-implementation")
	bin := filepath.Join(t.TempDir(), "cloudinfra-sim"+exeSuffix())
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin, "./cmd/cloudinfra-sim")
	build.Dir = impl
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// RESOURCE_BUDGET §2: 첫 실행(새 실행파일의 디스크 캐시·백신 검사 영향)은 판정에서 제외한다. 한 번 실행해 둔다.
	if out, err := exec.Command(bin, "version").CombinedOutput(); err != nil {
		t.Fatalf("warm-up: %v %s", err, out)
	}

	// NFR-0112: 실행파일 + 활성 프로파일 1개 조합(vCenter·NSX)
	install := fileSize(t, bin) + dirSize(t, filepath.Join(impl, "data", "compatibility-profiles", "vcenter", "7.0.3.02500")) +
		dirSize(t, filepath.Join(impl, "data", "compatibility-profiles", "nsx", "3.2.4.3"))
	installMB := float64(install) / 1e6

	data := t.TempDir()
	vcAddr, nsxAddr := freePort(t, "127.0.0.2"), freePort(t, "127.0.0.3")
	cmd := exec.Command(bin, "--vcenter-version", "7.0.3.02500", "--nsx-version", "3.2.4.3",
		"--vcenter-addr", vcAddr, "--nsx-addr", nsxAddr, "--admin-addr", freePort(t, "127.0.0.1"),
		"--data-dir", data, "--profiles-dir", filepath.Join(impl, "data", "compatibility-profiles"),
		"--fixture", filepath.Join(impl, "data", "fixtures", "default-small.yaml"), "--log-level", "warn")
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	// NFR-0110: 프로세스 시작 → vCenter·NSX 첫 API 응답
	var pool *x509.CertPool
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(filepath.Join(data, "certs", "ca.pem")); err == nil {
			pool = x509.NewCertPool()
			if pool.AppendCertsFromPEM(b) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pool == nil {
		t.Fatal("CA not created")
	}
	client := func(addr string) *http.Client {
		return &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
			DialContext: func(ctx context.Context, n, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, n, addr)
			},
		}}
	}
	ready := func(c *http.Client, url string) bool {
		res, err := c.Get(url)
		if err != nil {
			return false
		}
		res.Body.Close()
		return res.StatusCode < 500
	}
	vcReady, nsxReady := false, false
	for time.Now().Before(deadline) && !(vcReady && nsxReady) {
		vcReady = vcReady || ready(client(vcAddr), "https://vcenter."+testenv.Domain+"/sdk/vimServiceVersions.xml")
		nsxReady = nsxReady || ready(client(nsxAddr), "https://nsx."+testenv.Domain+"/api/v1/node/version") // 403(미인증)도 응답 가능
		time.Sleep(10 * time.Millisecond)
	}
	startup := time.Since(start)
	if !vcReady || !nsxReady {
		t.Fatalf("not ready after %v (vcenter=%v nsx=%v)", startup, vcReady, nsxReady)
	}

	time.Sleep(5 * time.Second) // 유휴 진입
	mem0, cpu0 := sample(t, cmd.Process.Pid)
	time.Sleep(10 * time.Second)
	mem1, cpu1 := sample(t, cmd.Process.Pid)
	memMB := max(mem0, mem1) / 1e6
	idleCPU := cpu1 - cpu0

	t.Logf("startup=%v memory(max of private/working set or RSS)=%.1f MB idleCPU(10s)=%v install=%.1f MB",
		startup.Round(time.Millisecond), memMB, idleCPU, installMB)
	if startup > budgetStartup {
		t.Errorf("T-NFR-0110: startup %v > %v", startup, budgetStartup)
	}
	if memMB > budgetMemoryMB {
		t.Errorf("T-NFR-0109: memory %.1f MB > %d MB", memMB, budgetMemoryMB)
	}
	if idleCPU > budgetIdleCPU {
		t.Errorf("T-NFR-0111: idle CPU %v > %v", idleCPU, budgetIdleCPU)
	}
	if installMB > budgetInstallMB {
		t.Errorf("T-NFR-0112: install %.1f MB > %d MB", installMB, budgetInstallMB)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// sample은 메모리(바이트)와 누적 CPU 시간을 읽는다. Windows는 Private·Working Set 중 큰 값, Linux는 RSS.
func sample(t *testing.T, pid int) (float64, time.Duration) {
	t.Helper()
	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf("$p=Get-Process -Id %d; \"$($p.PrivateMemorySize64) $($p.WorkingSet64) $($p.TotalProcessorTime.TotalMilliseconds)\"", pid)).Output()
		if err != nil {
			t.Fatal(err)
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		priv, _ := strconv.ParseFloat(f[0], 64)
		ws, _ := strconv.ParseFloat(f[1], 64)
		cpu, _ := strconv.ParseFloat(f[2], 64)
		return max(priv, ws), time.Duration(cpu * float64(time.Millisecond))
	}
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		t.Skipf("memory sampling unsupported on %s", runtime.GOOS)
	}
	var rss float64
	for _, l := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(l, "VmRSS:") {
			kb, _ := strconv.ParseFloat(strings.Fields(l)[1], 64)
			rss = kb * 1024
		}
	}
	stat, _ := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	f := strings.Fields(string(stat))
	ut, _ := strconv.ParseFloat(f[13], 64)
	st, _ := strconv.ParseFloat(f[14], 64)
	return rss, time.Duration((ut + st) * float64(time.Second) / 100)
}

func fileSize(t *testing.T, p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

func dirSize(t *testing.T, dir string) int64 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n += fi.Size()
		}
		return nil
	})
	return n
}
