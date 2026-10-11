package contract

import (
	"context"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// T-TR-COV-0306: 클러스터 configurationEx.vsanConfigInfo는 vSAN 미사용이어도 enabled=false로 채워진다(실제 vCenter와 같음).
// community.vmware vmware_cluster_info가 이 값을 바로 읽어 비어 있으면 실패한다(2026-10-11 Ansible 자체 테스트에서 발견).
func TestClusterVsanConfigInfo(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, err := f.DefaultDatacenter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.SetDatacenter(dc)
	folders, _ := dc.Folders(ctx)
	_, err = folders.HostFolder.CreateCluster(ctx, "vsan-check-cluster", types.ClusterConfigSpecEx{})
	if err != nil {
		t.Fatal(err)
	}
	clusters, err := f.ClusterComputeResourceList(ctx, "*")
	if err != nil || len(clusters) < 2 {
		t.Fatalf("clusters: %d %v", len(clusters), err)
	}
	for _, cl := range clusters {
		var o mo.ClusterComputeResource
		if err := c.RetrieveOne(ctx, cl.Reference(), []string{"name", "configurationEx"}, &o); err != nil {
			t.Fatal(err)
		}
		cfg, ok := o.ConfigurationEx.(*types.ClusterConfigInfoEx)
		if !ok || cfg.VsanConfigInfo == nil || cfg.VsanConfigInfo.Enabled == nil || *cfg.VsanConfigInfo.Enabled {
			t.Fatalf("%s(%s) vsanConfigInfo: %+v", o.Name, cl.Reference().Value, cfg)
		}
		if cfg.VsanConfigInfo.DefaultConfig == nil {
			t.Fatalf("%s vsanConfigInfo.defaultConfig 없음", o.Name)
		}
	}
}

// T-TR-COV-0306: ClusterComputeResource.GetResourceUsage(vSphere 6.0 API, vcsim 미구현)는 클러스터 용량·사용량 요약을
// 돌려준다. community.vmware vmware_cluster_info가 부른다(2026-10-11 Ansible 자체 테스트에서 발견).
func TestClusterGetResourceUsage(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, err := f.DefaultDatacenter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.SetDatacenter(dc)
	cl, err := f.ClusterComputeResource(ctx, "DC0_C0")
	if err != nil {
		t.Fatal(err)
	}
	res, err := methods.GetResourceUsage(ctx, c.Client, &types.GetResourceUsage{This: cl.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	u := res.Returnval
	if u.CpuCapacityMHz <= 0 || u.MemCapacityMB <= 0 || u.StorageCapacityMB <= 0 {
		t.Fatalf("용량이 비었다: %+v", u)
	}
	if u.CpuUsedMHz > u.CpuCapacityMHz || u.MemUsedMB > u.MemCapacityMB || u.StorageUsedMB > u.StorageCapacityMB || u.StorageUsedMB < 0 {
		t.Fatalf("사용량이 용량을 넘는다: %+v", u)
	}
	// 클러스터가 아닌 대상은 vcsim 기본 처리(MethodNotFound)
	if _, err := methods.GetResourceUsage(ctx, c.Client, &types.GetResourceUsage{This: dc.Reference()}); err == nil {
		t.Fatal("데이터센터 대상 GetResourceUsage가 성공했다")
	}
}
