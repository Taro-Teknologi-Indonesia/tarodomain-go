package tarodomain

import (
	"context"
	"fmt"
	"time"
)

// EventEnvelope is the tracing data every event carries (§2.1). A zero UUID
// means "none".
type EventEnvelope struct {
	// ID identifies the event; a v7 UUID, so IDs sort roughly by time.
	ID UUID
	// CorrelationID is shared by everything one original request causes.
	CorrelationID UUID
	// CausationID is the command or event that directly caused this one.
	CausationID UUID
	// CommandCausationID is, for an event raised by a command, that
	// command's own causation ID. Commands aren't stored, so without it a
	// trace can't link a command to the event that caused it (§28.1).
	CommandCausationID UUID
	// Metadata is free-form primitive data.
	Metadata Metadata
	// OccurredAt is in UTC, in whole milliseconds.
	OccurredAt time.Time
}

// NewEventEnvelope makes an envelope from the execution context in ctx: a new
// v7 ID, the context's correlation ID (or a new root), its causation IDs and
// metadata, and the time from Now(ctx).
func NewEventEnvelope(ctx context.Context) EventEnvelope {
	ec, _ := ExecutionContextFrom(ctx)
	return EventEnvelope{
		ID:                 NewUUIDv7(),
		CorrelationID:      orNewRoot(ec.CorrelationID),
		CausationID:        ec.CausationID,
		CommandCausationID: ec.CommandCausationID,
		Metadata:           ec.Metadata,
		OccurredAt:         Now(ctx),
	}
}

// CommandEnvelope is the tracing data every command carries (§2.2). A zero
// UUID means "none".
type CommandEnvelope struct {
	// ID identifies the command and is the idempotency key (§14).
	ID UUID
	// CorrelationID is shared by everything one original request causes.
	CorrelationID UUID
	// CausationID is the command or event that directly caused this command.
	CausationID UUID
	// Metadata is free-form primitive data.
	Metadata Metadata
	// Timestamp is in UTC, in whole milliseconds.
	Timestamp time.Time
}

// NewCommandEnvelope makes an envelope from the execution context in ctx, the
// same way NewEventEnvelope does.
func NewCommandEnvelope(ctx context.Context) CommandEnvelope {
	ec, _ := ExecutionContextFrom(ctx)
	return CommandEnvelope{
		ID:            NewUUIDv7(),
		CorrelationID: orNewRoot(ec.CorrelationID),
		CausationID:   ec.CausationID,
		Metadata:      ec.Metadata,
		Timestamp:     Now(ctx),
	}
}

func orNewRoot(id UUID) UUID {
	if id.IsZero() {
		return NewUUIDv7()
	}
	return id
}

func parseID(code ErrorCode, field, s string) (UUID, error) {
	u, err := ParseUUID(s)
	if err != nil {
		return UUID{}, NewError(code, fmt.Sprintf("invalid %s: %q", field, s)).
			WithContext("value", s).WithCause(err)
	}
	return u, nil
}

// ParseEventID parses an event ID; failure is INVALID_EVENT_ID.
func ParseEventID(s string) (UUID, error) { return parseID(CodeInvalidEventID, "event ID", s) }

// ParseCorrelationID parses an event's correlation ID; failure is
// INVALID_CORRELATION_ID.
func ParseCorrelationID(s string) (UUID, error) {
	return parseID(CodeInvalidCorrelationID, "correlation ID", s)
}

// ParseCausationID parses an event's causation ID; failure is
// INVALID_CAUSATION_ID.
func ParseCausationID(s string) (UUID, error) {
	return parseID(CodeInvalidCausationID, "causation ID", s)
}

// ParseCommandID parses a command ID; failure is INVALID_COMMAND_ID.
func ParseCommandID(s string) (UUID, error) { return parseID(CodeInvalidCommandID, "command ID", s) }

// ParseCommandCorrelationID parses a command's correlation ID; failure is
// INVALID_COMMAND_CORRELATION_ID.
func ParseCommandCorrelationID(s string) (UUID, error) {
	return parseID(CodeInvalidCommandCorrelationID, "command correlation ID", s)
}

// ParseCommandCausationID parses a command's causation ID; failure is
// INVALID_COMMAND_CAUSATION_ID.
func ParseCommandCausationID(s string) (UUID, error) {
	return parseID(CodeInvalidCommandCausationID, "command causation ID", s)
}
