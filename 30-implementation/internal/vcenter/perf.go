package vcenter

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// 가상 메트릭(Phase 3 WP 3.7: FR-MET-0101 ~ 0104, FR-BE-0704, NFR-0507).
//
// vcsim QueryPerf는 표본 데이터에 무작위 잡음을 더해 재현되지 않고 할당량과도 무관하다. 여기서는 QueryPerf만
// Registry.Handler로 바꿔 조회 시점에 결정론적으로 계산한다: 값 = f(Namespace·Seed, 엔터티, 카운터, 표본 시각).
//   - 사용률 u(t) = 기준(엔터티별 0.15~0.55) + 0.1·sin(2π·t/1h + 위상): 시간에 따른 변동·패턴(FR-MET-0103)
//   - VM: 꺼져 있으면 표본 없음(-1). CPU MHz = u·vCPU·호스트 코어 MHz, 메모리 = u·할당 메모리(FR-BE-0704: 할당을 넘지 않음)
//   - Host: 켜진 VM 합 + 오버헤드 1%(용량으로 상한), Cluster: Host 합
//   - Override(Admin API): 값 고정 또는 결측(-1, 수집 실패)만 메모리에 저장한다(시계열은 저장 안 함, FR-MET-0104)
// 지원 카운터 밖은 -1(결측)이다. 단위는 vSphere 카운터 정의(percent는 1/100 단위)를 따른다.

// MetricOverride는 엔터티·카운터의 값 덮어쓰기다(FR-MET-0103 특정 사용량 변경·결측).
type MetricOverride struct {
	Entity  string `json:"entity"`  // MoRef 값(예: vm-57)
	Counter string `json:"counter"` // group.name.rollup(예: cpu.usage.average)
	Value   *int64 `json:"value,omitempty"`
	Missing bool   `json:"missing,omitempty"`
}

type metrics struct {
	mu        sync.Mutex
	overrides map[string]MetricOverride // entity|counter
}

func (m *metrics) get(entity, counter string) (MetricOverride, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.overrides[entity+"|"+counter]
	return o, ok
}

// SetMetricOverride는 덮어쓰기를 추가한다. value와 missing 중 하나가 필요하다.
func (v *VCenter) SetMetricOverride(o MetricOverride) error {
	if o.Entity == "" || strings.Count(o.Counter, ".") != 2 || (o.Value == nil) == !o.Missing {
		return fmt.Errorf("entity, counter(group.name.rollup)와 value 또는 missing 중 하나가 필요하다")
	}
	if o.Value != nil && *o.Value < 0 {
		return fmt.Errorf("value는 0 이상(결측은 missing)")
	}
	v.perf.mu.Lock()
	v.perf.overrides[o.Entity+"|"+o.Counter] = o
	v.perf.mu.Unlock()
	return nil
}

// MetricOverrides는 현재 덮어쓰기 목록이다.
func (v *VCenter) MetricOverrides() []MetricOverride {
	v.perf.mu.Lock()
	defer v.perf.mu.Unlock()
	out := make([]MetricOverride, 0, len(v.perf.overrides))
	for _, o := range v.perf.overrides {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Entity+out[i].Counter < out[j].Entity+out[j].Counter })
	return out
}

// ClearMetricOverrides는 덮어쓰기를 모두 지운다(환경 초기화 포함).
func (v *VCenter) ClearMetricOverrides() {
	v.perf.mu.Lock()
	v.perf.overrides = map[string]MetricOverride{}
	v.perf.mu.Unlock()
}

// perfManager는 vcsim PerformanceManager를 감싸 QueryPerf만 바꾼다. 레지스트리의 같은 MoRef 자리에 넣으므로
// 다른 메서드·속성(perfCounter 등)은 내장된 vcsim 객체 그대로다(Registry.Handler 대체는 로그인 세션에서 무시되므로).
type perfManager struct {
	*simulator.PerformanceManager
	v *VCenter
}

func installPerf(v *VCenter, m *simulator.Model) {
	if pm, ok := m.Map().Get(*m.ServiceContent.PerfManager).(*simulator.PerformanceManager); ok {
		m.Map().Put(&perfManager{PerformanceManager: pm, v: v})
	}
}

func counterName(c types.PerfCounterInfo) string {
	return c.GroupInfo.GetElementDescription().Key + "." + c.NameInfo.GetElementDescription().Key + "." + string(c.RollupType)
}

func (h *perfManager) QueryPerf(ctx *simulator.Context, req *types.QueryPerf) soap.HasFault {
	body := &methods.QueryPerfBody{Res: &types.QueryPerfResponse{}}
	names := map[int32]string{}
	for _, c := range h.PerfCounter {
		names[c.Key] = counterName(c)
	}
	for _, qs := range req.QuerySpec {
		if ctx.Map.Get(qs.Entity) == nil {
			body.Fault_ = simulator.Fault("", &types.ManagedObjectNotFound{Obj: qs.Entity})
			return body
		}
		interval := qs.IntervalId
		if interval <= 0 {
			interval = 20 // 실시간 표본 간격
		}
		end := h.v.Now() // 가상 시각 기준 통계(FR-BE-0608)
		if qs.EndTime != nil {
			end = *qs.EndTime
		}
		end = end.Truncate(time.Duration(interval) * time.Second) // 표본 시각을 간격 배수에 맞춘다(재현)
		start := end.Add(-time.Hour)
		if qs.StartTime != nil {
			start = *qs.StartTime
		}
		n := int32(end.Sub(start).Seconds())/interval + 1
		if n < 1 {
			n = 1
		}
		if qs.MaxSample > 0 && n > qs.MaxSample {
			n = qs.MaxSample
		}
		em := &types.PerfEntityMetric{PerfEntityMetricBase: types.PerfEntityMetricBase{Entity: qs.Entity}}
		for i := n - 1; i >= 0; i-- { // 오래된 순
			em.SampleInfo = append(em.SampleInfo, types.PerfSampleInfo{Timestamp: end.Add(-time.Duration(interval*i) * time.Second), Interval: interval})
		}
		for _, mid := range qs.MetricId {
			s := &types.PerfMetricIntSeries{PerfMetricSeries: types.PerfMetricSeries{Id: mid}}
			for _, si := range em.SampleInfo {
				s.Value = append(s.Value, h.v.metricValue(ctx, qs.Entity, names[mid.CounterId], si.Timestamp))
			}
			em.Value = append(em.Value, s)
		}
		body.Res.Returnval = append(body.Res.Returnval, em)
	}
	return body
}

func (v *VCenter) hash01(parts ...string) float64 {
	s := fmt.Sprintf("%s/%d/%s", v.fix.Namespace, v.fix.Seed, strings.Join(parts, "/"))
	var x uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		x ^= uint64(s[i])
		x *= 1099511628211
	}
	return float64(x%1000003) / 1000003
}

// util은 엔터티·자원의 시각 t 사용률(0~1)이다.
func (v *VCenter) util(entity, res string, t time.Time) float64 {
	base := 0.15 + 0.4*v.hash01(entity, res, "base")
	phase := 2 * math.Pi * v.hash01(entity, res, "phase")
	u := base + 0.1*math.Sin(2*math.Pi*float64(t.Unix()%3600)/3600+phase)
	return math.Max(0, math.Min(1, u))
}

// metricValue는 카운터 값이다. 덮어쓰기가 있으면 그 값(결측은 -1).
func (v *VCenter) metricValue(ctx *simulator.Context, ref types.ManagedObjectReference, counter string, t time.Time) int64 {
	if o, ok := v.perf.get(ref.Value, counter); ok {
		if o.Missing {
			return -1
		}
		return *o.Value
	}
	switch obj := ctx.Map.Get(ref).(type) {
	case *simulator.VirtualMachine:
		return v.vmMetric(ctx, obj, counter, t, true)
	case *simulator.HostSystem:
		return v.hostMetric(ctx, obj, counter, t)
	case *simulator.ClusterComputeResource:
		var sum int64
		percent := strings.HasSuffix(counter, "usage.average") && (strings.HasPrefix(counter, "cpu.") || strings.HasPrefix(counter, "mem."))
		var hosts []types.ManagedObjectReference
		ctx.WithLock(obj, func() { hosts = append(hosts, obj.Host...) })
		for _, hr := range hosts {
			if h, ok := ctx.Map.Get(hr).(*simulator.HostSystem); ok {
				sum += v.hostMetric(ctx, h, counter, t)
			}
		}
		if percent && len(hosts) > 0 {
			return sum / int64(len(hosts))
		}
		return sum
	}
	return -1
}

func (v *VCenter) hostCPUMhz(ctx *simulator.Context, ref *types.ManagedObjectReference) (mhz int64, cores int64) {
	mhz, cores = 2000, int64(v.fix.VCenter.HostCPUCores) // 기본값: 호스트 정보가 없을 때
	if ref == nil {
		return
	}
	if h, ok := ctx.Map.Get(*ref).(*simulator.HostSystem); ok {
		ctx.WithLock(h, func() {
			if hw := h.Summary.Hardware; hw != nil && hw.CpuMhz > 0 && hw.NumCpuCores > 0 {
				cores = int64(hw.NumCpuCores)
				mhz = int64(hw.CpuMhz)
			}
		})
	}
	return
}

// vmMetric은 VM 카운터 값이다. external이면 꺼진 VM은 실시간 표본이 없으므로 -1(결측, 가정)이고,
// 내부 합산(호스트)에서는 0이다. 0을 응답에 쓰지 않는 까닭: govmomi XML 인코딩은 omitempty 정수 배열의 0 원소를 빠뜨린다.
func (v *VCenter) vmMetric(ctx *simulator.Context, vm *simulator.VirtualMachine, counter string, t time.Time, external bool) int64 {
	var on bool
	var cpus, memMB int64
	var host *types.ManagedObjectReference
	ctx.WithLock(vm, func() {
		on = vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn
		if vm.Config != nil {
			cpus, memMB = int64(vm.Config.Hardware.NumCPU), int64(vm.Config.Hardware.MemoryMB)
		}
		host = vm.Runtime.Host
	})
	if !on {
		if _, ok := supported[counter]; ok {
			if external {
				return -1
			}
			return 0 // 꺼진 VM은 호스트 사용량에 더하지 않는다(FR-BE-0704)
		}
		return -1
	}
	mhz, _ := v.hostCPUMhz(ctx, host)
	id := vm.Self.Value
	switch counter {
	case "cpu.usage.average":
		return int64(math.Round(v.util(id, "cpu", t) * 10000))
	case "cpu.usagemhz.average":
		return int64(v.util(id, "cpu", t) * float64(cpus*mhz))
	case "mem.usage.average":
		return int64(math.Round(v.util(id, "mem", t) * 10000))
	case "mem.consumed.average", "mem.active.average":
		f := v.util(id, "mem", t)
		if counter == "mem.active.average" {
			f *= 0.6
		}
		return int64(f * float64(memMB*1024))
	case "net.usage.average":
		return int64(v.util(id, "net", t) * 10000) // KBps(가상 vNIC 10 Gbps 기준이 아닌 시뮬레이션 상한 10,000)
	case "disk.usage.average":
		return int64(v.util(id, "disk", t) * 5000)
	}
	return -1
}

var supported = map[string]bool{"cpu.usage.average": true, "cpu.usagemhz.average": true, "mem.usage.average": true,
	"mem.consumed.average": true, "mem.active.average": true, "net.usage.average": true, "disk.usage.average": true}

func (v *VCenter) hostMetric(ctx *simulator.Context, h *simulator.HostSystem, counter string, t time.Time) int64 {
	if !supported[counter] {
		return -1
	}
	var vms []types.ManagedObjectReference
	var memMB int64
	ctx.WithLock(h, func() {
		vms = append(vms, h.Vm...)
		if hw := h.Summary.Hardware; hw != nil {
			memMB = hw.MemorySize / (1024 * 1024)
		}
	})
	ref := h.Self
	mhz, cores := v.hostCPUMhz(ctx, &ref)
	if memMB <= 0 {
		memMB = int64(v.fix.VCenter.HostMemoryGB) * 1024
	}
	capMhz := mhz * cores
	var cpuMhz, memKB, other int64
	for _, r := range vms {
		vm, ok := ctx.Map.Get(r).(*simulator.VirtualMachine)
		if !ok {
			continue
		}
		cpuMhz += v.vmMetric(ctx, vm, "cpu.usagemhz.average", t, false)
		memKB += v.vmMetric(ctx, vm, "mem.consumed.average", t, false)
		if counter == "net.usage.average" || counter == "disk.usage.average" || counter == "mem.active.average" {
			other += v.vmMetric(ctx, vm, counter, t, false)
		}
	}
	// 하이퍼바이저 오버헤드: 용량의 1%(가정). 유휴 호스트도 0이 아니다.
	cpuMhz += capMhz / 100
	memKB += memMB * 1024 / 100
	cpuMhz, memKB = min(cpuMhz, capMhz), min(memKB, memMB*1024) // 용량 상한(FR-BE-0704)
	switch counter {
	case "cpu.usagemhz.average":
		return cpuMhz
	case "cpu.usage.average":
		return cpuMhz * 10000 / max(capMhz, 1)
	case "mem.consumed.average":
		return memKB
	case "mem.usage.average":
		return memKB * 10000 / max(memMB*1024, 1)
	}
	return other
}
