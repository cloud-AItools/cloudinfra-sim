package fixture

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// 선택 구성(독립 호스트·하위 풀·vApp)은 vcsim 모델 값으로 옮겨지고, 생략하면 0, 음수는 거부한다.
func TestTopologyFields(t *testing.T) {
	base := "name: t\nvcenter:\n  datacenters: 1\n  clusters: 1\n  hosts_per_cluster: 2\n  vms_per_resource_pool: 2\n"
	f, err := Load(writeFixture(t, base+"  standalone_hosts: 1\n  child_pools: 2\n  vapps: 3\n  datacenter_folders: 1\n  storage_pods: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := f.Model()
	if m.Host != 1 || m.Pool != 2 || m.App != 3 || m.Folder != 1 || m.Pod != 2 || m.Machine != 2 {
		t.Fatalf("model Host=%d Pool=%d App=%d Folder=%d Pod=%d Machine=%d", m.Host, m.Pool, m.App, m.Folder, m.Pod, m.Machine)
	}
	f, err = Load(writeFixture(t, base))
	if err != nil {
		t.Fatal(err)
	}
	if m := f.Model(); m.Host != 0 || m.Pool != 0 || m.App != 0 || m.Folder != 0 || m.Pod != 0 {
		t.Fatalf("생략 시 0이어야 한다: Host=%d Pool=%d App=%d", m.Host, m.Pool, m.App)
	}
	// datacenter_folders는 0 ~ datacenters(여기서는 1)
	for _, bad := range []string{"  standalone_hosts: -1\n", "  child_pools: -1\n", "  vapps: -2\n", "  datacenter_folders: -1\n", "  datacenter_folders: 2\n", "  storage_pods: -1\n"} {
		if _, err := Load(writeFixture(t, base+bad)); err == nil {
			t.Errorf("%q를 받아들였다", bad)
		}
	}
}
