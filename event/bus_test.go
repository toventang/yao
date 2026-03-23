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

// --- Test handler ---

type recordHandler struct {
	mu       sync.Mutex
	calls    []string // records ev.Type for each Handle call
	shutdown bool
}

func (h *recordHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	h.mu.Lock()
	h.calls = append(h.calls, ev.Type)
	h.mu.Unlock()

	if ev.IsCall {
		var p string
		if err := ev.Should(&p); err == nil {
			resp <- types.Result{Data: "echo:" + p}
		} else {
			resp <- types.Result{Data: "echo:" + ev.Type}
		}
	}
}

func (h *recordHandler) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shutdown = true
	return nil
}

func (h *recordHandler) getCalls() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]string, len(h.calls))
	copy(cp, h.calls)
	return cp
}

// --- Phase 3: Push / Call basic routing (no queue) ---

func TestPush_NoQueue(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &recordHandler{}
	event.Register("foo", h)
	if err := event.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = event.Stop(context.Background()) }()

	id, err := event.Push(context.Background(), "foo.bar", "payload1")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty event ID")
	}

	// Wait for async handler
	time.Sleep(50 * time.Millisecond)
	calls := h.getCalls()
	if len(calls) != 1 || calls[0] != "foo.bar" {
		t.Fatalf("expected [foo.bar], got %v", calls)
	}
}

func TestCall_NoQueue(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &recordHandler{}
	event.Register("foo", h)
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	id, data, err := event.Call(context.Background(), "foo.get", "hello")
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty event ID")
	}
	if data != "echo:hello" {
		t.Fatalf("expected echo:hello, got %v", data)
	}
}

func TestPush_UnregisteredPrefix(t *testing.T) {
	event.Reset()
	defer event.Reset()

	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, err := event.Push(context.Background(), "unknown.thing", nil)
	if err != event.ErrNoHandler {
		t.Fatalf("expected ErrNoHandler, got %v", err)
	}
}

func TestPush_NotStarted(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	_, err := event.Push(context.Background(), "foo.bar", nil)
	if err != event.ErrNotStarted {
		t.Fatalf("expected ErrNotStarted, got %v", err)
	}
}

func TestPush_SIDAndAuth(t *testing.T) {
	event.Reset()
	defer event.Reset()

	var captured *types.Event
	var mu sync.Mutex

	h := &captureHandler{onHandle: func(ev *types.Event) {
		mu.Lock()
		captured = ev
		mu.Unlock()
	}}
	event.Register("foo", h)
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	ctx := event.WithSID(context.Background(), "sess-abc")
	ctx = event.WithAuth(ctx, &types.AuthorizedInfo{UserID: "u-1"})

	_, err := event.Push(ctx, "foo.bar", "data")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if captured == nil {
		t.Fatal("handler was not called")
	}
	if captured.SID != "sess-abc" {
		t.Fatalf("expected SID sess-abc, got %s", captured.SID)
	}
	if captured.Auth == nil || captured.Auth.UserID != "u-1" {
		t.Fatalf("expected Auth.UserID u-1, got %+v", captured.Auth)
	}
}

// captureHandler captures the event for inspection.
type captureHandler struct {
	onHandle func(*types.Event)
}

func (h *captureHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	if h.onHandle != nil {
		h.onHandle(ev)
	}
	if ev.IsCall {
		resp <- types.Result{Data: "ok"}
	}
}

func (h *captureHandler) Shutdown(ctx context.Context) error { return nil }

// --- Coverage: prefixOf without dot ---

func TestPush_TypeWithoutDot(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &recordHandler{}
	event.Register("nodot", h)
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	id, err := event.Push(context.Background(), "nodot", "payload")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty event ID")
	}
	time.Sleep(50 * time.Millisecond)
	calls := h.getCalls()
	if len(calls) != 1 || calls[0] != "nodot" {
		t.Fatalf("expected [nodot], got %v", calls)
	}
}

// --- Coverage: Call unregistered prefix ---

func TestCall_UnregisteredPrefix(t *testing.T) {
	event.Reset()
	defer event.Reset()

	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, _, err := event.Call(context.Background(), "unknown.thing", nil)
	if err != event.ErrNoHandler {
		t.Fatalf("expected ErrNoHandler, got %v", err)
	}
}

// --- Coverage: Call with queue (happy path) ---

func TestCall_WithQueue(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &recordHandler{}
	event.Register("foo", h)
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, err := event.QueueCreate("foo")
	if err != nil {
		t.Fatalf("QueueCreate failed: %v", err)
	}
	defer event.QueueRelease(qID)

	id, data, err := event.Call(context.Background(), "foo.get", "hello", event.Queue(qID))
	if err != nil {
		t.Fatalf("Call with queue failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty event ID")
	}
	if data != "echo:hello" {
		t.Fatalf("expected echo:hello, got %v", data)
	}
}

// --- Coverage: Call with non-existent queue ---

func TestCall_QueueNotFound(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("foo", &recordHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, _, err := event.Call(context.Background(), "foo.get", nil, event.Queue("no-such-queue"))
	if err != event.ErrQueueNotFound {
		t.Fatalf("expected ErrQueueNotFound, got %v", err)
	}
}

// --- Coverage: Call ctx timeout ---

func TestCall_CtxTimeout(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &captureHandler{onHandle: func(ev *types.Event) {
		time.Sleep(500 * time.Millisecond)
	}}
	event.Register("slow", h)
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, _, err := event.Call(ctx, "slow.op", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// --- Coverage: Call no-queue dispatch failure (ctx cancelled) ---

func TestCall_NoQueue_DispatchFail(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &concurrencyHandler{
		peak:    &atomic.Int32{},
		current: &atomic.Int32{},
		delay:   200 * time.Millisecond,
	}
	event.Register("tiny", h, event.MaxWorkers(1), event.ReservedWorkers(0))
	_ = event.Start()

	// Saturate the single total slot with a Call in background
	bgDone := make(chan struct{})
	go func() {
		defer close(bgDone)
		_, _, _ = event.Call(context.Background(), "tiny.work", nil)
	}()
	time.Sleep(10 * time.Millisecond)

	// Another Call with already-cancelled context should fail at dispatch
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := event.Call(ctx, "tiny.op", nil)
	if err == nil {
		t.Fatal("expected error for cancelled ctx call")
	}

	// Wait for background goroutine to finish before Stop
	<-bgDone
	_ = event.Stop(context.Background())
}
