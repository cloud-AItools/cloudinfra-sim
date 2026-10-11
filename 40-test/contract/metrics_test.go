// 가상 메트릭 (Phase 3 WP 3.7: FR-MET-0101 ~ 0104, FR-BE-0704, NFR-0507).
package contract

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// 추적: T-FR-MET-0101 T-FR-MET-0102 T-FR-MET-0103 T-FR-MET-0104 T-FR-BE-0704 T-NFR-0507
func TestVirtualMetrics(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	pm := performance.NewManager(c.Client)
	counters, err := pm.CounterInfoByName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	end := time.Now().Truncate(20 * time.Second)
	query := func(ref types.ManagedObjectReference, names []string, samples int32) map[string][]int64 {
		t.Helper()
		var ids []types.PerfMetricId
		for _, n := range names {
			ids = append(ids, types.PerfMetricId{CounterId: counters[n].Key, Instance: ""})
		}
		res, err := methods.QueryPerf(ctx, c.Client, &types.QueryPerf{This: *c.ServiceContent.PerfManager, QuerySpec: []types.PerfQuerySpec{{
			Entity: ref, MetricId: ids, IntervalId: 20, MaxSample: samples, EndTime: &end,
		}}})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][]int64{}
		em := res.Returnval[0].(*types.PerfEntityMetric)
		for i, s := range em.Value {
			out[names[i]] = s.(*types.PerfMetricIntSeries).Value
		}
		return out
	}
	// 켜진 VM 하나와 꺼진 VM 하나
	var on, off *mo.VirtualMachine
	for _, v := range vms {
		var m mo.VirtualMachine
		_ = v.Properties(ctx, v.Reference(), []string{"runtime", "config.hardware"}, &m)
		mm := m
		if m.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn && on == nil {
			on = &mm
		} else if m.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOff && off == nil {
			off = &mm
		}
	}
	if off == nil {
		tk, _ := vms[len(vms)-1].PowerOff(ctx)
		_ = tk.Wait(ctx)
		var m mo.VirtualMachine
		_ = vms[len(vms)-1].Properties(ctx, vms[len(vms)-1].Reference(), []string{"runtime", "config.hardware"}, &m)
		off = &m
	}
	names := []string{"cpu.usage.average", "cpu.usagemhz.average", "mem.consumed.average", "mem.usage.average"}
	a := query(on.Self, names, 180)
	b := query(on.Self, names, 180)
	// 결정론(FR-MET-0104): 같은 조회는 같은 값
	for _, n := range names {
		if strings.Join(fmtInts(a[n]), ",") != strings.Join(fmtInts(b[n]), ",") {
			t.Fatalf("%s not deterministic", n)
		}
	}
	// 시간 변동(FR-MET-0103): 1시간 표본이 한 값이 아니다
	distinct := map[int64]bool{}
	for _, x := range a["cpu.usagemhz.average"] {
		distinct[x] = true
	}
	if len(distinct) < 5 {
		t.Fatalf("time variation: %d distinct values", len(distinct))
	}
	// 할당량 정합(FR-BE-0704): 사용량 ≤ 할당
	var host mo.HostSystem
	_ = c.RetrieveOne(ctx, *on.Runtime.Host, []string{"summary.hardware", "vm"}, &host)
	maxMhz := int64(on.Config.Hardware.NumCPU) * int64(host.Summary.Hardware.CpuMhz)
	for i := range a["cpu.usagemhz.average"] {
		if mhz := a["cpu.usagemhz.average"][i]; mhz < 0 || mhz > maxMhz || a["cpu.usage.average"][i] > 10000 ||
			a["mem.consumed.average"][i] > int64(on.Config.Hardware.MemoryMB)*1024 {
			t.Fatalf("usage exceeds allocation at %d: %v", i, a)
		}
	}
	// 꺼진 VM은 실시간 표본 없음(-1, 가정)
	for _, x := range query(off.Self, names, 3)["cpu.usagemhz.average"] {
		if x != -1 {
			t.Fatalf("powered-off VM usage %d", x)
		}
	}
	// 호스트 = 켜진 VM 합 + 오버헤드(용량의 1%), 용량 이하(같은 표본 시각)
	hv := query(host.Self, []string{"cpu.usagemhz.average"}, 1)["cpu.usagemhz.average"][0]
	var sum int64
	for _, r := range host.Vm {
		q := query(r, []string{"cpu.usagemhz.average"}, 1)
		if x := q["cpu.usagemhz.average"][0]; x > 0 {
			sum += x
		}
	}
	capMhz := int64(host.Summary.Hardware.NumCpuCores) * int64(host.Summary.Hardware.CpuMhz)
	if hv != min(sum+capMhz/100, capMhz) {
		t.Fatalf("host usage %d != sum of VMs %d + overhead %d", hv, sum, capMhz/100)
	}
	// 덮어쓰기(특정 값)와 결측(-1, 수집 실패)
	put := func(body string) int {
		res, _ := e.AdminDo(http.MethodPut, "/v1/metrics/overrides", strings.NewReader(body))
		res.Body.Close()
		return res.StatusCode
	}
	if put(`{"entity":"`+on.Self.Value+`","counter":"cpu.usage.average","value":9500}`) != 200 ||
		put(`{"entity":"`+on.Self.Value+`","counter":"mem.usage.average","missing":true}`) != 200 {
		t.Fatal("override")
	}
	if put(`{"entity":"x","counter":"cpu"}`) != 400 {
		t.Fatal("invalid override must be rejected")
	}
	o := query(on.Self, []string{"cpu.usage.average", "mem.usage.average"}, 2)
	if o["cpu.usage.average"][0] != 9500 || o["mem.usage.average"][0] != -1 {
		t.Fatalf("override values: %v", o)
	}
	res, _ := e.AdminDo(http.MethodDelete, "/v1/metrics/overrides", nil)
	res.Body.Close()
	if x := query(on.Self, []string{"cpu.usage.average"}, 1)["cpu.usage.average"][0]; x == 9500 {
		t.Fatal("override must be cleared")
	}
}

func fmtInts(xs []int64) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = time.Duration(x).String()
	}
	return out
}
