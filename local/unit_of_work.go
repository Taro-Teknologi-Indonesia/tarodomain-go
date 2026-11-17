package local

import (
	"context"
	"sync"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// UnitOfWork is the in-memory [tarodomain.UnitOfWork]. Begin takes the
// database's turn, so units run one at a time, and copies the tables. Reads
// and writes whose ctx carries the unit (see [tarodomain.ExecuteInUnit]) use
// the copy: the unit sees its own writes and other goroutines don't. Commit
// swaps the copy in; Rollback throws it away.
type UnitOfWork struct {
	db *Database

	mu     sync.Mutex
	active bool
	tables tables
}

// NewUnitOfWork returns an inactive unit over db.
func NewUnitOfWork(db *Database) *UnitOfWork { return &UnitOfWork{db: db} }

var _ tarodomain.UnitOfWork = (*UnitOfWork)(nil)

// Begin implements [tarodomain.UnitOfWork]. It waits for the database's turn,
// and gives up if ctx ends first.
func (u *UnitOfWork) Begin(ctx context.Context) error {
	u.mu.Lock()
	active := u.active
	u.mu.Unlock()
	if active {
		return tarodomain.NewError(tarodomain.CodeInvalidConfiguration, "unit of work is already active")
	}
	if err := u.db.acquireTurn(ctx); err != nil {
		return err
	}
	cp := u.db.copyTables()
	u.mu.Lock()
	u.active, u.tables = true, cp
	u.mu.Unlock()
	return nil
}

// Commit implements [tarodomain.UnitOfWork].
func (u *UnitOfWork) Commit(context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.active {
		return tarodomain.NewError(tarodomain.CodeInvalidConfiguration, "unit of work is not active")
	}
	u.db.swapTables(u.tables)
	u.finish()
	return nil
}

// Rollback implements [tarodomain.UnitOfWork].
func (u *UnitOfWork) Rollback(context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.active {
		u.finish()
	}
	return nil
}

// finish ends the unit and gives the turn back. The caller holds u.mu.
func (u *UnitOfWork) finish() {
	u.active, u.tables = false, nil
	u.db.releaseTurn()
}

// IsActive implements [tarodomain.UnitOfWork].
func (u *UnitOfWork) IsActive() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.active
}
