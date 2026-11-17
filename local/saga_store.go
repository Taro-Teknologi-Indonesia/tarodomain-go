package local

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

const sagasTable = "sagas"

// SagaStore is the in-memory [tarodomain.SagaStoreAdapter].
type SagaStore struct{ db *Database }

// NewSagaStore returns a saga store over db.
func NewSagaStore(db *Database) *SagaStore { return &SagaStore{db: db} }

var _ tarodomain.SagaStoreAdapter = (*SagaStore)(nil)

func sagaKey(typ string, id tarodomain.UUID) string { return typ + "\x00" + id.String() }

// Save implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) Save(ctx context.Context, in tarodomain.SagaSaveInput) error {
	return s.db.write(ctx, func(tb tables) error {
		m := tableOf[tarodomain.SagaRecord](tb, sagasTable).m
		key := sagaKey(in.SagaType, in.SagaID)
		in.State = cloneRaw(in.State)
		m[key] = m[key].Saved(in, s.db.Now())
		return nil
	})
}

func outRecord(r tarodomain.SagaRecord) tarodomain.SagaRecord {
	r.State = cloneRaw(r.State)
	return r
}

// Load implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) Load(ctx context.Context, typ string, id tarodomain.UUID) (tarodomain.SagaRecord, bool, error) {
	var r tarodomain.SagaRecord
	var ok bool
	err := s.db.read(ctx, func(tb tables) error {
		r, ok = tableOf[tarodomain.SagaRecord](tb, sagasTable).m[sagaKey(typ, id)]
		r = outRecord(r)
		return nil
	})
	return r, ok, err
}

// LoadByCorrelationID implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) LoadByCorrelationID(ctx context.Context, typ string, correlationID tarodomain.UUID) (tarodomain.SagaRecord, bool, error) {
	return s.Load(ctx, typ, correlationID)
}

func (s *SagaStore) end(ctx context.Context, typ string, id tarodomain.UUID, f func(tarodomain.SagaRecord, time.Time) (tarodomain.SagaRecord, bool)) error {
	return s.db.write(ctx, func(tb tables) error {
		m := tableOf[tarodomain.SagaRecord](tb, sagasTable).m
		key := sagaKey(typ, id)
		if r, ok := m[key]; ok {
			if next, changed := f(r, s.db.Now()); changed {
				m[key] = next
			}
		}
		return nil
	})
}

// Complete implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) Complete(ctx context.Context, typ string, id tarodomain.UUID) error {
	return s.end(ctx, typ, id, tarodomain.SagaRecord.Complete)
}

// Fail implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) Fail(ctx context.Context, typ string, id tarodomain.UUID, reason string) error {
	return s.end(ctx, typ, id, func(r tarodomain.SagaRecord, now time.Time) (tarodomain.SagaRecord, bool) {
		return r.Fail(reason, now)
	})
}

// list returns the matching records ordered by UpdatedAt, type, ID, with the
// limit applied after the filter.
func (s *SagaStore) list(ctx context.Context, q tarodomain.SagaQuery, match func(tarodomain.SagaRecord) bool) ([]tarodomain.SagaRecord, error) {
	var out []tarodomain.SagaRecord
	err := s.db.read(ctx, func(tb tables) error {
		for _, r := range tableOf[tarodomain.SagaRecord](tb, sagasTable).m {
			if (q.SagaType == "" || r.SagaType == q.SagaType) && match(r) {
				out = append(out, outRecord(r))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.Before(b.UpdatedAt)
		}
		if a.SagaType != b.SagaType {
			return a.SagaType < b.SagaType
		}
		return bytes.Compare(a.SagaID[:], b.SagaID[:]) < 0
	})
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, err
}

// LoadByStatus implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) LoadByStatus(ctx context.Context, status tarodomain.SagaStatus, q tarodomain.SagaQuery) ([]tarodomain.SagaRecord, error) {
	return s.list(ctx, q, func(r tarodomain.SagaRecord) bool { return r.Status == status })
}

// LoadTimedOut implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) LoadTimedOut(ctx context.Context, before time.Time, q tarodomain.SagaQuery) ([]tarodomain.SagaRecord, error) {
	return s.list(ctx, q, func(r tarodomain.SagaRecord) bool { return r.IsTimedOut(before) })
}

// Delete implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) Delete(ctx context.Context, typ string, id tarodomain.UUID) error {
	return s.db.write(ctx, func(tb tables) error {
		delete(tableOf[tarodomain.SagaRecord](tb, sagasTable).m, sagaKey(typ, id))
		return nil
	})
}

// Count implements [tarodomain.SagaStoreAdapter].
func (s *SagaStore) Count(ctx context.Context, typ string, status tarodomain.SagaStatus) (int, error) {
	out, err := s.list(ctx, tarodomain.SagaQuery{SagaType: typ}, func(r tarodomain.SagaRecord) bool {
		return status == "" || r.Status == status
	})
	return len(out), err
}
