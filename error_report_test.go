package tarodomain

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestToJSON(t *testing.T) {
	e := OptimisticConcurrency("s", 1, 2)
	b, err := json.Marshal(e.ToJSON())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"code", "category", "message", "retryable", "httpStatus", "context"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q in %s", k, b)
		}
	}
	if m["httpStatus"] != float64(409) || m["retryable"] != true {
		t.Errorf("bad values: %s", b)
	}
	b, _ = json.Marshal(NewError(CodeEventStoreError, "x").ToJSON())
	if !strings.Contains(string(b), `"context":{}`) {
		t.Errorf("empty context should be {}: %s", b)
	}
}

func TestHTTPResponseContextOnly4xx(t *testing.T) {
	r := AggregateNotFound("Account", "1").ToHTTPResponse()
	body := r.Body["error"].(map[string]any)
	if r.Status != 400 || body["context"] == nil {
		t.Errorf("4xx: %+v", r)
	}
	r = OptimisticConcurrency("s", 1, 2).ToHTTPResponse()
	if r.Status != 409 || r.Body["error"].(map[string]any)["context"] == nil {
		t.Errorf("409 should include context: %+v", r)
	}
	r = NewError(CodeEventStoreError, "x").WithContext("secret", "dsn").ToHTTPResponse()
	body = r.Body["error"].(map[string]any)
	if r.Status != 500 {
		t.Errorf("status %d", r.Status)
	}
	if _, ok := body["context"]; ok {
		t.Error("5xx leaked context")
	}
}

func TestHTTPResponseNeverLeaksCause(t *testing.T) {
	e := NewDomainError("X", "bad").WithCause(errors.New("password=hunter2"))
	b, _ := json.Marshal(e.ToHTTPResponse().Body)
	if strings.Contains(string(b), "hunter2") {
		t.Errorf("cause leaked: %s", b)
	}
	b, _ = json.Marshal(e.ToJSON())
	if strings.Contains(string(b), "hunter2") {
		t.Errorf("cause leaked in ToJSON: %s", b)
	}
}

func TestLogEntryLevels(t *testing.T) {
	if NewDomainError("X", "").ToLogEntry().Level != slog.LevelWarn {
		t.Error("domain should warn")
	}
	for _, e := range []*Error{
		OptimisticConcurrency("s", 1, 2),
		NewError(CodeEventStoreError, ""),
		NewError(CodeInvalidConfiguration, ""),
	} {
		if e.ToLogEntry().Level != slog.LevelError {
			t.Errorf("%s should log at error", e.Code)
		}
	}
}

func TestLogEntryCauseChainAndValuer(t *testing.T) {
	inner := errors.New("disk full")
	e := NewError(CodeEventAppendFailed, "append").WithContext("streamId", "s").WithCause(inner)
	entry := e.ToLogEntry()
	if len(entry.Causes) != 1 || entry.Causes[0] != "disk full" {
		t.Errorf("causes: %v", entry.Causes)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Error("failed", "err", e)
	out := buf.String()
	for _, want := range []string{"EVENT_APPEND_FAILED", "infrastructure", "err.context.streamId=s", "disk full"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q lacks %q", out, want)
		}
	}
}
