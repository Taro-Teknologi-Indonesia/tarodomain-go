package local

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

func TestUnitCommitAndRollbackAcrossTables(t *testing.T) {
	db := NewDatabase(nil)
	ctx := context.Background()
	events, sagas := NewEventStore(db), NewSagaStore(db)
	sid := stream("Account")

	boom := errors.New("boom")
	err := tarodomain.ExecuteInUnit(ctx, NewUnitOfWork(db), func(u context.Context) error {
		if _, err := events.AppendToStream(u, sid, evs(1), 0); err != nil {
			return err
		}
		if err := sagas.Save(u, saga(1, "T", tarodomain.SagaRunning)); err != nil {
			return err
		}
		return boom
	})
	if err != boom {
		t.Fatalf("err = %v", err)
	}
	if v, _ := events.StreamVersion(ctx, sid); v != 0 {
		t.Fatal("event survived rollback")
	}
	if _, ok, _ := sagas.Load(ctx, "T", uid(1)); ok {
		t.Fatal("saga survived rollback")
	}

	err = tarodomain.ExecuteInUnit(ctx, NewUnitOfWork(db), func(u context.Context) error {
		if _, err := events.AppendToStream(u, sid, evs(1), 0); err != nil {
			return err
		}
		return sagas.Save(u, saga(1, "T", tarodomain.SagaRunning))
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := events.StreamVersion(ctx, sid); v != 1 {
		t.Fatal("event lost on commit")
	}
	if _, ok, _ := sagas.Load(ctx, "T", uid(1)); !ok {
		t.Fatal("saga lost on commit")
	}
}

func TestOthersSeeWritesOnlyAfterCommitAndWritesWait(t *testing.T) {
	db := NewDatabase(nil)
	ctx := context.Background()
	rs := Table[int](db, "n")
	inside, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- tarodomain.ExecuteInUnit(ctx, NewUnitOfWork(db), func(u context.Context) error {
			if err := rs.Put(u, "k", 1); err != nil {
				return err
			}
			close(inside)
			<-release
			return nil
		})
	}()
	<-inside
	if _, ok, _ := rs.Get(ctx, "k"); ok {
		t.Fatal("uncommitted write is visible")
	}
	written := make(chan error, 1)
	go func() { written <- rs.Put(ctx, "other", 2) }()
	select {
	case <-written:
		t.Fatal("a write outside did not wait for the unit")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := rs.Get(ctx, "k"); !ok || v != 1 {
		t.Fatalf("after commit %d %v", v, ok)
	}
	if v, ok, _ := rs.Get(ctx, "other"); !ok || v != 2 {
		t.Fatalf("waiting write lost: %d %v", v, ok)
	}
}

func TestUnitsRunOneAtATime(t *testing.T) {
	db := NewDatabase(nil)
	ctx := context.Background()
	rs := Table[int](db, "n")
	const n = 8
	errs := make(chan error, n)
	for range n {
		go func() {
			errs <- tarodomain.ExecuteInUnit(ctx, NewUnitOfWork(db), func(u context.Context) error {
				v, _, _ := rs.Get(u, "c")
				time.Sleep(time.Millisecond)
				return rs.Put(u, "c", v+1)
			})
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if v, _, _ := rs.Get(ctx, "c"); v != n {
		t.Fatalf("counter %d, want %d: units overlapped", v, n)
	}
}

func TestBeginWaitsAndGivesUp(t *testing.T) {
	db := NewDatabase(nil)
	first := NewUnitOfWork(db)
	if err := first.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second := NewUnitOfWork(db)
	if err := second.Begin(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if second.IsActive() {
		t.Fatal("failed Begin left the unit active")
	}
	_ = first.Rollback(context.Background())
	if err := second.Begin(context.Background()); err != nil {
		t.Fatalf("turn not released: %v", err)
	}
	_ = second.Rollback(context.Background())
}

func TestUnitStateChecks(t *testing.T) {
	ctx := context.Background()
	u := NewUnitOfWork(NewDatabase(nil))
	invalid := tarodomain.ErrCode(tarodomain.CodeInvalidConfiguration)
	if err := u.Commit(ctx); !errors.Is(err, invalid) {
		t.Fatalf("commit inactive: %v", err)
	}
	if err := u.Rollback(ctx); err != nil {
		t.Fatalf("rollback inactive: %v", err)
	}
	if u.IsActive() {
		t.Fatal("active")
	}
	if err := u.Begin(ctx); err != nil || !u.IsActive() {
		t.Fatalf("begin: %v", err)
	}
	if err := u.Begin(ctx); !errors.Is(err, invalid) {
		t.Fatalf("begin twice: %v", err)
	}
	if err := u.Commit(ctx); err != nil || u.IsActive() {
		t.Fatalf("commit: %v", err)
	}
	if err := u.Commit(ctx); !errors.Is(err, invalid) {
		t.Fatalf("commit twice: %v", err)
	}
	// A unit can be used again after it ends.
	if err := u.Begin(ctx); err != nil {
		t.Fatal(err)
	}
	_ = u.Rollback(ctx)
}

func TestNestedUnitJoins(t *testing.T) {
	db := NewDatabase(nil)
	ctx := context.Background()
	rs := Table[int](db, "n")
	err := tarodomain.ExecuteInUnit(ctx, NewUnitOfWork(db), func(u context.Context) error {
		_ = rs.Put(u, "k", 1)
		return tarodomain.ExecuteInUnit(u, NewUnitOfWork(db), func(u2 context.Context) error {
			_ = rs.Put(u2, "j", 2)
			return errors.New("inner fails")
		})
	})
	if err == nil {
		t.Fatal("expected the error")
	}
	if n, _ := rs.Len(ctx); n != 0 {
		t.Fatalf("outer unit was not rolled back: %d rows", n)
	}
}

func TestUnitOverAnotherDatabaseIsIgnored(t *testing.T) {
	ctx := context.Background()
	a, b := NewDatabase(nil), NewDatabase(nil)
	rb := Table[int](b, "n")
	err := tarodomain.ExecuteInUnit(ctx, NewUnitOfWork(a), func(u context.Context) error {
		// b is not part of a's unit: its write is immediate, and a's rollback keeps it.
		if err := rb.Put(u, "k", 1); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if v, ok, _ := rb.Get(ctx, "k"); !ok || v != 1 {
		t.Fatalf("write to the other database was lost: %d %v", v, ok)
	}
}
