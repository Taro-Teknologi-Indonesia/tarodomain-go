package tarotest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

// ConcurrencyContract checks that a provider's event store holds up under
// concurrent use.
func ConcurrencyContract(t T, newProvider Factory) {
	t.Helper()
	each := func(name string, f func(t T, ctx context.Context, s tarodomain.EventStoreAdapter)) {
		t.Run(name, func(t T) {
			ctx := context.Background()
			s, err := fresh(ctx, t, newProvider).CreateEventStoreAdapter()
			if err != nil {
				t.Fatalf("CreateEventStoreAdapter: %v", err)
			}
			f(t, ctx, s)
		})
	}
	// report fails the case for every error, outside the goroutines.
	report := func(t T, errs []error) {
		t.Helper()
		for _, err := range errs {
			if err != nil {
				t.Errorf("%v", err)
			}
		}
	}

	each("a reader following a cursor sees every event once, in append order", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		const appenders, perAppender = 4, 15
		var finished atomic.Bool
		var wg sync.WaitGroup
		errs := make([]error, appenders+1)
		for a := 0; a < appenders; a++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sid := newStream("Account")
				for i := 0; i < perAppender; i++ {
					// Data counts up, the ID counts down: IDs are out of append order.
					e := plainEvent(eventID(a*1000+(perAppender-1-i)), a*1000+i)
					if _, err := s.AppendToStream(ctx, sid, []tarodomain.SerializedEvent{e}, int64(i)); err != nil {
						errs[a] = fmt.Errorf("appender %d, event %d: %w", a, i, err)
						return
					}
				}
			}()
		}
		var seen []tarodomain.SerializedEvent
		readerDone := make(chan struct{})
		go func() {
			defer close(readerDone)
			var cursor tarodomain.UUID
			for {
				last := finished.Load() // read before the batch: one more pass after the appenders end
				batch, err := s.LoadAll(ctx, cursor, 7)
				if err != nil {
					errs[appenders] = fmt.Errorf("reader: %w", err)
					return
				}
				seen = append(seen, batch...)
				if len(batch) > 0 {
					cursor = batch[len(batch)-1].EventID
				} else if last {
					return
				}
			}
		}()
		wg.Wait()
		finished.Store(true)
		<-readerDone
		report(t, errs)

		total := appenders * perAppender
		if len(seen) != total {
			t.Fatalf("the reader saw %d events, want %d", len(seen), total)
		}
		ids := map[tarodomain.UUID]bool{}
		for _, e := range seen {
			if ids[e.EventID] {
				t.Errorf("event %v seen twice", e.EventID)
			}
			ids[e.EventID] = true
		}
		full, err := s.LoadAll(ctx, tarodomain.UUID{}, 0)
		noErr(t, err)
		if !sameInts(numbers(full), numbers(seen)) {
			t.Errorf("the reader's order differs from a full read")
		}
		// Within one appender's stream, events keep their append order.
		last := map[int]int{}
		for _, n := range numbers(seen) {
			a, i := n/1000, n%1000
			if prev, ok := last[a]; ok && i != prev+1 {
				t.Errorf("appender %d: event %d came after %d", a, i, prev)
			}
			last[a] = i
		}
	})

	each("writers retrying on conflict lose no update", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		const writers, perWriter = 4, 5
		sid := newStream("Account")
		var wg sync.WaitGroup
		errs := make([]error, writers)
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWriter; i++ {
					e := plainEvent(eventID(w*100+i), w*100+i)
					for {
						v, err := s.StreamVersion(ctx, sid)
						if err != nil {
							errs[w] = err
							return
						}
						_, err = s.AppendToStream(ctx, sid, []tarodomain.SerializedEvent{e}, v)
						if err == nil {
							break
						}
						if !hasCode(err, tarodomain.CodeOptimisticConcurrency) {
							errs[w] = err
							return
						}
					}
				}
			}()
		}
		wg.Wait()
		report(t, errs)
		got, err := s.LoadStream(ctx, sid, 0, 0)
		noErr(t, err)
		if len(got) != writers*perWriter {
			t.Fatalf("%d events stored, want %d", len(got), writers*perWriter)
		}
		if v, _ := s.StreamVersion(ctx, sid); v != writers*perWriter {
			t.Errorf("version = %d", v)
		}
		counts := map[int]int{}
		for _, n := range numbers(got) {
			counts[n]++
		}
		for w := 0; w < writers; w++ {
			for i := 0; i < perWriter; i++ {
				if counts[w*100+i] != 1 {
					t.Errorf("event %d stored %d times", w*100+i, counts[w*100+i])
				}
			}
		}
	})

	each("concurrent readers never fail and agree", func(t T, ctx context.Context, s tarodomain.EventStoreAdapter) {
		sid := newStream("Account")
		_, err := s.AppendToStream(ctx, sid, events(1, 2, 3, 4, 5, 6, 7, 8, 9, 10), 0)
		noErr(t, err)
		const readers = 16
		var wg sync.WaitGroup
		errs := make([]error, readers)
		got := make([][]int, readers)
		for r := 0; r < readers; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				stream, err := s.LoadStream(ctx, sid, 0, 0)
				if err != nil {
					errs[r] = err
					return
				}
				all, err := s.LoadAll(ctx, tarodomain.UUID{}, 0)
				if err != nil {
					errs[r] = err
					return
				}
				got[r] = append(numbers(stream), numbers(all)...)
			}()
		}
		wg.Wait()
		report(t, errs)
		for r := 1; r < readers; r++ {
			if !sameInts(got[0], got[r]) {
				t.Errorf("reader %d disagrees with reader 0", r)
			}
		}
		if len(got[0]) != 20 {
			t.Errorf("reader 0 read %d events", len(got[0]))
		}
	})
}
