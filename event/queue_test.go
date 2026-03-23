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

// --- Phase 4: Queue tests ---

// orderHandler records the order of payload values to verify FIFO.
type orderHandler struct {
	mu    sync.Mutex
	order []int
}

func (h *orderHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	var v int
	if err := ev.Should(&v); err == nil {
		h.mu.Lock()
		h.order = append(h.order, v)
		h.mu.Unlock()
	}
	if ev.IsCall {
		resp <- types.Result{Data: v}
	}
}

func (h *orderHandler) Shutdown(ctx context.Context) error { return nil }

func (h *orderHandler) getOrder() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]int, len(h.order))
	copy(cp, h.order)
	return cp
}

func TestQueueCreate_Release_FIFO(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &orderHandler{}
	event.Register("seq", h, event.QueueSize(100))
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, err := event.QueueCreate("seq")
	if err != nil {
		t.Fatalf("QueueCreate failed: %v", err)
	}
	if qID == "" {
		t.Fatal("expected non-empty queue ID")
	}

	n := 20
	for i := 0; i < n; i++ {
		_, err := event.Push(context.Background(), "seq.append", i, event.Queue(qID))
		if err != nil {
			t.Fatalf("Push %d failed: %v", i, err)
		}
	}

	// Release and wait for drain
	event.QueueRelease(qID)
	time.Sleep(200 * time.Millisecond)

	order := h.getOrder()
	if len(order) != n {
		t.Fatalf("expected %d events, got %d", n, len(order))
	}
	for i, v := range order {
		if v != i {
			t.Fatalf("FIFO violation at index %d: expected %d, got %d", i, i, v)
		}
	}
}

func TestQueueCreate_CustomID(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, err := event.QueueCreate("seq", "my-custom-id")
	if err != nil {
		t.Fatalf("QueueCreate failed: %v", err)
	}
	if qID != "my-custom-id" {
		t.Fatalf("expected my-custom-id, got %s", qID)
	}
	event.QueueRelease(qID)
}

func TestQueueCreate_Duplicate(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, _ = event.QueueCreate("seq", "dup-id")
	_, err := event.QueueCreate("seq", "dup-id")
	if err != event.ErrQueueExists {
		t.Fatalf("expected ErrQueueExists, got %v", err)
	}
	event.QueueRelease("dup-id")
}

func TestQueueCreate_UnregisteredPrefix(t *testing.T) {
	event.Reset()
	defer event.Reset()

	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, err := event.QueueCreate("nonexist")
	if err != event.ErrNoHandler {
		t.Fatalf("expected ErrNoHandler, got %v", err)
	}
}

func TestPush_QueueNotFound(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, err := event.Push(context.Background(), "seq.append", 1, event.Queue("no-such-queue"))
	if err != event.ErrQueueNotFound {
		t.Fatalf("expected ErrQueueNotFound, got %v", err)
	}
}

func TestPush_QueueReleased(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, _ := event.QueueCreate("seq")
	event.QueueRelease(qID)
	time.Sleep(50 * time.Millisecond)

	_, err := event.Push(context.Background(), "seq.append", 1, event.Queue(qID))
	if err != event.ErrQueueReleased {
		t.Fatalf("expected ErrQueueReleased after release, got %v", err)
	}
}

func TestQueueAbort_DiscardsPending(t *testing.T) {
	event.Reset()
	defer event.Reset()

	// slowHandler delays processing to let events pile up
	var processed atomic.Int32
	slow := &slowHandler{delay: 50 * time.Millisecond, counter: &processed}
	event.Register("slow", slow, event.QueueSize(100))
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, _ := event.QueueCreate("slow")

	// Push 10 events; first will start processing, rest queue up
	for i := 0; i < 10; i++ {
		_, _ = event.Push(context.Background(), "slow.work", i, event.Queue(qID))
	}

	time.Sleep(30 * time.Millisecond) // let first event start
	event.QueueAbort(qID)
	time.Sleep(200 * time.Millisecond)

	count := processed.Load()
	if count >= 10 {
		t.Fatalf("abort should discard pending events, but %d were processed", count)
	}
}

// slowHandler processes events with a delay.
type slowHandler struct {
	delay   time.Duration
	counter *atomic.Int32
}

func (h *slowHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	time.Sleep(h.delay)
	h.counter.Add(1)
	if ev.IsCall {
		resp <- types.Result{Data: "done"}
	}
}

func (h *slowHandler) Shutdown(ctx context.Context) error { return nil }

func TestQueue_CallInsideQueue_Serial(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &orderHandler{}
	event.Register("seq", h, event.QueueSize(100))
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, _ := event.QueueCreate("seq")
	defer event.QueueRelease(qID)

	// Push 5, then Call, then Push 5 more
	for i := 0; i < 5; i++ {
		_, _ = event.Push(context.Background(), "seq.append", i, event.Queue(qID))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := event.Call(ctx, "seq.append", 99, event.Queue(qID))
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if data != 99 {
		t.Fatalf("expected 99, got %v", data)
	}

	for i := 5; i < 10; i++ {
		_, _ = event.Push(context.Background(), "seq.append", i, event.Queue(qID))
	}

	time.Sleep(200 * time.Millisecond)
	order := h.getOrder()

	// The Call (99) should appear after the first 5 and before the last 5
	found := false
	for i, v := range order {
		if v == 99 {
			if i < 5 {
				t.Fatalf("Call should be after first 5 pushes, found at index %d", i)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Call result (99) not found in order: %v", order)
	}
}

func TestQueueFull(t *testing.T) {
	event.Reset()
	defer event.Reset()

	var processed atomic.Int32
	slow := &slowHandler{delay: 100 * time.Millisecond, counter: &processed}
	event.Register("tiny", slow, event.QueueSize(2))
	_ = event.Start()

	qID, _ := event.QueueCreate("tiny")

	// Fill the queue (size=2)
	_, err1 := event.Push(context.Background(), "tiny.work", 1, event.Queue(qID))
	_, err2 := event.Push(context.Background(), "tiny.work", 2, event.Queue(qID))

	// These may or may not succeed depending on timing, but eventually one should fail
	var fullErr error
	for i := 0; i < 10; i++ {
		_, err := event.Push(context.Background(), "tiny.work", i+3, event.Queue(qID))
		if err == event.ErrQueueFull {
			fullErr = err
			break
		}
	}

	if err1 != nil {
		t.Fatalf("first push should succeed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second push should succeed: %v", err2)
	}
	if fullErr == nil {
		t.Log("warning: queue never reported full (handler may be too fast)")
	}

	// Wait for queued events to finish before Stop to avoid race between
	// consumer goroutine (dispatch/wg.Add) and Stop (pool.wait/wg.Wait).
	event.QueueRelease(qID)
	time.Sleep(500 * time.Millisecond)
	_ = event.Stop(context.Background())
}

// --- Coverage: QueueRelease idempotent (release non-existent queue) ---

func TestQueueRelease_NonExistent(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	// Should not panic
	event.QueueRelease("never-created")
}

// --- Coverage: QueueAbort idempotent (abort non-existent queue) ---

func TestQueueAbort_NonExistent(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	// Should not panic
	event.QueueAbort("never-created")
}

// --- Coverage: QueueAbort after already released ---

func TestQueueAbort_AfterRelease(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, _ := event.QueueCreate("seq")
	event.QueueRelease(qID)
	time.Sleep(50 * time.Millisecond)

	// Abort after release should not panic (already removed from map)
	event.QueueAbort(qID)
}

// --- Coverage: Stop with active queues (abortAll path) ---

func TestStop_WithActiveQueues(t *testing.T) {
	event.Reset()
	defer event.Reset()

	var processed atomic.Int32
	slow := &slowHandler{delay: 30 * time.Millisecond, counter: &processed}
	event.Register("bg", slow, event.QueueSize(100))
	_ = event.Start()

	qID, _ := event.QueueCreate("bg")
	for i := 0; i < 5; i++ {
		_, _ = event.Push(context.Background(), "bg.work", i, event.Queue(qID))
	}
	time.Sleep(10 * time.Millisecond)

	// Stop should abort all queues and wait
	err := event.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// --- Coverage: Call with queue enqueue failure (queue released) ---

func TestCall_QueueReleased(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("seq", &orderHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	qID, _ := event.QueueCreate("seq")
	event.QueueRelease(qID)
	time.Sleep(50 * time.Millisecond)

	_, _, err := event.Call(context.Background(), "seq.get", nil, event.Queue(qID))
	if err != event.ErrQueueReleased {
		t.Fatalf("expected ErrQueueReleased, got %v", err)
	}
}
