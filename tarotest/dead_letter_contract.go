package tarotest

import (
	"context"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// DeadLetterContract checks a provider's [tarodomain.DeadLetterStore].
func DeadLetterContract(t T, newProvider Factory) {
	t.Helper()
	each := func(name string, f func(t T, ctx context.Context, s tarodomain.DeadLetterStore)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			s, err := fresh(ctx, t, newProvider).CreateDeadLetterStore()
			if err != nil {
				t.Fatalf("CreateDeadLetterStore: %v", err)
			}
			f(t, ctx, s)
		})
	}
	input := func(handler string, n int) tarodomain.DeadLetterInput {
		return tarodomain.DeadLetterInput{
			HandlerName: handler, EventID: eventID(n), EventType: "ContractEvent",
			EventData: []byte(`{"n":1}`), CorrelationID: otherID(n),
			ErrorCode: "DOMAIN_ERROR", ErrorMessage: "it broke", Attempts: 4,
		}
	}
	write := func(t T, ctx context.Context, s tarodomain.DeadLetterStore, handler string, n int) int64 {
		t.Helper()
		id, err := s.Write(ctx, input(handler, n))
		noErr(t, err)
		return id
	}

	each("a written entry comes back with its fields", func(t T, ctx context.Context, s tarodomain.DeadLetterStore) {
		in := input("proj", 1)
		id, err := s.Write(ctx, in)
		noErr(t, err)
		e, ok, err := s.GetByID(ctx, id)
		noErr(t, err)
		if !ok {
			t.Fatalf("not found")
		}
		if e.ID != id || e.HandlerName != "proj" || e.EventID != in.EventID || e.EventType != in.EventType ||
			!sameJSON(e.EventData, in.EventData) || e.CorrelationID != in.CorrelationID ||
			e.ErrorCode != "DOMAIN_ERROR" || e.ErrorMessage != "it broke" || e.Attempts != 4 || e.CreatedAt.IsZero() {
			t.Errorf("entry = %+v", e)
		}
		if _, ok, _ := s.GetByID(ctx, id+1000); ok {
			t.Errorf("found an entry that was never written")
		}
	})

	each("IDs increase and are never reused", func(t T, ctx context.Context, s tarodomain.DeadLetterStore) {
		a := write(t, ctx, s, "proj", 1)
		b := write(t, ctx, s, "proj", 2)
		c := write(t, ctx, s, "other", 3)
		if !(a < b && b < c) {
			t.Fatalf("IDs = %d, %d, %d", a, b, c)
		}
		noErr(t, s.Delete(ctx, c))
		noErr(t, s.Delete(ctx, c))
		if d := write(t, ctx, s, "other", 4); d <= c {
			t.Errorf("ID %d reused or not increasing after deleting %d", d, c)
		}
		if _, ok, _ := s.GetByID(ctx, c); ok {
			t.Errorf("deleted entry found")
		}
	})

	each("List is in ID order per handler", func(t T, ctx context.Context, s tarodomain.DeadLetterStore) {
		a := write(t, ctx, s, "proj", 1)
		write(t, ctx, s, "other", 2)
		c := write(t, ctx, s, "proj", 3)
		list, err := s.List(ctx, "proj", 0)
		noErr(t, err)
		if len(list) != 2 || list[0].ID != a || list[1].ID != c {
			t.Errorf("list = %+v", list)
		}
		if limited, _ := s.List(ctx, "proj", 1); len(limited) != 1 || limited[0].ID != a {
			t.Errorf("limited = %+v", limited)
		}
		if list, _ = s.List(ctx, "nobody", 0); len(list) != 0 {
			t.Errorf("list of an unknown handler = %+v", list)
		}
	})

	each("Get returns the latest entry for the event and handler", func(t T, ctx context.Context, s tarodomain.DeadLetterStore) {
		write(t, ctx, s, "proj", 1)
		second := write(t, ctx, s, "proj", 1)
		write(t, ctx, s, "other", 1)
		e, ok, err := s.Get(ctx, eventID(1), "proj")
		noErr(t, err)
		if !ok || e.ID != second || e.HandlerName != "proj" {
			t.Errorf("Get = %+v, %v", e, ok)
		}
		if _, ok, _ := s.Get(ctx, eventID(2), "proj"); ok {
			t.Errorf("found an entry for another event")
		}
		if _, ok, _ := s.Get(ctx, eventID(1), "nobody"); ok {
			t.Errorf("found an entry for another handler")
		}
	})

	each("Count", func(t T, ctx context.Context, s tarodomain.DeadLetterStore) {
		write(t, ctx, s, "proj", 1)
		write(t, ctx, s, "proj", 2)
		write(t, ctx, s, "other", 3)
		for _, c := range []struct {
			handler string
			want    int
		}{{"", 3}, {"proj", 2}, {"other", 1}, {"nobody", 0}} {
			if n, err := s.Count(ctx, c.handler); err != nil || n != c.want {
				t.Errorf("Count(%q) = %d, %v; want %d", c.handler, n, err, c.want)
			}
		}
	})

	each("DeleteOld is exclusive of its cut-off", func(t T, ctx context.Context, s tarodomain.DeadLetterStore) {
		id := write(t, ctx, s, "proj", 1)
		e, _, _ := s.GetByID(ctx, id)
		for _, c := range []struct {
			cutoff time.Time
			want   int
		}{{farPast, 0}, {e.CreatedAt, 0}, {e.CreatedAt.Add(time.Millisecond), 1}} {
			if n, err := s.DeleteOld(ctx, c.cutoff); err != nil || n != c.want {
				t.Errorf("DeleteOld(%v) = %d, %v; want %d", c.cutoff, n, err, c.want)
			}
		}
		if n, _ := s.Count(ctx, ""); n != 0 {
			t.Errorf("count = %d", n)
		}
		write(t, ctx, s, "proj", 2)
		if n, _ := s.DeleteOld(ctx, farFuture); n != 1 {
			t.Errorf("DeleteOld(far future) = %d", n)
		}
	})
}
