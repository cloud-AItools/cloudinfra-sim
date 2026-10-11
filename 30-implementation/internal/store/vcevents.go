package store

import (
	"context"
	"strings"
	"time"
)

// VCEvent는 보관한 vCenter 이벤트 하나다(R-44). vcsim EventManager는 최근 1,000건만 메모리에 두므로 시뮬레이터
// 저장소로 복제해 실행 프로필 한도(lite 1,000건, full 100,000건·7일)까지 보관한다. Epoch는 vCenter 모델 세대(Reset·
// 복원마다 바뀌고 vcsim 이벤트 키가 1부터 다시 시작한다)다.
type VCEvent struct {
	Seq        int64     `json:"seq"`
	Epoch      int64     `json:"epoch"`
	Key        int32     `json:"key"`
	At         time.Time `json:"created_time"`
	Type       string    `json:"type"`
	User       string    `json:"user,omitempty"`
	Entity     string    `json:"entity,omitempty"`
	EntityName string    `json:"entity_name,omitempty"`
	Message    string    `json:"message"`
}

// RecordVCEvents는 이벤트를 한 트랜잭션으로 넣고 보존 한도를 넘는 오래된 것을 지운다.
func (s *Store) RecordVCEvents(ctx context.Context, evs []VCEvent) error {
	if len(evs) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `INSERT INTO vcenter_events (epoch, key, at, type, username, entity, entity_name, message) VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, e := range evs {
		if _, err := st.ExecContext(ctx, e.Epoch, e.Key, e.At.UTC().Format(time.RFC3339Nano), e.Type, e.User, e.Entity, e.EntityName, e.Message); err != nil {
			return err
		}
	}
	if s.Limits.Events > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM vcenter_events WHERE seq <= (SELECT MAX(seq) FROM vcenter_events) - ?`, s.Limits.Events); err != nil {
			return err
		}
	}
	if s.Limits.MaxAge > 0 {
		cut := s.now().Add(-s.Limits.MaxAge).UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `DELETE FROM vcenter_events WHERE at < ?`, cut); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// VCEvents는 보관한 이벤트를 최근 순으로 돌려준다. typ(이벤트 타입 이름)·entity(MoRef 값)는 비면 거르지 않는다.
func (s *Store) VCEvents(ctx context.Context, limit int, typ, entity string) ([]VCEvent, error) {
	if limit <= 0 || limit > 100000 {
		limit = 100
	}
	q := `SELECT seq, epoch, key, at, type, username, entity, entity_name, message FROM vcenter_events`
	var where []string
	var args []any
	if typ != "" {
		where, args = append(where, "type = ?"), append(args, typ)
	}
	if entity != "" {
		where, args = append(where, "entity = ?"), append(args, entity)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY seq DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VCEvent{}
	for rows.Next() {
		var e VCEvent
		var at string
		if err := rows.Scan(&e.Seq, &e.Epoch, &e.Key, &at, &e.Type, &e.User, &e.Entity, &e.EntityName, &e.Message); err != nil {
			return nil, err
		}
		e.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ClearVCEvents는 보관한 vCenter 이벤트를 모두 지운다(환경 초기화).
func (s *Store) ClearVCEvents(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM vcenter_events`)
	return err
}
