package trace_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/trace"
	"github.com/yaoapp/yao/trace/types"
)

// stableGoroutines waits for runtime to settle and returns goroutine count.
func stableGoroutines() int {
	for i := 0; i < 5; i++ {
		runtime.GC()
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
	return runtime.NumGoroutine()
}

// TestLeak_SubscriptionClientDisconnect reproduces the goroutine leak that
// occurs when an SSE client subscribes to a trace and then disconnects
// without the trace ever completing (no UpdateTypeComplete sent).
//
// The subscription goroutine in subscription.go blocks on
// `for ev := range liveCh` and never exits because:
//  1. liveCh is never closed (event.Unsubscribe only deletes the map entry)
//  2. The goroutine only returns on UpdateTypeComplete
//  3. No context/cancellation mechanism exists
//
// This simulates the real-world scenario: SSE handler returns on client
// disconnect, but the subscription goroutine keeps running forever.
func TestLeak_SubscriptionClientDisconnect(t *testing.T) {
	drivers := trace.GetTestDrivers()

	for _, d := range drivers {
		t.Run(d.Name, func(t *testing.T) {
			ctx := context.Background()

			before := stableGoroutines()

			const numClients = 10

			for i := 0; i < numClients; i++ {
				traceID, manager, err := trace.New(ctx, d.DriverType, nil, d.DriverOptions...)
				assert.NoError(t, err)

				// Client subscribes (like SSE handler calling manager.Subscribe())
				updates, cancel, err := manager.Subscribe()
				assert.NoError(t, err)
				assert.NotNil(t, updates)

				// Simulate some trace activity
				_, err = manager.Add("step", types.TraceNodeOption{Label: "Processing"})
				assert.NoError(t, err)

				// Read a couple of events (like the SSE handler would)
				timeout := time.After(500 * time.Millisecond)
			drain:
				for {
					select {
					case _, ok := <-updates:
						if !ok {
							break drain
						}
					case <-timeout:
						break drain
					}
				}

				// Client disconnects: SSE handler calls cancel (deferred).
				// This triggers event.Unsubscribe which closes liveCh,
				// allowing the subscription goroutine to exit.
				cancel()

				trace.Release(traceID)
			}

			// Wait for goroutines to settle
			time.Sleep(1 * time.Second)
			after := stableGoroutines()

			leaked := after - before
			t.Logf("goroutines: before=%d after=%d leaked=%d (over %d simulated client disconnects)", before, after, leaked, numClients)

			// Each Subscribe() spawns a goroutine that should eventually exit.
			// If it doesn't, we'll see roughly numClients leaked goroutines.
			if leaked >= numClients {
				t.Errorf("goroutine leak detected: %d goroutines leaked after %d client disconnects. "+
					"Subscription goroutines are not cleaned up when clients disconnect without trace completion.",
					leaked, numClients)
			}
		})
	}
}

// TestLeak_SubscriptionEventServiceStop reproduces the goroutine leak when
// event.Stop() is called (e.g., during shutdown) while subscriptions are active.
//
// event.Stop() calls smgr.clear() which deletes all subscriber entries but
// does NOT close their channels, leaving goroutines blocked on `range liveCh`.
func TestLeak_SubscriptionEventServiceStop(t *testing.T) {
	drivers := trace.GetTestDrivers()

	for _, d := range drivers {
		t.Run(d.Name, func(t *testing.T) {
			ctx := context.Background()

			before := stableGoroutines()

			const numSubs = 5

			traceID, manager, err := trace.New(ctx, d.DriverType, nil, d.DriverOptions...)
			assert.NoError(t, err)

			// Create multiple subscriptions (simulating multiple SSE clients)
			for i := 0; i < numSubs; i++ {
				_, _, err := manager.Subscribe()
				assert.NoError(t, err)
			}

			// Simulate some activity
			_, err = manager.Add("work", types.TraceNodeOption{Label: "Working"})
			assert.NoError(t, err)

			time.Sleep(200 * time.Millisecond)

			// Stop event service (like during server shutdown)
			err = event.Stop(ctx)
			assert.NoError(t, err)

			// Restart event service for other tests
			err = event.Start()
			if err != nil && err != event.ErrAlreadyStart {
				t.Fatalf("Failed to restart event service: %v", err)
			}

			trace.Release(traceID)

			time.Sleep(1 * time.Second)
			after := stableGoroutines()

			leaked := after - before
			t.Logf("goroutines: before=%d after=%d leaked=%d (over %d subscriptions + event.Stop)", before, after, leaked, numSubs)

			if leaked >= numSubs {
				t.Errorf("goroutine leak detected: %d goroutines leaked after event.Stop() with %d active subscriptions. "+
					"smgr.clear() does not close subscriber channels.",
					leaked, numSubs)
			}
		})
	}
}
