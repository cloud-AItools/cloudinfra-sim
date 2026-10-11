// 장시간 운전 자원 회수(R-45 ④). soak에서 메모리가 단계마다 늘던 두 원인을 고정한다:
// ① vcsim 세션 객체(뷰·속성 필터·수집기)를 파괴해도 전역 락 항목이 남던 누수 ② 떠난 클라이언트의 유휴 keep-alive 연결.
package contract

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/gateway"
	"cloudinfra-sim/test/internal/testenv"
)

// T-NFR-0501 T-NFR-0115: 뷰·수집기 생성/파괴와 관계 조정(내부 뷰 사용)을 반복해도 vcsim 락 항목이 늘지 않는다.
func TestSessionObjectLocksReleased(t *testing.T) {
	e := testenv.Start(t, nil)
	c := e.Login(testenv.VCenterVersion())
	ctx := context.Background()
	round := func(i int) {
		v, err := view.NewManager(c.Client).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := v.Find(ctx, []string{"VirtualMachine"}, nil); err != nil {
			t.Fatal(err)
		}
		if err := v.Destroy(ctx); err != nil {
			t.Fatal(err)
		}
		pc, err := property.DefaultCollector(c.Client).Create(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := pc.Destroy(ctx); err != nil {
			t.Fatal(err)
		}
		// NSX 쓰기 → 관계 조정(내부 클라이언트가 ContainerView를 만들고 지운다). 이름은 고정: 이름 변경은 포트그룹
		// Rename Task를 만들고, 완료 Task는 TaskRetention(200)까지 보관되므로(그 락 포함) 여기서는 제외한다.
		mustNSX(t, e, "PATCH", p4+"/infra/segments/lr-seg", fmt.Sprintf(`{"display_name":"lr-seg","description":"%d"}`, i), 200)
	}
	settled := func() int {
		var last int
		for range 40 { // 지연 회수(최대 수 초)가 끝날 때까지 기다린다
			time.Sleep(100 * time.Millisecond)
			_, n := e.Sim.VC.RegistryStats()
			if n == last {
				return n
			}
			last = n
		}
		return last
	}
	for i := range 5 { // 처음 만들어지는 객체(Segment 포트그룹 등)의 락은 기준선에 넣는다
		round(i)
	}
	base := settled()
	for i := range 60 {
		round(100 + i)
	}
	if got := settled(); got > base+5 {
		t.Fatalf("vcsim 락 항목이 늘었다: %d → %d (60회 반복)", base, got)
	}
	// Admin 상태에 진단 값이 나온다
	st, _ := e.Sim.Status().(map[string]any)
	rt, _ := st["retention_usage"].(map[string]any)["runtime"].(map[string]any)
	for _, k := range []string{"goroutines", "heap_alloc_mb", "vcsim_objects", "vcsim_locks"} {
		if _, ok := rt[k]; !ok {
			t.Errorf("retention_usage.runtime.%s 없음: %v", k, rt)
		}
	}
}

// T-NFR-0501: 세션 객체를 파괴하지 않고 세션이 끝나도(Logout·TerminateSession) vcsim 락 항목이 남지 않는다.
// TerminateSession·유휴 만료는 vcsim이 세션 뷰를 전역 변경 처리기에서 빼지 않아 계속 통지를 받았다 — 이것도 함께 정리한다.
func TestSessionEndLocksReleased(t *testing.T) {
	e := testenv.Start(t, nil)
	admin := e.Login(testenv.VCenterVersion())
	ctx := context.Background()
	settled := func() int {
		time.Sleep(time.Second) // 세션 종료 정리는 종료 요청 0.5초 뒤에 돈다(viewlocks.go sessionEndDelay)
		var last int
		for range 40 {
			time.Sleep(150 * time.Millisecond)
			_, n := e.Sim.VC.RegistryStats()
			if n == last {
				return n
			}
			last = n
		}
		return last
	}
	use := func() *govmomi.Client {
		c := e.Login(testenv.VCenterVersion())
		v, err := view.NewManager(c.Client).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := v.Find(ctx, []string{"VirtualMachine"}, nil); err != nil { // 세션 객체 호출 → 락 항목 생성
			t.Fatal(err)
		}
		pc, err := property.DefaultCollector(c.Client).Create(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var content []types.ObjectContent
		if err := pc.RetrieveOne(ctx, c.ServiceContent.RootFolder, []string{"name"}, &content); err != nil {
			t.Fatal(err)
		}
		return c // 뷰·수집기를 파괴하지 않는다
	}
	use().Logout(ctx)
	base := settled()
	for i := range 20 {
		c := use()
		if i%2 == 0 {
			if err := c.Logout(ctx); err != nil {
				t.Fatal(err)
			}
			continue
		}
		cur, err := c.SessionManager.UserSession(ctx)
		if err != nil || cur == nil {
			t.Fatalf("current session: %v", err)
		}
		if err := admin.SessionManager.TerminateSession(ctx, []string{cur.Key}); err != nil {
			t.Fatal(err)
		}
	}
	if got := settled(); got > base+5 {
		t.Fatalf("세션 종료 뒤 vcsim 락 항목이 남았다: %d → %d (세션 20개)", base, got)
	}
}

// T-NFR-0501: 요청 뒤 유휴 상태로 남은 연결은 Gateway IdleTimeout이 지나면 서버가 닫는다.
func TestGatewayIdleTimeout(t *testing.T) {
	old := gateway.IdleTimeout
	gateway.IdleTimeout = time.Second
	t.Cleanup(func() { gateway.IdleTimeout = old })
	e := testenv.Start(t, nil)
	name := "nsx." + testenv.Domain
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", e.Sim.Addrs["nsx"].String(),
		&tls.Config{RootCAs: e.Sim.Certs.CAPool, ServerName: name})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "GET /api/v1/node/version HTTP/1.1\r\nHost: %s\r\n\r\n", name); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := conn.Read(buf); err != nil || n == 0 {
		t.Fatalf("응답 없음: %v", err)
	}
	// 응답 뒤 남은 바이트를 비우고, 유휴 제한 뒤 서버가 연결을 닫는지(EOF) 본다.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	for {
		_, err := conn.Read(buf)
		if err == nil {
			continue
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatalf("유휴 연결이 %s 동안 닫히지 않았다", time.Since(start).Round(time.Millisecond))
		}
		if !errors.Is(err, io.EOF) {
			t.Logf("연결 종료: %v", err)
		}
		break
	}
	if el := time.Since(start); el < 500*time.Millisecond {
		t.Fatalf("연결이 너무 일찍 닫혔다(%s): keep-alive가 동작하지 않는다", el)
	}
}
