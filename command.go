package tarodomain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Command is a request to change state (§2.2): a plain struct of business
// fields. CommandName is the registered type name; implement it on a value
// receiver.
type Command interface {
	CommandName() string
}

// CommandMessage is a command with its envelope. It is a value; the With…
// methods return changed copies.
type CommandMessage[C Command] struct {
	Envelope CommandEnvelope
	Command  C
}

// NewCommandMessage wraps c in an envelope made from the execution context and
// clock in ctx (see NewCommandEnvelope).
func NewCommandMessage[C Command](ctx context.Context, c C) CommandMessage[C] {
	return CommandMessage[C]{Envelope: NewCommandEnvelope(ctx), Command: c}
}

// WithID returns a copy with the command ID set, for idempotent retries.
func (m CommandMessage[C]) WithID(id UUID) CommandMessage[C] {
	m.Envelope.ID = id
	return m
}

// WithCorrelationID returns a copy with the correlation ID set.
func (m CommandMessage[C]) WithCorrelationID(id UUID) CommandMessage[C] {
	m.Envelope.CorrelationID = id
	return m
}

// WithCausationID returns a copy with the causation ID set.
func (m CommandMessage[C]) WithCausationID(id UUID) CommandMessage[C] {
	m.Envelope.CausationID = id
	return m
}

// WithMetadata returns a copy with the metadata replaced.
func (m CommandMessage[C]) WithMetadata(md Metadata) CommandMessage[C] {
	m.Envelope.Metadata = md
	return m
}

// AnyCommand is a command with its Go type erased, for queues, registries and
// sagas that handle commands of many types.
type AnyCommand struct {
	name     string
	envelope CommandEnvelope
	payload  Command
}

// Erase turns a typed message into an AnyCommand.
func Erase[C Command](m CommandMessage[C]) AnyCommand {
	return AnyCommand{name: m.Command.CommandName(), envelope: m.Envelope, payload: m.Command}
}

// Name returns the command's registered type name.
func (a AnyCommand) Name() string { return a.name }

// Envelope returns the command's envelope.
func (a AnyCommand) Envelope() CommandEnvelope { return a.envelope }

// Payload returns the business command.
func (a AnyCommand) Payload() Command { return a.payload }

// CommandAs returns the typed message if the payload is a C.
func CommandAs[C Command](a AnyCommand) (CommandMessage[C], bool) {
	c, ok := a.payload.(C)
	if !ok {
		return CommandMessage[C]{}, false
	}
	return CommandMessage[C]{Envelope: a.envelope, Command: c}, true
}

// WithDerivedID returns a copy whose ID is DerivedUUIDv7(seed, index, at): the
// same inputs always give the same ID, so a saga that handles a redelivered
// event recreates the same command IDs and command idempotency skips them.
func (a AnyCommand) WithDerivedID(seed string, index int, at time.Time) AnyCommand {
	a.envelope.ID = DerivedUUIDv7(seed, index, at.UnixMilli())
	return a
}

// SerializedCommand is a command in its stored or wire form (§2.2). A zero
// CausationID is written as null.
type SerializedCommand struct {
	CommandID     UUID
	CommandType   string
	CausationID   UUID
	CorrelationID UUID
	Data          json.RawMessage
	Metadata      Metadata
	Timestamp     time.Time
}

// Serialize converts the command to its stored form. A payload that has no
// JSON form is SERIALIZATION_ERROR.
func (a AnyCommand) Serialize() (SerializedCommand, error) {
	data, err := json.Marshal(a.payload)
	if err != nil {
		return SerializedCommand{}, NewError(CodeSerializationError,
			fmt.Sprintf("cannot serialize command %s", a.name)).
			WithContext("commandType", a.name).WithCause(err)
	}
	e := a.envelope
	return SerializedCommand{
		CommandID:     e.ID,
		CommandType:   a.name,
		CausationID:   e.CausationID,
		CorrelationID: e.CorrelationID,
		Data:          data,
		Metadata:      e.Metadata,
		Timestamp:     e.Timestamp,
	}, nil
}

// Envelope returns the stored form's tracing data as a CommandEnvelope.
func (s SerializedCommand) Envelope() CommandEnvelope {
	return CommandEnvelope{
		ID:            s.CommandID,
		CorrelationID: s.CorrelationID,
		CausationID:   s.CausationID,
		Metadata:      s.Metadata,
		Timestamp:     s.Timestamp,
	}
}

type serializedCommandJSON struct {
	CommandID     string          `json:"command_id"`
	CommandType   string          `json:"command_type"`
	CausationID   *string         `json:"causation_id"`
	CorrelationID string          `json:"correlation_id"`
	Data          json.RawMessage `json:"data"`
	Metadata      Metadata        `json:"metadata"`
	Timestamp     string          `json:"timestamp"`
}

// MarshalJSON writes the §2.2 field names.
func (s SerializedCommand) MarshalJSON() ([]byte, error) {
	data := s.Data
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	return json.Marshal(serializedCommandJSON{
		CommandID:     s.CommandID.String(),
		CommandType:   s.CommandType,
		CausationID:   optionalID(s.CausationID),
		CorrelationID: s.CorrelationID.String(),
		Data:          data,
		Metadata:      s.Metadata,
		Timestamp:     FormatTimestamp(s.Timestamp),
	})
}

// UnmarshalJSON reads the §2.2 field names. A bad ID is the matching
// INVALID_COMMAND_*_ID error and a bad timestamp is SERIALIZATION_ERROR.
func (s *SerializedCommand) UnmarshalJSON(b []byte) error {
	var w serializedCommandJSON
	if err := json.Unmarshal(b, &w); err != nil {
		if _, ok := AsError(err); ok {
			return err
		}
		return NewError(CodeSerializationError, "invalid serialized command").WithCause(err)
	}
	var out SerializedCommand
	var err error
	if out.CommandID, err = ParseCommandID(w.CommandID); err != nil {
		return err
	}
	if out.CorrelationID, err = ParseCommandCorrelationID(w.CorrelationID); err != nil {
		return err
	}
	if w.CausationID != nil {
		if out.CausationID, err = ParseCommandCausationID(*w.CausationID); err != nil {
			return err
		}
	}
	if out.Timestamp, err = ParseTimestamp(w.Timestamp); err != nil {
		return err
	}
	out.CommandType, out.Data, out.Metadata = w.CommandType, w.Data, w.Metadata
	*s = out
	return nil
}
