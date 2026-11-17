package tarotest

import (
	"context"
	"encoding/json"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// contractPayload is the event the provider suite publishes on a bus.
type contractPayload struct{ N int }

func (contractPayload) EventName() string { return "ContractPayload" }

// recordingHandler remembers the events it was given.
type recordingHandler struct {
	name string
	got  []tarodomain.DomainEvent
}

func (h *recordingHandler) Name() string { return h.name }

func (h *recordingHandler) Handle(_ context.Context, e tarodomain.DomainEvent) error {
	h.got = append(h.got, e)
	return nil
}

// ProviderContract checks the parts of a [tarodomain.Provider] that are not
// one store's: its type, shared storage, initialization, buses and units.
func ProviderContract(t T, newProvider Factory) {
	t.Helper()
	each := func(name string, f func(t T, ctx context.Context, p tarodomain.Provider)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			f(t, ctx, fresh(ctx, t, newProvider))
		})
	}

	each("the provider has a type", func(t T, ctx context.Context, p tarodomain.Provider) {
		if p.ProviderType() == "" {
			t.Errorf("ProviderType is empty")
		}
	})

	each("adapters of one provider share storage", func(t T, ctx context.Context, p tarodomain.Provider) {
		a, err := p.CreateEventStoreAdapter()
		noErr(t, err)
		b, err := p.CreateEventStoreAdapter()
		noErr(t, err)
		sid := newStream("Account")
		_, err = a.AppendToStream(ctx, sid, events(1), 0)
		noErr(t, err)
		if v, _ := b.StreamVersion(ctx, sid); v != 1 {
			t.Errorf("a second event store adapter sees version %d", v)
		}
		s1, err := p.CreateSagaStoreAdapter()
		noErr(t, err)
		s2, err := p.CreateSagaStoreAdapter()
		noErr(t, err)
		noErr(t, s1.Save(ctx, tarodomain.SagaSaveInput{SagaID: otherID(1), SagaType: "Transfer",
			CorrelationID: otherID(1), State: json.RawMessage(`{}`), Status: tarodomain.SagaRunning}))
		if _, ok, _ := s2.Load(ctx, "Transfer", otherID(1)); !ok {
			t.Errorf("a second saga store adapter does not see the saga")
		}
	})

	each("Initialize can run again without losing data", func(t T, ctx context.Context, p tarodomain.Provider) {
		s, err := p.CreateEventStoreAdapter()
		noErr(t, err)
		sid := newStream("Account")
		_, err = s.AppendToStream(ctx, sid, events(1, 2), 0)
		noErr(t, err)
		noErr(t, p.Initialize(ctx))
		noErr(t, p.Initialize(ctx))
		if v, _ := s.StreamVersion(ctx, sid); v != 2 {
			t.Errorf("version after Initialize = %d", v)
		}
		again, err := p.CreateEventStoreAdapter()
		noErr(t, err)
		if es, _ := again.LoadStream(ctx, sid, 0, 0); !sameInts(numbers(es), []int{1, 2}) {
			t.Errorf("events after Initialize = %v", numbers(es))
		}
	})

	each("the strong bus delivers to subscribers", func(t T, ctx context.Context, p tarodomain.Provider) {
		bus, err := p.CreateEventBus(tarodomain.EventBusConfig{Consistency: tarodomain.ConsistencyStrong})
		noErr(t, err)
		sub, ok := bus.(tarodomain.SubscribableEventBus)
		if !ok {
			t.Fatalf("the strong bus is %T, which is not subscribable", bus)
		}
		h := &recordingHandler{name: "h"}
		noErr(t, sub.Subscribe(h))
		if err := sub.Subscribe(&recordingHandler{name: "h"}); !hasCode(err, tarodomain.CodeInvalidConfiguration) {
			t.Errorf("a second handler with the same name: %v", err)
		}
		ev, err := tarodomain.NewDomainEvent(tarodomain.NewEventEnvelope(ctx), contractPayload{N: 1})
		noErr(t, err)
		noErr(t, bus.PublishAll(ctx, []tarodomain.DomainEvent{ev}))
		if len(h.got) != 1 || h.got[0].Envelope().ID != ev.Envelope().ID {
			t.Errorf("the handler received %d events", len(h.got))
		}
		if !sub.Unsubscribe("h") || sub.Unsubscribe("h") {
			t.Errorf("Unsubscribe did not report correctly")
		}
		noErr(t, bus.PublishAll(ctx, []tarodomain.DomainEvent{ev}))
		if len(h.got) != 1 {
			t.Errorf("an unsubscribed handler still received events")
		}
	})

	each("an unknown transport is rejected", func(t T, ctx context.Context, p tarodomain.Provider) {
		for _, mode := range []tarodomain.ConsistencyMode{tarodomain.ConsistencyStrong, tarodomain.ConsistencyEventual} {
			_, err := p.CreateEventBus(tarodomain.EventBusConfig{Consistency: mode, Transport: "no-such-transport"})
			if !hasCode(err, tarodomain.CodeInvalidConfiguration) {
				t.Errorf("%s: err = %v", mode, err)
			}
		}
	})

	each("a unit of work is new and not begun", func(t T, ctx context.Context, p tarodomain.Provider) {
		a := p.CreateUnitOfWork()
		if a == nil {
			t.Skipf("provider %q has no unit of work", p.ProviderType())
			return
		}
		b := p.CreateUnitOfWork()
		if a == b {
			t.Errorf("CreateUnitOfWork returned the same unit twice")
		}
		if a.IsActive() || b.IsActive() {
			t.Errorf("a new unit is active")
		}
	})
}
