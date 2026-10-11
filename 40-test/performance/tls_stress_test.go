package performance

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloudinfra-sim/test/internal/testenv"
)

// TLS 핸드셰이크 스트레스(R-45 ③ 조사). 시뮬레이터(별도 프로세스, full)에 NSX 쓰기 부하를 걸고, 동시에 매 요청 새 연결로
// NSX GET을 보내 핸드셰이크 실패를 센다. 클라이언트 쪽 변수(TLS_MAXVER=12, TLS_CURVE=x25519|p256|mlkem)를 바꿔 비교한다.
// 실행: PERF_RUN=1 TLS_STRESS=1 go test ./performance -run TestTLSHandshakeStress -v
func TestTLSHandshakeStress(t *testing.T) {
	if os.Getenv("PERF_RUN") == "" || os.Getenv("TLS_STRESS") == "" {
		t.Skip("PERF_RUN=1 TLS_STRESS=1일 때만")
	}
	segs, _ := strconv.Atoi(os.Getenv("TLS_SEGMENTS"))
	if segs == 0 {
		segs = 1000
	}
	dur := 60 * time.Second
	if v, err := time.ParseDuration(os.Getenv("TLS_DURATION")); err == nil {
		dur = v
	}
	p := start(t, fixture(t, 20, segs))
	nu, np := p.user("nsx")
	cfg := &tls.Config{RootCAs: p.pool}
	switch os.Getenv("TLS_MAXVER") {
	case "12":
		cfg.MaxVersion = tls.VersionTLS12
	}
	switch os.Getenv("TLS_CURVE") {
	case "x25519":
		cfg.CurvePreferences = []tls.CurveID{tls.X25519}
	case "p256":
		cfg.CurvePreferences = []tls.CurveID{tls.CurveP256}
	case "mlkem":
		cfg.CurvePreferences = []tls.CurveID{tls.X25519MLKEM768}
	}
	dial := func(ctx context.Context, n, addr string) (net.Conn, error) {
		target := p.vcAddr
		if strings.HasPrefix(addr, "nsx.") {
			target = p.nsxAddr
		}
		return (&net.Dialer{}).DialContext(ctx, n, target)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	// 배경 부하: Segment 쓰기(관계 조정·교차 락)와 조회, 연결 재사용
	load := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: p.pool}, DialContext: dial, MaxIdleConnsPerHost: 100}}
	loadWorkers := 30
	if os.Getenv("TLS_NOLOAD") != "" {
		loadWorkers = 0
	}
	for w := 0; w < loadWorkers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				var r *http.Request
				if i%2 == 0 {
					r, _ = http.NewRequest(http.MethodPatch, fmt.Sprintf("https://nsx.%s/policy/api/v1/infra/segments/stress-%d", testenv.Domain, w), strings.NewReader(`{"display_name":"s"}`))
					r.Header.Set("Content-Type", "application/json")
				} else {
					r, _ = http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/policy/api/v1/infra/segments?page_size=50", nil)
				}
				r.SetBasicAuth(nu, np)
				if res, err := load.Do(r); err == nil {
					_, _ = io.Copy(io.Discard, res.Body)
					res.Body.Close()
				}
			}
		}(w)
	}
	// 핸드셰이크 폭주: 매 요청 새 연결
	var n atomic.Int64
	var mu sync.Mutex
	kinds := map[string]int{}
	t0 := time.Now()
	for w := 0; w < 50; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg.Clone(), DialContext: dial, DisableKeepAlives: true}}
			for time.Since(t0) < dur {
				r, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/policy/api/v1/infra/segments/seg-03", nil)
				r.SetBasicAuth(nu, np)
				res, err := c.Do(r)
				n.Add(1)
				k := "ok"
				if err != nil {
					k = err.Error()
					if i := strings.LastIndex(k, "\": "); i > 0 {
						k = k[i+3:]
					}
				} else {
					_, _ = io.Copy(io.Discard, res.Body)
					res.Body.Close()
				}
				mu.Lock()
				kinds[k]++
				mu.Unlock()
			}
		}()
	}
	time.Sleep(dur)
	close(stop)
	wg.Wait()
	var keys []string
	for k := range kinds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%7d %s", kinds[k], k)
	}
	t.Logf("handshakes %d (TLS_MAXVER=%q TLS_CURVE=%q TLS_NOLOAD=%q)", n.Load(), os.Getenv("TLS_MAXVER"), os.Getenv("TLS_CURVE"), os.Getenv("TLS_NOLOAD"))
	p.logTLS(t)
}
