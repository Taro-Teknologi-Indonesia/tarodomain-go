package tarodomain

import (
	"context"
	"errors"
	"fmt"
	"maps"
)

// Error is the one error type of the framework (§22). It carries a code, the
// code's category, a message, optional context and an optional cause.
//
// An *Error is never changed after it is built: WithContext and WithCause
// return copies, so a shared error value is safe to reuse.
type Error struct {
	// Code identifies the kind of error.
	Code ErrorCode
	// Category tells a caller whether to retry and which HTTP status applies.
	Category ErrorCategory
	// Message is the human-readable description.
	Message string
	// Context holds details such as stream IDs. Treat it as read-only.
	Context map[string]any

	cause error
}

// NewError builds an error for a framework code, taking its category from the
// code. A code the framework doesn't define is treated as infrastructure.
func NewError(code ErrorCode, message string) *Error {
	cat, ok := code.Category()
	if !ok {
		cat = CategoryInfrastructure
	}
	return &Error{Code: code, Category: cat, Message: message}
}

// NewDomainError builds an application error with its own code in the domain
// category (§22.3), for example "INSUFFICIENT_FUNDS".
func NewDomainError(code, message string) *Error {
	return &Error{Code: ErrorCode(code), Category: CategoryDomain, Message: message}
}

// NewConcurrencyError builds an application error in the concurrency category.
func NewConcurrencyError(code, message string) *Error {
	return &Error{Code: ErrorCode(code), Category: CategoryConcurrency, Message: message}
}

// NewInfrastructureError builds an application error in the infrastructure category.
func NewInfrastructureError(code, message string) *Error {
	return &Error{Code: ErrorCode(code), Category: CategoryInfrastructure, Message: message}
}

// NewConfigurationError builds an application error in the configuration category.
func NewConfigurationError(code, message string) *Error {
	return &Error{Code: ErrorCode(code), Category: CategoryConfiguration, Message: message}
}

// Error implements the error interface as "CODE: message".
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the cause, if any.
func (e *Error) Unwrap() error { return e.cause }

// Is makes errors.Is(err, ErrCode(code)) match on the code.
func (e *Error) Is(target error) bool {
	if m, ok := target.(codeMatcher); ok {
		return e.Code == m.code
	}
	return false
}

// Retryable reports whether retrying might succeed, from the category.
func (e *Error) Retryable() bool { return e.Category.Retryable() }

// HTTPStatus is the HTTP status for the error's category.
func (e *Error) HTTPStatus() int { return e.Category.HTTPStatus() }

// WithContext returns a copy with one more context entry.
func (e *Error) WithContext(key string, value any) *Error {
	c := *e
	c.Context = make(map[string]any, len(e.Context)+1)
	maps.Copy(c.Context, e.Context)
	c.Context[key] = value
	return &c
}

// WithCause returns a copy whose cause is err.
func (e *Error) WithCause(err error) *Error {
	c := *e
	c.cause = err
	return &c
}

type codeMatcher struct{ code ErrorCode }

func (m codeMatcher) Error() string { return "error code " + string(m.code) }

// ErrCode returns a target for errors.Is that matches any *Error with the code.
func ErrCode(code ErrorCode) error { return codeMatcher{code} }

// AsError finds the first *Error in err's chain.
func AsError(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// CodeOf returns the code of the first *Error in err's chain.
func CodeOf(err error) (ErrorCode, bool) {
	if e, ok := AsError(err); ok {
		return e.Code, true
	}
	return "", false
}

// Infrastructure wraps a foreign error (a driver or network failure) as an
// infrastructure *Error with code INFRASTRUCTURE_ERROR, keeping it as the
// cause. An error that already contains an *Error is returned unchanged, and
// nil stays nil.
func Infrastructure(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsError(err); ok {
		return err
	}
	return NewInfrastructureError("INFRASTRUCTURE_ERROR", err.Error()).WithCause(err)
}

// ShouldRetry reports whether an operation that failed with err should be
// retried (§22.1). Cancellation and deadline errors are never retried, even
// inside an *Error. An *Error answers with its category; any other error is
// taken as infrastructure and retried.
func ShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if e, ok := AsError(err); ok {
		return e.Retryable()
	}
	return true
}

// OptimisticConcurrency reports that a stream's version isn't the expected one.
func OptimisticConcurrency(streamID string, expected, actual int64) *Error {
	return NewError(CodeOptimisticConcurrency,
		fmt.Sprintf("stream %s is at version %d, expected %d", streamID, actual, expected)).
		WithContext("streamId", streamID).
		WithContext("expectedVersion", expected).
		WithContext("actualVersion", actual)
}

// AggregateNotFound reports that no aggregate of the type has the ID.
func AggregateNotFound(aggregateType, id string) *Error {
	return NewError(CodeAggregateNotFound,
		fmt.Sprintf("%s %s not found", aggregateType, id)).
		WithContext("aggregateType", aggregateType).
		WithContext("aggregateId", id)
}
