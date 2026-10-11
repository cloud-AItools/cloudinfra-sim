package vcenter

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// REST(/api, 7.0 U2+)는 vcsim에 없으므로 vcsim 인벤토리 뷰로 구현한다(FR-VC-0205, R-17).
//
// 계약 근거(SRC-VAPI-SDK-7032, 2026-10-10 결정 ②): vsphere-automation-sdk-python v7.0.3.2의
// vapi_client_bindings 3.8.0(오퍼레이션·구조체·열거형·선언 오류)과 vapi_runtime 2.34.0
// (vapi_to_http_error_map, DELETE → 204, 그 밖의 성공 기본 200). 추출: .agentic/poc/tools/extract_vapi.py.
// 가정(근거 미확보): POST /api/session 성공 코드 201(서버 메타데이터의 success_response_code는 클라이언트 바인딩에 없음),
// guest_OS·하드웨어 버전 이름 변환 규칙.

const legacySession = "/rest/com/vmware/cis/session"

func xmlEscapeTo(w io.Writer, s string) error { return xml.EscapeText(w, []byte(s)) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// vAPI 표준 오류 → HTTP 상태(vapi_runtime vapi_to_http_error_map)와 /api 오류 본문(com.vmware.vapi.std.errors.error).
var errorStatus = map[string]int{
	"UNAUTHENTICATED": http.StatusUnauthorized, "UNAUTHORIZED": http.StatusForbidden, "NOT_FOUND": http.StatusNotFound,
	"INVALID_ARGUMENT": http.StatusBadRequest, "ALREADY_EXISTS": http.StatusBadRequest, "SERVICE_UNAVAILABLE": http.StatusServiceUnavailable,
	"INTERNAL_SERVER_ERROR": http.StatusInternalServerError, "OPERATION_NOT_FOUND": http.StatusNotFound,
}

func restError(w http.ResponseWriter, errType, msgID, msg string) {
	writeJSON(w, errorStatus[errType], map[string]any{
		"error_type": errType,
		"messages":   []any{map[string]any{"id": msgID, "default_message": msg, "args": []string{}}},
	})
}

func unauthenticated(w http.ResponseWriter) {
	restError(w, "UNAUTHENTICATED", "vapi.method.authentication.required", "Authentication required.")
}

// legacyREST는 vcsim vAPI(/rest)를 그대로 쓰되, MVP 범위 밖인 SAML 토큰 인증(SIGN)을 막는다(D10).
// vcsim vAPI는 "SIGN "으로 시작하는 Authorization을 검증 없이 허용하기 때문이다.
func (v *VCenter) legacyREST(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "SIGN ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sessionInfo는 vcsim vAPI 세션 저장소에서 세션 정보를 읽는다. 없으면 nil.
func sessionInfo(m *simulator.Model, id string) map[string]any {
	if id == "" {
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, legacySession+"?~action=get", nil)
	req.Header.Set("vmware-api-session-id", id)
	rec := httptest.NewRecorder()
	m.Service.ServeMux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return nil
	}
	var wrapped struct {
		Value map[string]any `json:"value"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wrapped)
	if wrapped.Value == nil {
		wrapped.Value = map[string]any{}
	}
	return wrapped.Value
}

func (v *VCenter) rest(m *simulator.Model) http.Handler {
	mux := http.NewServeMux()
	// com.vmware.cis.session.create: 출력 SecretType(토큰 문자열). 구형 세션 저장소에 연결한다(PoC-02).
	mux.HandleFunc("POST /api/session", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "SIGN ") {
			unauthenticated(w)
			return
		}
		req := r.Clone(r.Context())
		req.URL.Path = legacySession
		req.RequestURI = ""
		rec := httptest.NewRecorder()
		m.Service.ServeMux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			unauthenticated(w)
			return
		}
		var wrapped struct {
			Value string `json:"value"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &wrapped)
		writeJSON(w, http.StatusCreated, wrapped.Value) // 201: 가정
	})
	// com.vmware.cis.session.delete: DELETE → 204(vapi_runtime handler)
	mux.HandleFunc("DELETE /api/session", func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("vmware-api-session-id")
		if !v.restSessionOK(m, id) {
			unauthenticated(w)
			return
		}
		req := httptest.NewRequest(http.MethodDelete, legacySession, nil)
		req.Header.Set("vmware-api-session-id", id)
		m.Service.ServeMux.ServeHTTP(httptest.NewRecorder(), req)
		w.WriteHeader(http.StatusNoContent)
	})
	// com.vmware.cis.session.get → Session.Info {user, created_time, last_accessed_time}
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		var info map[string]any
		if id := r.Header.Get("vmware-api-session-id"); v.restSessionOK(m, id) {
			info = sessionInfo(m, id)
		}
		if info == nil {
			unauthenticated(w)
			return
		}
		out := map[string]any{}
		for _, k := range []string{"user", "created_time", "last_accessed_time"} {
			out[k] = info[k]
		}
		writeJSON(w, http.StatusOK, out)
	})

	authed := func(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !v.restSessionOK(m, r.Header.Get("vmware-api-session-id")) {
				unauthenticated(w)
				return
			}
			h(w, r)
		}
	}
	// com.vmware.appliance.system.version.get → Version.VersionStruct
	mux.HandleFunc("GET /api/appliance/system/version", authed(func(w http.ResponseWriter, _ *http.Request) {
		out := map[string]any{}
		for k, val := range v.prof.VCenter.ApplianceVersion {
			out[k] = val
		}
		if _, ok := out["install_time"]; !ok {
			out["install_time"] = v.created.Format(time.RFC3339) // 가정: 형식
		}
		writeJSON(w, http.StatusOK, out)
	}))
	mux.HandleFunc("GET /api/vcenter/vm", authed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, list(m, "VirtualMachine", vmSummary))
	}))
	// com.vmware.vcenter.VM.get → VM.Info
	mux.HandleFunc("GET /api/vcenter/vm/{vm}", authed(func(w http.ResponseWriter, r *http.Request) {
		obj := m.Map().Get(types.ManagedObjectReference{Type: "VirtualMachine", Value: r.PathValue("vm")})
		if obj == nil {
			restError(w, "NOT_FOUND", "com.vmware.api.vcenter.vm.not_found", "Virtual machine with identifier '"+r.PathValue("vm")+"' does not exist.")
			return
		}
		writeJSON(w, http.StatusOK, withLock(m, obj, vmInfo))
	}))
	for path, kind := range map[string]string{
		"/api/vcenter/datacenter": "Datacenter",
		"/api/vcenter/cluster":    "ClusterComputeResource",
		"/api/vcenter/host":       "HostSystem",
		"/api/vcenter/datastore":  "Datastore",
		"/api/vcenter/network":    "", // Network·DVPG를 함께 나열
	} {
		kind := kind
		mux.HandleFunc("GET "+path, authed(func(w http.ResponseWriter, _ *http.Request) {
			if kind == "" {
				out := list(m, "Network", networkSummary)
				out = append(out, list(m, "DistributedVirtualPortgroup", networkSummary)...)
				writeJSON(w, http.StatusOK, out)
				return
			}
			writeJSON(w, http.StatusOK, list(m, kind, inventorySummary))
		}))
	}
	v.contentRoutes(mux, m, authed) // Content Library(content.go, WP 4.11)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		restError(w, "OPERATION_NOT_FOUND", "vapi.rest.operation.not_found", "Operation not found: "+r.Method+" "+r.URL.Path)
	})
	return mux
}

type summarizer func(mo.Reference) map[string]any

func withLock(m *simulator.Model, obj mo.Reference, f summarizer) map[string]any {
	var out map[string]any
	m.Map().WithLock(m.Service.Context, obj, func() { out = f(obj) })
	return out
}

func list(m *simulator.Model, kind string, f summarizer) []map[string]any {
	objs := m.Map().AllReference(kind)
	sort.Slice(objs, func(i, j int) bool { return objs[i].Reference().Value < objs[j].Reference().Value })
	out := make([]map[string]any, 0, len(objs))
	for _, o := range objs {
		out = append(out, withLock(m, o, f))
	}
	return out
}

var powerState = map[types.VirtualMachinePowerState]string{
	types.VirtualMachinePowerStatePoweredOn:  "POWERED_ON",
	types.VirtualMachinePowerStatePoweredOff: "POWERED_OFF",
	types.VirtualMachinePowerStateSuspended:  "SUSPENDED",
}

// vmSummary → VM.Summary. REST ID = SOAP MoRef 값(FR-DM-0204).
func vmSummary(o mo.Reference) map[string]any {
	vm := o.(*simulator.VirtualMachine)
	out := map[string]any{"vm": vm.Self.Value, "name": vm.Name, "power_state": powerState[vm.Runtime.PowerState]}
	if vm.Config != nil {
		out["cpu_count"] = vm.Config.Hardware.NumCPU
		out["memory_size_MiB"] = vm.Config.Hardware.MemoryMB
	}
	return out
}

var camel = regexp.MustCompile(`([a-z])([A-Z0-9])|([0-9])([A-Z])`)

// guestOS는 vim guestId(otherGuest, rhel8_64Guest)를 vAPI guest_OS(OTHER, RHEL_8_64)로 바꾼다(가정: 이름 변환 규칙).
func guestOS(id string) string {
	s := strings.TrimSuffix(id, "Guest")
	s = camel.ReplaceAllString(s, "${1}${3}_${2}${4}")
	s = regexp.MustCompile(`([a-zA-Z])([0-9])`).ReplaceAllString(s, "${1}_${2}")
	s = strings.ToUpper(strings.ReplaceAll(s, "__", "_"))
	if s == "" {
		return "OTHER"
	}
	return s
}

// vmInfo → VM.Info. 필수 필드는 모두 채우고, vcsim에 없는 값은 계약상 기본값을 쓴다.
func vmInfo(o mo.Reference) map[string]any {
	vm := o.(*simulator.VirtualMachine)
	c := vm.Config
	out := map[string]any{
		"name": vm.Name, "power_state": powerState[vm.Runtime.PowerState], "guest_OS": guestOS(c.GuestId),
		"identity":       map[string]any{"name": vm.Name, "bios_uuid": c.Uuid, "instance_uuid": c.InstanceUuid},
		"boot_devices":   []any{},
		"cdroms":         map[string]any{},
		"floppies":       map[string]any{},
		"parallel_ports": map[string]any{},
		"serial_ports":   map[string]any{},
		"sata_adapters":  map[string]any{},
		"scsi_adapters":  map[string]any{},
		"nvme_adapters":  map[string]any{},
	}
	ver := strings.ToUpper(strings.ReplaceAll(c.Version, "-", "_"))
	out["hardware"] = map[string]any{"version": ver, "upgrade_policy": "NEVER", "upgrade_status": "NONE"}
	boot := map[string]any{"type": "BIOS", "delay": 0, "retry": false, "retry_delay": 10000, "enter_setup_mode": false}
	if strings.EqualFold(c.Firmware, "efi") {
		boot["type"] = "EFI"
	}
	if b := c.BootOptions; b != nil {
		boot["delay"] = b.BootDelay
		boot["retry_delay"] = b.BootRetryDelay
		boot["retry"] = b.BootRetryEnabled != nil && *b.BootRetryEnabled
		boot["enter_setup_mode"] = b.EnterBIOSSetup != nil && *b.EnterBIOSSetup
	}
	out["boot"] = boot
	cps := int32(1)
	if c.Hardware.NumCoresPerSocket != nil && *c.Hardware.NumCoresPerSocket > 0 {
		cps = *c.Hardware.NumCoresPerSocket
	}
	out["cpu"] = map[string]any{"count": c.Hardware.NumCPU, "cores_per_socket": cps,
		"hot_add_enabled": c.CpuHotAddEnabled != nil && *c.CpuHotAddEnabled, "hot_remove_enabled": c.CpuHotRemoveEnabled != nil && *c.CpuHotRemoveEnabled}
	out["memory"] = map[string]any{"size_MiB": c.Hardware.MemoryMB, "hot_add_enabled": c.MemoryHotAddEnabled != nil && *c.MemoryHotAddEnabled}

	disks, nics := map[string]any{}, map[string]any{}
	controllers := map[int32]types.BaseVirtualDevice{}
	for _, d := range c.Hardware.Device {
		controllers[d.GetVirtualDevice().Key] = d
	}
	for _, d := range c.Hardware.Device {
		dev := d.GetVirtualDevice()
		key := strconv.Itoa(int(dev.Key))
		label := ""
		if dev.DeviceInfo != nil {
			label = dev.DeviceInfo.GetDescription().Label
		}
		switch x := d.(type) {
		case *types.VirtualDisk:
			disk := map[string]any{"label": label, "type": "SCSI", "capacity": x.CapacityInBytes,
				"backing": map[string]any{"type": "VMDK_FILE"}}
			if fb, ok := x.Backing.(types.BaseVirtualDeviceFileBackingInfo); ok {
				disk["backing"].(map[string]any)["vmdk_file"] = fb.GetVirtualDeviceFileBackingInfo().FileName
			}
			if ctl, ok := controllers[x.ControllerKey]; ok {
				addr := map[string]any{"bus": ctl.GetVirtualDevice().Key % 100, "unit": 0}
				if x.UnitNumber != nil {
					addr["unit"] = *x.UnitNumber
				}
				switch ctl.(type) {
				case *types.VirtualIDEController:
					disk["type"] = "IDE"
					disk["ide"] = map[string]any{"primary": addr["bus"] == int32(0), "master": addr["unit"] == int32(0)}
				case *types.VirtualAHCIController:
					disk["type"] = "SATA"
					disk["sata"] = addr
				case *types.VirtualNVMEController:
					disk["type"] = "NVME"
					disk["nvme"] = addr
				default:
					disk["scsi"] = addr
				}
			}
			disks[key] = disk
		case types.BaseVirtualEthernetCard:
			card := x.GetVirtualEthernetCard()
			nic := map[string]any{"label": label, "type": nicType(d), "mac_type": "ASSIGNED", "mac_address": card.MacAddress,
				"start_connected":     card.Connectable != nil && card.Connectable.StartConnected,
				"allow_guest_control": card.Connectable != nil && card.Connectable.AllowGuestControl,
				"wake_on_lan_enabled": card.WakeOnLanEnabled != nil && *card.WakeOnLanEnabled,
				"state":               "NOT_CONNECTED"}
			if strings.EqualFold(card.AddressType, "manual") {
				nic["mac_type"] = "MANUAL"
			} else if strings.EqualFold(card.AddressType, "generated") {
				nic["mac_type"] = "GENERATED"
			}
			if card.Connectable != nil && card.Connectable.Connected {
				nic["state"] = "CONNECTED"
			}
			backing := map[string]any{"type": "STANDARD_PORTGROUP"}
			switch b := card.Backing.(type) {
			case *types.VirtualEthernetCardNetworkBackingInfo:
				backing["network_name"] = b.DeviceName
				if b.Network != nil {
					backing["network"] = b.Network.Value
				}
			case *types.VirtualEthernetCardDistributedVirtualPortBackingInfo:
				backing["type"] = "DISTRIBUTED_PORTGROUP"
				backing["distributed_switch_uuid"] = b.Port.SwitchUuid
				backing["network"] = b.Port.PortgroupKey
			case *types.VirtualEthernetCardOpaqueNetworkBackingInfo:
				backing["type"] = "OPAQUE_NETWORK"
				backing["opaque_network_id"] = b.OpaqueNetworkId
				backing["opaque_network_type"] = b.OpaqueNetworkType
			}
			nic["backing"] = backing
			nics[key] = nic
		}
	}
	out["disks"], out["nics"] = disks, nics
	return out
}

func nicType(d types.BaseVirtualDevice) string {
	switch d.(type) {
	case *types.VirtualE1000:
		return "E1000"
	case *types.VirtualE1000e:
		return "E1000E"
	case *types.VirtualPCNet32:
		return "PCNET32"
	case *types.VirtualVmxnet2:
		return "VMXNET2"
	case *types.VirtualVmxnet3:
		return "VMXNET3"
	default:
		return "VMXNET"
	}
}

var hostConnection = map[types.HostSystemConnectionState]string{
	types.HostSystemConnectionStateConnected: "CONNECTED", types.HostSystemConnectionStateDisconnected: "DISCONNECTED",
	types.HostSystemConnectionStateNotResponding: "NOT_RESPONDING",
}

var hostPower = map[types.HostSystemPowerState]string{
	types.HostSystemPowerStatePoweredOn: "POWERED_ON", types.HostSystemPowerStatePoweredOff: "POWERED_OFF",
	types.HostSystemPowerStateStandBy: "STANDBY",
}

// inventorySummary → Datacenter·Cluster·Host·Datastore Summary.
func inventorySummary(o mo.Reference) map[string]any {
	ref := o.Reference()
	key := map[string]string{"Datacenter": "datacenter", "ClusterComputeResource": "cluster", "HostSystem": "host", "Datastore": "datastore"}[ref.Type]
	out := map[string]any{key: ref.Value}
	if e, ok := o.(mo.Entity); ok {
		out["name"] = e.Entity().Name
	}
	switch x := o.(type) {
	case *simulator.HostSystem:
		out["connection_state"] = hostConnection[x.Runtime.ConnectionState]
		if p, ok := hostPower[x.Runtime.PowerState]; ok {
			out["power_state"] = p
		}
	case *simulator.Datastore:
		out["type"] = strings.ToUpper(x.Summary.Type)
		out["capacity"] = x.Summary.Capacity
		out["free_space"] = x.Summary.FreeSpace
	case *simulator.ClusterComputeResource:
		out["drs_enabled"], out["ha_enabled"] = false, false
		if c, ok := x.ConfigurationEx.(*types.ClusterConfigInfoEx); ok {
			out["drs_enabled"] = c.DrsConfig.Enabled != nil && *c.DrsConfig.Enabled
			out["ha_enabled"] = c.DasConfig.Enabled != nil && *c.DasConfig.Enabled
		}
	}
	return out
}

// networkSummary → Network.Summary.
func networkSummary(o mo.Reference) map[string]any {
	ref := o.Reference()
	typ := map[string]string{"Network": "STANDARD_PORTGROUP", "DistributedVirtualPortgroup": "DISTRIBUTED_PORTGROUP", "OpaqueNetwork": "OPAQUE_NETWORK"}[ref.Type]
	out := map[string]any{"network": ref.Value, "type": typ}
	if e, ok := o.(mo.Entity); ok {
		out["name"] = e.Entity().Name
	}
	return out
}

// deleteRESTSession은 vAPI 세션 저장소에서 세션을 지운다(만료 처리).
func deleteRESTSession(m *simulator.Model, id string) {
	req := httptest.NewRequest(http.MethodDelete, legacySession, nil)
	req.Header.Set("vmware-api-session-id", id)
	m.Service.ServeMux.ServeHTTP(httptest.NewRecorder(), req)
}
