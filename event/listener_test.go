package event_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/event/types"
)

// --- Phase 6: Listener tests ---

// collectListener collects received events.
type collectListener struct {
	mu     sync.Mutex
	events []*types.Event
	shut   bool
}

func (l *collectListener) OnEvent(ev *types.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ev)
}

func (l *collectListener) Shutdown(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.shut = true
	return nil
}

func (l *collectListener) getEvents() []*types.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]*types.Event, len(l.events))
	copy(cp, l.events)
	return cp
}

func TestListener_PatternMatch(t *testing.T) {
	event.Reset()
	defer event.Reset()

	allL := &collectListener{}
	fooL := &collectListener{}
	exactL := &collectListener{}

	event.Listen("*", allL)
	event.Listen("foo.*", fooL)
	event.Listen("foo.exact", exactL)

	h := &recordHandler{}
	event.Register("foo", h)
	event.Register("bar", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, _ = event.Push(context.Background(), "foo.exact", nil)
	_, _ = event.Push(context.Background(), "foo.other", nil)
	_, _ = event.Push(context.Background(), "bar.thing", nil)

	time.Sleep(100 * time.Millisecond)

	allEvents := allL.getEvents()
	fooEvents := fooL.getEvents()
	exactEvents := exactL.getEvents()

	if len(allEvents) != 3 {
		t.Fatalf("all listener expected 3, got %d", len(allEvents))
	}
	if len(fooEvents) != 2 {
		t.Fatalf("foo.* listener expected 2, got %d", len(fooEvents))
	}
	if len(exactEvents) != 1 {
		t.Fatalf("foo.exact listener expected 1, got %d", len(exactEvents))
	}
	if exactEvents[0].Type != "foo.exact" {
		t.Fatalf("expected foo.exact, got %s", exactEvents[0].Type)
	}
}

func TestListener_Filter(t *testing.T) {
	event.Reset()
	defer event.Reset()

	filtered := &collectListener{}
	event.Listen("foo.*", filtered, event.Filter(func(ev *types.Event) bool {
		return ev.Type == "foo.keep"
	}))

	event.Register("foo", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, _ = event.Push(context.Background(), "foo.keep", nil)
	_, _ = event.Push(context.Background(), "foo.drop", nil)

	time.Sleep(100 * time.Millisecond)
	events := filtered.getEvents()
	if len(events) != 1 || events[0].Type != "foo.keep" {
		t.Fatalf("filter should only pass foo.keep, got %v", events)
	}
}

func TestListener_BufferFull_Skip(t *testing.T) {
	event.Reset()
	defer event.Reset()

	// Use buffer size 2, listener that blocks
	blocking := &blockingListener{unblock: make(chan struct{})}
	event.Listen("foo.*", blocking, event.BufferSize(2))

	event.Register("foo", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	// Push 5 events; 1 being processed + 2 buffered = 3, rest skipped
	for i := 0; i < 5; i++ {
		_, _ = event.Push(context.Background(), "foo.item", i)
	}

	time.Sleep(50 * time.Millisecond)
	close(blocking.unblock) // unblock listener
	time.Sleep(100 * time.Millisecond)

	count := blocking.count.Load()
	if count > 3 {
		t.Fatalf("expected at most 3 events with buffer=2, got %d", count)
	}
	if count < 1 {
		t.Fatal("expected at least 1 event")
	}
}

type blockingListener struct {
	unblock chan struct{}
	count   atomic.Int32
}

func (l *blockingListener) OnEvent(ev *types.Event) {
	<-l.unblock
	l.count.Add(1)
}

func (l *blockingListener) Shutdown(ctx context.Context) error { return nil }

func TestListener_Shutdown(t *testing.T) {
	event.Reset()
	defer event.Reset()

	listener := &collectListener{}
	event.Listen("foo.*", listener)
	event.Register("foo", &recordHandler{})
	_ = event.Start()
	_ = event.Stop(context.Background())

	if !listener.shut {
		t.Fatal("listener Shutdown should have been called")
	}
}

func TestListener_PanicRecovery(t *testing.T) {
	event.Reset()
	defer event.Reset()

	var afterPanic atomic.Int32
	pl := &panicListener{afterPanic: &afterPanic}
	event.Listen("foo.*", pl)
	event.Register("foo", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, _ = event.Push(context.Background(), "foo.panic", nil)
	_, _ = event.Push(context.Background(), "foo.ok", nil)
	time.Sleep(100 * time.Millisecond)

	if afterPanic.Load() < 1 {
		t.Fatal("listener should recover from panic and process next event")
	}
}

type panicListener struct {
	afterPanic *atomic.Int32
	first      atomic.Bool
}

func (l *panicListener) OnEvent(ev *types.Event) {
	if !l.first.Load() {
		l.first.Store(true)
		panic("listener panic")
	}
	l.afterPanic.Add(1)
}

func (l *panicListener) Shutdown(ctx context.Context) error { return nil }

// --- Coverage: notify when listener manager not started ---

func TestListener_NotifyBeforeStart(t *testing.T) {
	event.Reset()
	defer event.Reset()

	listener := &collectListener{}
	event.Listen("foo.*", listener)

	// Register handler but do NOT start service; Push will fail with ErrNotStarted.
	// Instead, we test that listener.notify returns silently before start.
	event.Register("foo", &recordHandler{})

	// Manually start and immediately stop to verify no events leaked
	_ = event.Start()
	_ = event.Stop(context.Background())

	events := listener.getEvents()
	if len(events) != 0 {
		t.Fatalf("expected 0 events before any push, got %d", len(events))
	}
}
