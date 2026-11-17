package tarodomain

import (
	"context"
	"time"
)

// ProcessedEventStore remembers which events a handler instance has handled
// (§12.3), so a redelivered event is skipped. The key is (handler name,
// handler instance, event ID).
//
// For a projection, name and instance are both the projection's name (or its
// shard's). For a saga, the name is the saga type and the instance its ID.
//
// MarkProcessed is idempotent: marking twice keeps the first record. Deleting
// what doesn't exist is a no-op. DeleteOld is exclusive of its cut-off.
type ProcessedEventStore interface {
	// Exists reports whether the event was marked for this handler instance.
	Exists(ctx context.Context, handlerName, handlerInstance string, eventID UUID) (bool, error)
	// MarkProcessed records the event as handled.
	MarkProcessed(ctx context.Context, handlerName, handlerInstance string, eventID UUID) error
	// Count returns how many events are marked for the handler; an empty
	// instance counts every instance.
	Count(ctx context.Context, handlerName, handlerInstance string) (int, error)
	// DeleteByHandler removes the handler's marks (every instance if
	// handlerInstance is empty) and returns how many it removed.
	DeleteByHandler(ctx context.Context, handlerName, handlerInstance string) (int, error)
	// DeleteOld removes marks made before olderThan and returns how many.
	DeleteOld(ctx context.Context, olderThan time.Time) (int, error)
}

// ProcessedCommand is the record of a command that ran (§14.1).
type ProcessedCommand struct {
	CommandID   UUID
	CommandType string
	HandlerName string
	// ProcessedAt is when the command was first marked.
	ProcessedAt time.Time
}

// CommandIdempotencyStore remembers which commands ran (§14.1). The pipeline
// checks it before dispatch and marks after a successful save.
//
// MarkProcessed is idempotent: marking twice keeps the first record. List is
// ordered by ProcessedAt, then command ID.
type CommandIdempotencyStore interface {
	// Exists reports whether the command was marked.
	Exists(ctx context.Context, commandID UUID) (bool, error)
	// MarkProcessed records the command as run.
	MarkProcessed(ctx context.Context, commandID UUID, commandType, handlerName string) error
	// ProcessedAt returns when the command was first marked, and false if it
	// wasn't.
	ProcessedAt(ctx context.Context, commandID UUID) (time.Time, bool, error)
	// Count returns how many commands are marked; an empty handler name counts
	// all.
	Count(ctx context.Context, handlerName string) (int, error)
	// List returns the handler's processed commands, oldest first. A limit of
	// 0 or less means no limit.
	List(ctx context.Context, handlerName string, limit int) ([]ProcessedCommand, error)
	// DeleteOld removes records made before olderThan and returns how many.
	DeleteOld(ctx context.Context, olderThan time.Time) (int, error)
}
