package tarodomain

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fixedRandom float64

func (f fixedRandom) Float64() float64 { return float64(f) }

func TestBackoffNoJitter(t *testing.T) {
	b := Backoff{Initial: 10 * time.Millisecond, Multiplier: 2, Max: time.Second}
	want := []time.Duration{10, 20, 40, 80}
	for i, w := range want {
		if got := b.Delay(i, nil); got != w*time.Millisecond {
			t.Errorf("attempt %d: %v", i, got)
		}
	}
}

func TestBackoffCap(t *testing.T) {
	b := Backoff{Initial: 100 * time.Millisecond, Multiplier: 10, Max: time.Second, Jitter: 0.1}
	if got := b.Delay(5, fixedRandom(0.99)); got != time.Second {
		t.Errorf("not capped: %v", got)
	}
	if got := b.Delay(1000, nil); got != time.Second {
		t.Errorf("overflow not capped: %v", got)
	}
}

func TestBackoffJitterBounds(t *testing.T) {
	b := Backoff{Initial: 100 * time.Millisecond, Multiplier: 2, Max: time.Hour, Jitter: 0.1}
	if got := b.Delay(0, fixedRandom(0)); got != 90*time.Millisecond {
		t.Errorf("low: %v", got)
	}
	if got := b.Delay(0, fixedRandom(0.5)); got != 100*time.Millisecond {
		t.Errorf("mid: %v", got)
	}
	for i := 0; i < 200; i++ {
		got := b.Delay(0, nil)
		if got < 90*time.Millisecond || got > 110*time.Millisecond {
			t.Fatalf("out of ±10%%: %v", got)
		}
	}
}

func TestTimerSleeper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	if err := (TimerSleeper{}).Sleep(ctx, time.Millisecond); err != nil {
		t.Error(err)
	}
	cancel()
	start := time.Now()
	if err := (TimerSleeper{}).Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Error("cancelled sleep did not return at once")
	}
}

func TestTimerSleeperCancelsMidSleep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := (TimerSleeper{}).Sleep(ctx, time.Hour); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v", err)
	}
}

func TestRecordingSleeper(t *testing.T) {
	s := &RecordingSleeper{}
	ctx := context.Background()
	_ = s.Sleep(ctx, 10*time.Millisecond)
	_ = s.Sleep(ctx, 20*time.Millisecond)
	if d := s.Delays(); len(d) != 2 || d[0] != 10*time.Millisecond || d[1] != 20*time.Millisecond {
		t.Errorf("delays: %v", d)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Sleep(cctx, time.Second); !errors.Is(err, context.Canceled) || len(s.Delays()) != 2 {
		t.Errorf("cancelled: %v %v", err, s.Delays())
	}
}
