package local

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

func TestProviderSharesStorage(t *testing.T) {
	ctx := context.Background()
	p := NewProvider()
	if p.ProviderType() != "local" {
		t.Fatal(p.ProviderType())
	}
	if err := p.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	a, _ := p.CreateEventStoreAdapter()
	b, _ := p.CreateEventStoreAdapter()
	sid := stream("Account")
	_, _ = a.AppendToStream(ctx, sid, evs(1), 0)
	if v, _ := b.StreamVersion(ctx, sid); v != 1 {
		t.Fatal("two event store adapters do not share data")
	}
	// Another provider over the same database sees it too; a new one does not.
	c, _ := NewProviderWithDatabase(p.Database()).CreateEventStoreAdapter()
	if v, _ := c.StreamVersion(ctx, sid); v != 1 {
		t.Fatal("shared database not shared")
	}
	d, _ := NewProvider().CreateEventStoreAdapter()
	if v, _ := d.StreamVersion(ctx, sid); v != 0 {
		t.Fatal("new provider is not empty")
	}
	if err := p.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.StreamVersion(ctx, sid); v != 1 {
		t.Fatal("Initialize lost data")
	}
	for _, f := range []func() error{
		func() error { _, e := p.CreateSnapshotStoreAdapter(); return e },
		func() error { _, e := p.CreateSagaStoreAdapter(); return e },
		func() error { _, e := p.CreateProcessedEventStore(); return e },
		func() error { _, e := p.CreateCommandIdempotencyStore(); return e },
		func() error { _, e := p.CreateDeadLetterStore(); return e },
	} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProviderUnitsAndBuses(t *testing.T) {
	p := NewProvider()
	u1, u2 := p.CreateUnitOfWork(), p.CreateUnitOfWork()
	if u1 == u2 || u1.IsActive() {
		t.Fatal("units must be new and inactive")
	}
	for _, mode := range []tarodomain.ConsistencyMode{tarodomain.ConsistencyStrong, tarodomain.ConsistencyEventual} {
		for _, tr := range []string{"", tarodomain.TransportInMemory} {
			bus, err := p.CreateEventBus(tarodomain.EventBusConfig{Consistency: mode, Transport: tr})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := bus.(tarodomain.SubscribableEventBus); !ok {
				t.Fatalf("%s bus is not subscribable", mode)
			}
		}
	}
	b1, _ := p.CreateEventBus(tarodomain.EventBusConfig{Consistency: tarodomain.ConsistencyStrong})
	b2, _ := p.CreateEventBus(tarodomain.EventBusConfig{Consistency: tarodomain.ConsistencyStrong})
	if b1 == b2 {
		t.Fatal("buses must be new")
	}
	_, err := p.CreateEventBus(tarodomain.EventBusConfig{Consistency: tarodomain.ConsistencyEventual, Transport: "queue"})
	if !errors.Is(err, tarodomain.ErrCode(tarodomain.CodeInvalidConfiguration)) {
		t.Fatalf("queue transport: %v", err)
	}
	if _, err := p.CreateEventBus(tarodomain.EventBusConfig{Consistency: "sometimes"}); err == nil {
		t.Fatal("bad consistency accepted")
	}
}

func TestProviderClock(t *testing.T) {
	clock := tarodomain.NewManualClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p := NewProviderWithDatabase(NewDatabase(clock))
	if tarodomain.ClockOf(p) != tarodomain.Clock(clock) {
		t.Fatal("provider clock is not the database clock")
	}
	ctx := context.Background()
	s, _ := p.CreateSagaStoreAdapter()
	_ = s.Save(ctx, saga(1, "T", tarodomain.SagaRunning))
	r, _, _ := s.Load(ctx, "T", uid(1))
	if !r.StartedAt.Equal(clock.Now()) {
		t.Fatalf("stamped %v", r.StartedAt)
	}
	if _, ok := tarodomain.SleeperOf(p).(tarodomain.TimerSleeper); !ok {
		t.Fatal("sleeper fallback")
	}
}
