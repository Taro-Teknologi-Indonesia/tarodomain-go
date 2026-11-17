package tarodomain

import (
	"errors"
	"log/slog"
)

// ErrorJSON is the JSON form of an *Error (§22.3). The cause is not part of it.
type ErrorJSON struct {
	Code       ErrorCode      `json:"code"`
	Category   ErrorCategory  `json:"category"`
	Message    string         `json:"message"`
	Retryable  bool           `json:"retryable"`
	HTTPStatus int            `json:"httpStatus"`
	Context    map[string]any `json:"context"`
}

// ToJSON returns the error's JSON form. Context is never nil, so it encodes as {}.
func (e *Error) ToJSON() ErrorJSON {
	ctx := make(map[string]any, len(e.Context))
	for k, v := range e.Context {
		ctx[k] = v
	}
	return ErrorJSON{
		Code:       e.Code,
		Category:   e.Category,
		Message:    e.Message,
		Retryable:  e.Retryable(),
		HTTPStatus: e.HTTPStatus(),
		Context:    ctx,
	}
}

// HTTPErrorResponse is an HTTP status and a JSON-encodable body.
type HTTPErrorResponse struct {
	Status int
	Body   map[string]any
}

// ToHTTPResponse maps the error to an HTTP response. body["error"] holds code,
// category, message and retryable, plus context for client errors (4xx) only,
// so server-side details never reach the caller. The cause is never included.
func (e *Error) ToHTTPResponse() HTTPErrorResponse {
	status := e.HTTPStatus()
	body := map[string]any{
		"code":      string(e.Code),
		"category":  string(e.Category),
		"message":   e.Message,
		"retryable": e.Retryable(),
	}
	if status >= 400 && status < 500 {
		ctx := make(map[string]any, len(e.Context))
		for k, v := range e.Context {
			ctx[k] = v
		}
		body["context"] = ctx
	}
	return HTTPErrorResponse{Status: status, Body: map[string]any{"error": body}}
}

// LogEntry is the structured log form of an error.
type LogEntry struct {
	// Level is Warn for domain errors (rejections, not faults) and Error otherwise.
	Level    slog.Level
	Code     ErrorCode
	Category ErrorCategory
	Message  string
	Context  map[string]any
	// Causes holds the message of each error in the cause chain, outermost first.
	Causes []string
}

// ToLogEntry builds the error's log entry.
func (e *Error) ToLogEntry() LogEntry {
	level := slog.LevelError
	if e.Category == CategoryDomain {
		level = slog.LevelWarn
	}
	var causes []string
	for c := errors.Unwrap(e); c != nil; c = errors.Unwrap(c) {
		causes = append(causes, c.Error())
	}
	return LogEntry{
		Level:    level,
		Code:     e.Code,
		Category: e.Category,
		Message:  e.Message,
		Context:  e.Context,
		Causes:   causes,
	}
}

// LogValue implements slog.LogValuer.
func (l LogEntry) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String("code", string(l.Code)),
		slog.String("category", string(l.Category)),
		slog.String("message", l.Message),
	}
	if len(l.Context) > 0 {
		ctx := make([]any, 0, len(l.Context)*2)
		for k, v := range l.Context {
			ctx = append(ctx, k, v)
		}
		attrs = append(attrs, slog.Group("context", ctx...))
	}
	if len(l.Causes) > 0 {
		attrs = append(attrs, slog.Any("causes", l.Causes))
	}
	return slog.GroupValue(attrs...)
}

// LogValue implements slog.LogValuer, so logger.Error("failed", "err", err)
// records the structured form.
func (e *Error) LogValue() slog.Value { return e.ToLogEntry().LogValue() }
