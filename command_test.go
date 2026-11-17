package tarodomain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type deposit struct {
	Amount int64 `json:"amount"`
}

func (deposit) CommandName() string { return "Deposit" }

type withdraw struct{}

func (withdraw) CommandName() string { return "Withdraw" }

type badCommand struct{ F func() }

func (badCommand) CommandName() string { return "Bad" }

func TestCommandMessageDefaultsFromContext(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 8_000_000, time.UTC)
	ec := ExecutionContext{CorrelationID: NewUUIDv7(), CausationID: NewUUIDv7(),
		Metadata: MustMetadata(map[string]any{"k": "v"})}
	ctx := WithClock(WithExecutionContext(context.Background(), ec), NewManualClock(at))
	m := NewCommandMessage(ctx, deposit{5})
	e := m.Envelope
	if !e.ID.IsV7() || e.CorrelationID != ec.CorrelationID || e.CausationID != ec.CausationID ||
		!e.Metadata.Equal(ec.Metadata) || !e.Timestamp.Equal(at) {
		t.Errorf("%+v", e)
	}
}

func TestCommandMessageWithMethods(t *testing.T) {
	m := NewCommandMessage(context.Background(), deposit{1})
	id, corr, cause := NewUUIDv7(), NewUUIDv7(), NewUUIDv7()
	md := MustMetadata(map[string]any{"a": 1})
	n := m.WithID(id).WithCorrelationID(corr).WithCausationID(cause).WithMetadata(md)
	if n.Envelope.ID != id || n.Envelope.CorrelationID != corr || n.Envelope.CausationID != cause ||
		!n.Envelope.Metadata.Equal(md) || n.Command.Amount != 1 {
		t.Errorf("%+v", n)
	}
	if m.Envelope.ID == id || m.Envelope.Metadata.Len() != 0 {
		t.Error("With… changed the original")
	}
}

func TestAnyCommandErasureAndCommandAs(t *testing.T) {
	m := NewCommandMessage(context.Background(), deposit{9})
	a := Erase(m)
	if a.Name() != "Deposit" || a.Envelope().ID != m.Envelope.ID {
		t.Errorf("%+v", a)
	}
	back, ok := CommandAs[deposit](a)
	if !ok || back.Command.Amount != 9 || back.Envelope.ID != m.Envelope.ID {
		t.Errorf("%+v %v", back, ok)
	}
	if _, ok := CommandAs[withdraw](a); ok {
		t.Error("wrong type matched")
	}
}

func TestSerializedCommandShape(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	ctx := WithClock(context.Background(), NewManualClock(at))
	s, err := Erase(NewCommandMessage(ctx, deposit{3})).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"command_id", "command_type", "causation_id", "correlation_id", "data", "metadata", "timestamp"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %s in %s", k, b)
		}
	}
	if string(m["timestamp"]) != `"2026-03-04T05:06:07.000Z"` || string(m["causation_id"]) != "null" ||
		string(m["data"]) != `{"amount":3}` {
		t.Errorf("got %s", b)
	}
	var back SerializedCommand
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.CommandID != s.CommandID || back.CorrelationID != s.CorrelationID || !back.Timestamp.Equal(at) ||
		back.CommandType != "Deposit" || !back.CausationID.IsZero() {
		t.Errorf("%+v", back)
	}
}

func TestSerializedCommandErrors(t *testing.T) {
	if _, err := Erase(NewCommandMessage(context.Background(), badCommand{})).Serialize(); !isCode(err, CodeSerializationError) {
		t.Errorf("got %v", err)
	}
	id := NewUUIDv7().String()
	cases := map[string]ErrorCode{
		`{"command_id":"x","correlation_id":"` + id + `","timestamp":"2026-01-01T00:00:00.000Z"}`:                             CodeInvalidCommandID,
		`{"command_id":"` + id + `","correlation_id":"x","timestamp":"2026-01-01T00:00:00.000Z"}`:                             CodeInvalidCommandCorrelationID,
		`{"command_id":"` + id + `","correlation_id":"` + id + `","causation_id":"x","timestamp":"2026-01-01T00:00:00.000Z"}`: CodeInvalidCommandCausationID,
		`{"command_id":"` + id + `","correlation_id":"` + id + `","timestamp":"soon"}`:                                        CodeSerializationError,
	}
	for in, code := range cases {
		var s SerializedCommand
		if err := json.Unmarshal([]byte(in), &s); !isCode(err, code) {
			t.Errorf("%s: got %v, want %s", strings.Split(in, ",")[0], err, code)
		}
	}
}

func TestDerivedCommandIDs(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 8_000_000, time.UTC)
	a := Erase(NewCommandMessage(context.Background(), deposit{1}))
	x := a.WithDerivedID("saga|evt", 0, at)
	y := Erase(NewCommandMessage(context.Background(), deposit{1})).WithDerivedID("saga|evt", 0, at)
	if x.Envelope().ID != y.Envelope().ID {
		t.Error("derived IDs differ for the same input")
	}
	if !x.Envelope().ID.IsV7() || x.Envelope().ID.UnixMilli() != at.UnixMilli() {
		t.Error("derived ID should be v7 with the event's time")
	}
	if x.Envelope().ID == a.WithDerivedID("saga|evt", 1, at).Envelope().ID {
		t.Error("index ignored")
	}
	if a.Envelope().ID == x.Envelope().ID {
		t.Error("original changed")
	}
}
