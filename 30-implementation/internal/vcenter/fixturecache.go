package vcenter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/vmware/govmomi/simulator"
)

// Fixture 캐시(R-45 ②): vcsim은 Fixture VM을 하나씩 만들며 VM마다 데이터스토어 파일(.vmx·.vmdk 등)을 만든다.
// Windows에서는 이 파일 작업이 VM당 약 40 ms라 VM 5,000개 기동이 200초를 넘는다. 처음 만든 인벤토리를 Snapshot
// 형식으로 데이터 디렉터리에 저장해 두고, 같은 Fixture·프로파일·실행파일로 다시 기동하거나 Reset하면 vcsim
// Model.Load로 적재한다(파일 복사·읽기는 병렬).
//
// 같은 상태를 보장하는 범위: 인벤토리 객체·속성(UUID 등 Namespace 기반 값 포함), 데이터스토어 파일, MoRef 일련번호
// (이후 새 객체의 MoRef가 캐시 없이 만든 경우와 같다). 다른 점: vcsim 이벤트 기록(Fixture 생성 이벤트)은 저장되지
// 않아 캐시로 적재하면 비어 있다(BEHAVIOR_SPEC §14).

// FixtureCache는 캐시 설정이다. nil이면 쓰지 않는다.
type FixtureCache struct {
	Dir    string // 저장 위치(<data-dir>/fixture-cache)
	MinVMs int    // 이 수 이상의 VM을 만드는 Fixture에만 쓴다(0 = 항상)
	Keep   int    // 남길 캐시 수(오래된 것부터 지움). 0이면 2
}

// Option은 New의 선택 설정이다.
type Option func(*VCenter)

// WithFixtureCache는 Fixture 캐시를 켠다.
func WithFixtureCache(c *FixtureCache) Option { return func(v *VCenter) { v.cache = c } }

// CacheStatus는 마지막 Reset이 캐시를 어떻게 썼는지다(hit | miss | off)와 걸린 시간이다.
type CacheStatus struct {
	Mode     string `json:"mode"`
	Key      string `json:"key,omitempty"`
	Duration string `json:"duration"`
	Error    string `json:"error,omitempty"`
}

func (v *VCenter) LastCache() CacheStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.cacheStatus
}

// fixtureVMs는 Fixture가 만드는 VM 수다: 데이터센터마다 (독립 호스트 + 클러스터마다 (하위 풀 또는 루트 풀 1 + vApp))
// × vms_per_resource_pool(placement.go와 같은 구성).
func (v *VCenter) fixtureVMs() int {
	f := v.fix.VCenter
	return f.Datacenters * (f.StandaloneHosts + f.Clusters*(max(f.ChildPools, 1)+f.VApps)) * f.VMsPerResourcePool
}

func (v *VCenter) cacheEnabled() bool {
	return v.cache != nil && v.cache.Dir != "" && v.fixtureVMs() >= v.cache.MinVMs
}

var exeHash = sync.OnceValue(func() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
})

// cacheKey는 인벤토리를 결정하는 입력의 해시다: Fixture 전체, vCenter 프로파일, 실행파일(구현·vcsim 버전).
func (v *VCenter) cacheKey() (string, error) {
	exe := exeHash()
	if exe == "" {
		return "", fmt.Errorf("실행파일 해시를 구하지 못했다")
	}
	fx, err := json.Marshal(v.fix)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "fixture-cache-v1\n%s\n%s\n%s\n", fx, v.prof.VCHash, exe)
	return v.fix.Name + "-" + hex.EncodeToString(h.Sum(nil))[:16], nil
}

// resetFromCache는 캐시가 있으면 적재해 활성화한다. 없거나 쓸 수 없으면 false.
func (v *VCenter) resetFromCache() (bool, CacheStatus) {
	st := CacheStatus{Mode: "miss"}
	key, err := v.cacheKey()
	if err != nil {
		st.Error = err.Error()
		return false, st
	}
	st.Key = key
	dir := filepath.Join(v.cache.Dir, key)
	if _, err := os.Stat(filepath.Join(dir, "vcenter.json")); err != nil {
		return false, st
	}
	m, meta, err := v.loadModel(dir)
	if err != nil {
		v.log.Warn("Fixture 캐시를 적재하지 못해 새로 만든다", "cache", dir, "err", err)
		_ = os.RemoveAll(dir)
		st.Error = err.Error()
		return false, st
	}
	v.pbmImport(meta.PBM)
	if err := v.activate(m, v.Now().UTC()); err != nil { // 환경 생성 시각은 지금(Create와 같음)
		st.Error = err.Error()
		return false, st
	}
	_ = os.Chtimes(dir, time.Now(), time.Now()) // 최근 사용 표시(정리 순서)
	st.Mode = "hit"
	return true, st
}

// saveCache는 방금 만든 인벤토리를 캐시로 저장한다. 실패해도 기동은 계속한다(다음에 다시 만든다).
func (v *VCenter) saveCache(key string) error {
	if err := os.MkdirAll(v.cache.Dir, 0o700); err != nil {
		return err
	}
	dir := filepath.Join(v.cache.Dir, key)
	tmp := fmt.Sprintf("%s.tmp-%d", dir, os.Getpid())
	_ = os.RemoveAll(tmp)
	if err := v.SaveSnapshot(tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	_ = os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	v.pruneCache(key)
	return nil
}

// pruneCache는 최근에 쓴 Keep개만 남긴다(지금 쓰는 key는 항상 남긴다).
func (v *VCenter) pruneCache(current string) {
	keep := v.cache.Keep
	if keep <= 0 {
		keep = 2
	}
	ents, err := os.ReadDir(v.cache.Dir)
	if err != nil {
		return
	}
	type ent struct {
		name string
		mod  time.Time
	}
	var list []ent
	for _, e := range ents {
		if !e.IsDir() || e.Name() == current {
			continue
		}
		if info, err := e.Info(); err == nil {
			list = append(list, ent{e.Name(), info.ModTime()})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
	for i, e := range list {
		if i >= keep-1 {
			_ = os.RemoveAll(filepath.Join(v.cache.Dir, e.name))
		}
	}
}

// registryCounter·setRegistryCounter는 vcsim Registry의 MoRef 일련번호(비공개 필드 counter)를 읽고 쓴다.
// vcsim에 공개 API가 없어 reflect로 접근한다(govmomi v0.56.0). 필드가 없어지면 0/무시 — 결정론 시험이 잡는다.
func counterField(r *simulator.Registry) *int64 {
	f := reflect.ValueOf(r).Elem().FieldByName("counter")
	if !f.IsValid() || f.Kind() != reflect.Int64 || !f.CanAddr() {
		return nil
	}
	return (*int64)(unsafe.Pointer(f.UnsafeAddr()))
}

func registryCounter(r *simulator.Registry) int64 {
	if p := counterField(r); p != nil {
		return atomic.LoadInt64(p)
	}
	return 0
}

// setRegistryCounter는 일련번호를 n으로 둔다(작아질 수도 있다). 캐시 없이 만든 모델도 고정 ID 객체보다 작은 일련번호로
// 시작하므로 같은 값으로 두면 이후 번호 부여(충돌 가능성 포함)가 캐시 없이 만든 경우와 똑같다.
func setRegistryCounter(r *simulator.Registry, n int64) {
	if p := counterField(r); p != nil {
		atomic.StoreInt64(p, n)
	}
}
