package tarotest

import (
	"fmt"
	"runtime"
	"sync"
)

// Suite is one contract suite and its name.
type Suite struct {
	// Name is the suite's subtest name, such as "EventStore".
	Name string
	// Run runs the suite.
	Run func(t T, newProvider Factory)
}

// Suites lists every contract suite, in the order [RunAll] runs them.
func Suites() []Suite {
	return []Suite{
		{"Provider", ProviderContract},
		{"EventStore", EventStoreContract},
		{"Snapshot", SnapshotContract},
		{"SagaStore", SagaStoreContract},
		{"ProcessedEvents", ProcessedEventsContract},
		{"CommandIdempotency", CommandIdempotencyContract},
		{"DeadLetter", DeadLetterContract},
		{"UnitOfWork", UnitOfWorkContract},
		{"Concurrency", ConcurrencyContract},
	}
}

// RunAll runs every suite as a named subtest. A provider's test file is
// three lines:
//
//	func TestContract(t *testing.T) {
//		tarotest.RunAll(tarotest.Wrap(t), newProvider)
//	}
func RunAll(t T, newProvider Factory) {
	t.Helper()
	for _, s := range Suites() {
		t.Run(s.Name, func(t T) { s.Run(t, newProvider) })
	}
}

// Recorder is a [T] that collects failures instead of reporting them to
// package testing, for hosts without "go test" (the Cloudflare harness runs
// the suites inside workerd). Every case runs in a goroutine of its own, so a
// Fatalf or Skipf stops that case only. Safe for concurrent use.
type Recorder struct {
	state *recorderState
	path  string
}

type recorderState struct {
	mu       sync.Mutex
	failures []string
	skipped  []string
	cases    int
}

// NewRecorder returns an empty recorder.
func NewRecorder() *Recorder { return &Recorder{state: &recorderState{}} }

var _ T = (*Recorder)(nil)

func (r *Recorder) note(list *[]string, format string, args []any) {
	msg := fmt.Sprintf(format, args...)
	if r.path != "" {
		msg = r.path + ": " + msg
	}
	r.state.mu.Lock()
	*list = append(*list, msg)
	r.state.mu.Unlock()
}

// Helper does nothing.
func (r *Recorder) Helper() {}

// Logf does nothing.
func (r *Recorder) Logf(string, ...any) {}

// Errorf records a failure and carries on.
func (r *Recorder) Errorf(format string, args ...any) { r.note(&r.state.failures, format, args) }

// Fatalf records a failure and stops the case.
func (r *Recorder) Fatalf(format string, args ...any) {
	r.note(&r.state.failures, format, args)
	runtime.Goexit()
}

// Skipf records a skip and stops the case.
func (r *Recorder) Skipf(format string, args ...any) {
	r.note(&r.state.skipped, format, args)
	runtime.Goexit()
}

// Run runs f as a case of its own and waits for it. A panic in f is recorded
// as a failure.
func (r *Recorder) Run(name string, f func(t T)) {
	child := &Recorder{state: r.state, path: name}
	if r.path != "" {
		child.path = r.path + "/" + name
	}
	r.state.mu.Lock()
	r.state.cases++
	r.state.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if v := recover(); v != nil {
				child.note(&child.state.failures, "panic: %v", []any{v})
			}
		}()
		f(child)
	}()
	<-done
}

// Failures returns the recorded failures, each prefixed with its case path.
func (r *Recorder) Failures() []string {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return append([]string(nil), r.state.failures...)
}

// Skipped returns the recorded skips, each prefixed with its case path.
func (r *Recorder) Skipped() []string {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return append([]string(nil), r.state.skipped...)
}

// Cases returns how many cases ran, suites included.
func (r *Recorder) Cases() int {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return r.state.cases
}

// Failed reports whether any case failed.
func (r *Recorder) Failed() bool { return len(r.Failures()) > 0 }
