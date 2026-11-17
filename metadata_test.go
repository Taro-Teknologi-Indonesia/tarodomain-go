package tarodomain

import (
	"encoding/json"
	"testing"
)

func TestMetadataWithReturnsCopy(t *testing.T) {
	a := MustMetadata(map[string]any{"x": 1})
	b, err := a.With("y", "two")
	if err != nil {
		t.Fatal(err)
	}
	if a.Len() != 1 || b.Len() != 2 {
		t.Fatalf("lens %d %d", a.Len(), b.Len())
	}
	if _, ok := a.Get("y"); ok {
		t.Error("With changed the receiver")
	}
	m := b.Map()
	m["z"] = 1
	if b.Len() != 2 {
		t.Error("Map exposed internal storage")
	}
}

func TestMetadataRejectsNonPrimitive(t *testing.T) {
	for _, v := range []any{[]int{1}, map[string]any{}, struct{}{}} {
		_, err := NewMetadata(map[string]any{"k": v})
		if !isCode(err, CodeInvalidConfiguration) {
			t.Errorf("%T: got %v", v, err)
		}
	}
	if _, err := MustMetadata(nil).With("f", nan()); err == nil {
		t.Error("NaN accepted")
	}
}

func nan() float64 { var z float64; return z / z }

func isCode(err error, code ErrorCode) bool {
	c, ok := CodeOf(err)
	return ok && c == code
}

func TestMetadataJSONKeyOrderAndExactIntegers(t *testing.T) {
	md := MustMetadata(map[string]any{"b": true, "a": int64(9007199254740993), "c": nil, "d": "s", "e": 1.5})
	data, err := json.Marshal(md)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"a":9007199254740993,"b":true,"c":null,"d":"s","e":1.5}`
	if string(data) != want {
		t.Fatalf("got %s", data)
	}
	var back Metadata
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(md) {
		t.Errorf("round trip changed: %v", back.Map())
	}
	v, _ := back.Get("a")
	if v != json.Number("9007199254740993") {
		t.Errorf("integer lost precision: %v", v)
	}
}

func TestMetadataJSONRejects(t *testing.T) {
	for _, in := range []string{`{"a":[1]}`, `{"a":{"b":1}}`, `[1]`, `"x"`, `{"a":1} {}`} {
		var md Metadata
		if err := json.Unmarshal([]byte(in), &md); err == nil {
			t.Errorf("%s accepted", in)
		}
	}
}

func TestMetadataZeroValue(t *testing.T) {
	var md Metadata
	data, _ := json.Marshal(md)
	if string(data) != "{}" {
		t.Errorf("got %s", data)
	}
	var back Metadata
	if err := json.Unmarshal([]byte(`null`), &back); err != nil || back.Len() != 0 {
		t.Errorf("null: %v %d", err, back.Len())
	}
}

func TestMetadataMergeAndKeys(t *testing.T) {
	a := MustMetadata(map[string]any{"a": 1, "b": 2})
	b := MustMetadata(map[string]any{"b": 3, "c": 4})
	m := a.Merge(b)
	if v, _ := m.Get("b"); v != json.Number("3") {
		t.Errorf("other should win: %v", v)
	}
	if k := m.Keys(); len(k) != 3 || k[0] != "a" || k[2] != "c" {
		t.Errorf("keys %v", k)
	}
	if a.Len() != 2 {
		t.Error("Merge changed the receiver")
	}
}
