package tarodomain

import (
	"math"
	"math/rand/v2"
	"time"
)

// RandomSource supplies random numbers, so tests can fix the jitter.
type RandomSource interface {
	// Float64 returns a number in [0, 1).
	Float64() float64
}

type defaultRandom struct{}

func (defaultRandom) Float64() float64 { return rand.Float64() }

// Backoff computes retry delays: Initial * Multiplier^attempt, moved by up to
// ±Jitter (a fraction, 0.1 is ±10%), and never more than Max.
type Backoff struct {
	Initial    time.Duration
	Multiplier float64
	Max        time.Duration
	Jitter     float64
}

// Delay returns the wait before retry number attempt, counting from 0. A nil
// random uses math/rand/v2.
func (b Backoff) Delay(attempt int, random RandomSource) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	mult := b.Multiplier
	if mult <= 0 {
		mult = 1
	}
	d := float64(b.Initial) * math.Pow(mult, float64(attempt))
	if b.Jitter > 0 {
		if random == nil {
			random = defaultRandom{}
		}
		d *= 1 + b.Jitter*(2*random.Float64()-1)
	}
	if b.Max > 0 && d > float64(b.Max) {
		d = float64(b.Max)
	}
	if d < 0 {
		d = 0
	}
	if d >= math.MaxInt64 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(d)
}
