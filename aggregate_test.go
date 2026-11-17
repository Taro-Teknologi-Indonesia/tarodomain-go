package tarodomain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type opened struct{}

func (opened) EventName() string { return "AccountOpened" }

type unknownEvent struct{}

func (unknownEvent) EventName() string { return "Unknown" }

type acctTag struct{}

type account struct {
	AggregateRoot
	id      ID[acctTag]
	balance int64
}

func (a *account) AggregateType() string    { return "account" }
func (a *account) AggregateID() AggregateID { return a.id.Untyped() }
func (a *account) Apply(e Event) error {
	switch e := e.(type) {
	case opened:
	case moneyDeposited:
		a.balance += e.Amount
	case moneyWithdrawn:
		if e.Amount > a.balance {
			return errors.New("overdrawn")
		}
		a.balance -= e.Amount
	default:
		return MissingEventHandler(e)
	}
	return nil
}
func (a *account) SnapshotState() (any, error) { return map[string]int64{"balance": a.balance}, nil }
func (a *account) RestoreSnapshotState(raw json.RawMessage) error {
	var s struct{ Balance int64 }
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	a.balance = s.Balance
	return nil
}

func newAccount() *account { return &account{id: NewID[acctTag]()} }

func TestRaiseVersions(t *testing.T) {
	a := newAccount()
	ctx := context.Background()
	if err := Raise(ctx, a, moneyDeposited{10}); err != nil {
		t.Fatal(err)
	}
	if err := Raise(ctx, a, moneyDeposited{5}); err != nil {
		t.Fatal(err)
	}
	r := a.Root()
	if a.balance != 15 || r.Version() != 2 || r.CommittedVersion() != 0 || len(r.UncommittedChanges()) != 2 {
		t.Errorf("%d %d %d", a.balance, r.Version(), r.CommittedVersion())
	}
	r.MarkChangesAsCommitted()
	if r.HasUncommittedChanges() || r.CommittedVersion() != 2 || r.Version() != 2 {
		t.Error("commit marking wrong")
	}
}

func TestRaiseFailureQueuesNothing(t *testing.T) {
	a := newAccount()
	if err := Raise(context.Background(), a, moneyWithdrawn{1}); err == nil {
		t.Fatal("expected failure")
	}
	if a.Version() != 0 || a.HasUncommittedChanges() {
		t.Error("failed Apply left traces")
	}
	if err := Raise(context.Background(), a, &moneyDeposited{1}); !isCode(err, CodeInvalidConfiguration) {
		t.Errorf("pointer event: %v", err)
	}
	if a.balance != 0 {
		t.Error("pointer event was applied")
	}
}

func TestRaiseUsesContextualTracing(t *testing.T) {
	ec := NewExecutionContext(UUID{})
	ec.CausationID = NewUUIDv7()
	ctx := WithExecutionContext(context.Background(), ec)
	a := newAccount()
	_ = Raise(ctx, a, opened{})
	e := a.UncommittedChanges()[0].Envelope()
	if e.CorrelationID != ec.CorrelationID || e.CausationID != ec.CausationID {
		t.Errorf("%+v", e)
	}
}

func TestRaiseOwnContextWins(t *testing.T) {
	own := NewExecutionContext(UUID{})
	other := NewExecutionContext(UUID{})
	a := newAccount()
	a.SetExecutionContext(own)
	_ = Raise(WithExecutionContext(context.Background(), other), a, opened{})
	if got := a.UncommittedChanges()[0].Envelope().CorrelationID; got != own.CorrelationID {
		t.Errorf("got %v, want the aggregate's own", got)
	}
}

func TestRaiseOutsideContextStartsRoot(t *testing.T) {
	a := newAccount()
	_ = Raise(context.Background(), a, opened{})
	_ = Raise(context.Background(), a, opened{})
	ch := a.UncommittedChanges()
	e := ch[0].Envelope()
	if !e.CorrelationID.IsV7() || !e.CausationID.IsZero() {
		t.Errorf("%+v", e)
	}
}

func TestRaiseTimeFromClock(t *testing.T) {
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	ctx := WithClock(context.Background(), NewManualClock(at))
	a := newAccount()
	_ = Raise(ctx, a, opened{})
	if got := a.UncommittedChanges()[0].Envelope().OccurredAt; !got.Equal(at) {
		t.Errorf("got %v", got)
	}
}

func TestLoadFromHistory(t *testing.T) {
	src := newAccount()
	ctx := context.Background()
	_ = Raise(ctx, src, moneyDeposited{10})
	_ = Raise(ctx, src, moneyDeposited{5})
	history := src.UncommittedChanges()

	a := newAccount()
	if err := LoadFromHistory(a, history); err != nil {
		t.Fatal(err)
	}
	if a.balance != 15 || a.Version() != 2 || a.HasUncommittedChanges() || a.CommittedVersion() != 2 {
		t.Errorf("%d %d", a.balance, a.Version())
	}
	_ = Raise(ctx, a, moneyDeposited{1})
	if err := LoadFromHistory(a, history); !isCode(err, CodeInvalidAggregateState) {
		t.Errorf("got %v", err)
	}
}

func TestUnhandledEvent(t *testing.T) {
	a := newAccount()
	if err := Raise(context.Background(), a, unknownEvent{}); !isCode(err, CodeMissingEventHandler) {
		t.Errorf("got %v", err)
	}
}

type recorder struct{ failed int }

func (r *recorder) Helper()               {}
func (r *recorder) Errorf(string, ...any) { r.failed++ }

func TestAssertAppliesAll(t *testing.T) {
	r := &recorder{}
	AssertAppliesAll(r, newAccount(), opened{}, moneyDeposited{1})
	if r.failed != 0 {
		t.Error("handled events failed")
	}
	AssertAppliesAll(r, newAccount(), unknownEvent{})
	if r.failed != 1 {
		t.Errorf("failed %d", r.failed)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	a := newAccount()
	ctx := context.Background()
	_ = Raise(ctx, a, moneyDeposited{42})
	a.MarkChangesAsCommitted()
	snap, err := ToSnapshot(a)
	if err != nil {
		t.Fatal(err)
	}
	b := newAccount()
	if err := RestoreSnapshot(b, snap, 1); err != nil {
		t.Fatal(err)
	}
	if b.balance != 42 || b.Version() != 1 || b.CommittedVersion() != 1 {
		t.Errorf("%d %d", b.balance, b.Version())
	}
	if err := LoadFromHistory(b, []DomainEvent{a.mustEvent(t, moneyDeposited{8})}); err != nil || b.Version() != 2 || b.balance != 50 {
		t.Errorf("tail replay: %v %d %d", err, b.Version(), b.balance)
	}
	if err := RestoreSnapshot(newAccount(), json.RawMessage(`[1]`), 1); !isCode(err, CodeSerializationError) {
		t.Errorf("misfit snapshot: %v", err)
	}
	if err := RestoreSnapshot(b, snap, 1); err != nil {
		t.Errorf("no uncommitted changes, should restore: %v", err)
	}
}

func (a *account) mustEvent(t *testing.T, e Event) DomainEvent {
	t.Helper()
	d, err := NewDomainEvent(EventEnvelope{}, e)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

type plainAggregate struct{ AggregateRoot }

func (*plainAggregate) AggregateType() string    { return "plain" }
func (*plainAggregate) AggregateID() AggregateID { return AggregateID{} }
func (*plainAggregate) Apply(Event) error        { return nil }

func TestSnapshotNeedsSnapshotter(t *testing.T) {
	if _, err := ToSnapshot(&plainAggregate{}); !isCode(err, CodeSerializationError) {
		t.Errorf("got %v", err)
	}
}

func TestRestoreVersion(t *testing.T) {
	a := newAccount()
	if err := RestoreVersion(a, 7); err != nil || a.Version() != 7 || a.CommittedVersion() != 7 {
		t.Errorf("%v %d", err, a.Version())
	}
	_ = Raise(context.Background(), a, opened{})
	if a.CommittedVersion() != 7 || a.Version() != 8 {
		t.Errorf("%d %d", a.CommittedVersion(), a.Version())
	}
	if err := RestoreVersion(a, 1); !isCode(err, CodeInvalidAggregateState) {
		t.Errorf("got %v", err)
	}
}
