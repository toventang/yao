package event_test

import (
	"context"
	"testing"
	"time"

	"github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/event/types"
)

// --- Phase 7: Subscriber tests ---

func TestSubscribe_Basic(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	ch := make(chan *types.Event, 10)
	subID := event.Subscribe("foo.*", ch)
	if subID == "" {
		t.Fatal("expected non-empty subscription ID")
	}
	defer event.Unsubscribe(subID)

	_, _ = event.Push(context.Background(), "foo.bar", "payload")
	_, _ = event.Push(context.Background(), "foo.baz", "payload2")

	received := drainChan(ch, 2, 200*time.Millisecond)
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Type != "foo.bar" {
		t.Fatalf("expected foo.bar, got %s", received[0].Type)
	}
	if received[1].Type != "foo.baz" {
		t.Fatalf("expected foo.baz, got %s", received[1].Type)
	}
}

func TestSubscribe_PatternFilter(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	event.Register("bar", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	ch := make(chan *types.Event, 10)
	subID := event.Subscribe("foo.*", ch, event.Filter(func(ev *types.Event) bool {
		return ev.Type == "foo.keep"
	}))
	defer event.Unsubscribe(subID)

	_, _ = event.Push(context.Background(), "foo.keep", nil)
	_, _ = event.Push(context.Background(), "foo.drop", nil)
	_, _ = event.Push(context.Background(), "bar.thing", nil)

	received := drainChan(ch, 1, 200*time.Millisecond)
	if len(received) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(received))
	}
	if received[0].Type != "foo.keep" {
		t.Fatalf("expected foo.keep, got %s", received[0].Type)
	}
}

func TestSubscribe_Unsubscribe(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	ch := make(chan *types.Event, 10)
	subID := event.Subscribe("foo.*", ch)

	_, _ = event.Push(context.Background(), "foo.first", nil)
	time.Sleep(50 * time.Millisecond)

	event.Unsubscribe(subID)

	_, _ = event.Push(context.Background(), "foo.second", nil)
	time.Sleep(50 * time.Millisecond)

	received := drainChan(ch, 10, 100*time.Millisecond)
	for _, ev := range received {
		if ev.Type == "foo.second" {
			t.Fatal("should not receive events after Unsubscribe")
		}
	}
}

func TestSubscribe_ChanFull_Skip(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	ch := make(chan *types.Event, 1) // tiny buffer
	subID := event.Subscribe("foo.*", ch)
	defer event.Unsubscribe(subID)

	// Push multiple events quickly; only 1 should fit in buffer
	for i := 0; i < 5; i++ {
		_, _ = event.Push(context.Background(), "foo.item", i)
	}

	time.Sleep(100 * time.Millisecond)

	// Should have at most 1 in channel (rest skipped)
	count := len(ch)
	if count > 1 {
		t.Fatalf("expected at most 1 buffered event, got %d", count)
	}
}

func TestSubscribe_WildcardAll(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	event.Register("bar", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	ch := make(chan *types.Event, 10)
	subID := event.Subscribe("*", ch)
	defer event.Unsubscribe(subID)

	_, _ = event.Push(context.Background(), "foo.one", nil)
	_, _ = event.Push(context.Background(), "bar.two", nil)

	received := drainChan(ch, 2, 200*time.Millisecond)
	if len(received) != 2 {
		t.Fatalf("wildcard * should receive all events, got %d", len(received))
	}
}

func TestSubscribe_StopClearsSubscribers(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	_ = event.Start()

	ch := make(chan *types.Event, 10)
	_ = event.Subscribe("foo.*", ch)

	_ = event.Stop(context.Background())

	// After Stop, Push should fail
	_, err := event.Push(context.Background(), "foo.bar", nil)
	if err != event.ErrNotStarted {
		t.Fatalf("expected ErrNotStarted after Stop, got %v", err)
	}
}

// drainChan reads up to n events from ch within timeout.
// Stops early if the channel is closed.
func drainChan(ch chan *types.Event, n int, timeout time.Duration) []*types.Event {
	var result []*types.Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for range n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return result
			}
			result = append(result, ev)
		case <-timer.C:
			return result
		}
	}
	return result
}
