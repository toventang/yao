package event_test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/event/types"
)

// ---------------------------------------------------------------------------
// Shared bench handler: lightweight, simulates minimal real work.
// ---------------------------------------------------------------------------

type benchHandler struct {
	processed atomic.Int64
}

func (h *benchHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	h.processed.Add(1)
	if ev.IsCall {
		resp <- types.Result{Data: "ok"}
	}
}

func (h *benchHandler) Shutdown(ctx context.Context) error { return nil }

// benchListener counts received events.
type benchListener struct {
	received atomic.Int64
}

func (l *benchListener) OnEvent(ev *types.Event)            { l.received.Add(1) }
func (l *benchListener) Shutdown(ctx context.Context) error { return nil }

// ---------------------------------------------------------------------------
// Benchmark: Push throughput (no queue, pure worker dispatch)
// ---------------------------------------------------------------------------

func BenchmarkPush_NoQueue(b *testing.B) {
	event.Reset()
	h := &benchHandler{}
	event.Register("bench", h, event.MaxWorkers(512))
	_ = event.Start()
	b.Cleanup(func() { _ = event.Stop(context.Background()); event.Reset() })

	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = event.Push(ctx, "bench.work", nil)
		}
	})
	b.StopTimer()

	// Drain workers
	time.Sleep(100 * time.Millisecond)
	b.ReportMetric(float64(h.processed.Load()), "events_handled")
}

// ---------------------------------------------------------------------------
// Benchmark: Call throughput (no queue, synchronous round-trip)
// ---------------------------------------------------------------------------

func BenchmarkCall_NoQueue(b *testing.B) {
	event.Reset()
	h := &benchHandler{}
	event.Register("bench", h, event.MaxWorkers(512))
	_ = event.Start()
	b.Cleanup(func() { _ = event.Stop(context.Background()); event.Reset() })

	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = event.Call(ctx, "bench.get", nil)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: Push throughput with Queue (serial per queue)
// ---------------------------------------------------------------------------

func BenchmarkPush_WithQueue(b *testing.B) {
	event.Reset()
	h := &benchHandler{}
	event.Register("bench", h, event.MaxWorkers(512), event.QueueSize(8192))
	_ = event.Start()
	b.Cleanup(func() { _ = event.Stop(context.Background()); event.Reset() })

	qID, _ := event.QueueCreate("bench")
	b.Cleanup(func() { event.QueueRelease(qID) })

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = event.Push(ctx, "bench.work", nil, event.Queue(qID))
	}
	b.StopTimer()

	event.QueueRelease(qID)
	time.Sleep(200 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Scenario: 1000 concurrent users, each with trace + job queues.
//
// Simulates:
//   - 1000 users × 2 queues (trace + job) = 2000 queues
//   - Each user pushes 20 trace events + 5 job events + 1 Call per queue
//   - 200 SSE subscribers watching "trace.*" and "job.*"
//   - 2 Listeners (trace.* + job.*)
//
// Reports: total duration, events/sec, memory delta.
// ---------------------------------------------------------------------------

func TestScenario_1000Users(t *testing.T) {
	event.Reset()
	defer event.Reset()

	traceH := &benchHandler{}
	jobH := &benchHandler{}
	event.Register("trace", traceH, event.MaxWorkers(512), event.ReservedWorkers(20), event.QueueSize(8192))
	event.Register("job", jobH, event.MaxWorkers(256), event.ReservedWorkers(10), event.QueueSize(4096))

	traceL := &benchListener{}
	jobL := &benchListener{}
	event.Listen("trace.*", traceL)
	event.Listen("job.*", jobL)

	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	const (
		numUsers          = 1000
		tracePushPerUser  = 20
		jobPushPerUser    = 5
		callsPerQueue     = 1
		numSubscribers    = 200
		subscriberBufSize = 256
	)

	// --- Subscribers ---
	subChans := make([]chan *types.Event, numSubscribers)
	subIDs := make([]string, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		ch := make(chan *types.Event, subscriberBufSize)
		subChans[i] = ch
		pattern := "trace.*"
		if i%2 == 1 {
			pattern = "job.*"
		}
		subIDs[i] = event.Subscribe(pattern, ch)
	}
	defer func() {
		for _, id := range subIDs {
			event.Unsubscribe(id)
		}
	}()

	// Drain subscribers in background
	var subReceived atomic.Int64
	subDone := make(chan struct{})
	go func() {
		defer close(subDone)
		for _, ch := range subChans {
			go func(c chan *types.Event) {
				for range c {
					subReceived.Add(1)
				}
			}(ch)
		}
	}()

	// --- Memory before ---
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// --- Run ---
	start := time.Now()
	var wg sync.WaitGroup

	for u := 0; u < numUsers; u++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			ctx := event.WithSID(context.Background(), fmt.Sprintf("sess-%d", userID))
			ctx = event.WithAuth(ctx, &types.AuthorizedInfo{UserID: fmt.Sprintf("u-%d", userID)})

			// Create trace queue
			traceQID, err := event.QueueCreate("trace")
			if err != nil {
				t.Errorf("user %d: trace QueueCreate: %v", userID, err)
				return
			}

			// Create job queue
			jobQID, err := event.QueueCreate("job")
			if err != nil {
				t.Errorf("user %d: job QueueCreate: %v", userID, err)
				event.QueueRelease(traceQID)
				return
			}

			// Push trace events
			for i := 0; i < tracePushPerUser; i++ {
				_, _ = event.Push(ctx, "trace.add", i, event.Queue(traceQID))
			}

			// Push job events
			for i := 0; i < jobPushPerUser; i++ {
				_, _ = event.Push(ctx, "job.progress", i, event.Queue(jobQID))
			}

			// Call on each queue
			for i := 0; i < callsPerQueue; i++ {
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, _, _ = event.Call(callCtx, "trace.get", nil, event.Queue(traceQID))
				cancel()

				callCtx2, cancel2 := context.WithTimeout(ctx, 5*time.Second)
				_, _, _ = event.Call(callCtx2, "job.status", nil, event.Queue(jobQID))
				cancel2()
			}

			// Release queues
			event.QueueRelease(traceQID)
			event.QueueRelease(jobQID)
		}(u)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Wait for queues to drain
	time.Sleep(500 * time.Millisecond)

	// --- Memory after ---
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// --- Results ---
	totalPush := int64(numUsers) * int64(tracePushPerUser+jobPushPerUser)
	totalCall := int64(numUsers) * int64(callsPerQueue) * 2
	totalEvents := totalPush + totalCall
	traceProcessed := traceH.processed.Load()
	jobProcessed := jobH.processed.Load()
	listenerTrace := traceL.received.Load()
	listenerJob := jobL.received.Load()
	memDeltaMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / 1024 / 1024

	t.Logf("=== 1000-User Scenario Results ===")
	t.Logf("Users:            %d", numUsers)
	t.Logf("Queues created:   %d (trace: %d, job: %d)", numUsers*2, numUsers, numUsers)
	t.Logf("Subscribers:      %d", numSubscribers)
	t.Logf("Total events:     %d (push: %d, call: %d)", totalEvents, totalPush, totalCall)
	t.Logf("Trace processed:  %d", traceProcessed)
	t.Logf("Job processed:    %d", jobProcessed)
	t.Logf("Listener trace:   %d", listenerTrace)
	t.Logf("Listener job:     %d", listenerJob)
	t.Logf("Sub received:     %d", subReceived.Load())
	t.Logf("Elapsed:          %v", elapsed)
	t.Logf("Throughput:       %.0f events/sec", float64(totalEvents)/elapsed.Seconds())
	t.Logf("Memory delta:     %.2f MB (TotalAlloc)", memDeltaMB)

	// --- Assertions ---
	expectedProcessed := totalPush + totalCall
	actualProcessed := traceProcessed + jobProcessed
	if actualProcessed < expectedProcessed {
		t.Errorf("processed %d < expected %d (some events lost)", actualProcessed, expectedProcessed)
	}

	if elapsed > 30*time.Second {
		t.Errorf("scenario took %v, expected < 30s", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Benchmark: Queue create/release churn (lifecycle overhead)
// ---------------------------------------------------------------------------

func BenchmarkQueueCreateRelease(b *testing.B) {
	event.Reset()
	h := &benchHandler{}
	event.Register("bench", h, event.QueueSize(64))
	_ = event.Start()
	b.Cleanup(func() { _ = event.Stop(context.Background()); event.Reset() })

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			qID, err := event.QueueCreate("bench")
			if err != nil {
				b.Fatalf("QueueCreate: %v", err)
			}
			event.QueueRelease(qID)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: Subscriber notify throughput (fanout to 200 subscribers)
// ---------------------------------------------------------------------------

func BenchmarkSubscriberFanout(b *testing.B) {
	event.Reset()
	h := &benchHandler{}
	event.Register("bench", h, event.MaxWorkers(512))
	_ = event.Start()
	b.Cleanup(func() { _ = event.Stop(context.Background()); event.Reset() })

	const numSubs = 200
	for i := 0; i < numSubs; i++ {
		ch := make(chan *types.Event, 1024)
		event.Subscribe("bench.*", ch)
		go func(c chan *types.Event) {
			for range c {
			}
		}(ch)
	}

	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = event.Push(ctx, "bench.work", nil)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: Mixed Push/Call with 2000 queues (1000 users × 2)
// ---------------------------------------------------------------------------

func BenchmarkMixed_2000Queues(b *testing.B) {
	event.Reset()
	h := &benchHandler{}
	event.Register("mix", h, event.MaxWorkers(512), event.ReservedWorkers(20), event.QueueSize(4096))
	_ = event.Start()
	b.Cleanup(func() { _ = event.Stop(context.Background()); event.Reset() })

	const numQueues = 2000
	queueIDs := make([]string, numQueues)
	for i := 0; i < numQueues; i++ {
		qID, err := event.QueueCreate("mix")
		if err != nil {
			b.Fatalf("QueueCreate %d: %v", i, err)
		}
		queueIDs[i] = qID
	}
	b.Cleanup(func() {
		for _, qID := range queueIDs {
			event.QueueRelease(qID)
		}
		time.Sleep(200 * time.Millisecond)
	})

	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			qID := queueIDs[i%numQueues]
			if i%10 == 0 {
				callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				_, _, _ = event.Call(callCtx, "mix.get", nil, event.Queue(qID))
				cancel()
			} else {
				_, _ = event.Push(ctx, "mix.work", nil, event.Queue(qID))
			}
			i++
		}
	})
}
