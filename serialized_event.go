package tarodomain

import (
	"encoding/json"
	"fmt"
	"time"
)

// timestampLayout always has exactly three fractional digits, which is how
// every store writes occurred_at.
const timestampLayout = "2006-01-02T15:04:05.000Z"

// FormatTimestamp writes t as ISO 8601 in UTC with exactly three fractional
// digits, for example 2026-01-02T03:04:05.006Z.
func FormatTimestamp(t time.Time) string { return NormalizeTime(t).Format(timestampLayout) }

// ParseTimestamp reads an ISO 8601 / RFC 3339 timestamp with any offset and
// returns it in UTC, cut to milliseconds.
func ParseTimestamp(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, NewError(CodeSerializationError, fmt.Sprintf("invalid timestamp %q", s)).WithCause(err)
	}
	return NormalizeTime(t), nil
}

// SerializedEvent is an event in its stored form (§2.1). A zero CausationID
// is stored as null.
type SerializedEvent struct {
	EventID            UUID
	EventType          string
	EventVersion       string
	CausationID        UUID
	CorrelationID      UUID
	CommandCausationID UUID
	Data               json.RawMessage
	Metadata           Metadata
	OccurredAt         time.Time
}

// Serialize converts the event to its stored form. A payload that has no JSON
// form is SERIALIZATION_ERROR.
func (d DomainEvent) Serialize() (SerializedEvent, error) {
	data, err := json.Marshal(d.payload)
	if err != nil {
		return SerializedEvent{}, NewError(CodeSerializationError,
			fmt.Sprintf("cannot serialize event %s", d.name)).
			WithContext("eventType", d.name).WithCause(err)
	}
	e := d.envelope
	return SerializedEvent{
		EventID:            e.ID,
		EventType:          d.name,
		EventVersion:       d.version,
		CausationID:        e.CausationID,
		CorrelationID:      e.CorrelationID,
		CommandCausationID: e.CommandCausationID,
		Data:               data,
		Metadata:           e.Metadata,
		OccurredAt:         e.OccurredAt,
	}, nil
}

// Envelope returns the stored form's tracing data as an EventEnvelope.
func (s SerializedEvent) Envelope() EventEnvelope {
	return EventEnvelope{
		ID:                 s.EventID,
		CorrelationID:      s.CorrelationID,
		CausationID:        s.CausationID,
		CommandCausationID: s.CommandCausationID,
		Metadata:           s.Metadata,
		OccurredAt:         s.OccurredAt,
	}
}

type serializedEventJSON struct {
	EventID            string          `json:"event_id"`
	EventType          string          `json:"event_type"`
	EventVersion       string          `json:"event_version"`
	CausationID        *string         `json:"causation_id"`
	CorrelationID      string          `json:"correlation_id"`
	CommandCausationID *string         `json:"command_causation_id,omitempty"`
	Data               json.RawMessage `json:"data"`
	Metadata           Metadata        `json:"metadata"`
	OccurredAt         string          `json:"occurred_at"`
}

func optionalID(id UUID) *string {
	if id.IsZero() {
		return nil
	}
	s := id.String()
	return &s
}

// MarshalJSON writes the §2.1 field names.
func (s SerializedEvent) MarshalJSON() ([]byte, error) {
	data := s.Data
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	return json.Marshal(serializedEventJSON{
		EventID:            s.EventID.String(),
		EventType:          s.EventType,
		EventVersion:       s.EventVersion,
		CausationID:        optionalID(s.CausationID),
		CorrelationID:      s.CorrelationID.String(),
		CommandCausationID: optionalID(s.CommandCausationID),
		Data:               data,
		Metadata:           s.Metadata,
		OccurredAt:         FormatTimestamp(s.OccurredAt),
	})
}

// UnmarshalJSON reads the §2.1 field names. A bad ID is the matching
// INVALID_*_ID error and a bad timestamp is SERIALIZATION_ERROR.
func (s *SerializedEvent) UnmarshalJSON(b []byte) error {
	var w serializedEventJSON
	if err := json.Unmarshal(b, &w); err != nil {
		if _, ok := AsError(err); ok {
			return err
		}
		return NewError(CodeSerializationError, "invalid serialized event").WithCause(err)
	}
	var out SerializedEvent
	var err error
	if out.EventID, err = ParseEventID(w.EventID); err != nil {
		return err
	}
	if out.CorrelationID, err = ParseCorrelationID(w.CorrelationID); err != nil {
		return err
	}
	if w.CausationID != nil {
		if out.CausationID, err = ParseCausationID(*w.CausationID); err != nil {
			return err
		}
	}
	if w.CommandCausationID != nil {
		if out.CommandCausationID, err = ParseCommandCausationID(*w.CommandCausationID); err != nil {
			return err
		}
	}
	if out.OccurredAt, err = ParseTimestamp(w.OccurredAt); err != nil {
		return err
	}
	out.EventType, out.EventVersion, out.Data, out.Metadata = w.EventType, w.EventVersion, w.Data, w.Metadata
	*s = out
	return nil
}
