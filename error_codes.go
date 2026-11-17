package tarodomain

import "fmt"

// ErrorCategory groups error codes by how a caller should react (§22.1).
type ErrorCategory string

// The four error categories.
const (
	// CategoryDomain is a business rule violation; retrying won't help.
	CategoryDomain ErrorCategory = "domain"
	// CategoryConcurrency is a transient conflict; a retry might succeed.
	CategoryConcurrency ErrorCategory = "concurrency"
	// CategoryInfrastructure is a transient failure; a retry might succeed.
	CategoryInfrastructure ErrorCategory = "infrastructure"
	// CategoryConfiguration is a framework misconfiguration; retrying won't help.
	CategoryConfiguration ErrorCategory = "configuration"
)

// Valid reports whether c is one of the four known categories.
func (c ErrorCategory) Valid() bool {
	switch c {
	case CategoryDomain, CategoryConcurrency, CategoryInfrastructure, CategoryConfiguration:
		return true
	}
	return false
}

// Retryable reports whether an error of this category is worth retrying.
// An unknown category is treated as infrastructure, as §22.1 does for foreign errors.
func (c ErrorCategory) Retryable() bool {
	switch c {
	case CategoryDomain, CategoryConfiguration:
		return false
	}
	return true
}

// HTTPStatus is the HTTP status an error of this category maps to.
func (c ErrorCategory) HTTPStatus() int {
	switch c {
	case CategoryDomain:
		return 400
	case CategoryConcurrency:
		return 409
	}
	return 500
}

// String returns the category's lower-case name.
func (c ErrorCategory) String() string { return string(c) }

// ErrorCode identifies one kind of error. The framework's 51 codes are the
// constants below; applications add their own with NewDomainError and its
// siblings.
type ErrorCode string

// Domain codes (§22.2).
const (
	CodeInvalidEventID                ErrorCode = "INVALID_EVENT_ID"
	CodeInvalidCorrelationID          ErrorCode = "INVALID_CORRELATION_ID"
	CodeInvalidCausationID            ErrorCode = "INVALID_CAUSATION_ID"
	CodeAggregateAlreadyExists        ErrorCode = "AGGREGATE_ALREADY_EXISTS"
	CodeAggregateNotFound             ErrorCode = "AGGREGATE_NOT_FOUND"
	CodeInvalidAggregateState         ErrorCode = "INVALID_AGGREGATE_STATE"
	CodeCommandValidation             ErrorCode = "COMMAND_VALIDATION"
	CodeInvalidCommandID              ErrorCode = "INVALID_COMMAND_ID"
	CodeInvalidCommandCorrelationID   ErrorCode = "INVALID_COMMAND_CORRELATION_ID"
	CodeInvalidCommandCausationID     ErrorCode = "INVALID_COMMAND_CAUSATION_ID"
	CodeSagaAlreadyCompleted          ErrorCode = "SAGA_ALREADY_COMPLETED"
	CodeSagaAlreadyStarted            ErrorCode = "SAGA_ALREADY_STARTED"
	CodeSagaNotStarted                ErrorCode = "SAGA_NOT_STARTED"
	CodeCompensationFailed            ErrorCode = "COMPENSATION_FAILED"
	CodeSagaTimeout                   ErrorCode = "SAGA_TIMEOUT"
	CodeInvalidSagaTransition         ErrorCode = "INVALID_SAGA_TRANSITION"
	CodeValueObjectInvalid            ErrorCode = "VALUE_OBJECT_INVALID"
	CodeIncomparableValue             ErrorCode = "INCOMPARABLE_VALUE"
	CodeValueObjectConstructionFailed ErrorCode = "VALUE_OBJECT_CONSTRUCTION_FAILED"
)

// Concurrency codes (§22.2).
const (
	CodeOptimisticConcurrency   ErrorCode = "OPTIMISTIC_CONCURRENCY"
	CodeCommandAlreadyProcessed ErrorCode = "COMMAND_ALREADY_PROCESSED"
	CodeConcurrentModification  ErrorCode = "CONCURRENT_MODIFICATION"
)

// Infrastructure codes (§22.2).
const (
	CodeEventStoreError                ErrorCode = "EVENT_STORE_ERROR"
	CodeEventAppendFailed              ErrorCode = "EVENT_APPEND_FAILED"
	CodeEventLoadFailed                ErrorCode = "EVENT_LOAD_FAILED"
	CodeStreamNotFound                 ErrorCode = "STREAM_NOT_FOUND"
	CodeSnapshotStoreError             ErrorCode = "SNAPSHOT_STORE_ERROR"
	CodeSnapshotLoadFailed             ErrorCode = "SNAPSHOT_LOAD_FAILED"
	CodeSnapshotSaveFailed             ErrorCode = "SNAPSHOT_SAVE_FAILED"
	CodeEventBusError                  ErrorCode = "EVENT_BUS_ERROR"
	CodeEventPublishFailed             ErrorCode = "EVENT_PUBLISH_FAILED"
	CodeEventHandlerFailed             ErrorCode = "EVENT_HANDLER_FAILED"
	CodeSagaStoreError                 ErrorCode = "SAGA_STORE_ERROR"
	CodeProjectionError                ErrorCode = "PROJECTION_ERROR"
	CodeProjectionHandlerFailed        ErrorCode = "PROJECTION_HANDLER_FAILED"
	CodeProjectionIdempotencyViolation ErrorCode = "PROJECTION_IDEMPOTENCY_VIOLATION"
	CodeQueueError                     ErrorCode = "QUEUE_ERROR"
	CodeSerializationError             ErrorCode = "SERIALIZATION_ERROR"
	CodeOperationTimeout               ErrorCode = "OPERATION_TIMEOUT"
	CodeDeadLetter                     ErrorCode = "DEAD_LETTER"
)

// Configuration codes (§22.2).
const (
	CodeMissingEventHandler         ErrorCode = "MISSING_EVENT_HANDLER"
	CodeUnknownEventType            ErrorCode = "UNKNOWN_EVENT_TYPE"
	CodeDuplicateEventRegistration  ErrorCode = "DUPLICATE_EVENT_REGISTRATION"
	CodeEventVersionMismatch        ErrorCode = "EVENT_VERSION_MISMATCH"
	CodeCommandHandlerNotRegistered ErrorCode = "COMMAND_HANDLER_NOT_REGISTERED"
	CodeInvalidConfiguration        ErrorCode = "INVALID_CONFIGURATION"
	CodeMissingDependency           ErrorCode = "MISSING_DEPENDENCY"
	CodeAggregateNotRegistered      ErrorCode = "AGGREGATE_NOT_REGISTERED"
	CodeProjectionNotRegistered     ErrorCode = "PROJECTION_NOT_REGISTERED"
	CodeSagaNotRegistered           ErrorCode = "SAGA_NOT_REGISTERED"
	CodeCircularDependency          ErrorCode = "CIRCULAR_DEPENDENCY"
)

var errorCodeCategories = map[ErrorCode]ErrorCategory{}

// allErrorCodes lists every framework code, grouped by category, in spec order.
var allErrorCodes = func() []ErrorCode {
	groups := []struct {
		cat   ErrorCategory
		codes []ErrorCode
	}{
		{CategoryDomain, []ErrorCode{
			CodeInvalidEventID, CodeInvalidCorrelationID, CodeInvalidCausationID,
			CodeAggregateAlreadyExists, CodeAggregateNotFound, CodeInvalidAggregateState,
			CodeCommandValidation, CodeInvalidCommandID, CodeInvalidCommandCorrelationID,
			CodeInvalidCommandCausationID, CodeSagaAlreadyCompleted, CodeSagaAlreadyStarted,
			CodeSagaNotStarted, CodeCompensationFailed, CodeSagaTimeout,
			CodeInvalidSagaTransition, CodeValueObjectInvalid, CodeIncomparableValue,
			CodeValueObjectConstructionFailed,
		}},
		{CategoryConcurrency, []ErrorCode{
			CodeOptimisticConcurrency, CodeCommandAlreadyProcessed, CodeConcurrentModification,
		}},
		{CategoryInfrastructure, []ErrorCode{
			CodeEventStoreError, CodeEventAppendFailed, CodeEventLoadFailed, CodeStreamNotFound,
			CodeSnapshotStoreError, CodeSnapshotLoadFailed, CodeSnapshotSaveFailed,
			CodeEventBusError, CodeEventPublishFailed, CodeEventHandlerFailed,
			CodeSagaStoreError, CodeProjectionError, CodeProjectionHandlerFailed,
			CodeProjectionIdempotencyViolation, CodeQueueError, CodeSerializationError,
			CodeOperationTimeout, CodeDeadLetter,
		}},
		{CategoryConfiguration, []ErrorCode{
			CodeMissingEventHandler, CodeUnknownEventType, CodeDuplicateEventRegistration,
			CodeEventVersionMismatch, CodeCommandHandlerNotRegistered, CodeInvalidConfiguration,
			CodeMissingDependency, CodeAggregateNotRegistered, CodeProjectionNotRegistered,
			CodeSagaNotRegistered, CodeCircularDependency,
		}},
	}
	var all []ErrorCode
	for _, g := range groups {
		for _, c := range g.codes {
			errorCodeCategories[c] = g.cat
			all = append(all, c)
		}
	}
	return all
}()

// ErrorCodes returns the 51 framework codes in the order of §22.2.
func ErrorCodes() []ErrorCode {
	return append([]ErrorCode(nil), allErrorCodes...)
}

// Category returns the category of a framework code. The second result is
// false for a code the framework doesn't define (an application's own code).
func (c ErrorCode) Category() (ErrorCategory, bool) {
	cat, ok := errorCodeCategories[c]
	return cat, ok
}

// String returns the code's text.
func (c ErrorCode) String() string { return string(c) }

// ParseErrorCode returns the framework code with the given text, or an error
// if there is none.
func ParseErrorCode(s string) (ErrorCode, error) {
	c := ErrorCode(s)
	if _, ok := errorCodeCategories[c]; !ok {
		return "", fmt.Errorf("tarodomain: unknown error code %q", s)
	}
	return c, nil
}
