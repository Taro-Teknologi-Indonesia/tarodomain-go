package tarodomain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SagaStatus is where a saga is in its life (§13.2).
type SagaStatus string

const (
	// SagaRunning is a saga that is moving forward.
	SagaRunning SagaStatus = "RUNNING"
	// SagaCompensating is a saga that is undoing its steps.
	SagaCompensating SagaStatus = "COMPENSATING"
	// SagaCompleted is a saga that finished. Terminal.
	SagaCompleted SagaStatus = "COMPLETED"
	// SagaFailed is a saga that ended in failure. Terminal.
	SagaFailed SagaStatus = "FAILED"
)

// String returns the stored text, such as "RUNNING".
func (s SagaStatus) String() string { return string(s) }

// IsActive reports whether the saga is RUNNING or COMPENSATING.
func (s SagaStatus) IsActive() bool { return s == SagaRunning || s == SagaCompensating }

// IsTerminal reports whether the saga is COMPLETED or FAILED.
func (s SagaStatus) IsTerminal() bool { return s == SagaCompleted || s == SagaFailed }

// ParseSagaStatus reads the stored text. Anything else is SERIALIZATION_ERROR.
func ParseSagaStatus(text string) (SagaStatus, error) {
	s := SagaStatus(text)
	switch s {
	case SagaRunning, SagaCompensating, SagaCompleted, SagaFailed:
		return s, nil
	}
	return "", NewError(CodeSerializationError, fmt.Sprintf("unknown saga status %q", text))
}

// SagaSaveInput is a saga to store (§9.2). The key is (SagaType, SagaID), and
// SagaID is the correlation ID of the event that started the saga.
type SagaSaveInput struct {
	SagaID        UUID
	SagaType      string
	CorrelationID UUID
	// State is the saga's JSON state, including its compensation stack.
	State         json.RawMessage
	Status        SagaStatus
	CurrentStep   string
	FailureReason string
}

// SagaRecord is a stored saga.
type SagaRecord struct {
	SagaSaveInput
	// StartedAt is when the saga was first saved. Later saves keep it.
	StartedAt time.Time
	// UpdatedAt is when the saga was last written.
	UpdatedAt time.Time
	// CompletedAt is when the saga reached a terminal status; zero while it
	// is active.
	CompletedAt time.Time
}

// SagaQuery narrows a list. An empty SagaType matches every type, and a Limit
// of 0 or less means no limit. The type filter applies before the limit.
type SagaQuery struct {
	SagaType string
	Limit    int
}

// Saved returns the record after saving in over r, the record already stored
// (the zero SagaRecord if none), at time now. These are the write rules every
// adapter follows: StartedAt is kept from r (else now), UpdatedAt is now, and
// CompletedAt is set when the status is terminal (kept if r already had one)
// and cleared otherwise.
func (r SagaRecord) Saved(in SagaSaveInput, now time.Time) SagaRecord {
	out := SagaRecord{SagaSaveInput: in, StartedAt: r.StartedAt, UpdatedAt: now}
	if out.StartedAt.IsZero() {
		out.StartedAt = now
	}
	if in.Status.IsTerminal() {
		out.CompletedAt = r.CompletedAt
		if out.CompletedAt.IsZero() {
			out.CompletedAt = now
		}
	}
	return out
}

// Complete returns the record ended as COMPLETED at now, and true. A saga
// already in a terminal status is returned unchanged, with false: ending a
// saga twice is a no-op.
func (r SagaRecord) Complete(now time.Time) (SagaRecord, bool) {
	if r.Status.IsTerminal() {
		return r, false
	}
	r.Status, r.UpdatedAt, r.CompletedAt = SagaCompleted, now, now
	return r, true
}

// Fail returns the record ended as FAILED with reason at now, and true. A saga
// already in a terminal status is returned unchanged, with false.
func (r SagaRecord) Fail(reason string, now time.Time) (SagaRecord, bool) {
	if r.Status.IsTerminal() {
		return r, false
	}
	r.Status, r.FailureReason, r.UpdatedAt, r.CompletedAt = SagaFailed, reason, now, now
	return r, true
}

// IsTimedOut reports whether the saga is active and was last updated before
// the given time (exclusive).
func (r SagaRecord) IsTimedOut(before time.Time) bool {
	return r.Status.IsActive() && r.UpdatedAt.Before(before)
}

// SagaStoreAdapter is the storage contract for saga state (§9.1).
//
//   - Save is an upsert following SagaRecord.Saved.
//   - Complete and Fail follow SagaRecord.Complete and Fail, and do nothing
//     for a missing saga.
//   - LoadByStatus is ordered by UpdatedAt ascending, then type and ID.
//   - LoadTimedOut returns only active sagas, ordered the same way.
type SagaStoreAdapter interface {
	// Save stores the saga, replacing the one with the same key.
	Save(ctx context.Context, in SagaSaveInput) error
	// Load returns the saga, and false if there is none.
	Load(ctx context.Context, sagaType string, sagaID UUID) (SagaRecord, bool, error)
	// LoadByCorrelationID returns the saga of that type started by the
	// correlation ID (its saga ID), and false if there is none.
	LoadByCorrelationID(ctx context.Context, sagaType string, correlationID UUID) (SagaRecord, bool, error)
	// Complete marks the saga COMPLETED. Idempotent.
	Complete(ctx context.Context, sagaType string, sagaID UUID) error
	// Fail marks the saga FAILED with reason. Idempotent.
	Fail(ctx context.Context, sagaType string, sagaID UUID, reason string) error
	// LoadByStatus lists sagas with the status.
	LoadByStatus(ctx context.Context, status SagaStatus, q SagaQuery) ([]SagaRecord, error)
	// LoadTimedOut lists active sagas last updated before the given time.
	LoadTimedOut(ctx context.Context, before time.Time, q SagaQuery) ([]SagaRecord, error)
	// Delete removes the saga.
	Delete(ctx context.Context, sagaType string, sagaID UUID) error
	// Count returns how many sagas match; an empty type or status matches all.
	Count(ctx context.Context, sagaType string, status SagaStatus) (int, error)
}
