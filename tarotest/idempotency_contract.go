package tarotest

import (
	"bytes"
	"context"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// ProcessedEventsContract checks a provider's [tarodomain.ProcessedEventStore]
// and [tarodomain.CommandIdempotencyStore].
func ProcessedEventsContract(t T, newProvider Factory) {
	t.Helper()
	t.Run("processed events", func(t T) {
		ctx := context.Background()
		s, err := fresh(ctx, t, newProvider).CreateProcessedEventStore()
		noErr(t, err)
		exists := func(handler, instance string, n int) bool {
			ok, err := s.Exists(ctx, handler, instance, eventID(n))
			noErr(t, err)
			return ok
		}
		count := func(handler, instance string) int {
			n, err := s.Count(ctx, handler, instance)
			noErr(t, err)
			return n
		}

		noErr(t, s.MarkProcessed(ctx, "proj", "i1", eventID(1)))
		noErr(t, s.MarkProcessed(ctx, "proj", "i1", eventID(1))) // idempotent
		noErr(t, s.MarkProcessed(ctx, "proj", "i2", eventID(1)))
		noErr(t, s.MarkProcessed(ctx, "proj", "i1", eventID(2)))
		noErr(t, s.MarkProcessed(ctx, "saga", "x", eventID(1)))

		if !exists("proj", "i1", 1) || !exists("proj", "i2", 1) || !exists("saga", "x", 1) {
			t.Errorf("a marked event does not exist")
		}
		if exists("proj", "i2", 2) || exists("proj", "i3", 1) || exists("other", "i1", 1) {
			t.Errorf("an unmarked event exists")
		}
		if n := count("proj", ""); n != 3 {
			t.Errorf("count of every instance = %d", n)
		}
		if n := count("proj", "i1"); n != 2 {
			t.Errorf("count of i1 = %d", n)
		}
		if n := count("nobody", ""); n != 0 {
			t.Errorf("count of an unknown handler = %d", n)
		}

		if n, err := s.DeleteOld(ctx, farPast); err != nil || n != 0 {
			t.Errorf("DeleteOld(far past) = %d, %v", n, err)
		}
		if n, err := s.DeleteByHandler(ctx, "proj", "i2"); err != nil || n != 1 {
			t.Errorf("DeleteByHandler(instance) = %d, %v", n, err)
		}
		if exists("proj", "i2", 1) || !exists("proj", "i1", 1) {
			t.Errorf("DeleteByHandler removed the wrong marks")
		}
		if n, _ := s.DeleteByHandler(ctx, "proj", ""); n != 2 {
			t.Errorf("DeleteByHandler(all instances) = %d", n)
		}
		if n, _ := s.DeleteByHandler(ctx, "proj", ""); n != 0 {
			t.Errorf("a second DeleteByHandler removed %d", n)
		}
		if !exists("saga", "x", 1) {
			t.Errorf("DeleteByHandler removed another handler's mark")
		}
		if n, err := s.DeleteOld(ctx, farFuture); err != nil || n != 1 {
			t.Errorf("DeleteOld(far future) = %d, %v", n, err)
		}
		if exists("saga", "x", 1) {
			t.Errorf("DeleteOld left a mark")
		}
	})
}

// CommandIdempotencyContract checks a provider's
// [tarodomain.CommandIdempotencyStore].
func CommandIdempotencyContract(t T, newProvider Factory) {
	t.Helper()
	each := func(name string, f func(t T, ctx context.Context, s tarodomain.CommandIdempotencyStore)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			s, err := fresh(ctx, t, newProvider).CreateCommandIdempotencyStore()
			if err != nil {
				t.Fatalf("CreateCommandIdempotencyStore: %v", err)
			}
			f(t, ctx, s)
		})
	}

	each("marking twice keeps the first record", func(t T, ctx context.Context, s tarodomain.CommandIdempotencyStore) {
		if ok, _ := s.Exists(ctx, otherID(1)); ok {
			t.Errorf("an unmarked command exists")
		}
		if _, ok, _ := s.ProcessedAt(ctx, otherID(1)); ok {
			t.Errorf("an unmarked command has a time")
		}
		noErr(t, s.MarkProcessed(ctx, otherID(1), "Deposit", "first"))
		at, ok, err := s.ProcessedAt(ctx, otherID(1))
		noErr(t, err)
		if !ok || at.IsZero() {
			t.Fatalf("ProcessedAt = %v, %v", at, ok)
		}
		noErr(t, s.MarkProcessed(ctx, otherID(1), "Withdraw", "second"))
		if again, _, _ := s.ProcessedAt(ctx, otherID(1)); !again.Equal(at) {
			t.Errorf("the time changed from %v to %v", at, again)
		}
		list, _ := s.List(ctx, "", 0)
		if len(list) != 1 || list[0].CommandType != "Deposit" || list[0].HandlerName != "first" || list[0].CommandID != otherID(1) {
			t.Errorf("list = %+v", list)
		}
		if ok, _ := s.Exists(ctx, otherID(1)); !ok {
			t.Errorf("a marked command does not exist")
		}
	})

	each("list and count", func(t T, ctx context.Context, s tarodomain.CommandIdempotencyStore) {
		for n := 1; n <= 4; n++ {
			handler := "h"
			if n == 4 {
				handler = "g"
			}
			noErr(t, s.MarkProcessed(ctx, otherID(n), "Cmd", handler))
		}
		for _, c := range []struct {
			handler string
			want    int
		}{{"", 4}, {"h", 3}, {"g", 1}, {"nobody", 0}} {
			if n, err := s.Count(ctx, c.handler); err != nil || n != c.want {
				t.Errorf("Count(%q) = %d, %v; want %d", c.handler, n, err, c.want)
			}
		}
		list, err := s.List(ctx, "h", 0)
		noErr(t, err)
		if len(list) != 3 {
			t.Fatalf("list = %d", len(list))
		}
		for i := 1; i < len(list); i++ {
			a, b := list[i-1], list[i]
			if b.ProcessedAt.Before(a.ProcessedAt) ||
				(b.ProcessedAt.Equal(a.ProcessedAt) && bytes.Compare(b.CommandID[:], a.CommandID[:]) < 0) {
				t.Errorf("list is out of order at %d", i)
			}
		}
		if limited, _ := s.List(ctx, "", 2); len(limited) != 2 {
			t.Errorf("limit = %d", len(limited))
		}
	})

	each("DeleteOld is exclusive of its cut-off", func(t T, ctx context.Context, s tarodomain.CommandIdempotencyStore) {
		noErr(t, s.MarkProcessed(ctx, otherID(1), "Cmd", "h"))
		at, _, _ := s.ProcessedAt(ctx, otherID(1))
		for _, c := range []struct {
			cutoff time.Time
			want   int
		}{{farPast, 0}, {at, 0}, {at.Add(time.Millisecond), 1}} {
			if n, err := s.DeleteOld(ctx, c.cutoff); err != nil || n != c.want {
				t.Errorf("DeleteOld(%v) = %d, %v; want %d", c.cutoff, n, err, c.want)
			}
		}
		if ok, _ := s.Exists(ctx, otherID(1)); ok {
			t.Errorf("the command is still marked")
		}
		noErr(t, s.MarkProcessed(ctx, otherID(2), "Cmd", "h"))
		if n, _ := s.DeleteOld(ctx, farFuture); n != 1 {
			t.Errorf("DeleteOld(far future) = %d", n)
		}
	})
}
