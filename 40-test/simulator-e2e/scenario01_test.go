// Scenario 01 — API 연결 및 환경 인식 (PRD-2 TR-E2E-0101 ~ TR-E2E-0104, Phase 1 종료 게이트).
// 테스트 ID는 TEST_STRATEGY.md 추적표의 T-<요구사항 ID>와 연결한다.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

func nsxGet(t *testing.T, e *testenv.Env, path, user, pass string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+path, nil)
	req.SetBasicAuth(user, pass)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	b, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out
}

// TR-E2E-0101 절차 ①~⑥, 성공 기준 TR-E2E-0102 ~ TR-E2E-0104.
// 추적: T-TR-E2E-0101 T-TR-E2E-0102 T-TR-E2E-0103 T-TR-E2E-0104 T-TR-COV-0307
// 추적: T-FR-VC-0201 T-FR-VC-0303~0306 T-FR-NSX-0201 T-NFR-0302 T-NFR-ARC-0107 T-NFR-ARC-0301 T-NFR-ARC-0304 T-NFR-0101 T-NFR-0102
func TestScenario01_ConnectAndDiscover(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()

	// ① vCenter 로그인 (TR-E2E-0102: FR-API-0201, FR-VC-0201)
	c := e.Login("")
	about := c.ServiceContent.About

	// ② 제품 버전·인벤토리 (TR-E2E-0104: FR-VC-0307, FR-API-0306 / TR-E2E-0103: FR-VC-0301 ~ 0306)
	want := e.Sim.Profile.VCenter.About // 활성 프로파일 값(기준 7.0.3 / 추가 버전 WP 4.7)
	if about.Version != want["version"] || about.Build != want["build"] || about.ApiVersion != want["apiVersion"] || about.ApiType != "VirtualCenter" {
		t.Fatalf("T-FR-API-0306: about %+v", about)
	}
	f := find.NewFinder(c.Client, true)
	dcs, err := f.DatacenterList(ctx, "*")
	if err != nil || len(dcs) != 1 {
		t.Fatalf("T-FR-VC-0301: datacenters %d %v", len(dcs), err)
	}
	f.SetDatacenter(dcs[0])
	clusters, _ := f.ClusterComputeResourceList(ctx, "*")
	hosts, _ := f.HostSystemList(ctx, "*/*")
	dss, _ := f.DatastoreList(ctx, "*")
	nets, _ := f.NetworkList(ctx, "*")
	vms, _ := f.VirtualMachineList(ctx, "*")
	t.Logf("inventory: dc=%d cluster=%d host=%d datastore=%d network=%d vm=%d", len(dcs), len(clusters), len(hosts), len(dss), len(nets), len(vms))
	if len(clusters) != 1 || len(hosts) != 3 || len(dss) != 2 || len(vms) != 20 {
		t.Fatal("T-FR-VC-0302~0305: inventory does not match fixture default-small")
	}

	// ③ SOAP 세션으로 후속 조회 (FR-API-0203)
	var vm mo.VirtualMachine
	if err := vms[0].Properties(ctx, vms[0].Reference(), []string{"name", "runtime.powerState", "config.uuid"}, &vm); err != nil {
		t.Fatalf("T-FR-API-0203: %v", err)
	}
	active, err := c.SessionManager.SessionIsActive(ctx)
	if err != nil || !active {
		t.Fatalf("T-FR-API-0203: session active=%v err=%v", active, err)
	}

	// ④ NSX 인증 (TR-E2E-0102: FR-API-0206 ~ 0208) — Basic 인증
	nu := e.User("nsx")
	code, ver := nsxGet(t, e, "/api/v1/node/version", nu.Name, nu.Password)
	if code != 200 {
		t.Fatalf("T-FR-API-0206: %d", code)
	}

	// ⑤ NSX 버전·Node/Cluster·Compute Manager·Transport Zone (TR-E2E-0103: FR-NSX-0202 ~ 0205, FR-NSX-0601)
	if ver["product_version"] != "3.2.4.3.0.24942842" {
		t.Fatalf("T-FR-NSX-0202: %v", ver)
	}
	code, cs := nsxGet(t, e, "/api/v1/cluster/status", nu.Name, nu.Password)
	if code != 200 || cs["mgmt_cluster_status"] == nil {
		t.Fatalf("T-FR-NSX-0203: %d %v", code, cs)
	}
	code, cms := nsxGet(t, e, "/api/v1/fabric/compute-managers", nu.Name, nu.Password)
	results, _ := cms["results"].([]any)
	if code != 200 || len(results) != 1 {
		t.Fatalf("T-FR-NSX-0204: %d %v", code, cms)
	}
	cm := results[0].(map[string]any)
	thumb := cm["credential"].(map[string]any)["thumbprint"]
	if cm["server"] != "vcenter."+testenv.Domain || thumb != e.Sim.Certs.ThumbprintSHA256() {
		t.Fatalf("T-FR-NSX-0601: compute manager %v (gateway thumbprint %s)", cm, e.Sim.Certs.ThumbprintSHA256())
	}
	code, tzs := nsxGet(t, e, "/api/v1/transport-zones", nu.Name, nu.Password)
	if code != 200 || tzs["result_count"] != float64(2) {
		t.Fatalf("T-FR-NSX-0205: %d %v", code, tzs)
	}
	code, ptz := nsxGet(t, e, "/policy/api/v1/infra/sites/default/enforcement-points/default/transport-zones", nu.Name, nu.Password)
	if code != 200 || ptz["result_count"] != float64(2) {
		t.Fatalf("T-FR-NSX-0210: %d %v", code, ptz)
	}
	code, infra := nsxGet(t, e, "/policy/api/v1/infra", nu.Name, nu.Password)
	if code != 200 || infra["resource_type"] != "Infra" {
		t.Fatalf("T-FR-NSX-0209: %d %v", code, infra)
	}

	// ⑥ 세션 종료 및 인증 상태 확인
	if err := c.Logout(ctx); err != nil {
		t.Fatalf("T-FR-API-0201 logout: %v", err)
	}
	if _, err := f.DatacenterList(ctx, "*"); err == nil {
		t.Fatal("T-FR-API-0203: call after logout must fail")
	}
}

// MVP 기본 Negative (TR-COV-0303, FR-FLT-0101): 잘못된 자격 증명은 계약 오류로 거부된다.
// 추적: T-FR-API-0210 T-FR-API-0205
func TestScenario01_AuthenticationFailures(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.SOAP("")
	err := c.Login(ctx, url.UserPassword(e.User("vcenter").Name, "wrong-password"))
	if err == nil || !soap.IsSoapFault(err) {
		t.Fatalf("T-FR-FLT-0101 SOAP: want SOAP fault, got %v", err)
	}
	if _, ok := soap.ToSoapFault(err).VimFault().(types.InvalidLogin); !ok {
		t.Fatalf("T-FR-FLT-0101 SOAP: want InvalidLogin, got %T", soap.ToSoapFault(err).VimFault())
	}
	// NSX 사용자는 vCenter 제품 권한이 없다(FR-API-0205 사용자별 분리)
	nu := e.User("nsx")
	if err := e.SOAP("").Login(ctx, url.UserPassword(nu.Name, nu.Password)); err == nil {
		t.Fatal("T-FR-API-0205: NSX-only user must not log in to vCenter")
	}
	code, body := nsxGet(t, e, "/api/v1/node/version", nu.Name, "wrong-password")
	if code != http.StatusForbidden || body["error_message"] == nil || body["error_code"] == nil {
		t.Fatalf("T-FR-FLT-0101 NSX: %d %v", code, body)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/api/v1/node/version", nil)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("T-FR-API-0208 unauthenticated NSX: %d", res.StatusCode)
	}
}

// 지원 범위 밖 요청은 성공으로 위장하지 않고 집계된다(FR-API-0401 ~ 0404, NFR-ARC-0304).
// 추적: T-FR-VC-0608 T-FR-NSX-0105 T-FR-API-0402 T-FR-API-0404
func TestScenario01_UnsupportedAndRouting(t *testing.T) {
	e := testenv.Start(t, nil)
	nu := e.User("nsx")
	code, _ := nsxGet(t, e, "/policy/api/v1/orgs/default/projects", nu.Name, nu.Password)
	if code != http.StatusNotFound {
		t.Fatalf("T-FR-API-0401: NSX 4.x-only path → %d", code)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://unknown."+testenv.Domain+"/sdk", nil)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("T-NFR-ARC-0304: unknown virtual host → %d", res.StatusCode)
	}
	res, err = e.AdminDo(http.MethodGet, "/v1/unsupported", nil)
	if err != nil {
		t.Fatal(err)
	}
	var u struct {
		APIs []struct {
			Key   string `json:"key"`
			Count int    `json:"count"`
		} `json:"apis"`
	}
	_ = json.NewDecoder(res.Body).Decode(&u)
	res.Body.Close()
	found := false
	for _, a := range u.APIs {
		if strings.Contains(a.Key, "/policy/api/v1/orgs/default/projects") {
			found = true
		}
	}
	if !found {
		t.Fatalf("T-FR-API-0403: unsupported call not recorded: %+v", u.APIs)
	}
}
