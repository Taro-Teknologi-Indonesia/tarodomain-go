package tarodomain

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

// Aggregate is a consistency boundary whose state is built from events
// (§5.1). An aggregate is a pointer to a struct that embeds AggregateRoot:
//
//	type Account struct {
//		tarodomain.AggregateRoot
//		id      AccountID
//		balance int64
//	}
//
// Embedding AggregateRoot supplies Root. Apply must change state only; it
// must not fail on business rules, because it also replays stored history.
type Aggregate interface {
	// AggregateType is the stable name used in stream IDs.
	AggregateType() string
	// AggregateID returns the aggregate's identity.
	AggregateID() AggregateID
	// Root returns the embedded AggregateRoot.
	Root() *AggregateRoot
	// Apply changes state for one event. A type switch whose default case
	// returns MissingEventHandler(event) is the usual shape.
	Apply(event Event) error
}

// AggregateRoot holds an aggregate's version and its uncommitted events. It
// is not safe for concurrent use; load one aggregate per command.
type AggregateRoot struct {
	version     int64
	uncommitted []DomainEvent
	ec          *ExecutionContext
}

// Root returns r, so embedding an AggregateRoot satisfies Aggregate.
func (r *AggregateRoot) Root() *AggregateRoot { return r }

// Version is the number of events applied, committed or not.
func (r *AggregateRoot) Version() int64 { return r.version }

// CommittedVersion is the version in the store: Version minus the uncommitted
// events. A save appends at this version.
func (r *AggregateRoot) CommittedVersion() int64 { return r.version - int64(len(r.uncommitted)) }

// HasUncommittedChanges reports whether events are waiting to be saved.
func (r *AggregateRoot) HasUncommittedChanges() bool { return len(r.uncommitted) > 0 }

// UncommittedChanges returns a copy of the events waiting to be saved.
func (r *AggregateRoot) UncommittedChanges() []DomainEvent { return slices.Clone(r.uncommitted) }

// MarkChangesAsCommitted forgets the uncommitted events, after they are saved.
func (r *AggregateRoot) MarkChangesAsCommitted() { r.uncommitted = nil }

// SetExecutionContext gives the aggregate its own execution context, which
// Raise prefers over the one in the ctx. Repositories set it on load.
func (r *AggregateRoot) SetExecutionContext(ec ExecutionContext) { r.ec = &ec }

// Raise applies a new event to the aggregate, raises its version and queues
// the event for saving. The envelope takes its tracing data from the
// aggregate's own execution context, else the one in ctx, else a new root, and
// its time from Now(ctx). If Apply fails nothing is queued.
func Raise(ctx context.Context, a Aggregate, event Event) error {
	root := a.Root()
	if _, err := NewDomainEvent(EventEnvelope{}, event); err != nil {
		return err
	}
	if err := a.Apply(event); err != nil {
		return err
	}
	envCtx := ctx
	if root.ec != nil {
		envCtx = WithExecutionContext(ctx, *root.ec)
	}
	de, err := NewDomainEvent(NewEventEnvelope(envCtx), event)
	if err != nil {
		return err
	}
	root.version++
	root.uncommitted = append(root.uncommitted, de)
	return nil
}

// LoadFromHistory replays stored events onto the aggregate, continuing from
// its current version (which RestoreSnapshot sets). Replayed events aren't
// queued. An aggregate with uncommitted changes is INVALID_AGGREGATE_STATE.
func LoadFromHistory(a Aggregate, events []DomainEvent) error {
	root := a.Root()
	if root.HasUncommittedChanges() {
		return NewError(CodeInvalidAggregateState, "cannot load history into an aggregate with uncommitted changes").
			WithContext("aggregateType", a.AggregateType())
	}
	for _, e := range events {
		if err := a.Apply(e.Payload()); err != nil {
			return err
		}
		root.version++
	}
	return nil
}

// RestoreVersion sets the version of a state-stored aggregate that was loaded
// without events (§6.3). An aggregate with uncommitted changes is
// INVALID_AGGREGATE_STATE.
func RestoreVersion(a Aggregate, version int64) error {
	root := a.Root()
	if root.HasUncommittedChanges() {
		return NewError(CodeInvalidAggregateState, "cannot restore the version of an aggregate with uncommitted changes").
			WithContext("aggregateType", a.AggregateType())
	}
	root.version = version
	return nil
}

// Snapshotter is implemented by aggregates that support snapshots (§5.4).
type Snapshotter interface {
	// SnapshotState returns a JSON-encodable copy of the aggregate's state.
	SnapshotState() (any, error)
	// RestoreSnapshotState sets the state from what SnapshotState produced.
	RestoreSnapshotState(state json.RawMessage) error
}

// SnapshotData is the JSON form of an aggregate's state.
type SnapshotData = json.RawMessage

// ToSnapshot captures the aggregate's state. An aggregate that isn't a
// Snapshotter, or whose state has no JSON form, is SERIALIZATION_ERROR.
func ToSnapshot(a Aggregate) (SnapshotData, error) {
	s, ok := a.(Snapshotter)
	if !ok {
		return nil, NewError(CodeSerializationError,
			fmt.Sprintf("aggregate %s does not implement Snapshotter", a.AggregateType()))
	}
	state, err := s.SnapshotState()
	if err != nil {
		return nil, NewError(CodeSerializationError, "cannot capture snapshot of "+a.AggregateType()).WithCause(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, NewError(CodeSerializationError, "cannot encode snapshot of "+a.AggregateType()).WithCause(err)
	}
	return data, nil
}

// RestoreSnapshot sets the aggregate's state from data and its version to
// version. A snapshot that doesn't fit is SERIALIZATION_ERROR; an aggregate
// with uncommitted changes is INVALID_AGGREGATE_STATE.
func RestoreSnapshot(a Aggregate, data SnapshotData, version int64) error {
	s, ok := a.(Snapshotter)
	if !ok {
		return NewError(CodeSerializationError,
			fmt.Sprintf("aggregate %s does not implement Snapshotter", a.AggregateType()))
	}
	if a.Root().HasUncommittedChanges() {
		return NewError(CodeInvalidAggregateState, "cannot restore a snapshot into an aggregate with uncommitted changes").
			WithContext("aggregateType", a.AggregateType())
	}
	if err := s.RestoreSnapshotState(data); err != nil {
		if _, isFramework := AsError(err); isFramework {
			return err
		}
		return NewError(CodeSerializationError, "snapshot does not fit "+a.AggregateType()).WithCause(err)
	}
	a.Root().version = version
	return nil
}

// MissingEventHandler is the error for the default case of Apply:
// MISSING_EVENT_HANDLER.
func MissingEventHandler(event Event) error {
	return NewError(CodeMissingEventHandler, fmt.Sprintf("no handler for event %s", event.EventName())).
		WithContext("eventType", event.EventName())
}

// TB is the part of *testing.T that AssertAppliesAll uses.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertAppliesAll is a test helper: it fails t for each sample event that
// Apply rejects, so an event added to the domain but not to Apply is caught.
// Pass one sample of every event the aggregate handles. It changes the
// aggregate, so give it a throwaway.
func AssertAppliesAll(t TB, a Aggregate, samples ...Event) {
	t.Helper()
	for _, e := range samples {
		if err := a.Apply(e); err != nil {
			t.Errorf("%s.Apply(%s): %v", a.AggregateType(), e.EventName(), err)
		}
	}
}
