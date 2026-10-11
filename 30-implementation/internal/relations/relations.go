// Package relations는 vCenter ↔ NSX 교차 시스템 관계를 조정(reconcile)한다(Phase 3 WP 3.1 ~ 3.3, 3.6).
//
// 방향:
//   - NSX → vCenter(FR-NSX-0603·0606·0607, D8): NSX 준비 VDS(Fixture vds_transport_zones)에 연결된 TZ의 Segment는
//     vCenter에 NSX DVPG(backingType=nsx, segmentId=Policy Path, logicalSwitchUuid=Segment unique_id)로 나타난다.
//     모든 Segment가 자동으로 vCenter 네트워크가 되는 것은 아니다(TZ가 VDS에 연결된 경우만).
//   - vCenter → NSX(FR-NSX-0602·0604·0608, FR-NSX-0226·0227): 호스트 → Discovered Node, VM → Fabric VM(external_id =
//     instanceUuid), vNIC → VIF, NSX DVPG에 연결된 vNIC → Segment Port(attachment). 파생 객체는 시스템 소유다.
//
// 실행: 조정은 멱등이며 전체를 다시 계산한다(Reconciliation, FR-DM-0402·0403). NSX 쓰기는 요청 고루틴 안에서 동기로
// 조정한다(Gateway CrossLock을 이미 쥔 상태). vcsim 변경 핸들러의 신호는 별도 고루틴이 CrossLock을 잡고 처리한다(R-15).
// vCenter 쪽 변경은 같은 프로세스의 vcsim Service.RoundTrip으로 한다(FR-BE-0406, 외부 SOAP 호출 없음).
package relations

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/fixture"
	"cloudinfra-sim/internal/nsx"
	"cloudinfra-sim/internal/store"
	"cloudinfra-sim/internal/vcenter"
)

// Locker는 교차 시스템 락(gateway.CrossLock)이다.
type Locker interface {
	Lock()
	Unlock()
}

// Report는 한 번의 조정 결과다(Admin API, 로그).
type Report struct {
	Time      time.Time         `json:"time"`
	Cause     string            `json:"cause"`
	Mapped    int               `json:"mapped_segments"`
	Created   []string          `json:"created_portgroups,omitempty"`
	Renamed   []string          `json:"renamed_portgroups,omitempty"`
	Deleted   []string          `json:"deleted_portgroups,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
	Drift     []string          `json:"drift,omitempty"` // 고칠 수 없어 남긴 불일치
	VMs       int               `json:"fabric_vms"`
	VIFs      int               `json:"vifs"`
	Ports     int               `json:"segment_ports"`
	Hosts     int               `json:"discovered_nodes"`
	Mode      string            `json:"derive_mode"`   // full | incremental(R-45)
	Refreshed int               `json:"refreshed_vms"` // 이번에 다시 조회한 VM 수

	DurationMS int64 `json:"duration_ms"`
}

type Engine struct {
	vc   *vcenter.VCenter
	nsx  *nsx.Engine
	fix  *fixture.Fixture
	lock Locker
	log  *slog.Logger
	// Audit은 관계 조정이 만든 변경을 감사 기록에 남긴다(원인 = 촉발한 요청, FR-BE-0209). nil이면 기록 안 함.
	Audit func(store.AuditEvent)

	signal chan struct{}
	close  sync.Once
	done   chan struct{}
	wg     sync.WaitGroup

	mu   sync.Mutex
	last Report
	runs int

	// 증분 조정(R-45, derive.go). dirty는 vcsim 변경 핸들러가 표시한 객체 MoRef 값이다(dirtyMu만 잡는다, R-15).
	// cache는 Reconcile 안(교차 락 보유)에서만 쓴다.
	dirtyMu sync.Mutex
	dirty   map[string]bool
	pgDirty map[string]bool // 변경 표시된 분산 포트그룹 MoRef 값
	cache   deriveCache
	pgCache pgCache
	// Verify는 증분 결과를 매번 전체 계산과 대조한다(시험용, SIM_RELATIONS_VERIFY). 차이는 Report.Errors와 Mismatches에 남는다.
	Verify     bool
	mismatches int
}

func New(vc *vcenter.VCenter, n *nsx.Engine, fix *fixture.Fixture, lock Locker, log *slog.Logger) *Engine {
	e := &Engine{vc: vc, nsx: n, fix: fix, lock: lock, log: log, signal: make(chan struct{}, 1), done: make(chan struct{}), dirty: map[string]bool{}, pgDirty: map[string]bool{}}
	n.OnChange(func(cause string) { e.Reconcile("nsx-write " + cause) }) // 요청 고루틴, CrossLock 보유
	vc.OnChange(func(ref types.ManagedObjectReference) {                 // vcsim 변경 핸들러: 표시·신호만
		e.dirtyMu.Lock()
		e.dirty[ref.Type+":"+ref.Value] = true
		if ref.Type == "DistributedVirtualPortgroup" {
			e.pgDirty[ref.Value] = true
		}
		e.dirtyMu.Unlock()
		e.kick()
	})
	e.wg.Add(1)
	go e.loop()
	return e
}

func (e *Engine) kick() {
	select {
	case e.signal <- struct{}{}:
	default:
	}
}

func (e *Engine) loop() {
	defer e.wg.Done()
	for {
		select {
		case <-e.done:
			return
		case <-e.signal:
			e.lock.Lock()
			e.Reconcile("vcenter-change")
			e.lock.Unlock()
		}
	}
}

func (e *Engine) Close() {
	e.close.Do(func() { close(e.done) })
	e.wg.Wait()
}

// Last는 마지막 조정 결과와 횟수다.
func (e *Engine) Last() (Report, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.last, e.runs
}

// Reconcile은 조정을 한 번 실행한다. 호출자는 CrossLock을 쥐고 있어야 한다.
func (e *Engine) Reconcile(cause string) Report {
	start := time.Now()
	rep := Report{Time: start.UTC(), Cause: cause, Errors: map[string]string{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := e.reconcile(ctx, &rep, fullCause(cause)); err != nil {
		rep.Errors["reconcile"] = err.Error()
		e.log.Error("relations reconcile failed", "cause", cause, "err", err)
	}
	rep.DurationMS = time.Since(start).Milliseconds()
	if len(rep.Errors) == 0 {
		rep.Errors = nil
	}
	e.mu.Lock()
	e.last, e.runs = rep, e.runs+1
	e.mu.Unlock()
	if e.Audit != nil {
		for _, x := range []struct {
			action string
			paths  []string
		}{{"create NSX portgroup", rep.Created}, {"rename NSX portgroup", rep.Renamed}, {"delete NSX portgroup", rep.Deleted}} {
			for _, p := range x.paths {
				e.Audit(store.AuditEvent{Product: "vcenter", Principal: "system", Action: "relations " + x.action, Target: p, Cause: "relations " + cause})
			}
		}
		for p, msg := range rep.Errors {
			e.Audit(store.AuditEvent{Product: "nsx", Principal: "system", Action: "relations sync failed", Target: p + ": " + msg, Status: 500, Cause: "relations " + cause})
		}
	}
	if len(rep.Created)+len(rep.Renamed)+len(rep.Deleted)+len(rep.Errors)+len(rep.Drift) > 0 {
		e.log.Info("relations reconciled", "cause", cause, "created", rep.Created, "renamed", rep.Renamed, "deleted", rep.Deleted,
			"errors", rep.Errors, "drift", rep.Drift, "duration_ms", rep.DurationMS)
	}
	return rep
}

type pgInfo struct {
	ref                         types.ManagedObjectReference
	name, key, segment, backing string
	vms                         int
}

// fullCause는 캐시를 쓰지 않고 전체를 다시 계산할 원인이다: 기동·초기화·복원·관리자 수동 조정(복구 절차, FR-DM-0403).
func fullCause(cause string) bool {
	for _, p := range []string{"startup", "reset", "restore", "admin"} {
		if strings.HasPrefix(cause, p) {
			return true
		}
	}
	return false
}

// Mismatches는 검증 모드에서 증분과 전체 계산이 달랐던 횟수다.
func (e *Engine) Mismatches() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mismatches
}

func (e *Engine) reconcile(ctx context.Context, rep *Report, full bool) error {
	c, err := e.vc.InternalClient(ctx)
	if err != nil {
		return err
	}
	// 1) NSX → vCenter 네트워크(DVPG)
	dvs, err := e.preparedVDS(ctx, c)
	if err != nil {
		return err
	}
	pgs, err := e.portgroups(ctx, c, full)
	if err != nil {
		return err
	}
	existing := map[string]pgInfo{}
	for _, pg := range pgs {
		if pg.backing == "nsx" && strings.HasPrefix(pg.segment, "/infra/segments/") {
			existing[pg.segment] = pg
		}
	}
	tzType := e.nsx.TransportZones()
	attached := map[string]bool{}
	for _, id := range e.fix.NSX.VDSTransportZones {
		attached[id] = true
	}
	defaultTZ := ""
	for _, tz := range e.fix.NSX.TransportZones {
		if tz.TransportType == "OVERLAY" {
			defaultTZ = tz.ID // 가정: TZ를 지정하지 않은 Segment는 기본 Overlay TZ(Fixture의 첫 Overlay)
			break
		}
	}
	want := map[string]bool{}
	for _, s := range e.nsx.Segments() {
		tz := defaultTZ
		if s.TZPath != "" {
			tz = s.TZPath[strings.LastIndex(s.TZPath, "/")+1:]
			if _, ok := tzType[s.TZPath]; !ok {
				tz = ""
			}
		}
		if !attached[tz] || len(e.fix.NSX.VDSTransportZones) == 0 {
			e.nsx.SetSegmentSync(s.Path, "") // vCenter 네트워크 매핑 대상 아님: 동기화할 것 없음
			continue
		}
		want[s.Path] = true
		rep.Mapped++
		if dvs == nil {
			msg := "NSX-prepared VDS not found in vCenter; segment cannot be realized on the host switch"
			rep.Errors[s.Path] = msg
			e.nsx.SetSegmentSync(s.Path, msg)
			continue
		}
		if cur, ok := existing[s.Path]; ok {
			if cur.name != s.Name {
				if err := waitTask(ctx)(object.NewCommon(c, cur.ref).Rename(ctx, s.Name)); err != nil {
					rep.Errors[s.Path] = err.Error()
					e.nsx.SetSegmentSync(s.Path, "vCenter portgroup rename failed: "+err.Error())
					continue
				}
				rep.Renamed = append(rep.Renamed, s.Path)
			}
			e.nsx.SetSegmentSync(s.Path, "")
			continue
		}
		spec := types.DVPortgroupConfigSpec{
			Name: s.Name, Type: string(types.DistributedVirtualPortgroupPortgroupTypeEphemeral),
			BackingType: string(types.DistributedVirtualPortgroupBackingTypeNsx), LogicalSwitchUuid: s.UniqueID, SegmentId: s.Path,
		}
		if err := waitTask(ctx)(dvs.AddPortgroup(ctx, []types.DVPortgroupConfigSpec{spec})); err != nil {
			rep.Errors[s.Path] = err.Error()
			e.nsx.SetSegmentSync(s.Path, "vCenter portgroup creation failed: "+err.Error())
			continue
		}
		rep.Created = append(rep.Created, s.Path)
		e.nsx.SetSegmentSync(s.Path, "")
	}
	for seg, pg := range existing {
		if want[seg] {
			continue
		}
		if pg.vms > 0 {
			rep.Drift = append(rep.Drift, fmt.Sprintf("%s: portgroup %s still has %d VM(s)", seg, pg.ref.Value, pg.vms))
			continue
		}
		if err := waitTask(ctx)(object.NewDistributedVirtualPortgroup(c, pg.ref).Destroy(ctx)); err != nil {
			rep.Errors[seg] = err.Error()
			continue
		}
		rep.Deleted = append(rep.Deleted, seg)
	}
	sort.Strings(rep.Created)
	sort.Strings(rep.Deleted)
	sort.Strings(rep.Drift)

	// 2) vCenter → NSX 인벤토리 파생
	if len(rep.Created)+len(rep.Deleted) > 0 {
		if pgs, err = e.portgroups(ctx, c, false); err != nil {
			return err
		}
	}
	return e.derive(ctx, c, dvs, pgs, rep, full)
}

func waitTask(ctx context.Context) func(*object.Task, error) error {
	return func(t *object.Task, err error) error {
		if err != nil {
			return err
		}
		_, err = t.WaitForResult(ctx, nil)
		return err
	}
}

// preparedVDS는 NSX가 준비한 VDS다: Fixture에 vds_transport_zones가 있으면 MoRef 순 첫 VDS(가정, D8).
func (e *Engine) preparedVDS(ctx context.Context, c *vim25.Client) (*object.DistributedVirtualSwitch, error) {
	if len(e.fix.NSX.VDSTransportZones) == 0 {
		return nil, nil
	}
	refs, err := find(ctx, c, "DistributedVirtualSwitch") // VmwareDistributedVirtualSwitch 포함
	if err != nil || len(refs) == 0 {
		return nil, err
	}
	sortRefs(refs)
	return object.NewDistributedVirtualSwitch(c, refs[0]), nil
}

func find(ctx context.Context, c *vim25.Client, kind string) ([]types.ManagedObjectReference, error) {
	v, err := view.NewManager(c).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{kind}, true)
	if err != nil {
		return nil, err
	}
	defer v.Destroy(ctx)
	return v.Find(ctx, []string{kind}, nil)
}

func sortRefs(refs []types.ManagedObjectReference) {
	sort.Slice(refs, func(i, j int) bool { return natLess(refs[i].Value, refs[j].Value) })
}

// natLess는 vm-9 < vm-10처럼 숫자 꼬리를 수로 비교한다.
func natLess(a, b string) bool {
	ai, bi := strings.LastIndex(a, "-"), strings.LastIndex(b, "-")
	if ai > 0 && bi > 0 && a[:ai] == b[:bi] {
		x, e1 := strconv.Atoi(a[ai+1:])
		y, e2 := strconv.Atoi(b[bi+1:])
		if e1 == nil && e2 == nil {
			return x < y
		}
	}
	return a < b
}

func (e *Engine) portgroups(ctx context.Context, c *vim25.Client, full bool) ([]pgInfo, error) {
	// 증분(R-45): 포트그룹별 결과를 캐시하고 변경 표시가 있거나 새로 생긴 포트그룹만 다시 조회한다.
	e.dirtyMu.Lock()
	dirty := e.pgDirty
	e.pgDirty = map[string]bool{}
	e.dirtyMu.Unlock()
	refs, err := find(ctx, c, "DistributedVirtualPortgroup")
	if err != nil {
		return nil, err
	}
	model := e.vc.Model()
	pc := &e.pgCache
	if full || pc.pgs == nil || pc.model != model {
		pc.pgs, pc.model = map[string]pgInfo{}, model
	}
	current := map[string]bool{}
	var fetch []types.ManagedObjectReference
	for _, r := range refs {
		current[r.Value] = true
		if _, ok := pc.pgs[r.Value]; !ok || dirty[r.Value] {
			fetch = append(fetch, r)
		}
	}
	for v := range pc.pgs {
		if !current[v] {
			delete(pc.pgs, v)
		}
	}
	fresh, err := retrievePortgroups(ctx, c, fetch)
	if err != nil {
		pc.pgs = nil
		return nil, err
	}
	for _, p := range fresh {
		pc.pgs[p.ref.Value] = p
	}
	out := make([]pgInfo, 0, len(refs))
	for _, r := range refs {
		out = append(out, pc.pgs[r.Value])
	}
	if e.Verify && len(fetch) < len(refs) {
		all, err := retrievePortgroups(ctx, c, refs)
		if err == nil {
			e.dirtyMu.Lock()
			pending := map[string]bool{}
			for k := range e.pgDirty {
				pending[k] = true
			}
			e.dirtyMu.Unlock()
			for i, p := range all {
				if !pending[p.ref.Value] && p != out[i] {
					e.mu.Lock()
					e.mismatches++
					e.mu.Unlock()
					e.log.Error("incremental portgroup cache differs", "portgroup", p.ref.Value, "cached", out[i], "fresh", p)
				}
			}
		}
	}
	return out, nil
}

// pgCache는 포트그룹 조회 캐시다(model이 바뀌면 버린다).
type pgCache struct {
	model any
	pgs   map[string]pgInfo
}

func retrievePortgroups(ctx context.Context, c *vim25.Client, refs []types.ManagedObjectReference) ([]pgInfo, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	var mos []mo.DistributedVirtualPortgroup
	if err := property.DefaultCollector(c).Retrieve(ctx, refs, []string{"name", "config", "vm"}, &mos); err != nil {
		return nil, err
	}
	byRef := map[string]pgInfo{}
	for _, p := range mos {
		byRef[p.Self.Value] = pgInfo{ref: p.Self, name: p.Name, key: p.Config.Key, segment: p.Config.SegmentId, backing: p.Config.BackingType, vms: len(p.Vm)}
	}
	out := make([]pgInfo, 0, len(refs))
	for _, r := range refs {
		if p, ok := byRef[r.Value]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}
