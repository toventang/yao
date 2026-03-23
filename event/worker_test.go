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

// --- Phase 5: Worker pool tests ---

func TestWorker_MaxConcurrency(t *testing.T) {
	event.Reset()
	defer event.Reset()

	var peak atomic.Int32
	var current atomic.Int32

	h := &concurrencyHandler{peak: &peak, current: &current, delay: 30 * time.Millisecond}
	event.Register("conc", h, event.MaxWorkers(4), event.ReservedWorkers(1))
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = event.Push(context.Background(), "conc.work", i)
		}(i)
	}
	wg.Wait()
	time.Sleep(300 * time.Millisecond)

	p := peak.Load()
	if p > 4 {
		t.Fatalf("peak concurrency %d exceeded MaxWorkers 4", p)
	}
	if p < 2 {
		t.Fatalf("peak concurrency %d seems too low, expected at least 2", p)
	}
}

type concurrencyHandler struct {
	peak    *atomic.Int32
	current *atomic.Int32
	delay   time.Duration
}

func (h *concurrencyHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	c := h.current.Add(1)
	for {
		old := h.peak.Load()
		if c <= old || h.peak.CompareAndSwap(old, c) {
			break
		}
	}
	time.Sleep(h.delay)
	h.current.Add(-1)
	if ev.IsCall {
		resp <- types.Result{Data: "ok"}
	}
}

func (h *concurrencyHandler) Shutdown(ctx context.Context) error { return nil }

func TestWorker_CallReservation(t *testing.T) {
	event.Reset()
	defer event.Reset()

	// MaxWorkers=4, ReservedWorkers=2 => Push can use 2, Call can use 4
	var pushActive atomic.Int32
	var callDone atomic.Int32

	h := &reservationHandler{pushActive: &pushActive, callDone: &callDone}
	event.Register("res", h, event.MaxWorkers(4), event.ReservedWorkers(2))
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	// Saturate push slots (only 2 available for push)
	for i := 0; i < 4; i++ {
		_, _ = event.Push(context.Background(), "res.work", i)
	}
	time.Sleep(20 * time.Millisecond) // let pushes start

	// Call should still work (reserved slots)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := event.Call(ctx, "res.get", "ping")
	if err != nil {
		t.Fatalf("Call should succeed with reserved workers: %v", err)
	}
	if data != "pong" {
		t.Fatalf("expected pong, got %v", data)
	}
}

type reservationHandler struct {
	pushActive *atomic.Int32
	callDone   *atomic.Int32
}

func (h *reservationHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	if ev.IsCall {
		resp <- types.Result{Data: "pong"}
		h.callDone.Add(1)
		return
	}
	h.pushActive.Add(1)
	time.Sleep(100 * time.Millisecond)
	h.pushActive.Add(-1)
}

func (h *reservationHandler) Shutdown(ctx context.Context) error { return nil }

func TestWorker_PanicRecovery(t *testing.T) {
	event.Reset()
	defer event.Reset()

	var afterPanic atomic.Bool

	h := &panicHandler{afterPanic: &afterPanic}
	event.Register("pan", h)
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	// First push panics
	_, _ = event.Push(context.Background(), "pan.crash", "boom")
	time.Sleep(50 * time.Millisecond)

	// Second push should still work
	_, _ = event.Push(context.Background(), "pan.ok", "fine")
	time.Sleep(50 * time.Millisecond)

	if !afterPanic.Load() {
		t.Fatal("handler should have processed event after panic recovery")
	}
}

type panicHandler struct {
	afterPanic *atomic.Bool
}

func (h *panicHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	if ev.Type == "pan.crash" {
		panic("test panic")
	}
	h.afterPanic.Store(true)
	if ev.IsCall {
		resp <- types.Result{Data: "ok"}
	}
}

func (h *panicHandler) Shutdown(ctx context.Context) error { return nil }

// --- Coverage: ReservedWorkers >= MaxWorkers (pushSlots clamped to 1) ---

func TestWorker_ReservedExceedsMax(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &recordHandler{}
	event.Register("edge", h, event.MaxWorkers(2), event.ReservedWorkers(5))
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	_, err := event.Push(context.Background(), "edge.work", "data")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	calls := h.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
}

// --- Coverage: dispatch Call ctx cancel while waiting for semTotal ---

func TestWorker_Call_CtxCancel_SemTotal(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &concurrencyHandler{
		peak:    &atomic.Int32{},
		current: &atomic.Int32{},
		delay:   200 * time.Millisecond,
	}
	event.Register("lim", h, event.MaxWorkers(1), event.ReservedWorkers(0))
	_ = event.Start()

	bgDone := make(chan struct{})
	go func() {
		defer close(bgDone)
		_, _, _ = event.Call(context.Background(), "lim.work", nil)
	}()
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, _, err := event.Call(ctx, "lim.op", nil)
	if err == nil {
		t.Fatal("expected error for call with saturated pool")
	}

	<-bgDone
	_ = event.Stop(context.Background())
}
