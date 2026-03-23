package event_test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/event/types"
)

// ---------------------------------------------------------------------------
// Helper: snapshot goroutine count after GC stabilization.
// ---------------------------------------------------------------------------

func stableGoroutineCount() int {
	// Let runtime settle: GC + finalizers + scheduler
	for i := 0; i < 5; i++ {
		runtime.GC()
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
	return runtime.NumGoroutine()
}

// leakHandler is a no-op handler for leak tests.
type leakHandler struct{}

func (h *leakHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	if ev.IsCall {
		resp <- types.Result{Data: "ok"}
	}
}

func (h *leakHandler) Shutdown(ctx context.Context) error { return nil }

// leakListener is a no-op listener for leak tests.
type leakListener struct{}

func (l *leakListener) OnEvent(ev *types.Event)            {}
func (l *leakListener) Shutdown(ctx context.Context) error { return nil }

// ---------------------------------------------------------------------------
// Test: 1000 Queue create/release cycles leak no goroutines.
// ---------------------------------------------------------------------------

func TestLeak_QueueCreateRelease(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("leak", &leakHandler{}, event.QueueSize(64))
	_ = event.Start()

	before := stableGoroutineCount()

	const cycles = 1000
	for i := 0; i < cycles; i++ {
		qID, err := event.QueueCreate("leak")
		if err != nil {
			t.Fatalf("cycle %d: QueueCreate: %v", i, err)
		}
		// Push a few events to exercise consumer goroutine
		for j := 0; j < 3; j++ {
			_, _ = event.Push(context.Background(), "leak.work", j, event.Queue(qID))
		}
		event.QueueRelease(qID)
	}

	// Let all consumer goroutines drain and exit
	time.Sleep(500 * time.Millisecond)
	after := stableGoroutineCount()

	_ = event.Stop(context.Background())

	leaked := after - before
	t.Logf("goroutines: before=%d after=%d delta=%d (over %d cycles)", before, after, leaked, cycles)

	// Allow a small margin for runtime jitter (GC, timers, etc.)
	if leaked > 5 {
		t.Errorf("goroutine leak: %d goroutines accumulated over %d queue cycles", leaked, cycles)
	}
}

// ---------------------------------------------------------------------------
// Test: 1000 Queue create/abort cycles leak no goroutines.
// ---------------------------------------------------------------------------

func TestLeak_QueueCreateAbort(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("leak", &leakHandler{}, event.QueueSize(64))
	_ = event.Start()

	before := stableGoroutineCount()

	const cycles = 1000
	for i := 0; i < cycles; i++ {
		qID, err := event.QueueCreate("leak")
		if err != nil {
			t.Fatalf("cycle %d: QueueCreate: %v", i, err)
		}
		for j := 0; j < 3; j++ {
			_, _ = event.Push(context.Background(), "leak.work", j, event.Queue(qID))
		}
		event.QueueAbort(qID)
	}

	time.Sleep(500 * time.Millisecond)
	after := stableGoroutineCount()

	_ = event.Stop(context.Background())

	leaked := after - before
	t.Logf("goroutines: before=%d after=%d delta=%d (over %d cycles)", before, after, leaked, cycles)

	if leaked > 5 {
		t.Errorf("goroutine leak: %d goroutines accumulated over %d abort cycles", leaked, cycles)
	}
}

// ---------------------------------------------------------------------------
// Test: Subscriber create/unsubscribe cycles leak no goroutines or memory.
// ---------------------------------------------------------------------------

func TestLeak_SubscriberLifecycle(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("leak", &leakHandler{})
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	before := stableGoroutineCount()
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	const cycles = 1000
	for i := 0; i < cycles; i++ {
		ch := make(chan *types.Event, 16)
		subID := event.Subscribe("leak.*", ch)

		_, _ = event.Push(context.Background(), "leak.work", nil)
		time.Sleep(time.Microsecond) // let notify propagate

		event.Unsubscribe(subID)
	}

	time.Sleep(200 * time.Millisecond)
	after := stableGoroutineCount()

	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	leaked := after - before
	memDeltaMB := float64(int64(memAfter.HeapInuse)-int64(memBefore.HeapInuse)) / 1024 / 1024

	t.Logf("goroutines: before=%d after=%d delta=%d", before, after, leaked)
	t.Logf("heap in-use delta: %.2f MB", memDeltaMB)

	if leaked > 3 {
		t.Errorf("goroutine leak: %d goroutines after %d sub/unsub cycles", leaked, cycles)
	}
}

// ---------------------------------------------------------------------------
// Test: Start/Stop cycles leak no goroutines.
// ---------------------------------------------------------------------------

func TestLeak_StartStopCycles(t *testing.T) {
	before := stableGoroutineCount()

	const cycles = 20
	for i := 0; i < cycles; i++ {
		event.Reset()
		event.Register("leak", &leakHandler{})
		event.Listen("leak.*", &leakListener{})
		_ = event.Start()

		ctx := context.Background()
		for j := 0; j < 10; j++ {
			_, _ = event.Push(ctx, "leak.work", j)
		}
		time.Sleep(10 * time.Millisecond)

		_ = event.Stop(ctx)
	}
	event.Reset()

	time.Sleep(300 * time.Millisecond)
	after := stableGoroutineCount()

	leaked := after - before
	t.Logf("goroutines: before=%d after=%d delta=%d (over %d start/stop cycles)", before, after, leaked, cycles)

	if leaked > 3 {
		t.Errorf("goroutine leak: %d goroutines after %d start/stop cycles", leaked, cycles)
	}
}

// ---------------------------------------------------------------------------
// Test: 1000 concurrent users creating/using/releasing queues, verify
// no goroutine leak when everything settles.
// ---------------------------------------------------------------------------

func TestLeak_1000Users_FullCycle(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("trace", &leakHandler{}, event.MaxWorkers(512), event.QueueSize(8192))
	event.Register("job", &leakHandler{}, event.MaxWorkers(256), event.QueueSize(4096))
	event.Listen("trace.*", &leakListener{})
	_ = event.Start()

	before := stableGoroutineCount()

	const numUsers = 1000
	var wg sync.WaitGroup
	for u := 0; u < numUsers; u++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			ctx := event.WithSID(context.Background(), fmt.Sprintf("s-%d", uid))

			tqID, err := event.QueueCreate("trace")
			if err != nil {
				return
			}
			jqID, err := event.QueueCreate("job")
			if err != nil {
				event.QueueRelease(tqID)
				return
			}

			for i := 0; i < 10; i++ {
				_, _ = event.Push(ctx, "trace.add", i, event.Queue(tqID))
			}
			for i := 0; i < 3; i++ {
				_, _ = event.Push(ctx, "job.progress", i, event.Queue(jqID))
			}

			callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, _, _ = event.Call(callCtx, "trace.get", nil, event.Queue(tqID))
			cancel()

			event.QueueRelease(tqID)
			event.QueueRelease(jqID)
		}(u)
	}

	wg.Wait()
	time.Sleep(1 * time.Second) // let all consumers drain

	after := stableGoroutineCount()

	_ = event.Stop(context.Background())

	// Final check after full stop
	afterStop := stableGoroutineCount()

	leaked := after - before
	leakedAfterStop := afterStop - before

	t.Logf("goroutines: before=%d after_drain=%d after_stop=%d", before, after, afterStop)
	t.Logf("delta after drain: %d, delta after stop: %d", leaked, leakedAfterStop)

	if leaked > 10 {
		t.Errorf("goroutine leak after drain: %d (1000 users × 2 queues)", leaked)
	}
	if leakedAfterStop > 3 {
		t.Errorf("goroutine leak after stop: %d", leakedAfterStop)
	}
}

// ---------------------------------------------------------------------------
// Test: Memory stability under sustained load.
// Push 100k events through 100 queues, measure heap growth.
// ---------------------------------------------------------------------------

func TestLeak_MemoryStability(t *testing.T) {
	event.Reset()
	defer event.Reset()

	event.Register("mem", &leakHandler{}, event.MaxWorkers(256), event.QueueSize(8192))
	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	const (
		numQueues      = 100
		eventsPerQueue = 1000
		totalEvents    = numQueues * eventsPerQueue
	)

	queueIDs := make([]string, numQueues)
	for i := 0; i < numQueues; i++ {
		qID, _ := event.QueueCreate("mem")
		queueIDs[i] = qID
	}

	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	ctx := context.Background()
	var wg sync.WaitGroup
	for q := 0; q < numQueues; q++ {
		wg.Add(1)
		go func(qIdx int) {
			defer wg.Done()
			qID := queueIDs[qIdx]
			for i := 0; i < eventsPerQueue; i++ {
				_, _ = event.Push(ctx, "mem.work", i, event.Queue(qID))
			}
		}(q)
	}
	wg.Wait()

	// Release all and wait
	for _, qID := range queueIDs {
		event.QueueRelease(qID)
	}
	time.Sleep(1 * time.Second)

	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Use signed arithmetic to handle GC reclaiming memory between snapshots.
	heapDeltaMB := float64(int64(memAfter.HeapInuse)-int64(memBefore.HeapInuse)) / 1024 / 1024
	allocDeltaMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / 1024 / 1024

	t.Logf("=== Memory Stability ===")
	t.Logf("Events:          %d (%d queues × %d events)", totalEvents, numQueues, eventsPerQueue)
	t.Logf("HeapInuse delta: %.2f MB", heapDeltaMB)
	t.Logf("TotalAlloc:      %.2f MB", allocDeltaMB)
	t.Logf("Alloc/event:     %.0f bytes", allocDeltaMB*1024*1024/float64(totalEvents))

	// After drain, heap should not retain significant memory.
	// Allow generous 50 MB for 100k events (runtime overhead, GC timing).
	if heapDeltaMB > 50 {
		t.Errorf("heap grew %.2f MB after %d events, possible leak", heapDeltaMB, totalEvents)
	}
}
