package local

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// uid returns a UUID that sorts by n.
func uid(n int) tarodomain.UUID {
	var u tarodomain.UUID
	u[0] = 1
	u[14] = byte(n >> 8)
	u[15] = byte(n)
	return u
}

func sev(n int) tarodomain.SerializedEvent {
	return tarodomain.SerializedEvent{
		EventID: uid(n), EventType: "Thing", EventVersion: "1.0.0",
		Data:       []byte(fmt.Sprintf(`{"n":%d}`, n)),
		OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func evs(ns ...int) []tarodomain.SerializedEvent {
	var out []tarodomain.SerializedEvent
	for _, n := range ns {
		out = append(out, sev(n))
	}
	return out
}

func stream(typ string) string {
	return tarodomain.StreamForAggregate(typ, tarodomain.NewAggregateID())
}

func ids(es []tarodomain.SerializedEvent) []int {
	var out []int
	for _, e := range es {
		out = append(out, int(e.EventID[14])<<8|int(e.EventID[15]))
	}
	return out
}

func eq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newStore() (*EventStore, context.Context) {
	return NewEventStore(NewDatabase(nil)), context.Background()
}

func TestAppendVersionsAndConflicts(t *testing.T) {
	s, ctx := newStore()
	sid := stream("Account")
	if v, _ := s.StreamVersion(ctx, sid); v != 0 {
		t.Fatalf("missing stream version %d", v)
	}
	res, err := s.AppendToStream(ctx, sid, evs(1, 2), 0)
	if err != nil || res.PreviousVersion != 0 || res.NewVersion != 2 {
		t.Fatalf("res %+v err %v", res, err)
	}
	_, err = s.AppendToStream(ctx, sid, evs(3), 0)
	if !errors.Is(err, tarodomain.ErrCode(tarodomain.CodeOptimisticConcurrency)) {
		t.Fatalf("err = %v", err)
	}
	e, _ := tarodomain.AsError(err)
	if e.Context["expectedVersion"] != int64(0) || e.Context["actualVersion"] != int64(2) || e.Context["streamId"] != sid {
		t.Fatalf("context %v", e.Context)
	}
	if _, err := s.AppendToStream(ctx, sid, evs(3), 2); err != nil {
		t.Fatal(err)
	}
	md, ok, _ := s.StreamMetadata(ctx, sid)
	if !ok || md.Version != 3 || md.CreatedAt.IsZero() {
		t.Fatalf("metadata %+v", md)
	}
}

func TestEmptyAppend(t *testing.T) {
	s, ctx := newStore()
	sid := stream("Account")
	if _, err := s.AppendToStream(ctx, sid, nil, 0); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.StreamExists(ctx, sid); ok {
		t.Fatal("empty append created a stream")
	}
	if _, err := s.AppendToStream(ctx, sid, nil, 4); !errors.Is(err, tarodomain.ErrCode(tarodomain.CodeOptimisticConcurrency)) {
		t.Fatalf("err = %v", err)
	}
}

func TestAppendOnlyToAggregateStreams(t *testing.T) {
	s, ctx := newStore()
	for _, sid := range []string{tarodomain.StreamAll, "Account"} {
		_, err := s.AppendToStream(ctx, sid, evs(1), 0)
		if !errors.Is(err, tarodomain.ErrCode(tarodomain.CodeEventAppendFailed)) {
			t.Fatalf("%s: err = %v", sid, err)
		}
	}
}

func TestDuplicateIDsWriteNothing(t *testing.T) {
	s, ctx := newStore()
	a, b := stream("Account"), stream("Account")
	if _, err := s.AppendToStream(ctx, a, evs(1), 0); err != nil {
		t.Fatal(err)
	}
	for _, batch := range [][]tarodomain.SerializedEvent{evs(1), evs(2, 3, 2), evs(5, 1)} {
		_, err := s.AppendToStream(ctx, b, batch, 0)
		if !errors.Is(err, tarodomain.ErrCode(tarodomain.CodeEventAppendFailed)) {
			t.Fatalf("err = %v", err)
		}
		if v, _ := s.StreamVersion(ctx, b); v != 0 {
			t.Fatalf("stream written: version %d", v)
		}
	}
	if all, _ := s.LoadAll(ctx, tarodomain.UUID{}, 0); len(all) != 1 {
		t.Fatalf("%d events stored", len(all))
	}
}

func TestLoadStreamWindow(t *testing.T) {
	s, ctx := newStore()
	sid := stream("Account")
	_, _ = s.AppendToStream(ctx, sid, evs(1, 2, 3, 4, 5), 0)
	got, _ := s.LoadStream(ctx, sid, 2, 0)
	if !eq(ids(got), []int{3, 4, 5}) {
		t.Fatalf("after 2: %v", ids(got))
	}
	got, _ = s.LoadStream(ctx, sid, 1, 2)
	if !eq(ids(got), []int{2, 3}) {
		t.Fatalf("max 2: %v", ids(got))
	}
	if got, _ = s.LoadStream(ctx, stream("Account"), 0, 0); len(got) != 0 {
		t.Fatal("missing stream not empty")
	}
}

func TestEventsRoundTripAndAreCopies(t *testing.T) {
	s, ctx := newStore()
	sid := stream("Account")
	in := sev(1)
	in.CorrelationID, in.CausationID, in.CommandCausationID = uid(90), uid(91), uid(92)
	in.Metadata, _ = tarodomain.NewMetadata(map[string]any{"k": "v"})
	_, _ = s.AppendToStream(ctx, sid, []tarodomain.SerializedEvent{in}, 0)
	in.Data[2] = 'X' // the caller's buffer must not reach the store
	got, _ := s.LoadStream(ctx, sid, 0, 0)
	if string(got[0].Data) != `{"n":1}` || got[0].CorrelationID != uid(90) || got[0].CommandCausationID != uid(92) || !got[0].Metadata.Equal(in.Metadata) {
		t.Fatalf("got %+v", got[0])
	}
	got[0].Data[2] = 'Y'
	again, _ := s.LoadStream(ctx, sid, 0, 0)
	if string(again[0].Data) != `{"n":1}` {
		t.Fatal("a returned event shares memory with the store")
	}
}

func TestAppendOrderAcrossStreamsAndCursors(t *testing.T) {
	s, ctx := newStore()
	a, b := stream("Account"), stream("Account")
	o := stream("Order")
	// IDs deliberately out of order: 9 is appended before 2.
	_, _ = s.AppendToStream(ctx, a, evs(9), 0)
	_, _ = s.AppendToStream(ctx, o, evs(5), 0)
	_, _ = s.AppendToStream(ctx, b, evs(2), 0)
	all, _ := s.LoadAll(ctx, tarodomain.UUID{}, 0)
	if !eq(ids(all), []int{9, 5, 2}) {
		t.Fatalf("append order: %v", ids(all))
	}
	// A cursor resolves by position, not by ID value.
	after, _ := s.LoadAll(ctx, uid(9), 0)
	if !eq(ids(after), []int{5, 2}) {
		t.Fatalf("after 9: %v", ids(after))
	}
	cat, _ := s.LoadStreamAfterEvent(ctx, "Account", tarodomain.UUID{}, 0)
	if !eq(ids(cat), []int{9, 2}) {
		t.Fatalf("category: %v", ids(cat))
	}
	one, _ := s.LoadStreamAfterEvent(ctx, b, tarodomain.UUID{}, 0)
	if !eq(ids(one), []int{2}) {
		t.Fatalf("aggregate: %v", ids(one))
	}
	lim, _ := s.LoadAll(ctx, tarodomain.UUID{}, 2)
	if !eq(ids(lim), []int{9, 5}) {
		t.Fatalf("limit: %v", ids(lim))
	}
}

func TestDeletedCursorFallsBackToIDs(t *testing.T) {
	s, ctx := newStore()
	a, b := stream("Account"), stream("Account")
	_, _ = s.AppendToStream(ctx, a, evs(1), 0)
	_, _ = s.AppendToStream(ctx, b, evs(2, 4), 0)
	_ = s.DeleteStream(ctx, a)
	got, _ := s.LoadAll(ctx, uid(1), 0)
	if !eq(ids(got), []int{2, 4}) {
		t.Fatalf("got %v", ids(got))
	}
	got, _ = s.LoadAll(ctx, uid(3), 0)
	if !eq(ids(got), []int{4}) {
		t.Fatalf("got %v", ids(got))
	}
}

func TestDeleteStream(t *testing.T) {
	s, ctx := newStore()
	sid := stream("Account")
	_, _ = s.AppendToStream(ctx, sid, evs(1, 2), 0)
	if err := s.DeleteStream(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteStream(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.StreamExists(ctx, sid); ok {
		t.Fatal("stream still exists")
	}
	if _, ok, _ := s.StreamMetadata(ctx, sid); ok {
		t.Fatal("metadata still exists")
	}
	// The IDs are free again.
	if _, err := s.AppendToStream(ctx, sid, evs(1), 0); err != nil {
		t.Fatal(err)
	}
}

func TestLoadByCorrelationID(t *testing.T) {
	s, ctx := newStore()
	a, b := stream("Account"), stream("Order")
	e1, e2, e3 := sev(1), sev(2), sev(3)
	e1.CorrelationID, e2.CorrelationID, e3.CorrelationID = uid(50), uid(51), uid(50)
	_, _ = s.AppendToStream(ctx, a, []tarodomain.SerializedEvent{e1, e2}, 0)
	_, _ = s.AppendToStream(ctx, b, []tarodomain.SerializedEvent{e3}, 0)
	got, _ := s.LoadByCorrelationID(ctx, uid(50))
	if !eq(ids(got), []int{1, 3}) {
		t.Fatalf("got %v", ids(got))
	}
}

func TestSubscriptions(t *testing.T) {
	s, ctx := newStore()
	a, b := stream("Account"), stream("Account")
	_, _ = s.AppendToStream(ctx, a, evs(9), 0)
	_, _ = s.AppendToStream(ctx, b, evs(2), 0)

	sub, err := s.Subscribe(ctx, "p", "")
	if err != nil || sub.StreamID != tarodomain.StreamAll || !sub.LastSeen.IsZero() || sub.CreatedAt.IsZero() {
		t.Fatalf("sub %+v err %v", sub, err)
	}
	if again, _ := s.Subscribe(ctx, "p", "Account"); again != sub {
		t.Fatalf("Subscribe not idempotent: %+v", again)
	}
	_, _ = s.Subscribe(ctx, "a", "Account")

	if n, _ := s.RecordFailedAttempt(ctx, "p", uid(9)); n != 1 {
		t.Fatalf("n = %d", n)
	}
	if n, _ := s.RecordFailedAttempt(ctx, "p", uid(9)); n != 2 {
		t.Fatalf("n = %d", n)
	}
	if n, _ := s.RecordFailedAttempt(ctx, "p", uid(2)); n != 1 {
		t.Fatalf("different event: n = %d", n)
	}

	// Ack moves forward in append order (9 then 2) and clears the failure.
	if err := s.AckSubscription(ctx, "p", uid(9)); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.GetSubscription(ctx, "p")
	if got.LastSeen != uid(9) || got.FailedAttempts != 0 || !got.FailedEventID.IsZero() {
		t.Fatalf("after ack %+v", got)
	}
	_ = s.AckSubscription(ctx, "p", uid(2))
	_ = s.AckSubscription(ctx, "p", uid(9)) // back: ignored
	_ = s.AckSubscription(ctx, "p", tarodomain.UUID{})
	got, _, _ = s.GetSubscription(ctx, "p")
	if got.LastSeen != uid(2) {
		t.Fatalf("cursor moved back to %v", got.LastSeen)
	}

	list, _ := s.ListSubscriptions(ctx)
	if len(list) != 2 || list[0].Name != "a" || list[1].Name != "p" {
		t.Fatalf("list %+v", list)
	}
	_ = s.DeleteSubscription(ctx, "p")
	_ = s.DeleteSubscription(ctx, "p")
	if _, ok, _ := s.GetSubscription(ctx, "p"); ok {
		t.Fatal("not deleted")
	}
	for _, err := range []error{
		s.AckSubscription(ctx, "nope", uid(1)),
		func() error { _, e := s.RecordFailedAttempt(ctx, "nope", uid(1)); return e }(),
	} {
		if !errors.Is(err, tarodomain.ErrCode(tarodomain.CodeEventStoreError)) {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestConcurrentAppendsOneWins(t *testing.T) {
	s, ctx := newStore()
	sid := stream("Account")
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.AppendToStream(ctx, sid, evs(i+1), 0)
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, tarodomain.ErrCode(tarodomain.CodeOptimisticConcurrency)) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d winners", wins)
	}
}

func TestStoreUsesUnitCopy(t *testing.T) {
	s, ctx := newStore()
	sid := stream("Account")
	err := tarodomain.ExecuteInUnit(ctx, NewUnitOfWork(s.db), func(u context.Context) error {
		if _, err := s.AppendToStream(u, sid, evs(1), 0); err != nil {
			return err
		}
		if v, _ := s.StreamVersion(u, sid); v != 1 {
			t.Fatalf("unit sees version %d", v)
		}
		if v, _ := s.StreamVersion(ctx, sid); v != 0 {
			t.Fatalf("outside sees version %d", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.StreamVersion(ctx, sid); v != 1 {
		t.Fatalf("after commit %d", v)
	}
}
