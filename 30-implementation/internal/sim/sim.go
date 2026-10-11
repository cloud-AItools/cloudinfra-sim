// Package sim은 시뮬레이터 구성 요소를 조립한다(권고안 A: 단일 프로세스 — NFR-ARC-0301).
package sim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"cloudinfra-sim/internal/admin"
	"cloudinfra-sim/internal/certs"
	"cloudinfra-sim/internal/clock"
	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/fault"
	"cloudinfra-sim/internal/fixture"
	"cloudinfra-sim/internal/gateway"
	"cloudinfra-sim/internal/nsx"
	"cloudinfra-sim/internal/profile"
	"cloudinfra-sim/internal/relations"
	"cloudinfra-sim/internal/store"
	"cloudinfra-sim/internal/vcenter"
)

// BuildCommit은 ldflags로 넣는 커밋 식별자다(release 모드 증거 대조, WP 1.13).
var BuildCommit = ""

type Simulator struct {
	Cfg       *config.Config
	Log       *slog.Logger
	Profile   *profile.Set
	Fixture   *fixture.Fixture
	Users     *Users
	Certs     *certs.Bundle
	VC        *vcenter.VCenter
	NSX       *nsx.Engine
	GW        *gateway.Gateway
	Store     *store.Store
	Rel       *relations.Engine // vCenter ↔ NSX 관계 조정(Phase 3)
	Clock     *clock.Clock      // Virtual Clock(WP 4.10)
	Retention map[string]int    // 적용 중인 보존 한도(Admin 상태, NFR-0508). events는 vcsim 고정값

	fixtureHash string // Snapshot 대조(snapshot.go)
	stopOnce    sync.Once
	evStop      chan struct{} // vCenter 이벤트 보관 고루틴 종료(R-44)
	evDone      chan struct{}
	scen        scenarioRuns // 관리 API Scenario Runner(scenarios.go)
	adminToken  string

	adminSrv *http.Server
	Addrs    map[string]net.Addr
	started  time.Time
}

// New는 프로파일을 검증하고 엔진을 만든다. 리스너는 Start에서 연다.
func New(cfg *config.Config, log *slog.Logger) (*Simulator, error) {
	s := &Simulator{Cfg: cfg, Log: log, Addrs: map[string]net.Addr{}}
	var err error
	if s.Profile, err = profile.Load(cfg.ProfilesDir, cfg.VCenterVersion, cfg.NSXVersion); err != nil {
		return nil, err
	}
	if err := s.Profile.CheckMode(cfg.VerificationMode, BuildCommit); err != nil {
		return nil, fmt.Errorf("프로파일 기동 검증 실패 — %w", err)
	}
	if s.Profile.Interop != "verified" {
		log.Warn("vCenter × NSX 조합의 상호운용성이 확인되지 않았다(development 모드에서만 기동)", "vcenter", cfg.VCenterVersion, "nsx", cfg.NSXVersion, "interop", s.Profile.Interop)
	}
	if s.Fixture, err = fixture.Load(cfg.FixtureFile); err != nil {
		return nil, err
	}
	s.fixtureHash = fileHash(cfg.FixtureFile)
	var created bool
	if s.Users, created, err = LoadOrCreateUsers(cfg.DataDir); err != nil {
		return nil, fmt.Errorf("사용자 저장소: %w", err)
	}
	if created {
		log.Info("사용자·자격 증명을 만들었다(비밀번호는 파일에만 기록)", "file", s.Users.Path)
	}
	var newCert bool
	if s.Certs, newCert, err = certs.LoadOrCreate(filepath.Join(cfg.DataDir, "certs"), cfg.Domain); err != nil {
		return nil, err
	}
	if newCert {
		log.Info("시뮬레이터 루트 CA와 서버 인증서를 만들었다. 테스트 클라이언트에 CA를 신뢰 등록한다",
			"ca", filepath.Join(cfg.DataDir, "certs", "ca.pem"))
	}
	// 보존 한도: 실행 프로파일 기본값(RESOURCE_BUDGET §4)에 설정값을 덮어쓴다(NFR-0508).
	limits, tasks, traceLimit, snapshots := store.LiteLimits(), 200, 10000, 5
	if cfg.RunProfile == "full" {
		limits, tasks, traceLimit, snapshots = store.FullLimits(), 10000, 1000000, 20
	}
	if limits.MaxAge > 0 && cfg.RetainDays > 0 {
		limits.MaxAge = time.Duration(cfg.RetainDays) * 24 * time.Hour
	}
	limits.Transitions = config.Pick(cfg.RetainTransitions, limits.Transitions)
	limits.Operations = config.Pick(cfg.RetainOperations, limits.Operations)
	vcenter.TaskRetention = config.Pick(cfg.RetainTasks, tasks)
	traceLimit = config.Pick(cfg.RetainTrace, traceLimit)
	s.Retention = map[string]int{"tasks": vcenter.TaskRetention, "transitions": limits.Transitions, "operations": limits.Operations, "trace": traceLimit,
		"events": limits.Events, "snapshots": config.Pick(cfg.RetainSnapshots, snapshots)}
	if s.Store, err = store.Open(cfg.RunProfile, cfg.DataDir, limits); err != nil {
		return nil, fmt.Errorf("저장소: %w", err)
	}
	s.Clock = clock.New()
	s.Store.Now = s.Clock.Now
	var vcOpts []vcenter.Option
	if cfg.FixtureCache != "off" { // R-45 ②: 큰 Fixture는 처음 만든 인벤토리를 저장해 두고 다음 기동·Reset에 적재
		min := config.FixtureCacheMinVMs
		if cfg.FixtureCache == "on" {
			min = 0
		}
		vcOpts = append(vcOpts, vcenter.WithFixtureCache(&vcenter.FixtureCache{Dir: filepath.Join(cfg.DataDir, "fixture-cache"), MinVMs: min}))
	}
	if s.VC, err = vcenter.New(s.Profile, s.Fixture, s.Users.Check, cfg.Host("vcenter"), log.With("engine", "vcenter"), s.Store, &s.Certs.Server, vcOpts...); err != nil {
		s.Store.Close()
		return nil, err
	}
	s.VC.Clock = s.Clock
	cm := nsx.ComputeManagerInfo{Server: cfg.Host("vcenter"), Thumbprint: s.Certs.ThumbprintSHA256()}
	if s.NSX, err = nsx.New(s.Profile, s.Fixture, s.Users.Check, cm, os.Getenv("SIM_NSX_SPEC_DIR"), s.Store); err != nil {
		s.VC.Close()
		s.Store.Close()
		return nil, err
	}
	s.NSX.Now = s.Clock.Now
	ringLimit := traceLimit
	if cfg.RunProfile == "full" {
		ringLimit = 1 // full은 아래에서 TraceDB로 바꾼다. 메모리 링버퍼를 한도 크기로 미리 잡지 않는다.
	}
	s.GW = gateway.New(cfg.Domain, s.Certs.Server, log.With("component", "gateway"), ringLimit)
	s.GW.Handle("vcenter", s.VC)
	s.GW.Handle("esx", s.VC)
	s.GW.Handle("nsx", s.NSX)
	s.GW.Audit = s.recordAudit
	// 리스너별 TLS 최대 버전(프로파일 tls.max_version). 값은 profile.Load에서 이미 검증했다.
	vcMax, _ := s.Profile.VCenter.TLS.MaxVersionID()
	nsxMax, _ := s.Profile.NSX.TLS.MaxVersionID()
	s.GW.MaxTLS = map[string]uint16{"vcenter": vcMax, "nsx": nsxMax}
	// full: Trace 본문 포함·파일 저장·기간/용량 회전(NFR-0504, WP 5.9)
	if cfg.RunProfile == "full" {
		td, err := gateway.NewTraceDB(s.Store.DB, traceLimit, int64(config.Pick(cfg.RetainTraceMB, 2048))<<20, limits.MaxAge, s.Clock.Now)
		if err != nil {
			s.VC.Close()
			s.Store.Close()
			return nil, fmt.Errorf("trace 저장소: %w", err)
		}
		s.GW.Trace, s.GW.Bodies = td, true
		s.Retention["trace_mb"] = config.Pick(cfg.RetainTraceMB, 2048)
		s.Retention["days"] = int(limits.MaxAge / (24 * time.Hour))
	}
	// Phase 4a: Virtual Clock(WP 4.10)·Fault 계층(WP 4.3)·세션 만료(WP 4.1) 연결
	s.GW.Now = s.Clock.Now
	s.VC.Faults, s.NSX.Faults, s.NSX.Log = s.GW.Faults, s.GW.Faults, log.With("engine", "nsx")
	s.GW.ExpireSession = func(kind, id string) {
		if kind == "nsx" {
			s.NSX.ExpireSession(id)
			return
		}
		s.VC.ExpireSession(kind, id)
	}
	s.GW.Faults.OnChange(s.VC.ApplyStateFaults)
	s.VC.Role, s.NSX.Role = s.Users.Role, s.Users.Role
	s.Rel = relations.New(s.VC, s.NSX, s.Fixture, s.GW.Lock, log.With("component", "relations"))
	s.Rel.Audit = s.recordAudit
	s.Rel.Verify = os.Getenv("SIM_RELATIONS_VERIFY") != "" // 시험용: 증분 조정을 매번 전체 계산과 대조(R-45)
	s.GW.Lock.Lock()
	s.Rel.Reconcile("startup")
	s.GW.Lock.Unlock()
	// 재기동 후 상태 복원(FR-BE-0208, NFR-0205): 지정한 경우에만. 실패하면 기동하지 않는다(조용히 Fixture로 시작하지 않음).
	if cfg.RestoreSnapshot != "" {
		res, err := s.Restore(cfg.RestoreSnapshot)
		if err != nil {
			s.Shutdown(context.Background())
			return nil, fmt.Errorf("--restore-snapshot %s: %w", cfg.RestoreSnapshot, err)
		}
		log.Info("기동 시 Snapshot을 복원했다", "snapshot", res.Snapshot, "consistent", res.Consistent, "issues", res.Issues, "interrupted", res.Interrupted)
	}
	return s, nil
}

// Start는 vCenter·NSX·Admin 리스너를 연다.
func (s *Simulator) Start() error {
	s.started = time.Now()
	s.evStop, s.evDone = make(chan struct{}), make(chan struct{})
	go s.archiveEvents()
	for _, l := range []struct{ name, addr string }{{"vcenter", s.Cfg.VCenterAddr}, {"nsx", s.Cfg.NSXAddr}} {
		a, err := s.GW.Listen(l.addr, l.name)
		if err != nil {
			return fmt.Errorf("%s 수신 주소 %s 바인딩 실패: %w (다른 프로그램의 443 점유 여부 확인, --%s-addr로 변경 가능)", l.name, l.addr, err, l.name)
		}
		s.Addrs[l.name] = a
	}
	host, _, err := net.SplitHostPort(s.Cfg.AdminAddr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("Admin API는 루프백 주소에만 바인딩한다: %s", s.Cfg.AdminAddr)
	}
	token, err := admin.LoadOrCreateToken(s.Cfg.DataDir)
	if err != nil {
		return err
	}
	s.adminToken = token
	ln, err := net.Listen("tcp", s.Cfg.AdminAddr)
	if err != nil {
		return fmt.Errorf("Admin API 바인딩 실패: %w", err)
	}
	s.Addrs["admin"] = ln.Addr()
	s.adminSrv = &http.Server{Handler: admin.Handler(s, token), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := s.adminSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.Log.Error("admin listener stopped", "err", err)
		}
	}()
	s.Log.Info("cloudinfra-sim 시작",
		"vcenter", fmt.Sprintf("https://%s (%s)", s.Cfg.Host("vcenter"), s.Addrs["vcenter"]),
		"nsx", fmt.Sprintf("https://%s (%s)", s.Cfg.Host("nsx"), s.Addrs["nsx"]),
		"admin", "http://"+s.Addrs["admin"].String(),
		"profile", s.Profile.VCenter.Version+"/"+s.Profile.VCenter.Build+" + NSX "+s.Profile.NSX.Version,
		"verification_mode", s.Cfg.VerificationMode, "run_profile", s.Cfg.RunProfile,
		"thumbprint_sha256", s.Certs.ThumbprintSHA256())
	return nil
}

func (s *Simulator) Shutdown(ctx context.Context) {
	s.stopOnce.Do(func() { s.shutdown(ctx) })
}

// syncEvents는 vCenter 이벤트를 저장소에 보관한다(교차 락 안: Reset·복원과 겹치지 않는다, R-44).
func (s *Simulator) syncEvents() {
	s.GW.Lock.Lock()
	defer s.GW.Lock.Unlock()
	if err := s.VC.SyncEvents(context.Background()); err != nil {
		s.Log.Warn("vCenter event archive", "err", err)
	}
}

// archiveEvents는 1초마다 보관한다(vcsim 링버퍼 1,000건을 넘기 전에 옮긴다). 변경이 없으면 조회하지 않는다.
func (s *Simulator) archiveEvents() {
	defer close(s.evDone)
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	for {
		select {
		case <-s.evStop:
			return
		case <-tk.C:
			s.syncEvents()
		}
	}
}

func (s *Simulator) shutdown(ctx context.Context) {
	// 실행 중인 시나리오를 중지한다(정리 단계가 Admin API를 쓰므로 리스너를 닫기 전에)
	s.scen.mu.Lock()
	active := s.scen.active
	s.scen.mu.Unlock()
	if active != nil {
		active.cancel()
		select {
		case <-active.done:
		case <-ctx.Done():
		}
	}
	// full 프로필: 정상 종료 때 상태를 _shutdown Snapshot으로 남긴다(NFR-0205). --restore-snapshot _shutdown|last로 복원.
	if s.Cfg.RunProfile == "full" && s.adminSrv != nil {
		s.GW.Lock.Lock()
		if _, err := s.createSnapshotLocked(ShutdownSnapshot, true); err != nil {
			s.Log.Warn("shutdown snapshot skipped", "err", err)
		}
		s.GW.Lock.Unlock()
	}
	if s.evStop != nil {
		close(s.evStop)
		<-s.evDone
		s.syncEvents() // 마지막 이벤트까지 보관(저장소를 닫기 전)
	}
	if s.adminSrv != nil {
		_ = s.adminSrv.Shutdown(ctx)
	}
	s.GW.Shutdown(ctx)
	s.Rel.Close()
	s.VC.Close()
	_ = s.Store.Close()
}

// --- admin.Backend ---

func (s *Simulator) Reset() error {
	s.GW.Lock.Lock()
	defer s.GW.Lock.Unlock()
	return s.resetLocked()
}

func (s *Simulator) resetLocked() error {
	s.GW.Faults.Clear() // 인프라 상태 Fault를 먼저 되돌린다
	s.Clock.Reset()     // 가정: 환경 초기화는 정상 모드(가상 = 실제)로 돌린다
	if err := s.VC.Reset(); err != nil {
		return err
	}
	if err := s.NSX.Reset(); err != nil {
		return err
	}
	if err := s.VC.ResetEventArchive(context.Background()); err != nil { // 새 환경의 이벤트만 보관(R-44)
		return err
	}
	s.Rel.Reconcile("reset")
	s.GW.Trace.Reset()
	s.GW.Unsup.Reset()
	s.VC.ClearMetricOverrides()
	s.Log.Info("environment reset", "fixture", s.Fixture.Name)
	s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: "reset", Target: s.Fixture.Name, Status: 200, Cause: "admin"})
	return nil
}

func (s *Simulator) SetRealizationDelayMS(ms int) {
	s.NSX.SetRealizationDelay(time.Duration(ms) * time.Millisecond)
}

func (s *Simulator) Trace(limit int) any {
	entries, dropped := s.GW.Trace.Recent(limit)
	return map[string]any{"entries": entries, "dropped": dropped}
}

func (s *Simulator) Unsupported() any {
	stats, other := s.GW.Unsup.Snapshot()
	return map[string]any{"apis": stats, "other_bucket": other}
}

// Status는 활성 프로파일과 검증 상태를 돌려준다(FR-API-0304). 실행 범위 밖 API는 '지원됨'으로 표시하지 않는다.
func (s *Simulator) Status() any {
	scope := func(ids []string, v profile.Verification) []map[string]string {
		in := map[string]bool{}
		out := []map[string]string{}
		for _, id := range ids {
			in[id] = true
			out = append(out, map[string]string{"api": id, "status": v.APIs[id].Status, "scope": "execution"})
		}
		var extra []string
		for id := range v.APIs {
			if !in[id] {
				extra = append(extra, id)
			}
		}
		sort.Strings(extra)
		for _, id := range extra {
			out = append(out, map[string]string{"api": id, "status": v.APIs[id].Status, "scope": "out-of-scope (미검증, 지원 표시 안 함)", "note": v.APIs[id].Note})
		}
		return out
	}
	usage := map[string]any{}
	if td, ok := s.GW.Trace.(*gateway.TraceDB); ok {
		usage["trace"] = td.Usage()
	}
	for _, t := range []string{"simulation_operations", "operation_transitions"} {
		if n, err := s.Store.Count(context.Background(), t); err == nil {
			usage[t] = n
		}
	}
	// 장시간 운전 진단(R-45 ④): 고루틴·Go 힙·vcsim 레지스트리 크기
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	objs, locks := s.VC.RegistryStats()
	usage["fixture_cache"] = s.VC.LastCache()
	usage["runtime"] = map[string]any{"goroutines": runtime.NumGoroutine(), "heap_alloc_mb": float64(ms.HeapAlloc>>10) / 1024,
		"vcsim_objects": objs, "vcsim_locks": locks}
	return map[string]any{
		"retention_usage":   usage,
		"build":             map[string]string{"kind": config.BuildKind, "commit": BuildCommit},
		"verification_mode": s.Cfg.VerificationMode,
		"verification_cap":  "Integration Tested (승인 범위 자체 테스트, CMP·실환경 미검증)",
		"vcenter":           map[string]any{"version": s.Profile.VCenter.Version, "build": s.Profile.VCenter.Build, "api_version": s.Profile.VCenter.APIVersion, "apis": scope(s.Profile.VCenter.ExecutionScope, s.Profile.VCVerification)},
		"nsx":               map[string]any{"version": s.Profile.NSX.Version, "build": s.Profile.NSX.Build, "contract_spec": "3.2.1.0.0 (3.2.4.3과 차이 미검증)", "apis": scope(s.Profile.NSX.ExecutionScope, s.Profile.NSXVerification)},
		"certificate":       map[string]string{"thumbprint_sha1": s.Certs.Thumbprint(), "thumbprint_sha256": s.Certs.ThumbprintSHA256()},
		"fixture":           s.Fixture.Name,
		"retention":         s.Retention,
		"uptime_seconds":    int(time.Since(s.started).Seconds()),
	}
}

func (s *Simulator) Faults() *fault.Store { return s.GW.Faults }

// Reconcile은 vCenter ↔ NSX 관계를 즉시 조정한다(FR-DM-0403 복구 절차, Admin API).
func (s *Simulator) Reconcile() any {
	s.GW.Lock.Lock()
	defer s.GW.Lock.Unlock()
	return s.Rel.Reconcile("admin")
}

// Relations는 마지막 조정 결과다.
func (s *Simulator) Relations() any {
	r, n := s.Rel.Last()
	return map[string]any{"last": r, "runs": n}
}

// recordAudit는 감사 기록을 저장한다(NFR-0306). 실패는 로그만 남긴다(요청을 막지 않음).
func (s *Simulator) recordAudit(e store.AuditEvent) {
	if err := s.Store.RecordAudit(context.Background(), e); err != nil {
		s.Log.Warn("audit record failed", "err", err)
	}
}

// AuditLog는 최근 감사 기록이다. cid가 있으면 그 요청과 그것이 원인인 기록만.
func (s *Simulator) AuditLog(limit int, cid string) any {
	ev, err := s.Store.Audit(context.Background(), limit, cid)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	return map[string]any{"events": ev}
}

// AuditAdmin은 Admin API의 변경 동작을 감사 기록에 남긴다.
func (s *Simulator) AuditAdmin(action string, status int) {
	s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: action, Status: status, Cause: "admin"})
}

// MetricOverrides·SetMetricOverride는 가상 메트릭 덮어쓰기 관리다(FR-MET-0103·0104, Admin API).
func (s *Simulator) MetricOverrides() any { return map[string]any{"overrides": s.VC.MetricOverrides()} }

func (s *Simulator) SetMetricOverride(o vcenter.MetricOverride) error {
	return s.VC.SetMetricOverride(o)
}

func (s *Simulator) ClearMetricOverrides() { s.VC.ClearMetricOverrides() }

// ClockStatus·SetClock은 Virtual Clock 제어다(TR-SCN-0108, Admin API).
func (s *Simulator) ClockStatus() any { return s.Clock.Status() }

func (s *Simulator) SetClock(ch clock.Change) error { return s.Clock.Apply(ch) }

// UserList·AddUser·RemoveUser는 시험용 사용자 관리다(RBAC 시나리오, FR-API-0213).
func (s *Simulator) UserList() any { return map[string]any{"users": s.Users.List()} }

func (s *Simulator) AddUser(name string, products []string, role, scope string) (any, error) {
	return s.Users.Add(User{Name: name, Products: products, Role: role, Scope: scope})
}

func (s *Simulator) RemoveUser(name string) bool { return s.Users.Remove(name) }

// Events는 vCenter 이벤트(최근 순)와 NSX 알람이다(TR-SCN-0110, Admin API). limit 기본 100.
func (s *Simulator) Events(limit int, typ, entity string) (any, error) {
	if limit <= 0 {
		limit = 100
	}
	// 저장소 보관본(R-44: lite 1,000건, full 100,000건·7일). 조회 전에 아직 옮기지 않은 이벤트를 보관한다.
	s.syncEvents()
	ev, err := s.Store.VCEvents(context.Background(), limit, typ, entity)
	if err != nil {
		return nil, err
	}
	return map[string]any{"vcenter": ev, "nsx_alarms": s.NSX.AlarmList(),
		"retention": map[string]any{"vcenter_events": s.Retention["events"], "vcenter_events_dropped": s.VC.EventsDropped()}}, nil
}
