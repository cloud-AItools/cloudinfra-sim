package clock

import (
	"testing"
	"time"
)

// T-FR-BE-0602 T-FR-BE-0603 T-FR-BE-0604 T-FR-BE-0609 T-TR-SCN-0108 T-FR-BE-0612
func TestVirtualClock(t *testing.T) {
	base := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	real := base
	c := &Clock{realNow: func() time.Time { return real }}
	c.Reset()
	if !c.Identity() || !c.Now().Equal(base) {
		t.Fatal("normal mode must equal real time")
	}
	one := 1.0
	if err := c.Apply(Change{Rate: &one}); err == nil {
		t.Fatal("control requires test mode")
	}
	ten := 10.0
	if err := c.Apply(Change{Mode: ModeTest, Rate: &ten}); err != nil {
		t.Fatal(err)
	}
	real = base.Add(time.Second)
	if got := c.Now(); !got.Equal(base.Add(10 * time.Second)) {
		t.Fatalf("x10: %v", got)
	}
	frozen := true
	_ = c.Apply(Change{Frozen: &frozen})
	real = base.Add(5 * time.Second)
	if got := c.Now(); !got.Equal(base.Add(10 * time.Second)) {
		t.Fatalf("frozen: %v", got)
	}
	if err := c.Apply(Change{AdvanceMS: 3600_000}); err != nil {
		t.Fatal(err)
	}
	if got := c.Now(); !got.Equal(base.Add(time.Hour + 10*time.Second)) {
		t.Fatalf("advance: %v", got)
	}
	// 과거 실제 시각 변환: 가속 구간의 0.5초 → 가상 5초
	if got := c.ToVirtual(base.Add(500 * time.Millisecond)); !got.Equal(base.Add(5 * time.Second)) {
		t.Fatalf("history mapping: %v", got)
	}
	if err := c.Apply(Change{AdvanceMS: -1}); err == nil {
		t.Fatal("cannot go back")
	}
	past := base
	if err := c.Apply(Change{AdvanceTo: &past}); err == nil {
		t.Fatal("advance_to must be in the future")
	}
	if c.Identity() || c.Status().Mode != ModeTest || !c.Status().Frozen {
		t.Fatalf("status: %+v", c.Status())
	}
	_ = c.Apply(Change{Mode: ModeNormal})
	if !c.Identity() || !c.Now().Equal(real) {
		t.Fatal("back to normal")
	}
}

// 가상 시간 대기는 시계 진행 알림으로 바로 깨어난다(폴링 없음).
func TestSleepWakesOnAdvance(t *testing.T) {
	c := New()
	_ = c.Apply(Change{Mode: ModeTest})
	frozen := true
	_ = c.Apply(Change{Frozen: &frozen})
	done := make(chan bool)
	go func() { done <- c.Sleep(time.Hour, nil) }()
	time.Sleep(50 * time.Millisecond)
	_ = c.Apply(Change{AdvanceMS: 3600_000})
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("sleep aborted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sleep did not wake on advance")
	}
}

// T-FR-BE-0611: 가상 시각 범위(예: 이벤트 시간 필터)를 실제 시각 범위로 되돌린다(가속·정지·진행 구간 포함).
func TestVirtualToRealRange(t *testing.T) {
	base := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	real := base
	c := &Clock{realNow: func() time.Time { return real }}
	c.Reset()
	ten := 10.0
	_ = c.Apply(Change{Mode: ModeTest, Rate: &ten}) // 실제 0~1초 → 가상 0~10초
	real = base.Add(time.Second)
	frozen := true
	_ = c.Apply(Change{Frozen: &frozen}) // 실제 1~3초 → 가상 10초 고정
	real = base.Add(3 * time.Second)
	_ = c.Apply(Change{AdvanceMS: 60_000}) // 가상 70초로 이동(정지 유지)
	real = base.Add(4 * time.Second)
	cases := []struct {
		v        time.Duration
		earliest time.Duration
		latest   time.Duration
	}{
		{5 * time.Second, 500 * time.Millisecond, 500 * time.Millisecond},
		{10 * time.Second, time.Second, 3*time.Second - time.Nanosecond}, // 정지 구간 전체가 가상 10초
		{30 * time.Second, 3 * time.Second, 3*time.Second - time.Nanosecond},
	}
	for _, tc := range cases {
		v := base.Add(tc.v)
		if got := c.ToRealEarliest(v); !got.Equal(base.Add(tc.earliest)) {
			t.Errorf("earliest(%v) = %v, want %v", tc.v, got.Sub(base), tc.earliest)
		}
		if got := c.ToRealLatest(v); !got.Equal(base.Add(tc.latest)) {
			t.Errorf("latest(%v) = %v, want %v", tc.v, got.Sub(base), tc.latest)
		}
		if r := c.ToRealEarliest(v); c.ToVirtual(r).Before(v) && tc.v != 30*time.Second {
			t.Errorf("earliest(%v) maps back before v", tc.v)
		}
	}
	// 마지막 정지 구간의 가상 70초: 끝은 열린 범위(현재 이후)
	if got := c.ToRealLatest(base.Add(70 * time.Second)); !got.After(real) {
		t.Errorf("latest in open frozen segment: %v", got.Sub(base))
	}
	// 가속 전 시각
	if got := c.ToRealEarliest(base.Add(-time.Minute)); !got.Equal(base.Add(-time.Minute)) {
		t.Errorf("before clock: %v", got.Sub(base))
	}
}
