package relations

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	managerBase = "/api/v1"
	policyBase  = "/policy/api/v1"
)

// derive는 vCenter 인벤토리에서 NSX 파생 객체를 만든다(FR-NSX-0602·0604·0608, FR-NSX-0226·0227).
// 필드 값 형식(local_id_on_host, compute_ids, Segment Port ID·display_name)은 스펙 필드 정의에 맞춘 가정이다.
//
// 증분 조정(R-45): VM별 파생 결과(Fabric VM·VIF·Segment Port)를 캐시하고, 이번 조정에서는 vcsim 변경 핸들러가 표시한
// VM, 새로 생기거나 없어진 VM, 매핑이 바뀐 포트그룹에 NIC가 붙은 VM만 다시 조회한다. 호스트·Transport Node는 수가
// 적어 매번 계산한다. full이거나 vcsim 모델이 바뀌었거나(Reset·복원) 호스트 UUID가 바뀌면 모든 VM을 다시 조회한다.
// Fabric VM의 tags(NSX 소유)는 조립할 때 붙인다. Verify면 같은 입력으로 전체 계산을 한 번 더 해 결과를 대조한다.
func (e *Engine) derive(ctx context.Context, c *vim25.Client, dvs *object.DistributedVirtualSwitch, pgs []pgInfo, rep *Report, full bool) error {
	// 변경 표시는 조회 전에 가져온다: 조회 중 생긴 변경은 다음 조정에 남는다.
	e.dirtyMu.Lock()
	dirty := e.dirty
	e.dirty = map[string]bool{}
	e.dirtyMu.Unlock()

	pc := property.DefaultCollector(c)
	cm := e.nsx.ComputeManagerID()
	objs := map[string]map[string]any{}

	hostRefs, err := find(ctx, c, "HostSystem")
	if err != nil {
		return err
	}
	hostUUID := map[string]string{}
	if len(hostRefs) > 0 {
		var hosts []mo.HostSystem
		if err := pc.Retrieve(ctx, hostRefs, []string{"name", "parent", "hardware.systemInfo.uuid", "summary.config.product", "runtime.connectionState", "runtime.inMaintenanceMode"}, &hosts); err != nil {
			return err
		}
		for _, h := range hosts {
			id := h.Hardware.SystemInfo.Uuid
			hostUUID[h.Self.Value] = id
			n := map[string]any{
				"resource_type": "DiscoveredNode", "external_id": id, "display_name": h.Name,
				"cm_local_id": h.Self.Value, "node_type": "HostNode", "origin_id": cm, "os_type": "ESXI",
			}
			if h.Parent != nil {
				n["parent_compute_collection"] = h.Parent.Value
			}
			if p := h.Summary.Config.Product; p != nil {
				n["os_version"] = p.Version
			}
			objs[managerBase+"/fabric/discovered-nodes/"+id] = n
			rep.Hosts++
		}
		if err := e.deriveTransportNodes(ctx, c, dvs, hosts, objs); err != nil {
			return err
		}
	}

	segOfPG := map[string]string{}
	for _, pg := range pgs {
		if pg.backing == "nsx" && strings.HasPrefix(pg.segment, "/infra/segments/") {
			segOfPG[pg.key] = pg.segment
		}
	}
	vmRefs, err := find(ctx, c, "VirtualMachine")
	if err != nil {
		return err
	}
	cache := &e.cache
	model := e.vc.Model()
	if full || cache.vms == nil || cache.model != model || !sameMap(cache.hostUUID, hostUUID) {
		cache.vms, rep.Mode = map[string]*vmEntry{}, "full"
	} else {
		rep.Mode = "incremental"
	}
	changedPG := diffKeys(cache.segOfPG, segOfPG)
	current := map[string]bool{}
	var fetch []types.ManagedObjectReference
	for _, ref := range vmRefs {
		current[ref.Value] = true
		ent, cached := cache.vms[ref.Value]
		if !cached || dirty["VirtualMachine:"+ref.Value] || ent.usesAny(changedPG) {
			fetch = append(fetch, ref)
		}
	}
	for v := range cache.vms {
		if !current[v] {
			delete(cache.vms, v) // 삭제·등록 해제된 VM
		}
	}
	if err := e.refreshVMs(ctx, pc, fetch, hostUUID, segOfPG, cm, cache.vms); err != nil {
		cache.vms = nil // 다음 조정은 전체 계산
		return err
	}
	cache.model, cache.hostUUID, cache.segOfPG = model, hostUUID, segOfPG
	rep.Refreshed = len(fetch)

	e.assemble(cache.vms, objs, rep)
	if e.Verify && rep.Mode == "incremental" {
		e.verify(ctx, pc, vmRefs, hostUUID, segOfPG, cm, cache.vms, rep)
	}
	e.nsx.ReplaceDerived(objs)
	return nil
}

// vmEntry는 VM 하나의 파생 결과다(태그 제외). pgKeys는 NIC가 붙은 분산 포트그룹 키다.
type vmEntry struct {
	inst   string
	objs   map[string]map[string]any
	pgKeys []string
	vifs   int
	ports  int
}

func (v *vmEntry) usesAny(keys map[string]bool) bool {
	for _, k := range v.pgKeys {
		if keys[k] {
			return true
		}
	}
	return false
}

// deriveCache는 증분 조정 상태다. model이 바뀌면(Reset·복원) 버린다.
type deriveCache struct {
	model    any
	hostUUID map[string]string
	segOfPG  map[string]string
	vms      map[string]*vmEntry // VM MoRef 값 →
}

// refreshVMs는 refs의 VM을 조회해 파생 결과를 into에 넣는다.
func (e *Engine) refreshVMs(ctx context.Context, pc *property.Collector, refs []types.ManagedObjectReference, hostUUID, segOfPG map[string]string, cm string, into map[string]*vmEntry) error {
	if len(refs) == 0 {
		return nil
	}
	var vms []mo.VirtualMachine
	if err := pc.Retrieve(ctx, refs, []string{"name", "config.instanceUuid", "config.uuid", "config.hardware.device", "runtime.powerState", "runtime.host"}, &vms); err != nil {
		return err
	}
	for _, vm := range vms {
		into[vm.Self.Value] = e.vmObjects(vm, hostUUID, segOfPG, cm)
	}
	return nil
}

// vmObjects는 VM 하나의 Fabric VM·VIF·Segment Port를 만든다.
func (e *Engine) vmObjects(vm mo.VirtualMachine, hostUUID, segOfPG map[string]string, cm string) *vmEntry {
	ent := &vmEntry{objs: map[string]map[string]any{}}
	if vm.Config == nil || vm.Config.InstanceUuid == "" {
		return ent
	}
	inst, local := vm.Config.InstanceUuid, localID(vm.Self.Value)
	ent.inst = inst
	host := ""
	if vm.Runtime.Host != nil {
		host = hostUUID[vm.Runtime.Host.Value]
	}
	ent.objs[managerBase+"/fabric/virtual-machines/"+inst] = map[string]any{
		"resource_type": "VirtualMachine", "external_id": inst, "display_name": vm.Name,
		"local_id_on_host": local, "power_state": powerState(vm.Runtime.PowerState), "type": "REGULAR",
		"compute_ids": []string{"moIdOnHost:" + local, "hostLocalId:" + local, "instanceUuid:" + inst, "biosUuid:" + vm.Config.Uuid, "externalId:" + inst},
		"host_id":     host,
		"source":      map[string]any{"target_id": cm, "target_type": "ComputeManager"},
	}
	for _, d := range vm.Config.Hardware.Device {
		nic, ok := d.(types.BaseVirtualEthernetCard)
		if !ok {
			continue
		}
		card := nic.GetVirtualEthernetCard()
		ext := fmt.Sprintf("%s-%d", inst, card.Key)
		label := "Network adapter"
		if card.DeviceInfo != nil {
			label = card.DeviceInfo.GetDescription().Label
		}
		vif := map[string]any{
			"resource_type": "VirtualNetworkInterface", "external_id": ext, "owner_vm_id": inst, "owner_vm_type": "REGULAR",
			"vm_local_id_on_host": local, "device_key": strconv.Itoa(int(card.Key)), "device_name": label,
			"mac_address": card.MacAddress, "host_id": host, "display_name": vm.Name + "/" + label,
		}
		if b, ok := card.Backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo); ok {
			ent.pgKeys = append(ent.pgKeys, b.Port.PortgroupKey)
			if seg, ok := segOfPG[b.Port.PortgroupKey]; ok {
				att := e.uuid("attachment/" + ext)
				vif["lport_attachment_id"] = att
				ent.objs[policyBase+seg+"/ports/"+att] = map[string]any{
					"resource_type": "SegmentPort", "id": att, "display_name": vm.Name + "/" + label,
					"attachment": map[string]any{"id": att}, "admin_state": "UP", "unique_id": att,
				}
				ent.ports++
			}
		}
		ent.objs[managerBase+"/fabric/vifs/"+ext] = vif
		ent.vifs++
	}
	return ent
}

// assemble은 캐시한 VM 결과를 복사해 objs에 모으고 NSX 소유 태그를 붙인다. 복사본을 쓰므로 캐시는
// ReplaceDerived가 붙이는 메타데이터와 섞이지 않는다(이전 전체 계산과 같은 결과).
func (e *Engine) assemble(vms map[string]*vmEntry, objs map[string]map[string]any, rep *Report) {
	tags := e.nsx.VMTags()
	for _, ent := range vms {
		if ent.inst == "" {
			continue
		}
		for k, o := range ent.objs {
			cp := make(map[string]any, len(o)+1)
			for f, v := range o {
				cp[f] = v
			}
			if strings.HasPrefix(k, managerBase+"/fabric/virtual-machines/") {
				if t, ok := tags[ent.inst]; ok && len(t) > 0 {
					cp["tags"] = t
				}
			}
			objs[k] = cp
		}
		rep.VMs++
		rep.VIFs += ent.vifs
		rep.Ports += ent.ports
	}
}

// verify는 모든 VM을 다시 조회한 결과와 캐시를 VM 단위로 대조한다(시험용). vcsim Task는 교차 락 밖에서도 VM을 바꾸므로
// 두 조회 사이에 바뀐 VM은 변경 표시가 이미 남아 있다(변경 핸들러가 동기로 표시) — 그런 VM은 다음 조정이 반영하므로
// 차이로 세지 않는다. 표시 없이 다른 VM만 증분 조정이 놓친 변경이다.
func (e *Engine) verify(ctx context.Context, pc *property.Collector, vmRefs []types.ManagedObjectReference, hostUUID, segOfPG map[string]string, cm string, cached map[string]*vmEntry, rep *Report) {
	all := map[string]*vmEntry{}
	if err := e.refreshVMs(ctx, pc, vmRefs, hostUUID, segOfPG, cm, all); err != nil {
		rep.Errors["incremental-verify"] = err.Error()
		return
	}
	e.dirtyMu.Lock()
	pending := make(map[string]bool, len(e.dirty))
	for k := range e.dirty {
		pending[k] = true
	}
	e.dirtyMu.Unlock()
	var diff []string
	for ref, want := range all {
		got, ok := cached[ref]
		if pending["VirtualMachine:"+ref] {
			continue
		}
		if !ok {
			diff = append(diff, "missing "+ref)
		} else if want.inst != got.inst || want.vifs != got.vifs || want.ports != got.ports || !reflect.DeepEqual(want.objs, got.objs) {
			diff = append(diff, "differs "+ref)
		}
	}
	for ref := range cached {
		if _, ok := all[ref]; !ok && !pending["VirtualMachine:"+ref] {
			diff = append(diff, "extra "+ref)
		}
	}
	if len(diff) > 0 {
		sort.Strings(diff)
		e.mu.Lock()
		e.mismatches++
		e.mu.Unlock()
		first := diff[:min(len(diff), 5)]
		rep.Errors["incremental-verify"] = fmt.Sprintf("%d VM(s) differ: %v", len(diff), first)
		e.log.Error("incremental reconcile differs from full derive", "vms", len(diff), "first", first)
	}
}

// diffKeys는 두 맵에서 값이 다르거나 한쪽에만 있는 키다.
func diffKeys(a, b map[string]string) map[string]bool {
	out := map[string]bool{}
	for k, v := range a {
		if b[k] != v {
			out[k] = true
		}
	}
	for k, v := range b {
		if a[k] != v {
			out[k] = true
		}
	}
	return out
}

func sameMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// deriveTransportNodes는 NSX 준비 VDS(D8)에 연결된 호스트를 Host Transport Node로 만든다(FR-NSX-0206).
// TransportNode(호스트 스위치 VDS, TZ 끝점 = vds_transport_zones)와 /transport-nodes/{id}/state(TransportNodeState)를 둔다.
// 상태는 vCenter 호스트 상태에서 파생한다: 연결 끊김 → failed(가정), 유지보수 모드 → maintenance_mode ENABLED.
// ID는 호스트 MoRef 기반 결정론적 UUID다. node_id = id, discovered_node_id = Discovered Node external_id(가정).
func (e *Engine) deriveTransportNodes(ctx context.Context, c *vim25.Client, dvs *object.DistributedVirtualSwitch, hosts []mo.HostSystem, objs map[string]map[string]any) error {
	if dvs == nil {
		return nil
	}
	var sw mo.DistributedVirtualSwitch
	if err := property.DefaultCollector(c).RetrieveOne(ctx, dvs.Reference(), []string{"uuid", "name", "summary.hostMember"}, &sw); err != nil {
		return err
	}
	member := map[string]bool{}
	for _, h := range sw.Summary.HostMember {
		member[h.Value] = true
	}
	tzIDs := e.nsx.TransportZoneIDs()
	var eps []any
	for _, tz := range e.fix.NSX.VDSTransportZones {
		if id := tzIDs[tz]; id != "" {
			eps = append(eps, map[string]any{"transport_zone_id": id})
		}
	}
	cm := e.nsx.ComputeManagerID()
	for _, h := range hosts {
		if !member[h.Self.Value] {
			continue
		}
		id := e.uuid("transport-node/" + h.Self.Value)
		mm := "DISABLED"
		if h.Runtime.InMaintenanceMode {
			mm = "ENABLED"
		}
		node := map[string]any{"resource_type": "HostNode", "id": id, "display_name": h.Name, "external_id": h.Hardware.SystemInfo.Uuid,
			"os_type": "ESXI", "discovered_node_id": h.Hardware.SystemInfo.Uuid, "managed_by_server": cm, "fqdn": h.Name, "ip_addresses": []any{}}
		if p := h.Summary.Config.Product; p != nil {
			node["os_version"] = p.Version
		}
		if h.Parent != nil {
			node["compute_collection_id"] = cm + ":" + h.Parent.Value
		}
		objs[managerBase+"/transport-nodes/"+id] = map[string]any{
			"resource_type": "TransportNode", "id": id, "node_id": id, "display_name": h.Name, "maintenance_mode": mm,
			"node_deployment_info": node,
			"host_switch_spec": map[string]any{"resource_type": "StandardHostSwitchSpec", "host_switches": []any{map[string]any{
				"host_switch_type": "VDS", "host_switch_id": sw.Uuid, "host_switch_name": sw.Name, "host_switch_mode": "STANDARD",
				"transport_zone_endpoints": eps,
			}}},
		}
		state := map[string]any{"transport_node_id": id, "state": "success", "node_deployment_state": map[string]any{"state": "NODE_READY"},
			"maintenance_mode_state": mm}
		if h.Runtime.ConnectionState != types.HostSystemConnectionStateConnected {
			state["state"] = "failed"
			state["node_deployment_state"] = map[string]any{"state": "MPA_DISCONNECTED"}
			state["failure_message"] = "Host " + h.Name + " is " + string(h.Runtime.ConnectionState) + " in vCenter."
		}
		objs[managerBase+"/transport-nodes/"+id+"/state"] = state
	}
	return nil
}

func localID(moref string) string {
	if i := strings.LastIndex(moref, "-"); i >= 0 {
		return moref[i+1:]
	}
	return moref
}

func powerState(p types.VirtualMachinePowerState) string {
	switch p {
	case types.VirtualMachinePowerStatePoweredOn:
		return "VM_RUNNING"
	case types.VirtualMachinePowerStatePoweredOff:
		return "VM_STOPPED"
	case types.VirtualMachinePowerStateSuspended:
		return "VM_SUSPENDED"
	}
	return "UNKNOWN"
}

// uuid는 환경 Namespace·Seed 기반 결정론적 UUID다(FR-DM-0207·0210).
func (e *Engine) uuid(key string) string {
	h := func(s string) uint64 {
		var x uint64 = 14695981039346656037
		for i := 0; i < len(s); i++ {
			x ^= uint64(s[i])
			x *= 1099511628211
		}
		return x
	}
	ns := fmt.Sprintf("%s/%d/", e.fix.Namespace, e.fix.Seed)
	b := fmt.Sprintf("%016x%016x", h(ns+key), h(key+"/"+ns))
	return b[0:8] + "-" + b[8:12] + "-4" + b[13:16] + "-a" + b[17:20] + "-" + b[20:32]
}
