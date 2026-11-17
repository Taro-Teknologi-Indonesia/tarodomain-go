package tarodomain

import (
	"context"
	"testing"
	"time"
)

func TestEnvelopeDefaultsOutsideContext(t *testing.T) {
	e := NewEventEnvelope(context.Background())
	if !e.ID.IsV7() || !e.CorrelationID.IsV7() {
		t.Error("IDs should be v7")
	}
	if !e.CausationID.IsZero() || !e.CommandCausationID.IsZero() {
		t.Error("no causation outside a context")
	}
	if e.OccurredAt.Location() != time.UTC || e.OccurredAt.Nanosecond()%1e6 != 0 {
		t.Errorf("time not UTC milliseconds: %v", e.OccurredAt)
	}
	c := NewCommandEnvelope(context.Background())
	if !c.ID.IsV7() || !c.CorrelationID.IsV7() || !c.CausationID.IsZero() {
		t.Error("bad command defaults")
	}
	if e.CorrelationID == NewEventEnvelope(context.Background()).CorrelationID {
		t.Error("each root should be new")
	}
}

func TestEnvelopeInheritsFromContext(t *testing.T) {
	ec := ExecutionContext{
		CorrelationID:      NewUUIDv7(),
		CausationID:        NewUUIDv7(),
		CommandCausationID: NewUUIDv7(),
		Metadata:           MustMetadata(map[string]any{"user": "u1"}),
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 6_789_000, time.FixedZone("x", 3600))
	ctx := WithClock(WithExecutionContext(context.Background(), ec), NewManualClock(at))

	e := NewEventEnvelope(ctx)
	if e.CorrelationID != ec.CorrelationID || e.CausationID != ec.CausationID ||
		e.CommandCausationID != ec.CommandCausationID || !e.Metadata.Equal(ec.Metadata) {
		t.Errorf("event envelope %+v", e)
	}
	if want := time.Date(2026, 1, 2, 2, 4, 5, 6_000_000, time.UTC); !e.OccurredAt.Equal(want) {
		t.Errorf("OccurredAt %v, want %v", e.OccurredAt, want)
	}
	c := NewCommandEnvelope(ctx)
	if c.CorrelationID != ec.CorrelationID || c.CausationID != ec.CausationID || !c.Timestamp.Equal(e.OccurredAt) {
		t.Errorf("command envelope %+v", c)
	}
}

func TestParseIDCodes(t *testing.T) {
	cases := []struct {
		parse func(string) (UUID, error)
		code  ErrorCode
	}{
		{ParseEventID, CodeInvalidEventID},
		{ParseCorrelationID, CodeInvalidCorrelationID},
		{ParseCausationID, CodeInvalidCausationID},
		{ParseCommandID, CodeInvalidCommandID},
		{ParseCommandCorrelationID, CodeInvalidCommandCorrelationID},
		{ParseCommandCausationID, CodeInvalidCommandCausationID},
	}
	good := NewUUIDv7()
	for _, tc := range cases {
		if _, err := tc.parse("nope"); !isCode(err, tc.code) {
			t.Errorf("%s: got %v", tc.code, err)
		}
		if u, err := tc.parse(good.String()); err != nil || u != good {
			t.Errorf("%s: valid ID failed: %v", tc.code, err)
		}
	}
}

func TestExecutionContextPropagation(t *testing.T) {
	// §28.2: request → command → event → saga command.
	root := NewExecutionContext(UUID{})
	if root.CorrelationID.IsZero() || !root.CausationID.IsZero() {
		t.Fatalf("root %+v", root)
	}
	ctx := WithExecutionContext(context.Background(), root)

	cmd := NewCommandEnvelope(ctx)
	if cmd.CorrelationID != root.CorrelationID {
		t.Fatal("command lost correlation")
	}
	cmdCtx := WithExecutionContext(ctx, root.ForCommand(cmd))
	ev := NewEventEnvelope(cmdCtx)
	if ev.CorrelationID != root.CorrelationID || ev.CausationID != cmd.ID || ev.CommandCausationID != cmd.CausationID {
		t.Fatalf("event %+v from command %+v", ev, cmd)
	}
	evCtx := WithExecutionContext(ctx, root.ForEvent(ev))
	next := NewCommandEnvelope(evCtx)
	if next.CorrelationID != root.CorrelationID || next.CausationID != ev.ID {
		t.Fatalf("saga command %+v", next)
	}
	if !root.ForEvent(ev).CommandCausationID.IsZero() {
		t.Error("ForEvent should clear CommandCausationID")
	}
}

func TestExecutionContextDerivedDoesNotChangeParent(t *testing.T) {
	parent := WithExecutionContext(context.Background(), NewExecutionContext(UUID{}))
	before, _ := ExecutionContextFrom(parent)
	child := WithExecutionContext(parent, NewExecutionContext(UUID{}))
	after, _ := ExecutionContextFrom(parent)
	if before.CorrelationID != after.CorrelationID {
		t.Error("parent changed")
	}
	if c, _ := ExecutionContextFrom(child); c.CorrelationID == before.CorrelationID {
		t.Error("child not replaced")
	}
	if _, ok := ExecutionContextFrom(context.Background()); ok {
		t.Error("empty context reported one")
	}
}
