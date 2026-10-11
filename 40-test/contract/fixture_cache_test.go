// Fixture 캐시(R-45 ②): 캐시로 적재한 인벤토리는 새로 만든 것과 같아야 한다 — VM·UUID·배치, 이후 새 객체의 MoRef
// (결정론, FR-DM-0207), 불일치 검사·관계 조정(검증 모드) 결과.
package contract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/vcenter"
	"cloudinfra-sim/test/internal/testenv"
)

// vmList는 인벤토리 VM 요약(MoRef·이름·UUID·호스트·데이터스토어)이다.
func vmList(t *testing.T, e *testenv.Env) []string {
	t.Helper()
	l := snapVC(t, e)
	v, err := view.NewManager(l.c.Client).CreateContainerView(l.ctx, l.c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Destroy(context.Background())
	var vms []mo.VirtualMachine
	if err := v.Retrieve(l.ctx, []string{"VirtualMachine"}, []string{"name", "config.uuid", "config.instanceUuid", "runtime.host", "datastore"}, &vms); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, vm := range vms {
		out = append(out, fmt.Sprintf("%s %s %s %s %s %v", vm.Self.Value, vm.Name, vm.Config.Uuid, vm.Config.InstanceUuid, vm.Runtime.Host.Value, vm.Datastore))
	}
	sort.Strings(out)
	return out
}

// vmState는 vmList와 그 뒤 새로 만든 VM의 MoRef다(기본 리소스 풀이 하나인 Fixture).
func vmState(t *testing.T, e *testenv.Env) ([]string, string) {
	t.Helper()
	out := vmList(t, e)
	return out, snapVC(t, e).create("cache-probe").Value
}

func firstDiff(got, want []string) string {
	if len(got) != len(want) {
		return fmt.Sprintf("VM 수 %d, 기대 %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			return fmt.Sprintf("got %q, want %q", got[i], want[i])
		}
	}
	return ""
}

func consistent(t *testing.T, e *testenv.Env) {
	t.Helper()
	if issues := append(e.Sim.VC.Consistency(), e.Sim.NSX.Consistency()...); len(issues) > 0 {
		t.Fatalf("불일치: %v", issues)
	}
}

// T-NFR-0110 T-FR-DM-0207 T-TR-SCN-0102
func TestFixtureCache(t *testing.T) {
	t.Setenv("SIM_RELATIONS_VERIFY", "1") // 관계 조정 증분·전체 대조(차이가 나면 정리 단계가 실패시킨다)
	// 기준: 캐시 없이 만든 상태
	off := testenv.Start(t, func(c *config.Config) { c.FixtureCache = "off" })
	wantVMs, wantRef := vmState(t, off)
	if st := off.Sim.VC.LastCache(); st.Mode != "off" {
		t.Fatalf("off: cache mode %q", st.Mode)
	}

	e := testenv.Start(t, func(c *config.Config) { c.FixtureCache = "on" })
	// 기동: 캐시 없음 → 만들고 저장
	if st := e.Sim.VC.LastCache(); st.Mode != "miss" || st.Key == "" || st.Error != "" {
		t.Fatalf("첫 기동 cache = %+v, miss 기대", st)
	}
	key := e.Sim.VC.LastCache().Key
	if _, err := os.Stat(filepath.Join(e.Sim.Cfg.DataDir, "fixture-cache", key, "vcenter.json")); err != nil {
		t.Fatalf("캐시 저장본 없음: %v", err)
	}
	gotVMs, gotRef := vmState(t, e)
	// 호스트 배치까지 같다(결정론적 배치, vcenter/placement.go — 이전에는 vcsim이 무작위로 골랐다)
	if d := firstDiff(gotVMs, wantVMs); d != "" || gotRef != wantRef {
		t.Fatalf("캐시 저장 경로가 상태를 바꿨다: ref %s vs %s, %s", gotRef, wantRef, d)
	}
	// 이벤트 기록(Fixture 생성 이벤트)도 캐시에 저장해 적재 때 같은 키로 다시 게시한다(R-44)
	evBefore, err := e.Sim.VC.RecentEvents(1000, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Reset: 같은 입력 → 캐시 적재
	if err := e.Sim.Reset(); err != nil {
		t.Fatal(err)
	}
	if st := e.Sim.VC.LastCache(); st.Mode != "hit" || st.Key != key {
		t.Fatalf("Reset cache = %+v, hit 기대", st)
	}
	consistent(t, e)
	hitVMs, hitRef := vmState(t, e)
	if d := firstDiff(hitVMs, wantVMs); d != "" {
		t.Fatalf("캐시 적재 인벤토리가 다르다: %s", d)
	}
	if hitRef != wantRef {
		t.Fatalf("캐시 적재 뒤 새 VM MoRef %s, 새로 만든 경우 %s(결정론)", hitRef, wantRef)
	}
	evAfter, err := e.Sim.VC.RecentEvents(1000, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// 캐시 저장 뒤 vmState가 만든 VM(cache-probe)의 이벤트는 저장본 이후라 빠진다 → 저장 시점까지의 기록과 비교
	created := func(rs []vcenter.EventRecord) string {
		var keys []string
		for _, r := range rs {
			if r.Type == "VmCreatedEvent" && r.Name != "cache-probe" {
				keys = append(keys, fmt.Sprint(r.Key, r.Entity))
			}
		}
		return strings.Join(keys, ",")
	}
	if c := created(evAfter); c == "" || c != created(evBefore) {
		t.Fatalf("캐시 적재 뒤 생성 이벤트가 다르다: got %s, want %s", c, created(evBefore))
	}
	if r, _ := e.Sim.Rel.Last(); len(r.Errors) > 0 {
		t.Fatalf("관계 조정 오류: %v", r.Errors)
	}
	// Admin 상태에 캐시 사용 결과가 나온다
	st, _ := e.Sim.Status().(map[string]any)
	if _, ok := st["retention_usage"].(map[string]any)["fixture_cache"]; !ok {
		t.Error("retention_usage.fixture_cache 없음")
	}
}

// auto는 작은 Fixture(기본 VM 20개)에서 캐시를 쓰지 않는다. 잘못된 값은 기동 거부.
func TestFixtureCacheAutoAndValidation(t *testing.T) {
	e := testenv.Start(t, nil)
	if st := e.Sim.VC.LastCache(); st.Mode != "off" {
		t.Fatalf("auto·VM 20개: cache mode %q, off 기대", st.Mode)
	}
	if _, err := os.Stat(filepath.Join(e.Sim.Cfg.DataDir, "fixture-cache")); err == nil {
		t.Fatal("auto·작은 Fixture인데 캐시 디렉터리를 만들었다")
	}
	if _, err := config.Parse([]string{"--vcenter-version", "x", "--nsx-version", "y", "--fixture-cache", "yes"}, func(string) string { return "" }); err == nil {
		t.Fatal("--fixture-cache yes를 받아들였다")
	}
}
