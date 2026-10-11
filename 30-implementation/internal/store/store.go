// Package store는 시뮬레이터 상태 저장소(SQLite)다. vCenter 인벤토리는 vcsim이 원본이므로 저장하지 않는다(FR-DM-0303).
// 관련: FR-DM-0301·0302·0305·0307, NFR-0106, NFR-0201 ~ NFR-0204, NFR-0501·0506, NFR-ARC-0106.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // 순수 Go SQLite(cgo 불필요)
)

// Limits는 보존 한도다(RESOURCE_BUDGET §4, D4).
type Limits struct {
	Transitions int // operation_transitions
	Operations  int // simulation_operations (완료된 것)
	Audit       int // audit_events(NFR-0503: lite 최근 1,000건)
	Events      int // vcenter_events(vCenter 이벤트 보관, R-44: lite 1,000건 — vcsim 링버퍼와 같음, full 100,000건)
	// full만(RESOURCE_BUDGET §4): 기간 한도(전이·감사 7일)와 감사 총용량(1 GB). 0이면 적용 안 함(lite).
	MaxAge     time.Duration
	AuditBytes int64
}

func LiteLimits() Limits {
	return Limits{Transitions: 5000, Operations: 5000, Audit: 1000, Events: 1000}
}
func FullLimits() Limits {
	return Limits{Transitions: 500000, Operations: 500000, Audit: 1000000, Events: 100000, MaxAge: 7 * 24 * time.Hour, AuditBytes: 1 << 30}
}

type Store struct {
	DB     *sql.DB
	Limits Limits
	// Now는 기록 시각과 기간 한도의 기준(가상 시각, FR-BE-0611)이다. nil이면 실제 시간.
	Now func() time.Time

	auditAdds int
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const schema = `
CREATE TABLE IF NOT EXISTS nsx_resources (
  key TEXT PRIMARY KEY,            -- basePath + 상대 경로
  body TEXT NOT NULL               -- 스펙 검증을 통과한 객체(JSON)
);
CREATE TABLE IF NOT EXISTS simulation_operations (
  op_id TEXT PRIMARY KEY, target_ref TEXT, op_type TEXT, initiator TEXT, state TEXT,
  progress INTEGER, started_at TEXT, completed_at TEXT, parent_op_id TEXT, result TEXT, error TEXT
);
CREATE TABLE IF NOT EXISTS operation_transitions (
  seq INTEGER PRIMARY KEY AUTOINCREMENT, op_id TEXT, from_state TEXT, to_state TEXT, at TEXT
);
CREATE TABLE IF NOT EXISTS audit_events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, cid TEXT, product TEXT, principal TEXT, action TEXT, target TEXT,
  status INTEGER, cause TEXT
);
CREATE TABLE IF NOT EXISTS vcenter_events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT, epoch INTEGER, key INTEGER, at TEXT, type TEXT, username TEXT,
  entity TEXT, entity_name TEXT, message TEXT
);
CREATE INDEX IF NOT EXISTS vcenter_events_at ON vcenter_events(at);
CREATE TABLE IF NOT EXISTS event_outbox (
  seq INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT, ref TEXT, payload TEXT, enqueued_at TEXT, processed_at TEXT
);`

// Open은 저장소를 연다. lite는 인메모리, full은 데이터 디렉터리의 파일(WAL)을 쓴다(FR-DM-0307, NFR-0106).
// full은 synchronous=NORMAL이다: WAL에서 손상 없이 일관성을 지키며, 정전 시 마지막 커밋 일부만 잃을 수 있다
// (요청마다 fsync하던 FULL은 모든 요청의 Trace 기록을 디스크 동기화에 묶어 처리량을 제한했다 — Phase 5 벤치마크, R-09).
func Open(runProfile, dataDir string, limits Limits) (*Store, error) {
	var dsn string
	if runProfile == "full" {
		dsn = "file:" + filepath.ToSlash(filepath.Join(dataDir, "cloudinfra-sim.db")) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	} else {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		dsn = "file:mem-" + hex.EncodeToString(b) + "?mode=memory&cache=shared&_pragma=busy_timeout(5000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if runProfile != "full" {
		db.SetMaxIdleConns(1) // 인메모리 DB는 연결이 하나라도 남아 있어야 유지된다
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{DB: db, Limits: limits}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// Reset은 모든 상태를 지운다(TR-SCN-0102).
func (s *Store) Reset(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"nsx_resources", "simulation_operations", "operation_transitions", "event_outbox"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// NSXTx는 NSX 변경 묶음이다. 한 요청(H-API 포함)의 변경은 하나의 트랜잭션으로 반영한다(NFR-0204, FR-DM-0305).
type NSXTx struct{ tx *sql.Tx }

func (s *Store) BeginNSX(ctx context.Context) (*NSXTx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	return &NSXTx{tx: tx}, err
}

func (t *NSXTx) Put(key string, obj map[string]any) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec("INSERT INTO nsx_resources(key, body) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET body = excluded.body", key, string(b))
	return err
}

func (t *NSXTx) Delete(key string) error {
	_, err := t.tx.Exec("DELETE FROM nsx_resources WHERE key = ?", key)
	return err
}

func (t *NSXTx) Commit() error   { return t.tx.Commit() }
func (t *NSXTx) Rollback() error { return t.tx.Rollback() }

// LoadNSX는 저장된 NSX 객체 전체를 읽는다(full 프로필 재기동 시, FR-BE-0208은 MVP 이후).
func (s *Store) LoadNSX(ctx context.Context) (map[string]map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT key, body FROM nsx_resources")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]any{}
	for rows.Next() {
		var k, b string
		if err := rows.Scan(&k, &b); err != nil {
			return nil, err
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(b), &obj); err != nil {
			return nil, err
		}
		out[k] = obj
	}
	return out, rows.Err()
}

// Operation은 내부 작업 모델이다(FR-BE-0502). 외부 계약(SOAP Task 등)으로는 엔진이 변환한다(FR-BE-0503).
type Operation struct {
	ID, Target, Type, Initiator, State string
	Progress                           int
	StartedAt, CompletedAt             time.Time
	Parent, Result, Error              string
}

// RecordTransition은 작업 상태 전이를 기록하고 보존 한도를 넘는 오래된 전이·완료 작업을 지운다(NFR-0506).
func (s *Store) RecordTransition(ctx context.Context, op Operation, from string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := func(t time.Time) any {
		if t.IsZero() {
			return nil
		}
		return t.UTC().Format(time.RFC3339Nano)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO simulation_operations(op_id, target_ref, op_type, initiator, state, progress, started_at, completed_at, parent_op_id, result, error)
		VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(op_id) DO UPDATE SET state=excluded.state, progress=excluded.progress,
		started_at=COALESCE(excluded.started_at, started_at), completed_at=excluded.completed_at, result=excluded.result, error=excluded.error`,
		op.ID, op.Target, op.Type, op.Initiator, op.State, op.Progress, ts(op.StartedAt), ts(op.CompletedAt), op.Parent, op.Result, op.Error); err != nil {
		return err
	}
	if from != op.State {
		if _, err := tx.ExecContext(ctx, "INSERT INTO operation_transitions(op_id, from_state, to_state, at) VALUES(?,?,?,?)",
			op.ID, from, op.State, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM operation_transitions WHERE seq <= (SELECT MAX(seq) FROM operation_transitions) - ?`, s.Limits.Transitions); err != nil {
		return err
	}
	if s.Limits.MaxAge > 0 {
		cut := s.now().Add(-s.Limits.MaxAge).UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `DELETE FROM operation_transitions WHERE at < ?`, cut); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM simulation_operations WHERE completed_at IS NOT NULL AND completed_at < ?`, cut); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM simulation_operations WHERE completed_at IS NOT NULL AND op_id IN (
		SELECT op_id FROM simulation_operations WHERE completed_at IS NOT NULL ORDER BY completed_at DESC LIMIT -1 OFFSET ?)`, s.Limits.Operations); err != nil {
		return err
	}
	return tx.Commit()
}

// Count는 시험·Admin API용 행 수다.
func (s *Store) Count(ctx context.Context, table string) (int, error) {
	switch table {
	case "nsx_resources", "simulation_operations", "operation_transitions", "event_outbox":
	default:
		return 0, fmt.Errorf("unknown table %s", table)
	}
	var n int
	err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n)
	return n, err
}

// stateTables는 Snapshot에 담는 상태 테이블이다. audit_events는 기록(로그)이라 담지 않고 복원도 기록으로 남긴다.
var stateTables = []string{"nsx_resources", "simulation_operations", "operation_transitions", "event_outbox"}

// Backup은 상태를 같은 시점의 SQLite 파일로 백업한다(FR-DM-0306: 호출자가 단일 락을 쥔 채 vcsim 덤프와 함께 수행).
func (s *Store) Backup(ctx context.Context, path string) error {
	_, err := s.DB.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}

// RestoreFrom은 Backup 파일의 상태 테이블로 현재 상태를 한 트랜잭션에 바꾼다(NFR-0204).
func (s *Store) RestoreFrom(ctx context.Context, path string) error {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS snap", "file:"+filepath.ToSlash(path)+"?mode=ro"); err != nil {
		return fmt.Errorf("attach: %w", err)
	}
	defer conn.ExecContext(context.Background(), "DETACH DATABASE snap") //nolint:errcheck
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range stateTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM main."+t); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO main."+t+" SELECT * FROM snap."+t); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
	}
	return tx.Commit()
}

// InterruptedOperations는 끝나지 않은 작업을 오류로 닫는다(NFR-0208 재처리 정책: 재기동·복원 뒤 자동 재실행하지 않고
// 'interrupted'로 기록, 재시도는 클라이언트가 한다). 닫은 작업 ID를 돌려준다.
func (s *Store) InterruptedOperations(ctx context.Context, at time.Time) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT op_id, state FROM simulation_operations WHERE completed_at IS NULL")
	if err != nil {
		return nil, err
	}
	type op struct{ id, state string }
	var open []op
	for rows.Next() {
		var o op
		if err := rows.Scan(&o.id, &o.state); err != nil {
			rows.Close()
			return nil, err
		}
		open = append(open, o)
	}
	rows.Close()
	var ids []string
	for _, o := range open {
		if err := s.RecordTransition(ctx, Operation{ID: o.id, State: "error", CompletedAt: at, Error: "interrupted: simulator restarted or state restored"}, o.state); err != nil {
			return ids, err
		}
		ids = append(ids, o.id)
	}
	return ids, nil
}
