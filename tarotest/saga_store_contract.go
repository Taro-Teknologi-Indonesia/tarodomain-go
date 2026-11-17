package tarotest

import (
	"bytes"
	"context"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// SagaStoreContract checks a provider's [tarodomain.SagaStoreAdapter].
func SagaStoreContract(t T, newProvider Factory) {
	t.Helper()
	each := func(name string, f func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			s, err := fresh(ctx, t, newProvider).CreateSagaStoreAdapter()
			if err != nil {
				t.Fatalf("CreateSagaStoreAdapter: %v", err)
			}
			f(t, ctx, s)
		})
	}
	input := func(typ string, n int, status tarodomain.SagaStatus) tarodomain.SagaSaveInput {
		return tarodomain.SagaSaveInput{SagaID: otherID(n), SagaType: typ, CorrelationID: otherID(n),
			State: []byte(`{"step":1}`), Status: status, CurrentStep: "reserve"}
	}
	load := func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter, typ string, n int) tarodomain.SagaRecord {
		t.Helper()
		r, ok, err := s.Load(ctx, typ, otherID(n))
		noErr(t, err)
		if !ok {
			t.Fatalf("saga %s/%d not found", typ, n)
		}
		return r
	}
	// ordered checks the list order: UpdatedAt, then type, then ID.
	ordered := func(t T, rs []tarodomain.SagaRecord) {
		t.Helper()
		for i := 1; i < len(rs); i++ {
			a, b := rs[i-1], rs[i]
			bad := b.UpdatedAt.Before(a.UpdatedAt) ||
				(b.UpdatedAt.Equal(a.UpdatedAt) && (b.SagaType < a.SagaType ||
					(b.SagaType == a.SagaType && bytes.Compare(b.SagaID[:], a.SagaID[:]) < 0)))
			if bad {
				t.Errorf("records %d and %d are out of order", i-1, i)
			}
		}
	}
	ids := func(rs []tarodomain.SagaRecord) map[tarodomain.UUID]bool {
		m := map[tarodomain.UUID]bool{}
		for _, r := range rs {
			m[r.SagaID] = true
		}
		return m
	}

	each("a saved saga comes back with its fields", func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter) {
		in := input("Transfer", 1, tarodomain.SagaRunning)
		noErr(t, s.Save(ctx, in))
		r := load(t, ctx, s, "Transfer", 1)
		if r.SagaID != in.SagaID || r.SagaType != "Transfer" || r.CorrelationID != in.CorrelationID ||
			!sameJSON(r.State, in.State) || r.Status != tarodomain.SagaRunning || r.CurrentStep != "reserve" {
			t.Errorf("record = %+v", r)
		}
		if r.StartedAt.IsZero() || r.UpdatedAt.Before(r.StartedAt) || !r.CompletedAt.IsZero() {
			t.Errorf("timestamps = %v %v %v", r.StartedAt, r.UpdatedAt, r.CompletedAt)
		}
		if _, ok, _ := s.Load(ctx, "Other", otherID(1)); ok {
			t.Errorf("found a saga of another type")
		}
		byCorr, ok, err := s.LoadByCorrelationID(ctx, "Transfer", otherID(1))
		noErr(t, err)
		if !ok || byCorr.SagaID != in.SagaID {
			t.Errorf("by correlation = %+v", byCorr)
		}
		if _, ok, _ := s.LoadByCorrelationID(ctx, "Transfer", otherID(2)); ok {
			t.Errorf("found a saga for an unknown correlation ID")
		}
	})

	each("saving again keeps StartedAt and moves UpdatedAt", func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter) {
		noErr(t, s.Save(ctx, input("Transfer", 1, tarodomain.SagaRunning)))
		first := load(t, ctx, s, "Transfer", 1)
		in := input("Transfer", 1, tarodomain.SagaCompensating)
		in.State, in.CurrentStep = []byte(`{"step":2}`), "refund"
		noErr(t, s.Save(ctx, in))
		second := load(t, ctx, s, "Transfer", 1)
		if !second.StartedAt.Equal(first.StartedAt) {
			t.Errorf("StartedAt changed")
		}
		if second.UpdatedAt.Before(first.UpdatedAt) {
			t.Errorf("UpdatedAt moved back")
		}
		if second.Status != tarodomain.SagaCompensating || second.CurrentStep != "refund" || !sameJSON(second.State, in.State) {
			t.Errorf("record = %+v", second)
		}
		if n, _ := s.Count(ctx, "", ""); n != 1 {
			t.Errorf("count = %d", n)
		}
	})

	each("a terminal status sets CompletedAt and an active one clears it", func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter) {
		noErr(t, s.Save(ctx, input("Transfer", 1, tarodomain.SagaFailed)))
		failed := load(t, ctx, s, "Transfer", 1)
		if failed.CompletedAt.IsZero() {
			t.Errorf("a FAILED save left CompletedAt zero")
		}
		noErr(t, s.Save(ctx, input("Transfer", 1, tarodomain.SagaFailed)))
		if again := load(t, ctx, s, "Transfer", 1); !again.CompletedAt.Equal(failed.CompletedAt) {
			t.Errorf("CompletedAt moved on a second terminal save")
		}
		noErr(t, s.Save(ctx, input("Transfer", 1, tarodomain.SagaRunning)))
		if back := load(t, ctx, s, "Transfer", 1); !back.CompletedAt.IsZero() {
			t.Errorf("an active save kept CompletedAt")
		}
	})

	each("Complete and Fail end a saga once", func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter) {
		noErr(t, s.Save(ctx, input("Transfer", 1, tarodomain.SagaRunning)))
		noErr(t, s.Save(ctx, input("Transfer", 2, tarodomain.SagaRunning)))
		noErr(t, s.Complete(ctx, "Transfer", otherID(1)))
		done := load(t, ctx, s, "Transfer", 1)
		if done.Status != tarodomain.SagaCompleted || done.CompletedAt.IsZero() {
			t.Errorf("completed = %+v", done)
		}
		noErr(t, s.Complete(ctx, "Transfer", otherID(1)))
		noErr(t, s.Fail(ctx, "Transfer", otherID(1), "too late"))
		again := load(t, ctx, s, "Transfer", 1)
		if again.Status != tarodomain.SagaCompleted || again.FailureReason != "" ||
			!again.CompletedAt.Equal(done.CompletedAt) || !again.UpdatedAt.Equal(done.UpdatedAt) {
			t.Errorf("a finished saga changed: %+v", again)
		}
		noErr(t, s.Fail(ctx, "Transfer", otherID(2), "card declined"))
		failed := load(t, ctx, s, "Transfer", 2)
		if failed.Status != tarodomain.SagaFailed || failed.FailureReason != "card declined" || failed.CompletedAt.IsZero() {
			t.Errorf("failed = %+v", failed)
		}
		noErr(t, s.Complete(ctx, "Transfer", otherID(2)))
		if still := load(t, ctx, s, "Transfer", 2); still.Status != tarodomain.SagaFailed {
			t.Errorf("a failed saga became %s", still.Status)
		}
		// A missing saga is a no-op.
		noErr(t, s.Complete(ctx, "Transfer", otherID(99)))
		noErr(t, s.Fail(ctx, "Transfer", otherID(99), "x"))
		if _, ok, _ := s.Load(ctx, "Transfer", otherID(99)); ok {
			t.Errorf("ending a missing saga created it")
		}
	})

	each("LoadByStatus is ordered and filters by type before the limit", func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter) {
		for n := 1; n <= 3; n++ {
			noErr(t, s.Save(ctx, input("SagaA", n, tarodomain.SagaRunning)))
		}
		noErr(t, s.Save(ctx, input("SagaB", 4, tarodomain.SagaRunning)))
		noErr(t, s.Save(ctx, input("SagaB", 5, tarodomain.SagaCompensating)))
		noErr(t, s.Save(ctx, input("SagaB", 6, tarodomain.SagaCompleted)))

		running, err := s.LoadByStatus(ctx, tarodomain.SagaRunning, tarodomain.SagaQuery{})
		noErr(t, err)
		if len(running) != 4 {
			t.Errorf("running = %d", len(running))
		}
		ordered(t, running)
		onlyB, _ := s.LoadByStatus(ctx, tarodomain.SagaRunning, tarodomain.SagaQuery{SagaType: "SagaB"})
		if len(onlyB) != 1 || onlyB[0].SagaType != "SagaB" {
			t.Errorf("type filter = %+v", onlyB)
		}
		// SagaB sorts after SagaA, so applying the limit first would hide it.
		limited, _ := s.LoadByStatus(ctx, tarodomain.SagaRunning, tarodomain.SagaQuery{SagaType: "SagaB", Limit: 1})
		if len(limited) != 1 || limited[0].SagaType != "SagaB" {
			t.Errorf("filter before limit = %+v", limited)
		}
		if two, _ := s.LoadByStatus(ctx, tarodomain.SagaRunning, tarodomain.SagaQuery{Limit: 2}); len(two) != 2 {
			t.Errorf("limit = %d", len(two))
		}
		if done, _ := s.LoadByStatus(ctx, tarodomain.SagaCompleted, tarodomain.SagaQuery{}); len(done) != 1 {
			t.Errorf("completed = %d", len(done))
		}
	})

	each("LoadTimedOut returns only active sagas before the cut-off", func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter) {
		noErr(t, s.Save(ctx, input("SagaA", 1, tarodomain.SagaRunning)))
		noErr(t, s.Save(ctx, input("SagaA", 2, tarodomain.SagaCompensating)))
		noErr(t, s.Save(ctx, input("SagaA", 3, tarodomain.SagaCompleted)))
		noErr(t, s.Save(ctx, input("SagaA", 4, tarodomain.SagaFailed)))
		noErr(t, s.Save(ctx, input("SagaB", 5, tarodomain.SagaRunning)))

		got, err := s.LoadTimedOut(ctx, farFuture, tarodomain.SagaQuery{})
		noErr(t, err)
		m := ids(got)
		if len(got) != 3 || !m[otherID(1)] || !m[otherID(2)] || !m[otherID(5)] {
			t.Errorf("timed out = %+v", got)
		}
		ordered(t, got)
		if got, _ = s.LoadTimedOut(ctx, farPast, tarodomain.SagaQuery{}); len(got) != 0 {
			t.Errorf("far past = %+v", got)
		}
		if got, _ = s.LoadTimedOut(ctx, farFuture, tarodomain.SagaQuery{SagaType: "SagaB", Limit: 1}); len(got) != 1 || got[0].SagaType != "SagaB" {
			t.Errorf("type filter before limit = %+v", got)
		}
		// The cut-off is exclusive: a saga updated at exactly that time is not
		// timed out. SagaB's record is the only one that can be isolated by type.
		b := load(t, ctx, s, "SagaB", 5)
		got, _ = s.LoadTimedOut(ctx, b.UpdatedAt, tarodomain.SagaQuery{SagaType: "SagaB"})
		if len(got) != 0 {
			t.Errorf("cut-off at UpdatedAt returned %+v", got)
		}
		got, _ = s.LoadTimedOut(ctx, b.UpdatedAt.Add(time.Millisecond), tarodomain.SagaQuery{SagaType: "SagaB"})
		if len(got) != 1 {
			t.Errorf("cut-off just after UpdatedAt returned %+v", got)
		}
	})

	each("Delete and Count", func(t T, ctx context.Context, s tarodomain.SagaStoreAdapter) {
		noErr(t, s.Save(ctx, input("SagaA", 1, tarodomain.SagaRunning)))
		noErr(t, s.Save(ctx, input("SagaA", 2, tarodomain.SagaFailed)))
		noErr(t, s.Save(ctx, input("SagaB", 3, tarodomain.SagaRunning)))
		for _, c := range []struct {
			typ    string
			status tarodomain.SagaStatus
			want   int
		}{
			{"", "", 3}, {"SagaA", "", 2}, {"", tarodomain.SagaRunning, 2},
			{"SagaA", tarodomain.SagaRunning, 1}, {"SagaB", tarodomain.SagaFailed, 0},
		} {
			if n, err := s.Count(ctx, c.typ, c.status); err != nil || n != c.want {
				t.Errorf("Count(%q, %q) = %d, %v; want %d", c.typ, c.status, n, err, c.want)
			}
		}
		noErr(t, s.Delete(ctx, "SagaA", otherID(2)))
		noErr(t, s.Delete(ctx, "SagaA", otherID(2)))
		if _, ok, _ := s.Load(ctx, "SagaA", otherID(2)); ok {
			t.Errorf("saga not deleted")
		}
		if n, _ := s.Count(ctx, "", ""); n != 2 {
			t.Errorf("count after delete = %d", n)
		}
	})
}
