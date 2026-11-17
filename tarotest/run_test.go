package tarotest

import (
	"strings"
	"testing"
)

func TestRecorderStopsACaseOnFatalfAndKeepsGoing(t *testing.T) {
	r := NewRecorder()
	after := false
	r.Run("suite", func(t T) {
		t.Run("a", func(t T) {
			t.Errorf("first %d", 1)
			t.Fatalf("second")
			after = true
		})
		t.Run("b", func(t T) { t.Skipf("no units") })
		t.Run("c", func(t T) { panic("boom") })
		t.Run("d", func(t T) {})
	})
	if after {
		t.Error("code after Fatalf ran")
	}
	f := r.Failures()
	if len(f) != 3 || f[0] != "suite/a: first 1" || f[1] != "suite/a: second" || !strings.HasPrefix(f[2], "suite/c: panic: boom") {
		t.Errorf("failures = %q", f)
	}
	if s := r.Skipped(); len(s) != 1 || s[0] != "suite/b: no units" {
		t.Errorf("skipped = %q", s)
	}
	if !r.Failed() || r.Cases() != 5 {
		t.Errorf("failed=%v cases=%d", r.Failed(), r.Cases())
	}
}

func TestSuiteNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Suites() {
		if s.Name == "" || s.Run == nil || seen[s.Name] {
			t.Errorf("bad suite %q", s.Name)
		}
		seen[s.Name] = true
	}
	if len(seen) != 9 {
		t.Errorf("%d suites", len(seen))
	}
}
