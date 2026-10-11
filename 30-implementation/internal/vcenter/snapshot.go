package vcenter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
)

// vCenter 인벤토리 Snapshot·Restore(WP 5.2, FR-DM-0304·0306, TR-SCN-0104·0105, NFR-0205·0206).
//
// 근거(SRC-GOVMOMI v0.56.0): vcsim의 저장·복원 기능은 `govc object.save`(cli/object/save.go)가 쓰는 ObjectContent XML
// 디렉터리와 이를 읽는 simulator.Model.Load다(`vcsim -load`). 저장은 govc object.save와 같은 조회 — ServiceContent의
// 관리 객체, 루트 폴더 컨테이너 뷰의 전체 속성(Task·호스트 하위 관리자 포함), 일부 메서드 응답(FetchDVPorts,
// QueryConfigOption 등) — 를 메모리 내 클라이언트(simulator.Service.RoundTrip)로 수행한다. 저장 코드는 govc의 구조를
// 따른다(Apache-2.0).
//
// vcsim 공백 보완(R-43): ① Windows에서 Model.Load가 저장된 로컬 데이터스토어 경로(C:\…)를 URL로 해석하지 못해
// 실패한다 → 저장본의 vcsim 홈 경로를 지우고(상대 이름), 복원 후 호스트 마운트 경로를 새 위치로 고친다.
// ② object.save는 데이터스토어 파일(.vmx 등)을 저장하지 않는다 → 데이터스토어 디렉터리를 함께 복사한다.
// 제한: vAPI 상태(Tagging, Content Library, 구형 /rest 세션)는 vcsim vAPI 시뮬레이터의 내부 상태라 포함하지 않는다.
// 세션은 모두 무효가 된다(Reset과 같다).

type internalCall struct{}

// snapshotMeta는 인벤토리 밖의 vCenter 엔진 상태다.
type snapshotMeta struct {
	Created    time.Time           `json:"created"`    // appliance install_time
	PBM        map[string][]string `json:"pbm"`        // VM → 스토리지 정책(pbm.go)
	Datastores []string            `json:"datastores"` // 복사한 데이터스토어 디렉터리 이름
	Objects    int                 `json:"objects"`
	Counter    int64               `json:"counter,omitempty"` // vcsim MoRef 일련번호(loadModel)
}

// RunningTasks는 끝나지 않은 Task다. Snapshot은 진행 중 작업이 없을 때만 만든다(NFR-0208 재처리 정책).
func (v *VCenter) RunningTasks() []string {
	m := v.Model()
	var out []string
	for _, obj := range m.Map().AllReference("Task") {
		t, ok := obj.(*simulator.Task)
		if !ok {
			continue
		}
		var st types.TaskInfoState
		var name string
		m.Service.Context.WithLock(t, func() { st, name = t.Info.State, t.Info.DescriptionId })
		if st == types.TaskInfoStateRunning || st == types.TaskInfoStateQueued {
			out = append(out, t.Self.Value+" "+name)
		}
	}
	sort.Strings(out)
	return out
}

// SaveSnapshot은 현재 인벤토리를 dir에 저장한다. 호출자는 교차 락(CrossLock)을 쥐고 있어야 한다.
func (v *VCenter) SaveSnapshot(dir string) error {
	m := v.Model()
	ctx := context.WithValue(context.Background(), internalCall{}, true)
	c, err := vim25.NewClient(ctx, m.Service)
	if err != nil {
		return err
	}
	inv := filepath.Join(dir, "inventory")
	if err := os.MkdirAll(inv, 0o700); err != nil {
		return err
	}
	// 저장본의 vcsim 홈 경로는 쓰기 전에 지운다(경로 정규화, R-43 ①). 파일 쓰기는 병렬(R-45 ②).
	s := &saver{c: c, dir: inv, home: vcsimHome(m)}
	if err := s.run(ctx); err != nil {
		return fmt.Errorf("인벤토리 저장: %w", err)
	}
	meta := snapshotMeta{Created: v.created, PBM: v.pbmExport(), Objects: s.n, Counter: registryCounter(m.Map())}
	if err := exportEvents(m, dir); err != nil { // vcsim 이벤트 기록(Model.Load는 복원하지 않는다, events.go)
		return fmt.Errorf("이벤트 기록 저장: %w", err)
	}
	for _, obj := range m.Map().AllReference("Datastore") {
		ds, ok := obj.(*simulator.Datastore)
		if !ok {
			continue
		}
		url := ds.Info.GetDatastoreInfo().Url
		name := filepath.Base(url)
		if err := os.MkdirAll(filepath.Join(dir, "datastores"), 0o700); err != nil {
			return err
		}
		if err := tarTree(url, filepath.Join(dir, "datastores", name+".tar")); err != nil { // 파일 하나로 묶는다(parallel.go)
			return fmt.Errorf("데이터스토어 %s 복사: %w", name, err)
		}
		meta.Datastores = append(meta.Datastores, name)
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	return os.WriteFile(filepath.Join(dir, "vcenter.json"), b, 0o600)
}

// RestoreSnapshot은 dir의 인벤토리로 모델을 바꾼다. 호출자는 교차 락을 쥐고 있어야 한다.
func (v *VCenter) RestoreSnapshot(dir string) error {
	m, meta, err := v.loadModel(dir)
	if err != nil {
		return err
	}
	v.pbmImport(meta.PBM)
	return v.activate(m, meta.Created)
}

// loadModel은 저장본으로 새 vcsim 모델을 만든다(활성화 전). Snapshot 복원과 Fixture 캐시(fixturecache.go)가 함께 쓴다.
func (v *VCenter) loadModel(dir string) (*simulator.Model, snapshotMeta, error) {
	var meta snapshotMeta
	b, err := os.ReadFile(filepath.Join(dir, "vcenter.json"))
	if err != nil {
		return nil, meta, err
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, meta, err
	}
	warmTree(filepath.Join(dir, "inventory")) // vcsim Load는 파일을 하나씩 읽는다 → 미리 병렬로 읽어 둔다(R-45 ②)
	m := v.fix.Model()
	if err := m.Load(filepath.Join(dir, "inventory")); err != nil {
		return nil, meta, fmt.Errorf("vcsim 인벤토리 적재: %w", err)
	}
	// Load는 저장본의 vcsim.home 설정을 읽은 뒤 새 홈 디렉터리를 같은 키로 하나 더 덧붙인다(OptionManager.model).
	// 앞의 것(저장 당시 값)이 남으면 이 모델로 다시 저장할 때 홈 경로 정리가 틀려 그 Snapshot을 복원할 수 없다 → 마지막 것만 남긴다.
	dedupeHome(m)
	// Load는 MoRef 일련번호를 객체 값 숫자의 최댓값에 맞춘다(AlignCounter). 이 값은 저장 당시와 다르다: vcsim 기본
	// 알람처럼 고정 ID(alarm-384 등)를 가진 객체 때문에 더 크거나, 지워진 Task 때문에 더 작다. 저장 당시 값으로 정확히
	// 되돌려야 이후 새 객체의 MoRef가 캐시 없이 만든 경우와 같다(결정론, FR-DM-0207).
	if meta.Counter > 0 {
		setRegistryCounter(m.Map(), meta.Counter)
	}
	// Model.Load는 VirtualMachineSnapshot을 vcsim 래퍼 없이 적재한다(kinds 목록에 없음) → 래퍼로 바꿔 스냅샷 메서드가 동작하게 한다.
	for _, obj := range m.Map().AllReference("VirtualMachineSnapshot") {
		var snap mo.VirtualMachineSnapshot
		switch x := obj.(type) {
		case *mo.VirtualMachineSnapshot:
			snap = *x
		case mo.VirtualMachineSnapshot:
			snap = x
		default:
			continue
		}
		m.Map().Put(&simulator.VirtualMachineSnapshot{VirtualMachineSnapshot: snap, DataSets: map[string]*simulator.DataSet{}})
	}
	urls := map[string]string{}
	for _, obj := range m.Map().AllReference("Datastore") {
		if ds, ok := obj.(*simulator.Datastore); ok {
			url := ds.Info.GetDatastoreInfo().Url
			urls[filepath.Base(url)] = url
		}
	}
	for _, name := range meta.Datastores {
		dst, ok := urls[name]
		if !ok {
			return nil, meta, fmt.Errorf("데이터스토어 %s가 복원 인벤토리에 없다", name)
		}
		// 묶음(.tar) 또는 이전 형식(디렉터리)
		if tarPath := filepath.Join(dir, "datastores", name+".tar"); fileExists(tarPath) {
			if err := untarTree(tarPath, dst); err != nil {
				return nil, meta, err
			}
		} else if err := copyTree(filepath.Join(dir, "datastores", name), dst); err != nil {
			return nil, meta, err
		}
	}
	for _, obj := range m.Map().AllReference("HostSystem") {
		h, ok := obj.(*simulator.HostSystem)
		if !ok || h.Config == nil || h.Config.FileSystemVolume == nil {
			continue
		}
		for i := range h.Config.FileSystemVolume.MountInfo {
			mi := &h.Config.FileSystemVolume.MountInfo[i].MountInfo
			if u, ok := urls[filepath.Base(mi.Path)]; ok {
				mi.Path = u
			}
		}
	}
	if err := importEvents(m, dir); err != nil {
		return nil, meta, err
	}
	return m, meta, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// vcsimHome은 모델의 vcsim 홈 디렉터리다(여러 개면 마지막 = 실제 디렉터리).
func vcsimHome(m *simulator.Model) string {
	home := ""
	for _, o := range m.Map().OptionManager().Setting {
		if ov := o.GetOptionValue(); ov.Key == "vcsim.home" {
			if s, ok := ov.Value.(string); ok {
				home = s
			}
		}
	}
	return home
}

// dedupeHome은 vcsim.home 설정을 마지막(실제 홈) 하나만 남긴다.
func dedupeHome(m *simulator.Model) {
	opt := m.Map().OptionManager()
	home := vcsimHome(m)
	var out []types.BaseOptionValue
	kept := false
	for i := len(opt.Setting) - 1; i >= 0; i-- {
		o := opt.Setting[i]
		if ov := o.GetOptionValue(); ov.Key == "vcsim.home" {
			if kept || ov.Value != home {
				continue
			}
			kept = true
		}
		out = append(out, o)
	}
	slices.Reverse(out)
	opt.Setting = out
}

// saver는 govc object.save와 같은 방식으로 ObjectContent를 XML로 쓴다.
type saver struct {
	c    *vim25.Client
	dir  string
	home string // vcsim 홈 경로(저장본에서 지운다)
	n    int
	out  chan fileJob
	wg   sync.WaitGroup
	mu   sync.Mutex
	err  error
}

type fileJob struct {
	path string
	data []byte
}

// write는 XML로 인코딩해 쓰기 작업자에게 넘긴다(쓰기는 병렬, 인코딩·번호 부여는 호출 순서대로).
func (s *saver) write(name string, data any) error {
	var buf bytes.Buffer
	e := xml.NewEncoder(&buf)
	e.Indent("", "  ")
	if err := e.Encode(data); err != nil {
		return err
	}
	b := buf.Bytes()
	if s.home != "" {
		b = bytes.ReplaceAll(b, []byte(s.home+"/"), nil)
		b = bytes.ReplaceAll(b, []byte(s.home+`\`), nil)
		b = bytes.ReplaceAll(b, []byte(s.home), []byte("vcsim-home"))
	}
	s.out <- fileJob{filepath.Join(s.dir, name) + ".xml", b}
	return nil
}

func (s *saver) start() {
	s.out = make(chan fileJob, 4*IOWorkers)
	for range IOWorkers {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for j := range s.out {
				if err := os.WriteFile(j.path, j.data, 0o600); err != nil {
					s.mu.Lock()
					if s.err == nil {
						s.err = err
					}
					s.mu.Unlock()
				}
			}
		}()
	}
}

func (s *saver) finish() error {
	close(s.out)
	s.wg.Wait()
	return s.err
}

type savedMethod struct {
	name string
	data any
}

// saveMethods는 객체 속성만으로 다시 만들 수 없는 메서드 응답이다(govc object.save의 saveObjects).
var saveMethods = map[string]func(context.Context, *vim25.Client, types.ManagedObjectReference) ([]savedMethod, error){
	"VmwareDistributedVirtualSwitch": func(ctx context.Context, c *vim25.Client, ref types.ManagedObjectReference) ([]savedMethod, error) {
		res, err := methods.FetchDVPorts(ctx, c, &types.FetchDVPorts{This: ref})
		if err != nil {
			return nil, err
		}
		return []savedMethod{{"FetchDVPorts", res}}, nil
	},
	"EnvironmentBrowser": func(ctx context.Context, c *vim25.Client, ref types.ManagedObjectReference) ([]savedMethod, error) {
		o, err := methods.QueryConfigOption(ctx, c, &types.QueryConfigOption{This: ref})
		if err != nil {
			return nil, err
		}
		t, err := methods.QueryConfigTarget(ctx, c, &types.QueryConfigTarget{This: ref})
		if err != nil {
			return nil, err
		}
		cp, err := methods.QueryTargetCapabilities(ctx, c, &types.QueryTargetCapabilities{This: ref})
		if err != nil {
			return nil, err
		}
		return []savedMethod{{"QueryConfigOption", o}, {"QueryConfigTarget", t}, {"QueryTargetCapabilities", cp}}, nil
	},
	"HostNetworkSystem": func(ctx context.Context, c *vim25.Client, ref types.ManagedObjectReference) ([]savedMethod, error) {
		res, err := methods.QueryNetworkHint(ctx, c, &types.QueryNetworkHint{This: ref})
		if err != nil {
			return nil, err
		}
		return []savedMethod{{"QueryNetworkHint", res}}, nil
	},
	"HostSystem": func(ctx context.Context, c *vim25.Client, ref types.ManagedObjectReference) ([]savedMethod, error) {
		res, err := methods.QueryTpmAttestationReport(ctx, c, &types.QueryTpmAttestationReport{This: ref})
		if err != nil {
			return nil, err
		}
		return []savedMethod{{"QueryTpmAttestationReport", res}}, nil
	},
	"AlarmManager": func(ctx context.Context, c *vim25.Client, ref types.ManagedObjectReference) ([]savedMethod, error) {
		res, err := methods.GetAlarm(ctx, c, &types.GetAlarm{This: ref})
		if err != nil {
			return nil, err
		}
		var content []types.ObjectContent
		if err := property.DefaultCollector(c).Retrieve(ctx, res.Returnval, nil, &content); err != nil {
			return nil, err
		}
		return []savedMethod{{"GetAlarm", res}, {"", content}}, nil
	},
	"LicenseAssignmentManager": func(ctx context.Context, c *vim25.Client, ref types.ManagedObjectReference) ([]savedMethod, error) {
		res, err := methods.QueryAssignedLicenses(ctx, c, &types.QueryAssignedLicenses{This: ref})
		if err != nil {
			return nil, err
		}
		return []savedMethod{{"QueryAssignedLicenses", res}}, nil
	},
}

func (s *saver) save(ctx context.Context, content []types.ObjectContent) error {
	for _, x := range content {
		x.MissingSet = nil
		ref := x.Obj.Encode()
		if err := s.write(fmt.Sprintf("%04d-%s", s.n, ref), x); err != nil {
			return err
		}
		s.n++
		f, ok := saveMethods[x.Obj.Type]
		if !ok {
			continue
		}
		objs, err := f(ctx, s.c, x.Obj)
		if err != nil {
			if fault.Is(err, &types.HostNotConnected{}) || fault.Is(err, &types.NotSupported{}) {
				continue
			}
			return err
		}
		if err := os.MkdirAll(filepath.Join(s.dir, ref), 0o700); err != nil {
			return err
		}
		for _, o := range objs {
			if o.name == "" {
				if err := s.save(ctx, o.data.([]types.ObjectContent)); err != nil {
					return err
				}
				continue
			}
			if err := s.write(filepath.Join(ref, o.name), o.data); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *saver) run(ctx context.Context) error {
	s.start()
	err := s.collect(ctx)
	if ferr := s.finish(); err == nil {
		err = ferr
	}
	return err
}

func (s *saver) collect(ctx context.Context) error {
	c := s.c
	pc := property.DefaultCollector(c)
	var content []types.ObjectContent
	if err := pc.RetrieveOne(ctx, vim25.ServiceInstance, []string{"content"}, &content); err != nil {
		return err
	}
	if err := s.save(ctx, content); err != nil {
		return err
	}
	req := types.RetrievePropertiesEx{This: pc.Reference(), Options: types.RetrieveOptions{MaxObjects: 10}}
	for _, p := range content[0].PropSet {
		sc, ok := p.Val.(types.ServiceContent)
		if !ok {
			continue
		}
		for _, ref := range mo.References(sc) {
			all := types.NewBool(true)
			var path []string
			var sel []types.BaseSelectionSpec
			var props []types.PropertySpec
			switch ref.Type {
			case "LicenseManager": // 라이선스 키는 저장하지 않는다(govc 기본값)
				path, all = []string{"licenseAssignmentManager"}, nil
			case "ServiceManager":
				all = nil
			}
			req.SpecSet = append(req.SpecSet, types.PropertyFilterSpec{
				ObjectSet: []types.ObjectSpec{{Obj: ref, SelectSet: sel}},
				PropSet:   append(props, types.PropertySpec{Type: ref.Type, All: all, PathSet: path}),
			})
		}
	}
	cv, err := view.NewManager(c).CreateContainerView(ctx, c.ServiceContent.RootFolder, nil, true)
	if err != nil {
		return err
	}
	defer func() { _ = cv.Destroy(ctx) }()
	ts := func(name, typ, path string) *types.TraversalSpec {
		return &types.TraversalSpec{SelectionSpec: types.SelectionSpec{Name: name}, Type: typ, Path: path}
	}
	sel := func(names ...string) []types.BaseSelectionSpec {
		var out []types.BaseSelectionSpec
		for _, n := range names {
			out = append(out, &types.SelectionSpec{Name: n})
		}
		return out
	}
	all := types.NewBool(true)
	names := []string{"computeTraversalSpec", "datastoreTraversalSpec", "hostDatastoreSystemTraversalSpec", "hostNetworkSystemTraversalSpec",
		"hostVirtualNicManagerTraversalSpec", "hostCertificateManagerTraversalSpec", "entityTraversalSpec", "vmSnapshotTraversalSpec"}
	// govc object.save는 VM 스냅샷 객체(ManagedEntity가 아님)를 저장하지 않는다 → 스냅샷 트리를 따라가 함께 저장한다(R-43).
	vmSnap := ts("vmSnapshotTraversalSpec", "VirtualMachine", "rootSnapshot")
	vmSnap.SelectSet = sel("childSnapshotTraversalSpec")
	childSnap := ts("childSnapshotTraversalSpec", "VirtualMachineSnapshot", "childSnapshot")
	childSnap.SelectSet = sel("childSnapshotTraversalSpec")
	req.SpecSet = append(req.SpecSet, types.PropertyFilterSpec{
		ObjectSet: []types.ObjectSpec{{
			Obj:  cv.Reference(),
			Skip: types.NewBool(false),
			SelectSet: []types.BaseSelectionSpec{
				&types.TraversalSpec{Type: cv.Reference().Type, Path: "view", SelectSet: sel(names...)},
				ts("computeTraversalSpec", "ComputeResource", "environmentBrowser"),
				ts("datastoreTraversalSpec", "Datastore", "browser"),
				ts("hostNetworkSystemTraversalSpec", "HostSystem", "configManager.networkSystem"),
				ts("hostVirtualNicManagerTraversalSpec", "HostSystem", "configManager.virtualNicManager"),
				ts("hostCertificateManagerTraversalSpec", "HostSystem", "configManager.certificateManager"),
				ts("hostDatastoreSystemTraversalSpec", "HostSystem", "configManager.datastoreSystem"),
				ts("entityTraversalSpec", "ManagedEntity", "recentTask"),
				vmSnap, childSnap,
			},
		}},
		PropSet: []types.PropertySpec{
			{Type: "EnvironmentBrowser", All: all}, {Type: "HostDatastoreBrowser", All: all}, {Type: "HostDatastoreSystem", All: all},
			{Type: "HostNetworkSystem", All: all}, {Type: "HostVirtualNicManager", All: all}, {Type: "HostCertificateManager", All: all},
			{Type: "ManagedEntity", All: all}, {Type: "Task", All: all}, {Type: "VirtualMachineSnapshot", All: all},
		},
	})
	res, err := methods.RetrievePropertiesEx(ctx, c, &req)
	if err != nil {
		return err
	}
	if err := s.save(ctx, res.Returnval.Objects); err != nil {
		return err
	}
	for token := res.Returnval.Token; token != ""; {
		cres, err := methods.ContinueRetrievePropertiesEx(ctx, c, &types.ContinueRetrievePropertiesEx{This: req.This, Token: token})
		if err != nil {
			return err
		}
		token = cres.Returnval.Token
		if err := s.save(ctx, cres.Returnval.Objects); err != nil {
			return err
		}
	}
	return nil
}

// Consistency는 vcsim 인벤토리의 양방향 참조 불일치를 찾는다(NFR-0207, FR-DM-0209): VM이 가리키는 호스트·풀·
// 데이터스토어·네트워크·부모 폴더가 있고 그 객체도 VM을 가리키는지, 폴더 자식이 있는지, 정책 연결 VM이 있는지.
func (v *VCenter) Consistency() []string {
	m := v.Model()
	ctx := m.Service.Context
	reg := m.Map()
	var out []string
	exists := func(ref types.ManagedObjectReference) bool { return reg.Get(ref) != nil }
	has := func(list []types.ManagedObjectReference, ref types.ManagedObjectReference) bool {
		for _, r := range list {
			if r == ref {
				return true
			}
		}
		return false
	}
	for _, obj := range reg.AllReference("VirtualMachine") {
		vm, ok := obj.(*simulator.VirtualMachine)
		if !ok {
			continue
		}
		var host, pool, parent *types.ManagedObjectReference
		var dss, nets []types.ManagedObjectReference
		ctx.WithLock(vm, func() {
			host, pool, parent = vm.Runtime.Host, vm.ResourcePool, vm.Parent
			dss, nets = append(dss, vm.Datastore...), append(nets, vm.Network...)
		})
		id := vm.Self.Value
		if host == nil || !exists(*host) {
			out = append(out, "vcenter "+id+": runtime.host missing")
		} else if h, ok := reg.Get(*host).(*simulator.HostSystem); ok && !has(h.Vm, vm.Self) {
			out = append(out, "vcenter "+id+": not listed in "+host.Value+".vm")
		}
		if pool != nil {
			// 리소스 풀 또는 vApp(vApp도 리소스 풀이다 — Fixture vapps, placement.go)
			var members []types.ManagedObjectReference
			found := true
			switch p := reg.Get(*pool).(type) {
			case *simulator.ResourcePool:
				members = p.Vm
			case *simulator.VirtualApp:
				members = p.Vm
			default:
				found = false
			}
			if !found {
				out = append(out, "vcenter "+id+": resourcePool missing")
			} else if !has(members, vm.Self) {
				out = append(out, "vcenter "+id+": not listed in "+pool.Value+".vm")
			}
		}
		if parent == nil || !exists(*parent) {
			out = append(out, "vcenter "+id+": parent folder missing")
		} else if f, ok := reg.Get(*parent).(*simulator.Folder); ok && !has(f.ChildEntity, vm.Self) {
			out = append(out, "vcenter "+id+": not a child of "+parent.Value)
		}
		for _, ds := range dss {
			if d, ok := reg.Get(ds).(*simulator.Datastore); !ok {
				out = append(out, "vcenter "+id+": datastore "+ds.Value+" missing")
			} else if !has(d.Vm, vm.Self) {
				out = append(out, "vcenter "+id+": not listed in "+ds.Value+".vm")
			}
		}
		for _, n := range nets {
			if !exists(n) {
				out = append(out, "vcenter "+id+": network "+n.Value+" missing")
			}
		}
	}
	for _, obj := range reg.AllReference("Folder") {
		if f, ok := obj.(*simulator.Folder); ok {
			for _, c := range f.ChildEntity {
				if !exists(c) {
					out = append(out, "vcenter "+f.Self.Value+": child "+c.Value+" missing")
				}
			}
		}
	}
	for key := range v.pbmExport() { // VM 또는 "VM:디스크 키"(디스크 단위 정책)
		vm, _, _ := strings.Cut(key, ":")
		if !exists(types.ManagedObjectReference{Type: "VirtualMachine", Value: vm}) {
			out = append(out, "vcenter storage policy association for missing VM "+key)
		}
	}
	sort.Strings(out)
	return out
}
