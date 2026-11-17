package tarodomain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAggregateIDParsing(t *testing.T) {
	v7 := NewUUIDv7().String()
	if id, err := AggregateIDFrom(strings.ToUpper(v7)); err != nil || id.String() != v7 {
		t.Errorf("upper case: %v %v", id, err)
	}
	upper, _ := AggregateIDFrom(strings.ToUpper(v7))
	lower, _ := AggregateIDFrom(v7)
	if !upper.Equal(lower) || upper != lower {
		t.Error("case should not matter")
	}
	if _, err := AggregateIDFrom("nope"); !isCode(err, CodeValueObjectInvalid) {
		t.Errorf("got %v", err)
	}
	const v4 = "550e8400-e29b-41d4-a716-446655440000"
	if _, err := AggregateIDFrom(v4); err != nil {
		t.Errorf("v4 should be accepted: %v", err)
	}
	if _, err := AggregateIDFromV7(v4); !isCode(err, CodeValueObjectInvalid) {
		t.Errorf("v4 should be rejected by V7 factory: %v", err)
	}
	if _, err := AggregateIDFromV7(v7); err != nil {
		t.Error(err)
	}
	if !NewAggregateID().UUID().IsV7() || (AggregateID{}).IsZero() == false {
		t.Error("new IDs are v7, zero is zero")
	}
}

type accountTag struct{}
type orderTag struct{}

func TestTypedIDJSONRoundTrip(t *testing.T) {
	type doc struct {
		Account ID[accountTag] `json:"account"`
		Order   ID[orderTag]   `json:"order"`
	}
	in := doc{NewID[accountTag](), NewID[orderTag]()}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out doc
	if err := json.Unmarshal(data, &out); err != nil || out != in {
		t.Fatalf("%v %s", err, data)
	}
	if out.Account.Untyped().String() != in.Account.String() {
		t.Error("Untyped differs")
	}
	if err := json.Unmarshal([]byte(`{"account":"bad"}`), &out); !isCode(err, CodeValueObjectInvalid) {
		t.Errorf("got %v", err)
	}
	if _, err := IDFromV7[accountTag]("550e8400-e29b-41d4-a716-446655440000"); err == nil {
		t.Error("IDFromV7 accepted v4")
	}
}
