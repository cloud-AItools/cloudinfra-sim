// Package fixture는 선언형 Seed(Fixture)를 읽는다(TR-SCN-0201 ~ TR-SCN-0204).
package fixture

import (
	"fmt"
	"os"

	"github.com/vmware/govmomi/simulator"
	"go.yaml.in/yaml/v3"
)

type TransportZone struct {
	ID            string `yaml:"id"`
	DisplayName   string `yaml:"display_name"`
	TransportType string `yaml:"transport_type"`
}

type Fixture struct {
	Name string `yaml:"name"`
	Seed int64  `yaml:"seed"`
	// Namespace는 환경 ID 공간이다(FR-DM-0207). 결정론적 ID(UUID 등)를 seed와 함께 만든다. 비면 name.
	Namespace string `yaml:"namespace"`
	VCenter   struct {
		Datacenters        int `yaml:"datacenters"`
		Clusters           int `yaml:"clusters"`
		HostsPerCluster    int `yaml:"hosts_per_cluster"`
		Datastores         int `yaml:"datastores"`
		Portgroups         int `yaml:"portgroups"`
		VMsPerResourcePool int `yaml:"vms_per_resource_pool"`
		// 선택 구성(기본 0): 데이터센터마다 독립 호스트, 클러스터마다 하위 리소스 풀(있으면 VM은 루트 풀 대신 하위 풀에),
		// 클러스터마다 vApp. 각 풀·vApp·독립 호스트에 vms_per_resource_pool개 VM(vcsim Model.Host·Pool·App).
		StandaloneHosts int `yaml:"standalone_hosts"`
		ChildPools      int `yaml:"child_pools"`
		VApps           int `yaml:"vapps"`
		// DatacenterFolders: 마지막 N개 데이터센터를 루트 아래 폴더 F0..에 두고, 그 자원(호스트·네트워크·VM·데이터스토어)도
		// 각 기본 폴더의 하위 폴더 F<i>에 둔다(vcsim Model.Folder). 0 ~ datacenters.
		DatacenterFolders int `yaml:"datacenter_folders"`
		// StoragePods: 데이터센터마다 빈 스토리지 Pod(Datastore Cluster, Storage DRS 켜짐) DCn_PODk를 데이터센터의 기본
		// datastore 폴더에 만든다(vcsim Model.Pod). 데이터스토어는 넣지 않는다 — 클라이언트가 MoveIntoFolder로 옮길 수 있다.
		StoragePods  int `yaml:"storage_pods"`
		HostCPUCores int `yaml:"host_cpu_cores"`
		HostMemoryGB int `yaml:"host_memory_gb"`
	} `yaml:"vcenter"`
	NSX struct {
		TransportZones   []TransportZone `yaml:"transport_zones"`
		Tier1s           []string        `yaml:"tier1s"`
		Segments         int             `yaml:"segments"`
		Groups           int             `yaml:"groups"`
		SecurityPolicies int             `yaml:"security_policies"`
		Tier0s           []string        `yaml:"tier0s"`
		EdgeClusters     []string        `yaml:"edge_clusters"` // Edge Cluster 표시 이름(ID는 Namespace·Seed 기반 UUID)
		// VDSTransportZones는 NSX가 준비한(VDS 7.0+) 첫 번째 VDS에 연결된 Transport Zone이다(D8, FR-NSX-0603·0607).
		// 이 TZ의 Segment만 vCenter에 NSX DVPG(backingType=nsx)로 나타난다. 비면 vCenter 네트워크 매핑 없음.
		VDSTransportZones []string `yaml:"vds_transport_zones"`
	} `yaml:"nsx"`
}

func Load(path string) (*Fixture, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Fixture
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("fixture %s: %w", path, err)
	}
	if f.Namespace == "" {
		f.Namespace = f.Name
	}
	if f.VCenter.Datacenters < 1 || f.VCenter.Clusters < 1 || f.VCenter.HostsPerCluster < 1 {
		return nil, fmt.Errorf("fixture %s: datacenters·clusters·hosts_per_cluster는 1 이상", path)
	}
	if f.VCenter.StandaloneHosts < 0 || f.VCenter.ChildPools < 0 || f.VCenter.VApps < 0 || f.VCenter.VMsPerResourcePool < 0 {
		return nil, fmt.Errorf("fixture %s: standalone_hosts·child_pools·vapps·vms_per_resource_pool은 0 이상", path)
	}
	if f.VCenter.StoragePods < 0 {
		return nil, fmt.Errorf("fixture %s: storage_pods는 0 이상", path)
	}
	if f.VCenter.DatacenterFolders < 0 || f.VCenter.DatacenterFolders > f.VCenter.Datacenters {
		return nil, fmt.Errorf("fixture %s: datacenter_folders는 0 ~ datacenters(%d)", path, f.VCenter.Datacenters)
	}
	return &f, nil
}

// Model은 vcsim VPX 모델을 Fixture 규모로 만든다. 컨테이너 기반 VM은 쓰지 않는다(FR-VC-0107).
func (f *Fixture) Model() *simulator.Model {
	m := simulator.VPX()
	m.Datacenter = f.VCenter.Datacenters
	m.Cluster = f.VCenter.Clusters
	m.ClusterHost = f.VCenter.HostsPerCluster
	m.Host = f.VCenter.StandaloneHosts
	m.Pool = f.VCenter.ChildPools
	m.App = f.VCenter.VApps
	m.Folder = f.VCenter.DatacenterFolders
	m.Pod = f.VCenter.StoragePods
	m.Datastore = f.VCenter.Datastores
	m.Portgroup = f.VCenter.Portgroups
	m.Machine = f.VCenter.VMsPerResourcePool
	return m
}
