package tarodomain

import (
	"context"
	"encoding/json"
	"time"
)

// SnapshotInput is a snapshot to store (§8.2). The key is
// (SourceID, SourceVersion).
type SnapshotInput struct {
	// SourceID is the aggregate the snapshot belongs to.
	SourceID AggregateID
	// SourceVersion is the aggregate version the snapshot was taken at.
	SourceVersion int64
	// SourceType is the aggregate type.
	SourceType string
	// Data is the JSON state from Snapshotter.
	Data json.RawMessage
	// Metadata is optional.
	Metadata Metadata
}

// Snapshot is a stored snapshot: its input and when it was written.
type Snapshot struct {
	SnapshotInput
	// CreatedAt is when the snapshot was first stored. A save over the same
	// key keeps it.
	CreatedAt time.Time
}

// SnapshotListOptions limits a List call.
type SnapshotListOptions struct {
	// Limit is the most snapshots to return; 0 or less means no limit.
	Limit int
}

// SnapshotStoreAdapter is the storage contract for snapshots (§8.1).
//
//   - Save is an upsert on (SourceID, SourceVersion).
//   - Latest is the snapshot with the highest version.
//   - List returns snapshots newest first, by version.
//   - Deleting what doesn't exist is a no-op.
type SnapshotStoreAdapter interface {
	// Save stores the snapshot, replacing one with the same key.
	Save(ctx context.Context, in SnapshotInput) error
	// LoadLatest returns the snapshot with the highest version, and false if
	// there is none.
	LoadLatest(ctx context.Context, sourceID AggregateID, sourceType string) (Snapshot, bool, error)
	// LoadAtVersion returns the snapshot taken at exactly version.
	LoadAtVersion(ctx context.Context, sourceID AggregateID, sourceType string, version int64) (Snapshot, bool, error)
	// List returns the aggregate's snapshots, newest first.
	List(ctx context.Context, sourceID AggregateID, sourceType string, opts SnapshotListOptions) ([]Snapshot, error)
	// DeleteAll removes every snapshot of the aggregate.
	DeleteAll(ctx context.Context, sourceID AggregateID, sourceType string) error
	// DeleteBefore removes snapshots with a version below version (the
	// cut-off itself stays) and returns how many it removed.
	DeleteBefore(ctx context.Context, sourceID AggregateID, sourceType string, version int64) (int, error)
	// Count returns how many snapshots the aggregate has.
	Count(ctx context.Context, sourceID AggregateID, sourceType string) (int, error)
}
