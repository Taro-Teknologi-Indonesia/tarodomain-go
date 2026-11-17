package local

import (
	"context"
	"testing"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func clocked() (*Database, *tarodomain.ManualClock, context.Context) {
	c := tarodomain.NewManualClock(t0)
	return NewDatabase(c), c, context.Background()
}

func TestSnapshots(t *testing.T) {
	db, clock, ctx := clocked()
	s := NewSnapshotStore(db)
	id, other := tarodomain.NewAggregateID(), tarodomain.NewAggregateID()
	save := func(id tarodomain.AggregateID, typ string, v int64, data string) {
		if err := s.Save(ctx, tarodomain.SnapshotInput{SourceID: id, SourceType: typ, SourceVersion: v, Data: []byte(data)}); err != nil {
			t.Fatal(err)
		}
	}
	save(id, "A", 5, `{"v":5}`)
	clock.Advance(time.Second)
	save(id, "A", 10, `{"v":10}`)
	save(id, "B", 99, `{}`)
	save(other, "A", 50, `{}`)

	latest, ok, _ := s.LoadLatest(ctx, id, "A")
	if !ok || latest.SourceVersion != 10 || string(latest.Data) != `{"v":10}` || !latest.CreatedAt.Equal(t0.Add(time.Second)) {
		t.Fatalf("latest %+v", latest)
	}
	// Upsert: new data, original CreatedAt.
	clock.Advance(time.Hour)
	save(id, "A", 5, `{"v":"5b"}`)
	at5, ok, _ := s.LoadAtVersion(ctx, id, "A", 5)
	if !ok || string(at5.Data) != `{"v":"5b"}` || !at5.CreatedAt.Equal(t0) {
		t.Fatalf("at5 %+v", at5)
	}
	if _, ok, _ := s.LoadAtVersion(ctx, id, "A", 6); ok {
		t.Fatal("found missing version")
	}
	if n, _ := s.Count(ctx, id, "A"); n != 2 {
		t.Fatalf("count %d", n)
	}
	list, _ := s.List(ctx, id, "A", tarodomain.SnapshotListOptions{})
	if len(list) != 2 || list[0].SourceVersion != 10 {
		t.Fatalf("list %+v", list)
	}
	if list, _ = s.List(ctx, id, "A", tarodomain.SnapshotListOptions{Limit: 1}); len(list) != 1 {
		t.Fatal("limit ignored")
	}
	if n, _ := s.DeleteBefore(ctx, id, "A", 10); n != 1 {
		t.Fatalf("DeleteBefore %d", n)
	}
	if n, _ := s.DeleteBefore(ctx, id, "A", 10); n != 0 {
		t.Fatalf("cut-off must stay, removed %d", n)
	}
	_ = s.DeleteAll(ctx, id, "A")
	_ = s.DeleteAll(ctx, id, "A")
	if n, _ := s.Count(ctx, id, "A"); n != 0 {
		t.Fatalf("count %d", n)
	}
	if n, _ := s.Count(ctx, id, "B"); n != 1 {
		t.Fatal("DeleteAll touched another type")
	}
	if n, _ := s.Count(ctx, other, "A"); n != 1 {
		t.Fatal("DeleteAll touched another aggregate")
	}
}

func saga(id int, typ string, status tarodomain.SagaStatus) tarodomain.SagaSaveInput {
	return tarodomain.SagaSaveInput{SagaID: uid(id), SagaType: typ, CorrelationID: uid(id),
		State: []byte(`{}`), Status: status, CurrentStep: "s"}
}

func TestSagas(t *testing.T) {
	db, clock, ctx := clocked()
	s := NewSagaStore(db)
	_ = s.Save(ctx, saga(1, "T", tarodomain.SagaRunning))
	clock.Advance(time.Minute)
	_ = s.Save(ctx, saga(1, "T", tarodomain.SagaRunning))
	r, ok, _ := s.Load(ctx, "T", uid(1))
	if !ok || !r.StartedAt.Equal(t0) || !r.UpdatedAt.Equal(t0.Add(time.Minute)) || !r.CompletedAt.IsZero() {
		t.Fatalf("record %+v", r)
	}
	if c, ok, _ := s.LoadByCorrelationID(ctx, "T", uid(1)); !ok || c.SagaID != uid(1) {
		t.Fatal("by correlation")
	}

	clock.Advance(time.Minute)
	_ = s.Complete(ctx, "T", uid(1))
	done, _, _ := s.Load(ctx, "T", uid(1))
	clock.Advance(time.Minute)
	_ = s.Complete(ctx, "T", uid(1))
	_ = s.Fail(ctx, "T", uid(1), "late")
	again, _, _ := s.Load(ctx, "T", uid(1))
	if again.Status != tarodomain.SagaCompleted || !again.CompletedAt.Equal(done.CompletedAt) || again.FailureReason != "" {
		t.Fatalf("ended twice: %+v", again)
	}
	_ = s.Complete(ctx, "T", uid(404)) // missing: no-op
	_ = s.Fail(ctx, "T", uid(404), "x")
	if _, ok, _ := s.Load(ctx, "T", uid(404)); ok {
		t.Fatal("end created a saga")
	}

	// List order and filter-before-limit.
	s2 := NewSagaStore(NewDatabase(clock))
	_ = s2.Save(ctx, saga(3, "A", tarodomain.SagaRunning))
	_ = s2.Save(ctx, saga(2, "A", tarodomain.SagaRunning))
	_ = s2.Save(ctx, saga(1, "B", tarodomain.SagaRunning))
	clock.Advance(time.Second)
	_ = s2.Save(ctx, saga(4, "B", tarodomain.SagaCompensating))
	_ = s2.Save(ctx, saga(5, "B", tarodomain.SagaFailed))
	got, _ := s2.LoadByStatus(ctx, tarodomain.SagaRunning, tarodomain.SagaQuery{})
	if len(got) != 3 || got[0].SagaID != uid(2) || got[1].SagaID != uid(3) || got[2].SagaType != "B" {
		t.Fatalf("order: %+v", got)
	}
	got, _ = s2.LoadByStatus(ctx, tarodomain.SagaRunning, tarodomain.SagaQuery{SagaType: "B", Limit: 1})
	if len(got) != 1 || got[0].SagaType != "B" {
		t.Fatalf("filter before limit: %+v", got)
	}
	to, _ := s2.LoadTimedOut(ctx, t0.Add(time.Hour), tarodomain.SagaQuery{SagaType: "B", Limit: 5})
	if len(to) != 2 { // running + compensating, not failed
		t.Fatalf("timed out: %+v", to)
	}
	if to, _ = s2.LoadTimedOut(ctx, t0, tarodomain.SagaQuery{}); len(to) != 0 {
		t.Fatalf("cut-off is exclusive: %+v", to)
	}
	if n, _ := s2.Count(ctx, "", ""); n != 5 {
		t.Fatalf("count all %d", n)
	}
	if n, _ := s2.Count(ctx, "B", tarodomain.SagaFailed); n != 1 {
		t.Fatalf("count B failed %d", n)
	}
	_ = s2.Delete(ctx, "B", uid(5))
	_ = s2.Delete(ctx, "B", uid(5))
	if n, _ := s2.Count(ctx, "B", ""); n != 2 {
		t.Fatalf("count B %d", n)
	}
}

func TestProcessedEvents(t *testing.T) {
	db, clock, ctx := clocked()
	p := NewProcessedEvents(db)
	_ = p.MarkProcessed(ctx, "h", "i1", uid(1))
	clock.Advance(time.Hour)
	_ = p.MarkProcessed(ctx, "h", "i1", uid(1)) // keeps first time
	_ = p.MarkProcessed(ctx, "h", "i2", uid(1))
	_ = p.MarkProcessed(ctx, "other", "i1", uid(1))
	if ok, _ := p.Exists(ctx, "h", "i1", uid(1)); !ok {
		t.Fatal("missing")
	}
	if ok, _ := p.Exists(ctx, "h", "i3", uid(1)); ok {
		t.Fatal("other instance exists")
	}
	if n, _ := p.Count(ctx, "h", ""); n != 2 {
		t.Fatalf("count %d", n)
	}
	if n, _ := p.Count(ctx, "h", "i1"); n != 1 {
		t.Fatalf("count i1 %d", n)
	}
	// i1's mark is from t0, so a cut-off at t0 removes none and one hour later removes it.
	if n, _ := p.DeleteOld(ctx, t0); n != 0 {
		t.Fatalf("exclusive cut-off removed %d", n)
	}
	if n, _ := p.DeleteOld(ctx, t0.Add(time.Minute)); n != 1 {
		t.Fatalf("DeleteOld %d", n)
	}
	if n, _ := p.DeleteByHandler(ctx, "h", "i2"); n != 1 {
		t.Fatalf("by instance %d", n)
	}
	if n, _ := p.DeleteByHandler(ctx, "other", ""); n != 1 {
		t.Fatalf("by handler %d", n)
	}
	if n, _ := p.DeleteByHandler(ctx, "other", ""); n != 0 {
		t.Fatal("deleted twice")
	}
}

func TestProcessedCommands(t *testing.T) {
	db, clock, ctx := clocked()
	p := NewProcessedCommands(db)
	_ = p.MarkProcessed(ctx, uid(2), "Cmd", "h")
	_ = p.MarkProcessed(ctx, uid(1), "Cmd", "h") // same time: ordered by ID
	clock.Advance(time.Hour)
	_ = p.MarkProcessed(ctx, uid(2), "Cmd", "changed") // first record kept
	_ = p.MarkProcessed(ctx, uid(3), "Cmd", "g")
	at, ok, _ := p.ProcessedAt(ctx, uid(2))
	if !ok || !at.Equal(t0) {
		t.Fatalf("at %v %v", at, ok)
	}
	if ok, _ := p.Exists(ctx, uid(9)); ok {
		t.Fatal("unknown exists")
	}
	list, _ := p.List(ctx, "h", 0)
	if len(list) != 2 || list[0].CommandID != uid(1) || list[1].CommandID != uid(2) || list[1].HandlerName != "h" {
		t.Fatalf("list %+v", list)
	}
	if all, _ := p.List(ctx, "", 2); len(all) != 2 {
		t.Fatal("limit")
	}
	if n, _ := p.Count(ctx, ""); n != 3 {
		t.Fatalf("count %d", n)
	}
	if n, _ := p.DeleteOld(ctx, t0); n != 0 {
		t.Fatalf("exclusive removed %d", n)
	}
	if n, _ := p.DeleteOld(ctx, t0.Add(time.Minute)); n != 2 {
		t.Fatalf("DeleteOld %d", n)
	}
}

func TestDeadLetters(t *testing.T) {
	db, clock, ctx := clocked()
	d := NewDeadLetters(db)
	in := func(h string, ev int) tarodomain.DeadLetterInput {
		return tarodomain.DeadLetterInput{HandlerName: h, EventID: uid(ev), EventType: "E",
			EventData: []byte(`{}`), ErrorCode: "X", ErrorMessage: "boom", Attempts: 3}
	}
	id1, _ := d.Write(ctx, in("h", 1))
	clock.Advance(time.Hour)
	id2, _ := d.Write(ctx, in("h", 1))
	id3, _ := d.Write(ctx, in("g", 2))
	if !(id1 < id2 && id2 < id3) {
		t.Fatalf("ids %d %d %d", id1, id2, id3)
	}
	_ = d.Delete(ctx, id3)
	_ = d.Delete(ctx, id3)
	if id4, _ := d.Write(ctx, in("g", 3)); id4 <= id3 {
		t.Fatalf("id %d reused after delete of %d", id4, id3)
	}
	latest, ok, _ := d.Get(ctx, uid(1), "h")
	if !ok || latest.ID != id2 || latest.Attempts != 3 {
		t.Fatalf("latest %+v", latest)
	}
	if e, ok, _ := d.GetByID(ctx, id1); !ok || !e.CreatedAt.Equal(t0) {
		t.Fatalf("by id %+v", e)
	}
	if _, ok, _ := d.GetByID(ctx, id3); ok {
		t.Fatal("deleted entry found")
	}
	list, _ := d.List(ctx, "h", 0)
	if len(list) != 2 || list[0].ID != id1 {
		t.Fatalf("list %+v", list)
	}
	if n, _ := d.Count(ctx, ""); n != 3 {
		t.Fatalf("count %d", n)
	}
	if n, _ := d.DeleteOld(ctx, t0); n != 0 {
		t.Fatalf("exclusive removed %d", n)
	}
	if n, _ := d.DeleteOld(ctx, t0.Add(time.Minute)); n != 1 {
		t.Fatalf("DeleteOld %d", n)
	}
}
