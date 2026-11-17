package tarodomain

import (
	"strings"
	"testing"
)

func TestStreamForAggregateRoundTrip(t *testing.T) {
	for _, typ := range []string{"BankAccount", "bank-account", "a-b-c"} {
		id := NewAggregateID()
		s := StreamForAggregate(typ, id)
		if s != typ+"-"+id.String() {
			t.Fatalf("stream = %q", s)
		}
		ref := ParseStreamID(s)
		if ref.Kind != StreamKindAggregate || ref.Category != typ || ref.AggregateID != id {
			t.Errorf("%q parsed as %+v", s, ref)
		}
		if CategoryOf(s) != typ {
			t.Errorf("CategoryOf(%q) = %q", s, CategoryOf(s))
		}
	}
}

func TestParseStreamIDKinds(t *testing.T) {
	id := NewAggregateID().String()
	nohyphen := strings.ReplaceAll(id, "-", "")
	cases := []struct {
		in       string
		kind     StreamKind
		category string
	}{
		{StreamAll, StreamKindAll, ""},
		{"BankAccount", StreamKindCategory, "BankAccount"},
		{"Bank-Account", StreamKindCategory, "Bank-Account"},
		{"", StreamKindCategory, ""},
		{"Acct-" + strings.ToUpper(id), StreamKindCategory, "Acct-" + strings.ToUpper(id)},
		{"Acct" + id, StreamKindCategory, "Acct" + id},
		{"-" + id, StreamKindCategory, "-" + id},
		{"Acct-" + nohyphen, StreamKindCategory, "Acct-" + nohyphen},
		{"Acct-" + id, StreamKindAggregate, "Acct"},
	}
	for _, c := range cases {
		ref := ParseStreamID(c.in)
		if ref.Kind != c.kind || ref.Category != c.category {
			t.Errorf("ParseStreamID(%q) = %+v, want kind %v category %q", c.in, ref, c.kind, c.category)
		}
		if (ref.Kind == StreamKindAggregate) == ref.AggregateID.IsZero() {
			t.Errorf("ParseStreamID(%q): wrong aggregate ID for kind %v", c.in, ref.Kind)
		}
	}
}

func TestStreamKindString(t *testing.T) {
	for k, want := range map[StreamKind]string{
		StreamKindCategory: "category", StreamKindAggregate: "aggregate", StreamKindAll: "all",
	} {
		if k.String() != want {
			t.Errorf("%d = %q, want %q", k, k.String(), want)
		}
	}
}
