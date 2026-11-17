package tarodomain

import "context"

// ExecutionContext carries the tracing identifiers of the work in progress
// (§5.5, §28). It travels in a context.Context, so each request has its own.
// Treat a value as read-only; derive a new one with ForCommand or ForEvent.
//
// A zero UUID means "none".
type ExecutionContext struct {
	// CorrelationID is shared by everything one original request causes.
	CorrelationID UUID
	// CausationID is the ID of the command or event that directly caused the
	// work now running.
	CausationID UUID
	// CommandCausationID is, for events raised by a command, that command's
	// own causation ID.
	CommandCausationID UUID
	// Metadata is copied onto every envelope made in this context.
	Metadata Metadata
}

// NewExecutionContext starts a root context: no causation, and the given
// correlation ID, or a new v7 ID if correlationID is zero.
func NewExecutionContext(correlationID UUID) ExecutionContext {
	if correlationID.IsZero() {
		correlationID = NewUUIDv7()
	}
	return ExecutionContext{CorrelationID: correlationID}
}

// ForCommand returns the context in which a command's handler runs: the same
// correlation, the command as the cause, and the command's own cause kept as
// CommandCausationID (§28.2).
func (ec ExecutionContext) ForCommand(c CommandEnvelope) ExecutionContext {
	return ExecutionContext{
		CorrelationID:      c.CorrelationID,
		CausationID:        c.ID,
		CommandCausationID: c.CausationID,
		Metadata:           c.Metadata,
	}
}

// ForEvent returns the context in which a handler of the event runs: the same
// correlation and the event as the cause. Commands it creates trace back to
// the event; CommandCausationID doesn't carry over.
func (ec ExecutionContext) ForEvent(e EventEnvelope) ExecutionContext {
	return ExecutionContext{
		CorrelationID: e.CorrelationID,
		CausationID:   e.ID,
		Metadata:      e.Metadata,
	}
}

type executionContextKey struct{}

// WithExecutionContext returns a context that carries ec. The parent is not
// changed.
func WithExecutionContext(ctx context.Context, ec ExecutionContext) context.Context {
	return context.WithValue(ctx, executionContextKey{}, ec)
}

// ExecutionContextFrom returns the execution context in ctx, if any.
func ExecutionContextFrom(ctx context.Context) (ExecutionContext, bool) {
	ec, ok := ctx.Value(executionContextKey{}).(ExecutionContext)
	return ec, ok
}
