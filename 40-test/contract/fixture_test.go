// Fixture 계약: 선언형 정의와 규모 변경 (TR-SCN-0201, TR-SCN-0203, TR-SCN-0204).
package contract

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vmware/govmomi/find"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

// T-TR-SCN-0201 T-TR-SCN-0203 T-TR-SCN-0204: YAML로 규모를 바꾸면 인벤토리가 따라 바뀐다(실제 CPU·RAM 소비 없음).
func TestFixtureScaleChange(t *testing.T) {
	src := filepath.Join(testenv.Repo(), "30-implementation", "data", "fixtures", "default-small.yaml")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	y := strings.Replace(string(b), "vms_per_resource_pool: 20", "vms_per_resource_pool: 200", 1)
	y = strings.Replace(y, "hosts_per_cluster: 3", "hosts_per_cluster: 8", 1)
	y = strings.Replace(y, "name: default-small", "name: scaled", 1)
	scaled := filepath.Join(t.TempDir(), "scaled.yaml")
	if err := os.WriteFile(scaled, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	e := testenv.Start(t, func(c *config.Config) { c.FixtureFile = scaled })
	c := e.Login("")
	ctx := context.Background()
	f := find.NewFinder(c.Client, true)
	vms, _ := f.VirtualMachineList(ctx, "/*/vm/*")
	hosts, _ := f.HostSystemList(ctx, "/*/host/*/*")
	if len(vms) != 200 || len(hosts) != 8 {
		t.Fatalf("scaled fixture: vms=%d hosts=%d", len(vms), len(hosts))
	}
}
