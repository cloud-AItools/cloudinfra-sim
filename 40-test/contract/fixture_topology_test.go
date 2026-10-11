// Fixture 선택 구성(독립 호스트·하위 리소스 풀·vApp·데이터센터 폴더·스토리지 Pod, 2026-10-11): 시뮬레이터가 이 구성으로 기동하고, 인벤토리가
// 일관되며(불일치 검사·관계 조정 검증 모드), SOAP·REST가 같은 VM을 보고, 배치가 결정론적이고, Fixture 캐시가
// 같은 상태를 적재한다.
package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

// topologyFixture는 기본 Fixture에 독립 호스트 1·하위 풀 2·vApp 1·데이터센터 폴더 1·스토리지 Pod 2를 더한 파일이다
// (VM: (1 + 2 + 1) × 3 = 12, DC0은 폴더 F0 아래, Pod DC0_POD0·1).
func topologyFixture(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(testenv.Repo(), "30-implementation", "data", "fixtures", "default-small.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(src), "name: default-small", "name: topology", 1)
	s = strings.Replace(s, "vms_per_resource_pool: 20", "vms_per_resource_pool: 3\n  standalone_hosts: 1\n  child_pools: 2\n  vapps: 1\n  datacenter_folders: 1\n  storage_pods: 2", 1)
	p := filepath.Join(t.TempDir(), "topology.yaml")
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// podState는 스토리지 Pod 요약(이름·상위 폴더·Storage DRS·들어 있는 데이터스토어 수)이다.
func podState(t *testing.T, e *testenv.Env) []string {
	t.Helper()
	reg := e.Sim.VC.Model().Map()
	var out []string
	for _, o := range reg.AllReference("StoragePod") {
		p := o.(*simulator.StoragePod)
		parent := ""
		if f, ok := reg.Get(*p.Parent).(*simulator.Folder); ok {
			parent = f.Name
		}
		drs := p.PodStorageDrsEntry != nil && p.PodStorageDrsEntry.StorageDrsConfig.PodConfig.Enabled
		out = append(out, fmt.Sprintf("%s parent=%s drs=%v children=%d", p.Name, parent, drs, len(p.ChildEntity)))
	}
	sort.Strings(out)
	return out
}

func restVMIDs(t *testing.T, e *testenv.Env) []string {
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
	token := strings.Trim(strings.TrimSpace(string(b)), `"`)
	req, _ = http.NewRequest(http.MethodGet, "https://vcenter."+testenv.Domain+"/api/vcenter/vm", nil)
	req.Header.Set("vmware-api-session-id", token)
	res, err = e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var vms []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&vms); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, vm := range vms {
		ids = append(ids, vm["vm"].(string))
	}
	return ids
}

// T-FR-DM-0207 T-TR-SCN-0201
func TestFixtureTopology(t *testing.T) {
	t.Setenv("SIM_RELATIONS_VERIFY", "1")
	fx := topologyFixture(t)
	off := testenv.Start(t, func(c *config.Config) { c.FixtureFile = fx; c.FixtureCache = "off" })
	vms := vmList(t, off)
	if len(vms) != 12 {
		t.Fatalf("VM %d개, 기대 12: %v", len(vms), vms)
	}
	consistent(t, off)
	if ids := restVMIDs(t, off); len(ids) != len(vms) {
		t.Fatalf("REST VM %d개, SOAP %d개", len(ids), len(vms))
	}
	names := strings.Join(vms, "\n")
	for _, want := range []string{"DC0_H0_VM0", "DC0_C0_RP1_VM2", "DC0_C0_RP2_VM0", "DC0_C0_APP0_VM1"} {
		if !strings.Contains(names, want) {
			t.Errorf("%s가 없다", want)
		}
	}
	// 데이터센터 폴더: DC0은 루트 아래 F0 안, VM은 VM 폴더의 하위 폴더 F0 안
	reg := off.Sim.VC.Model().Map()
	for _, o := range reg.AllReference("Datacenter") {
		dc := o.(*simulator.Datacenter)
		if p, ok := reg.Get(*dc.Parent).(*simulator.Folder); !ok || p.Name != "F0" {
			t.Errorf("%s의 상위 폴더가 F0이 아니다", dc.Name)
		}
	}
	for _, o := range reg.AllReference("VirtualMachine") {
		vm := o.(*simulator.VirtualMachine)
		if p, ok := reg.Get(*vm.Parent).(*simulator.Folder); !ok || p.Name != "F0" {
			t.Errorf("%s의 폴더가 F0이 아니다", vm.Name)
		}
	}
	// 스토리지 Pod: 데이터센터의 기본 datastore 폴더(하위 폴더 F0이 아님 — vcsim은 하위 폴더를 만들기 전에 Pod를 만든다)에
	// 빈 Pod 2개, Storage DRS 켜짐
	wantPods := []string{"DC0_POD0 parent=datastore drs=true children=0", "DC0_POD1 parent=datastore drs=true children=0"}
	if got := podState(t, off); fmt.Sprint(got) != fmt.Sprint(wantPods) {
		t.Fatalf("pods %v, want %v", got, wantPods)
	}
	// 클라이언트가 데이터스토어를 Pod로 옮길 수 있고(MoveIntoFolder), 인벤토리는 일관된다
	l := snapVC(t, off)
	dss, err := l.f.DatastoreList(l.ctx, "*")
	if err != nil || len(dss) == 0 {
		t.Fatalf("datastores: %v", err)
	}
	var podRef types.ManagedObjectReference
	for _, o := range reg.AllReference("StoragePod") {
		if p := o.(*simulator.StoragePod); p.Name == "DC0_POD0" {
			podRef = p.Self
		}
	}
	task, err := object.NewStoragePod(l.c.Client, podRef).MoveInto(l.ctx, []types.ManagedObjectReference{dss[0].Reference()})
	if err == nil {
		err = task.Wait(l.ctx)
	}
	if err != nil {
		t.Fatalf("데이터스토어를 Pod로 옮기기: %v", err)
	}
	if got := podState(t, off); !strings.Contains(fmt.Sprint(got), "DC0_POD0 parent=datastore drs=true children=1") {
		t.Fatalf("이동 뒤 pods %v", got)
	}
	consistent(t, off)
	if strings.Contains(names, "DC0_C0_RP0_VM") {
		t.Error("하위 풀이 있으면 루트 풀에는 VM을 두지 않는다(vcsim 규칙)")
	}
	// 결정론: 다시 만들어도(Reset) 같다
	if err := off.Sim.Reset(); err != nil {
		t.Fatal(err)
	}
	if again := vmList(t, off); firstDiff(again, vms) != "" {
		t.Fatalf("Reset 뒤 다르다: %s", firstDiff(again, vms))
	}
	if got := podState(t, off); fmt.Sprint(got) != fmt.Sprint(wantPods) {
		t.Fatalf("Reset 뒤 pods %v, want %v(빈 Pod)", got, wantPods)
	}
	// Fixture 캐시: 저장(miss) → Reset 적재(hit)가 캐시 없이 만든 상태와 같다
	e := testenv.Start(t, func(c *config.Config) { c.FixtureFile = fx; c.FixtureCache = "on" })
	if st := e.Sim.VC.LastCache(); st.Mode != "miss" || st.Error != "" {
		t.Fatalf("cache %+v", st)
	}
	if err := e.Sim.Reset(); err != nil {
		t.Fatal(err)
	}
	if st := e.Sim.VC.LastCache(); st.Mode != "hit" {
		t.Fatalf("Reset cache %+v, hit 기대", st)
	}
	consistent(t, e)
	if hit := vmList(t, e); firstDiff(hit, vms) != "" {
		t.Fatalf("캐시 적재 인벤토리가 다르다: %s", firstDiff(hit, vms))
	}
	if got := podState(t, e); fmt.Sprint(got) != fmt.Sprint(wantPods) {
		t.Fatalf("캐시 적재 뒤 pods %v, want %v", got, wantPods)
	}
}
