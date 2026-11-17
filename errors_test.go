package tarodomain

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestErrorCodeCounts(t *testing.T) {
	counts := map[ErrorCategory]int{}
	for _, c := range ErrorCodes() {
		cat, ok := c.Category()
		if !ok {
			t.Fatalf("%s has no category", c)
		}
		counts[cat]++
	}
	want := map[ErrorCategory]int{
		CategoryDomain: 19, CategoryConcurrency: 3, CategoryInfrastructure: 18, CategoryConfiguration: 11,
	}
	for cat, n := range want {
		if counts[cat] != n {
			t.Errorf("%s: got %d codes, want %d", cat, counts[cat], n)
		}
	}
	if len(ErrorCodes()) != 51 {
		t.Errorf("got %d codes, want 51", len(ErrorCodes()))
	}
}

func TestCategoryMatrix(t *testing.T) {
	for _, c := range ErrorCodes() {
		e := NewError(c, "m")
		cat, _ := c.Category()
		var retry bool
		var status int
		switch cat {
		case CategoryDomain:
			retry, status = false, 400
		case CategoryConcurrency:
			retry, status = true, 409
		case CategoryInfrastructure:
			retry, status = true, 500
		case CategoryConfiguration:
			retry, status = false, 500
		}
		if e.Retryable() != retry || e.HTTPStatus() != status || ShouldRetry(e) != retry {
			t.Errorf("%s: retryable=%v status=%d", c, e.Retryable(), e.HTTPStatus())
		}
	}
}

func TestParseErrorCodeRoundTrip(t *testing.T) {
	for _, c := range ErrorCodes() {
		got, err := ParseErrorCode(c.String())
		if err != nil || got != c {
			t.Errorf("%s: got %q, %v", c, got, err)
		}
	}
	if _, err := ParseErrorCode("NOPE"); err == nil {
		t.Error("unknown code parsed")
	}
}

func TestCustomCodeKeepsCategory(t *testing.T) {
	e := NewDomainError("INSUFFICIENT_FUNDS", "no")
	if e.Category != CategoryDomain || e.Retryable() || e.HTTPStatus() != 400 {
		t.Errorf("bad domain error: %+v", e)
	}
	if NewConcurrencyError("X", "").HTTPStatus() != 409 ||
		!NewInfrastructureError("X", "").Retryable() ||
		NewConfigurationError("X", "").Retryable() {
		t.Error("sibling constructors have the wrong category")
	}
}

func TestIsAndAs(t *testing.T) {
	base := OptimisticConcurrency("s-1", 2, 3)
	wrapped := fmt.Errorf("saving: %w", base)
	if !errors.Is(wrapped, ErrCode(CodeOptimisticConcurrency)) {
		t.Error("errors.Is by code failed")
	}
	if errors.Is(wrapped, ErrCode(CodeAggregateNotFound)) {
		t.Error("matched the wrong code")
	}
	if !errors.Is(wrapped, base) {
		t.Error("errors.Is by identity failed")
	}
	e, ok := AsError(wrapped)
	if !ok || e.Context["streamId"] != "s-1" || e.Context["expectedVersion"] != int64(2) || e.Context["actualVersion"] != int64(3) {
		t.Errorf("AsError: %+v %v", e, ok)
	}
	if c, ok := CodeOf(wrapped); !ok || c != CodeOptimisticConcurrency {
		t.Errorf("CodeOf: %q %v", c, ok)
	}
	if _, ok := CodeOf(errors.New("x")); ok {
		t.Error("CodeOf matched a foreign error")
	}
}

func TestWithReturnsCopies(t *testing.T) {
	base := NewError(CodeEventStoreError, "m").WithContext("a", 1)
	other := base.WithContext("b", 2)
	if _, ok := base.Context["b"]; ok {
		t.Error("WithContext changed the original")
	}
	if other.Context["a"] != 1 || other.Context["b"] != 2 {
		t.Errorf("copy lost entries: %v", other.Context)
	}
	cause := errors.New("boom")
	c := base.WithCause(cause)
	if base.Unwrap() != nil || !errors.Is(c, cause) {
		t.Error("WithCause")
	}
}

func TestShouldRetry(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"foreign", errors.New("net down"), true},
		{"domain", NewDomainError("X", ""), false},
		{"concurrency", OptimisticConcurrency("s", 1, 2), true},
		{"canceled", context.Canceled, false},
		{"deadline", fmt.Errorf("w: %w", context.DeadlineExceeded), false},
		{"canceled inside error", Infrastructure(context.Canceled), false},
	}
	for _, c := range cases {
		if got := ShouldRetry(c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestInfrastructure(t *testing.T) {
	if Infrastructure(nil) != nil {
		t.Error("nil changed")
	}
	foreign := errors.New("driver")
	w := Infrastructure(foreign)
	if e, ok := AsError(w); !ok || e.Category != CategoryInfrastructure || !errors.Is(w, foreign) {
		t.Errorf("wrap: %v", w)
	}
	own := NewDomainError("X", "")
	if Infrastructure(own) != error(own) {
		t.Error("an *Error was rewrapped")
	}
}
