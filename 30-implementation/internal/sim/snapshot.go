package sim

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"cloudinfra-sim/internal/store"
	"cloudinfra-sim/internal/vcenter"
)

// Snapshot·Restore·재기동 복원(WP 5.2: TR-SCN-0104·0105, FR-DM-0209·0304·0306, FR-BE-0208, NFR-0205 ~ 0208).
//
// 일관성(FR-DM-0306, 권고안 A): 교차 락(CrossLock, 모든 변경 요청을 직렬화) 하나를 쥔 채 vcsim 인벤토리 덤프,
// SQLite 백업, NSX 엔진 상태를 함께 저장해 같은 시점의 Snapshot을 만든다. 진행 중 Task가 있으면 만들지 않는다.
// 복원은 같은 락 안에서 셋을 바꾼 뒤 관계를 다시 조정하고 불일치를 검사한다(NFR-0207). 실패하면 Fixture로 초기화한다.
// 파일 형식: <data-dir>/snapshots/<이름>/{manifest.json, vcenter.json, inventory/, datastores/, state.db, nsx.json}.
//
// 재처리 정책(NFR-0208, 가정): 재기동·복원 뒤 끝나지 않은 작업은 다시 실행하지 않고 오류("interrupted")로 닫는다.
// 클라이언트가 재시도한다(실제 vCenter의 재기동 뒤 Task와 같은 결과 — 근거 미확보, BEHAVIOR_SPEC §14).
// Snapshot·복원은 Fault Rule을 비우고(인프라 상태 Fault 원복), Virtual Clock·Realization 지연·메트릭 덮어쓰기는 유지한다.

var (
	ErrSnapshotBusy     = errors.New("진행 중인 Task가 있다")
	ErrSnapshotExists   = errors.New("같은 이름의 Snapshot이 있다")
	ErrSnapshotLimit    = errors.New("Snapshot 보존 한도에 도달했다")
	ErrSnapshotNotFound = errors.New("Snapshot이 없다")
	ErrSnapshotMismatch = errors.New("Snapshot의 프로파일·Fixture가 실행 중인 것과 다르다")
	ErrSnapshotCorrupt  = errors.New("Snapshot 파일 무결성 검사 실패")
	ErrSnapshotName     = errors.New("Snapshot 이름은 영문·숫자로 시작하는 1~64자(영문·숫자·.-_)")
)

var snapName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ShutdownSnapshot은 full 프로필 정상 종료 때 자동으로 만드는 Snapshot 이름이다(NFR-0205).
const ShutdownSnapshot = "_shutdown"

// SnapshotManifest는 Snapshot 메타데이터다.
type SnapshotManifest struct {
	Name        string            `json:"name"`
	Created     time.Time         `json:"created"`      // 실제 시각
	VirtualTime time.Time         `json:"virtual_time"` // 만든 시점의 가상 시각
	VCenter     string            `json:"vcenter"`      // 버전/빌드
	NSX         string            `json:"nsx"`
	Fixture     string            `json:"fixture"`
	FixtureHash string            `json:"fixture_sha256"`
	Build       string            `json:"build_commit,omitempty"`
	Objects     int               `json:"vcenter_objects"`
	NSXObjects  int               `json:"nsx_objects"`
	SizeBytes   int64             `json:"size_bytes"`
	Files       map[string]string `json:"sha256"` // 상대 경로 → SHA-256
	Excluded    []string          `json:"not_included"`
}

// RestoreResult는 복원 결과와 불일치 검사다.
type RestoreResult struct {
	Snapshot    string   `json:"snapshot"`
	Consistent  bool     `json:"consistent"`
	Issues      []string `json:"issues,omitempty"`
	Interrupted []string `json:"interrupted_operations,omitempty"`
	DurationMS  int64    `json:"duration_ms"`
}

func (s *Simulator) snapshotRoot() string { return filepath.Join(s.Cfg.DataDir, "snapshots") }

// Snapshots는 저장된 Snapshot 목록이다(최근 것 먼저).
func (s *Simulator) ListSnapshots() []SnapshotManifest {
	dirs, _ := os.ReadDir(s.snapshotRoot())
	var out []SnapshotManifest
	for _, d := range dirs {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		if m, err := readManifest(filepath.Join(s.snapshotRoot(), d.Name())); err == nil {
			out = append(out, *m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

func readManifest(dir string) (*SnapshotManifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m SnapshotManifest
	return &m, json.Unmarshal(b, &m)
}

// CreateSnapshot은 현재 상태의 Snapshot을 만든다(TR-SCN-0104).
func (s *Simulator) CreateSnapshotManifest(name string) (*SnapshotManifest, error) {
	if !snapName.MatchString(name) {
		return nil, ErrSnapshotName
	}
	s.GW.Lock.Lock()
	defer s.GW.Lock.Unlock()
	return s.createSnapshotLocked(name, false)
}

func (s *Simulator) createSnapshotLocked(name string, replace bool) (*SnapshotManifest, error) {
	if err := s.VC.SyncEvents(context.Background()); err != nil { // 보관본을 Snapshot 시점까지 맞춘다(R-44)
		return nil, err
	}
	if run := s.VC.RunningTasks(); len(run) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotBusy, strings.Join(run, ", "))
	}
	final := filepath.Join(s.snapshotRoot(), name)
	if _, err := os.Stat(final); err == nil && !replace {
		return nil, ErrSnapshotExists
	}
	if !replace && !strings.HasPrefix(name, "_") {
		n := 0
		for _, m := range s.ListSnapshots() {
			if !strings.HasPrefix(m.Name, "_") {
				n++
			}
		}
		if n >= s.Retention["snapshots"] {
			return nil, fmt.Errorf("%w(%d개): 오래된 Snapshot을 지운 뒤 다시 만든다", ErrSnapshotLimit, s.Retention["snapshots"])
		}
	}
	start := time.Now()
	tmp := filepath.Join(s.snapshotRoot(), ".tmp-"+name)
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, err
	}
	fail := func(err error) (*SnapshotManifest, error) {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	if err := s.VC.SaveSnapshot(tmp); err != nil {
		return fail(err)
	}
	if err := s.Store.Backup(context.Background(), filepath.Join(tmp, "state.db")); err != nil {
		return fail(fmt.Errorf("저장소 백업: %w", err))
	}
	st, err := s.NSX.ExportState()
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "nsx.json"), st, 0o600); err != nil {
		return fail(err)
	}
	nsxN, _ := s.Store.Count(context.Background(), "nsx_resources")
	var vc struct {
		Objects int `json:"objects"`
	}
	if b, err := os.ReadFile(filepath.Join(tmp, "vcenter.json")); err == nil {
		_ = json.Unmarshal(b, &vc)
	}
	m := &SnapshotManifest{
		Name: name, Created: time.Now().UTC(), VirtualTime: s.Clock.Now().UTC(),
		VCenter: s.Profile.VCenter.Version + "/" + s.Profile.VCenter.Build, NSX: s.Profile.NSX.Version + "/" + s.Profile.NSX.Build,
		Fixture: s.Fixture.Name, FixtureHash: s.fixtureHash, Build: BuildCommit, Objects: vc.Objects, NSXObjects: nsxN,
		Excluded: []string{"vAPI 상태(Tagging, Content Library)", "세션", "Fault Rule", "API Trace·감사 기록"},
	}
	if m.Files, m.SizeBytes, err = hashTree(tmp); err != nil {
		return fail(err)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, "manifest.json"), b, 0o600); err != nil {
		return fail(err)
	}
	if replace {
		_ = os.RemoveAll(final)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fail(err)
	}
	s.Log.Info("snapshot created", "name", name, "vcenter_objects", m.Objects, "nsx_objects", nsxN, "bytes", m.SizeBytes, "duration_ms", time.Since(start).Milliseconds())
	s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: "snapshot create", Target: name, Status: 201, Cause: "admin"})
	return m, nil
}

// DeleteSnapshot은 Snapshot을 지운다.
func (s *Simulator) DeleteSnapshot(name string) error {
	if !snapName.MatchString(name) && name != ShutdownSnapshot {
		return ErrSnapshotName
	}
	dir := filepath.Join(s.snapshotRoot(), name)
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		return ErrSnapshotNotFound
	}
	s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: "snapshot delete", Target: name, Status: 204, Cause: "admin"})
	return os.RemoveAll(dir)
}

// RestoreSnapshot은 Snapshot으로 상태를 복원한다(TR-SCN-0105). name이 "last"면 가장 최근 것.
func (s *Simulator) Restore(name string) (*RestoreResult, error) {
	s.GW.Lock.Lock()
	defer s.GW.Lock.Unlock()
	return s.restoreLocked(name)
}

func (s *Simulator) restoreLocked(name string) (*RestoreResult, error) {
	if name == "last" {
		list := s.ListSnapshots()
		if len(list) == 0 {
			return nil, ErrSnapshotNotFound
		}
		name = list[0].Name
	}
	if !snapName.MatchString(name) && name != ShutdownSnapshot {
		return nil, ErrSnapshotName
	}
	dir := filepath.Join(s.snapshotRoot(), name)
	m, err := readManifest(dir)
	if err != nil {
		return nil, ErrSnapshotNotFound
	}
	if want := s.Profile.VCenter.Version + "/" + s.Profile.VCenter.Build; m.VCenter != want || m.NSX != s.Profile.NSX.Version+"/"+s.Profile.NSX.Build ||
		m.Fixture != s.Fixture.Name || m.FixtureHash != s.fixtureHash {
		return nil, fmt.Errorf("%w: snapshot %s·%s·%s, 실행 중 %s·%s·%s", ErrSnapshotMismatch, m.VCenter, m.NSX, m.Fixture, want, s.Profile.NSX.Version+"/"+s.Profile.NSX.Build, s.Fixture.Name)
	}
	files, _, err := hashTree(dir)
	if err != nil {
		return nil, err
	}
	delete(files, "manifest.json")
	for p, h := range m.Files {
		if files[p] != h {
			return nil, fmt.Errorf("%w: %s", ErrSnapshotCorrupt, p)
		}
	}
	if len(files) != len(m.Files) {
		return nil, fmt.Errorf("%w: 파일 수 %d, 기록 %d", ErrSnapshotCorrupt, len(files), len(m.Files))
	}
	start := time.Now()
	s.GW.Faults.Clear()
	nsxState, err := os.ReadFile(filepath.Join(dir, "nsx.json"))
	if err != nil {
		return nil, err
	}
	if err := s.VC.RestoreSnapshot(dir); err != nil {
		return nil, fmt.Errorf("vCenter 복원(상태 변경 없음): %w", err)
	}
	// 여기부터 실패하면 vCenter와 NSX가 다른 시점이 되므로 Fixture로 초기화한다.
	ctx := context.Background()
	if err := s.Store.RestoreFrom(ctx, filepath.Join(dir, "state.db")); err != nil {
		return nil, s.abortRestore(fmt.Errorf("저장소 복원: %w", err))
	}
	if err := s.NSX.ImportState(nsxState); err != nil {
		return nil, s.abortRestore(err)
	}
	// 복원한 저장소에는 Snapshot 시점까지의 보관 이벤트가 있고, vCenter 모델은 같은 이벤트를 다시 게시했다 → 중복 보관하지 않는다.
	s.VC.MarkEventsArchived()
	res := &RestoreResult{Snapshot: name}
	res.Interrupted, _ = s.Store.InterruptedOperations(ctx, s.Clock.Now())
	rep := s.Rel.Reconcile("restore " + name)
	res.Issues = append(res.Issues, s.VC.Consistency()...)
	res.Issues = append(res.Issues, s.NSX.Consistency()...)
	for _, d := range rep.Drift {
		res.Issues = append(res.Issues, "relations drift "+d)
	}
	for k, e := range rep.Errors {
		res.Issues = append(res.Issues, "relations error "+k+": "+e)
	}
	res.Consistent = len(res.Issues) == 0
	res.DurationMS = time.Since(start).Milliseconds()
	s.Log.Info("snapshot restored", "name", name, "consistent", res.Consistent, "issues", len(res.Issues), "duration_ms", res.DurationMS)
	s.recordAudit(store.AuditEvent{Product: "admin", Principal: "admin", Action: "snapshot restore", Target: name, Status: 200, Cause: "admin"})
	return res, nil
}

func (s *Simulator) abortRestore(err error) error {
	s.Log.Error("snapshot restore failed; resetting to fixture", "err", err)
	if rerr := s.resetLocked(); rerr != nil {
		return fmt.Errorf("%v; 초기화도 실패: %w", err, rerr)
	}
	return fmt.Errorf("%v (환경을 Fixture로 초기화했다)", err)
}

// admin.Backend 어댑터
func (s *Simulator) Snapshots() any { return s.ListSnapshots() }

func (s *Simulator) CreateSnapshot(name string) (any, error) { return s.CreateSnapshotManifest(name) }

func (s *Simulator) RestoreSnapshot(name string) (any, error) { return s.Restore(name) }

// ErrorStatus는 Admin API 오류 상태다.
func (s *Simulator) ErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrSnapshotName):
		return 400
	case errors.Is(err, ErrSnapshotNotFound), errors.Is(err, ErrScenarioNotFound), errors.Is(err, ErrRunNotFound):
		return 404
	case errors.Is(err, ErrScenarioRunning), errors.Is(err, ErrRunFinished):
		return 409
	case errors.Is(err, ErrSnapshotBusy), errors.Is(err, ErrSnapshotExists), errors.Is(err, ErrSnapshotLimit), errors.Is(err, ErrSnapshotMismatch):
		return 409
	case errors.Is(err, ErrSnapshotCorrupt):
		return 422
	}
	return 500
}

// Consistency는 현재 상태의 불일치 검사다(NFR-0207, Admin API).
func (s *Simulator) Consistency() any {
	s.GW.Lock.Lock()
	defer s.GW.Lock.Unlock()
	issues := append(s.VC.Consistency(), s.NSX.Consistency()...)
	r, _ := s.Rel.Last()
	for _, d := range r.Drift {
		issues = append(issues, "relations drift "+d)
	}
	return map[string]any{"consistent": len(issues) == 0, "issues": issues}
}

// hashTree는 dir 아래 파일의 SHA-256(상대 경로, / 구분)과 총 크기다. manifest.json도 포함해 돌려준다.
func hashTree(dir string) (map[string]string, int64, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	// 파일마다의 열기 지연이 병목이라 병렬로 읽는다(R-45 ②)
	out := make(map[string]string, len(files))
	var total int64
	var mu sync.Mutex
	work := make(chan string)
	errc := make(chan error, 1)
	var wg sync.WaitGroup
	for range min(vcenter.IOWorkers, max(len(files), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				sum, n, err := hashFile(p)
				if err != nil {
					select {
					case errc <- err:
					default:
					}
					continue
				}
				rel, _ := filepath.Rel(dir, p)
				mu.Lock()
				out[filepath.ToSlash(rel)] = sum
				total += n
				mu.Unlock()
			}
		}()
	}
	for _, p := range files {
		work <- p
	}
	close(work)
	wg.Wait()
	select {
	case err := <-errc:
		return nil, 0, err
	default:
	}
	return out, total, nil
}

func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
