package tarodomain

import "context"

// EventBus publishes domain events (§10.1). Repositories publish through it
// after an append.
type EventBus interface {
	// Name identifies the bus in logs and errors.
	Name() string
	// PublishAll delivers the events in order.
	PublishAll(ctx context.Context, events []DomainEvent) error
}

// EventHandler reacts to domain events (§10.2): projections and sagas
// implement it. Handle may be called more than once for the same event, so it
// must be idempotent.
type EventHandler interface {
	// Name identifies the handler. It is unique on one bus.
	Name() string
	// Handle processes one event.
	Handle(ctx context.Context, event DomainEvent) error
}

// SubscribableEventBus is a bus that delivers to handlers registered on it.
type SubscribableEventBus interface {
	EventBus
	// Subscribe registers the handler. A name already taken is
	// INVALID_CONFIGURATION.
	Subscribe(handler EventHandler) error
	// Unsubscribe removes the handler with the name, and reports whether there
	// was one.
	Unsubscribe(name string) bool
}
