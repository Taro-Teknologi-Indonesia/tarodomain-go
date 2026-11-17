package tarotest

import (
	"context"
	"testing"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// T is the part of *testing.T the suites use. Fatalf and Skipf stop the
// current case, as they do in package testing.
type T interface {
	// Helper marks the caller as a test helper.
	Helper()
	// Logf records a message.
	Logf(format string, args ...any)
	// Errorf records a failure and carries on.
	Errorf(format string, args ...any)
	// Fatalf records a failure and stops the case.
	Fatalf(format string, args ...any)
	// Skipf stops the case without failing it.
	Skipf(format string, args ...any)
	// Run runs f as a named case of its own.
	Run(name string, f func(t T))
}

// Factory returns a provider with empty storage, ready to use. Each case asks
// for a fresh one.
type Factory func(ctx context.Context) (tarodomain.Provider, error)

type goT struct{ *testing.T }

// Wrap adapts a *testing.T to [T].
func Wrap(t *testing.T) T { return goT{t} }

// Run implements [T].
func (g goT) Run(name string, f func(t T)) {
	g.T.Run(name, func(t *testing.T) { f(Wrap(t)) })
}
