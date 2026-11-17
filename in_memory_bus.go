package tarodomain

import (
	"context"
	"fmt"
	"sync"
)

// Delivery is the outcome of handing one event to one handler.
type Delivery struct {
	Handler string
	Event   DomainEvent
	// Err is nil if the handler succeeded.
	Err error
}

// InMemoryEventBus delivers events synchronously, in the caller's goroutine
// (§10.4). It is the strong-consistency bus.
type InMemoryEventBus struct {
	name     string
	mu       sync.RWMutex
	handlers []EventHandler
}

var _ SubscribableEventBus = (*InMemoryEventBus)(nil)

// NewInMemoryEventBus returns an empty bus.
func NewInMemoryEventBus(name string) *InMemoryEventBus {
	return &InMemoryEventBus{name: name}
}

// Name returns the bus's name.
func (b *InMemoryEventBus) Name() string { return b.name }

// Subscribe registers the handler after those already registered.
func (b *InMemoryEventBus) Subscribe(handler EventHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, h := range b.handlers {
		if h.Name() == handler.Name() {
			return NewError(CodeInvalidConfiguration,
				fmt.Sprintf("handler %q is already subscribed to bus %q", handler.Name(), b.name)).
				WithContext("handler", handler.Name()).WithContext("bus", b.name)
		}
	}
	b.handlers = append(b.handlers, handler)
	return nil
}

// Unsubscribe removes the handler with the name.
func (b *InMemoryEventBus) Unsubscribe(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, h := range b.handlers {
		if h.Name() == name {
			// A new slice, so a delivery in progress keeps its own list.
			b.handlers = append(append([]EventHandler(nil), b.handlers[:i]...), b.handlers[i+1:]...)
			return true
		}
	}
	return false
}

// snapshot returns the handlers now. The lock is not held while they run, so
// a handler may publish to this bus.
func (b *InMemoryEventBus) snapshot() []EventHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.handlers
}

// PublishAll gives each event to every handler, in subscription order, before
// the next event. It stops at the first failure and returns the handler's
// error unchanged.
func (b *InMemoryEventBus) PublishAll(ctx context.Context, events []DomainEvent) error {
	for _, e := range events {
		for _, h := range b.snapshot() {
			if err := h.Handle(ctx, e); err != nil {
				return err
			}
		}
	}
	return nil
}

// PublishAllWithResults delivers every event to every handler even if some
// fail, and returns one Delivery per handler and event, in delivery order.
func (b *InMemoryEventBus) PublishAllWithResults(ctx context.Context, events []DomainEvent) []Delivery {
	var out []Delivery
	for _, e := range events {
		for _, h := range b.snapshot() {
			out = append(out, Delivery{Handler: h.Name(), Event: e, Err: h.Handle(ctx, e)})
		}
	}
	return out
}
