package tarodomain

import (
	"testing"
	"time"
)

func TestSagaStatusRoundTrip(t *testing.T) {
	for _, s := range []SagaStatus{SagaRunning, SagaCompensating, SagaCompleted, SagaFailed} {
		got, err := ParseSagaStatus(s.String())
		if err != nil || got != s {
			t.Errorf("%s: got %q, %v", s, got, err)
		}
		if s.IsActive() == s.IsTerminal() {
			t.Errorf("%s: active and terminal must differ", s)
		}
	}
	if _, err := ParseSagaStatus("running"); !isCode(err, CodeSerializationError) {
		t.Errorf("lower-case status: %v", err)
	}
}

func TestSagaSavedTimestamps(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1, t2 := t0.Add(time.Second), t0.Add(2*time.Second)

	first := SagaRecord{}.Saved(SagaSaveInput{Status: SagaRunning}, t0)
	if !first.StartedAt.Equal(t0) || !first.UpdatedAt.Equal(t0) || !first.CompletedAt.IsZero() {
		t.Fatalf("first save: %+v", first)
	}
	second := first.Saved(SagaSaveInput{Status: SagaCompensating}, t1)
	if !second.StartedAt.Equal(t0) || !second.UpdatedAt.Equal(t1) || !second.CompletedAt.IsZero() {
		t.Fatalf("second save: %+v", second)
	}
	done := second.Saved(SagaSaveInput{Status: SagaCompleted}, t2)
	if !done.StartedAt.Equal(t0) || !done.CompletedAt.Equal(t2) {
		t.Fatalf("terminal save: %+v", done)
	}
	again := done.Saved(SagaSaveInput{Status: SagaFailed}, t2.Add(time.Second))
	if !again.CompletedAt.Equal(t2) {
		t.Errorf("CompletedAt moved: %v", again.CompletedAt)
	}
	reopened := done.Saved(SagaSaveInput{Status: SagaRunning}, t2.Add(time.Second))
	if !reopened.CompletedAt.IsZero() {
		t.Errorf("CompletedAt not cleared: %v", reopened.CompletedAt)
	}
}

func TestSagaEndOnlyOnce(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	r := SagaRecord{}.Saved(SagaSaveInput{Status: SagaRunning}, t0)

	done, changed := r.Complete(t1)
	if !changed || done.Status != SagaCompleted || !done.CompletedAt.Equal(t1) || !done.UpdatedAt.Equal(t1) {
		t.Fatalf("complete: %+v %v", done, changed)
	}
	if again, changed := done.Complete(t1.Add(time.Hour)); changed || !sameEnd(again, done) {
		t.Errorf("second complete changed the record")
	}
	if again, changed := done.Fail("late", t1.Add(time.Hour)); changed || !sameEnd(again, done) {
		t.Errorf("fail after complete changed the record")
	}

	failed, changed := r.Fail("boom", t1)
	if !changed || failed.Status != SagaFailed || failed.FailureReason != "boom" || !failed.CompletedAt.Equal(t1) {
		t.Fatalf("fail: %+v %v", failed, changed)
	}
	if _, changed := failed.Fail("again", t1); changed {
		t.Error("second fail changed the record")
	}
	if r.Status != SagaRunning {
		t.Error("original record was modified")
	}
}

func TestSagaIsTimedOut(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		status SagaStatus
		before time.Time
		want   bool
	}{
		{SagaRunning, t0.Add(time.Second), true},
		{SagaCompensating, t0.Add(time.Second), true},
		{SagaRunning, t0, false},
		{SagaRunning, t0.Add(-time.Second), false},
		{SagaCompleted, t0.Add(time.Hour), false},
		{SagaFailed, t0.Add(time.Hour), false},
	} {
		r := SagaRecord{SagaSaveInput: SagaSaveInput{Status: c.status}, UpdatedAt: t0}
		if got := r.IsTimedOut(c.before); got != c.want {
			t.Errorf("%s before %v: %v, want %v", c.status, c.before, got, c.want)
		}
	}
}

func sameEnd(a, b SagaRecord) bool {
	return a.Status == b.Status && a.FailureReason == b.FailureReason &&
		a.UpdatedAt.Equal(b.UpdatedAt) && a.CompletedAt.Equal(b.CompletedAt)
}
