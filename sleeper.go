package tarodomain

import (
	"context"
	"sync"
	"time"
)

// Sleeper waits for a duration. It returns the context's error if the context
// ends first, so a retry loop stops as soon as its caller cancels.
type Sleeper interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// TimerSleeper waits with a real timer.
type TimerSleeper struct{}

// Sleep waits for d or until ctx ends.
func (TimerSleeper) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RecordingSleeper records the requested delays and returns at once, for
// tests. It is safe for concurrent use.
type RecordingSleeper struct {
	mu     sync.Mutex
	delays []time.Duration
}

// Sleep records d. It returns the context's error if ctx has ended.
func (s *RecordingSleeper) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.delays = append(s.delays, d)
	s.mu.Unlock()
	return nil
}

// Delays returns a copy of the recorded delays.
func (s *RecordingSleeper) Delays() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.delays...)
}
