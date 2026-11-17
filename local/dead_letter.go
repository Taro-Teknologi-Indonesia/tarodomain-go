package local

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

const (
	deadLettersTable = "dead_letters"
	deadLetterSeq    = "dead_letter_seq" // table holding the last ID written
	deadLetterSeqKey = "last"
)

// DeadLetters is the in-memory [tarodomain.DeadLetterStore].
type DeadLetters struct{ db *Database }

// NewDeadLetters returns a dead letter store over db.
func NewDeadLetters(db *Database) *DeadLetters { return &DeadLetters{db: db} }

var _ tarodomain.DeadLetterStore = (*DeadLetters)(nil)

func dlOut(e tarodomain.DeadLetterEntry) tarodomain.DeadLetterEntry {
	e.EventData = cloneRaw(e.EventData)
	return e
}

// Write implements [tarodomain.DeadLetterStore].
func (d *DeadLetters) Write(ctx context.Context, in tarodomain.DeadLetterInput) (int64, error) {
	var id int64
	err := d.db.write(ctx, func(tb tables) error {
		seq := tableOf[int64](tb, deadLetterSeq).m
		id = seq[deadLetterSeqKey] + 1 // never reused, even after a delete
		seq[deadLetterSeqKey] = id
		in.EventData = cloneRaw(in.EventData)
		tableOf[tarodomain.DeadLetterEntry](tb, deadLettersTable).m[strconv.FormatInt(id, 10)] =
			tarodomain.DeadLetterEntry{DeadLetterInput: in, ID: id, CreatedAt: d.db.Now()}
		return nil
	})
	return id, err
}

func (d *DeadLetters) all(ctx context.Context, match func(tarodomain.DeadLetterEntry) bool) ([]tarodomain.DeadLetterEntry, error) {
	var out []tarodomain.DeadLetterEntry
	err := d.db.read(ctx, func(tb tables) error {
		for _, e := range tableOf[tarodomain.DeadLetterEntry](tb, deadLettersTable).m {
			if match(e) {
				out = append(out, dlOut(e))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, err
}

// List implements [tarodomain.DeadLetterStore].
func (d *DeadLetters) List(ctx context.Context, handler string, limit int) ([]tarodomain.DeadLetterEntry, error) {
	out, err := d.all(ctx, func(e tarodomain.DeadLetterEntry) bool { return handler == "" || e.HandlerName == handler })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

// Get implements [tarodomain.DeadLetterStore].
func (d *DeadLetters) Get(ctx context.Context, eventID tarodomain.UUID, handler string) (tarodomain.DeadLetterEntry, bool, error) {
	out, err := d.all(ctx, func(e tarodomain.DeadLetterEntry) bool {
		return e.EventID == eventID && e.HandlerName == handler
	})
	if err != nil || len(out) == 0 {
		return tarodomain.DeadLetterEntry{}, false, err
	}
	return out[len(out)-1], true, nil
}

// GetByID implements [tarodomain.DeadLetterStore].
func (d *DeadLetters) GetByID(ctx context.Context, id int64) (tarodomain.DeadLetterEntry, bool, error) {
	var e tarodomain.DeadLetterEntry
	var ok bool
	err := d.db.read(ctx, func(tb tables) error {
		e, ok = tableOf[tarodomain.DeadLetterEntry](tb, deadLettersTable).m[strconv.FormatInt(id, 10)]
		e = dlOut(e)
		return nil
	})
	return e, ok, err
}

// Delete implements [tarodomain.DeadLetterStore].
func (d *DeadLetters) Delete(ctx context.Context, id int64) error {
	return d.db.write(ctx, func(tb tables) error {
		delete(tableOf[tarodomain.DeadLetterEntry](tb, deadLettersTable).m, strconv.FormatInt(id, 10))
		return nil
	})
}

// Count implements [tarodomain.DeadLetterStore].
func (d *DeadLetters) Count(ctx context.Context, handler string) (int, error) {
	out, err := d.List(ctx, handler, 0)
	return len(out), err
}

// DeleteOld implements [tarodomain.DeadLetterStore].
func (d *DeadLetters) DeleteOld(ctx context.Context, olderThan time.Time) (int, error) {
	n := 0
	err := d.db.write(ctx, func(tb tables) error {
		m := tableOf[tarodomain.DeadLetterEntry](tb, deadLettersTable).m
		for k, e := range m {
			if e.CreatedAt.Before(olderThan) {
				delete(m, k)
				n++
			}
		}
		return nil
	})
	return n, err
}
