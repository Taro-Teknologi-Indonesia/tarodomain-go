package tarodomain

import "fmt"

// AggregateID identifies one aggregate (§2.4). It wraps a UUID; the zero
// value is the nil UUID and means "no ID".
type AggregateID struct{ u UUID }

// NewAggregateID returns a new v7 aggregate ID.
func NewAggregateID() AggregateID { return AggregateID{NewUUIDv7()} }

// AggregateIDFrom parses a UUID of any version. Failure is
// VALUE_OBJECT_INVALID.
func AggregateIDFrom(s string) (AggregateID, error) {
	u, err := ParseUUID(s)
	if err != nil {
		return AggregateID{}, invalidID(s, err)
	}
	return AggregateID{u}, nil
}

// AggregateIDFromV7 is like AggregateIDFrom but accepts only version 7.
func AggregateIDFromV7(s string) (AggregateID, error) {
	id, err := AggregateIDFrom(s)
	if err != nil {
		return id, err
	}
	if !id.u.IsV7() {
		return AggregateID{}, invalidID(s, fmt.Errorf("not a version 7 UUID"))
	}
	return id, nil
}

func invalidID(s string, cause error) *Error {
	return NewError(CodeValueObjectInvalid, fmt.Sprintf("invalid aggregate ID %q", s)).
		WithContext("value", s).WithCause(cause)
}

// UUID returns the underlying UUID.
func (id AggregateID) UUID() UUID { return id.u }

// String returns the lower-case hyphenated UUID.
func (id AggregateID) String() string { return id.u.String() }

// IsZero reports whether id is unset.
func (id AggregateID) IsZero() bool { return id.u.IsZero() }

// Equal reports whether both are the same UUID. Case never matters, because
// the text form is parsed.
func (id AggregateID) Equal(other AggregateID) bool { return id.u == other.u }

// MarshalText implements encoding.TextMarshaler, and so JSON as a string.
func (id AggregateID) MarshalText() ([]byte, error) { return id.u.MarshalText() }

// UnmarshalText implements encoding.TextUnmarshaler; failure is
// VALUE_OBJECT_INVALID.
func (id *AggregateID) UnmarshalText(text []byte) error {
	v, err := AggregateIDFrom(string(text))
	if err != nil {
		return err
	}
	*id = v
	return nil
}

// ID is an aggregate ID tagged with a type, so two kinds of ID can't be mixed
// up at compile time:
//
//	type accountTag struct{}
//	type AccountID = tarodomain.ID[accountTag]
//
// IDs are comparable and can be map keys.
type ID[Tag any] struct{ id AggregateID }

// NewID returns a new v7 ID.
func NewID[Tag any]() ID[Tag] { return ID[Tag]{NewAggregateID()} }

// IDFrom parses a UUID of any version; failure is VALUE_OBJECT_INVALID.
func IDFrom[Tag any](s string) (ID[Tag], error) {
	id, err := AggregateIDFrom(s)
	return ID[Tag]{id}, err
}

// IDFromV7 is like IDFrom but accepts only version 7.
func IDFromV7[Tag any](s string) (ID[Tag], error) {
	id, err := AggregateIDFromV7(s)
	return ID[Tag]{id}, err
}

// Untyped returns the plain AggregateID, which repositories take.
func (i ID[Tag]) Untyped() AggregateID { return i.id }

// String returns the lower-case hyphenated UUID.
func (i ID[Tag]) String() string { return i.id.String() }

// IsZero reports whether the ID is unset.
func (i ID[Tag]) IsZero() bool { return i.id.IsZero() }

// MarshalText implements encoding.TextMarshaler, and so JSON as a string.
func (i ID[Tag]) MarshalText() ([]byte, error) { return i.id.MarshalText() }

// UnmarshalText implements encoding.TextUnmarshaler.
func (i *ID[Tag]) UnmarshalText(text []byte) error { return i.id.UnmarshalText(text) }
