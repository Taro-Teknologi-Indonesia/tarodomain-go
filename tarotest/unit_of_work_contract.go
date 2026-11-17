package tarotest

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// stores is every adapter of one provider.
type stores struct {
	provider  tarodomain.Provider
	events    tarodomain.EventStoreAdapter
	snapshots tarodomain.SnapshotStoreAdapter
	sagas     tarodomain.SagaStoreAdapter
	processed tarodomain.ProcessedEventStore
	commands  tarodomain.CommandIdempotencyStore
	letters   tarodomain.DeadLetterStore
}

// newStores returns the adapters of a fresh provider.
func newStores(ctx context.Context, t T, newProvider Factory) stores {
	t.Helper()
	p := fresh(ctx, t, newProvider)
	s := stores{provider: p}
	var err1, err2, err3, err4, err5, err6 error
	s.events, err1 = p.CreateEventStoreAdapter()
	s.snapshots, err2 = p.CreateSnapshotStoreAdapter()
	s.sagas, err3 = p.CreateSagaStoreAdapter()
	s.processed, err4 = p.CreateProcessedEventStore()
	s.commands, err5 = p.CreateCommandIdempotencyStore()
	s.letters, err6 = p.CreateDeadLetterStore()
	for _, err := range []error{err1, err2, err3, err4, err5, err6} {
		if err != nil {
			t.Fatalf("creating the adapters: %v", err)
		}
	}
	return s
}

// writeAll writes one record to every store. The event goes to stream sid.
func (s stores) writeAll(ctx context.Context, sid string, aggregate tarodomain.AggregateID) error {
	if _, err := s.events.AppendToStream(ctx, sid, events(1), 0); err != nil {
		return err
	}
	if err := s.sagas.Save(ctx, tarodomain.SagaSaveInput{SagaID: otherID(1), SagaType: "Transfer",
		CorrelationID: otherID(1), State: json.RawMessage(`{}`), Status: tarodomain.SagaRunning}); err != nil {
		return err
	}
	if err := s.snapshots.Save(ctx, tarodomain.SnapshotInput{SourceID: aggregate, SourceType: "Account",
		SourceVersion: 1, Data: json.RawMessage(`{}`)}); err != nil {
		return err
	}
	if err := s.processed.MarkProcessed(ctx, "proj", "proj", eventID(1)); err != nil {
		return err
	}
	if err := s.commands.MarkProcessed(ctx, otherID(1), "Cmd", "h"); err != nil {
		return err
	}
	_, err := s.letters.Write(ctx, tarodomain.DeadLetterInput{HandlerName: "proj", EventID: eventID(1),
		EventType: "ContractEvent", EventData: json.RawMessage(`{}`)})
	return err
}

// countAll returns how many records of each kind the stores hold, as seen by
// ctx, in the order of writeAll.
func (s stores) countAll(ctx context.Context, t T, sid string, aggregate tarodomain.AggregateID) []int {
	t.Helper()
	n := func(v int, err error) int {
		t.Helper()
		noErr(t, err)
		return v
	}
	version, err := s.events.StreamVersion(ctx, sid)
	noErr(t, err)
	return []int{
		int(version),
		n(s.sagas.Count(ctx, "Transfer", "")),
		n(s.snapshots.Count(ctx, aggregate, "Account")),
		n(s.processed.Count(ctx, "proj", "")),
		n(s.commands.Count(ctx, "")),
		n(s.letters.Count(ctx, "")),
	}
}

// countingUnit counts the calls that change a unit's state.
type countingUnit struct {
	tarodomain.UnitOfWork
	mu    sync.Mutex
	calls int
}

func (c *countingUnit) Begin(ctx context.Context) error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.UnitOfWork.Begin(ctx)
}

func (c *countingUnit) Commit(ctx context.Context) error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.UnitOfWork.Commit(ctx)
}

func (c *countingUnit) Rollback(ctx context.Context) error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.UnitOfWork.Rollback(ctx)
}

// UnitOfWorkContract checks a provider's [tarodomain.UnitOfWork]. A provider
// without units (CreateUnitOfWork returns nil) skips the suite.
func UnitOfWorkContract(t T, newProvider Factory) {
	t.Helper()
	probe := fresh(context.Background(), t, newProvider)
	if probe.CreateUnitOfWork() == nil {
		t.Skipf("provider %q has no unit of work", probe.ProviderType())
		return
	}
	each := func(name string, f func(t T, ctx context.Context, s stores)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			f(t, ctx, newStores(ctx, t, newProvider))
		})
	}
	zero := []int{0, 0, 0, 0, 0, 0}
	one := []int{1, 1, 1, 1, 1, 1}
	equal := func(t T, what string, got, want []int) {
		t.Helper()
		if !sameInts(got, want) {
			t.Errorf("%s: counts = %v, want %v", what, got, want)
		}
	}

	each("a new unit is inactive", func(t T, ctx context.Context, s stores) {
		a, b := s.provider.CreateUnitOfWork(), s.provider.CreateUnitOfWork()
		if a == nil || b == nil || a == b {
			t.Fatalf("CreateUnitOfWork must return a new unit each call")
		}
		if a.IsActive() {
			t.Errorf("a new unit is active")
		}
		noErr(t, a.Begin(ctx))
		if !a.IsActive() || b.IsActive() {
			t.Errorf("active state: a=%v b=%v", a.IsActive(), b.IsActive())
		}
		noErr(t, a.Rollback(ctx))
		if a.IsActive() {
			t.Errorf("still active after rollback")
		}
	})

	each("commit keeps the writes of every store", func(t T, ctx context.Context, s stores) {
		sid, agg := newStream("Account"), tarodomain.NewAggregateID()
		err := tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(u context.Context) error {
			return s.writeAll(u, sid, agg)
		})
		noErr(t, err)
		equal(t, "after commit", s.countAll(ctx, t, sid, agg), one)
	})

	each("rollback discards the writes of every store", func(t T, ctx context.Context, s stores) {
		sid, agg := newStream("Account"), tarodomain.NewAggregateID()
		boom := errors.New("boom")
		err := tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(u context.Context) error {
			if err := s.writeAll(u, sid, agg); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Errorf("the work's error did not come back: %v", err)
		}
		equal(t, "after rollback", s.countAll(ctx, t, sid, agg), zero)
	})

	each("a framework error from the work comes back with its code", func(t T, ctx context.Context, s stores) {
		domain := tarodomain.NewDomainError("INSUFFICIENT_FUNDS", "no money")
		err := tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(context.Context) error { return domain })
		if code, ok := tarodomain.CodeOf(err); !ok || code != "INSUFFICIENT_FUNDS" {
			t.Errorf("err = %v", err)
		}
	})

	each("reads inside the unit see its writes and readers outside don't", func(t T, ctx context.Context, s stores) {
		sid := newStream("Account")
		err := tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(u context.Context) error {
			if _, err := s.events.AppendToStream(u, sid, events(1, 2), 0); err != nil {
				return err
			}
			if err := s.sagas.Save(u, tarodomain.SagaSaveInput{SagaID: otherID(1), SagaType: "Transfer",
				CorrelationID: otherID(1), State: json.RawMessage(`{}`), Status: tarodomain.SagaRunning}); err != nil {
				return err
			}
			if err := s.commands.MarkProcessed(u, otherID(1), "Cmd", "h"); err != nil {
				return err
			}
			if err := s.processed.MarkProcessed(u, "proj", "proj", eventID(1)); err != nil {
				return err
			}

			if v, _ := s.events.StreamVersion(u, sid); v != 2 {
				t.Errorf("inside: version = %d", v)
			}
			if es, _ := s.events.LoadStream(u, sid, 0, 0); !sameInts(numbers(es), []int{1, 2}) {
				t.Errorf("inside: events = %v", numbers(es))
			}
			if _, ok, _ := s.sagas.Load(u, "Transfer", otherID(1)); !ok {
				t.Errorf("inside: the saga is not visible")
			}
			if ok, _ := s.commands.Exists(u, otherID(1)); !ok {
				t.Errorf("inside: the command mark is not visible")
			}
			if ok, _ := s.processed.Exists(u, "proj", "proj", eventID(1)); !ok {
				t.Errorf("inside: the event mark is not visible")
			}

			if v, _ := s.events.StreamVersion(ctx, sid); v != 0 {
				t.Errorf("outside: version = %d", v)
			}
			if _, ok, _ := s.sagas.Load(ctx, "Transfer", otherID(1)); ok {
				t.Errorf("outside: the saga is visible")
			}
			if ok, _ := s.commands.Exists(ctx, otherID(1)); ok {
				t.Errorf("outside: the command mark is visible")
			}
			return nil
		})
		noErr(t, err)
		if v, _ := s.events.StreamVersion(ctx, sid); v != 2 {
			t.Errorf("after commit: version = %d", v)
		}
	})

	each("a nested call joins and never begins", func(t T, ctx context.Context, s stores) {
		sid, agg := newStream("Account"), tarodomain.NewAggregateID()
		inner := &countingUnit{UnitOfWork: s.provider.CreateUnitOfWork()}
		joined := errors.New("inner failed")
		err := tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(u context.Context) error {
			if err := s.writeAll(u, sid, agg); err != nil {
				return err
			}
			return tarodomain.ExecuteInUnit(u, inner, func(context.Context) error { return joined })
		})
		if !errors.Is(err, joined) {
			t.Errorf("err = %v", err)
		}
		if inner.calls != 0 {
			t.Errorf("the nested unit was used %d times", inner.calls)
		}
		equal(t, "the inner failure rolled back the outer unit", s.countAll(ctx, t, sid, agg), zero)

		ok := &countingUnit{UnitOfWork: s.provider.CreateUnitOfWork()}
		err = tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(u context.Context) error {
			if err := s.writeAll(u, sid, agg); err != nil {
				return err
			}
			return tarodomain.ExecuteInUnit(u, ok, func(context.Context) error { return nil })
		})
		noErr(t, err)
		if ok.calls != 0 {
			t.Errorf("the nested unit was used %d times", ok.calls)
		}
		equal(t, "after the outer commit", s.countAll(ctx, t, sid, agg), one)
	})

	each("a stale append inside a unit rolls back its other writes", func(t T, ctx context.Context, s stores) {
		sid := newStream("Account")
		_, err := s.events.AppendToStream(ctx, sid, events(1), 0)
		noErr(t, err)
		err = tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(u context.Context) error {
			if err := s.sagas.Save(u, tarodomain.SagaSaveInput{SagaID: otherID(1), SagaType: "Transfer",
				CorrelationID: otherID(1), State: json.RawMessage(`{}`), Status: tarodomain.SagaRunning}); err != nil {
				return err
			}
			if err := s.commands.MarkProcessed(u, otherID(1), "Cmd", "h"); err != nil {
				return err
			}
			_, err := s.events.AppendToStream(u, sid, events(2), 0) // stale: the stream is at 1
			return err
		})
		if !hasCode(err, tarodomain.CodeOptimisticConcurrency) {
			t.Fatalf("err = %v", err)
		}
		if v, _ := s.events.StreamVersion(ctx, sid); v != 1 {
			t.Errorf("version = %d", v)
		}
		if _, ok, _ := s.sagas.Load(ctx, "Transfer", otherID(1)); ok {
			t.Errorf("the saga survived the rollback")
		}
		if ok, _ := s.commands.Exists(ctx, otherID(1)); ok {
			t.Errorf("the command mark survived the rollback")
		}
	})

	each("of two racing units on one stream exactly one wins", func(t T, ctx context.Context, s stores) {
		sid := newStream("Account")
		const racers = 4
		errs := make([]error, racers)
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = tarodomain.ExecuteInUnit(ctx, s.provider.CreateUnitOfWork(), func(u context.Context) error {
					if err := s.commands.MarkProcessed(u, otherID(i), "Cmd", "h"); err != nil {
						return err
					}
					_, err := s.events.AppendToStream(u, sid, events(10+i), 0)
					return err
				})
			}()
		}
		wg.Wait()
		wins := 0
		for i, err := range errs {
			switch {
			case err == nil:
				wins++
				if ok, _ := s.commands.Exists(ctx, otherID(i)); !ok {
					t.Errorf("the winner's command mark is missing")
				}
			case !hasCode(err, tarodomain.CodeOptimisticConcurrency):
				t.Errorf("a loser failed with %v", err)
			default:
				if ok, _ := s.commands.Exists(ctx, otherID(i)); ok {
					t.Errorf("a loser's command mark survived")
				}
			}
		}
		if wins != 1 {
			t.Errorf("%d units won", wins)
		}
		if v, _ := s.events.StreamVersion(ctx, sid); v != 1 {
			t.Errorf("version = %d", v)
		}
		if n, _ := s.commands.Count(ctx, ""); n != 1 {
			t.Errorf("%d command marks", n)
		}
	})
}
