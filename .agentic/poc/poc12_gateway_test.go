package poc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
)

// nsxStub는 PoC-12의 NSX 측 자리표시자다. 실제 NSX 엔진은 PoC-17(스펙 확보 후)에서 검증한다.
func nsxStub() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/node/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"product_version": "3.2.4.3.0.24942842",
			"node_version":    "3.2.4.3.0.24942842",
		})
	})
	return mux
}

// PoC-12: vcsim 임베드 + 단일 바이너리 내장 Gateway(SNI로 vCenter/NSX 분기).
// PoC-19 일부: 루프백 별칭 주소(127.0.0.2)에 관리자 권한 없이 바인딩되는지 함께 확인한다.
func TestPoC12_SNIGateway(t *testing.T) {
	_, vc := newVCenterHandler(t, defaultSmall())
	g := startGateway(t, "127.0.0.2", map[string]http.Handler{
		"vcenter": vc,
		"nsx":     nsxStub(),
	})
	defer g.close()
	t.Logf("gateway listening on %s", g.addr)

	// vCenter: govmomi SOAP 로그인과 인벤토리 조회가 SNI 'vcenter'로 전달된다.
	c := login(t, g)
	ctx := context.Background()
	about := c.ServiceContent.About
	t.Logf("vCenter about: %s %s (apiVersion %s)", about.Name, about.Version, about.ApiVersion)

	m := view.NewManager(c.Client)
	v, err := m.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var vms []mo.VirtualMachine
	if err := v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"name"}, &vms); err != nil {
		t.Fatal(err)
	}
	t.Logf("VM count via gateway: %d", len(vms))
	if len(vms) == 0 {
		t.Fatal("no VMs")
	}

	// NSX: 같은 리스너에서 SNI 'nsx'로 분기된다.
	hc := &http.Client{Transport: g.transport()}
	res, err := hc.Get("https://nsx." + simDomain + "/api/v1/node/version")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("nsx status %d", res.StatusCode)
	}
	t.Logf("NSX via gateway: %s", body)

	// 등록되지 않은 가상 호스트는 다른 엔진으로 새지 않는다.
	res, err = hc.Get("https://unknown." + simDomain + "/sdk")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("unknown host status %d, want 421", res.StatusCode)
	}
}
