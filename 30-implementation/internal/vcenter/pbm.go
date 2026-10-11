package vcenter

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	pbmmethods "github.com/vmware/govmomi/pbm/methods"
	pbmsim "github.com/vmware/govmomi/pbm/simulator"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
)

// PBM(스토리지 정책, FR-VC-0210, WP 4.11). vcsim PBM 시뮬레이터(/pbm/sdk)를 재사용하고(NFR-ARC-0104), 다음 공백을 메운다(R-37):
//   - vcsim PbmQueryAssociatedProfile(s)는 항상 빈 결과다. VM 생성·복제·재구성·이전 요청에 지정한 VM 홈 정책
//     (VirtualMachineDefinedProfileSpec.profileId)을 Task 성공 시 VM과 연결하고, 조회에 그 정책을 돌려준다.
//     VirtualMachineEmptyProfileSpec은 연결을 지운다.
//   - 디스크 단위 정책(2026-10-11): 생성·재구성의 deviceChange[].profile(가상 디스크)과 복제·이전의 디스크 위치
//     지정(disk[].profile)을 디스크(PbmServerObjectRef virtualDiskId, 키 "<VM MoRef>:<디스크 장치 키>")와 연결한다.
//     새로 추가하는 디스크는 요청의 임시 키 대신, Task 성공 뒤 VM에 새로 생긴 디스크(키 오름차순)에 추가 순서대로 대응시킨다
//     (재구성은 요청 직전 디스크 목록을 어댑터에서 기록). 디스크 제거·VM 삭제 뒤의 연결은 지운다.
//   - 기본 정책 목록은 vcsim 값(vCenter 6.7 기본 정책 기반)을 그대로 쓴다. 7.0 기본 정책 목록과의 차이는 미검증이다.

var (
	pbmOwners sync.Map // vim 레지스트리 → *VCenter (PBM 래퍼가 연결 상태를 찾는다)
	pbmTaskRe = regexp.MustCompile(`<returnval type="Task">(task-[0-9]+)</returnval>`)
)

func init() {
	// pbm/simulator의 init이 먼저 PBM 레지스트리를 등록한다. 여기서 같은 경로로 다시 등록하면 객체가 덮어써지므로
	// ProfileManager를 래퍼로 바꾼다(vcsim Service.RegisterSDK: 같은 경로면 객체를 복사).
	simulator.RegisterEndpoint(func(s *simulator.Service, r *simulator.Registry) {
		owner, ok := pbmOwners.Load(r)
		if !ok || !r.IsVPX() {
			return
		}
		pr := pbmsim.New()
		ref := types.ManagedObjectReference{Type: "PbmProfileProfileManager", Value: "ProfileManager"}
		pm, ok := pr.Get(ref).(*pbmsim.ProfileManager)
		if !ok {
			return
		}
		pr.Put(&pbmProfileManager{ProfileManager: pm, v: owner.(*VCenter)})
		s.RegisterSDK(pr)
	})
}

type pbmPending struct {
	method string
	this   string   // 재구성·이전: 대상 VM
	vmSet  bool     // VM 홈 정책 지정이 있다
	ids    []string // 비면 연결 삭제(VirtualMachineEmptyProfileSpec)
	disks  []pbmDisk
	prev   map[int32]bool // 재구성 요청 직전의 디스크 키(새 디스크 판별)
}

// pbmDisk는 디스크 하나의 정책 지정이다. key ≥ 0이면 기존 디스크, 아니면 newIndex번째로 추가한 디스크.
type pbmDisk struct {
	key      int32
	newIndex int
	ids      []string // 비면 연결 삭제
	remove   bool     // 디스크 제거
}

type pbmState struct {
	mu      sync.Mutex
	assoc   map[string][]string   // VM MoRef 또는 "VM MoRef:디스크 키" → 정책 ID
	pending map[string]pbmPending // Task MoRef → 지정 정책
	prev    map[string]map[int32]bool
}

func newPBMState() *pbmState {
	return &pbmState{assoc: map[string][]string{}, pending: map[string]pbmPending{}, prev: map[string]map[int32]bool{}}
}

func profileIDs(spec []types.BaseVirtualMachineProfileSpec) []string {
	var ids []string
	for _, s := range spec {
		if d, ok := s.(*types.VirtualMachineDefinedProfileSpec); ok && d.ProfileId != "" {
			ids = append(ids, d.ProfileId)
		}
	}
	return ids
}

// diskChanges는 deviceChange의 가상 디스크 정책 지정·제거를 읽는다(추가 디스크는 추가 순서 번호).
func diskChanges(changes []types.BaseVirtualDeviceConfigSpec) []pbmDisk {
	var out []pbmDisk
	added := 0
	for _, c := range changes {
		spec := c.GetVirtualDeviceConfigSpec()
		disk, ok := spec.Device.(*types.VirtualDisk)
		if !ok {
			continue
		}
		switch spec.Operation {
		case types.VirtualDeviceConfigSpecOperationAdd:
			if spec.Profile != nil {
				out = append(out, pbmDisk{key: -1, newIndex: added, ids: profileIDs(spec.Profile)})
			}
			added++
		case types.VirtualDeviceConfigSpecOperationEdit:
			if spec.Profile != nil {
				out = append(out, pbmDisk{key: disk.Key, ids: profileIDs(spec.Profile)})
			}
		case types.VirtualDeviceConfigSpecOperationRemove:
			out = append(out, pbmDisk{key: disk.Key, remove: true})
		}
	}
	return out
}

func locatorDisks(locs []types.VirtualMachineRelocateSpecDiskLocator) []pbmDisk {
	var out []pbmDisk
	for _, l := range locs {
		if l.Profile != nil {
			out = append(out, pbmDisk{key: l.DiskId, ids: profileIDs(l.Profile)})
		}
	}
	return out
}

// notePBMReconfig는 어댑터 훅(요청 처리 전)에서 재구성 대상 VM의 현재 디스크 키를 기록한다(추가 디스크 판별용).
func (v *VCenter) notePBMReconfig(ctx *simulator.Context, vm *simulator.VirtualMachine, spec *types.VirtualMachineConfigSpec) {
	if len(diskChanges(spec.DeviceChange)) == 0 {
		return
	}
	keys := map[int32]bool{}
	ctx.WithLock(vm, func() {
		for _, d := range vm.Config.Hardware.Device {
			if disk, ok := d.(*types.VirtualDisk); ok {
				keys[disk.Key] = true
			}
		}
	})
	v.pbm.mu.Lock()
	v.pbm.prev[vm.Self.Value] = keys
	v.pbm.mu.Unlock()
}

// observePBM은 성공 응답을 받은 VM 생성·복제·재구성·이전 요청에서 VM 홈 정책 지정을 읽어 Task와 연결한다.
func (v *VCenter) observePBM(method string, req, resp []byte) {
	var spec []types.BaseVirtualMachineProfileSpec
	p := pbmPending{method: method}
	switch method {
	case "CreateVM_Task":
		var b struct {
			Req types.CreateVM_Task `xml:"Body>CreateVM_Task"`
		}
		if decodeSOAP(req, &b) == nil {
			spec, p.disks = b.Req.Config.VmProfile, diskChanges(b.Req.Config.DeviceChange)
		}
	case "ReconfigVM_Task":
		var b struct {
			Req types.ReconfigVM_Task `xml:"Body>ReconfigVM_Task"`
		}
		if decodeSOAP(req, &b) == nil {
			spec, p.disks, p.this = b.Req.Spec.VmProfile, diskChanges(b.Req.Spec.DeviceChange), b.Req.This.Value
		}
	case "CloneVM_Task":
		var b struct {
			Req types.CloneVM_Task `xml:"Body>CloneVM_Task"`
		}
		if decodeSOAP(req, &b) == nil {
			spec, p.disks = b.Req.Spec.Location.Profile, locatorDisks(b.Req.Spec.Location.Disk)
		}
	case "RelocateVM_Task":
		var b struct {
			Req types.RelocateVM_Task `xml:"Body>RelocateVM_Task"`
		}
		if decodeSOAP(req, &b) == nil {
			spec, p.disks, p.this = b.Req.Spec.Profile, locatorDisks(b.Req.Spec.Disk), b.Req.This.Value
		}
	}
	p.vmSet, p.ids = spec != nil, profileIDs(spec)
	v.pbm.mu.Lock()
	defer v.pbm.mu.Unlock()
	if method == "ReconfigVM_Task" {
		p.prev = v.pbm.prev[p.this]
		delete(v.pbm.prev, p.this)
	}
	if !p.vmSet && len(p.disks) == 0 {
		return
	}
	m := pbmTaskRe.FindSubmatch(resp)
	if m == nil {
		return
	}
	v.pbm.pending[string(m[1])] = p
}

func decodeSOAP(b []byte, out any) error {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.TypeFunc = types.TypeFunc()
	return dec.Decode(out)
}

// resolveLocked는 끝난 Task의 정책 지정을 VM 연결로 옮긴다.
func (v *VCenter) resolvePBMLocked() {
	m := v.Model()
	if m == nil {
		return
	}
	for ref, p := range v.pbm.pending {
		t, ok := m.Map().Get(types.ManagedObjectReference{Type: "Task", Value: ref}).(*simulator.Task)
		if !ok {
			delete(v.pbm.pending, ref)
			continue
		}
		var state types.TaskInfoState
		var result any
		ctxLock(m, t, func() { state, result = t.Info.State, t.Info.Result })
		switch state {
		case types.TaskInfoStateSuccess:
			vm := p.this
			if r, ok := result.(types.ManagedObjectReference); ok && vm == "" {
				vm = r.Value
			}
			if vm != "" {
				if p.vmSet {
					v.setAssocLocked(vm, p.ids)
				}
				v.resolveDisksLocked(m, vm, p)
			}
			delete(v.pbm.pending, ref)
		case types.TaskInfoStateError:
			delete(v.pbm.pending, ref)
		}
	}
	v.prunePBMLocked(m)
}

func (v *VCenter) setAssocLocked(key string, ids []string) {
	if len(ids) == 0 {
		delete(v.pbm.assoc, key)
	} else {
		v.pbm.assoc[key] = ids
	}
}

// vmDiskKeys는 VM의 가상 디스크 장치 키(오름차순)다. VM이 없으면 nil, false.
func vmDiskKeys(m *simulator.Model, vm string) ([]int32, bool) {
	obj, ok := m.Map().Get(types.ManagedObjectReference{Type: "VirtualMachine", Value: vm}).(*simulator.VirtualMachine)
	if !ok {
		return nil, false
	}
	var keys []int32
	ctx := &simulator.Context{Map: m.Map(), Context: context.Background(), Session: &simulator.Session{}}
	ctx.WithLock(obj, func() {
		if obj.Config == nil {
			return
		}
		for _, d := range obj.Config.Hardware.Device {
			if disk, ok := d.(*types.VirtualDisk); ok {
				keys = append(keys, disk.Key)
			}
		}
	})
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys, true
}

// resolveDisksLocked는 끝난 Task의 디스크 정책 지정을 디스크 연결로 옮긴다.
func (v *VCenter) resolveDisksLocked(m *simulator.Model, vm string, p pbmPending) {
	if len(p.disks) == 0 {
		return
	}
	keys, ok := vmDiskKeys(m, vm)
	if !ok {
		return
	}
	var added []int32 // 이번 Task로 새로 생긴 디스크(생성은 모든 디스크)
	for _, k := range keys {
		if p.method == "CreateVM_Task" || (p.method == "ReconfigVM_Task" && p.prev != nil && !p.prev[k]) {
			added = append(added, k)
		}
	}
	for _, d := range p.disks {
		key := d.key
		if key < 0 {
			if d.newIndex >= len(added) {
				continue // 대응하는 새 디스크를 찾지 못함(요청 직전 목록 없음 등)
			}
			key = added[d.newIndex]
		}
		k := fmt.Sprintf("%s:%d", vm, key)
		if d.remove {
			delete(v.pbm.assoc, k)
			continue
		}
		v.setAssocLocked(k, d.ids)
	}
}

// prunePBMLocked는 없어진 VM·디스크의 연결을 지운다.
func (v *VCenter) prunePBMLocked(m *simulator.Model) {
	disks := map[string]map[int32]bool{}
	for key := range v.pbm.assoc {
		vm, disk, isDisk := strings.Cut(key, ":")
		dk, seen := disks[vm]
		if !seen {
			if keys, exists := vmDiskKeys(m, vm); exists {
				dk = map[int32]bool{}
				for _, k := range keys {
					dk[k] = true
				}
			}
			disks[vm] = dk
		}
		if dk == nil { // VM 없음
			delete(v.pbm.assoc, key)
			continue
		}
		if isDisk {
			if n, err := strconv.Atoi(disk); err != nil || !dk[int32(n)] {
				delete(v.pbm.assoc, key)
			}
		}
	}
}

func ctxLock(m *simulator.Model, obj *simulator.Task, f func()) {
	ctx := &simulator.Context{Map: m.Map(), Context: context.Background(), Session: &simulator.Session{}}
	ctx.WithLock(obj, f)
}

// pbmProfileManager는 vcsim ProfileManager에 VM 정책 연결 조회를 더한 래퍼다.
type pbmProfileManager struct {
	*pbmsim.ProfileManager
	v *VCenter
}

func (w *pbmProfileManager) associated(obj pbmtypes.PbmServerObjectRef) []pbmtypes.PbmProfileId {
	switch obj.ObjectType {
	case string(pbmtypes.PbmObjectTypeVirtualMachine), string(pbmtypes.PbmObjectTypeVirtualDiskId):
	default:
		return nil
	}
	var out []pbmtypes.PbmProfileId
	for _, id := range w.v.pbm.assoc[obj.Key] {
		out = append(out, pbmtypes.PbmProfileId{UniqueId: id})
	}
	return out
}

func (w *pbmProfileManager) PbmQueryAssociatedProfile(req *pbmtypes.PbmQueryAssociatedProfile) soap.HasFault {
	w.v.pbm.mu.Lock()
	defer w.v.pbm.mu.Unlock()
	w.v.resolvePBMLocked()
	return &pbmmethods.PbmQueryAssociatedProfileBody{Res: &pbmtypes.PbmQueryAssociatedProfileResponse{Returnval: w.associated(req.Entity)}}
}

func (w *pbmProfileManager) PbmQueryAssociatedProfiles(req *pbmtypes.PbmQueryAssociatedProfiles) soap.HasFault {
	w.v.pbm.mu.Lock()
	defer w.v.pbm.mu.Unlock()
	w.v.resolvePBMLocked()
	res := &pbmtypes.PbmQueryAssociatedProfilesResponse{}
	for _, e := range req.Entities {
		res.Returnval = append(res.Returnval, pbmtypes.PbmQueryProfileResult{Object: e, ProfileId: w.associated(e)})
	}
	return &pbmmethods.PbmQueryAssociatedProfilesBody{Res: res}
}

// pbmExport·pbmImport는 VM ↔ 정책 연결을 Snapshot에 담고 되돌린다(snapshot.go). nil이면 비운다.
func (v *VCenter) pbmExport() map[string][]string {
	v.pbm.mu.Lock()
	defer v.pbm.mu.Unlock()
	if v.Model() != nil {
		v.resolvePBMLocked()
	}
	out := map[string][]string{}
	for vm, ids := range v.pbm.assoc {
		out[vm] = append([]string{}, ids...)
	}
	return out
}

func (v *VCenter) pbmImport(assoc map[string][]string) {
	v.pbm.mu.Lock()
	defer v.pbm.mu.Unlock()
	v.pbm.assoc, v.pbm.pending, v.pbm.prev = map[string][]string{}, map[string]pbmPending{}, map[string]map[int32]bool{}
	for vm, ids := range assoc {
		v.pbm.assoc[vm] = append([]string{}, ids...)
	}
}
