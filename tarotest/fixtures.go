package tarotest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// epoch is the documented origin of every fixture time: 2026-01-01T00:00:00Z.
// Fixture n is n milliseconds after it.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// timeOf returns the fixture time n milliseconds after the epoch.
func timeOf(n int) time.Time { return epoch.Add(time.Duration(n) * time.Millisecond) }

// eventID returns a UUIDv7-shaped event ID whose order follows n: its
// timestamp is timeOf(n) and the rest is zero.
func eventID(n int) tarodomain.UUID { return eventIDSeq(n, 0) }

// eventIDSeq returns an ID with the timestamp of timeOf(n) and a sequence in
// its random bits, so two IDs of the same millisecond differ.
func eventIDSeq(n int, seq uint16) tarodomain.UUID {
	ms := uint64(timeOf(n).UnixMilli())
	var u tarodomain.UUID
	for i := 0; i < 6; i++ {
		u[i] = byte(ms >> (8 * (5 - i)))
	}
	u[6] = 0x70
	u[8] = 0x80
	u[14], u[15] = byte(seq>>8), byte(seq)
	return u
}

// reversedPair returns two IDs of one millisecond that sort in reverse of the
// order they are appended in: first is appended first and compares greater.
func reversedPair(n int) (first, second tarodomain.UUID) {
	return eventIDSeq(n, 2), eventIDSeq(n, 1)
}

// otherID returns a non-event UUID (correlation and causation IDs) that
// differs for each n.
func otherID(n int) tarodomain.UUID {
	u := eventIDSeq(1000+n, 7)
	u[13] = 0xAA
	return u
}

// newStream returns the stream ID of a new aggregate of the type.
func newStream(aggregateType string) string {
	return tarodomain.StreamForAggregate(aggregateType, tarodomain.NewAggregateID())
}

// plainEvent returns an event with the required fields only.
func plainEvent(id tarodomain.UUID, n int) tarodomain.SerializedEvent {
	return tarodomain.SerializedEvent{
		EventID:      id,
		EventType:    "ContractEvent",
		EventVersion: "1.0.0",
		Data:         json.RawMessage(fmt.Sprintf(`{"n":%d}`, n)),
		OccurredAt:   timeOf(n),
	}
}

// fullEvent returns an event with every optional field set.
func fullEvent(id tarodomain.UUID, n int) tarodomain.SerializedEvent {
	e := plainEvent(id, n)
	e.EventVersion = "2.3.4"
	e.CorrelationID = otherID(1)
	e.CausationID = otherID(2)
	e.CommandCausationID = otherID(3)
	e.Metadata, _ = tarodomain.NewMetadata(map[string]any{"source": "contract", "flag": true})
	return e
}

// events returns plain events with IDs eventID(n) for each n.
func events(ns ...int) []tarodomain.SerializedEvent {
	out := make([]tarodomain.SerializedEvent, len(ns))
	for i, n := range ns {
		out[i] = plainEvent(eventID(n), n)
	}
	return out
}

// numbers returns the n of each event, read from its data. A failed read
// gives -1.
func numbers(es []tarodomain.SerializedEvent) []int {
	out := make([]int, len(es))
	for i, e := range es {
		var d struct{ N int }
		if json.Unmarshal(e.Data, &d) != nil {
			d.N = -1
		}
		out[i] = d.N
	}
	return out
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameEvent reports why two stored events differ, or "" if they are equal.
// JSON data is compared as JSON: a store may reformat whitespace.
func sameEvent(want, got tarodomain.SerializedEvent) string {
	switch {
	case want.EventID != got.EventID:
		return "event ID"
	case want.EventType != got.EventType:
		return "event type"
	case want.EventVersion != got.EventVersion:
		return "event version"
	case want.CorrelationID != got.CorrelationID:
		return "correlation ID"
	case want.CausationID != got.CausationID:
		return "causation ID"
	case want.CommandCausationID != got.CommandCausationID:
		return "command causation ID"
	case !want.OccurredAt.Equal(got.OccurredAt):
		return fmt.Sprintf("occurred at (%v vs %v)", want.OccurredAt, got.OccurredAt)
	case !want.Metadata.Equal(got.Metadata):
		return "metadata"
	}
	var a, b any
	if json.Unmarshal(want.Data, &a) != nil || json.Unmarshal(got.Data, &b) != nil {
		return "data (not JSON)"
	}
	wa, _ := json.Marshal(a)
	wb, _ := json.Marshal(b)
	if string(wa) != string(wb) {
		return fmt.Sprintf("data (%s vs %s)", wa, wb)
	}
	return ""
}

// fresh returns a provider with empty storage, initialized.
func fresh(ctx context.Context, t T, newProvider Factory) tarodomain.Provider {
	t.Helper()
	p, err := newProvider(ctx)
	if err != nil {
		t.Fatalf("creating the provider: %v", err)
	}
	if err := p.Initialize(ctx); err != nil {
		t.Fatalf("initializing the provider: %v", err)
	}
	return p
}
