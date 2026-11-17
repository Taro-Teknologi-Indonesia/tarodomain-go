package local

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

const snapshotsTable = "snapshots"

func cloneRaw(b json.RawMessage) json.RawMessage {
	if b == nil {
		return nil
	}
	return append(json.RawMessage{}, b...)
}

// SnapshotStore is the in-memory [tarodomain.SnapshotStoreAdapter].
type SnapshotStore struct{ db *Database }

// NewSnapshotStore returns a snapshot store over db.
func NewSnapshotStore(db *Database) *SnapshotStore { return &SnapshotStore{db: db} }

var _ tarodomain.SnapshotStoreAdapter = (*SnapshotStore)(nil)

func snapshotKey(typ string, id tarodomain.AggregateID, version int64) string {
	return fmt.Sprintf("%s\x00%s\x00%020d", typ, id, version)
}

// of returns the aggregate's snapshots, newest version first.
func (s *SnapshotStore) of(tb tables, id tarodomain.AggregateID, typ string) []tarodomain.Snapshot {
	var out []tarodomain.Snapshot
	for _, sn := range tableOf[tarodomain.Snapshot](tb, snapshotsTable).m {
		if sn.SourceID == id && sn.SourceType == typ {
			sn.Data = cloneRaw(sn.Data)
			out = append(out, sn)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceVersion > out[j].SourceVersion })
	return out
}

// Save implements [tarodomain.SnapshotStoreAdapter].
func (s *SnapshotStore) Save(ctx context.Context, in tarodomain.SnapshotInput) error {
	return s.db.write(ctx, func(tb tables) error {
		m := tableOf[tarodomain.Snapshot](tb, snapshotsTable).m
		key := snapshotKey(in.SourceType, in.SourceID, in.SourceVersion)
		in.Data = cloneRaw(in.Data)
		created := s.db.Now()
		if old, ok := m[key]; ok {
			created = old.CreatedAt
		}
		m[key] = tarodomain.Snapshot{SnapshotInput: in, CreatedAt: created}
		return nil
	})
}

// LoadLatest implements [tarodomain.SnapshotStoreAdapter].
func (s *SnapshotStore) LoadLatest(ctx context.Context, id tarodomain.AggregateID, typ string) (tarodomain.Snapshot, bool, error) {
	var out tarodomain.Snapshot
	var ok bool
	err := s.db.read(ctx, func(tb tables) error {
		if all := s.of(tb, id, typ); len(all) > 0 {
			out, ok = all[0], true
		}
		return nil
	})
	return out, ok, err
}

// LoadAtVersion implements [tarodomain.SnapshotStoreAdapter].
func (s *SnapshotStore) LoadAtVersion(ctx context.Context, id tarodomain.AggregateID, typ string, version int64) (tarodomain.Snapshot, bool, error) {
	var out tarodomain.Snapshot
	var ok bool
	err := s.db.read(ctx, func(tb tables) error {
		out, ok = tableOf[tarodomain.Snapshot](tb, snapshotsTable).m[snapshotKey(typ, id, version)]
		out.Data = cloneRaw(out.Data)
		return nil
	})
	return out, ok, err
}

// List implements [tarodomain.SnapshotStoreAdapter].
func (s *SnapshotStore) List(ctx context.Context, id tarodomain.AggregateID, typ string, opts tarodomain.SnapshotListOptions) ([]tarodomain.Snapshot, error) {
	var out []tarodomain.Snapshot
	err := s.db.read(ctx, func(tb tables) error {
		out = s.of(tb, id, typ)
		if opts.Limit > 0 && len(out) > opts.Limit {
			out = out[:opts.Limit]
		}
		return nil
	})
	return out, err
}

// DeleteAll implements [tarodomain.SnapshotStoreAdapter].
func (s *SnapshotStore) DeleteAll(ctx context.Context, id tarodomain.AggregateID, typ string) error {
	_, err := s.deleteWhere(ctx, id, typ, func(int64) bool { return true })
	return err
}

// DeleteBefore implements [tarodomain.SnapshotStoreAdapter].
func (s *SnapshotStore) DeleteBefore(ctx context.Context, id tarodomain.AggregateID, typ string, version int64) (int, error) {
	return s.deleteWhere(ctx, id, typ, func(v int64) bool { return v < version })
}

func (s *SnapshotStore) deleteWhere(ctx context.Context, id tarodomain.AggregateID, typ string, del func(int64) bool) (int, error) {
	n := 0
	err := s.db.write(ctx, func(tb tables) error {
		m := tableOf[tarodomain.Snapshot](tb, snapshotsTable).m
		for k, sn := range m {
			if sn.SourceID == id && sn.SourceType == typ && del(sn.SourceVersion) {
				delete(m, k)
				n++
			}
		}
		return nil
	})
	return n, err
}

// Count implements [tarodomain.SnapshotStoreAdapter].
func (s *SnapshotStore) Count(ctx context.Context, id tarodomain.AggregateID, typ string) (int, error) {
	n := 0
	err := s.db.read(ctx, func(tb tables) error {
		n = len(s.of(tb, id, typ))
		return nil
	})
	return n, err
}
