package vcenter

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/esx"
	"github.com/vmware/govmomi/units"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// 결정론적 VM 호스트 배치(FR-DM-0207, 2026-10-11): vcsim Model.Create는 리소스 풀·vApp의 VM을 호스트 지정 없이
// 만들고, createVM이 클러스터 호스트 중 하나를 시드 없는 rand.Intn으로 고른다. 그래서 같은 Fixture라도 기동마다
// VM의 runtime.host·host.vm·생성 이벤트의 호스트가 달랐다(Fixture 캐시 작업 중 발견, DEVELOPMENT_PLAN §13.5).
//
// 모델은 Machine=0으로 만들고(인프라·풀·vApp만), VM은 여기서 vcsim CreateInfrastructure의 addMachine과 같은 요청·
// 순서로 만들되 호스트를 명시한다. 요청과 순서가 같으므로 MoRef 번호·이름·장치·파일·이벤트 순서는 vcsim이 만든 것과
// 같고 배치만 고정된다. 근거(SRC-GOVMOMI v0.56.0): simulator/model.go CreateInfrastructure·addMachine(Apache-2.0).
//
// 순서(데이터센터마다): ① 독립 호스트 DCn_Hk의 VM(그 호스트, 원래도 결정론적) ② 클러스터마다 루트 풀(하위 풀이 없을 때)
// 또는 하위 풀 RP1..N의 VM ③ 그 클러스터의 vApp APP0..의 VM. 배치 규칙: 클러스터 안에서 만드는 순서대로(풀·vApp을
// 가로질러) 호스트 목록(추가 순서)을 라운드 로빈 — k번째 VM → k mod n번째 호스트.
// 데이터센터 폴더(Model.Folder = k): 마지막 k개 데이터센터는 루트 아래 폴더 F<i>에 들어가고, 그 데이터센터의 호스트·
// 네트워크·VM(·데이터스토어) 자원도 각 기본 폴더 아래 같은 이름의 하위 폴더 F<i>에 만든다(vcsim 규칙) → 같은 경로로 찾는다.

// 결정론적 데이터스토어(2026-10-11): vcsim은 로컬 데이터스토어를 데이터센터 맵(hostMap)을 순회하며 만들어(Go 맵 순서
// 무작위) 데이터센터가 2개 이상이면 어느 데이터센터의 LocalDS_k가 먼저 MoRef 번호를 받는지가 실행마다 달랐다.
// 모델은 Datastore=0으로 만들고 여기서 데이터센터 순서(DC0, DC1, …)로 같은 요청(vcsim createLocalDatastore: 디렉터리
// <vcsim.home>/DCn-LocalDS_k, 데이터센터의 모든 호스트에 CreateLocalDatastore — 호스트는 추가 순서)을 보낸다.

// fixtureContent는 Model.Create에서 떼어 내 결정론적으로 만드는 Fixture 내용(VM·로컬 데이터스토어)이다.
type fixtureContent struct{ perPool, datastores int }

// deferContent는 m에서 VM·데이터스토어 생성을 떼어 낸다(Machine·Datastore = 0). Create 뒤 create로 만든다.
func deferContent(m *simulator.Model) fixtureContent {
	fc := fixtureContent{perPool: m.Machine, datastores: m.Datastore}
	m.Machine, m.Datastore = 0, 0
	return fc
}

// create는 떼어 낸 데이터스토어·VM을 vcsim과 같은 순서(데이터스토어 전부 → VM)로 만든다.
func (fc fixtureContent) create(m *simulator.Model) error {
	ctx := context.Background()
	c, err := vim25.NewClient(ctx, m.Service)
	if err != nil {
		return err
	}
	dcs, err := fixtureDatacenters(m)
	if err != nil {
		return err
	}
	if err := createDatastores(ctx, c, m, dcs, fc.datastores); err != nil {
		return err
	}
	return createFixtureVMs(ctx, c, m, dcs, fc.perPool)
}

// fixtureDC는 데이터센터와 그 자원 폴더(데이터센터 폴더를 쓰면 하위 폴더 F<i>)다.
type fixtureDC struct {
	name             string
	dc               *simulator.Datacenter
	vmF, hostF, netF types.ManagedObjectReference
}

func byName(reg *simulator.Registry, name string, refs []types.ManagedObjectReference) (types.ManagedObjectReference, error) {
	if e := reg.FindByName(name, refs); e != nil {
		return e.Reference(), nil
	}
	return types.ManagedObjectReference{}, fmt.Errorf("vcsim 인벤토리에 %s가 없다", name)
}

func folderOf(reg *simulator.Registry, ref types.ManagedObjectReference) *simulator.Folder {
	return reg.Get(ref).(*simulator.Folder)
}

// fixtureDatacenters는 데이터센터를 생성 순서로 찾는다(vcsim CreateInfrastructure와 같은 폴더 규칙).
func fixtureDatacenters(m *simulator.Model) ([]fixtureDC, error) {
	reg := m.Map()
	root := folderOf(reg, m.ServiceContent.RootFolder)
	var out []fixtureDC
	nfolder := 0
	for ndc := 0; ndc < m.Datacenter; ndc++ {
		dcName := fmt.Sprintf("DC%d", ndc)
		parent := root
		fName := fmt.Sprintf("F%d", nfolder)
		inFolder := nfolder < m.Folder && ndc >= m.Datacenter-m.Folder // vcsim CreateInfrastructure와 같은 조건
		if inFolder {
			f, err := byName(reg, fName, root.ChildEntity)
			if err != nil {
				return nil, err
			}
			parent = folderOf(reg, f)
		}
		dcRef, err := byName(reg, dcName, parent.ChildEntity)
		if err != nil {
			return nil, err
		}
		d := fixtureDC{name: dcName, dc: reg.Get(dcRef).(*simulator.Datacenter)}
		d.vmF, d.hostF, d.netF = d.dc.VmFolder, d.dc.HostFolder, d.dc.NetworkFolder
		if inFolder {
			for _, sub := range []*types.ManagedObjectReference{&d.vmF, &d.hostF, &d.netF} {
				f, err := byName(reg, fName, folderOf(reg, *sub).ChildEntity)
				if err != nil {
					return nil, fmt.Errorf("%s의 하위 폴더: %w", dcName, err)
				}
				*sub = f
			}
			nfolder++
		}
		out = append(out, d)
	}
	return out, nil
}

// dcHosts는 데이터센터의 호스트를 추가 순서(MoRef 번호 순)로 돌려준다: 독립 호스트와 클러스터 호스트.
func dcHosts(reg *simulator.Registry, d fixtureDC) []types.ManagedObjectReference {
	var hosts []types.ManagedObjectReference
	for _, ref := range folderOf(reg, d.hostF).ChildEntity {
		switch cr := reg.Get(ref).(type) {
		case *mo.ComputeResource:
			hosts = append(hosts, cr.Host...)
		case *simulator.ClusterComputeResource:
			hosts = append(hosts, cr.Host...)
		}
	}
	sort.Slice(hosts, func(i, j int) bool { return refNum(hosts[i].Value) < refNum(hosts[j].Value) })
	return hosts
}

func refNum(v string) int {
	n, _ := strconv.Atoi(v[strings.LastIndex(v, "-")+1:])
	return n
}

// createDatastores는 데이터센터마다 로컬 데이터스토어 LocalDS_0..n-1을 만든다(vcsim createLocalDatastore와 같은 요청).
func createDatastores(ctx context.Context, c *vim25.Client, m *simulator.Model, dcs []fixtureDC, n int) error {
	if n <= 0 {
		return nil
	}
	home := vcsimHome(m)
	if home == "" {
		return fmt.Errorf("vcsim.home 없음")
	}
	reg := m.Map()
	for _, d := range dcs {
		hosts := dcHosts(reg, d)
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("LocalDS_%d", i)
			dir := path.Join(home, d.name+"-"+name) // vcsim createTempDir와 같은 경로 표기
			if err := os.Mkdir(dir, 0o700); err != nil {
				return err
			}
			for _, h := range hosts {
				dss, err := object.NewHostSystem(c, h).ConfigManager().DatastoreSystem(ctx)
				if err != nil {
					return err
				}
				if _, err := dss.CreateLocalDatastore(ctx, name, dir); err != nil {
					return fmt.Errorf("%s %s: %w", d.name, name, err)
				}
			}
		}
	}
	return nil
}

// createFixtureVMs는 Fixture VM을 만든다(perPool = 풀·vApp·독립 호스트마다 VM 수).
func createFixtureVMs(ctx context.Context, c *vim25.Client, m *simulator.Model, dcs []fixtureDC, perPool int) error {
	if perPool <= 0 {
		return nil
	}
	reg := m.Map()
	byName := func(name string, refs []types.ManagedObjectReference) (types.ManagedObjectReference, error) {
		return byName(reg, name, refs)
	}
	folderOf := func(ref types.ManagedObjectReference) *simulator.Folder { return folderOf(reg, ref) }
	for _, d := range dcs {
		dcName, vmF, hostF, netF := d.name, d.vmF, d.hostF, d.netF
		vmFolder := object.NewFolder(c, vmF)
		// VM eth0 백킹: 첫 번째 DVPG(vcsim과 같음). 포트그룹이 없으면 기본 백킹.
		vmnet := esx.EthernetCard.Backing
		if m.Portgroup > 0 {
			net := folderOf(netF)
			pg, err := byName(dcName+"_DVPG0", net.ChildEntity)
			if err != nil {
				return err
			}
			if vmnet, err = object.NewDistributedVirtualPortgroup(c, pg).EthernetCardBackingInfo(ctx); err != nil {
				return err
			}
		}
		hostFolder := folderOf(hostF)
		mk := func(prefix string, pool types.ManagedObjectReference, pick func() types.ManagedObjectReference) error {
			return addMachines(ctx, c, m, prefix, vmFolder, object.NewResourcePool(c, pool), pick, vmnet, perPool)
		}
		// ① 독립 호스트: 그 호스트의 컴퓨트 리소스 풀, 호스트 고정
		for nhost := 0; nhost < m.Host; nhost++ {
			name := fmt.Sprintf("%s_H%d", dcName, nhost)
			crRef, err := byName(name, hostFolder.ChildEntity)
			if err != nil {
				return err
			}
			cr, ok := reg.Get(crRef).(*mo.ComputeResource)
			if !ok || len(cr.Host) == 0 || cr.ResourcePool == nil {
				return fmt.Errorf("독립 호스트 %s의 컴퓨트 리소스가 올바르지 않다", name)
			}
			host := cr.Host[0]
			if err := mk(name, *cr.ResourcePool, func() types.ManagedObjectReference { return host }); err != nil {
				return err
			}
		}
		// ②·③ 클러스터: 루트 풀 또는 하위 풀, 그다음 vApp. 클러스터 안 라운드 로빈.
		for ncl := 0; ncl < m.Cluster; ncl++ {
			clName := fmt.Sprintf("%s_C%d", dcName, ncl)
			clRef, err := byName(clName, hostFolder.ChildEntity)
			if err != nil {
				return err
			}
			cl := reg.Get(clRef).(*simulator.ClusterComputeResource)
			hosts := append([]types.ManagedObjectReference(nil), cl.Host...)
			if len(hosts) == 0 || cl.ResourcePool == nil {
				return fmt.Errorf("클러스터 %s에 호스트·리소스 풀이 없다", clName)
			}
			next := 0
			pick := func() types.ManagedObjectReference {
				h := hosts[next%len(hosts)]
				next++
				return h
			}
			rootRP := reg.Get(*cl.ResourcePool).(*simulator.ResourcePool)
			children := append([]types.ManagedObjectReference(nil), rootRP.ResourcePool.ResourcePool...)
			if m.Pool == 0 {
				if err := mk(clName+"_RP0", rootRP.Self, pick); err != nil {
					return err
				}
			}
			for rp := 1; rp <= m.Pool; rp++ {
				name := fmt.Sprintf("%s_RP%d", clName, rp)
				ref, err := byName(name, children)
				if err != nil {
					return err
				}
				if err := mk(name, ref, pick); err != nil {
					return err
				}
			}
			for napp := 0; napp < m.App; napp++ {
				name := fmt.Sprintf("%s_APP%d", clName, napp)
				ref, err := byName(name, children)
				if err != nil {
					return err
				}
				if err := mk(name, ref, pick); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// addMachines는 vcsim addMachine과 같은 VM n개를 만든다(호스트만 pick으로 명시). NIC 장치 값은 vcsim처럼 같은 풀의
// VM이 함께 쓴다.
func addMachines(ctx context.Context, c *vim25.Client, m *simulator.Model, prefix string, folder *object.Folder,
	pool *object.ResourcePool, pick func() types.ManagedObjectReference, vmnet types.BaseVirtualDeviceBackingInfo, n int) error {
	nic := esx.EthernetCard
	nic.Backing = vmnet
	ds := types.ManagedObjectReference{}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%s_VM%d", prefix, i)
		config := types.VirtualMachineConfigSpec{
			Name:    name,
			GuestId: string(types.VirtualMachineGuestOsIdentifierOtherGuest),
			Files:   &types.VirtualMachineFileInfo{VmPathName: "[LocalDS_0]"},
		}
		var devices object.VirtualDeviceList
		scsi, _ := devices.CreateSCSIController("pvscsi")
		ide, _ := devices.CreateIDEController()
		cdrom, _ := devices.CreateCdrom(ide.(*types.VirtualIDEController))
		disk := devices.CreateDisk(scsi.(types.BaseVirtualController), ds, config.Files.VmPathName+" "+path.Join(name, "disk1.vmdk"))
		disk.CapacityInKB = int64(units.GB*10) / units.KB
		disk.StorageIOAllocation = &types.StorageIOAllocationInfo{Limit: types.NewInt64(-1)}
		devices = append(devices, scsi, cdrom, disk, &nic)
		config.DeviceChange, _ = devices.ConfigSpec(types.VirtualDeviceConfigSpecOperationAdd)

		task, err := folder.CreateVM(ctx, config, pool, object.NewHostSystem(c, pick()))
		if err != nil {
			return err
		}
		info, err := task.WaitForResult(ctx, nil)
		if err != nil {
			return fmt.Errorf("VM %s 생성: %w", name, err)
		}
		if m.Autostart {
			vm := object.NewVirtualMachine(c, info.Result.(types.ManagedObjectReference))
			if t, err := vm.PowerOn(ctx); err == nil {
				_, _ = t.WaitForResult(ctx, nil)
			}
		}
	}
	return nil
}
