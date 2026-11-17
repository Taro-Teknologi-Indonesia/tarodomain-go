package tarodomain

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type recHandler struct {
	name string
	mu   *sync.Mutex
	log  *[]string
	err  error
	fn   func(context.Context, DomainEvent) error
}

func (h recHandler) Name() string { return h.name }
func (h recHandler) Handle(ctx context.Context, e DomainEvent) error {
	h.mu.Lock()
	*h.log = append(*h.log, h.name+":"+e.Name())
	h.mu.Unlock()
	if h.fn != nil {
		return h.fn(ctx, e)
	}
	return h.err
}

type busFixture struct {
	mu  sync.Mutex
	log []string
}

func (f *busFixture) h(name string, err error) recHandler {
	return recHandler{name: name, mu: &f.mu, log: &f.log, err: err}
}

func testEvents(t *testing.T, names ...string) []DomainEvent {
	t.Helper()
	var out []DomainEvent
	for _, n := range names {
		var p Event = opened{}
		if n == "second" {
			p = unknownEvent{}
		}
		e, err := NewDomainEvent(NewEventEnvelope(context.Background()), p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func TestBusDeliveryOrder(t *testing.T) {
	f := &busFixture{}
	b := NewInMemoryEventBus("b")
	_ = b.Subscribe(f.h("a", nil))
	_ = b.Subscribe(f.h("b", nil))
	if err := b.PublishAll(context.Background(), testEvents(t, "first", "second")); err != nil {
		t.Fatal(err)
	}
	want := []string{"a:AccountOpened", "b:AccountOpened", "a:Unknown", "b:Unknown"}
	if len(f.log) != 4 {
		t.Fatalf("log = %v", f.log)
	}
	for i := range want {
		if f.log[i] != want[i] {
			t.Fatalf("log = %v, want %v", f.log, want)
		}
	}
}

func TestBusFailFast(t *testing.T) {
	f := &busFixture{}
	boom := errors.New("boom")
	b := NewInMemoryEventBus("b")
	_ = b.Subscribe(f.h("a", boom))
	_ = b.Subscribe(f.h("b", nil))
	err := b.PublishAll(context.Background(), testEvents(t, "first", "second"))
	if err != boom {
		t.Fatalf("err = %v", err)
	}
	if len(f.log) != 1 {
		t.Errorf("later handlers or events ran: %v", f.log)
	}
}

func TestBusWithResults(t *testing.T) {
	f := &busFixture{}
	boom := errors.New("boom")
	b := NewInMemoryEventBus("b")
	_ = b.Subscribe(f.h("a", boom))
	_ = b.Subscribe(f.h("b", nil))
	res := b.PublishAllWithResults(context.Background(), testEvents(t, "first", "second"))
	if len(res) != 4 || len(f.log) != 4 {
		t.Fatalf("results %d, log %v", len(res), f.log)
	}
	if res[0].Handler != "a" || res[0].Err != boom || res[1].Handler != "b" || res[1].Err != nil {
		t.Errorf("results = %+v", res)
	}
}

func TestBusDuplicateAndUnsubscribe(t *testing.T) {
	f := &busFixture{}
	b := NewInMemoryEventBus("b")
	if b.Name() != "b" {
		t.Error("name")
	}
	_ = b.Subscribe(f.h("a", nil))
	if err := b.Subscribe(f.h("a", nil)); !isCode(err, CodeInvalidConfiguration) {
		t.Errorf("duplicate: %v", err)
	}
	if !b.Unsubscribe("a") || b.Unsubscribe("a") {
		t.Error("unsubscribe results")
	}
	_ = b.PublishAll(context.Background(), testEvents(t, "first"))
	if len(f.log) != 0 {
		t.Errorf("unsubscribed handler ran: %v", f.log)
	}
}

func TestBusHandlerPublishesToOwnBus(t *testing.T) {
	f := &busFixture{}
	b := NewInMemoryEventBus("b")
	inner := testEvents(t, "second")
	published := false
	h := recHandler{name: "re", mu: &f.mu, log: &f.log, fn: func(ctx context.Context, e DomainEvent) error {
		if published {
			return nil
		}
		published = true
		return b.PublishAll(ctx, inner)
	}}
	_ = b.Subscribe(h)
	if err := b.PublishAll(context.Background(), testEvents(t, "first")); err != nil {
		t.Fatal(err)
	}
	if len(f.log) != 2 {
		t.Errorf("log = %v", f.log)
	}
}

func TestBusConcurrentPublishAndSubscribe(t *testing.T) {
	f := &busFixture{}
	b := NewInMemoryEventBus("b")
	_ = b.Subscribe(f.h("a", nil))
	evs := testEvents(t, "first")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if err := b.PublishAll(context.Background(), evs); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 50; j++ {
			_ = b.Subscribe(f.h("x", nil))
			b.Unsubscribe("x")
		}
	}()
	wg.Wait()
}
