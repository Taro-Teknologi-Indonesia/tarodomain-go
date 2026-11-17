package tarodomain

import (
	"context"
	"sync"
	"time"
)

// Clock tells the time. Tests replace it to control timestamps.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the real time.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// ManualClock is a Clock that only moves when told to. It is safe for
// concurrent use.
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewManualClock returns a clock set to t.
func NewManualClock(t time.Time) *ManualClock { return &ManualClock{now: t} }

// Now returns the clock's current time.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Set moves the clock to t.
func (c *ManualClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

// Advance moves the clock forward by d.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type clockKey struct{}

// WithClock returns a context whose Now uses clock.
func WithClock(ctx context.Context, clock Clock) context.Context {
	return context.WithValue(ctx, clockKey{}, clock)
}

// Now returns the time from the clock in ctx, or the system clock if there is
// none. The result is in UTC and cut to whole milliseconds.
func Now(ctx context.Context) time.Time {
	var c Clock = SystemClock{}
	if v, ok := ctx.Value(clockKey{}).(Clock); ok && v != nil {
		c = v
	}
	return NormalizeTime(c.Now())
}

// NormalizeTime converts t to UTC and cuts it to whole milliseconds, the
// precision every store keeps.
func NormalizeTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}
