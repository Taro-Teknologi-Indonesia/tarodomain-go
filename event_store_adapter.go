package tarodomain

import (
	"context"
	"time"
)

// AppendResult is what a successful append reports.
type AppendResult struct {
	// StreamID is the stream appended to.
	StreamID string
	// PreviousVersion is the stream's version before the append, which equals
	// the expected version the caller passed.
	PreviousVersion int64
	// NewVersion is the version after the append: PreviousVersion plus the
	// number of events.
	NewVersion int64
}

// StreamMetadata describes one aggregate stream.
type StreamMetadata struct {
	// StreamID is the stream's ID.
	StreamID string
	// Version is the version of the stream's last event.
	Version int64
	// CreatedAt is when the first event was appended.
	CreatedAt time.Time
	// UpdatedAt is when the last event was appended.
	UpdatedAt time.Time
}

// Subscription is a named cursor over a stream (§18.1).
type Subscription struct {
	// Name identifies the subscription; it is unique in one store.
	Name string
	// StreamID is the stream followed. It is StreamAll, an aggregate stream
	// or a category.
	StreamID string
	// LastSeen is the last event acknowledged. It is the zero UUID until the
	// first acknowledgement.
	LastSeen UUID
	// CreatedAt is when the subscription was created.
	CreatedAt time.Time
	// FailedEventID is the event the subscription last failed on, or the zero
	// UUID.
	FailedEventID UUID
	// FailedAttempts counts consecutive failures on FailedEventID.
	FailedAttempts int
}

// EventStoreAdapter is the storage contract for events (§3.2). Every provider
// implements it; the contract suites in package tarotest check it.
//
// Semantics every adapter must give:
//
//   - Versions count from 1. A stream that doesn't exist has version 0.
//   - Reads across streams (LoadAll, category reads, LoadByCorrelationID,
//     LoadStreamAfterEvent) come back in global append order, not in event ID
//     order: IDs don't order events written in the same millisecond or by
//     different servers. A cursor event ID resolves to its append position.
//   - AppendToStream appends only to aggregate streams (StreamKindAggregate).
//     Any other stream ID is EVENT_APPEND_FAILED.
//   - An append is atomic: all its events are stored, or none are. An event ID
//     already stored, or repeated within one append, is EVENT_APPEND_FAILED
//     and nothing is written.
//   - A stale expected version fails with OPTIMISTIC_CONCURRENCY, carrying the
//     stream, expected and actual versions in its context. Of two racing
//     appends with the same expected version exactly one wins.
//   - An empty append still checks the expected version, and writes nothing.
//   - Deleting something that doesn't exist is a no-op, not an error.
//   - maxCount of 0 or less means no limit. A zero UUID cursor means "from the
//     start".
//
// Methods that take a ctx join the unit of work in it (see ActiveUnit).
type EventStoreAdapter interface {
	// AppendToStream appends events to an aggregate stream if its version is
	// expectedVersion (0 for a new stream).
	AppendToStream(ctx context.Context, streamID string, events []SerializedEvent, expectedVersion int64) (AppendResult, error)

	// LoadStream returns the events of an aggregate stream with a version
	// greater than afterVersion, in version order.
	LoadStream(ctx context.Context, streamID string, afterVersion int64, maxCount int) ([]SerializedEvent, error)

	// LoadStreamAfterEvent returns the events of a stream (aggregate or
	// category) after the event afterEventID, in append order. A missing
	// stream is empty.
	LoadStreamAfterEvent(ctx context.Context, streamID string, afterEventID UUID, maxCount int) ([]SerializedEvent, error)

	// LoadAll returns every event after afterEventID, in append order.
	LoadAll(ctx context.Context, afterEventID UUID, maxCount int) ([]SerializedEvent, error)

	// LoadByCorrelationID returns every event with the correlation ID, in
	// append order.
	LoadByCorrelationID(ctx context.Context, correlationID UUID) ([]SerializedEvent, error)

	// StreamExists reports whether an aggregate stream has any event.
	StreamExists(ctx context.Context, streamID string) (bool, error)

	// StreamVersion returns the stream's version, 0 if it doesn't exist.
	StreamVersion(ctx context.Context, streamID string) (int64, error)

	// StreamMetadata returns the stream's metadata, and false if it doesn't
	// exist.
	StreamMetadata(ctx context.Context, streamID string) (StreamMetadata, bool, error)

	// DeleteStream removes an aggregate stream and its events.
	DeleteStream(ctx context.Context, streamID string) error

	// Subscribe creates a subscription at the start of streamID ("" means
	// StreamAll) and returns it. It is idempotent: an existing subscription is
	// returned unchanged.
	Subscribe(ctx context.Context, name, streamID string) (Subscription, error)

	// GetSubscription returns the subscription, and false if there is none.
	GetSubscription(ctx context.Context, name string) (Subscription, bool, error)

	// AckSubscription moves the cursor to lastSeen and clears the failure
	// record. It never moves the cursor back in append order. A missing
	// subscription is EVENT_STORE_ERROR.
	AckSubscription(ctx context.Context, name string, lastSeen UUID) error

	// RecordFailedAttempt counts one more failure on eventID and returns the
	// count, which restarts at 1 for a different event.
	RecordFailedAttempt(ctx context.Context, name string, eventID UUID) (int, error)

	// DeleteSubscription removes the subscription.
	DeleteSubscription(ctx context.Context, name string) error

	// ListSubscriptions returns every subscription ordered by name.
	ListSubscriptions(ctx context.Context) ([]Subscription, error)
}
