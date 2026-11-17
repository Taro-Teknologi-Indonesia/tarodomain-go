package local

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

func TestEmptyDefault(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase(nil)
	rs := Table[int](db, "n")
	if n, err := rs.Len(ctx); err != nil || n != 0 {
		t.Fatalf("Len = %d, %v", n, err)
	}
	if _, ok, _ := rs.Get(ctx, "a"); ok {
		t.Fatal("empty table returned a row")
	}
	if _, ok := db.Clock().(tarodomain.SystemClock); !ok {
		t.Fatalf("default clock is %T", db.Clock())
	}
}

func TestSharedHandles(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase(nil)
	a, b := Table[string](db, "t"), Table[string](db, "t")
	if err := a.Put(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := b.Get(ctx, "k"); !ok || v != "v" {
		t.Fatalf("second handle got %q, %v", v, ok)
	}
	if err := b.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, "missing"); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.Len(ctx); n != 0 {
		t.Fatalf("Len = %d", n)
	}
	_ = a.Put(ctx, "b", "1")
	_ = a.Put(ctx, "a", "2")
	if keys, _ := a.Keys(ctx); !reflect.DeepEqual(keys, []string{"a", "b"}) {
		t.Fatalf("keys = %v", keys)
	}
}

func TestTableTypeMismatchPanics(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase(nil)
	_ = Table[int](db, "t").Put(ctx, "k", 1)
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	_, _, _ = Table[string](db, "t").Get(ctx, "k")
}

func TestClock(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.FixedZone("x", 3600))
	clock := tarodomain.NewManualClock(at)
	db := NewDatabase(clock)
	if got := db.Now(); !got.Equal(at.UTC().Truncate(time.Millisecond)) || got.Location() != time.UTC {
		t.Fatalf("Now = %v", got)
	}
	clock.Advance(time.Hour)
	if got := db.Now(); got.Hour() != 3 {
		t.Fatalf("after advance: %v", got)
	}
	db.SetClock(nil)
	if _, ok := db.Clock().(tarodomain.SystemClock); !ok {
		t.Fatalf("SetClock(nil) gave %T", db.Clock())
	}
}

func TestWriteWaitsForTurn(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase(nil)
	if err := db.acquireTurn(ctx); err != nil {
		t.Fatal(err)
	}
	rs := Table[int](db, "n")
	done := make(chan error, 1)
	go func() { done <- rs.Put(ctx, "k", 1) }()
	select {
	case err := <-done:
		t.Fatalf("write did not wait: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	// Reads do not need the turn.
	if _, _, err := rs.Get(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	db.releaseTurn()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := rs.Get(ctx, "k"); !ok || v != 1 {
		t.Fatalf("got %d, %v", v, ok)
	}
}

func TestWriteGivesUpWhenContextEnds(t *testing.T) {
	db := NewDatabase(nil)
	if err := db.acquireTurn(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer db.releaseTurn()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := Table[int](db, "n").Put(ctx, "k", 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	cancelled, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, _, err := Table[int](db, "n").Get(cancelled, "k"); !errors.Is(err, context.Canceled) {
		t.Fatalf("read err = %v", err)
	}
}

func TestCopyIsDeep(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase(nil)
	rs := Table[string](db, "t")
	_ = rs.Put(ctx, "k", "old")

	cp := db.copyTables()
	tableOf[string](cp, "t").m["k"] = "changed"
	tableOf[string](cp, "t").m["new"] = "x"
	if v, _, _ := rs.Get(ctx, "k"); v != "old" {
		t.Fatalf("original changed to %q", v)
	}
	if n, _ := rs.Len(ctx); n != 1 {
		t.Fatalf("original has %d rows", n)
	}

	// Inside a unit's copy, reads and writes use the copy, not the original.
	u := withTx(ctx, &tx{db: db, tables: cp})
	if v, _, _ := rs.Get(u, "k"); v != "changed" {
		t.Fatalf("unit read %q", v)
	}
	if err := rs.Put(u, "k", "in-unit"); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := rs.Get(ctx, "k"); v != "old" {
		t.Fatalf("outside saw %q", v)
	}
	db.swapTables(cp)
	if v, _, _ := rs.Get(ctx, "k"); v != "in-unit" {
		t.Fatalf("after swap %q", v)
	}

	// A unit made for another database is ignored.
	other := NewDatabase(nil)
	u2 := withTx(ctx, &tx{db: other, tables: tables{}})
	if v, ok, _ := rs.Get(u2, "k"); !ok || v != "in-unit" {
		t.Fatalf("foreign unit hid the row: %q, %v", v, ok)
	}
}
