// NSX 응답 스키마 대조 (PRD-2 TR-COV-0301, FR-API-0101·0102). 근거: SRC-NSX-321 응답 스키마·ApiError.
// MVP 실행 범위(NSX-001A ~ 011)의 정상·오류 응답을 스펙 응답 스키마에 대조한다: 타입, enum, required,
// 스펙에 없는 속성(시뮬레이터가 임의로 덧붙이거나 표준화한 필드) 탐지.
package contract

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"

	"cloudinfra-sim/test/internal/nsxcheck"
	"cloudinfra-sim/test/internal/testenv"
)

func nsxRaw(t *testing.T, e *testenv.Env, method, path, body string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, "https://nsx."+testenv.Domain+path, strings.NewReader(body))
	u := e.User("nsx")
	req.SetBasicAuth(u.Name, u.Password)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

// T-TR-COV-0301 T-FR-API-0101 T-FR-API-0102 T-FR-NSX-0416
func TestNSXResponseSchemaContract(t *testing.T) {
	e := testenv.Start(t, nil)
	dir := e.Sim.Profile.NSXDir
	specs := map[string]*nsxcheck.Spec{}
	for k, f := range map[string]string{"/api/v1": "nsx_api.json", "/policy/api/v1": "nsx_policy_api.json"} {
		s, err := nsxcheck.Load(filepath.Join(dir, f))
		if err != nil {
			t.Skipf("NSX 스펙 없음(Get-NsxSpec.ps1): %v", err)
		}
		specs[k] = s
	}
	const tz = "/infra/sites/default/enforcement-points/default/transport-zones/tz-overlay"
	setup := []struct{ method, path, body string }{
		{http.MethodPatch, "/policy/api/v1/infra/tier-1s/t1-s", `{}`},
		{http.MethodPatch, "/policy/api/v1/infra/segments/s1", `{"subnets":[{"gateway_address":"10.1.0.1/24"}],"connectivity_path":"/infra/tier-1s/t1-s","transport_zone_path":"` + tz + `","tags":[{"scope":"env","tag":"t"}]}`},
	}
	for _, s := range setup {
		if code, b := nsxRaw(t, e, s.method, s.path, s.body); code != 200 {
			t.Fatalf("setup %s %s: %d %s", s.method, s.path, code, b)
		}
	}
	// Phase 3 파생 객체: s1에 VM vNIC을 연결해 Segment Port·VIF가 생기게 한다
	attachVMToSegment(t, e, "/infra/segments/s1")
	_, cmList := nsxDo(t, e, http.MethodGet, "/api/v1/fabric/compute-managers", "")
	cmID := cmList["results"].([]any)[0].(map[string]any)["id"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, p := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments/s1/ports", "")
		if rs, _ := p["results"].([]any); len(rs) == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	cases := []struct {
		method, base, path, template, body string
		status                             int
	}{
		{"GET", "/api/v1", "/node/version", "/node/version", "", 200},
		{"GET", "/api/v1", "/cluster/status", "/cluster/status", "", 200},
		{"GET", "/api/v1", "/transport-zones", "/transport-zones", "", 200},
		{"GET", "/api/v1", "/fabric/compute-managers", "/fabric/compute-managers", "", 200},
		{"GET", "/policy/api/v1", "/infra", "/infra", "", 200},
		{"GET", "/policy/api/v1", "/infra/sites/default/enforcement-points/default/transport-zones", "/infra/sites/{site-id}/enforcement-points/{enforcementpoint-id}/transport-zones", "", 200},
		{"GET", "/policy/api/v1", "/infra/segments", "/infra/segments", "", 200},
		{"GET", "/policy/api/v1", "/infra/segments?page_size=2", "/infra/segments", "", 200},
		{"GET", "/policy/api/v1", "/infra/segments/s1", "/infra/segments/{segment-id}", "", 200},
		{"GET", "/policy/api/v1", "/infra/tier-1s", "/infra/tier-1s", "", 200},
		{"GET", "/policy/api/v1", "/infra/realized-state/status?intent_path=/infra/segments/s1", "/infra/realized-state/status", "", 200},
		{"GET", "/policy/api/v1", "/system-config/nsx-partial-patch-config", "/system-config/nsx-partial-patch-config", "", 200},
		{"PATCH", "/policy/api/v1", "/infra/segments/s2", "/infra/segments/{segment-id}", `{"display_name":"s2"}`, 200},
		// 오류 응답(ApiError)
		{"GET", "/policy/api/v1", "/infra/segments/none", "/infra/segments/{segment-id}", "", 404},
		{"PATCH", "/policy/api/v1", "/infra/segments/s1", "/infra/segments/{segment-id}", `{"admin_state":"SIDEWAYS"}`, 400},
		{"PUT", "/policy/api/v1", "/infra/segments/s1", "/infra/segments/{segment-id}", `{"display_name":"x","_revision":99}`, 412},
		// Phase 3 교차 시스템 파생·조회
		{"GET", "/api/v1", "/fabric/virtual-machines", "/fabric/virtual-machines", "", 200},
		{"GET", "/api/v1", "/fabric/vifs", "/fabric/vifs", "", 200},
		{"GET", "/api/v1", "/fabric/discovered-nodes", "/fabric/discovered-nodes", "", 200},
		{"GET", "/api/v1", "/fabric/compute-managers/" + cmID + "/status", "/fabric/compute-managers/{compute-manager-id}/status", "", 200},
		{"GET", "/policy/api/v1", "/infra/segments/s1/ports", "/infra/segments/{segment-id}/ports", "", 200},
		{"GET", "/policy/api/v1", "/infra/realized-state/realized-entities?intent_path=/infra/segments/s1", "/infra/realized-state/realized-entities", "", 200},
		{"GET", "/policy/api/v1", "/infra/realized-state/enforcement-points/default/virtual-machines", "/infra/realized-state/enforcement-points/{enforcement-point-name}/virtual-machines", "", 200},
		{"PATCH", "/policy/api/v1", "/infra/domains/default/groups/g1", "/infra/domains/{domain-id}/groups/{group-id}", `{"expression":[{"resource_type":"Condition","member_type":"VirtualMachine","key":"Name","operator":"CONTAINS","value":"VM"}]}`, 200},
		{"GET", "/policy/api/v1", "/infra/domains/default/groups/g1/members/virtual-machines", "/infra/domains/{domain-id}/groups/{group-id}/members/virtual-machines", "", 200},
		{"GET", "/api/v1", "/alarms", "/alarms", "", 200},
	}
	for _, c := range cases {
		code, b := nsxRaw(t, e, c.method, c.base+c.path, c.body)
		if code != c.status {
			t.Errorf("%s %s: status %d, want %d (%s)", c.method, c.path, code, c.status, b)
			continue
		}
		if errs := specs[c.base].Check(c.method, c.template, code, b); len(errs) > 0 {
			if len(errs) > 8 {
				errs = append(errs[:8], "...")
			}
			t.Errorf("%s %s%s (%d): %s", c.method, c.base, c.path, code, strings.Join(errs, "; "))
		}
	}
}

// attachVMToSegment는 Fixture 첫 VM에 segment(NSX DVPG)로 연결한 vNIC을 더한다.
func attachVMToSegment(t *testing.T, e *testenv.Env, segment string) {
	t.Helper()
	ctx := context.Background()
	c := e.Login("")
	m := view.NewManager(c.Client)
	v, err := m.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"DistributedVirtualPortgroup", "VirtualMachine"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Destroy(ctx)
	var pgs []mo.DistributedVirtualPortgroup
	if err := v.Retrieve(ctx, []string{"DistributedVirtualPortgroup"}, []string{"config"}, &pgs); err != nil {
		t.Fatal(err)
	}
	vms, err := v.Find(ctx, []string{"VirtualMachine"}, nil)
	if err != nil || len(vms) == 0 {
		t.Fatalf("vms: %v", err)
	}
	for _, pg := range pgs {
		if pg.Config.SegmentId != segment {
			continue
		}
		b, err := object.NewDistributedVirtualPortgroup(c.Client, pg.Self).EthernetCardBackingInfo(ctx)
		if err != nil {
			t.Fatal(err)
		}
		nic, _ := object.EthernetCardTypes().CreateEthernetCard("vmxnet3", b)
		if err := object.NewVirtualMachine(c.Client, vms[0]).AddDevice(ctx, nic); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("no NSX DVPG for %s", segment)
}
