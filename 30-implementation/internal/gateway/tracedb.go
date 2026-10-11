package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"sync"
	"time"
)

// full 실행 프로필의 API Trace(NFR-0504, RESOURCE_BUDGET §4): 본문 포함, 요청·응답 각 64 KB에서 자르고, 7일·2 GB·건수
// 한도 중 먼저 닿는 조건으로 오래된 기록부터 지운다. 저장은 데이터 디렉터리의 SQLite(api_trace)이며 환경 초기화(Reset)는
// 기록을 비우지만 재기동은 비우지 않는다(로그). 잘린 본문은 body_truncated로, 회전으로 지운 건수는 dropped로 알린다.
// 본문의 자격 증명(비밀번호·세션 토큰)은 저장 전에 가린다(NFR-0302).

// TraceSink는 Trace 저장소다(lite: 메모리 링버퍼 Trace, full: TraceDB).
type TraceSink interface {
	Add(e TraceEntry)
	Recent(n int) ([]TraceEntry, int64)
	Reset()
}

// MaxTraceBody는 full Trace의 요청·응답 본문 상한이다(RESOURCE_BUDGET §4: 64 KB).
const MaxTraceBody = 64 << 10

type TraceDB struct {
	db       *sql.DB
	maxRows  int
	maxBytes int64
	maxAge   time.Duration
	now      func() time.Time

	mu      sync.Mutex
	rows    int
	bytes   int64
	dropped int64
	adds    int
}

const traceSchema = `CREATE TABLE IF NOT EXISTS api_trace (
  seq INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, size INTEGER NOT NULL, entry TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS api_trace_at ON api_trace(at);`

// NewTraceDB는 full Trace 저장소를 연다. now는 기간 한도의 기준 시각(가상 시각)이다.
func NewTraceDB(db *sql.DB, maxRows int, maxBytes int64, maxAge time.Duration, now func() time.Time) (*TraceDB, error) {
	if _, err := db.Exec(traceSchema); err != nil {
		return nil, err
	}
	t := &TraceDB{db: db, maxRows: maxRows, maxBytes: maxBytes, maxAge: maxAge, now: now}
	if err := db.QueryRow("SELECT COUNT(*), COALESCE(SUM(size),0) FROM api_trace").Scan(&t.rows, &t.bytes); err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.pruneLocked()
	t.mu.Unlock()
	return t, nil
}

func (t *TraceDB) Add(e TraceEntry) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	at := t.now().UTC().Format(time.RFC3339Nano)
	if _, err := t.db.Exec("INSERT INTO api_trace(at, size, entry) VALUES(?,?,?)", at, len(b), string(b)); err != nil {
		return
	}
	t.rows++
	t.bytes += int64(len(b))
	t.adds++
	// 기간 한도는 64건마다, 건수·용량 한도는 넘는 즉시 적용한다.
	if t.adds%64 == 0 || t.rows > t.maxRows || t.bytes > t.maxBytes {
		t.pruneLocked()
	}
}

// Prune은 기간 한도를 바로 적용한다(시험·Admin 상태 조회용).
func (t *TraceDB) Prune() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked()
}

func (t *TraceDB) pruneLocked() {
	remove := func(where string, args ...any) {
		var n int
		var size int64
		if t.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(size),0) FROM api_trace WHERE "+where, args...).Scan(&n, &size) != nil || n == 0 {
			return
		}
		if _, err := t.db.Exec("DELETE FROM api_trace WHERE "+where, args...); err == nil {
			t.rows -= n
			t.bytes -= size
			t.dropped += int64(n)
		}
	}
	if t.maxAge > 0 {
		remove("at < ?", t.now().Add(-t.maxAge).UTC().Format(time.RFC3339Nano))
	}
	if over := t.rows - t.maxRows; over > 0 {
		remove("seq IN (SELECT seq FROM api_trace ORDER BY seq LIMIT ?)", over)
	}
	for t.bytes > t.maxBytes && t.rows > 0 {
		// 오래된 것부터 지운다(한 번에 남은 건수의 1/8, 최소 1건)
		n := t.rows / 8
		if n < 1 {
			n = 1
		}
		before := t.rows
		remove("seq IN (SELECT seq FROM api_trace ORDER BY seq LIMIT ?)", n)
		if t.rows == before {
			return
		}
	}
}

func (t *TraceDB) Recent(n int) ([]TraceEntry, int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked() // 조회 결과가 기간 한도를 반영하도록
	if n <= 0 || n > t.rows {
		n = t.rows
	}
	rows, err := t.db.QueryContext(context.Background(), "SELECT entry FROM api_trace ORDER BY seq DESC LIMIT ?", n)
	if err != nil {
		return nil, t.dropped
	}
	defer rows.Close()
	var out []TraceEntry
	for rows.Next() {
		var s string
		var e TraceEntry
		if rows.Scan(&s) == nil && json.Unmarshal([]byte(s), &e) == nil {
			out = append(out, e)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, t.dropped
}

func (t *TraceDB) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.db.Exec("DELETE FROM api_trace"); err == nil {
		t.rows, t.bytes, t.dropped = 0, 0, 0
	}
}

// Usage는 현재 건수·바이트·회전 건수다(Admin 상태).
func (t *TraceDB) Usage() map[string]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]int64{"rows": int64(t.rows), "bytes": t.bytes, "dropped": t.dropped, "max_rows": int64(t.maxRows), "max_bytes": t.maxBytes}
}

// 본문 마스킹(NFR-0302): SOAP·JSON·폼의 비밀번호, vAPI 세션 토큰, SOAP 세션 키.
var bodyMasks = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)(<password[^>]*>)[^<]*(</password>)`), "${1}***${2}"},
	{regexp.MustCompile(`(?i)("(?:password|token)"\s*:\s*")[^"]*(")`), "${1}***${2}"},
	{regexp.MustCompile(`(?i)(j_password=)[^&\s]*`), "${1}***"},
	{regexp.MustCompile(`(?i)(<key>)[0-9a-f-]{16,}(</key>)`), "${1}***${2}"},
}

// maskBody는 본문을 자르고 자격 증명을 가린다. vAPI 세션 생성 응답(토큰 문자열 하나)은 통째로 가린다.
func maskBody(b []byte, sessionResponse bool) (string, bool) {
	truncated := false
	if len(b) > MaxTraceBody {
		b, truncated = b[:MaxTraceBody], true
	}
	if sessionResponse && len(b) > 0 {
		return `"***"`, truncated
	}
	s := string(b)
	for _, m := range bodyMasks {
		s = m.re.ReplaceAllString(s, m.repl)
	}
	return s, truncated
}
