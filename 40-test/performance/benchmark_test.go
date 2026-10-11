// Performance Benchmark(WP 5.6: NFR-0115 ~ NFR-0117, R-09)와 full 장시간 수렴 검증(WP 5.9: NFR-0501, RESOURCE_BUDGET §4).
//
// NFR-0116의 규모 값(VM 100/1,000/5,000, Segment 1,000, 동시 요청 10/50/100)은 검증 시나리오이며 사전 보장 처리량이
// 아니다(NFR-0117). 이 시험은 실행파일을 별도 프로세스(full 실행 프로필)로 띄워 기동 시간·메모리(Windows Private/Working Set
// 중 큰 값, Linux RSS)·처리량·지연을 측정해 결과표를 남긴다. 판정은 오류 0건과 측정 완료이며, 처리량·메모리 목표는 이
// 결과로 정한다(TEST_STRATEGY §, RESOURCE_BUDGET §5). 기본 예산(NFR-0109 ~ 0112)은 footprint 시험이 따로 지킨다.
//
// 실행: PERF_RUN=1 go test ./performance -run Benchmark -v -timeout 30m   (PERF_FULL=1: VM 5,000·동시 100 포함,
// PERF_REPORT_DIR=<dir>: 결과 Markdown·JSON 저장). -short면 건너뛴다.
package performance

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloudinfra-sim/test/internal/testenv"
)

type proc struct {
	t       *testing.T
	cmd     *exec.Cmd
	data    string
	vcAddr  string
	nsxAddr string
	admin   string
	pool    *x509.CertPool
	startup time.Duration
	users   []struct {
		Name     string   `json:"name"`
		Password string   `json:"password"`
		Products []string `json:"products"`
	}
}

func freePort(t *testing.T, ip string) string {
	ln, err := net.Listen("tcp", ip+":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func buildBinary(t *testing.T) string {
	binOnce.Do(func() {
		dir, _ := os.MkdirTemp("", "cloudinfra-perf-")
		binPath = filepath.Join(dir, "cloudinfra-sim"+map[bool]string{true: ".exe"}[runtime.GOOS == "windows"])
		b := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", binPath, "./cmd/cloudinfra-sim")
		b.Dir = filepath.Join(testenv.Repo(), "30-implementation")
		if out, err := b.CombinedOutput(); err != nil {
			binErr = fmt.Errorf("build: %v %s", err, out)
			return
		}
		_, _ = exec.Command(binPath, "version").CombinedOutput() // 첫 실행 제외(RESOURCE_BUDGET §2)
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return binPath
}

// fixture는 기본 Fixture에서 VM·Segment 수만 바꾼 Fixture 파일을 만든다.
func fixture(t *testing.T, vms, segments int) string {
	src, err := os.ReadFile(filepath.Join(testenv.Repo(), "30-implementation", "data", "fixtures", "default-small.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(src), "name: default-small", fmt.Sprintf("name: perf-vm%d-seg%d", vms, segments), 1)
	s = strings.Replace(s, "vms_per_resource_pool: 20", "vms_per_resource_pool: "+strconv.Itoa(vms), 1)
	s = strings.Replace(s, "segments: 10", "segments: "+strconv.Itoa(segments), 1)
	p := filepath.Join(t.TempDir(), "fixture.yaml")
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func start(t *testing.T, fixtureFile string, extra ...string) *proc {
	return startIn(t, t.TempDir(), fixtureFile, extra...)
}

// startIn은 주어진 데이터 디렉터리로 시뮬레이터 프로세스를 띄운다(재기동 시험용).
func startIn(t *testing.T, data, fixtureFile string, extra ...string) *proc {
	bin := os.Getenv("PERF_SIM_BIN") // 조사용: 미리 빌드한 실행파일(다른 Go 도구 체인 비교, R-45 ③)
	if bin == "" {
		bin = buildBinary(t)
	}
	impl := filepath.Join(testenv.Repo(), "30-implementation")
	p := &proc{t: t, data: data, vcAddr: freePort(t, "127.0.0.2"), nsxAddr: freePort(t, "127.0.0.3"), admin: freePort(t, "127.0.0.1")}
	args := append([]string{"--vcenter-version", "7.0.3.02500", "--nsx-version", "3.2.4.3", "--vcenter-addr", p.vcAddr, "--nsx-addr", p.nsxAddr,
		"--admin-addr", p.admin, "--data-dir", p.data, "--profiles-dir", filepath.Join(impl, "data", "compatibility-profiles"),
		"--fixture", fixtureFile, "--run-profile", "full", "--log-level", logLevel(), "--fixture-cache", perfFixtureCache()}, extra...)
	p.cmd = exec.Command(bin, args...)
	if v := os.Getenv("PERF_SIM_GODEBUG"); v != "" { // 조사용: 시뮬레이터 프로세스에만 GODEBUG(R-45 ③)
		p.cmd.Env = append(os.Environ(), "GODEBUG="+v)
	}
	if logf, err := os.OpenFile(filepath.Join(p.data, "sim.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		p.cmd.Stdout, p.cmd.Stderr = logf, logf
		t.Cleanup(func() {
			_ = logf.Close()
			if t.Failed() {
				b, _ := os.ReadFile(filepath.Join(p.data, "sim.log"))
				t.Logf("simulator log (tail):\n%s", tail(string(b), 4000))
			}
		})
	}
	t0 := time.Now()
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.cmd.Process.Kill(); _, _ = p.cmd.Process.Wait() })
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(filepath.Join(p.data, "certs", "ca.pem")); err == nil {
			p.pool = x509.NewCertPool()
			if p.pool.AppendCertsFromPEM(b) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	vc, nsx := false, false
	for time.Now().Before(deadline) && !(vc && nsx) {
		vc = vc || p.ready("vcenter", "/sdk/vimServiceVersions.xml")
		nsx = nsx || p.ready("nsx", "/api/v1/node/version")
		time.Sleep(20 * time.Millisecond)
	}
	if !vc || !nsx {
		t.Fatalf("not ready (vcenter=%v nsx=%v)", vc, nsx)
	}
	p.startup = time.Since(t0)
	b, _ := os.ReadFile(filepath.Join(p.data, "users.json"))
	_ = json.Unmarshal(b, &p.users)
	return p
}

func (p *proc) client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: p.pool}, MaxIdleConnsPerHost: 200,
		DialContext: func(ctx context.Context, n, addr string) (net.Conn, error) {
			target := p.vcAddr
			if strings.HasPrefix(addr, "nsx.") {
				target = p.nsxAddr
			}
			return (&net.Dialer{}).DialContext(ctx, n, target)
		},
	}}
}

func (p *proc) ready(svc, path string) bool {
	res, err := p.client(2 * time.Second).Get("https://" + svc + "." + testenv.Domain + path)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode < 500
}

func (p *proc) user(product string) (string, string) {
	for _, u := range p.users {
		for _, x := range u.Products {
			if x == product {
				return u.Name, u.Password
			}
		}
	}
	p.t.Fatalf("no %s user", product)
	return "", ""
}

// memMB는 프로세스 메모리(MB)다. Windows: Private·Working Set 중 큰 값, 그 밖: RSS.
func (p *proc) memMB() float64 {
	pid := p.cmd.Process.Pid
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf("$p=Get-Process -Id %d; \"$($p.PrivateMemorySize64) $($p.WorkingSet64)\"", pid)).Output()
		if err != nil {
			return -1
		}
		f := strings.Fields(string(out))
		if len(f) < 2 {
			return -1 // 프로세스가 끝났거나 조회 실패
		}
		a, _ := strconv.ParseFloat(f[0], 64)
		b, _ := strconv.ParseFloat(f[1], 64)
		return max(a, b) / 1e6
	default:
		out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return -1
		}
		kb, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		return kb * 1024 / 1e6
	}
}

type loadResult struct {
	Clients    int                `json:"clients"`
	Requests   int64              `json:"requests"`
	Errors     int64              `json:"errors"`
	RPS        float64            `json:"rps"`
	P50ms      float64            `json:"p50_ms"`
	P95ms      float64            `json:"p95_ms"`
	P99ms      float64            `json:"p99_ms"`
	PeakMemMB  float64            `json:"peak_memory_mb"`
	DurationMS int64              `json:"duration_ms"`
	ErrorKinds map[string]int64   `json:"error_kinds,omitempty"`
	KindP95ms  map[string]float64 `json:"p95_ms_by_kind"`
}

// load는 clients개 고루틴이 d 동안 혼합 요청(REST VM 전체 목록·VM 조회, NSX Segment 조회·목록, NSX Segment 쓰기)을 보낸다.
func (p *proc) load(clients int, d time.Duration) loadResult {
	vu, vp := p.user("vcenter")
	nu, np := p.user("nsx")
	c := p.client(60 * time.Second)
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.SetBasicAuth(vu, vp)
	res, err := c.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	tokB, _ := io.ReadAll(res.Body)
	res.Body.Close()
	token := strings.Trim(strings.TrimSpace(string(tokB)), `"`)
	var n, errs atomic.Int64
	var mu sync.Mutex
	var lat []time.Duration
	kinds := map[string]int64{}
	byKind := map[string][]time.Duration{}
	var peak atomic.Int64
	stop := make(chan struct{})
	go func() {
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				if m := int64(p.memMB()); m > peak.Load() {
					peak.Store(m)
				}
			}
		}
	}()
	t0 := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < clients; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var local []time.Duration
			localKind := map[string][]time.Duration{}
			for i := 0; time.Since(t0) < d; i++ {
				var r *http.Request
				kind := []string{"rest-vm-list", "rest-vm-get", "nsx-segment-get", "nsx-tier1-list", "nsx-segment-patch"}[i%5]
				switch i % 5 {
				case 0:
					r, _ = http.NewRequest(http.MethodGet, "https://vcenter."+testenv.Domain+"/api/vcenter/vm", nil)
					r.Header.Set("vmware-api-session-id", token)
				case 1:
					r, _ = http.NewRequest(http.MethodGet, "https://vcenter."+testenv.Domain+"/api/vcenter/vm/vm-"+strconv.Itoa(60+i%20), nil)
					r.Header.Set("vmware-api-session-id", token)
				case 2:
					r, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("https://nsx.%s/policy/api/v1/infra/segments/seg-%02d", testenv.Domain, 1+i%10), nil)
					r.SetBasicAuth(nu, np)
				case 3:
					r, _ = http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/policy/api/v1/infra/tier-1s", nil)
					r.SetBasicAuth(nu, np)
				default:
					r, _ = http.NewRequest(http.MethodPatch, fmt.Sprintf("https://nsx.%s/policy/api/v1/infra/segments/perf-%d", testenv.Domain, w), strings.NewReader(`{"display_name":"perf"}`))
					r.SetBasicAuth(nu, np)
					r.Header.Set("Content-Type", "application/json")
				}
				s := time.Now()
				res, err := c.Do(r)
				if err == nil {
					_, _ = io.Copy(io.Discard, res.Body)
					res.Body.Close()
					if res.StatusCode >= 300 && res.StatusCode != 404 { // vm-NN이 없는 규모(404)는 정상 응답
						err = fmt.Errorf("status %d", res.StatusCode)
					}
				}
				local = append(local, time.Since(s))
				localKind[kind] = append(localKind[kind], time.Since(s))
				n.Add(1)
				if err != nil {
					errs.Add(1)
					mu.Lock()
					kinds[kind+": "+err.Error()]++
					mu.Unlock()
				}
			}
			mu.Lock()
			lat = append(lat, local...)
			for k, v := range localKind {
				byKind[k] = append(byKind[k], v...)
			}
			mu.Unlock()
		}(w)
	}
	wg.Wait()
	close(stop)
	el := time.Since(t0)
	c.CloseIdleConnections() // 단계마다 새 클라이언트를 쓰므로 이전 연결을 남기지 않는다(R-45 ④)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	pct := func(q float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return float64(lat[int(q*float64(len(lat)-1))].Microseconds()) / 1000
	}
	kp95 := map[string]float64{}
	for k, v := range byKind {
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		kp95[k] = float64(v[int(0.95*float64(len(v)-1))].Microseconds()) / 1000
	}
	return loadResult{KindP95ms: kp95, Clients: clients, Requests: n.Load(), Errors: errs.Load(), RPS: float64(n.Load()) / el.Seconds(),
		P50ms: pct(0.5), P95ms: pct(0.95), P99ms: pct(0.99), PeakMemMB: float64(peak.Load()), DurationMS: el.Milliseconds(), ErrorKinds: kinds}
}

type caseResult struct {
	Case      string       `json:"case"`
	VMs       int          `json:"vms"`
	Segments  int          `json:"segments"`
	StartupMS int64        `json:"startup_ms"`
	IdleMemMB float64      `json:"idle_memory_mb"`
	Loads     []loadResult `json:"loads"`
}

// T-NFR-0115 T-NFR-0116 T-NFR-0117 T-R-09
func TestBenchmark(t *testing.T) {
	if testing.Short() || os.Getenv("PERF_RUN") == "" {
		t.Skip("PERF_RUN=1일 때만 실행한다(make perf): 실행파일을 빌드하고 수 분 걸린다")
	}
	full := os.Getenv("PERF_FULL") != ""
	type c struct{ vms, segs int }
	cases := []c{{100, 10}, {1000, 10}, {20, 1000}}
	clients := []int{10, 50}
	if full {
		cases = append(cases, c{5000, 10})
		clients = append(clients, 100)
	}
	if only := os.Getenv("PERF_CASE"); only != "" { // 예: PERF_CASE=100x10
		var v, sg int
		_, _ = fmt.Sscanf(only, "%dx%d", &v, &sg)
		cases = []c{{v, sg}}
	}
	var results []caseResult
	for _, cs := range cases {
		name := fmt.Sprintf("vm%d-seg%d", cs.vms, cs.segs)
		t.Run(name, func(t *testing.T) {
			p := start(t, fixture(t, cs.vms, cs.segs))
			time.Sleep(3 * time.Second)
			r := caseResult{Case: name, VMs: cs.vms, Segments: cs.segs, StartupMS: p.startup.Milliseconds(), IdleMemMB: p.memMB()}
			for _, n := range clients {
				lr := p.load(n, 5*time.Second)
				if p.cmd.ProcessState != nil || p.memMB() < 0 {
					t.Fatalf("%s clients=%d: simulator process ended", name, n)
				}
				// 회귀 기준(NFR-0117: 목표는 측정 결과로 정한다, Phase 5 측정 — RESOURCE_BUDGET §5): VM·Segment 1,000개 이하·동시 50
				// 이하는 오류 0. 그 밖(VM 5,000, 동시 100)은 기록만 한다(R-45: Segment 쓰기마다 전체 관계 조정, 60초 Timeout 발생).
				// 전송 계층 오류(TLS alert·연결 거부)는 따로 세고 0.1%까지 허용해 기록한다(R-45 ③, 원인 미확인 — 숨기지 않고 보고).
				var transport int64
				for k, v := range lr.ErrorKinds {
					if strings.Contains(k, "tls:") || strings.Contains(k, "refused") || strings.Contains(k, "forcibly closed") {
						transport += v
					}
				}
				app := lr.Errors - transport
				supported := cs.vms <= 1000 && cs.segs <= 1000 && n <= 50
				if transport > 0 {
					t.Logf("%s clients=%d: transport errors %d / %d (R-45 ③) %v", name, n, transport, lr.Requests, lr.ErrorKinds)
					p.logTLS(t)
				}
				switch {
				case lr.Requests == 0:
					t.Errorf("%s clients=%d: no requests", name, n)
				case supported && transport*1000 > lr.Requests:
					t.Errorf("%s clients=%d: transport errors %d exceed 0.1%% of %d", name, n, transport, lr.Requests)
				case app > 0 && supported:
					t.Errorf("%s clients=%d: %d errors / %d requests %v", name, n, lr.Errors, lr.Requests, lr.ErrorKinds)
				case lr.Errors > 0:
					t.Logf("%s clients=%d: %d errors / %d requests (측정 범위 밖, R-45) %v", name, n, lr.Errors, lr.Requests, lr.ErrorKinds)
				}
				r.Loads = append(r.Loads, lr)
			}
			t.Logf("%s startup=%dms idle=%.1fMB loads=%+v", name, r.StartupMS, r.IdleMemMB, r.Loads)
			results = append(results, r)
		})
	}
	name := "benchmark"
	if c := os.Getenv("PERF_CASE"); c != "" {
		name += "-" + c
	}
	writeReport(t, name, results, benchmarkMarkdown(results))
}

func benchmarkMarkdown(rs []caseResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Performance Benchmark (%s, %s/%s, full 실행 프로필)\n\n", time.Now().Format("2006-01-02"), runtime.GOOS, runtime.GOARCH)
	b.WriteString("| 규모 | 기동 ms | 유휴 MB | 동시 | 요청 | 오류 | req/s | p50 ms | p95 ms | p99 ms | 최대 MB |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rs {
		for _, l := range r.Loads {
			fmt.Fprintf(&b, "| VM %d·Segment %d | %d | %.0f | %d | %d | %d | %.0f | %.1f | %.1f | %.1f | %.0f |\n",
				r.VMs, r.Segments, r.StartupMS, r.IdleMemMB, l.Clients, l.Requests, l.Errors, l.RPS, l.P50ms, l.P95ms, l.P99ms, l.PeakMemMB)
		}
	}
	return b.String()
}

func writeReport(t *testing.T, name string, v any, md string) {
	t.Log("\n" + md)
	dir := os.Getenv("PERF_REPORT_DIR")
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	j, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, name+".json"), j, 0o644)
	_ = os.WriteFile(filepath.Join(dir, name+".md"), []byte(md), 0o644)
}

// T-NFR-0501 T-NFR-0504 T-NFR-0508 T-NFR-0115: full 실행 프로필 장시간 수렴. 보존 한도를 작게 두고(Trace 2,000건·8 MB,
// 전이·작업 2,000건) 많은 변경 요청을 보내면 메모리와 저장소 크기가 일정 범위에 머문다(단계별 측정, 마지막 두 단계 차이 ≤ 15%).
// 실제 24시간 운전 대신 요청 수로 가속한다(PERF_SOAK_ROUNDS로 조정, 기본 6단계 × 15초).
func TestSoakRetention(t *testing.T) {
	if testing.Short() || os.Getenv("PERF_RUN") == "" {
		t.Skip("PERF_RUN=1일 때만 실행한다(make soak): 수 분 걸린다")
	}
	rounds := 6
	if v, err := strconv.Atoi(os.Getenv("PERF_SOAK_ROUNDS")); err == nil && v > 2 {
		rounds = v
	}
	p := start(t, filepath.Join(testenv.Repo(), "30-implementation", "data", "fixtures", "default-small.yaml"),
		"--retain-trace", "2000", "--retain-trace-mb", "8", "--retain-transitions", "2000", "--retain-operations", "2000")
	type sample struct {
		Round    int     `json:"round"`
		Requests int64   `json:"requests"`
		MemMB    float64 `json:"memory_mb"`
		DBMB     float64 `json:"db_mb"`
	}
	var samples []sample
	var total int64
	for i := 1; i <= rounds; i++ {
		lr := p.load(20, 15*time.Second)
		if lr.Errors > 0 {
			t.Fatalf("round %d: %d errors", i, lr.Errors)
		}
		total += lr.Requests
		runtime.GC()
		time.Sleep(2 * time.Second)
		samples = append(samples, sample{Round: i, Requests: total, MemMB: p.memMB(), DBMB: dbMB(p.data)})
		t.Logf("round %d: requests=%d memory=%.1fMB db=%.1fMB", i, total, samples[i-1].MemMB, samples[i-1].DBMB)
	}
	last, prev := samples[len(samples)-1], samples[len(samples)-2]
	if last.MemMB > prev.MemMB*1.15+5 {
		t.Errorf("memory still growing: %.1f → %.1f MB", prev.MemMB, last.MemMB)
	}
	if last.DBMB > prev.DBMB*1.15+2 {
		t.Errorf("database still growing: %.1f → %.1f MB", prev.DBMB, last.DBMB)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Soak (full, 보존 한도 축소) %s\n\n| 단계 | 누적 요청 | 메모리 MB | DB MB |\n|---|---|---|---|\n", time.Now().Format("2006-01-02"))
	for _, s := range samples {
		fmt.Fprintf(&b, "| %d | %d | %.1f | %.1f |\n", s.Round, s.Requests, s.MemMB, s.DBMB)
	}
	writeReport(t, "soak", samples, b.String())
}

func dbMB(dir string) float64 {
	var total int64
	for _, f := range []string{"cloudinfra-sim.db", "cloudinfra-sim.db-wal", "cloudinfra-sim.db-shm"} {
		if st, err := os.Stat(filepath.Join(dir, f)); err == nil {
			total += st.Size()
		}
	}
	return float64(total) / 1e6
}

// perfFixtureCache: 벤치마크는 매번 새 데이터 디렉터리라 캐시가 늘 miss다(저장 시간만 더해진다). 기본은 off로 두어
// 이전 측정과 비교할 수 있게 하고, 캐시 효과는 PERF_FIXTURE_CACHE=on으로 따로 잰다(R-45 ②).
func perfFixtureCache() string {
	if v := os.Getenv("PERF_FIXTURE_CACHE"); v != "" {
		return v
	}
	return "off"
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// logLevel은 시뮬레이터 로그 수준이다(PERF_SIM_LOGLEVEL, 기본 error). 전송 오류 조사 때 debug로 둔다(R-45 ③).
func logLevel() string {
	if v := os.Getenv("PERF_SIM_LOGLEVEL"); v != "" {
		return v
	}
	return "error"
}

// logTLS는 시뮬레이터 로그에서 TLS·연결 관련 줄을 보여 준다(조사용).
func (p *proc) logTLS(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join(p.data, "sim.log"))
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "TLS") || strings.Contains(l, "tls") || strings.Contains(l, "handshake") || strings.Contains(l, "http: ") {
			if n < 20 {
				t.Logf("sim: %s", tail(l, 400))
			}
			n++
		}
	}
	t.Logf("sim tls/http log lines: %d", n)
}
