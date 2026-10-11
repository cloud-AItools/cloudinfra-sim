package store

import (
	"context"
	"time"
)

// AuditEvent는 감사 기록 하나다(NFR-0306, FR-BE-0209 변경 원인 추적). 환경 초기화(Reset)에도 지우지 않는다.
//   - API 요청: cid(상관 ID)·사용자·동작(SOAP 메서드 또는 HTTP 메서드+경로)·대상(MoRef 또는 경로, Task ID)·상태
//   - 내부 변경(관계 조정 등): cause에 원인(예: "relations nsx-write cid=...")
type AuditEvent struct {
	Seq       int64     `json:"seq"`
	At        time.Time `json:"at"`
	CID       string    `json:"cid,omitempty"`
	Product   string    `json:"product"`
	Principal string    `json:"principal,omitempty"`
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	Status    int       `json:"status,omitempty"`
	Cause     string    `json:"cause"`
}

// RecordAudit는 감사 기록을 남기고 보존 한도를 넘는 오래된 기록을 지운다.
func (s *Store) RecordAudit(ctx context.Context, e AuditEvent) error {
	if e.At.IsZero() {
		e.At = s.now().UTC()
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO audit_events (at, cid, product, principal, action, target, status, cause) VALUES (?,?,?,?,?,?,?,?)`,
		e.At.Format(time.RFC3339Nano), e.CID, e.Product, e.Principal, e.Action, e.Target, e.Status, e.Cause); err != nil {
		return err
	}
	if s.Limits.Audit > 0 {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM audit_events WHERE seq <= (SELECT MAX(seq) FROM audit_events) - ?`, s.Limits.Audit); err != nil {
			return err
		}
	}
	// full: 7일 기간 한도(매번), 1 GB 총용량(100건마다 추정 크기로 검사, NFR-0503)
	if s.Limits.MaxAge > 0 {
		cut := s.now().Add(-s.Limits.MaxAge).UTC().Format(time.RFC3339Nano)
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM audit_events WHERE at < ?`, cut); err != nil {
			return err
		}
	}
	s.auditAdds++
	if s.Limits.AuditBytes > 0 && s.auditAdds%100 == 0 {
		return s.pruneAuditBytes(ctx)
	}
	return nil
}

// Audit는 최근 감사 기록(오래된 순)이다. cid가 있으면 그 요청과 그것이 원인인 기록만 돌려준다.
func (s *Store) Audit(ctx context.Context, limit int, cid string) ([]AuditEvent, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	q := `SELECT seq, at, cid, product, principal, action, target, status, cause FROM audit_events`
	args := []any{}
	if cid != "" {
		q += ` WHERE cid = ? OR cause LIKE ?`
		args = append(args, cid, "%cid="+cid+"%")
	}
	q += ` ORDER BY seq DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		var at string
		if err := rows.Scan(&e.Seq, &at, &e.CID, &e.Product, &e.Principal, &e.Action, &e.Target, &e.Status, &e.Cause); err != nil {
			return nil, err
		}
		e.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, e)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// pruneAuditBytes는 감사 기록 추정 크기(열 길이 합)가 한도를 넘으면 오래된 것부터 지운다.
func (s *Store) pruneAuditBytes(ctx context.Context) error {
	const size = `length(at)+length(cid)+length(product)+length(principal)+length(action)+length(target)+length(cause)+16`
	for {
		var total int64
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(`+size+`),0), COUNT(*) FROM audit_events`).Scan(&total, &n); err != nil {
			return err
		}
		if total <= s.Limits.AuditBytes || n == 0 {
			return nil
		}
		drop := n / 8
		if drop < 1 {
			drop = 1
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM audit_events WHERE seq IN (SELECT seq FROM audit_events ORDER BY seq LIMIT ?)`, drop); err != nil {
			return err
		}
	}
}
