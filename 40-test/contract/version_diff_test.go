// 추가 버전 프로파일과 버전 간 Differential Test (PRD-4 WP 4.7, FR-API-0311, AC-16 확장, R-08).
// 같은 Fixture·같은 작업을 기준 프로파일(7.0.3)과 추가 프로파일(7.0.2)에 실행하고 응답 계약을 비교한다.
package contract

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/profile"
	"cloudinfra-sim/test/internal/testenv"
)

type versionRun struct {
	api      string
	about    types.AboutInfo
	fields   map[string]string // 응답 데이터 속성 → 도입 버전
	findings []string
	versions string
}

func runVersion(t *testing.T, vc string) versionRun {
	t.Helper()
	e := testenv.Start(t, func(c *config.Config) { c.VCenterVersion = vc })
	contract := loadVimContract(t, e)
	c, rec := e.RecordingLogin("") // 클라이언트 최신 버전 요청 → 프로파일 버전으로 협상
	mvpWorkload(t, e, c, "diff-vm")
	// 7.0.3.0 도입 속성을 상태에 둔다(두 버전 모두 같은 요청). 7.0.2 응답은 이 속성을 숨겨야 한다.
	vms := listVMs(t, c)
	tk, err := vms[0].Reconfigure(context.Background(), types.VirtualMachineConfigSpec{VmOpNotificationToAppEnabled: types.NewBool(true)})
	if err != nil {
		t.Fatal(err)
	}
	if err := tk.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	var mvm []mo.VirtualMachine
	if err := property.DefaultCollector(c.Client).Retrieve(context.Background(), []types.ManagedObjectReference{vms[0].Reference()}, []string{"config"}, &mvm); err != nil {
		t.Fatal(err)
	}
	run := versionRun{api: e.Sim.Profile.VCenter.APIVersion, about: c.ServiceContent.About, fields: map[string]string{}}
	for _, x := range rec.Exchanges {
		fs, fields := contract.CheckResponseFields(x.Method, x.SessionVersion, x.Response)
		for _, f := range fs {
			run.findings = append(run.findings, x.Method+" "+f.String())
		}
		for k, v := range fields {
			run.fields[k] = v
		}
		if x.SessionVersion != run.api {
			t.Fatalf("%s: session version %s, want profile %s", vc, x.SessionVersion, run.api)
		}
	}
	res, err := e.Client.Get("https://vcenter." + testenv.Domain + "/sdk/vimServiceVersions.xml")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	run.versions = string(b)
	return run
}

// T-FR-API-0311 T-FR-VC-0605 T-FR-VC-0606 T-FR-VC-0607 T-FR-VC-0207 T-FR-API-0305 T-FR-API-0306
func TestVersionDifferential(t *testing.T) {
	const older, base = "7.0.2.00000", testenv.BaseVCenterVersion
	a, b := runVersion(t, older), runVersion(t, base)

	// 1) 각 버전 응답이 자기 세션 버전 계약을 지킨다(없는 속성·순서·필수·열거값). 7.0.2 응답에 7.0.2.0 이후 속성이 없다.
	for _, r := range []versionRun{a, b} {
		if len(r.findings) > 0 {
			sort.Strings(r.findings)
			if len(r.findings) > 10 {
				r.findings = r.findings[:10]
			}
			t.Errorf("%s contract findings:\n  %s", r.api, strings.Join(r.findings, "\n  "))
		}
	}
	for f, since := range a.fields {
		if profile.Newer(since, a.api) {
			t.Errorf("7.0.2 response exposes %s (since %s)", f, since)
		}
	}

	// 2) 차이는 정확히 버전 도입 정보로 설명된다: 7.0.3에만 있는 속성은 7.0.2.0 이후 도입, 7.0.2에만 있는 속성은 없다.
	var onlyNew []string
	for f, since := range b.fields {
		if _, ok := a.fields[f]; ok {
			continue
		}
		if !profile.Newer(since, a.api) {
			t.Errorf("%s (since %s) present in 7.0.3 but missing in 7.0.2 response", f, since)
		}
		onlyNew = append(onlyNew, f+"@"+since)
	}
	for f := range a.fields {
		if _, ok := b.fields[f]; !ok {
			t.Errorf("%s present only in 7.0.2 response", f)
		}
	}
	sort.Strings(onlyNew)
	t.Logf("7.0.3-only fields (%d): %s", len(onlyNew), strings.Join(onlyNew, ", "))
	if !strings.Contains(strings.Join(onlyNew, ","), "VirtualMachineConfigInfo.vmOpNotificationToAppEnabled@7.0.3.0") {
		t.Error("7.0.3 must expose VirtualMachineConfigInfo.vmOpNotificationToAppEnabled set in state")
	}
	if len(onlyNew) == 0 {
		t.Error("differential is vacuous: the workload exercised no field introduced after 7.0.2.0")
	}

	// 3) 제품 식별(About)·버전 협상 문서는 프로파일별 값이다.
	if a.about.ApiVersion != "7.0.2.0" || a.about.Build != "17694817" || b.about.ApiVersion != "7.0.3.0" || b.about.Build != "24927011" {
		t.Errorf("about: 7.0.2=%s/%s 7.0.3=%s/%s", a.about.ApiVersion, a.about.Build, b.about.ApiVersion, b.about.Build)
	}
	if !strings.Contains(a.versions, "<version>7.0.2.0</version>") || strings.Contains(a.versions, "7.0.2.1") || strings.Contains(a.versions, "7.0.3") {
		t.Errorf("7.0.2 vimServiceVersions: %s", a.versions)
	}
	if !strings.Contains(b.versions, "<version>7.0.3.0</version>") || !strings.Contains(b.versions, "<version>7.0.2.1</version>") {
		t.Errorf("7.0.3 vimServiceVersions: %s", b.versions)
	}
}

// T-FR-VC-0607 T-FR-API-0311: 7.0.2.0 이후 도입 메서드는 7.0.2 프로파일에서 버전 미지원 Fault, 7.0.3에서는 버전 검사를 통과한다.
func TestVersionDifferentialMethods(t *testing.T) {
	call := func(vc string) error {
		e := testenv.Start(t, func(c *config.Config) { c.VCenterVersion = vc })
		c := e.Login("")
		ctx := context.Background()
		cl := c.Client
		req := types.GetSystemVMsRestrictedDatastores{This: firstRef(t, e, "ClusterComputeResource")} // since 7.0.3.0
		_, err := methods.GetSystemVMsRestrictedDatastores(ctx, cl, &req)
		return err
	}
	old := call("7.0.2.00000")
	if old == nil || !strings.Contains(old.Error(), "not available in API version 7.0.2.0") {
		t.Fatalf("7.0.2: want version fault, got %v", old)
	}
	if _, ok := soapFaultOf(old).(types.MethodNotFound); !ok {
		t.Fatalf("7.0.2: fault type %T", soapFaultOf(old))
	}
	cur := call(testenv.BaseVCenterVersion)
	if cur != nil && strings.Contains(cur.Error(), "not available in API version") {
		t.Fatalf("7.0.3: method must pass the version check, got %v", cur)
	}
	t.Logf("7.0.3 (vcsim 미구현 메서드): %v", cur)
}

func listVMs(t *testing.T, c *govmomi.Client) []*object.VirtualMachine {
	t.Helper()
	f := find.NewFinder(c.Client, true)
	dc, err := f.DefaultDatacenter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.SetDatacenter(dc)
	vms, err := f.VirtualMachineList(context.Background(), "*")
	if err != nil || len(vms) == 0 {
		t.Fatalf("vms: %v", err)
	}
	return vms
}

func soapFaultOf(err error) any {
	if soap.IsSoapFault(err) {
		return soap.ToSoapFault(err).VimFault()
	}
	return nil
}

// T-FR-API-0305: 상호운용성 미확인 조합은 development 모드에서만 기동하고 release는 거부한다.
func TestUnverifiedCombinationRefusedInRelease(t *testing.T) {
	err := newSim(t, func(c *config.Config) { c.VCenterVersion = "7.0.2.00000"; c.VerificationMode = "release" })
	if err == nil || !strings.Contains(err.Error(), "상호운용성 미확인") {
		t.Fatalf("want interop refusal, got %v", err)
	}
	_ = http.MethodGet
}
