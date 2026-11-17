package local

import (
	"context"
	"fmt"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// Provider is the in-memory [tarodomain.Provider]. Every adapter it creates
// shares one [Database].
type Provider struct {
	db *Database
}

// NewProvider returns a provider over a new database with the system clock.
func NewProvider() *Provider { return &Provider{db: NewDatabase(nil)} }

// NewProviderWithDatabase returns a provider over db, for example one with a
// manual clock, or one shared with a second provider.
func NewProviderWithDatabase(db *Database) *Provider { return &Provider{db: db} }

var (
	_ tarodomain.Provider      = (*Provider)(nil)
	_ tarodomain.ClockProvider = (*Provider)(nil)
)

// Database returns the shared database.
func (p *Provider) Database() *Database { return p.db }

// Clock implements [tarodomain.ClockProvider]: the database's clock.
func (p *Provider) Clock() tarodomain.Clock { return p.db.Clock() }

// ProviderType returns "local".
func (p *Provider) ProviderType() string { return "local" }

// CreateEventStoreAdapter implements [tarodomain.Provider].
func (p *Provider) CreateEventStoreAdapter() (tarodomain.EventStoreAdapter, error) {
	return NewEventStore(p.db), nil
}

// CreateSnapshotStoreAdapter implements [tarodomain.Provider].
func (p *Provider) CreateSnapshotStoreAdapter() (tarodomain.SnapshotStoreAdapter, error) {
	return NewSnapshotStore(p.db), nil
}

// CreateSagaStoreAdapter implements [tarodomain.Provider].
func (p *Provider) CreateSagaStoreAdapter() (tarodomain.SagaStoreAdapter, error) {
	return NewSagaStore(p.db), nil
}

// CreateProcessedEventStore implements [tarodomain.Provider].
func (p *Provider) CreateProcessedEventStore() (tarodomain.ProcessedEventStore, error) {
	return NewProcessedEvents(p.db), nil
}

// CreateCommandIdempotencyStore implements [tarodomain.Provider].
func (p *Provider) CreateCommandIdempotencyStore() (tarodomain.CommandIdempotencyStore, error) {
	return NewProcessedCommands(p.db), nil
}

// CreateDeadLetterStore implements [tarodomain.Provider].
func (p *Provider) CreateDeadLetterStore() (tarodomain.DeadLetterStore, error) {
	return NewDeadLetters(p.db), nil
}

// CreateEventBus returns a new in-process bus for either consistency mode. A
// transport other than "in_memory" (or empty) is INVALID_CONFIGURATION.
func (p *Provider) CreateEventBus(config tarodomain.EventBusConfig) (tarodomain.EventBus, error) {
	mode, err := tarodomain.ParseConsistencyMode(string(config.Consistency))
	if err != nil {
		return nil, err
	}
	if config.Transport != "" && config.Transport != tarodomain.TransportInMemory {
		return nil, tarodomain.NewError(tarodomain.CodeInvalidConfiguration,
			fmt.Sprintf("local provider has no %q transport", config.Transport)).
			WithContext("transport", config.Transport)
	}
	return tarodomain.NewInMemoryEventBus("local-" + mode.String()), nil
}

// CreateUnitOfWork returns a new inactive unit each call.
func (p *Provider) CreateUnitOfWork() tarodomain.UnitOfWork { return NewUnitOfWork(p.db) }

// Initialize does nothing: memory needs no setup.
func (p *Provider) Initialize(ctx context.Context) error { return ctx.Err() }
