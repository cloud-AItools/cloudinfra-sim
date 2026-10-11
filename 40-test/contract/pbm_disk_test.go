package contract

import (
	"context"
	"fmt"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-VC-0210: 디스크 단위 스토리지 정책(R-37, 2026-10-11) — 생성·재구성의 deviceChange[].profile을 디스크
// (virtualDiskId "<VM>:<디스크 키>")와 연결하고, 정책 비우기·디스크 제거·VM 삭제 뒤 연결이 사라진다.
func TestPBMDiskPolicy(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	pc, err := pbm.NewClient(ctx, c.Client)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := pc.QueryProfile(ctx, pbmtypes.PbmProfileResourceType{ResourceType: string(pbmtypes.PbmProfileResourceTypeEnumSTORAGE)}, "")
	if err != nil || len(ids) < 2 {
		t.Fatalf("profiles: %d %v", len(ids), err)
	}
	polA, polB := ids[0].UniqueId, ids[1].UniqueId
	profile := func(id string) []types.BaseVirtualMachineProfileSpec {
		return []types.BaseVirtualMachineProfileSpec{&types.VirtualMachineDefinedProfileSpec{ProfileId: id}}
	}
	assoc := func(objType pbmtypes.PbmObjectType, key string) []string {
		t.Helper()
		got, err := pc.QueryAssociatedProfile(ctx, pbmtypes.PbmServerObjectRef{ObjectType: string(objType), Key: key})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range got {
			out = append(out, p.UniqueId)
		}
		return out
	}
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	folders, _ := dc.Folders(ctx)
	pool, _ := f.DefaultResourcePool(ctx)
	dss, _ := f.DatastoreList(ctx, "*")
	dsName, _ := dss[0].ObjectName(ctx)

	// 생성: SCSI 컨트롤러 + 정책 A 디스크
	var devs object.VirtualDeviceList
	scsi, _ := devs.CreateSCSIController("pvscsi")
	devs = append(devs, scsi)
	disk := devs.CreateDisk(scsi.(types.BaseVirtualController), types.ManagedObjectReference{}, "["+dsName+"] pbm-disk-vm/d1.vmdk")
	disk.CapacityInKB = 1024 * 1024
	add, _ := object.VirtualDeviceList{scsi}.ConfigSpec(types.VirtualDeviceConfigSpecOperationAdd)
	add = append(add, &types.VirtualDeviceConfigSpec{Operation: types.VirtualDeviceConfigSpecOperationAdd,
		FileOperation: types.VirtualDeviceConfigSpecFileOperationCreate, Device: disk, Profile: profile(polA)})
	spec := types.VirtualMachineConfigSpec{Name: "pbm-disk-vm", GuestId: "otherGuest64", NumCPUs: 1, MemoryMB: 256,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[" + dsName + "]"}, DeviceChange: add, VmProfile: profile(polA)}
	tk, err := folders.VmFolder.CreateVM(ctx, spec, pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := tk.WaitForResult(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	vmRef := info.Result.(types.ManagedObjectReference)
	vm := object.NewVirtualMachine(c.Client, vmRef)
	diskKeys := func() []int32 {
		t.Helper()
		var o mo.VirtualMachine
		if err := c.RetrieveOne(ctx, vmRef, []string{"config.hardware.device"}, &o); err != nil {
			t.Fatal(err)
		}
		var keys []int32
		for _, d := range object.VirtualDeviceList(o.Config.Hardware.Device).SelectByType((*types.VirtualDisk)(nil)) {
			keys = append(keys, d.GetVirtualDevice().Key)
		}
		return keys
	}
	k := diskKeys()
	if len(k) != 1 {
		t.Fatalf("디스크 %v", k)
	}
	d1 := fmt.Sprintf("%s:%d", vmRef.Value, k[0])
	if got := assoc(pbmtypes.PbmObjectTypeVirtualDiskId, d1); fmt.Sprint(got) != fmt.Sprint([]string{polA}) {
		t.Fatalf("생성 뒤 디스크 정책 %v, 기대 [%s]", got, polA)
	}

	// 재구성: 정책 B 디스크 추가(요청은 임시 키) → 새로 생긴 디스크에 연결
	var cur mo.VirtualMachine
	_ = c.RetrieveOne(ctx, vmRef, []string{"config.hardware.device"}, &cur)
	ctrl, _ := object.VirtualDeviceList(cur.Config.Hardware.Device).FindSCSIController("")
	disk2 := object.VirtualDeviceList(cur.Config.Hardware.Device).CreateDisk(ctrl, types.ManagedObjectReference{}, "["+dsName+"] pbm-disk-vm/d2.vmdk")
	disk2.CapacityInKB = 1024 * 1024
	run := func(s types.VirtualMachineConfigSpec) {
		t.Helper()
		tk, err := vm.Reconfigure(ctx, s)
		if err == nil {
			err = tk.Wait(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	run(types.VirtualMachineConfigSpec{DeviceChange: []types.BaseVirtualDeviceConfigSpec{&types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationAdd, FileOperation: types.VirtualDeviceConfigSpecFileOperationCreate,
		Device: disk2, Profile: profile(polB)}}})
	k = diskKeys()
	if len(k) != 2 {
		t.Fatalf("디스크 %v", k)
	}
	d2 := fmt.Sprintf("%s:%d", vmRef.Value, k[1])
	if got := assoc(pbmtypes.PbmObjectTypeVirtualDiskId, d2); fmt.Sprint(got) != fmt.Sprint([]string{polB}) {
		t.Fatalf("추가 디스크 정책 %v, 기대 [%s]", got, polB)
	}
	if got := assoc(pbmtypes.PbmObjectTypeVirtualDiskId, d1); fmt.Sprint(got) != fmt.Sprint([]string{polA}) {
		t.Fatalf("기존 디스크 정책이 바뀌었다: %v", got)
	}

	// 재구성: 첫 디스크 정책 비우기(edit + EmptyProfileSpec), 둘째 디스크 제거
	_ = c.RetrieveOne(ctx, vmRef, []string{"config.hardware.device"}, &cur)
	disks := object.VirtualDeviceList(cur.Config.Hardware.Device).SelectByType((*types.VirtualDisk)(nil))
	run(types.VirtualMachineConfigSpec{DeviceChange: []types.BaseVirtualDeviceConfigSpec{
		&types.VirtualDeviceConfigSpec{Operation: types.VirtualDeviceConfigSpecOperationEdit, Device: disks[0],
			Profile: []types.BaseVirtualMachineProfileSpec{&types.VirtualMachineEmptyProfileSpec{}}},
		&types.VirtualDeviceConfigSpec{Operation: types.VirtualDeviceConfigSpecOperationRemove, Device: disks[1]},
	}})
	if got := assoc(pbmtypes.PbmObjectTypeVirtualDiskId, d1); len(got) != 0 {
		t.Fatalf("정책을 비운 디스크에 연결이 남았다: %v", got)
	}
	if got := assoc(pbmtypes.PbmObjectTypeVirtualDiskId, d2); len(got) != 0 {
		t.Fatalf("제거한 디스크에 연결이 남았다: %v", got)
	}
	if got := assoc(pbmtypes.PbmObjectTypeVirtualMachine, vmRef.Value); fmt.Sprint(got) != fmt.Sprint([]string{polA}) {
		t.Fatalf("VM 홈 정책 %v", got)
	}

	// VM 삭제 → 연결 정리, 불일치 없음
	if tk, err := vm.Destroy(ctx); err != nil || tk.Wait(ctx) != nil {
		t.Fatalf("destroy: %v", err)
	}
	if got := assoc(pbmtypes.PbmObjectTypeVirtualMachine, vmRef.Value); len(got) != 0 {
		t.Fatalf("삭제한 VM에 정책 연결이 남았다: %v", got)
	}
	if issues := e.Sim.VC.Consistency(); len(issues) > 0 {
		t.Fatalf("불일치: %v", issues)
	}
}
