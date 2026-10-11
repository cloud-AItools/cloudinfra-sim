// Phase 4b vCenter 보조 서비스(WP 4.11, FR-VC-0208 Lookup Service, FR-VC-0210 PBM, NFR-ARC-0104 vcsim 보조 서비스 재사용).
// 선정 SDK(govmomi lookup·pbm 클라이언트)로 Gateway 경유 호출한다.
package contract

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/lookup"
	lookuptypes "github.com/vmware/govmomi/lookup/types"
	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-VC-0208 T-NFR-ARC-0104
func TestLookupService(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	lc, err := lookup.NewClient(ctx, c.Client)
	if err != nil {
		t.Fatal(err)
	}
	if lc.URL().Host != "vcenter."+testenv.Domain {
		t.Fatalf("lookup client URL %s", lc.URL())
	}
	regs, err := lc.List(ctx, &lookuptypes.LookupServiceRegistrationFilter{
		ServiceType: &lookuptypes.LookupServiceRegistrationServiceType{Product: "com.vmware.cis", Type: "vcenterserver"},
	})
	if err != nil || len(regs) != 1 {
		t.Fatalf("vcenterserver registrations: %d %v", len(regs), err)
	}
	ep := regs[0].ServiceEndpoints[0]
	u, _ := url.Parse(ep.Url)
	if ep.EndpointType.Type != "com.vmware.vim" || u.Host != "vcenter."+testenv.Domain || u.Path != "/sdk" || u.Scheme != "https" {
		t.Fatalf("vim endpoint: %+v", ep)
	}
	// 알리는 신뢰 인증서는 Gateway 서버 인증서다(클라이언트가 이 인증서로 TLS를 검증할 수 있어야 함)
	der, err := base64.StdEncoding.DecodeString(ep.SslTrust[0])
	if err != nil || !bytes.Equal(der, e.Sim.Certs.Server.Certificate[0]) {
		t.Fatalf("sslTrust is not the gateway certificate: %v", err)
	}
	if regs[0].ServiceId != c.ServiceContent.About.InstanceUuid {
		t.Fatalf("serviceId %s != instanceUuid %s", regs[0].ServiceId, c.ServiceContent.About.InstanceUuid)
	}
	// 알린 끝점으로 실제 로그인할 수 있다
	if _, err := lc.SiteID(ctx); err != nil {
		t.Fatal(err)
	}
}

// T-FR-VC-0210 T-NFR-ARC-0104
func TestPBMStoragePolicy(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	pc, err := pbm.NewClient(ctx, c.Client)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := pc.QueryProfile(ctx, pbmtypes.PbmProfileResourceType{ResourceType: string(pbmtypes.PbmProfileResourceTypeEnumSTORAGE)}, "")
	if err != nil || len(ids) == 0 {
		t.Fatalf("query profiles: %d %v", len(ids), err)
	}
	profiles, err := pc.RetrieveContent(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	var policy string
	for _, p := range profiles {
		if p.GetPbmProfile().Name == "vSAN Default Storage Policy" {
			policy = p.GetPbmProfile().ProfileId.UniqueId
		}
	}
	if policy == "" {
		t.Fatal("default storage policy not found")
	}
	// 정책을 지정해 VM 생성 → 연결 조회, 재구성으로 정책 제거
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	dss, _ := f.DatastoreList(ctx, "*")
	dsName, _ := dss[0].ObjectName(ctx)
	spec := types.VirtualMachineConfigSpec{Name: "pbm-vm", GuestId: "otherGuest64", NumCPUs: 1, MemoryMB: 256,
		Files:     &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"},
		VmProfile: []types.BaseVirtualMachineProfileSpec{&types.VirtualMachineDefinedProfileSpec{ProfileId: policy}}}
	tk, err := folders.VmFolder.CreateVM(ctx, spec, pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := tk.WaitForResult(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	vm := info.Result.(types.ManagedObjectReference)
	ref := pbmtypes.PbmServerObjectRef{ObjectType: string(pbmtypes.PbmObjectTypeVirtualMachine), Key: vm.Value}
	got, err := pc.QueryAssociatedProfile(ctx, ref)
	if err != nil || len(got) != 1 || got[0].UniqueId != policy {
		t.Fatalf("associated profile after create: %v %v", got, err)
	}
	res, err := pc.QueryAssociatedProfiles(ctx, []pbmtypes.PbmServerObjectRef{ref})
	if err != nil || len(res) != 1 || len(res[0].ProfileId) != 1 {
		t.Fatalf("associated profiles: %v %v", res, err)
	}
	// 호환 데이터스토어(placement)
	if hubs, err := pc.CheckRequirements(ctx, nil, nil, []pbmtypes.BasePbmPlacementRequirement{&pbmtypes.PbmPlacementCapabilityProfileRequirement{ProfileId: pbmtypes.PbmProfileId{UniqueId: policy}}}); err != nil || len(hubs) == 0 {
		t.Fatalf("check requirements: %v %v", hubs, err)
	}
	obj := find.NewFinder(c.Client, true)
	obj.SetDatacenter(dc)
	v, _ := obj.VirtualMachine(ctx, "pbm-vm")
	tk, err = v.Reconfigure(ctx, types.VirtualMachineConfigSpec{VmProfile: []types.BaseVirtualMachineProfileSpec{&types.VirtualMachineEmptyProfileSpec{}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tk.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := pc.QueryAssociatedProfile(ctx, ref); err != nil || len(got) != 0 {
		t.Fatalf("associated profile after empty spec: %v %v", got, err)
	}
}
