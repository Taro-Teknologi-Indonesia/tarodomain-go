package tarodomain

import (
	"encoding/json"
	"errors"
	"testing"
)

func positive(n int) (bool, error) { return n > 0, nil }

func TestValueObjectInvalidRejected(t *testing.T) {
	if _, err := NewValueObject(-1, positive); !isCode(err, CodeValueObjectInvalid) {
		t.Errorf("got %v", err)
	}
	v, err := NewValueObject(5, positive)
	if err != nil || v.Value() != 5 {
		t.Fatalf("%v %v", v, err)
	}
	if _, err := NewValueObject(-1, nil); err != nil {
		t.Errorf("nil validate should accept: %v", err)
	}
}

func TestValueObjectErrorMapping(t *testing.T) {
	custom := NewDomainError("TOO_SMALL", "too small")
	_, err := NewValueObject(1, func(int) (bool, error) { return false, custom })
	if err != error(custom) {
		t.Errorf("framework error should pass through, got %v", err)
	}
	cause := errors.New("boom")
	_, err = NewValueObject(1, func(int) (bool, error) { return true, cause })
	if !isCode(err, CodeValueObjectConstructionFailed) || !errors.Is(err, cause) {
		t.Errorf("got %v", err)
	}
}

func TestValueObjectWithValidatesAgain(t *testing.T) {
	v, _ := NewValueObject(5, positive)
	w, err := v.With(func(n int) int { return n * 2 })
	if err != nil || w.Value() != 10 || v.Value() != 5 {
		t.Fatalf("%v %v %v", v, w, err)
	}
	if _, err := v.With(func(n int) int { return -n }); !isCode(err, CodeValueObjectInvalid) {
		t.Errorf("got %v", err)
	}
}

func TestValueObjectEqualAndJSON(t *testing.T) {
	notEmpty := func(s string) (bool, error) { return s != "", nil }
	a, _ := NewValueObject("x", notEmpty)
	b, _ := NewValueObject("x", notEmpty)
	c, _ := NewValueObject("y", notEmpty)
	if !a.Equal(b) || a.Equal(c) {
		t.Error("Equal wrong")
	}
	data, err := json.Marshal(struct{ V ValueObject[string] }{a})
	if err != nil || string(data) != `{"V":"x"}` {
		t.Errorf("%s %v", data, err)
	}
}
