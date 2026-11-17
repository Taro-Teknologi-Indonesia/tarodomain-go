package tarodomain

import "context"

// UnitOfWork is the transaction boundary over the stores (§11.4). The strong
// pipeline begins it, and commits or rolls it back.
//
// A unit is not safe for use by several goroutines at once. Writes made with
// the ctx that ExecuteInUnit passes to the work belong to the unit: they are
// invisible to others until Commit and discarded by Rollback.
type UnitOfWork interface {
	// Begin starts the unit. Beginning an active unit is INVALID_CONFIGURATION.
	Begin(ctx context.Context) error
	// Commit applies every write of the unit atomically.
	Commit(ctx context.Context) error
	// Rollback discards every write. On an inactive unit it does nothing.
	Rollback(ctx context.Context) error
	// IsActive reports whether Begin ran and neither Commit nor Rollback has.
	IsActive() bool
}

type unitKey struct{}

// ActiveUnit returns the unit of work that ctx runs inside, if any. Adapters
// use it, with a type assertion to their own unit type, to join it.
func ActiveUnit(ctx context.Context) (UnitOfWork, bool) {
	u, ok := ctx.Value(unitKey{}).(UnitOfWork)
	if !ok || u == nil || !u.IsActive() {
		return nil, false
	}
	return u, true
}

// ExecuteInUnit runs work inside the unit: Begin, work, then Commit, or
// Rollback if the work fails or panics (a panic is raised again after the
// rollback). The ctx passed to work carries the unit.
//
// The work's error comes back unchanged. If the rollback also fails, an
// *Error from the work is returned as a copy with the rollback failure in its
// context under "rollbackError"; other errors are returned as they are.
//
// If ctx already runs inside an active unit, the work joins it: nothing is
// begun, committed or rolled back here, and the outer call decides.
func ExecuteInUnit(ctx context.Context, unit UnitOfWork, work func(ctx context.Context) error) (err error) {
	if _, ok := ActiveUnit(ctx); ok {
		return work(ctx)
	}
	if err := unit.Begin(ctx); err != nil {
		return err
	}
	inner := context.WithValue(ctx, unitKey{}, unit)

	finished := false
	defer func() {
		if finished {
			return
		}
		// The work panicked: undo, then let the panic go on.
		_ = unit.Rollback(ctx)
	}()

	workErr := work(inner)
	finished = true
	if workErr == nil {
		if cerr := unit.Commit(ctx); cerr != nil {
			if unit.IsActive() {
				_ = unit.Rollback(ctx)
			}
			return cerr
		}
		return nil
	}
	if rerr := unit.Rollback(ctx); rerr != nil {
		if fe, ok := workErr.(*Error); ok {
			return fe.WithContext("rollbackError", rerr.Error())
		}
	}
	return workErr
}
