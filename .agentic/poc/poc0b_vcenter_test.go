package poc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/simulator"
	_ "github.com/vmware/govmomi/vapi/simulator" // vAPI(REST) 세션·태깅 등 엔드포인트 등록
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// restVMView는 vcsim에 없는 /api/vcenter/vm 목록·단건 조회를 vcsim 인벤토리 뷰로 제공한다(FR-VC-0205, FR-DM-0204).
// 응답 필드는 vSphere Automation API(VM.Summary/Info)의 일부만 PoC로 구현한다.
func restVMView(m *simulator.Model, next http.Handler) http.Handler {
	powerState := map[types.VirtualMachinePowerState]string{
		types.VirtualMachinePowerStatePoweredOn:  "POWERED_ON",
		types.VirtualMachinePowerStatePoweredOff: "POWERED_OFF",
		types.VirtualMachinePowerStateSuspended:  "SUSPENDED",
	}
	summary := func(vm *simulator.VirtualMachine) map[string]any {
		var out map[string]any
		m.Map().WithLock(m.Service.Context, vm, func() {
			out = map[string]any{
				"vm":              vm.Self.Value, // REST ID = SOAP MoRef 값
				"name":            vm.Name,
				"power_state":     powerState[vm.Runtime.PowerState],
				"cpu_count":       vm.Config.Hardware.NumCPU,
				"memory_size_MiB": vm.Config.Hardware.MemoryMB,
			}
		})
		return out
	}
	const legacySession = "/rest/com/vmware/cis/session"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		// vcsim은 구형 /rest 세션만 제공한다. /api/session(7.0 U2+)은 같은 세션 저장소로 연결하고
		// 구형 응답 래핑({"value": ...})을 벗겨 신형 계약(201 + 토큰 문자열)으로 바꾼다(FR-VC-0204).
		if p == "/api/session" && r.Method == http.MethodPost {
			rec := httptest.NewRecorder()
			req := r.Clone(r.Context())
			req.URL.Path = legacySession
			next.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				w.WriteHeader(rec.Code)
				return
			}
			var wrapped struct {
				Value string `json:"value"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &wrapped)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(wrapped.Value)
			return
		}
		if p != "/api/vcenter/vm" && !strings.HasPrefix(p, "/api/vcenter/vm/") {
			next.ServeHTTP(w, r)
			return
		}
		// 세션 검증은 vcsim vAPI 세션 저장소에 위임한다.
		chk := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, legacySession+"?~action=get", nil)
		req.Header.Set("vmware-api-session-id", r.Header.Get("vmware-api-session-id"))
		next.ServeHTTP(chk, req)
		if chk.Code != http.StatusOK {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if p == "/api/vcenter/vm" {
			var list []map[string]any
			for _, o := range m.Map().AllReference("VirtualMachine") {
				list = append(list, summary(o.(*simulator.VirtualMachine)))
			}
			_ = json.NewEncoder(w).Encode(list)
			return
		}
		id := strings.TrimPrefix(p, "/api/vcenter/vm/")
		o := m.Map().Get(types.ManagedObjectReference{Type: "VirtualMachine", Value: id})
		if o == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error_type": "NOT_FOUND"})
			return
		}
		_ = json.NewEncoder(w).Encode(summary(o.(*simulator.VirtualMachine)))
	})
}

// PoC-01·02·03: SOAP 로그인, REST 세션, 같은 VM의 SOAP/REST 조회 일관성(R-04).
func TestPoC01_02_03_SessionsAndConsistency(t *testing.T) {
	m := defaultSmall()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	m.Service.RegisterEndpoints = true
	s := m.Service.NewServer()
	t.Cleanup(func() { s.Close(); m.Remove() })
	g := startGateway(t, "127.0.0.1", map[string]http.Handler{"vcenter": restVMView(m, m.Service.ServeMux)})
	defer g.close()
	ctx := context.Background()

	// PoC-01
	c := login(t, g)
	vm, err := find.NewFinder(c.Client, true).VirtualMachine(ctx, "/*/vm/*_VM0")
	if err != nil {
		t.Fatal(err)
	}

	// PoC-02: POST /api/session (Basic) → 세션 토큰
	hc := &http.Client{Transport: g.transport()}
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+simDomain+"/api/session", nil)
	req.SetBasicAuth(simulator.DefaultLogin.Username(), mustPassword())
	res, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var token string
	_ = json.NewDecoder(res.Body).Decode(&token)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK || token == "" {
		t.Fatalf("REST session: %d %q", res.StatusCode, token)
	}
	t.Logf("PoC-02 REST session: status %d, token len %d", res.StatusCode, len(token))

	get := func(path string) (int, []byte) {
		req, _ := http.NewRequest(http.MethodGet, "https://vcenter."+simDomain+path, nil)
		req.Header.Set("vmware-api-session-id", token)
		res, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, b
	}
	if code, _ := (func() (int, []byte) { // 세션 없는 요청은 거부
		req, _ := http.NewRequest(http.MethodGet, "https://vcenter."+simDomain+"/api/vcenter/vm", nil)
		res, _ := hc.Do(req)
		res.Body.Close()
		return res.StatusCode, nil
	})(); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated REST call: %d", code)
	}

	// PoC-03: SOAP으로 전원을 끈 뒤 REST에서 같은 ID·상태가 보이는가
	tk, err := vm.PowerOff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tk.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	code, b := get("/api/vcenter/vm/" + vm.Reference().Value)
	t.Logf("PoC-03 REST GET /api/vcenter/vm/%s → %d %s", vm.Reference().Value, code, strings.TrimSpace(string(b)))
	var info map[string]any
	_ = json.Unmarshal(b, &info)
	if code != http.StatusOK || info["vm"] != vm.Reference().Value || info["power_state"] != "POWERED_OFF" {
		t.Fatalf("SOAP/REST mismatch: %v", info)
	}
	var mvm mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"runtime.powerState"}, &mvm); err != nil {
		t.Fatal(err)
	}
	if mvm.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOff {
		t.Fatalf("SOAP state %s", mvm.Runtime.PowerState)
	}
	code, b = get("/api/vcenter/vm")
	var list []map[string]any
	_ = json.Unmarshal(b, &list)
	t.Logf("PoC-03 REST list → %d, %d VMs (SOAP sees the same inventory)", code, len(list))
}

func mustPassword() string {
	p, _ := simulator.DefaultLogin.Password()
	return p
}

// PoC-04: VM 생성과 Task 생명주기(MoRef, 상태, 시각, 결과, recentTask).
// PoC-05: PropertyCollector 변경 감시(WaitForUpdatesEx, 버전 토큰, Timeout).
func TestPoC04_05_TaskAndPropertyCollector(t *testing.T) {
	_, vc := newVCenterHandler(t, defaultSmall())
	g := startGateway(t, "127.0.0.1", map[string]http.Handler{"vcenter": vc})
	defer g.close()
	c := login(t, g)
	ctx := context.Background()
	finder := find.NewFinder(c.Client, true)
	dc, _ := finder.DefaultDatacenter(ctx)
	finder.SetDatacenter(dc)
	folders, _ := dc.Folders(ctx)
	pool, _ := finder.DefaultResourcePool(ctx)

	// PoC-04
	tk, err := folders.VmFolder.CreateVM(ctx, types.VirtualMachineConfigSpec{
		Name: "poc04", GuestId: string(types.VirtualMachineGuestOsIdentifierOtherGuest), NumCPUs: 2, MemoryMB: 2048,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[LocalDS_0]"},
	}, pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PoC-04 task MoRef: %s", tk.Reference())
	ti, err := tk.WaitForResult(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	vmRef := ti.Result.(types.ManagedObjectReference)
	t.Logf("PoC-04 task: state=%s progress=%v start=%v complete=%v result=%s descriptionId=%s",
		ti.State, ti.Progress, ti.StartTime != nil, ti.CompleteTime != nil, vmRef, ti.DescriptionId)
	if ti.State != types.TaskInfoStateSuccess || ti.StartTime == nil || ti.CompleteTime == nil || vmRef.Type != "VirtualMachine" {
		t.Fatal("task contract incomplete")
	}
	var tm mo.TaskManager
	_ = property.DefaultCollector(c.Client).RetrieveOne(ctx, *c.ServiceContent.TaskManager, []string{"recentTask"}, &tm)
	t.Logf("PoC-04 TaskManager.recentTask count=%d", len(tm.RecentTask))

	// PoC-05: 필터 생성 → 버전 토큰으로 초기 상태 수신 → 전원 켜기 → 변경 수신 → 변경 없을 때 Timeout
	pc := property.DefaultCollector(c.Client)
	filter := new(property.WaitFilter).Add(vmRef, "VirtualMachine", []string{"runtime.powerState"})
	ver := ""
	wait := func(maxWait int32) *types.UpdateSet {
		res, err := methods.WaitForUpdatesEx(ctx, c.Client, &types.WaitForUpdatesEx{
			This: pc.Reference(), Version: ver, Options: &types.WaitOptions{MaxWaitSeconds: &maxWait},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Returnval != nil {
			ver = res.Returnval.Version
		}
		return res.Returnval
	}
	pcFilter, err := pc.CreateFilter(ctx, filter.CreateFilter)
	if err != nil {
		t.Fatal(err)
	}
	_ = pcFilter
	first := wait(5)
	t.Logf("PoC-05 initial update: version=%q objects=%d", ver, len(first.FilterSet[0].ObjectSet))
	start := time.Now()
	if us := wait(1); us != nil {
		t.Fatalf("expected timeout with no changes, got %+v", us)
	}
	t.Logf("PoC-05 no-change wait returned nil after %v (MaxWaitSeconds=1)", time.Since(start).Round(100*time.Millisecond))

	go func() {
		time.Sleep(300 * time.Millisecond)
		vm := object.NewVirtualMachine(c.Client, vmRef)
		ptk, _ := vm.PowerOn(ctx)
		_ = ptk.Wait(ctx)
	}()
	us := wait(10)
	if us == nil {
		t.Fatal("no update after power on")
	}
	ch := us.FilterSet[0].ObjectSet[0].ChangeSet
	t.Logf("PoC-05 change: version=%q %s=%v", ver, ch[0].Name, ch[0].Val)
	if ch[0].Val != types.VirtualMachinePowerStatePoweredOn {
		t.Fatalf("unexpected change %+v", ch)
	}
}
