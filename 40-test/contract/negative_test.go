// MVP 기본 Negative 테스트 (PRD-2 TR-COV-0303, Phase 2a 종료 게이트).
// 범주: 잘못된 입력, 잘못된 ID, 중복 생성, Revision 충돌, 미지원 API, SOAP Fault, 참조·삭제 의존성, 스펙 제공.
package contract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

func soapFault(t *testing.T, err error) any {
	t.Helper()
	if err == nil || !soap.IsSoapFault(err) {
		t.Fatalf("want SOAP fault, got %v", err)
	}
	return soap.ToSoapFault(err).VimFault()
}

// 추적: T-TR-COV-0303 T-FR-VC-0417 T-FR-VC-0504 T-FR-DM-0205 T-NFR-0202 T-FR-BE-0402 T-FR-FLT-0101 T-FR-FLT-0104 T-FR-FLT-0105 T-FR-FLT-0106 T-FR-FLT-0121
func TestNegativeSOAP(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")

	// 잘못된 ID: 없는 VM에 대한 호출 → ManagedObjectNotFound
	ghost := object.NewVirtualMachine(c.Client, types.ManagedObjectReference{Type: "VirtualMachine", Value: "vm-99999"})
	_, err := ghost.PowerOn(ctx)
	if _, ok := soapFault(t, err).(types.ManagedObjectNotFound); !ok {
		t.Fatalf("invalid id: %T %v", soap.ToSoapFault(err).VimFault(), err)
	}

	// 잘못된 입력: 존재하지 않는 속성 경로 → InvalidProperty(Fault가 아닌 missingSet으로 보고됨)
	var vm mo.VirtualMachine
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, err := f.VirtualMachineList(ctx, "*")
	if err != nil || len(vms) == 0 {
		t.Fatalf("vm list: %v", err)
	}
	err = property.DefaultCollector(c.Client).RetrieveOne(ctx, vms[0].Reference(), []string{"config.noSuchField"}, &vm)
	if err == nil || !strings.Contains(err.Error(), "InvalidProperty") {
		t.Fatalf("invalid property: %v", err)
	}

	// 중복 생성: 같은 폴더에 같은 이름 → Task error DuplicateName
	name, _ := vms[0].ObjectName(ctx)
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	dss, _ := f.DatastoreList(ctx, "*")
	dsName, _ := dss[0].ObjectName(ctx)
	tk, err := folders.VmFolder.CreateVM(ctx, types.VirtualMachineConfigSpec{
		Name: name, GuestId: string(types.VirtualMachineGuestOsIdentifierOtherGuest64), NumCPUs: 1, MemoryMB: 512,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"},
	}, pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tk.WaitForResult(ctx, nil)
	te, ok := err.(task.Error)
	if !ok {
		t.Fatalf("duplicate create: want task error, got %v", err)
	}
	if _, ok := te.Fault().(*types.DuplicateName); !ok {
		t.Fatalf("duplicate create: %T", te.Fault())
	}

	// SOAP Fault: 인증 없는 세션 → NotAuthenticated
	anon := e.SOAP("")
	_, err = methods.CurrentTime(ctx, anon.Client, &types.CurrentTime{This: anon.ServiceContent.RootFolder})
	if err == nil {
		// CurrentTime은 ServiceInstance 메서드라 RootFolder에서는 거부돼야 한다
		t.Fatal("CurrentTime on Folder must fault")
	}
	_, err = methods.RetrievePropertiesEx(ctx, anon.Client, &types.RetrievePropertiesEx{
		This: anon.ServiceContent.PropertyCollector,
		SpecSet: []types.PropertyFilterSpec{{
			PropSet:   []types.PropertySpec{{Type: "Folder", PathSet: []string{"name"}}},
			ObjectSet: []types.ObjectSpec{{Obj: anon.ServiceContent.RootFolder}},
		}},
	})
	if _, ok := soapFault(t, err).(types.NotAuthenticated); !ok {
		t.Fatalf("unauthenticated: %T %v", soap.ToSoapFault(err).VimFault(), err)
	}
}

// 미지원 SOAP 메서드는 Fault로 응답하고 미지원 API 집계에 남는다.
// 추적: T-TR-COV-0303 T-FR-API-0401 T-FR-API-0402 T-FR-API-0404 T-FR-NSX-0231 T-FR-FLT-0109
func TestNegativeUnsupported(t *testing.T) {
	e := testenv.Start(t, nil)
	env := `<?xml version="1.0" encoding="UTF-8"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body>` +
		`<NoSuchMethod_Task xmlns="urn:vim25"><_this type="Folder">group-d1</_this></NoSuchMethod_Task></soapenv:Body></soapenv:Envelope>`
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/sdk", strings.NewReader(env))
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("SOAPAction", "urn:vim25/7.0.3.0")
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError || !strings.Contains(string(raw), "Fault") {
		t.Fatalf("unknown SOAP method: %d %s", res.StatusCode, raw)
	}

	code, _ := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/no-such-collection", "")
	if code != http.StatusNotFound {
		t.Fatalf("NSX path outside spec: %d", code)
	}
	// 활성 프로파일 스펙에 없는 Manager API(상위 버전에서 제거된 API 포함)는 성공 위장 없이 404(FR-NSX-0231)
	if code, o := nsxDo(t, e, http.MethodGet, "/api/v1/logical-switches-removed", ""); code != http.StatusNotFound || o["error_message"] == nil {
		t.Fatalf("Manager API outside spec: %d %v", code, o)
	}
	code, _ = nsxDo(t, e, http.MethodDelete, "/policy/api/v1/infra/realized-state/status?intent_path=/infra/segments/x", "")
	if code != http.StatusMethodNotAllowed {
		t.Fatalf("NSX method outside spec: %d", code)
	}

	res, err = e.AdminDo(http.MethodGet, "/v1/unsupported", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	for _, want := range []string{"SOAP NoSuchMethod_Task", "/policy/api/v1/infra/no-such-collection"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("unsupported aggregation missing %q: %s", want, raw)
		}
	}
}

// 추적: T-TR-COV-0303 T-FR-API-0203 T-FR-FLT-0101 T-FR-FLT-0105
func TestNegativeREST(t *testing.T) {
	e := testenv.Start(t, nil)
	c := loadContract(t)
	r := login(t, e, c)
	code, b := r.do(http.MethodGet, "/api/vcenter/vm/vm-99999", true)
	if code != http.StatusNotFound {
		t.Fatalf("REST invalid id: %d %s", code, b)
	}
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	if body["error_type"] != "NOT_FOUND" {
		t.Fatalf("REST error body: %s", b)
	}
	if code, _ := r.do(http.MethodGet, "/api/vcenter/vm", false); code != http.StatusUnauthorized {
		t.Fatalf("REST unauthenticated: %d", code)
	}
}

// 추적: T-TR-COV-0303 T-FR-NSX-0409 T-FR-NSX-0410 T-FR-FLT-0104 T-FR-FLT-0105 T-FR-FLT-0108 T-FR-NSX-0415 T-FR-BE-0205 T-FR-FLT-0107
// FR-NSX-0104(스펙 제공)도 여기서 시험하지만 경로 근거 미확보(가정)로 이월 유지(carryover.yaml).
func TestNegativeNSX(t *testing.T) {
	e := testenv.Start(t, nil)
	const seg = "/policy/api/v1/infra/segments/neg"

	// 잘못된 입력: JSON 파싱 불가, 타입 위반
	if code, o := nsxDo(t, e, http.MethodPatch, seg, `{"display_name":`); code != http.StatusBadRequest || o["error_message"] == nil {
		t.Fatalf("malformed JSON: %d %v", code, o)
	}
	if code, _ := nsxDo(t, e, http.MethodPatch, seg, `{"display_name":42}`); code != http.StatusBadRequest {
		t.Fatalf("type violation: %d", code)
	}
	// 잘못된 ID: 없는 객체 조회·삭제 대상
	if code, _ := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/segments/no-such", ""); code != http.StatusNotFound {
		t.Fatalf("invalid id: %d", code)
	}
	// 중복 생성: NSX PATCH는 upsert이므로 오류가 아니다(같은 ID 재생성 → 200, revision 증가)
	nsxDo(t, e, http.MethodPatch, seg, `{"display_name":"neg"}`)
	if code, _ := nsxDo(t, e, http.MethodPatch, seg, `{"display_name":"neg"}`); code != 200 {
		t.Fatalf("upsert: %d", code)
	}
	_, o := nsxDo(t, e, http.MethodGet, seg, "")
	if o["_revision"] != float64(1) {
		t.Fatalf("upsert revision: %v", o)
	}
	// Revision 충돌: 오래된 _revision → 412
	if code, o := nsxDo(t, e, http.MethodPatch, seg, `{"display_name":"neg","_revision":0}`); code != http.StatusPreconditionFailed || o["error_code"] == nil {
		t.Fatalf("stale revision: %d %v", code, o)
	}
	// 참조 검증: 없는 경로 참조 → 400
	if code, _ := nsxDo(t, e, http.MethodPatch, seg, `{"connectivity_path":"/infra/tier-1s/missing"}`); code != http.StatusBadRequest {
		t.Fatalf("dangling reference: %d", code)
	}
	// 삭제 의존성: 참조 중인 Tier-1 삭제 → 400, 참조 해제 후 삭제 → 200
	nsxDo(t, e, http.MethodPatch, "/policy/api/v1/infra/tier-1s/t1-neg", `{}`)
	if code, _ := nsxDo(t, e, http.MethodPatch, seg, `{"connectivity_path":"/infra/tier-1s/t1-neg"}`); code != 200 {
		t.Fatalf("valid reference: %d", code)
	}
	if code, _ := nsxDo(t, e, http.MethodDelete, "/policy/api/v1/infra/tier-1s/t1-neg", ""); code != http.StatusBadRequest {
		t.Fatalf("delete referenced: %d", code)
	}
	nsxDo(t, e, http.MethodDelete, seg, "")
	if code, _ := nsxDo(t, e, http.MethodDelete, "/policy/api/v1/infra/tier-1s/t1-neg", ""); code != 200 {
		t.Fatalf("delete after unreference: %d", code)
	}

	// 스펙 제공(FR-NSX-0104): 활성 프로파일 스펙은 제공, 그 외 파일은 404
	req, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/api/v1/spec/openapi/nsx_policy_api.json", nil)
	u := e.User("nsx")
	req.SetBasicAuth(u.Name, u.Password)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	_ = json.NewDecoder(res.Body).Decode(&spec)
	res.Body.Close()
	if res.StatusCode != 200 || spec["paths"] == nil {
		t.Fatalf("spec serving: %d", res.StatusCode)
	}
	if code, _ := nsxDo(t, e, http.MethodGet, "/api/v1/spec/openapi/../profile.yaml", ""); code != http.StatusNotFound {
		t.Fatalf("spec traversal: %d", code)
	}
}
