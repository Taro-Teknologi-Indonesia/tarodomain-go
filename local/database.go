package local

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// tableData is one table's rows, able to copy itself.
type tableData interface {
	clone() tableData
}

// rows is a table of values by string key. Values are copied in and out by
// assignment, so a row type must not share mutable memory with its caller.
type rows[T any] struct {
	m map[string]T
}

func (r *rows[T]) clone() tableData {
	c := &rows[T]{m: make(map[string]T, len(r.m))}
	for k, v := range r.m {
		c.m[k] = v
	}
	return c
}

// tables maps a table name to its rows.
type tables map[string]tableData

func (t tables) clone() tables {
	c := make(tables, len(t))
	for name, d := range t {
		c[name] = d.clone()
	}
	return c
}

// tableOf returns the named table of tb, creating it when missing. A name
// reused with another row type is a programming error and panics.
func tableOf[T any](tb tables, name string) *rows[T] {
	if d, ok := tb[name]; ok {
		r, ok := d.(*rows[T])
		if !ok {
			panic(fmt.Sprintf("local: table %q holds %T, not %T", name, d, &rows[T]{}))
		}
		return r
	}
	r := &rows[T]{m: map[string]T{}}
	tb[name] = r
	return r
}

// tx is the private copy of the tables that a unit of work reads and writes.
// Step 4.4 creates one per unit and puts it in the ctx.
type tx struct {
	db     *Database
	tables tables
}

type txKey struct{}

func withTx(ctx context.Context, t *tx) context.Context {
	return context.WithValue(ctx, txKey{}, t)
}

// txFor returns the unit's copy if ctx carries one made for db.
func (db *Database) txFor(ctx context.Context) *tx {
	if t, ok := ctx.Value(txKey{}).(*tx); ok && t != nil && t.db == db {
		return t
	}
	return nil
}

// Database holds the tables shared by every adapter of one provider.
//
// The mutex is held only inside the synchronous closures given to read and
// write, never across a call that can block. Writes also take the database's
// turn, a one-slot channel (so waiting respects ctx), which a unit of work
// holds for its whole life.
type Database struct {
	mu     sync.Mutex
	tables tables
	turn   chan struct{}

	clockMu sync.RWMutex
	clock   tarodomain.Clock
}

// NewDatabase returns an empty database. A nil clock means the system clock.
func NewDatabase(clock tarodomain.Clock) *Database {
	if clock == nil {
		clock = tarodomain.SystemClock{}
	}
	return &Database{
		tables: tables{},
		turn:   make(chan struct{}, 1),
		clock:  clock,
	}
}

// Clock returns the clock that stamps stored records.
func (db *Database) Clock() tarodomain.Clock {
	db.clockMu.RLock()
	defer db.clockMu.RUnlock()
	return db.clock
}

// SetClock replaces the clock. A nil clock means the system clock.
func (db *Database) SetClock(clock tarodomain.Clock) {
	if clock == nil {
		clock = tarodomain.SystemClock{}
	}
	db.clockMu.Lock()
	db.clock = clock
	db.clockMu.Unlock()
}

// Now returns the database's time in UTC, cut to milliseconds.
func (db *Database) Now() time.Time {
	return tarodomain.NormalizeTime(db.Clock().Now())
}

// acquireTurn waits for the database's turn, or for ctx to end.
func (db *Database) acquireTurn(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case db.turn <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (db *Database) releaseTurn() { <-db.turn }

// read runs fn over the tables the ctx sees: the unit's copy inside a unit,
// else the shared tables. fn must not block and must not change them.
func (db *Database) read(ctx context.Context, fn func(tb tables) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t := db.txFor(ctx); t != nil {
		return fn(t.tables)
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	return fn(db.tables)
}

// write runs fn over the tables the ctx sees, and may change them. Inside a
// unit the unit already holds the turn; outside, write waits for it. If fn
// returns an error, the changes it made so far stay: fn must check before it
// changes anything.
func (db *Database) write(ctx context.Context, fn func(tb tables) error) error {
	if t := db.txFor(ctx); t != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fn(t.tables)
	}
	if err := db.acquireTurn(ctx); err != nil {
		return err
	}
	defer db.releaseTurn()
	db.mu.Lock()
	defer db.mu.Unlock()
	return fn(db.tables)
}

// copyTables returns a deep copy of the shared tables. The caller must hold
// the turn, so no write runs meanwhile.
func (db *Database) copyTables() tables {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.tables.clone()
}

// swapTables replaces the shared tables, which is how a unit commits. The
// caller must hold the turn.
func (db *Database) swapTables(tb tables) {
	db.mu.Lock()
	db.tables = tb
	db.mu.Unlock()
}

// Rows is a handle to one named table of values of type T, so an application
// can keep in-memory read models in the provider's database and have them
// roll back with a unit of work.
type Rows[T any] struct {
	db   *Database
	name string
}

// Table returns the handle for the named table of db. Using one name with two
// different row types panics, as that is a programming error. Rows are copied
// by assignment: keep T free of shared mutable memory.
func Table[T any](db *Database, name string) *Rows[T] {
	return &Rows[T]{db: db, name: name}
}

// Get returns the row stored under key.
func (r *Rows[T]) Get(ctx context.Context, key string) (T, bool, error) {
	var out T
	var found bool
	err := r.db.read(ctx, func(tb tables) error {
		out, found = tableOf[T](tb, r.name).m[key]
		return nil
	})
	return out, found, err
}

// Put stores value under key, replacing any row there.
func (r *Rows[T]) Put(ctx context.Context, key string, value T) error {
	return r.db.write(ctx, func(tb tables) error {
		tableOf[T](tb, r.name).m[key] = value
		return nil
	})
}

// Delete removes the row under key. Deleting a missing row does nothing.
func (r *Rows[T]) Delete(ctx context.Context, key string) error {
	return r.db.write(ctx, func(tb tables) error {
		delete(tableOf[T](tb, r.name).m, key)
		return nil
	})
}

// Len returns the number of rows.
func (r *Rows[T]) Len(ctx context.Context) (int, error) {
	var n int
	err := r.db.read(ctx, func(tb tables) error {
		n = len(tableOf[T](tb, r.name).m)
		return nil
	})
	return n, err
}

// Keys returns the keys in ascending order.
func (r *Rows[T]) Keys(ctx context.Context) ([]string, error) {
	var keys []string
	err := r.db.read(ctx, func(tb tables) error {
		m := tableOf[T](tb, r.name).m
		keys = make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		return nil
	})
	sort.Strings(keys)
	return keys, err
}
