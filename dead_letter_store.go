package tarodomain

import (
	"context"
	"encoding/json"
	"time"
)

// DeadLetterInput is a failed event to quarantine (§21.1).
type DeadLetterInput struct {
	// HandlerName is the handler that failed.
	HandlerName string
	// EventID, EventType and EventData identify and carry the event.
	EventID   UUID
	EventType string
	EventData json.RawMessage
	// CorrelationID is the event's correlation ID.
	CorrelationID UUID
	// ErrorCode is the framework error code, if the failure had one.
	ErrorCode string
	// ErrorMessage is the failure's message.
	ErrorMessage string
	// Attempts is how many times the handler ran.
	Attempts int
}

// DeadLetterEntry is a stored dead letter.
type DeadLetterEntry struct {
	DeadLetterInput
	// ID identifies the entry. IDs increase in write order and are never
	// reused, even after a delete.
	ID int64
	// CreatedAt is when the entry was written.
	CreatedAt time.Time
}

// DeadLetterStore quarantines events a handler could not process (§21.1).
//
//   - Write returns the new entry's ID. IDs increase and are never reused.
//   - List is ordered by ID.
//   - Get returns the latest entry for the event and handler.
//   - Deleting what doesn't exist is a no-op. DeleteOld is exclusive of its
//     cut-off.
type DeadLetterStore interface {
	// Write stores the entry and returns its ID.
	Write(ctx context.Context, in DeadLetterInput) (int64, error)
	// List returns the handler's entries in ID order. A limit of 0 or less
	// means no limit.
	List(ctx context.Context, handlerName string, limit int) ([]DeadLetterEntry, error)
	// Get returns the latest entry for the event and handler, and false if
	// there is none.
	Get(ctx context.Context, eventID UUID, handlerName string) (DeadLetterEntry, bool, error)
	// GetByID returns the entry with the ID, and false if there is none.
	GetByID(ctx context.Context, id int64) (DeadLetterEntry, bool, error)
	// Delete removes the entry.
	Delete(ctx context.Context, id int64) error
	// Count returns how many entries the handler has; an empty name counts
	// all.
	Count(ctx context.Context, handlerName string) (int, error)
	// DeleteOld removes entries written before olderThan and returns how many.
	DeleteOld(ctx context.Context, olderThan time.Time) (int, error)
}
