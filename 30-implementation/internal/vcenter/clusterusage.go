package vcenter

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
)

// vcsim이 구현하지 않는 메서드를 Adapter가 직접 답한다. vcsim은 로그인 세션 요청에서 Registry.Handler가 돌려준
// 대체 객체를 무시하고(세션 레지스트리 조회가 우선), 클러스터 객체는 vcsim 내부 타입 단언 때문에 래퍼로 바꿀 수 없다.
// 그래서 Handler가 결과를 미리 만들어 두고 표지 Fault(adapterResultMarker+번호)를 돌려주면, soap()이 그 응답을 정상
// <Method>Response로 바꾼다(실패 Task 표지 rewriteAdapterFault와 같은 경로).

const adapterResultMarker = "cloudinfra-sim-adapter-result:"

var resultRe = regexp.MustCompile(regexp.QuoteMeta(adapterResultMarker) + `([0-9]+)`)

var (
	adapterResults sync.Map // 번호 → returnval XML(응답을 쓰면 지운다)
	adapterSeq     atomic.Int64
)

// answer는 returnval을 XML로 만들어 두고 표지 Fault를 돌려준다.
func answer(returnval any) types.BaseMethodFault {
	b, err := xml.Marshal(returnval)
	if err != nil {
		return &types.SystemError{Reason: err.Error()}
	}
	n := adapterSeq.Add(1)
	adapterResults.Store(n, b)
	return &types.SystemError{Reason: fmt.Sprintf("%s%d", adapterResultMarker, n)}
}

// rewriteAdapterResult는 표지 Fault 응답을 미리 만든 결과의 정상 응답으로 바꾼다. 해당 없으면 false.
func rewriteAdapterResult(w http.ResponseWriter, method string, body []byte) bool {
	m := resultRe.FindSubmatch(body)
	if m == nil {
		return false
	}
	var n int64
	fmt.Sscan(string(m[1]), &n)
	b, ok := adapterResults.LoadAndDelete(n)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<soapenv:Envelope xmlns:soapenc="http://schemas.xmlsoap.org/soap/encoding/" xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`+
		`<soapenv:Body><%sResponse xmlns="urn:vim25">%s</%sResponse></soapenv:Body></soapenv:Envelope>`,
		method, b, method)
	return true
}

// unimplemented는 vcsim 미구현 메서드를 처리한다. 처리했으면 (표지 Fault, true).
func unimplemented(ctx *simulator.Context, method *simulator.Method) (types.BaseMethodFault, bool) {
	switch method.Name {
	case "GetResourceUsage":
		// ClusterComputeResource.GetResourceUsage(vSphere 6.0 API). community.vmware vmware_cluster_info가 부른다
		// (TR-COV-0306 실행에서 발견). 클러스터가 아니면 vcsim 기본 처리(MethodNotFound).
		if c, ok := ctx.Map.Get(method.This).(*simulator.ClusterComputeResource); ok {
			return answer(struct {
				XMLName xml.Name `xml:"returnval"`
				types.ClusterResourceUsageSummary
			}{ClusterResourceUsageSummary: clusterUsage(ctx, c)}), true
		}
	}
	return nil, false
}

// clusterUsage: 용량은 클러스터 요약(소속 호스트 합), 사용량은 호스트 quickStats 합, 스토리지는 클러스터
// 데이터스토어 요약(같은 데이터스토어는 한 번)으로 계산한다. 계산 방식은 가정(실제 vCenter는 DRS 통계 기반).
func clusterUsage(ctx *simulator.Context, c *simulator.ClusterComputeResource) types.ClusterResourceUsageSummary {
	var r types.ClusterResourceUsageSummary
	var hosts, dss []types.ManagedObjectReference
	ctx.WithLock(c, func() {
		if s, ok := c.Summary.(*types.ClusterComputeResourceSummary); ok && s != nil {
			r.CpuCapacityMHz = s.TotalCpu
			r.MemCapacityMB = int32(s.TotalMemory >> 20)
		}
		hosts = append(hosts, c.Host...)
		dss = append(dss, c.Datastore...)
	})
	for _, ref := range hosts {
		if h, ok := ctx.Map.Get(ref).(*simulator.HostSystem); ok {
			ctx.WithLock(h, func() {
				r.CpuUsedMHz += h.Summary.QuickStats.OverallCpuUsage
				r.MemUsedMB += h.Summary.QuickStats.OverallMemoryUsage
			})
		}
	}
	seen := map[string]bool{}
	for _, ref := range dss {
		if seen[ref.Value] {
			continue
		}
		seen[ref.Value] = true
		if ds, ok := ctx.Map.Get(ref).(*simulator.Datastore); ok {
			ctx.WithLock(ds, func() {
				r.StorageCapacityMB += ds.Summary.Capacity >> 20
				r.StorageUsedMB += (ds.Summary.Capacity - ds.Summary.FreeSpace) >> 20
			})
		}
	}
	return r
}
