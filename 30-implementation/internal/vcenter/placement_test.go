package vcenter

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
)

// richModel은 독립 호스트·하위 풀·vApp·데이터센터 폴더·스토리지 Pod를 모두 쓰는 vcsim 모델이다.
func richModel() *simulator.Model {
	m := simulator.VPX()
	m.Datacenter, m.Cluster, m.ClusterHost = 2, 2, 3
	m.Host, m.Pool, m.App = 2, 2, 1
	m.Machine, m.Portgroup, m.Datastore = 3, 1, 2
	m.Folder = 1 // 두 번째 데이터센터(DC1)는 폴더 F0 아래, 자원도 하위 폴더 F0에
	m.Pod = 2    // 데이터센터마다 스토리지 Pod 2개(구조물 — VM 배치·MoRef 일련번호가 그대로인지 함께 확인)
	return m
}

type placedVM struct {
	ref, pool, vapp, parent, host string
	ds, net                       []string
}

func refs(l []types.ManagedObjectReference) []string {
	var out []string
	for _, r := range l {
		out = append(out, r.Value)
	}
	return out
}

func val(r *types.ManagedObjectReference) string {
	if r == nil {
		return ""
	}
	return r.Value
}

func placedVMs(m *simulator.Model) map[string]placedVM {
	out := map[string]placedVM{}
	for _, o := range m.Map().AllReference("VirtualMachine") {
		vm := o.(*simulator.VirtualMachine)
		// 데이터스토어는 MoRef가 아니라 데이터센터를 포함한 이름(DCn-LocalDS_k)으로 비교한다: vcsim은 로컬 데이터스토어를
		// 데이터센터 맵을 순회하며 만들어(Go 맵 순서 무작위) 데이터센터가 2개 이상이면 어느 쪽이 먼저 번호를 받는지가
		// vcsim 자체에서도 실행마다 다르다(VM 배치와 무관한 별도 비결정성, DEVELOPMENT_PLAN §13.6).
		var ds []string
		for _, r := range vm.Datastore {
			if d, ok := m.Map().Get(r).(*simulator.Datastore); ok {
				ds = append(ds, filepath.Base(d.Info.GetDatastoreInfo().Url))
			}
		}
		out[vm.Name] = placedVM{ref: vm.Self.Value, pool: val(vm.ResourcePool), vapp: val(vm.ParentVApp), parent: val(vm.Parent),
			host: val(vm.Runtime.Host), ds: ds, net: refs(vm.Network)}
	}
	return out
}

func moNum(v string) int {
	n, _ := strconv.Atoi(v[strings.LastIndex(v, "-")+1:])
	return n
}

// T-FR-DM-0207: placement.go가 만든 VM은 vcsim이 직접 만든 VM과 MoRef·이름·풀·vApp·폴더·데이터스토어·네트워크·
// 이후 일련번호가 같고, 호스트만 결정론적이다(독립 호스트는 그 호스트, 클러스터는 생성 순서 라운드 로빈).
func TestPlacementMatchesVcsim(t *testing.T) {
	ref := richModel()
	if err := ref.Create(); err != nil {
		t.Fatal(err)
	}
	defer ref.Remove()
	want := placedVMs(ref)

	ours := richModel()
	fc := deferContent(ours)
	if err := ours.Create(); err != nil {
		t.Fatal(err)
	}
	defer ours.Remove()
	if err := fc.create(ours); err != nil {
		t.Fatal(err)
	}
	got := placedVMs(ours)

	// VM 수: 데이터센터 2 × (독립 호스트 2 + 클러스터 2 × (하위 풀 2 + vApp 1)) × 3 = 48
	if len(want) != 48 || len(got) != len(want) {
		t.Fatalf("VM 수 vcsim %d, placement %d, 기대 48", len(want), len(got))
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Fatalf("%s가 없다", name)
		}
		if g.ref != w.ref || g.pool != w.pool || g.vapp != w.vapp || g.parent != w.parent ||
			fmt.Sprint(g.ds) != fmt.Sprint(w.ds) || fmt.Sprint(g.net) != fmt.Sprint(w.net) {
			t.Errorf("%s: placement %+v, vcsim %+v", name, g, w)
		}
		if strings.Contains(name, "_H") && !strings.Contains(name, "_C") && g.host != w.host {
			t.Errorf("%s(독립 호스트): host %s, vcsim %s", name, g.host, w.host)
		}
	}
	for _, mdl := range []*simulator.Model{ref, ours} {
		if n := len(mdl.Map().AllReference("StoragePod")); n != 4 { // 데이터센터 2 × Pod 2
			t.Fatalf("StoragePod %d개, 기대 4", n)
		}
	}
	if a, b := registryCounter(ours.Map()), registryCounter(ref.Map()); a != b || a == 0 {
		t.Errorf("MoRef 일련번호 placement %d, vcsim %d", a, b)
	}

	// 클러스터 라운드 로빈: 클러스터마다 MoRef(생성) 순서의 k번째 VM → 호스트 목록 k mod n번째
	for _, o := range ours.Map().AllReference("ClusterComputeResource") {
		cl := o.(*simulator.ClusterComputeResource)
		var names []string
		for name, g := range got {
			if strings.HasPrefix(name, cl.Name+"_") {
				names = append(names, name)
				_ = g
			}
		}
		sort.Slice(names, func(i, j int) bool { return moNum(got[names[i]].ref) < moNum(got[names[j]].ref) })
		if len(names) != 9 { // (하위 풀 2 + vApp 1) × 3
			t.Fatalf("%s VM %d개, 기대 9", cl.Name, len(names))
		}
		for k, name := range names {
			if want := cl.Host[k%len(cl.Host)].Value; got[name].host != want {
				t.Errorf("%s(%d번째): host %s, want %s", name, k, got[name].host, want)
			}
		}
		// 하위 풀 VM은 하위 풀에, vApp VM은 vApp에(루트 풀 아님)
		for _, name := range names {
			g := got[name]
			// vApp VM은 vApp을 리소스 풀로 가진다(vcsim CreateVM은 parentVApp을 채우지 않는다 — vcsim과 같음, 위에서 비교)
			_, inVApp := ours.Map().Get(types.ManagedObjectReference{Type: "VirtualApp", Value: g.pool}).(*simulator.VirtualApp)
			if strings.Contains(name, "_APP") != inVApp {
				t.Errorf("%s: pool %s(vApp %v)", name, g.pool, inVApp)
			}
			if p, ok := ours.Map().Get(types.ManagedObjectReference{Type: "ResourcePool", Value: g.pool}).(*simulator.ResourcePool); ok && p.Self == *cl.ResourcePool {
				t.Errorf("%s: 루트 풀에 있다(하위 풀·vApp 기대)", name)
			}
		}
	}
	// 폴더 안 데이터센터(DC1)의 VM은 VM 폴더의 하위 폴더 F0에 있다(vcsim과 같음은 위 parent 비교로 확인)
	for name, g := range got {
		if strings.HasPrefix(name, "DC1_") {
			f, ok := ours.Map().Get(types.ManagedObjectReference{Type: "Folder", Value: g.parent}).(*simulator.Folder)
			if !ok || f.Name != "F0" {
				t.Fatalf("%s: parent %s(%v), 하위 폴더 F0 기대", name, g.parent, ok)
			}
		}
	}
	// 같은 입력으로 다시 만들면 배치가 같다
	again := richModel()
	fc2 := deferContent(again)
	if err := again.Create(); err != nil {
		t.Fatal(err)
	}
	defer again.Remove()
	if err := fc2.create(again); err != nil {
		t.Fatal(err)
	}
	// 데이터스토어 MoRef도 실행마다 같다(vcsim은 데이터센터 맵 순서라 달라질 수 있었다)
	dsRefs := func(m *simulator.Model) map[string]string {
		out := map[string]string{}
		for _, o := range m.Map().AllReference("Datastore") {
			d := o.(*simulator.Datastore)
			out[filepath.Base(d.Info.GetDatastoreInfo().Url)] = d.Self.Value
		}
		return out
	}
	if a, b := dsRefs(ours), dsRefs(again); fmt.Sprint(a) != fmt.Sprint(b) || len(a) != 4 {
		t.Errorf("데이터스토어 MoRef가 실행마다 다르다: %v / %v", a, b)
	}
	for name, g := range placedVMs(again) {
		if got[name].host != g.host {
			t.Errorf("%s: 두 번째 생성 host %s, 첫 번째 %s", name, g.host, got[name].host)
		}
	}
}
