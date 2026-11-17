package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

const eventsTable = "events"

// entry is one stored event with its place in the global append order.
type entry struct {
	pos      int64
	streamID string
	category string
	version  int64
	event    tarodomain.SerializedEvent
}

type streamRow struct {
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

type subRow struct {
	sub     tarodomain.Subscription
	lastPos int64
}

// eventLog is the event store's one table: the events in append order, the
// streams and the subscriptions. It copies itself for a unit of work.
type eventLog struct {
	entries []entry // ascending by pos
	byID    map[tarodomain.UUID]int64
	streams map[string]streamRow
	subs    map[string]subRow
	nextPos int64
}

func newEventLog() *eventLog {
	return &eventLog{
		byID:    map[tarodomain.UUID]int64{},
		streams: map[string]streamRow{},
		subs:    map[string]subRow{},
		nextPos: 1,
	}
}

func (l *eventLog) clone() tableData {
	c := &eventLog{
		entries: make([]entry, len(l.entries)),
		byID:    make(map[tarodomain.UUID]int64, len(l.byID)),
		streams: make(map[string]streamRow, len(l.streams)),
		subs:    make(map[string]subRow, len(l.subs)),
		nextPos: l.nextPos,
	}
	copy(c.entries, l.entries) // stored events are never changed in place
	for k, v := range l.byID {
		c.byID[k] = v
	}
	for k, v := range l.streams {
		c.streams[k] = v
	}
	for k, v := range l.subs {
		c.subs[k] = v
	}
	return c
}

func eventLogOf(tb tables) *eventLog {
	if d, ok := tb[eventsTable]; ok {
		return d.(*eventLog)
	}
	l := newEventLog()
	tb[eventsTable] = l
	return l
}

func copyEvent(e tarodomain.SerializedEvent) tarodomain.SerializedEvent {
	e.Data = append(json.RawMessage(nil), e.Data...)
	return e
}

// EventStore is the in-memory [tarodomain.EventStoreAdapter].
type EventStore struct {
	db *Database
}

// NewEventStore returns an event store over db.
func NewEventStore(db *Database) *EventStore { return &EventStore{db: db} }

var _ tarodomain.EventStoreAdapter = (*EventStore)(nil)

func appendFailed(streamID, msg string) *tarodomain.Error {
	return tarodomain.NewError(tarodomain.CodeEventAppendFailed, msg).
		WithContext("streamId", streamID)
}

// AppendToStream implements [tarodomain.EventStoreAdapter].
func (s *EventStore) AppendToStream(ctx context.Context, streamID string, events []tarodomain.SerializedEvent, expected int64) (tarodomain.AppendResult, error) {
	var res tarodomain.AppendResult
	err := s.db.write(ctx, func(tb tables) error {
		ref := tarodomain.ParseStreamID(streamID)
		if ref.Kind != tarodomain.StreamKindAggregate {
			return appendFailed(streamID, fmt.Sprintf("%q is not an aggregate stream", streamID))
		}
		l := eventLogOf(tb)
		actual := l.streams[streamID].version
		if actual != expected {
			return tarodomain.OptimisticConcurrency(streamID, expected, actual)
		}
		seen := make(map[tarodomain.UUID]struct{}, len(events))
		for _, e := range events {
			if _, dup := l.byID[e.EventID]; dup {
				return appendFailed(streamID, fmt.Sprintf("event %s is already stored", e.EventID)).
					WithContext("eventId", e.EventID.String())
			}
			if _, dup := seen[e.EventID]; dup {
				return appendFailed(streamID, fmt.Sprintf("event %s appears twice", e.EventID)).
					WithContext("eventId", e.EventID.String())
			}
			seen[e.EventID] = struct{}{}
		}
		res = tarodomain.AppendResult{StreamID: streamID, PreviousVersion: expected, NewVersion: expected}
		if len(events) == 0 {
			return nil
		}
		now := s.db.Now()
		row, ok := l.streams[streamID]
		if !ok {
			row.createdAt = now
		}
		for _, e := range events {
			row.version++
			l.entries = append(l.entries, entry{
				pos: l.nextPos, streamID: streamID, category: ref.Category,
				version: row.version, event: copyEvent(e),
			})
			l.byID[e.EventID] = l.nextPos
			l.nextPos++
		}
		row.updatedAt = now
		l.streams[streamID] = row
		res.NewVersion = row.version
		return nil
	})
	return res, err
}

// scan returns the entries after the cursor that match, in append order.
// An event ID the log no longer holds (it was deleted) falls back to comparing
// IDs.
func (l *eventLog) scan(after tarodomain.UUID, max int, match func(entry) bool) []tarodomain.SerializedEvent {
	var afterPos int64
	byValue := false
	if !after.IsZero() {
		if p, ok := l.byID[after]; ok {
			afterPos = p
		} else {
			byValue = true
		}
	}
	var out []tarodomain.SerializedEvent
	for _, e := range l.entries {
		if e.pos <= afterPos || (byValue && bytes.Compare(e.event.EventID[:], after[:]) <= 0) {
			continue
		}
		if match != nil && !match(e) {
			continue
		}
		out = append(out, copyEvent(e.event))
		if max > 0 && len(out) == max {
			break
		}
	}
	return out
}

// LoadStream implements [tarodomain.EventStoreAdapter].
func (s *EventStore) LoadStream(ctx context.Context, streamID string, afterVersion int64, maxCount int) ([]tarodomain.SerializedEvent, error) {
	var out []tarodomain.SerializedEvent
	err := s.db.read(ctx, func(tb tables) error {
		for _, e := range eventLogOf(tb).entries {
			if e.streamID != streamID || e.version <= afterVersion {
				continue
			}
			out = append(out, copyEvent(e.event))
			if maxCount > 0 && len(out) == maxCount {
				break
			}
		}
		return nil
	})
	return out, err
}

// LoadStreamAfterEvent implements [tarodomain.EventStoreAdapter].
func (s *EventStore) LoadStreamAfterEvent(ctx context.Context, streamID string, afterEventID tarodomain.UUID, maxCount int) ([]tarodomain.SerializedEvent, error) {
	ref := tarodomain.ParseStreamID(streamID)
	var match func(entry) bool
	switch ref.Kind {
	case tarodomain.StreamKindAggregate:
		match = func(e entry) bool { return e.streamID == streamID }
	case tarodomain.StreamKindCategory:
		match = func(e entry) bool { return e.category == ref.Category }
	}
	var out []tarodomain.SerializedEvent
	err := s.db.read(ctx, func(tb tables) error {
		out = eventLogOf(tb).scan(afterEventID, maxCount, match)
		return nil
	})
	return out, err
}

// LoadAll implements [tarodomain.EventStoreAdapter].
func (s *EventStore) LoadAll(ctx context.Context, afterEventID tarodomain.UUID, maxCount int) ([]tarodomain.SerializedEvent, error) {
	return s.LoadStreamAfterEvent(ctx, tarodomain.StreamAll, afterEventID, maxCount)
}

// LoadByCorrelationID implements [tarodomain.EventStoreAdapter].
func (s *EventStore) LoadByCorrelationID(ctx context.Context, correlationID tarodomain.UUID) ([]tarodomain.SerializedEvent, error) {
	var out []tarodomain.SerializedEvent
	err := s.db.read(ctx, func(tb tables) error {
		out = eventLogOf(tb).scan(tarodomain.UUID{}, 0, func(e entry) bool {
			return e.event.CorrelationID == correlationID
		})
		return nil
	})
	return out, err
}

// StreamExists implements [tarodomain.EventStoreAdapter].
func (s *EventStore) StreamExists(ctx context.Context, streamID string) (bool, error) {
	v, err := s.StreamVersion(ctx, streamID)
	return v > 0, err
}

// StreamVersion implements [tarodomain.EventStoreAdapter].
func (s *EventStore) StreamVersion(ctx context.Context, streamID string) (int64, error) {
	var v int64
	err := s.db.read(ctx, func(tb tables) error {
		v = eventLogOf(tb).streams[streamID].version
		return nil
	})
	return v, err
}

// StreamMetadata implements [tarodomain.EventStoreAdapter].
func (s *EventStore) StreamMetadata(ctx context.Context, streamID string) (tarodomain.StreamMetadata, bool, error) {
	var md tarodomain.StreamMetadata
	var ok bool
	err := s.db.read(ctx, func(tb tables) error {
		var row streamRow
		row, ok = eventLogOf(tb).streams[streamID]
		if ok {
			md = tarodomain.StreamMetadata{StreamID: streamID, Version: row.version,
				CreatedAt: row.createdAt, UpdatedAt: row.updatedAt}
		}
		return nil
	})
	return md, ok, err
}

// DeleteStream implements [tarodomain.EventStoreAdapter].
func (s *EventStore) DeleteStream(ctx context.Context, streamID string) error {
	return s.db.write(ctx, func(tb tables) error {
		l := eventLogOf(tb)
		if _, ok := l.streams[streamID]; !ok {
			return nil
		}
		kept := make([]entry, 0, len(l.entries))
		for _, e := range l.entries {
			if e.streamID == streamID {
				delete(l.byID, e.event.EventID)
				continue
			}
			kept = append(kept, e)
		}
		l.entries = kept
		delete(l.streams, streamID)
		return nil
	})
}

// Subscribe implements [tarodomain.EventStoreAdapter].
func (s *EventStore) Subscribe(ctx context.Context, name, streamID string) (tarodomain.Subscription, error) {
	if streamID == "" {
		streamID = tarodomain.StreamAll
	}
	var out tarodomain.Subscription
	err := s.db.write(ctx, func(tb tables) error {
		l := eventLogOf(tb)
		if row, ok := l.subs[name]; ok {
			out = row.sub
			return nil
		}
		out = tarodomain.Subscription{Name: name, StreamID: streamID, CreatedAt: s.db.Now()}
		l.subs[name] = subRow{sub: out}
		return nil
	})
	return out, err
}

// GetSubscription implements [tarodomain.EventStoreAdapter].
func (s *EventStore) GetSubscription(ctx context.Context, name string) (tarodomain.Subscription, bool, error) {
	var out tarodomain.Subscription
	var ok bool
	err := s.db.read(ctx, func(tb tables) error {
		var row subRow
		row, ok = eventLogOf(tb).subs[name]
		out = row.sub
		return nil
	})
	return out, ok, err
}

// AckSubscription implements [tarodomain.EventStoreAdapter].
func (s *EventStore) AckSubscription(ctx context.Context, name string, lastSeen tarodomain.UUID) error {
	return s.db.write(ctx, func(tb tables) error {
		l := eventLogOf(tb)
		row, ok := l.subs[name]
		if !ok {
			return tarodomain.NewError(tarodomain.CodeEventStoreError,
				fmt.Sprintf("subscription %q does not exist", name)).WithContext("subscription", name)
		}
		pos, known := l.byID[lastSeen]
		switch {
		case lastSeen.IsZero():
			pos, known = 0, true
		case !known:
			// The event is gone: order by ID against the current cursor.
			if bytes.Compare(lastSeen[:], row.sub.LastSeen[:]) <= 0 {
				return nil
			}
			pos = row.lastPos
		}
		if known && pos < row.lastPos {
			return nil // never move back
		}
		row.sub.LastSeen = lastSeen
		row.lastPos = pos
		row.sub.FailedEventID = tarodomain.UUID{}
		row.sub.FailedAttempts = 0
		l.subs[name] = row
		return nil
	})
}

// RecordFailedAttempt implements [tarodomain.EventStoreAdapter].
func (s *EventStore) RecordFailedAttempt(ctx context.Context, name string, eventID tarodomain.UUID) (int, error) {
	var n int
	err := s.db.write(ctx, func(tb tables) error {
		l := eventLogOf(tb)
		row, ok := l.subs[name]
		if !ok {
			return tarodomain.NewError(tarodomain.CodeEventStoreError,
				fmt.Sprintf("subscription %q does not exist", name)).WithContext("subscription", name)
		}
		if row.sub.FailedEventID == eventID {
			row.sub.FailedAttempts++
		} else {
			row.sub.FailedEventID = eventID
			row.sub.FailedAttempts = 1
		}
		n = row.sub.FailedAttempts
		l.subs[name] = row
		return nil
	})
	return n, err
}

// DeleteSubscription implements [tarodomain.EventStoreAdapter].
func (s *EventStore) DeleteSubscription(ctx context.Context, name string) error {
	return s.db.write(ctx, func(tb tables) error {
		delete(eventLogOf(tb).subs, name)
		return nil
	})
}

// ListSubscriptions implements [tarodomain.EventStoreAdapter].
func (s *EventStore) ListSubscriptions(ctx context.Context) ([]tarodomain.Subscription, error) {
	var out []tarodomain.Subscription
	err := s.db.read(ctx, func(tb tables) error {
		for _, row := range eventLogOf(tb).subs {
			out = append(out, row.sub)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}
