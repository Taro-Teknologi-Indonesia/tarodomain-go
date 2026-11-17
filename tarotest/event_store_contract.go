package tarotest

import (
	"context"
	"errors"
	"sync"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// hasCode reports whether err is a framework error with the code.
func hasCode(err error, code tarodomain.ErrorCode) bool {
	return errors.Is(err, tarodomain.ErrCode(code))
}

// EventStoreContract checks a provider's [tarodomain.EventStoreAdapter].
func EventStoreContract(t T, newProvider Factory) {
	t.Helper()
	// each runs one case on a fresh adapter.
	each := func(name string, f func(t T, ctx context.Context, s tarodomain.EventStoreAdapter)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			s, err := fresh(ctx, t, newProvider).CreateEventStoreAdapter()
			if err != nil {
				t.Fatalf("CreateEventStoreAdapter: %v", err)
			}
			f(t, ctx, s)
		})
	}
	must := func(t T, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	all := func(t T, ctx context.Context, s tarodomain.EventStoreAdapter, after tarodomain.UUID) []int {
		t.Helper()
		es, err := s.LoadAll(ctx, after, 0)
		must(t, err)
		return numbers(es)
	}

	each("versions count from 1 and a missing stream is 0", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		sid := newStream("Account")
		v, err := s.StreamVersion(ctx, sid)
		must(t, err)
		if v != 0 {
			t.Errorf("missing stream version = %d", v)
		}
		if ok, _ := s.StreamExists(ctx, sid); ok {
			t.Errorf("missing stream exists")
		}
		if _, ok, _ := s.StreamMetadata(ctx, sid); ok {
			t.Errorf("missing stream has metadata")
		}
		res, err := s.AppendToStream(ctx, sid, events(1, 2), 0)
		must(t, err)
		if res.StreamID != sid || res.PreviousVersion != 0 || res.NewVersion != 2 {
			t.Errorf("result = %+v", res)
		}
		res, err = s.AppendToStream(ctx, sid, events(3), 2)
		must(t, err)
		if res.PreviousVersion != 2 || res.NewVersion != 3 {
			t.Errorf("second result = %+v", res)
		}
		if v, _ := s.StreamVersion(ctx, sid); v != 3 {
			t.Errorf("version = %d", v)
		}
		if ok, _ := s.StreamExists(ctx, sid); !ok {
			t.Errorf("stream does not exist")
		}
		md, ok, err := s.StreamMetadata(ctx, sid)
		must(t, err)
		if !ok || md.StreamID != sid || md.Version != 3 || md.CreatedAt.IsZero() || md.UpdatedAt.Before(md.CreatedAt) {
			t.Errorf("metadata = %+v", md)
		}
	})

	each("a stale expected version conflicts", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		sid := newStream("Account")
		_, err := s.AppendToStream(ctx, sid, events(1, 2), 0)
		must(t, err)
		for _, expected := range []int64{0, 1, 3} {
			_, err = s.AppendToStream(ctx, sid, events(9), expected)
			if !hasCode(err, tarodomain.CodeOptimisticConcurrency) {
				t.Fatalf("expected %d: err = %v", expected, err)
			}
		}
		_, err = s.AppendToStream(ctx, sid, events(9), 0)
		e, ok := tarodomain.AsError(err)
		if !ok {
			t.Fatalf("not a framework error: %v", err)
		}
		if e.Context["streamId"] != sid || e.Context["expectedVersion"] != int64(0) || e.Context["actualVersion"] != int64(2) {
			t.Errorf("context = %v", e.Context)
		}
		if got := all(t, ctx, s, tarodomain.UUID{}); !sameInts(got, []int{1, 2}) {
			t.Errorf("a conflict changed the store: %v", got)
		}
	})

	each("of two racing appends exactly one wins", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		sid := newStream("Account")
		const racers = 4
		errs := make([]error, racers)
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = s.AppendToStream(ctx, sid, events(10+i), 0)
			}()
		}
		wg.Wait()
		wins := 0
		for _, err := range errs {
			switch {
			case err == nil:
				wins++
			case !hasCode(err, tarodomain.CodeOptimisticConcurrency):
				t.Errorf("a loser failed with %v", err)
			}
		}
		if wins != 1 {
			t.Errorf("%d appends won", wins)
		}
		if v, _ := s.StreamVersion(ctx, sid); v != 1 {
			t.Errorf("version = %d", v)
		}
	})

	each("an empty append checks the version and writes nothing", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		sid := newStream("Account")
		_, err := s.AppendToStream(ctx, sid, nil, 0)
		must(t, err)
		if ok, _ := s.StreamExists(ctx, sid); ok {
			t.Errorf("an empty append created the stream")
		}
		if _, err = s.AppendToStream(ctx, sid, nil, 3); !hasCode(err, tarodomain.CodeOptimisticConcurrency) {
			t.Errorf("stale empty append: err = %v", err)
		}
		_, err = s.AppendToStream(ctx, sid, events(1), 0)
		must(t, err)
		if _, err = s.AppendToStream(ctx, sid, nil, 0); !hasCode(err, tarodomain.CodeOptimisticConcurrency) {
			t.Errorf("stale empty append on a real stream: err = %v", err)
		}
		if _, err = s.AppendToStream(ctx, sid, nil, 1); err != nil {
			t.Errorf("current empty append: %v", err)
		}
		if v, _ := s.StreamVersion(ctx, sid); v != 1 {
			t.Errorf("version = %d", v)
		}
	})

	each("only aggregate streams take appends", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		for _, sid := range []string{tarodomain.StreamAll, "Account"} {
			if _, err := s.AppendToStream(ctx, sid, events(1), 0); !hasCode(err, tarodomain.CodeEventAppendFailed) {
				t.Errorf("%s: err = %v", sid, err)
			}
		}
		if got := all(t, ctx, s, tarodomain.UUID{}); len(got) != 0 {
			t.Errorf("events were stored: %v", got)
		}
	})

	each("duplicate event IDs fail the whole append", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		a, b := newStream("Account"), newStream("Account")
		_, err := s.AppendToStream(ctx, a, events(1), 0)
		must(t, err)
		dup := events(2, 3, 2)
		for name, batch := range map[string][]tarodomain.SerializedEvent{
			"stored elsewhere": events(1),
			"inside one batch": dup,
			"after a new one":  events(5, 1),
		} {
			if _, err := s.AppendToStream(ctx, b, batch, 0); !hasCode(err, tarodomain.CodeEventAppendFailed) {
				t.Errorf("%s: err = %v", name, err)
			}
			if v, _ := s.StreamVersion(ctx, b); v != 0 {
				t.Errorf("%s: stream version = %d", name, v)
			}
		}
		if got := all(t, ctx, s, tarodomain.UUID{}); !sameInts(got, []int{1}) {
			t.Errorf("events = %v", got)
		}
	})

	each("events round-trip unchanged", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		sid := newStream("Account")
		want := []tarodomain.SerializedEvent{fullEvent(eventID(1), 1), plainEvent(eventID(2), 2)}
		_, err := s.AppendToStream(ctx, sid, want, 0)
		must(t, err)
		got, err := s.LoadStream(ctx, sid, 0, 0)
		must(t, err)
		if len(got) != len(want) {
			t.Fatalf("loaded %d events", len(got))
		}
		for i := range want {
			if why := sameEvent(want[i], got[i]); why != "" {
				t.Errorf("event %d differs in %s", i, why)
			}
		}
		viaAll, err := s.LoadAll(ctx, tarodomain.UUID{}, 0)
		must(t, err)
		if len(viaAll) != 2 || sameEvent(want[0], viaAll[0]) != "" {
			t.Errorf("LoadAll returns the event differently")
		}
	})

	each("LoadStream honours afterVersion and maxCount", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		sid := newStream("Account")
		_, err := s.AppendToStream(ctx, sid, events(1, 2, 3, 4, 5), 0)
		must(t, err)
		load := func(after int64, max int) []int {
			es, err := s.LoadStream(ctx, sid, after, max)
			must(t, err)
			return numbers(es)
		}
		for _, c := range []struct {
			after int64
			max   int
			want  []int
		}{
			{0, 0, []int{1, 2, 3, 4, 5}},
			{2, 0, []int{3, 4, 5}},
			{1, 2, []int{2, 3}},
			{5, 0, []int{}},
			{0, -1, []int{1, 2, 3, 4, 5}},
		} {
			if got := load(c.after, c.max); !sameInts(got, c.want) {
				t.Errorf("after %d max %d: got %v, want %v", c.after, c.max, got, c.want)
			}
		}
		if es, err := s.LoadStream(ctx, newStream("Account"), 0, 0); err != nil || len(es) != 0 {
			t.Errorf("missing stream: %v, %v", numbers(es), err)
		}
	})

	each("cross-stream reads follow append order", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		a1, a2, o := newStream("Account"), newStream("Account"), newStream("Order")
		// IDs are deliberately out of append order.
		_, err := s.AppendToStream(ctx, a1, events(9), 0)
		must(t, err)
		_, err = s.AppendToStream(ctx, o, events(5), 0)
		must(t, err)
		_, err = s.AppendToStream(ctx, a2, events(2, 7), 0)
		must(t, err)

		if got := all(t, ctx, s, tarodomain.UUID{}); !sameInts(got, []int{9, 5, 2, 7}) {
			t.Errorf("$all = %v", got)
		}
		if got := all(t, ctx, s, eventID(9)); !sameInts(got, []int{5, 2, 7}) {
			t.Errorf("$all after 9 = %v", got)
		}
		if got := all(t, ctx, s, eventID(2)); !sameInts(got, []int{7}) {
			t.Errorf("$all after 2 = %v", got)
		}
		limited, err := s.LoadAll(ctx, tarodomain.UUID{}, 2)
		must(t, err)
		if got := numbers(limited); !sameInts(got, []int{9, 5}) {
			t.Errorf("$all max 2 = %v", got)
		}
		cat, err := s.LoadStreamAfterEvent(ctx, "Account", tarodomain.UUID{}, 0)
		must(t, err)
		if got := numbers(cat); !sameInts(got, []int{9, 2, 7}) {
			t.Errorf("category = %v", got)
		}
		cat, err = s.LoadStreamAfterEvent(ctx, "Account", eventID(9), 0)
		must(t, err)
		if got := numbers(cat); !sameInts(got, []int{2, 7}) {
			t.Errorf("category after 9 = %v", got)
		}
		one, err := s.LoadStreamAfterEvent(ctx, a2, tarodomain.UUID{}, 0)
		must(t, err)
		if got := numbers(one); !sameInts(got, []int{2, 7}) {
			t.Errorf("aggregate stream = %v", got)
		}
		none, err := s.LoadStreamAfterEvent(ctx, newStream("Account"), tarodomain.UUID{}, 0)
		must(t, err)
		if len(none) != 0 {
			t.Errorf("missing stream = %v", numbers(none))
		}
		other, err := s.LoadStreamAfterEvent(ctx, "Nothing", tarodomain.UUID{}, 0)
		must(t, err)
		if len(other) != 0 {
			t.Errorf("missing category = %v", numbers(other))
		}
	})

	each("two events of one millisecond come back in append order", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		first, second := reversedPair(4)
		a, b := newStream("Account"), newStream("Account")
		_, err := s.AppendToStream(ctx, a, []tarodomain.SerializedEvent{plainEvent(first, 1)}, 0)
		must(t, err)
		_, err = s.AppendToStream(ctx, b, []tarodomain.SerializedEvent{plainEvent(second, 2)}, 0)
		must(t, err)
		if got := all(t, ctx, s, tarodomain.UUID{}); !sameInts(got, []int{1, 2}) {
			t.Errorf("$all = %v", got)
		}
		// Reading with the first as the cursor must still find the second,
		// although the second's ID is smaller.
		if got := all(t, ctx, s, first); !sameInts(got, []int{2}) {
			t.Errorf("after the first = %v", got)
		}
		if got := all(t, ctx, s, second); len(got) != 0 {
			t.Errorf("after the second = %v", got)
		}
	})

	each("a cursor whose event was deleted still works", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		a, b := newStream("Account"), newStream("Account")
		_, err := s.AppendToStream(ctx, a, events(1), 0)
		must(t, err)
		_, err = s.AppendToStream(ctx, b, events(2, 4), 0)
		must(t, err)
		must(t, s.DeleteStream(ctx, a))
		if got := all(t, ctx, s, eventID(1)); !sameInts(got, []int{2, 4}) {
			t.Errorf("after the deleted event = %v", got)
		}
		if got := all(t, ctx, s, eventID(3)); !sameInts(got, []int{4}) {
			t.Errorf("after an unknown event = %v", got)
		}
	})

	each("events load by correlation ID", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		a, o := newStream("Account"), newStream("Order")
		mk := func(n, corr int) tarodomain.SerializedEvent {
			e := plainEvent(eventID(n), n)
			e.CorrelationID = otherID(corr)
			return e
		}
		_, err := s.AppendToStream(ctx, a, []tarodomain.SerializedEvent{mk(1, 50), mk(2, 51)}, 0)
		must(t, err)
		_, err = s.AppendToStream(ctx, o, []tarodomain.SerializedEvent{mk(3, 50)}, 0)
		must(t, err)
		got, err := s.LoadByCorrelationID(ctx, otherID(50))
		must(t, err)
		if !sameInts(numbers(got), []int{1, 3}) {
			t.Errorf("correlation 50 = %v", numbers(got))
		}
		got, err = s.LoadByCorrelationID(ctx, otherID(99))
		must(t, err)
		if len(got) != 0 {
			t.Errorf("unknown correlation = %v", numbers(got))
		}
	})

	each("deleting a stream removes it, and is a no-op when missing", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		a, b := newStream("Account"), newStream("Account")
		_, err := s.AppendToStream(ctx, a, events(1, 2), 0)
		must(t, err)
		_, err = s.AppendToStream(ctx, b, events(3), 0)
		must(t, err)
		must(t, s.DeleteStream(ctx, a))
		must(t, s.DeleteStream(ctx, a))
		must(t, s.DeleteStream(ctx, newStream("Account")))
		if ok, _ := s.StreamExists(ctx, a); ok {
			t.Errorf("stream still exists")
		}
		if v, _ := s.StreamVersion(ctx, a); v != 0 {
			t.Errorf("version = %d", v)
		}
		if got := all(t, ctx, s, tarodomain.UUID{}); !sameInts(got, []int{3}) {
			t.Errorf("remaining = %v", got)
		}
		// The stream can be created again, from version 1.
		res, err := s.AppendToStream(ctx, a, events(1), 0)
		must(t, err)
		if res.NewVersion != 1 {
			t.Errorf("new version = %d", res.NewVersion)
		}
	})

	each("subscriptions", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		a, b := newStream("Account"), newStream("Account")
		_, err := s.AppendToStream(ctx, a, events(9), 0)
		must(t, err)
		_, err = s.AppendToStream(ctx, b, events(2), 0)
		must(t, err)

		sub, err := s.Subscribe(ctx, "p", "")
		must(t, err)
		if sub.Name != "p" || sub.StreamID != tarodomain.StreamAll || !sub.LastSeen.IsZero() ||
			sub.CreatedAt.IsZero() || !sub.FailedEventID.IsZero() || sub.FailedAttempts != 0 {
			t.Errorf("new subscription = %+v", sub)
		}
		again, err := s.Subscribe(ctx, "p", "Account")
		must(t, err)
		if again.StreamID != tarodomain.StreamAll || !again.CreatedAt.Equal(sub.CreatedAt) {
			t.Errorf("Subscribe is not idempotent: %+v", again)
		}
		_, err = s.Subscribe(ctx, "a", "Account")
		must(t, err)
		if _, ok, _ := s.GetSubscription(ctx, "nope"); ok {
			t.Errorf("found a missing subscription")
		}

		for i, want := range []int{1, 2} {
			if n, err := s.RecordFailedAttempt(ctx, "p", eventID(9)); err != nil || n != want {
				t.Errorf("attempt %d on the same event = %d, %v", i+1, n, err)
			}
		}
		if n, _ := s.RecordFailedAttempt(ctx, "p", eventID(2)); n != 1 {
			t.Errorf("a different event restarts the count: %d", n)
		}
		got, _, _ := s.GetSubscription(ctx, "p")
		if got.FailedEventID != eventID(2) || got.FailedAttempts != 1 {
			t.Errorf("failure record = %+v", got)
		}

		// Acks follow append order (9 then 2), and clear the failure record.
		must(t, s.AckSubscription(ctx, "p", eventID(9)))
		got, _, _ = s.GetSubscription(ctx, "p")
		if got.LastSeen != eventID(9) || got.FailedAttempts != 0 || !got.FailedEventID.IsZero() {
			t.Errorf("after ack = %+v", got)
		}
		must(t, s.AckSubscription(ctx, "p", eventID(2)))
		must(t, s.AckSubscription(ctx, "p", eventID(9)))
		must(t, s.AckSubscription(ctx, "p", tarodomain.UUID{}))
		got, _, _ = s.GetSubscription(ctx, "p")
		if got.LastSeen != eventID(2) {
			t.Errorf("the cursor moved back to %v", got.LastSeen)
		}

		list, err := s.ListSubscriptions(ctx)
		must(t, err)
		if len(list) != 2 || list[0].Name != "a" || list[1].Name != "p" || list[0].StreamID != "Account" {
			t.Errorf("list = %+v", list)
		}
		must(t, s.DeleteSubscription(ctx, "p"))
		must(t, s.DeleteSubscription(ctx, "p"))
		if _, ok, _ := s.GetSubscription(ctx, "p"); ok {
			t.Errorf("subscription not deleted")
		}
		if err := s.AckSubscription(ctx, "nope", eventID(1)); !hasCode(err, tarodomain.CodeEventStoreError) {
			t.Errorf("ack of a missing subscription: %v", err)
		}
		if _, err := s.RecordFailedAttempt(ctx, "nope", eventID(1)); !hasCode(err, tarodomain.CodeEventStoreError) {
			t.Errorf("failure on a missing subscription: %v", err)
		}
	})
}
