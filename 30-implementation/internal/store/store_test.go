package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// T-FR-DM-0301 T-FR-DM-0307 T-NFR-0106 T-NFR-0204 T-NFR-ARC-0106: lite 인메모리 SQLite, 트랜잭션 롤백 시 미반영.
func TestNSXTransaction(t *testing.T) {
	s, err := Open("lite", t.TempDir(), LiteLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	tx, _ := s.BeginNSX(ctx)
	_ = tx.Put("/policy/api/v1/infra/segments/a", map[string]any{"id": "a"})
	_ = tx.Rollback()
	if n, _ := s.Count(ctx, "nsx_resources"); n != 0 {
		t.Fatalf("rollback left %d rows", n)
	}
	tx, _ = s.BeginNSX(ctx)
	_ = tx.Put("/policy/api/v1/infra/segments/a", map[string]any{"id": "a", "_revision": 0})
	_ = tx.Put("/policy/api/v1/infra/segments/a", map[string]any{"id": "a", "_revision": 1})
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	all, _ := s.LoadNSX(ctx)
	if len(all) != 1 || all["/policy/api/v1/infra/segments/a"]["_revision"] != float64(1) {
		t.Fatalf("upsert: %v", all)
	}
}

// T-NFR-0506 T-NFR-0501: 상태 전이 기록은 보존 한도를 넘지 않는다.
func TestTransitionRetention(t *testing.T) {
	s, err := Open("lite", t.TempDir(), Limits{Transitions: 10, Operations: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("task-%d", i)
		op := Operation{ID: id, Target: "vm-1", Type: "powerOn", State: "running", StartedAt: time.Now()}
		if err := s.RecordTransition(ctx, op, "queued"); err != nil {
			t.Fatal(err)
		}
		op.State, op.CompletedAt = "success", time.Now()
		if err := s.RecordTransition(ctx, op, "running"); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := s.Count(ctx, "operation_transitions"); n != 10 {
		t.Fatalf("transitions %d, want 10", n)
	}
	if n, _ := s.Count(ctx, "simulation_operations"); n != 3 {
		t.Fatalf("completed operations %d, want 3", n)
	}
}

// T-NFR-0503 T-NFR-0506 T-NFR-0508 T-NFR-0208: full 기간 한도(가상 시각 기준)·감사 총용량·백업/복원·중단 작업 처리.
func TestFullRetentionAndBackup(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lim := FullLimits()
	lim.AuditBytes = 64 << 10
	s, err := Open("full", t.TempDir(), lim)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Now = func() time.Time { return now }
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = s.RecordAudit(ctx, AuditEvent{Product: "vcenter", Action: "old", Cause: "t"})
		_ = s.RecordTransition(ctx, Operation{ID: fmt.Sprint("op-old-", i), State: "success", CompletedAt: now}, "running")
	}
	now = now.Add(8 * 24 * time.Hour)
	_ = s.RecordAudit(ctx, AuditEvent{Product: "vcenter", Action: "new", Cause: "t"})
	_ = s.RecordTransition(ctx, Operation{ID: "op-new", State: "running"}, "queued")
	ev, _ := s.Audit(ctx, 0, "")
	if len(ev) != 1 || ev[0].Action != "new" {
		t.Fatalf("audit age limit: %+v", ev)
	}
	if n, _ := s.Count(ctx, "operation_transitions"); n != 1 {
		t.Fatalf("transition age limit: %d", n)
	}
	if n, _ := s.Count(ctx, "simulation_operations"); n != 1 {
		t.Fatalf("completed operation age limit: %d", n)
	}
	// 감사 총용량(64 KB 추정): 1 KB 대상 200건 → 한도 이하로 회전
	for i := 0; i < 200; i++ {
		_ = s.RecordAudit(ctx, AuditEvent{Product: "nsx", Action: "big", Target: fmt.Sprintf("%01024d", i), Cause: "t"})
	}
	if ev, _ := s.Audit(ctx, 0, ""); len(ev) == 0 || len(ev) > 64 {
		t.Fatalf("audit byte limit: %d rows", len(ev))
	}
	// 백업 → 변경 → 복원(같은 시점 상태), 끝나지 않은 작업은 interrupted로 닫힌다
	tx, _ := s.BeginNSX(ctx)
	_ = tx.Put("/k/a", map[string]any{"id": "a"})
	_ = tx.Commit()
	backup := t.TempDir() + "/state.db"
	if err := s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	tx, _ = s.BeginNSX(ctx)
	_ = tx.Delete("/k/a")
	_ = tx.Put("/k/b", map[string]any{"id": "b"})
	_ = tx.Commit()
	if err := s.RestoreFrom(ctx, backup); err != nil {
		t.Fatal(err)
	}
	all, _ := s.LoadNSX(ctx)
	if _, ok := all["/k/a"]; !ok || len(all) != 1 {
		t.Fatalf("restore: %v", all)
	}
	ids, err := s.InterruptedOperations(ctx, now)
	if err != nil || len(ids) != 1 || ids[0] != "op-new" {
		t.Fatalf("interrupted: %v %v", ids, err)
	}
}
