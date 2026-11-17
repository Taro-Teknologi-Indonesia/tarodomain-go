package tarodomain

import (
	"fmt"
	"reflect"
)

// DefaultEventVersion is the version of an event that doesn't declare one.
const DefaultEventVersion = "1.0.0"

// Event is a business fact (§2.1): a plain struct of business fields. The
// envelope (ID, correlation, time) is kept apart in EventEnvelope.
//
// EventName is the stored type name. Declare it explicitly, so renaming the Go
// type never orphans stored events. Implement it on a value receiver.
type Event interface {
	EventName() string
}

// VersionedEvent is an Event that declares a schema version (semver text).
type VersionedEvent interface {
	Event
	EventVersion() string
}

// EventVersionOf returns the event's declared version, or "1.0.0".
func EventVersionOf(e Event) string {
	if v, ok := e.(VersionedEvent); ok {
		return v.EventVersion()
	}
	return DefaultEventVersion
}

// DomainEvent is an event with its envelope: what buses, stores and handlers
// pass around. It is a value, so a handler can't change another handler's
// copy. Slices and maps inside the payload are shared; treat them as
// read-only.
type DomainEvent struct {
	envelope EventEnvelope
	name     string
	version  string
	payload  Event
}

// NewDomainEvent pairs a payload with an envelope. The payload must be a
// non-nil value, not a pointer: INVALID_CONFIGURATION otherwise.
func NewDomainEvent(envelope EventEnvelope, payload Event) (DomainEvent, error) {
	if payload == nil {
		return DomainEvent{}, NewError(CodeInvalidConfiguration, "event payload is nil")
	}
	if t := reflect.TypeOf(payload); t.Kind() == reflect.Pointer {
		return DomainEvent{}, NewError(CodeInvalidConfiguration,
			fmt.Sprintf("event type %s must be used by value, not as a pointer", t)).
			WithContext("type", t.String())
	}
	return DomainEvent{
		envelope: envelope,
		name:     payload.EventName(),
		version:  EventVersionOf(payload),
		payload:  payload,
	}, nil
}

// Envelope returns the event's envelope.
func (d DomainEvent) Envelope() EventEnvelope { return d.envelope }

// Name returns the stored event type name.
func (d DomainEvent) Name() string { return d.name }

// Version returns the event's schema version.
func (d DomainEvent) Version() string { return d.version }

// Payload returns the business event.
func (d DomainEvent) Payload() Event { return d.payload }

// Recorded is the typed view of a DomainEvent: the envelope and the payload
// as its Go type.
type Recorded[E Event] struct {
	Envelope EventEnvelope
	Event    E
}

// EventAs returns the typed view if the event's payload is an E.
func EventAs[E Event](d DomainEvent) (Recorded[E], bool) {
	e, ok := d.payload.(E)
	if !ok {
		return Recorded[E]{}, false
	}
	return Recorded[E]{Envelope: d.envelope, Event: e}, true
}

// EventIs reports whether the event's payload is an E.
func EventIs[E Event](d DomainEvent) bool {
	_, ok := d.payload.(E)
	return ok
}
