package tarodomain

import (
	"context"
	"fmt"
)

// ConsistencyMode says whether a command's events reach their handlers inside
// the command (strong) or later, through a queue (eventual) (§11).
type ConsistencyMode string

const (
	// ConsistencyStrong delivers events in the command's unit of work.
	ConsistencyStrong ConsistencyMode = "strong"
	// ConsistencyEventual delivers events after the command, through a
	// transport such as a queue.
	ConsistencyEventual ConsistencyMode = "eventual"
)

// String returns "strong" or "eventual".
func (m ConsistencyMode) String() string { return string(m) }

// ParseConsistencyMode reads "strong" or "eventual". Anything else is
// INVALID_CONFIGURATION.
func ParseConsistencyMode(text string) (ConsistencyMode, error) {
	m := ConsistencyMode(text)
	switch m {
	case ConsistencyStrong, ConsistencyEventual:
		return m, nil
	}
	return "", NewError(CodeInvalidConfiguration, fmt.Sprintf("unknown consistency mode %q", text))
}

// MarshalText implements encoding.TextMarshaler, and so JSON as a string.
func (m ConsistencyMode) MarshalText() ([]byte, error) {
	if _, err := ParseConsistencyMode(string(m)); err != nil {
		return nil, err
	}
	return []byte(m), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *ConsistencyMode) UnmarshalText(text []byte) error {
	v, err := ParseConsistencyMode(string(text))
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// Transport names used in EventBusConfig.
const (
	// TransportInMemory is the in-process bus.
	TransportInMemory = "in_memory"
	// TransportQueue is a queue-backed bus.
	TransportQueue = "queue"
)

// EventBusConfig selects the bus a provider creates.
type EventBusConfig struct {
	Consistency ConsistencyMode
	// Transport is "in_memory", "queue" or a provider's own name. Empty means
	// the provider's default for the consistency mode.
	Transport string
}

// Provider is a platform: it creates the stores and the bus (§24.1).
type Provider interface {
	// ProviderType names the platform, such as "local" or "cloudflare".
	ProviderType() string

	CreateEventStoreAdapter() (EventStoreAdapter, error)
	CreateSnapshotStoreAdapter() (SnapshotStoreAdapter, error)
	CreateSagaStoreAdapter() (SagaStoreAdapter, error)
	CreateProcessedEventStore() (ProcessedEventStore, error)
	CreateCommandIdempotencyStore() (CommandIdempotencyStore, error)
	CreateDeadLetterStore() (DeadLetterStore, error)

	// CreateEventBus returns a bus for the config. An unknown transport is
	// INVALID_CONFIGURATION.
	CreateEventBus(config EventBusConfig) (EventBus, error)

	// CreateUnitOfWork returns a new, inactive unit of work each call, or nil
	// if the platform has no transactions.
	CreateUnitOfWork() UnitOfWork

	// Initialize prepares storage (migrations, tables). It is safe to call
	// again.
	Initialize(ctx context.Context) error
}

// ClockProvider is implemented by providers that bring their own clock.
type ClockProvider interface {
	Clock() Clock
}

// SleeperProvider is implemented by providers that bring their own sleeper.
type SleeperProvider interface {
	Sleeper() Sleeper
}

// ClockOf returns the provider's clock, or SystemClock if it has none.
func ClockOf(p Provider) Clock {
	if cp, ok := p.(ClockProvider); ok {
		if c := cp.Clock(); c != nil {
			return c
		}
	}
	return SystemClock{}
}

// SleeperOf returns the provider's sleeper, or TimerSleeper if it has none.
func SleeperOf(p Provider) Sleeper {
	if sp, ok := p.(SleeperProvider); ok {
		if s := sp.Sleeper(); s != nil {
			return s
		}
	}
	return TimerSleeper{}
}
