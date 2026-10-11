package gateway

import (
	"sort"
	"sync"
	"time"
)

// TraceEntry는 자체 API 호출 기록의 메타데이터다(TR-SCN-0111). lite는 본문을 저장하지 않는다(NFR-0504).
type TraceEntry struct {
	Time           time.Time `json:"time"`
	CorrelationID  string    `json:"correlation_id"`
	Host           string    `json:"host"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	SOAPMethod     string    `json:"soap_method,omitempty"`
	SessionVersion string    `json:"session_version,omitempty"`
	FaultRule      string    `json:"fault_rule,omitempty"` // 적용된 전송 계층 Fault Rule(FR-FLT-0205)
	Status         int       `json:"status"`
	DurationMS     int64     `json:"duration_ms"`
	// full 실행 프로필만(NFR-0504): 마스킹한 본문, 64 KB에서 자름
	RequestBody   string `json:"request_body,omitempty"`
	ResponseBody  string `json:"response_body,omitempty"`
	BodyTruncated bool   `json:"body_truncated,omitempty"`
}

// Trace는 고정 크기 링버퍼다(NFR-0501, RESOURCE_BUDGET §4: lite 10,000건).
type Trace struct {
	mu      sync.Mutex
	buf     []TraceEntry
	next    int
	full    bool
	dropped int64
}

func NewTrace(limit int) *Trace { return &Trace{buf: make([]TraceEntry, limit)} }

func (t *Trace) Add(e TraceEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.full {
		t.dropped++
	}
	t.buf[t.next] = e
	t.next = (t.next + 1) % len(t.buf)
	if t.next == 0 {
		t.full = true
	}
}

// Recent는 최근 n건을 오래된 순으로 돌려준다. dropped는 회전으로 버려진 건수다.
func (t *Trace) Recent(n int) (out []TraceEntry, dropped int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	size := t.next
	if t.full {
		size = len(t.buf)
	}
	if n <= 0 || n > size {
		n = size
	}
	for i := size - n; i < size; i++ {
		idx := i
		if t.full {
			idx = (t.next + i) % len(t.buf)
		}
		out = append(out, t.buf[idx])
	}
	return out, t.dropped
}

func (t *Trace) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next, t.full, t.dropped = 0, false, 0
}

// Unsupported는 미지원 API 호출 집계다(FR-API-0403·0404). 집계 키 수에 상한을 두고 넘치면 '기타'로 모은다(NFR-0503).
type Unsupported struct {
	mu    sync.Mutex
	limit int
	m     map[string]*UnsupportedStat
	other int64
}

type UnsupportedStat struct {
	Key       string    `json:"key"`
	Count     int64     `json:"count"`
	Versions  []string  `json:"api_versions,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

func NewUnsupported(limit int) *Unsupported {
	return &Unsupported{limit: limit, m: map[string]*UnsupportedStat{}}
}

func (u *Unsupported) Add(key, version string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	s, ok := u.m[key]
	if !ok {
		if len(u.m) >= u.limit {
			u.other++
			return
		}
		s = &UnsupportedStat{Key: key, FirstSeen: time.Now()}
		u.m[key] = s
	}
	s.Count++
	s.LastSeen = time.Now()
	if version != "" && len(s.Versions) < 8 && !contains(s.Versions, version) {
		s.Versions = append(s.Versions, version)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func (u *Unsupported) Snapshot() ([]UnsupportedStat, int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]UnsupportedStat, 0, len(u.m))
	for _, s := range u.m {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out, u.other
}

func (u *Unsupported) Reset() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.m, u.other = map[string]*UnsupportedStat{}, 0
}
