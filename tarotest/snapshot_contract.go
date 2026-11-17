package tarotest

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// Cut-offs for time-based calls. The suites never compare with fixed times of
// the store's clock: they use these, or timestamps the store returned.
var (
	farPast   = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	farFuture = time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)
)

// noErr fails the case if err is not nil.
func noErr(t T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// sameJSON reports whether two documents are equal as JSON.
func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xa) == string(yb)
}

// SnapshotContract checks a provider's [tarodomain.SnapshotStoreAdapter].
func SnapshotContract(t T, newProvider Factory) {
	t.Helper()
	each := func(name string, f func(t T, ctx context.Context, s tarodomain.SnapshotStoreAdapter)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			s, err := fresh(ctx, t, newProvider).CreateSnapshotStoreAdapter()
			if err != nil {
				t.Fatalf("CreateSnapshotStoreAdapter: %v", err)
			}
			f(t, ctx, s)
		})
	}
	input := func(id tarodomain.AggregateID, typ string, version int64, data string) tarodomain.SnapshotInput {
		md, _ := tarodomain.NewMetadata(map[string]any{"k": "v"})
		return tarodomain.SnapshotInput{SourceID: id, SourceType: typ, SourceVersion: version,
			Data: json.RawMessage(data), Metadata: md}
	}

	each("a saved snapshot comes back with its fields", func(t T, ctx context.Context, s tarodomain.SnapshotStoreAdapter) {
		id := tarodomain.NewAggregateID()
		in := input(id, "Account", 5, `{"balance":50}`)
		noErr(t, s.Save(ctx, in))
		got, ok, err := s.LoadLatest(ctx, id, "Account")
		noErr(t, err)
		if !ok {
			t.Fatalf("not found")
		}
		if got.SourceID != id || got.SourceType != "Account" || got.SourceVersion != 5 ||
			!sameJSON(got.Data, in.Data) || !got.Metadata.Equal(in.Metadata) || got.CreatedAt.IsZero() {
			t.Errorf("got %+v", got)
		}
		if _, ok, _ := s.LoadLatest(ctx, tarodomain.NewAggregateID(), "Account"); ok {
			t.Errorf("found a snapshot of another aggregate")
		}
	})

	each("save is an upsert that keeps CreatedAt", func(t T, ctx context.Context, s tarodomain.SnapshotStoreAdapter) {
		id := tarodomain.NewAggregateID()
		noErr(t, s.Save(ctx, input(id, "Account", 5, `{"v":1}`)))
		first, _, _ := s.LoadAtVersion(ctx, id, "Account", 5)
		noErr(t, s.Save(ctx, input(id, "Account", 5, `{"v":2}`)))
		got, ok, err := s.LoadAtVersion(ctx, id, "Account", 5)
		noErr(t, err)
		if !ok || !sameJSON(got.Data, []byte(`{"v":2}`)) {
			t.Errorf("not replaced: %+v", got)
		}
		if !got.CreatedAt.Equal(first.CreatedAt) {
			t.Errorf("CreatedAt changed from %v to %v", first.CreatedAt, got.CreatedAt)
		}
		if n, _ := s.Count(ctx, id, "Account"); n != 1 {
			t.Errorf("count = %d", n)
		}
	})

	each("latest is the highest version, whatever the save order", func(t T, ctx context.Context, s tarodomain.SnapshotStoreAdapter) {
		id := tarodomain.NewAggregateID()
		for _, v := range []int64{10, 5, 20, 15} {
			noErr(t, s.Save(ctx, input(id, "Account", v, `{}`)))
		}
		got, ok, _ := s.LoadLatest(ctx, id, "Account")
		if !ok || got.SourceVersion != 20 {
			t.Errorf("latest = %+v", got)
		}
		at, ok, _ := s.LoadAtVersion(ctx, id, "Account", 10)
		if !ok || at.SourceVersion != 10 {
			t.Errorf("at 10 = %+v", at)
		}
		if _, ok, _ := s.LoadAtVersion(ctx, id, "Account", 11); ok {
			t.Errorf("found a version never saved")
		}
	})

	each("snapshots are kept per aggregate and type", func(t T, ctx context.Context, s tarodomain.SnapshotStoreAdapter) {
		id, other := tarodomain.NewAggregateID(), tarodomain.NewAggregateID()
		noErr(t, s.Save(ctx, input(id, "Account", 5, `{}`)))
		noErr(t, s.Save(ctx, input(id, "Order", 9, `{}`)))
		noErr(t, s.Save(ctx, input(other, "Account", 7, `{}`)))
		if got, _, _ := s.LoadLatest(ctx, id, "Account"); got.SourceVersion != 5 {
			t.Errorf("Account latest = %d", got.SourceVersion)
		}
		if got, _, _ := s.LoadLatest(ctx, id, "Order"); got.SourceVersion != 9 {
			t.Errorf("Order latest = %d", got.SourceVersion)
		}
		noErr(t, s.DeleteAll(ctx, id, "Account"))
		if n, _ := s.Count(ctx, id, "Order"); n != 1 {
			t.Errorf("DeleteAll removed another type's snapshot")
		}
		if n, _ := s.Count(ctx, other, "Account"); n != 1 {
			t.Errorf("DeleteAll removed another aggregate's snapshot")
		}
	})

	each("list is newest first and honours the limit", func(t T, ctx context.Context, s tarodomain.SnapshotStoreAdapter) {
		id := tarodomain.NewAggregateID()
		for _, v := range []int64{5, 15, 10} {
			noErr(t, s.Save(ctx, input(id, "Account", v, `{}`)))
		}
		list, err := s.List(ctx, id, "Account", tarodomain.SnapshotListOptions{})
		noErr(t, err)
		if len(list) != 3 || list[0].SourceVersion != 15 || list[1].SourceVersion != 10 || list[2].SourceVersion != 5 {
			t.Errorf("list = %+v", list)
		}
		list, _ = s.List(ctx, id, "Account", tarodomain.SnapshotListOptions{Limit: 2})
		if len(list) != 2 || list[0].SourceVersion != 15 {
			t.Errorf("limited list = %+v", list)
		}
		if list, _ = s.List(ctx, tarodomain.NewAggregateID(), "Account", tarodomain.SnapshotListOptions{}); len(list) != 0 {
			t.Errorf("list of another aggregate = %+v", list)
		}
	})

	each("deleting", func(t T, ctx context.Context, s tarodomain.SnapshotStoreAdapter) {
		id := tarodomain.NewAggregateID()
		for _, v := range []int64{5, 10, 15} {
			noErr(t, s.Save(ctx, input(id, "Account", v, `{}`)))
		}
		n, err := s.DeleteBefore(ctx, id, "Account", 10)
		noErr(t, err)
		if n != 1 {
			t.Errorf("DeleteBefore removed %d", n)
		}
		if _, ok, _ := s.LoadAtVersion(ctx, id, "Account", 10); !ok {
			t.Errorf("DeleteBefore removed the cut-off")
		}
		if n, _ = s.DeleteBefore(ctx, id, "Account", 10); n != 0 {
			t.Errorf("second DeleteBefore removed %d", n)
		}
		if n, _ = s.Count(ctx, id, "Account"); n != 2 {
			t.Errorf("count = %d", n)
		}
		noErr(t, s.DeleteAll(ctx, id, "Account"))
		noErr(t, s.DeleteAll(ctx, id, "Account"))
		noErr(t, s.DeleteAll(ctx, tarodomain.NewAggregateID(), "Account"))
		if n, _ = s.Count(ctx, id, "Account"); n != 0 {
			t.Errorf("count after DeleteAll = %d", n)
		}
		if _, ok, _ := s.LoadLatest(ctx, id, "Account"); ok {
			t.Errorf("snapshot survived DeleteAll")
		}
	})
}
