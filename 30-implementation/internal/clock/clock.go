// Package clock은 Virtual Clock이다(PRD-1 FR-BE-0602 ~ 0604, 0607 ~ 0610, WP 4.10).
//
// 정상 모드(normal): 가상 시간 = 실제 시간. 테스트 모드(test): 관리자가 가속(rate)·정지(frozen)·진행(advance)을 제어한다.
// 실제 시각 → 가상 시각 대응을 구간(segment) 목록으로 보관하므로, 가속·정지 뒤에도 과거 실제 시각(예: vcsim이 실제
// 시계로 남긴 Task·Event 시각)을 정확히 가상 시각으로 바꿀 수 있다(FR-BE-0611, R-12: vcsim에는 시간 소스 교체 지점이
// 없어 응답 시각을 변환한다). HTTP 타임아웃·네트워크 대기는 실제 시간 그대로다(FR-BE-0610).
// 가상 시간 대기는 시계 변경 알림 채널로 깨어나므로 주기적 폴링이 없다(FR-BE-0612).
package clock

import (
	"errors"
	"sync"
	"time"
)

const (
	ModeNormal = "normal"
	ModeTest   = "test"
	MaxRate    = 1000
)

type segment struct {
	real, virt time.Time
	rate       float64 // 0이면 정지
}

type Clock struct {
	mu      sync.Mutex
	mode    string
	segs    []segment
	changed chan struct{}
	realNow func() time.Time
}

func New() *Clock {
	c := &Clock{realNow: time.Now}
	c.Reset()
	return c
}

// Reset은 정상 모드(가상 = 실제)로 돌린다.
func (c *Clock) Reset() {
	c.mu.Lock()
	now := c.realNow()
	c.mode = ModeNormal
	c.segs = []segment{{real: now, virt: now, rate: 1}}
	c.notifyLocked()
	c.mu.Unlock()
}

func (c *Clock) notifyLocked() {
	if c.changed != nil {
		close(c.changed)
	}
	c.changed = make(chan struct{})
}

// Changed는 다음 시계 변경 때 닫히는 채널이다.
func (c *Clock) Changed() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changed
}

func (s segment) at(r time.Time) time.Time {
	return s.virt.Add(time.Duration(float64(r.Sub(s.real)) * s.rate))
}

// ToVirtual은 실제 시각 r의 가상 시각이다.
func (c *Clock) ToVirtual(r time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.toVirtualLocked(r)
}

func (c *Clock) toVirtualLocked(r time.Time) time.Time {
	seg := c.segs[0]
	for _, s := range c.segs {
		if !r.Before(s.real) {
			seg = s
		}
	}
	if r.Before(c.segs[0].real) { // 시계 이전 시각: 첫 구간 차이만큼 이동
		return r.Add(c.segs[0].virt.Sub(c.segs[0].real))
	}
	return seg.at(r)
}

// ToRealEarliest는 가상 시각이 v 이상이 되는 가장 이른 실제 시각이다(시간 범위 필터의 시작, WP 4.6).
func (c *Clock) ToRealEarliest(v time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	first := c.segs[0]
	if v.Before(first.virt) {
		return v.Add(first.real.Sub(first.virt))
	}
	for i, s := range c.segs {
		if !v.After(s.virt) {
			return s.real
		}
		if s.rate > 0 {
			r := s.real.Add(time.Duration(float64(v.Sub(s.virt)) / s.rate))
			if i == len(c.segs)-1 || r.Before(c.segs[i+1].real) {
				return r
			}
		}
	}
	return c.realNow().Add(time.Hour) // 아직 오지 않은 가상 시각
}

// ToRealLatest는 가상 시각이 v 이하인 가장 늦은 실제 시각이다(시간 범위 필터의 끝, WP 4.6). 정지 구간은 구간 끝까지 포함한다.
func (c *Clock) ToRealLatest(v time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	first := c.segs[0]
	if v.Before(first.virt) {
		return v.Add(first.real.Sub(first.virt))
	}
	for i := len(c.segs) - 1; i >= 0; i-- {
		s := c.segs[i]
		if v.Before(s.virt) {
			continue
		}
		last := i == len(c.segs)-1
		if s.rate == 0 {
			if last {
				return c.realNow().Add(time.Hour)
			}
			return c.segs[i+1].real.Add(-time.Nanosecond)
		}
		r := s.real.Add(time.Duration(float64(v.Sub(s.virt)) / s.rate))
		if !last && !r.Before(c.segs[i+1].real) {
			return c.segs[i+1].real.Add(-time.Nanosecond)
		}
		return r
	}
	return v.Add(first.real.Sub(first.virt))
}

// Now는 현재 가상 시각이다.
func (c *Clock) Now() time.Time { return c.ToVirtual(c.realNow()) }

// Identity는 가상 시간 = 실제 시간인가(변환 생략용).
func (c *Clock) Identity() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode == ModeNormal {
		return true
	}
	for _, s := range c.segs {
		if s.rate != 1 || !s.real.Equal(s.virt) {
			return false
		}
	}
	return true
}

// Status는 시계 상태다(Admin API).
type Status struct {
	Mode     string    `json:"mode"`
	Rate     float64   `json:"rate"`
	Frozen   bool      `json:"frozen"`
	Virtual  time.Time `json:"virtual_time"`
	Real     time.Time `json:"real_time"`
	OffsetMS int64     `json:"offset_ms"`
}

func (c *Clock) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.realNow()
	v := c.toVirtualLocked(r)
	last := c.segs[len(c.segs)-1]
	return Status{Mode: c.mode, Rate: last.rate, Frozen: last.rate == 0, Virtual: v.UTC(), Real: r.UTC(), OffsetMS: v.Sub(r).Milliseconds()}
}

// Change는 시계 제어 요청이다. 테스트 모드에서만 rate·frozen·advance가 허용된다(FR-BE-0609).
type Change struct {
	Mode      string     `json:"mode,omitempty"`
	Rate      *float64   `json:"rate,omitempty"`       // 가속(FR-BE-0602), 0 < rate <= 1000
	Frozen    *bool      `json:"frozen,omitempty"`     // 정지·재개(FR-BE-0603)
	AdvanceMS int64      `json:"advance_ms,omitempty"` // 진행(FR-BE-0604)
	AdvanceTo *time.Time `json:"advance_to,omitempty"` // 특정 시점까지 진행
}

func (c *Clock) Apply(ch Change) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch ch.Mode {
	case "":
	case ModeNormal:
		now := c.realNow()
		c.mode, c.segs = ModeNormal, []segment{{real: now, virt: now, rate: 1}}
		c.notifyLocked()
		if ch.Rate != nil || ch.Frozen != nil || ch.AdvanceMS != 0 || ch.AdvanceTo != nil {
			return errors.New("정상 모드에서는 가상 시간을 제어할 수 없다(mode=test)")
		}
		return nil
	case ModeTest:
		c.mode = ModeTest
	default:
		return errors.New("mode: normal | test")
	}
	if c.mode != ModeTest && (ch.Rate != nil || ch.Frozen != nil || ch.AdvanceMS != 0 || ch.AdvanceTo != nil) {
		return errors.New("가상 시간 제어는 테스트 모드에서만 가능하다(mode=test, FR-BE-0609)")
	}
	r := c.realNow()
	last := c.segs[len(c.segs)-1]
	v := last.at(r)
	rate := last.rate
	if ch.Rate != nil {
		if *ch.Rate <= 0 || *ch.Rate > MaxRate {
			return errors.New("rate: 0 초과 1000 이하(정지는 frozen)")
		}
		rate = *ch.Rate
	}
	if ch.Frozen != nil {
		if *ch.Frozen {
			rate = 0
		} else if rate == 0 {
			rate = 1
			if ch.Rate != nil {
				rate = *ch.Rate
			}
		}
	}
	if ch.AdvanceMS < 0 {
		return errors.New("advance_ms: 0 이상(시간을 되돌릴 수 없다)")
	}
	v = v.Add(time.Duration(ch.AdvanceMS) * time.Millisecond)
	if ch.AdvanceTo != nil {
		if ch.AdvanceTo.Before(v) {
			return errors.New("advance_to: 현재 가상 시각 이후여야 한다")
		}
		v = *ch.AdvanceTo
	}
	c.segs = append(c.segs, segment{real: r, virt: v, rate: rate})
	if len(c.segs) > 1024 { // 오래된 구간 정리(첫 구간은 이전 시각 변환 기준으로 남김)
		c.segs = append(c.segs[:1], c.segs[len(c.segs)-512:]...)
	}
	c.notifyLocked()
	return nil
}

// Sleep은 가상 시간 d만큼 기다린다. 시계가 바뀌면 다시 계산한다. 정지 중이면 진행·재개를 기다린다.
func (c *Clock) Sleep(d time.Duration, done <-chan struct{}) bool {
	deadline := c.Now().Add(d)
	return c.SleepUntil(deadline, done)
}

// SleepUntil은 가상 시각 deadline까지 기다린다. done이 닫히면 false.
func (c *Clock) SleepUntil(deadline time.Time, done <-chan struct{}) bool {
	for {
		c.mu.Lock()
		r := c.realNow()
		last := c.segs[len(c.segs)-1]
		v := last.at(r)
		ch := c.changed
		c.mu.Unlock()
		if !v.Before(deadline) {
			return true
		}
		var timer <-chan time.Time
		if last.rate > 0 {
			t := time.NewTimer(time.Duration(float64(deadline.Sub(v)) / last.rate))
			defer t.Stop()
			timer = t.C
		}
		select {
		case <-done:
			return false
		case <-ch:
		case <-timer:
		}
	}
}
