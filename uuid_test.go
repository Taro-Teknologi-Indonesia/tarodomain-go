package tarodomain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewUUIDv7Bits(t *testing.T) {
	before := time.Now().UnixMilli()
	u := NewUUIDv7()
	after := time.Now().UnixMilli()
	if !u.IsV7() || u.IsZero() {
		t.Fatalf("not v7: %s", u)
	}
	if u[6]>>4 != 7 || u[8]>>6 != 2 {
		t.Errorf("version/variant bits: %s", u)
	}
	if ms := u.UnixMilli(); ms < before || ms > after {
		t.Errorf("timestamp %d outside [%d, %d]", ms, before, after)
	}
	if NewUUIDv7() == u {
		t.Error("two v7 IDs equal")
	}
}

func TestIsV7RejectsOthers(t *testing.T) {
	u, err := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil || u.IsV7() {
		t.Errorf("v4 detected as v7: %v %v", u, err)
	}
	if (UUID{}).IsV7() {
		t.Error("nil UUID is not v7")
	}
}

func TestParseUUID(t *testing.T) {
	lower, err1 := ParseUUID("0192f0a1-7b3c-7def-8abc-0123456789ab")
	upper, err2 := ParseUUID("0192F0A1-7B3C-7DEF-8ABC-0123456789AB")
	if err1 != nil || err2 != nil || lower != upper {
		t.Fatalf("case: %v %v", err1, err2)
	}
	if lower.String() != "0192f0a1-7b3c-7def-8abc-0123456789ab" {
		t.Errorf("String: %s", lower)
	}
	for _, bad := range []string{
		"", "0192f0a17b3c7def8abc0123456789ab",
		"{0192f0a1-7b3c-7def-8abc-0123456789ab}",
		"urn:uuid:0192f0a1-7b3c-7def-8abc-0123456789ab",
		"0192f0a1-7b3c-7def-8abc-0123456789a",
		"0192f0a1-7b3c-7def-8abc-0123456789ag",
		"0192f0a1_7b3c-7def-8abc-0123456789ab",
	} {
		if _, err := ParseUUID(bad); !errors.Is(err, ErrInvalidUUID) {
			t.Errorf("%q accepted: %v", bad, err)
		}
	}
}

func TestDerivedUUIDv7(t *testing.T) {
	a := DerivedUUIDv7("saga|event", 0, 1_700_000_000_000)
	if a != DerivedUUIDv7("saga|event", 0, 1_700_000_000_000) {
		t.Error("not deterministic")
	}
	if !a.IsV7() || a.UnixMilli() != 1_700_000_000_000 {
		t.Errorf("not a v7 with its timestamp: %s", a)
	}
	for name, b := range map[string]UUID{
		"seed":  DerivedUUIDv7("other", 0, 1_700_000_000_000),
		"index": DerivedUUIDv7("saga|event", 1, 1_700_000_000_000),
		"time":  DerivedUUIDv7("saga|event", 0, 1_700_000_000_001),
	} {
		if a == b {
			t.Errorf("does not depend on %s", name)
		}
	}
	// The random part must not change with the timestamp alone.
	if c := DerivedUUIDv7("saga|event", 0, 5); string(a[6:]) != string(c[6:]) {
		t.Error("random bits changed with the timestamp")
	}
}

func TestUUIDJSON(t *testing.T) {
	type wrap struct {
		ID UUID            `json:"id"`
		M  map[UUID]string `json:"m"`
	}
	in := wrap{ID: NewUUIDv7(), M: map[UUID]string{NewUUIDv7(): "x"}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"id":"`+in.ID.String()+`"`) {
		t.Errorf("not a string: %s", b)
	}
	var out wrap
	if err := json.Unmarshal(b, &out); err != nil || out.ID != in.ID || len(out.M) != 1 {
		t.Errorf("round trip: %v %+v", err, out)
	}
	if err := json.Unmarshal([]byte(`{"id":"nope"}`), &out); err == nil {
		t.Error("invalid text accepted")
	}
}
