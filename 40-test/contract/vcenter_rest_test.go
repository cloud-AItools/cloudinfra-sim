// vCenter REST(/api) 계약 테스트 (VC-REST-001 ~ VC-REST-004). 근거: SRC-VAPI-SDK-7032(SDK 바인딩·런타임에서 추출한 계약).
package contract

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"cloudinfra-sim/test/internal/testenv"
	"cloudinfra-sim/test/internal/vapicheck"
)

func loadContract(t *testing.T) *vapicheck.Contract {
	t.Helper()
	c, err := vapicheck.Load(vapicheck.DefaultPath())
	if err != nil {
		t.Skipf("vAPI 계약 파일 없음(VAPI_CONTRACT): %v", err)
	}
	return c
}

type restClient struct {
	t     *testing.T
	e     *testenv.Env
	token string
}

func (r *restClient) do(method, path string, auth bool) (int, []byte) {
	r.t.Helper()
	req, _ := http.NewRequest(method, "https://vcenter."+testenv.Domain+path, nil)
	if auth {
		req.Header.Set("vmware-api-session-id", r.token)
	}
	res, err := r.e.Client.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func login(t *testing.T, e *testenv.Env, c *vapicheck.Contract) *restClient {
	t.Helper()
	u := e.User("vcenter")
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.SetBasicAuth(u.Name, u.Password)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode/100 != 2 {
		t.Fatalf("POST /api/session: %d %s", res.StatusCode, b)
	}
	if errs := c.CheckOperation("com.vmware.cis.session.create", b); len(errs) > 0 {
		t.Fatalf("session.create output: %v", errs)
	}
	return &restClient{t: t, e: e, token: strings.Trim(strings.TrimSpace(string(b)), `"`)}
}

// T-FR-API-0202 T-FR-API-0203: 세션 생성·조회·삭제와 오류 계약(VC-REST-001)
func TestRESTSessionContract(t *testing.T) {
	c := loadContract(t)
	e := testenv.Start(t, nil)
	r := login(t, e, c)
	code, b := r.do(http.MethodGet, "/api/session", true)
	if code != 200 {
		t.Fatalf("GET /api/session: %d %s", code, b)
	}
	if errs := c.CheckOperation("com.vmware.cis.session.get", b); len(errs) > 0 {
		t.Fatalf("session.get output: %v\n%s", errs, b)
	}
	// 잘못된 자격 증명 → 선언 오류 UNAUTHENTICATED, vAPI 매핑 401, Error 구조
	u := e.User("vcenter")
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.SetBasicAuth(u.Name, "wrong")
	res, _ := e.Client.Do(req)
	eb, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if errs := c.CheckError("com.vmware.cis.session.create", "UNAUTHENTICATED", res.StatusCode, eb); len(errs) > 0 {
		t.Fatalf("session.create error: %v\n%s", errs, eb)
	}
	// 삭제 → 204(vAPI 런타임 DELETE 규칙), 이후 조회는 UNAUTHENTICATED
	if code, _ := r.do(http.MethodDelete, "/api/session", true); code != http.StatusNoContent {
		t.Fatalf("DELETE /api/session: %d", code)
	}
	code, b = r.do(http.MethodGet, "/api/session", true)
	if errs := c.CheckError("com.vmware.cis.session.get", "UNAUTHENTICATED", code, b); len(errs) > 0 {
		t.Fatalf("session.get after delete: %v\n%s", errs, b)
	}
}

// T-FR-VC-0203 T-FR-VC-0205 T-FR-VC-0301 T-FR-VC-0302 T-FR-VC-0303 T-FR-VC-0304 T-FR-VC-0305 T-FR-VC-0306
// 인벤토리 조회 계약(VC-REST-002, VC-REST-003)
func TestRESTInventoryContract(t *testing.T) {
	c := loadContract(t)
	e := testenv.Start(t, nil)
	r := login(t, e, c)
	for _, tc := range []struct{ op, path string }{
		{"com.vmware.vcenter.VM.list", "/api/vcenter/vm"},
		{"com.vmware.vcenter.datacenter.list", "/api/vcenter/datacenter"},
		{"com.vmware.vcenter.cluster.list", "/api/vcenter/cluster"},
		{"com.vmware.vcenter.host.list", "/api/vcenter/host"},
		{"com.vmware.vcenter.datastore.list", "/api/vcenter/datastore"},
		{"com.vmware.vcenter.network.list", "/api/vcenter/network"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			op := c.Operations[tc.op]
			if op.Method != http.MethodGet || "/api"+op.URL != tc.path {
				t.Fatalf("contract route %s %s != GET %s", op.Method, op.URL, tc.path)
			}
			code, b := r.do(http.MethodGet, tc.path, true)
			if code != 200 {
				t.Fatalf("%d %s", code, b)
			}
			if errs := c.CheckOperation(tc.op, b); len(errs) > 0 {
				t.Fatalf("%s output: %v", tc.op, errs[:min(len(errs), 5)])
			}
			code, b = r.do(http.MethodGet, tc.path, false)
			if errs := c.CheckError(tc.op, "UNAUTHENTICATED", code, b); len(errs) > 0 {
				t.Fatalf("%s unauthenticated: %v", tc.op, errs)
			}
		})
	}
}

// T-FR-VC-0305 T-FR-DM-0204 T-FR-FLT-0105: VM 단건 조회는 VM.Info, 없는 VM은 NOT_FOUND(VC-REST-002)
func TestRESTVMGetContract(t *testing.T) {
	c := loadContract(t)
	e := testenv.Start(t, nil)
	r := login(t, e, c)
	vm := e.Sim.VC.Model().Map().AllReference("VirtualMachine")[0].Reference().Value
	code, b := r.do(http.MethodGet, "/api/vcenter/vm/"+vm, true)
	if code != 200 {
		t.Fatalf("GET vm: %d %s", code, b)
	}
	if errs := c.CheckOperation("com.vmware.vcenter.VM.get", b); len(errs) > 0 {
		t.Fatalf("VM.get output (%d issues): %v\n%s", len(errs), errs[:min(len(errs), 8)], b)
	}
	code, b = r.do(http.MethodGet, "/api/vcenter/vm/vm-999999", true)
	if errs := c.CheckError("com.vmware.vcenter.VM.get", "NOT_FOUND", code, b); len(errs) > 0 {
		t.Fatalf("VM.get not found: %v\n%s", errs, b)
	}
}

// T-FR-API-0306: 어플라이언스 버전(VC-REST-004)
func TestRESTApplianceVersionContract(t *testing.T) {
	c := loadContract(t)
	e := testenv.Start(t, nil)
	r := login(t, e, c)
	code, b := r.do(http.MethodGet, "/api/appliance/system/version", true)
	if code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	if errs := c.CheckOperation("com.vmware.appliance.system.version.get", b); len(errs) > 0 {
		t.Fatalf("version.get output: %v\n%s", errs, b)
	}
}
