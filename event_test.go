package tarodomain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type moneyDeposited struct {
	Amount int64 `json:"amount"`
}

func (moneyDeposited) EventName() string { return "MoneyDeposited" }

type moneyWithdrawn struct {
	Amount int64 `json:"amount"`
}

func (moneyWithdrawn) EventName() string    { return "MoneyWithdrawn" }
func (moneyWithdrawn) EventVersion() string { return "2.1.0" }

type unserializable struct{ F func() }

func (unserializable) EventName() string { return "Bad" }

func newEvent(t *testing.T, ctx context.Context, p Event) DomainEvent {
	t.Helper()
	d, err := NewDomainEvent(NewEventEnvelope(ctx), p)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDomainEventNameAndVersion(t *testing.T) {
	d := newEvent(t, context.Background(), moneyDeposited{5})
	if d.Name() != "MoneyDeposited" || d.Version() != "1.0.0" {
		t.Errorf("%s %s", d.Name(), d.Version())
	}
	if v := newEvent(t, context.Background(), moneyWithdrawn{1}).Version(); v != "2.1.0" {
		t.Errorf("version %s", v)
	}
}

func TestPointerAndNilPayloadRejected(t *testing.T) {
	if _, err := NewDomainEvent(EventEnvelope{}, &moneyDeposited{}); !isCode(err, CodeInvalidConfiguration) {
		t.Errorf("pointer: %v", err)
	}
	if _, err := NewDomainEvent(EventEnvelope{}, nil); !isCode(err, CodeInvalidConfiguration) {
		t.Errorf("nil: %v", err)
	}
}

func TestEventAs(t *testing.T) {
	d := newEvent(t, context.Background(), moneyDeposited{7})
	r, ok := EventAs[moneyDeposited](d)
	if !ok || r.Event.Amount != 7 || r.Envelope.ID != d.Envelope().ID {
		t.Errorf("%+v %v", r, ok)
	}
	if _, ok := EventAs[moneyWithdrawn](d); ok {
		t.Error("wrong type matched")
	}
	if !EventIs[moneyDeposited](d) || EventIs[moneyWithdrawn](d) {
		t.Error("EventIs wrong")
	}
}

func TestSerializedEventShapeAndRoundTrip(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC)
	ec := ExecutionContext{CorrelationID: NewUUIDv7(), CausationID: NewUUIDv7(),
		CommandCausationID: NewUUIDv7(), Metadata: MustMetadata(map[string]any{"u": "x"})}
	ctx := WithClock(WithExecutionContext(context.Background(), ec), NewManualClock(at))
	s, err := newEvent(t, ctx, moneyDeposited{42}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"event_id", "event_type", "event_version", "causation_id",
		"correlation_id", "command_causation_id", "data", "metadata", "occurred_at"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing field %s in %s", k, b)
		}
	}
	if string(m["occurred_at"]) != `"2026-01-02T03:04:05.006Z"` || string(m["data"]) != `{"amount":42}` {
		t.Errorf("got %s", b)
	}
	var back SerializedEvent
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.EventID != s.EventID || back.CausationID != s.CausationID || back.CommandCausationID != s.CommandCausationID ||
		!back.OccurredAt.Equal(at) || !back.Metadata.Equal(s.Metadata) || string(back.Data) != string(s.Data) {
		t.Errorf("round trip changed: %+v vs %+v", back, s)
	}
}

func TestSerializedEventNoCausation(t *testing.T) {
	s, _ := newEvent(t, context.Background(), moneyDeposited{1}).Serialize()
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"causation_id":null`) || strings.Contains(string(b), "command_causation_id") {
		t.Errorf("got %s", b)
	}
}

func TestSerializedEventErrors(t *testing.T) {
	d, _ := NewDomainEvent(EventEnvelope{}, unserializable{})
	if _, err := d.Serialize(); !isCode(err, CodeSerializationError) {
		t.Errorf("got %v", err)
	}
	var s SerializedEvent
	bad := `{"event_id":"x","correlation_id":"x","occurred_at":"2026-01-01T00:00:00.000Z","data":{}}`
	if err := json.Unmarshal([]byte(bad), &s); !isCode(err, CodeInvalidEventID) {
		t.Errorf("got %v", err)
	}
}

func TestTimestamps(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := FormatTimestamp(at); got != "2026-01-02T03:04:05.000Z" {
		t.Errorf("got %s", got)
	}
	p, err := ParseTimestamp("2026-01-02T10:04:05.0069+07:00")
	if err != nil || !p.Equal(time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC)) {
		t.Errorf("%v %v", p, err)
	}
	if _, err := ParseTimestamp("yesterday"); !isCode(err, CodeSerializationError) {
		t.Errorf("got %v", err)
	}
}
