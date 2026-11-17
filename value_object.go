package tarodomain

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// ValueObject is an immutable, validated value (§2.3). Its fields are
// unexported, so the only way to a ValueObject is NewValueObject, and the
// only way to change one is With, which validates again.
//
// If T is a slice, map or pointer, the ValueObject can't stop callers
// changing what it points to; use value types.
type ValueObject[T any] struct {
	value    T
	validate func(T) (bool, error)
}

// NewValueObject validates value and wraps it. validate may be nil, which
// accepts everything. When it returns false the result is
// VALUE_OBJECT_INVALID; a returned *Error passes through unchanged; any other
// error becomes VALUE_OBJECT_CONSTRUCTION_FAILED with that error as its cause.
func NewValueObject[T any](value T, validate func(T) (bool, error)) (ValueObject[T], error) {
	if validate != nil {
		ok, err := validate(value)
		if err != nil {
			if _, isFramework := AsError(err); isFramework {
				return ValueObject[T]{}, err
			}
			return ValueObject[T]{}, NewError(CodeValueObjectConstructionFailed,
				fmt.Sprintf("constructing %T failed", value)).WithCause(err)
		}
		if !ok {
			return ValueObject[T]{}, NewError(CodeValueObjectInvalid,
				fmt.Sprintf("invalid %T value: %v", value, value)).WithContext("value", fmt.Sprint(value))
		}
	}
	return ValueObject[T]{value: value, validate: validate}, nil
}

// Value returns the wrapped value.
func (v ValueObject[T]) Value() T { return v.value }

// Equal reports whether both wrap equal values. If T has an Equal(T) bool
// method it is used; otherwise the values are compared with reflect.DeepEqual.
func (v ValueObject[T]) Equal(other ValueObject[T]) bool {
	if eq, ok := any(v.value).(interface{ Equal(T) bool }); ok {
		return eq.Equal(other.value)
	}
	return reflect.DeepEqual(v.value, other.value)
}

// With returns a new ValueObject holding f(value), validated by the same
// rule. The receiver is unchanged.
func (v ValueObject[T]) With(f func(T) T) (ValueObject[T], error) {
	return NewValueObject(f(v.value), v.validate)
}

// MarshalJSON writes the wrapped value. To read one back, decode the plain
// value and pass it to NewValueObject, so it is validated.
func (v ValueObject[T]) MarshalJSON() ([]byte, error) { return json.Marshal(v.value) }
