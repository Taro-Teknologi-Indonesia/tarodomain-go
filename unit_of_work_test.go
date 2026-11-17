package tarodomain

import (
	"context"
	"errors"
	"testing"
)

type fakeUnit struct {
	active                           bool
	begins, commits, rollbacks       int
	beginErr, commitErr, rollbackErr error
}

func (u *fakeUnit) Begin(context.Context) error {
	if u.beginErr != nil {
		return u.beginErr
	}
	u.begins++
	u.active = true
	return nil
}
func (u *fakeUnit) Commit(context.Context) error {
	u.commits++
	if u.commitErr != nil {
		return u.commitErr
	}
	u.active = false
	return nil
}
func (u *fakeUnit) Rollback(context.Context) error {
	u.rollbacks++
	u.active = false
	return u.rollbackErr
}
func (u *fakeUnit) IsActive() bool { return u.active }

func TestExecuteInUnitCommit(t *testing.T) {
	u := &fakeUnit{}
	var saw UnitOfWork
	err := ExecuteInUnit(context.Background(), u, func(ctx context.Context) error {
		saw, _ = ActiveUnit(ctx)
		return nil
	})
	if err != nil || u.begins != 1 || u.commits != 1 || u.rollbacks != 0 {
		t.Fatalf("err=%v %+v", err, u)
	}
	if saw != UnitOfWork(u) {
		t.Error("work did not see the unit")
	}
	if _, ok := ActiveUnit(context.Background()); ok {
		t.Error("unit leaked into the parent ctx")
	}
}

func TestExecuteInUnitRollbackKeepsError(t *testing.T) {
	u := &fakeUnit{}
	want := errors.New("boom")
	err := ExecuteInUnit(context.Background(), u, func(context.Context) error { return want })
	if err != want || u.commits != 0 || u.rollbacks != 1 {
		t.Fatalf("err=%v %+v", err, u)
	}
}

func TestExecuteInUnitRollbackFailure(t *testing.T) {
	u := &fakeUnit{rollbackErr: errors.New("rb")}
	werr := NewDomainError("X", "bad")
	err := ExecuteInUnit(context.Background(), u, func(context.Context) error { return werr })
	fe, ok := err.(*Error)
	if !ok || fe.Code != "X" || fe.Context["rollbackError"] != "rb" {
		t.Fatalf("err = %#v", err)
	}
	if _, has := werr.Context["rollbackError"]; has {
		t.Error("original error was modified")
	}

	plain := errors.New("plain")
	if err := ExecuteInUnit(context.Background(), u, func(context.Context) error { return plain }); err != plain {
		t.Errorf("foreign error changed: %v", err)
	}
}

func TestExecuteInUnitBeginCommitFailures(t *testing.T) {
	begin := errors.New("begin")
	u := &fakeUnit{beginErr: begin}
	ran := false
	err := ExecuteInUnit(context.Background(), u, func(context.Context) error { ran = true; return nil })
	if err != begin || ran || u.rollbacks != 0 {
		t.Errorf("begin failure: err=%v ran=%v %+v", err, ran, u)
	}

	commit := errors.New("commit")
	u = &fakeUnit{commitErr: commit}
	err = ExecuteInUnit(context.Background(), u, func(context.Context) error { return nil })
	if err != commit || u.rollbacks != 1 || u.active {
		t.Errorf("commit failure: err=%v %+v", err, u)
	}
}

func TestExecuteInUnitPanicRollsBack(t *testing.T) {
	u := &fakeUnit{}
	defer func() {
		if r := recover(); r != "oops" {
			t.Fatalf("recovered %v", r)
		}
		if u.rollbacks != 1 || u.commits != 0 || u.active {
			t.Errorf("%+v", u)
		}
	}()
	_ = ExecuteInUnit(context.Background(), u, func(context.Context) error { panic("oops") })
}

func TestExecuteInUnitNestedJoins(t *testing.T) {
	outer, inner := &fakeUnit{}, &fakeUnit{}
	inner2 := errors.New("inner failed")
	err := ExecuteInUnit(context.Background(), outer, func(ctx context.Context) error {
		return ExecuteInUnit(ctx, inner, func(context.Context) error { return inner2 })
	})
	if err != inner2 {
		t.Fatalf("err = %v", err)
	}
	if inner.begins != 0 || inner.commits != 0 || inner.rollbacks != 0 {
		t.Errorf("nested unit was used: %+v", inner)
	}
	if outer.begins != 1 || outer.rollbacks != 1 || outer.commits != 0 {
		t.Errorf("outer: %+v", outer)
	}
}
