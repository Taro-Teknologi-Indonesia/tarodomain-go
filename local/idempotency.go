package local

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

const (
	processedEventsTable   = "processed_events"
	processedCommandsTable = "processed_commands"
)

type processedEvent struct {
	handler, instance string
	at                time.Time
}

// ProcessedEvents is the in-memory [tarodomain.ProcessedEventStore].
type ProcessedEvents struct{ db *Database }

// NewProcessedEvents returns a processed-event store over db.
func NewProcessedEvents(db *Database) *ProcessedEvents { return &ProcessedEvents{db: db} }

var _ tarodomain.ProcessedEventStore = (*ProcessedEvents)(nil)

func pevKey(handler, instance string, id tarodomain.UUID) string {
	return handler + "\x00" + instance + "\x00" + id.String()
}

// Exists implements [tarodomain.ProcessedEventStore].
func (p *ProcessedEvents) Exists(ctx context.Context, handler, instance string, id tarodomain.UUID) (bool, error) {
	var ok bool
	err := p.db.read(ctx, func(tb tables) error {
		_, ok = tableOf[processedEvent](tb, processedEventsTable).m[pevKey(handler, instance, id)]
		return nil
	})
	return ok, err
}

// MarkProcessed implements [tarodomain.ProcessedEventStore].
func (p *ProcessedEvents) MarkProcessed(ctx context.Context, handler, instance string, id tarodomain.UUID) error {
	return p.db.write(ctx, func(tb tables) error {
		m := tableOf[processedEvent](tb, processedEventsTable).m
		key := pevKey(handler, instance, id)
		if _, ok := m[key]; !ok {
			m[key] = processedEvent{handler: handler, instance: instance, at: p.db.Now()}
		}
		return nil
	})
}

func (p *ProcessedEvents) removeWhere(ctx context.Context, del func(processedEvent) bool, remove bool) (int, error) {
	n := 0
	err := p.db.write(ctx, func(tb tables) error {
		m := tableOf[processedEvent](tb, processedEventsTable).m
		for k, v := range m {
			if del(v) {
				n++
				if remove {
					delete(m, k)
				}
			}
		}
		return nil
	})
	return n, err
}

// Count implements [tarodomain.ProcessedEventStore].
func (p *ProcessedEvents) Count(ctx context.Context, handler, instance string) (int, error) {
	n := 0
	err := p.db.read(ctx, func(tb tables) error {
		for _, v := range tableOf[processedEvent](tb, processedEventsTable).m {
			if v.handler == handler && (instance == "" || v.instance == instance) {
				n++
			}
		}
		return nil
	})
	return n, err
}

// DeleteByHandler implements [tarodomain.ProcessedEventStore].
func (p *ProcessedEvents) DeleteByHandler(ctx context.Context, handler, instance string) (int, error) {
	return p.removeWhere(ctx, func(v processedEvent) bool {
		return v.handler == handler && (instance == "" || v.instance == instance)
	}, true)
}

// DeleteOld implements [tarodomain.ProcessedEventStore].
func (p *ProcessedEvents) DeleteOld(ctx context.Context, olderThan time.Time) (int, error) {
	return p.removeWhere(ctx, func(v processedEvent) bool { return v.at.Before(olderThan) }, true)
}

// ProcessedCommands is the in-memory [tarodomain.CommandIdempotencyStore].
type ProcessedCommands struct{ db *Database }

// NewProcessedCommands returns a command idempotency store over db.
func NewProcessedCommands(db *Database) *ProcessedCommands { return &ProcessedCommands{db: db} }

var _ tarodomain.CommandIdempotencyStore = (*ProcessedCommands)(nil)

// Exists implements [tarodomain.CommandIdempotencyStore].
func (p *ProcessedCommands) Exists(ctx context.Context, id tarodomain.UUID) (bool, error) {
	_, ok, err := p.ProcessedAt(ctx, id)
	return ok, err
}

// MarkProcessed implements [tarodomain.CommandIdempotencyStore].
func (p *ProcessedCommands) MarkProcessed(ctx context.Context, id tarodomain.UUID, commandType, handler string) error {
	return p.db.write(ctx, func(tb tables) error {
		m := tableOf[tarodomain.ProcessedCommand](tb, processedCommandsTable).m
		key := id.String()
		if _, ok := m[key]; !ok {
			m[key] = tarodomain.ProcessedCommand{CommandID: id, CommandType: commandType,
				HandlerName: handler, ProcessedAt: p.db.Now()}
		}
		return nil
	})
}

// ProcessedAt implements [tarodomain.CommandIdempotencyStore].
func (p *ProcessedCommands) ProcessedAt(ctx context.Context, id tarodomain.UUID) (time.Time, bool, error) {
	var rec tarodomain.ProcessedCommand
	var ok bool
	err := p.db.read(ctx, func(tb tables) error {
		rec, ok = tableOf[tarodomain.ProcessedCommand](tb, processedCommandsTable).m[id.String()]
		return nil
	})
	return rec.ProcessedAt, ok, err
}

// List implements [tarodomain.CommandIdempotencyStore].
func (p *ProcessedCommands) List(ctx context.Context, handler string, limit int) ([]tarodomain.ProcessedCommand, error) {
	var out []tarodomain.ProcessedCommand
	err := p.db.read(ctx, func(tb tables) error {
		for _, r := range tableOf[tarodomain.ProcessedCommand](tb, processedCommandsTable).m {
			if handler == "" || r.HandlerName == handler {
				out = append(out, r)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ProcessedAt.Equal(out[j].ProcessedAt) {
			return out[i].ProcessedAt.Before(out[j].ProcessedAt)
		}
		return bytes.Compare(out[i].CommandID[:], out[j].CommandID[:]) < 0
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

// Count implements [tarodomain.CommandIdempotencyStore].
func (p *ProcessedCommands) Count(ctx context.Context, handler string) (int, error) {
	out, err := p.List(ctx, handler, 0)
	return len(out), err
}

// DeleteOld implements [tarodomain.CommandIdempotencyStore].
func (p *ProcessedCommands) DeleteOld(ctx context.Context, olderThan time.Time) (int, error) {
	n := 0
	err := p.db.write(ctx, func(tb tables) error {
		m := tableOf[tarodomain.ProcessedCommand](tb, processedCommandsTable).m
		for k, r := range m {
			if r.ProcessedAt.Before(olderThan) {
				delete(m, k)
				n++
			}
		}
		return nil
	})
	return n, err
}
