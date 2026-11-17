package tarodomain

import (
	"context"
	"testing"
	"time"
)

func TestManualClock(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c := NewManualClock(start)
	c.Advance(time.Minute)
	if !c.Now().Equal(start.Add(time.Minute)) {
		t.Errorf("advance: %v", c.Now())
	}
	c.Set(start)
	if !c.Now().Equal(start) {
		t.Errorf("set: %v", c.Now())
	}
}

func TestManualClockConcurrent(t *testing.T) {
	c := NewManualClock(time.Unix(0, 0))
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.Advance(time.Millisecond)
				c.Now()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if got := c.Now().UnixMilli(); got != 800 {
		t.Errorf("got %d", got)
	}
}

func TestNowFromContext(t *testing.T) {
	loc := time.FixedZone("x", 7*3600)
	at := time.Date(2026, 5, 6, 7, 8, 9, 123_456_789, loc)
	ctx := WithClock(context.Background(), NewManualClock(at))
	got := Now(ctx)
	if got.Location() != time.UTC || got.Nanosecond() != 123_000_000 || !got.Equal(at.Truncate(time.Millisecond)) {
		t.Errorf("not UTC/ms: %v", got)
	}
	if d := time.Since(Now(context.Background())); d < -time.Second || d > time.Second {
		t.Errorf("system clock off by %v", d)
	}
}
