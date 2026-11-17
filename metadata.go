package tarodomain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
)

// Metadata is an immutable map from string to a primitive value: nil, bool,
// number or string (§2.1). The zero value is empty and ready to use.
//
// Numbers are kept as json.Number, so an integer survives a JSON round trip
// exactly, even above 2^53.
type Metadata struct {
	m map[string]any
}

// NewMetadata builds metadata from a map, copying it. A value that isn't nil,
// bool, a number or a string is INVALID_CONFIGURATION.
func NewMetadata(values map[string]any) (Metadata, error) {
	if len(values) == 0 {
		return Metadata{}, nil
	}
	m := make(map[string]any, len(values))
	for k, v := range values {
		n, err := normalizeMetadataValue(k, v)
		if err != nil {
			return Metadata{}, err
		}
		m[k] = n
	}
	return Metadata{m: m}, nil
}

// MustMetadata is like NewMetadata but panics on an invalid value. It is for
// literals in tests and examples.
func MustMetadata(values map[string]any) Metadata {
	md, err := NewMetadata(values)
	if err != nil {
		panic(err)
	}
	return md
}

func normalizeMetadataValue(key string, v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string:
		return x, nil
	case json.Number:
		return x, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return json.Number(fmt.Sprint(x)), nil
	case float32:
		return normalizeFloat(key, float64(x))
	case float64:
		return normalizeFloat(key, x)
	}
	return nil, NewError(CodeInvalidConfiguration,
		fmt.Sprintf("metadata value for %q must be nil, bool, number or string, got %T", key, v)).
		WithContext("key", key)
}

func normalizeFloat(key string, f float64) (any, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, NewError(CodeInvalidConfiguration,
			fmt.Sprintf("metadata value for %q is not a finite number", key)).WithContext("key", key)
	}
	b, _ := json.Marshal(f)
	return json.Number(b), nil
}

// With returns a copy of the metadata with key set to value. The receiver is
// unchanged.
func (md Metadata) With(key string, value any) (Metadata, error) {
	n, err := normalizeMetadataValue(key, value)
	if err != nil {
		return md, err
	}
	m := make(map[string]any, len(md.m)+1)
	maps.Copy(m, md.m)
	m[key] = n
	return Metadata{m: m}, nil
}

// Merge returns a copy holding the entries of md and then those of other;
// other wins on a shared key.
func (md Metadata) Merge(other Metadata) Metadata {
	if len(other.m) == 0 {
		return md
	}
	m := make(map[string]any, len(md.m)+len(other.m))
	maps.Copy(m, md.m)
	maps.Copy(m, other.m)
	return Metadata{m: m}
}

// Get returns the value for key and whether it is present. A number comes
// back as json.Number.
func (md Metadata) Get(key string) (any, bool) {
	v, ok := md.m[key]
	return v, ok
}

// Len returns the number of entries.
func (md Metadata) Len() int { return len(md.m) }

// Keys returns the keys in sorted order.
func (md Metadata) Keys() []string {
	return slices.Sorted(maps.Keys(md.m))
}

// Map returns a copy of the entries.
func (md Metadata) Map() map[string]any { return maps.Clone(md.m) }

// Equal reports whether both hold the same entries.
func (md Metadata) Equal(other Metadata) bool {
	return maps.Equal(md.m, other.m)
}

// MarshalJSON writes a JSON object with the keys in sorted order. Empty
// metadata is {}.
func (md Metadata) MarshalJSON() ([]byte, error) {
	if len(md.m) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(md.m)
}

// UnmarshalJSON reads a JSON object whose values are null, booleans, numbers
// or strings. Arrays and objects are rejected, and integers stay exact.
func (md *Metadata) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return NewError(CodeSerializationError, "metadata must be a JSON object").WithCause(err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return NewError(CodeSerializationError, "metadata has trailing data")
	}
	out, err := NewMetadata(raw)
	if err != nil {
		return NewError(CodeSerializationError, "metadata holds a non-primitive value").WithCause(err)
	}
	*md = out
	return nil
}
