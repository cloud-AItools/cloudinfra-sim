// Scenario 09 — Task 실패와 복구, Scenario 10 — 세션 만료와 재인증 (PRD-2 TR-E2E-0901 ~ 0904, TR-E2E-1001 ~ 1003,
// Phase 4a 종료 게이트). 장애는 Admin API Fault Rule(FR-FLT-0206)과 Virtual Clock(WP 4.10)으로 주입한다.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

func admin(t *testing.T, e *testenv.Env, method, path, body string) (int, map[string]any) {
	t.Helper()
	res, err := e.AdminDo(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out
}

func vmsNamed(t *testing.T, v *vcTarget, name string) int {
	t.Helper()
	list, _ := v.finder.VirtualMachineList(context.Background(), "*")
	n := 0
	for _, vm := range list {
		if vm.Name() == name {
			n++
		}
	}
	return n
}

// TR-E2E-0901 ①~⑤, 성공 기준 0902 ~ 0904.
// 추적: T-TR-E2E-0901 T-TR-E2E-0902 T-TR-E2E-0903 T-TR-E2E-0904 T-FR-FLT-0122 T-FR-FLT-0113 T-FR-FLT-0115 T-FR-FLT-0201 T-FR-FLT-0203 T-FR-FLT-0204 T-TR-COV-0308 T-TR-SCN-0303 T-FR-BE-0204 T-TR-COV-0309
func TestScenario09_TaskFailureRecovery(t *testing.T) {
	e := testenv.Start(t, nil)
	v := target(t, e)
	ctx := context.Background()
	user := e.User("vcenter").Name

	// ② Task 실행 중 실패 주입: 이 사용자의 다음 CreateVM_Task 1회, running으로 300 ms 뒤 InsufficientResourcesFault
	if code, r := admin(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"soap_method":"CreateVM_Task","user":"`+user+`"},"action":{"type":"task_fail","fault":"InsufficientResourcesFault","delay_ms":300},"count":1}`); code != 201 {
		t.Fatalf("add fault: %d %v", code, r)
	}
	// ① VM 생성 요청 ③ 오류 감지: running을 거쳐 error, 지정한 Fault 타입(TR-E2E-0902)
	dsName, _ := v.ds.ObjectName(ctx)
	spec := types.VirtualMachineConfigSpec{Name: "s09-vm", GuestId: "otherGuest64", NumCPUs: 1, MemoryMB: 512, Files: &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"}}
	tk, err := v.folder.CreateVM(ctx, spec, v.pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	var states []types.TaskInfoState
	pc := property.DefaultCollector(v.c.Client)
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_ = property.Wait(wctx, pc, tk.Reference(), []string{"info.state"}, func(ch []types.PropertyChange) bool {
		for _, c := range ch {
			if s, ok := c.Val.(types.TaskInfoState); ok {
				states = append(states, s)
				return s == types.TaskInfoStateError || s == types.TaskInfoStateSuccess
			}
		}
		return false
	})
	cancel()
	_, err = tk.WaitForResult(ctx, nil)
	if f := taskFault(t, err); f == nil {
		t.Fatal("expected task fault")
	} else if _, ok := f.(*types.InsufficientResourcesFault); !ok {
		t.Fatalf("TR-E2E-0902: fault %T", f)
	}
	sawRunning := false
	for _, s := range states {
		sawRunning = sawRunning || s == types.TaskInfoStateRunning
	}
	if !sawRunning {
		t.Fatalf("task must fail while running: %v", states)
	}
	// ④ 복구 Workflow: 실패한 시도가 만든 것이 없는지 확인(보상 불필요) 후 재시도 → 성공(TR-E2E-0903)
	if n := vmsNamed(t, v, "s09-vm"); n != 0 {
		t.Fatalf("failed attempt left %d VM(s)", n)
	}
	ref, err := v.create(t, "s09-vm", 1, 512)
	if err != nil {
		t.Fatalf("TR-E2E-0903 retry: %v", err)
	}
	// 보상: 전원 켜기 실패(인프라 Fault FR-FLT-0113) → 클라이언트가 만든 VM을 지워 원상 복구
	vm := object.NewVirtualMachine(v.c.Client, ref)
	admin(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"soap_method":"PowerOnVM_Task","resource":"`+ref.Value+`"},"action":{"type":"task_fail","fault":"InvalidState"},"count":1}`)
	tk, _ = vm.PowerOn(ctx)
	if _, err := tk.WaitForResult(ctx, nil); err == nil {
		t.Fatal("power on must fail")
	}
	tk, _ = vm.Destroy(ctx)
	if err := tk.Wait(ctx); err != nil {
		t.Fatalf("compensation destroy: %v", err)
	}
	// 재시도 후 최종 상태(⑤): 같은 이름 VM이 정확히 한 번 생성됐다가 보상으로 지워졌다 — 불필요한 중복 없음(TR-E2E-0904)
	ref2, err := v.create(t, "s09-vm", 1, 512)
	if err != nil {
		t.Fatal(err)
	}
	if n := vmsNamed(t, v, "s09-vm"); n != 1 {
		t.Fatalf("TR-E2E-0904: %d VMs named s09-vm", n)
	}
	tk, _ = object.NewVirtualMachine(v.c.Client, ref2).PowerOn(ctx)
	if err := tk.Wait(ctx); err != nil {
		t.Fatalf("power on after rule exhausted: %v", err)
	}
	// ResourcePool 할당 실패(FR-FLT-0115)도 같은 규칙 모델로: 지정 Fault가 그대로 보인다
	admin(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"soap_method":"CreateVM_Task"},"action":{"type":"task_fail","fault":"InsufficientCpuResourcesFault"},"count":1}`)
	spec.Name = "s09-rp"
	tk, _ = v.folder.CreateVM(ctx, spec, v.pool, nil)
	_, err = tk.WaitForResult(ctx, nil)
	if _, ok := taskFault(t, err).(*types.InsufficientCpuResourcesFault); !ok {
		t.Fatalf("resource pool fault: %v", err)
	}
}

// TR-E2E-1001 ①~⑥, 성공 기준 1002·1003. 주입(세션 계층 Fault)과 가상 시간 유휴 만료 두 경로를 모두 본다.
// 추적: T-TR-E2E-1001 T-TR-E2E-1002 T-TR-E2E-1003 T-FR-API-0204 T-FR-API-0209 T-FR-BE-0607 T-FR-FLT-0102 T-FR-API-0201 T-FR-API-0207 T-TR-SCN-0108 T-TR-COV-0309 T-TR-COV-0308
func TestScenario10_SessionExpiry(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	vu := e.User("vcenter")
	retrieve := func(c interface {
		RetrieveOne(context.Context, types.ManagedObjectReference, []string, any) error
	}, root types.ManagedObjectReference) error {
		var f mo.Folder
		return c.RetrieveOne(ctx, root, []string{"name"}, &f)
	}

	// ① 로그인 ② 세션 생성
	c := e.Login("")
	if err := retrieve(c, c.ServiceContent.RootFolder); err != nil {
		t.Fatal(err)
	}
	// ③ 만료 이벤트 주입(세션 계층, 이 사용자 다음 요청 1회)
	if code, r := admin(t, e, http.MethodPost, "/v1/faults", `{"layer":"session","match":{"product":"vcenter","user":"`+vu.Name+`"},"action":{"type":"expire"},"count":1}`); code != 201 {
		t.Fatalf("session fault: %d %v", code, r)
	}
	// ④ 재요청 ⑤ 인증 실패: NotAuthenticated(TR-E2E-1002)
	err := retrieve(c, c.ServiceContent.RootFolder)
	if err == nil || !soap.IsSoapFault(err) {
		t.Fatalf("expired session must fault: %v", err)
	}
	if _, ok := soap.ToSoapFault(err).VimFault().(types.NotAuthenticated); !ok {
		t.Fatalf("fault %T", soap.ToSoapFault(err).VimFault())
	}
	// ⑥ 재로그인(TR-E2E-1003): 같은 클라이언트로 다시 로그인하면 정상
	if err := c.Login(ctx, url.UserPassword(vu.Name, vu.Password)); err != nil {
		t.Fatalf("re-login: %v", err)
	}
	if err := retrieve(c, c.ServiceContent.RootFolder); err != nil {
		t.Fatalf("after re-login: %v", err)
	}

	// 가상 시간 유휴 만료(FR-BE-0607): 테스트 모드에서 31분 진행 → SOAP·REST·NSX 세션 모두 만료
	restLogin := func() string {
		req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
		req.SetBasicAuth(vu.Name, vu.Password)
		res, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var tok string
		_ = json.NewDecoder(res.Body).Decode(&tok)
		return tok
	}
	restGet := func(tok string) (int, map[string]any) {
		req, _ := http.NewRequest(http.MethodGet, "https://vcenter."+testenv.Domain+"/api/vcenter/vm", nil)
		req.Header.Set("vmware-api-session-id", tok)
		res, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	nu := e.User("nsx")
	nsxLogin := func() *http.Cookie {
		form := url.Values{"j_username": {nu.Name}, "j_password": {nu.Password}}
		res, err := e.Client.PostForm("https://nsx."+testenv.Domain+"/api/session/create", form)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		for _, ck := range res.Cookies() {
			if ck.Name == "JSESSIONID" {
				return ck
			}
		}
		t.Fatal("no JSESSIONID")
		return nil
	}
	nsxGet := func(ck *http.Cookie) int {
		req, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/api/v1/node/version", nil)
		req.AddCookie(ck)
		res, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	tok, ck := restLogin(), nsxLogin()
	if code, _ := restGet(tok); code != 200 || nsxGet(ck) != 200 {
		t.Fatal("sessions must work before expiry")
	}
	// 29분: 아직 유효(유휴 기준은 마지막 사용)
	if code, st := admin(t, e, http.MethodPut, "/v1/clock", `{"mode":"test","advance_ms":1740000}`); code != 200 || st["mode"] != "test" {
		t.Fatalf("clock: %d %v", code, st)
	}
	if err := retrieve(c, c.ServiceContent.RootFolder); err != nil {
		t.Fatalf("29 min idle must still be valid: %v", err)
	}
	// 마지막 사용 뒤 31분
	admin(t, e, http.MethodPut, "/v1/clock", `{"advance_ms":1860000}`)
	if err := retrieve(c, c.ServiceContent.RootFolder); err == nil {
		t.Fatal("SOAP session must expire after idle timeout")
	}
	if code, body := restGet(tok); code != http.StatusUnauthorized || body["error_type"] != "UNAUTHENTICATED" {
		t.Fatalf("REST expired: %d %v", code, body)
	}
	if code := nsxGet(ck); code != http.StatusForbidden {
		t.Fatalf("NSX expired session: %d", code)
	}
	// 재인증(TR-E2E-1003)
	if err := c.Login(ctx, url.UserPassword(vu.Name, vu.Password)); err != nil {
		t.Fatal(err)
	}
	if code, _ := restGet(restLogin()); code != 200 || nsxGet(nsxLogin()) != 200 {
		t.Fatal("re-authentication must succeed")
	}
	// 가상 시각이 SOAP 응답 시각에 반영된다(FR-BE-0611): CurrentTime ≈ 실제 + 1시간
	nowp, err := methods.GetCurrentTime(ctx, c.Client)
	var now time.Time
	if nowp != nil {
		now = *nowp
	}
	if err != nil || now.Sub(time.Now()) < 59*time.Minute {
		t.Fatalf("virtual CurrentTime: %v %v", now, err)
	}
}
