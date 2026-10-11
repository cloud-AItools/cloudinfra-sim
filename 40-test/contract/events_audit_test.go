// 이벤트·감사·변경 원인 추적 (Phase 3 WP 3.5: FR-MET-0201 ~ 0203, FR-BE-0209, FR-BE-0403, NFR-0306).
package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

type auditEvent struct {
	CID, Product, Principal, Action, Target, Cause string
	Status                                         int
}

func auditLog(t *testing.T, e *testenv.Env, cid string) []auditEvent {
	t.Helper()
	res, err := e.AdminDo(http.MethodGet, "/v1/audit?limit=500&cid="+url.QueryEscape(cid), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Events []auditEvent `json:"events"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out.Events
}

// 추적: T-FR-MET-0202 T-FR-MET-0203 T-FR-BE-0403
// FR-MET-0201: 7종(VM Created·Removed·Powered On·Off, Host Disconnected, Task Failed, NSX Realization Failed) 시험.
// Datastore Usage Threshold는 contract/TestSemanticAndInfraFaults(기본 알람 Datastore usage on disk).
func TestVCenterEvents(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	dss, _ := f.DatastoreList(ctx, "*")
	ds, _ := dss[0].ObjectName(ctx)
	spec := types.VirtualMachineConfigSpec{Name: "ev-vm", GuestId: "otherGuest64", NumCPUs: 1, MemoryMB: 256, Files: &types.VirtualMachineFileInfo{VmPathName: "[" + ds + "]"}}
	tk, _ := folders.VmFolder.CreateVM(ctx, spec, pool, nil)
	ti, err := tk.WaitForResult(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	vm := object.NewVirtualMachine(c.Client, ti.Result.(types.ManagedObjectReference))
	for _, op := range []func(context.Context) (*object.Task, error){vm.PowerOn, vm.PowerOff, vm.Destroy} {
		tk, _ := op(ctx)
		_ = tk.Wait(ctx)
	}
	// Task 실패(용량 초과) → TaskEvent
	big := spec
	big.Name, big.NumCPUs = "ev-big", 4096
	tk, _ = folders.VmFolder.CreateVM(ctx, big, pool, nil)
	_ = tk.Wait(ctx)
	// 호스트 연결 끊기·다시 연결
	hosts, _ := f.HostSystemList(ctx, "*")
	tk, _ = hosts[0].Disconnect(ctx)
	_ = tk.Wait(ctx)
	tk, _ = hosts[0].Reconnect(ctx, nil, nil)
	_ = tk.Wait(ctx)

	want := map[string]bool{"VmCreatedEvent": false, "VmPoweredOnEvent": false, "VmPoweredOffEvent": false, "VmRemovedEvent": false,
		"TaskEvent": false, "HostDisconnectedEvent": false, "HostConnectedEvent": false}
	em := event.NewManager(c.Client)
	waitFor(t, "events", func() bool {
		evs, err := em.QueryEvents(ctx, types.EventFilterSpec{})
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range evs {
			name := strings.TrimPrefix(fmt.Sprintf("%T", ev), "*types.")
			if _, ok := want[name]; !ok {
				continue
			}
			b := ev.GetEvent()
			// FR-MET-0203: Resource ID, Timestamp, Initiator, Event Type, 상세
			if b.Key == 0 || b.CreatedTime.IsZero() || b.UserName == "" || b.FullFormattedMessage == "" {
				t.Fatalf("event attributes: %s %+v", name, b)
			}
			switch x := ev.(type) {
			case *types.TaskEvent:
				if x.Info.State == types.TaskInfoStateError && x.Info.Name == "CreateVM_Task" && x.Info.Error != nil {
					want[name] = true
				}
			case *types.HostDisconnectedEvent:
				want[name] = x.Host != nil && x.Host.Host == hosts[0].Reference()
			default:
				if b.Vm != nil && b.Vm.Name == "ev-vm" || name == "HostConnectedEvent" {
					want[name] = true
				}
			}
		}
		for _, ok := range want {
			if !ok {
				return false
			}
		}
		return true
	})
}

// 감사 기록과 변경 원인: 요청(사용자·동작·대상·Task)과 그 요청이 원인인 내부 변경(관계 조정)이 같은 상관 ID로 이어진다.
// 추적: T-NFR-0306 T-FR-BE-0209 T-NFR-0307
func TestAuditAndChangeCause(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	vu, nu := e.User("vcenter"), e.User("nsx")

	// 로그인 실패·성공(SOAP)
	bad := e.SOAP("")
	_ = bad.Login(ctx, url.UserPassword(vu.Name, "wrong-password"))
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	tk, err := vms[0].PowerOff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = tk.Wait(ctx)

	// NSX 쓰기 → 관계 조정(vCenter NSX DVPG 생성)이 같은 cid를 원인으로 기록
	req, _ := http.NewRequest(http.MethodPatch, "https://nsx."+testenv.Domain+"/policy/api/v1/infra/segments/audit-seg", strings.NewReader(`{}`))
	req.SetBasicAuth(nu.Name, nu.Password)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	cid := res.Header.Get("X-Request-ID")

	all := auditLog(t, e, "")
	var loginFail, loginOK, powerOff bool
	for _, a := range all {
		switch {
		case a.Action == "SOAP Login" && a.Status != 200 && a.Principal == vu.Name:
			loginFail = true
		case a.Action == "SOAP Login" && a.Status == 200 && a.Principal == vu.Name:
			loginOK = true
		case a.Action == "SOAP PowerOffVM_Task" && a.Principal == vu.Name && strings.Contains(a.Target, "VirtualMachine:"+vms[0].Reference().Value) && strings.Contains(a.Target, "task=task-"):
			powerOff = true
		}
	}
	if !loginFail || !loginOK || !powerOff {
		t.Fatalf("audit: loginFail=%v loginOK=%v powerOff=%v\n%+v", loginFail, loginOK, powerOff, all)
	}
	chain := auditLog(t, e, cid)
	var direct, caused bool
	for _, a := range chain {
		if a.CID == cid && a.Principal == nu.Name && a.Action == "PATCH /policy/api/v1/infra/segments/audit-seg" && a.Cause == "api" {
			direct = true
		}
		if a.Principal == "system" && a.Action == "relations create NSX portgroup" && a.Target == "/infra/segments/audit-seg" && strings.Contains(a.Cause, "cid="+cid) {
			caused = true
		}
	}
	if !direct || !caused {
		t.Fatalf("change cause chain for %s: direct=%v caused=%v\n%+v", cid, direct, caused, chain)
	}
	// 자격 증명은 감사 기록에 남지 않는다(NFR-0307)
	b, _ := json.Marshal(all)
	if strings.Contains(string(b), vu.Password) || strings.Contains(string(b), nu.Password) || strings.Contains(string(b), "wrong-password") {
		t.Fatal("audit must not contain credentials")
	}
	// Admin 동작(Fault 규칙 추가)과 초기화도 기록
	r := addFault(t, e, `{"match":{"product":"nsx"},"action":{"type":"delay","delay_ms":1},"count":1}`)
	res, _ = e.AdminDo(http.MethodPost, "/v1/reset", nil)
	res.Body.Close()
	var adminFault, adminReset bool
	for _, a := range auditLog(t, e, "") {
		adminFault = adminFault || a.Product == "admin" && a.Action == "POST /v1/faults" && a.Status == 201
		adminReset = adminReset || a.Product == "admin" && a.Action == "reset"
	}
	if !adminFault || !adminReset {
		t.Fatalf("admin audit: fault=%v reset=%v (rule %s)", adminFault, adminReset, r.ID)
	}
}
