// Phase 4a: 권한(WP 4.2), Fault 계층 확장(WP 4.3), 오류 계약(WP 4.4), Task 지연·Realization Timeout(WP 4.5),
// Virtual Clock 효과(WP 4.10), 확장 Negative(WP 4.8, TR-COV-0308).
package contract

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

func adminJSON(t *testing.T, e *testenv.Env, method, path, body string) (int, map[string]any) {
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

func loginAs(t *testing.T, e *testenv.Env, user, pass string) *govmomi.Client {
	t.Helper()
	c := e.SOAP("")
	if err := c.Login(context.Background(), url.UserPassword(user, pass)); err != nil {
		t.Fatalf("login %s: %v", user, err)
	}
	return c
}

func inventory(t *testing.T, c *govmomi.Client) (*find.Finder, []*object.VirtualMachine) {
	t.Helper()
	ctx := context.Background()
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	return f, vms
}

func noPermission(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("want NoPermission, got success")
	}
	if te, ok := err.(interface{ Fault() types.BaseMethodFault }); ok {
		if np, ok := te.Fault().(*types.NoPermission); ok {
			return np.PrivilegeId
		}
	}
	if soap.IsSoapFault(err) {
		if np, ok := soap.ToSoapFault(err).VimFault().(types.NoPermission); ok {
			return np.PrivilegeId
		}
	}
	t.Fatalf("want NoPermission, got %v", err)
	return ""
}

// 추적: T-FR-API-0211 T-FR-API-0212 T-FR-API-0213 T-FR-FLT-0103 T-TR-COV-0308
func TestRBAC(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	_, vms := inventory(t, e.Login(""))
	scoped := vms[0].Reference().Value
	if st, _ := vms[0].PowerState(ctx); st != types.VirtualMachinePowerStatePoweredOn { // 범위 안 VM을 켠 상태로 둔다
		tk, _ := vms[0].PowerOn(ctx)
		_ = tk.Wait(ctx)
	}
	if st, _ := vms[1].PowerState(ctx); st != types.VirtualMachinePowerStatePoweredOn {
		tk, _ := vms[1].PowerOn(ctx)
		_ = tk.Wait(ctx)
	}

	code, ro := adminJSON(t, e, http.MethodPost, "/v1/users", `{"name":"auditor@vsphere.local","products":["vcenter"],"role":"ReadOnly"}`)
	if code != 201 || ro["password"] == "" {
		t.Fatalf("add user: %d %v", code, ro)
	}
	_, op := adminJSON(t, e, http.MethodPost, "/v1/users", `{"name":"operator@vsphere.local","products":["vcenter"],"role":"VirtualMachineOperator","scope":"`+scoped+`"}`)
	_, na := adminJSON(t, e, http.MethodPost, "/v1/users", `{"name":"nsx-auditor","products":["nsx"],"role":"auditor"}`)
	if code, _ := adminJSON(t, e, http.MethodPost, "/v1/users", `{"name":"x","products":["vcenter"],"role":"God"}`); code != 400 {
		t.Fatal("invalid role must be rejected")
	}
	// ReadOnly: 조회는 되고 전원 작업은 NoPermission(privilegeId = 공식 권한 ID)
	c := loginAs(t, e, "auditor@vsphere.local", ro["password"].(string))
	_, rvms := inventory(t, c)
	if len(rvms) != len(vms) {
		t.Fatal("read-only user must read inventory")
	}
	_, err := object.NewVirtualMachine(c.Client, vms[1].Reference()).PowerOff(ctx)
	if p := noPermission(t, err); p != "VirtualMachine.Interact.PowerOff" {
		t.Fatalf("privilege %q", p)
	}
	// VirtualMachineOperator(범위 = vms[0]): 범위 안 전원 작업은 되고, 범위 밖·VM 생성은 거부
	oc := loginAs(t, e, "operator@vsphere.local", op["password"].(string))
	tk, err := object.NewVirtualMachine(oc.Client, vms[0].Reference()).PowerOff(ctx)
	var werr error
	if err == nil {
		werr = tk.Wait(ctx)
	}
	if err != nil || werr != nil {
		t.Fatalf("in-scope power off: %v %v", err, werr)
	}
	_, err = object.NewVirtualMachine(oc.Client, vms[1].Reference()).PowerOff(ctx)
	noPermission(t, err)
	f, _ := inventory(t, oc)
	dc, _ := f.DefaultDatacenter(ctx)
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	_, err = folders.VmFolder.CreateVM(ctx, types.VirtualMachineConfigSpec{Name: "op-vm", GuestId: "otherGuest64", Files: &types.VirtualMachineFileInfo{VmPathName: "[LocalDS_0]"}}, pool, nil)
	if p := noPermission(t, err); p != "VirtualMachine.Inventory.Create" {
		t.Fatalf("privilege %q", p)
	}
	// NSX auditor: 조회만
	req, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/policy/api/v1/infra/segments", nil)
	req.SetBasicAuth("nsx-auditor", na["password"].(string))
	res, _ := e.Client.Do(req)
	res.Body.Close()
	req, _ = http.NewRequest(http.MethodPatch, "https://nsx."+testenv.Domain+"/policy/api/v1/infra/segments/x", strings.NewReader(`{}`))
	req.SetBasicAuth("nsx-auditor", na["password"].(string))
	req.Header.Set("Content-Type", "application/json")
	res2, _ := e.Client.Do(req)
	var ae map[string]any
	_ = json.NewDecoder(res2.Body).Decode(&ae)
	res2.Body.Close()
	if res.StatusCode != 200 || res2.StatusCode != http.StatusForbidden || ae["error_code"] == nil {
		t.Fatalf("nsx auditor: GET %d, PATCH %d %v", res.StatusCode, res2.StatusCode, ae)
	}
	// 사용자 목록은 비밀번호를 내보내지 않는다, 시험 사용자 삭제
	res, _ = e.AdminDo(http.MethodGet, "/v1/users", nil)
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(b), ro["password"].(string)) {
		t.Fatal("user list must not expose passwords")
	}
	if code, _ := adminJSON(t, e, http.MethodDelete, "/v1/users/auditor@vsphere.local", ""); code != 204 {
		t.Fatal("delete user")
	}
	if code, _ := adminJSON(t, e, http.MethodDelete, "/v1/users/"+e.User("vcenter").Name, ""); code != 404 {
		t.Fatal("default user must not be deletable")
	}
}

// vCenter 의미·인프라 Fault, 조건(nth·after·시각), NSX 의미 Fault, Realization Timeout(정지).
// 추적: T-FR-MET-0201 T-FR-FLT-0201 T-FR-FLT-0202 T-FR-FLT-0203 T-FR-FLT-0204 T-FR-FLT-0110 T-FR-FLT-0111 T-FR-FLT-0112 T-FR-FLT-0114 T-FR-FLT-0116 T-FR-FLT-0108 T-FR-FLT-0121 T-FR-NSX-0504 T-FR-NSX-0506 T-TR-SCN-0109 T-TR-COV-0308
func TestSemanticAndInfraFaults(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	f, vms := inventory(t, c)
	hosts, _ := f.HostSystemList(ctx, "*")
	dss, _ := f.DatastoreList(ctx, "*")
	hostRef, dsRef := hosts[0].Reference().Value, dss[0].Reference().Value
	dsName, _ := dss[0].ObjectName(ctx)

	// 인프라 상태: 호스트 연결 끊김·유지보수, 데이터스토어 여유 공간 0 — 규칙이 있는 동안 유지, 제거하면 복구
	_, hd := adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"resource":"`+hostRef+`"},"action":{"type":"host_disconnect"}}`)
	_, hm := adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"resource":"`+hosts[1].Reference().Value+`"},"action":{"type":"host_maintenance"}}`)
	_, df := adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"resource":"`+dsRef+`"},"action":{"type":"datastore_full"}}`)
	var h0, h1 mo.HostSystem
	var ds mo.Datastore
	_ = c.RetrieveOne(ctx, hosts[0].Reference(), []string{"runtime"}, &h0)
	_ = c.RetrieveOne(ctx, hosts[1].Reference(), []string{"runtime"}, &h1)
	_ = c.RetrieveOne(ctx, dss[0].Reference(), []string{"summary"}, &ds)
	if h0.Runtime.ConnectionState != types.HostSystemConnectionStateDisconnected || !h1.Runtime.InMaintenanceMode || ds.Summary.FreeSpace != 0 {
		t.Fatalf("infra faults: conn=%s maint=%v free=%d", h0.Runtime.ConnectionState, h1.Runtime.InMaintenanceMode, ds.Summary.FreeSpace)
	}
	dc, _ := f.DefaultDatacenter(ctx)
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	tk, _ := folders.VmFolder.CreateVM(ctx, types.VirtualMachineConfigSpec{Name: "full-ds", GuestId: "otherGuest64", NumCPUs: 1, MemoryMB: 256, Files: &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"}}, pool, nil)
	if _, err := tk.WaitForResult(ctx, nil); err == nil || !strings.Contains(fmt.Sprintf("%T", err.(interface{ Fault() types.BaseMethodFault }).Fault()), "NoDiskSpace") {
		t.Fatalf("FR-FLT-0112 datastore full: %v", err)
	}
	// 사용률 임계 알람(FR-MET-0201 Datastore Usage Threshold): 기본 알람이 빨강으로 바뀌고 이벤트가 남는다
	var dsa mo.Datastore
	_ = c.RetrieveOne(ctx, dss[0].Reference(), []string{"overallStatus", "triggeredAlarmState"}, &dsa)
	if dsa.OverallStatus != types.ManagedEntityStatusRed || len(dsa.TriggeredAlarmState) != 1 {
		t.Fatalf("datastore usage alarm: %s %v", dsa.OverallStatus, dsa.TriggeredAlarmState)
	}
	evs, _ := event.NewManager(c.Client).QueryEvents(ctx, types.EventFilterSpec{})
	alarmEvent := false
	for _, ev := range evs {
		if a, ok := ev.(*types.AlarmStatusChangedEvent); ok && a.Alarm.Name == "Datastore usage on disk" && a.To == "red" && a.Entity.Entity == dss[0].Reference() {
			alarmEvent = true
		}
	}
	if !alarmEvent {
		t.Fatal("AlarmStatusChangedEvent for datastore usage")
	}
	for _, r := range []map[string]any{hd, hm, df} {
		adminJSON(t, e, http.MethodDelete, "/v1/faults/"+r["id"].(string), "")
	}
	_ = c.RetrieveOne(ctx, hosts[0].Reference(), []string{"runtime"}, &h0)
	_ = c.RetrieveOne(ctx, hosts[1].Reference(), []string{"runtime"}, &h1)
	_ = c.RetrieveOne(ctx, dss[0].Reference(), []string{"summary"}, &ds)
	if h0.Runtime.ConnectionState != types.HostSystemConnectionStateConnected || h1.Runtime.InMaintenanceMode || ds.Summary.FreeSpace == 0 {
		t.Fatal("removing infra faults must restore state")
	}
	_ = c.RetrieveOne(ctx, dss[0].Reference(), []string{"overallStatus", "triggeredAlarmState"}, &dsa)
	if dsa.OverallStatus != types.ManagedEntityStatusGreen || len(dsa.TriggeredAlarmState) != 0 {
		t.Fatalf("alarm must clear after free space restored: %s", dsa.OverallStatus)
	}

	// 조건: N번째 호출(nth=2)·호출 순서(after)·SOAP Fault(작업이 아닌 메서드, FR-FLT-0121)·Clone 실패(FR-FLT-0114)
	vm := vms[2]
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"soap_method":"PowerOffVM_Task","resource":"`+vm.Reference().Value+`","nth":2},"action":{"type":"task_fail","fault":"InvalidState"},"count":1}`)
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"soap_method":"CurrentTime","after":"PowerOnVM_Task"},"action":{"type":"soap_fault","fault":"SystemError"},"count":1}`)
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"soap_method":"CloneVM_Task"},"action":{"type":"task_fail","fault":"VmConfigFault"},"count":1}`)
	if _, err := methodsCurrentTime(ctx, c); err != nil {
		t.Fatalf("CurrentTime before 'after' call must succeed: %v", err)
	}
	for i, wantErr := range []bool{false, false, true} { // 끔(1번째 PowerOff) → 켬 → 끔(2번째 PowerOff 실패)
		var tk *object.Task
		if i == 1 {
			tk, _ = vm.PowerOn(ctx)
		} else {
			st, _ := vm.PowerState(ctx)
			if st == types.VirtualMachinePowerStatePoweredOff {
				tk, _ = vm.PowerOn(ctx)
				_ = tk.Wait(ctx)
			}
			tk, _ = vm.PowerOff(ctx)
		}
		if err := tk.Wait(ctx); (err != nil) != wantErr {
			t.Fatalf("step %d: err=%v want error=%v", i, err, wantErr)
		}
	}
	if _, err := methodsCurrentTime(ctx, c); err == nil || !soap.IsSoapFault(err) {
		t.Fatalf("SOAP fault after PowerOnVM_Task observed: %v", err)
	}
	tk, _ = vm.Clone(ctx, folders.VmFolder, "clone-x", types.VirtualMachineCloneSpec{})
	if _, err := tk.WaitForResult(ctx, nil); err == nil {
		t.Fatal("FR-FLT-0114 clone failure")
	}

	// NSX 의미: Realization 실패(오류 정보·알람), 정지(Timeout 재현), Revision 충돌 강제
	n := func(method, path, body string) (int, map[string]any) {
		return nsxDo(t, e, method, "/policy/api/v1"+path, body)
	}
	realization := func(p string) string {
		_, o := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/realized-state/status?intent_path="+p, "")
		return o["consolidated_status"].(map[string]any)["consolidated_status"].(string)
	}
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"nsx","match":{"path_prefix":"/policy/api/v1/infra/segments/bad"},"action":{"type":"realization_fail","message":"Transport node unreachable"},"count":1}`)
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"nsx","match":{"path_prefix":"/policy/api/v1/infra/segments/stuck"},"action":{"type":"realization_hang"},"count":1}`)
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"nsx","match":{"method":"PUT"},"action":{"type":"revision_conflict"},"count":1}`)
	n(http.MethodPatch, "/infra/segments/bad", `{}`)
	n(http.MethodPatch, "/infra/segments/stuck", `{}`)
	_, re := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/realized-state/realized-entities?intent_path=/infra/segments/bad", "")
	if realization("/infra/segments/bad") != "ERROR" || !strings.Contains(fmt.Sprint(re), "Transport node unreachable") {
		t.Fatalf("realization failure: %v", re)
	}
	time.Sleep(100 * time.Millisecond)
	if realization("/infra/segments/stuck") != "IN_PROGRESS" {
		t.Fatal("hung realization must stay IN_PROGRESS (FR-NSX-0506)")
	}
	if code, _ := n(http.MethodPut, "/infra/segments/stuck", `{"_revision":0}`); code != http.StatusPreconditionFailed {
		t.Fatalf("forced revision conflict: %d", code)
	}
	// 재수정하면(규칙 소진) 정상 실현으로 회복
	n(http.MethodPatch, "/infra/segments/stuck", `{"description":"retry"}`)
	if realization("/infra/segments/stuck") != "SUCCESS" {
		t.Fatal("re-modification after hang must realize")
	}
}

// 전송 계층: HTTP Timeout·Connection Reset·간헐적 오류(확률 + Seed 재현).
// 추적: T-FR-FLT-0118 T-FR-FLT-0119 T-FR-FLT-0124 T-FR-FLT-0202
func TestTransportTimeoutResetIntermittent(t *testing.T) {
	e := testenv.Start(t, nil)
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"match":{"product":"nsx","path_prefix":"/api/v1/node/version"},"action":{"type":"timeout","delay_ms":3000},"count":1}`)
	tr := e.Transport()
	tr.TLSClientConfig = &tls.Config{RootCAs: e.Sim.Certs.CAPool}
	short := &http.Client{Transport: tr, Timeout: 500 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/api/v1/node/version", nil)
	u := e.User("nsx")
	req.SetBasicAuth(u.Name, u.Password)
	if _, err := short.Do(req); err == nil || !strings.Contains(err.Error(), "Timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("HTTP timeout: %v", err)
	}
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"match":{"product":"nsx","path_prefix":"/api/v1/cluster/status"},"action":{"type":"reset"},"count":1}`)
	req2, _ := http.NewRequest(http.MethodGet, "https://nsx."+testenv.Domain+"/api/v1/cluster/status", nil)
	req2.SetBasicAuth(u.Name, u.Password)
	if res, err := e.Client.Do(req2); err == nil {
		res.Body.Close()
		t.Fatalf("connection reset must fail the request: %d", res.StatusCode)
	}
	// 간헐적 오류: 같은 Seed → 같은 실패 순서
	pattern := func() string {
		adminJSON(t, e, http.MethodDelete, "/v1/faults", "")
		adminJSON(t, e, http.MethodPost, "/v1/faults", `{"match":{"product":"nsx","path_prefix":"/policy/api/v1/infra/tier-1s"},"action":{"type":"status","status":502},"probability":0.5,"seed":42}`)
		s := ""
		for i := 0; i < 12; i++ {
			code, _ := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/tier-1s", "")
			s += map[bool]string{true: "x", false: "."}[code == 502]
		}
		return s
	}
	a, b := pattern(), pattern()
	if a != b || !strings.Contains(a, "x") || !strings.Contains(a, ".") {
		t.Fatalf("intermittent pattern must be reproducible and mixed: %s %s", a, b)
	}
}

// Task 완료 지연(FR-BE-0605)과 Virtual Clock 가속(FR-BE-0602): 가상 20초 지연을 x100으로 실제 약 0.2초에 끝낸다.
// 진행 중에는 running, 끝나면 원래 동작의 결과. 안쪽 Task는 숨긴다(recentTask에 한 건).
// 추적: T-FR-BE-0605 T-FR-BE-0602 T-FR-BE-0603 T-FR-BE-0604 T-FR-BE-0608 T-FR-BE-0609 T-TR-SCN-0108 T-FR-NSX-0506
func TestTaskDelayAndVirtualClock(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	_, vms := inventory(t, c)
	vm := vms[3]
	if st, _ := vm.PowerState(ctx); st == types.VirtualMachinePowerStatePoweredOff {
		tk, _ := vm.PowerOn(ctx)
		_ = tk.Wait(ctx)
	}
	adminJSON(t, e, http.MethodPut, "/v1/clock", `{"mode":"test","rate":100}`)
	adminJSON(t, e, http.MethodPost, "/v1/faults", `{"layer":"vcenter","match":{"soap_method":"PowerOffVM_Task","resource":"`+vm.Reference().Value+`"},"action":{"type":"task_delay","delay_ms":20000},"count":1}`)
	start := time.Now()
	tk, err := vm.PowerOff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var info mo.Task
	_ = c.RetrieveOne(ctx, tk.Reference(), []string{"info"}, &info)
	if info.Info.State != types.TaskInfoStateRunning && info.Info.State != types.TaskInfoStateQueued {
		t.Fatalf("delayed task must be in progress: %s", info.Info.State)
	}
	if err := tk.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second || d < 100*time.Millisecond {
		t.Fatalf("virtual 20s at x100 took %v", d)
	}
	if st, _ := vm.PowerState(ctx); st != types.VirtualMachinePowerStatePoweredOff {
		t.Fatal("delayed task must perform the operation")
	}
	var tm mo.TaskManager
	_ = c.RetrieveOne(ctx, *c.ServiceContent.TaskManager, []string{"recentTask"}, &tm)
	var offs int
	for _, r := range tm.RecentTask {
		var x mo.Task
		if c.RetrieveOne(ctx, r, []string{"info"}, &x) == nil && x.Info.Name == "PowerOffVM_Task" && x.Info.Entity != nil && *x.Info.Entity == vm.Reference() && x.Info.QueueTime.After(start.Add(-time.Minute)) {
			offs++
		}
	}
	if offs != 1 {
		t.Fatalf("exactly one visible PowerOff task, got %d", offs)
	}
	// 정지(FR-BE-0603)·특정 시점까지 진행(FR-BE-0604): NSX Realization 지연 60초가 시계 진행으로 끝난다
	adminJSON(t, e, http.MethodPut, "/v1/clock", `{"frozen":true}`)
	adminJSON(t, e, http.MethodPut, "/v1/nsx/realization-delay", `{"ms":60000}`)
	nsxDo(t, e, http.MethodPatch, "/policy/api/v1/infra/segments/clocked", `{}`)
	status := func() string {
		_, o := nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/realized-state/status?intent_path=/infra/segments/clocked", "")
		return o["consolidated_status"].(map[string]any)["consolidated_status"].(string)
	}
	time.Sleep(200 * time.Millisecond)
	if status() != "IN_PROGRESS" {
		t.Fatal("frozen clock must keep realization pending")
	}
	_, st := adminJSON(t, e, http.MethodGet, "/v1/clock", "")
	vnow, _ := time.Parse(time.RFC3339Nano, st["virtual_time"].(string))
	if code, _ := adminJSON(t, e, http.MethodPut, "/v1/clock", `{"advance_to":"`+vnow.Add(61*time.Second).Format(time.RFC3339Nano)+`"}`); code != 200 {
		t.Fatal("advance_to")
	}
	if status() != "SUCCESS" {
		t.Fatal("advancing the clock must complete realization")
	}
	// 통계는 가상 시각 기준(FR-BE-0608): 끝 표본 시각 ≈ 가상 현재(실제보다 61초 이상 뒤)
	counters, _ := performance.NewManager(c.Client).CounterInfoByName(ctx)
	pr, err := methods.QueryPerf(ctx, c.Client, &types.QueryPerf{This: *c.ServiceContent.PerfManager, QuerySpec: []types.PerfQuerySpec{{
		Entity: vm.Reference(), MetricId: []types.PerfMetricId{{CounterId: counters["cpu.usage.average"].Key}}, IntervalId: 20, MaxSample: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	last := pr.Returnval[0].(*types.PerfEntityMetric).SampleInfo[0].Timestamp
	if last.Sub(time.Now()) < 40*time.Second {
		t.Fatalf("perf sample time must follow virtual clock: %v", last)
	}
	if code, _ := adminJSON(t, e, http.MethodPut, "/v1/clock", `{"mode":"normal","rate":2}`); code != 400 {
		t.Fatal("normal mode must reject control (FR-BE-0609)")
	}
}

func methodsCurrentTime(ctx context.Context, c *govmomi.Client) (*time.Time, error) {
	return methods.GetCurrentTime(ctx, c.Client)
}
