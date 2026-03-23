# event — Yao In-Process Event Bus

Global event service for async/sync event routing, serial queue processing, and real-time subscriptions. All operations are goroutine-safe.

## Import

```go
import (
    "github.com/yaoapp/yao/event"
    "github.com/yaoapp/yao/event/types"
)
```

## Core Concepts

| Concept | Description |
|---|---|
| **Push** | Async fire-and-forget delivery. Returns event ID immediately. |
| **Call** | Sync request-response. Blocks until handler writes to `resp`. |
| **Handler** | One per prefix (e.g. `"trace"`). Processes `Push` and `Call` events. |
| **Queue** | FIFO serial processing per entity (e.g. per traceID). Events in same queue never run concurrently. |
| **Listener** | Persistent background consumer (registered at startup). Gets a copy of every matching event. |
| **Subscriber** | Dynamic subscription (e.g. SSE/WebSocket). Non-blocking; skips if channel full. |

## Lifecycle

```go
// 1. Register handlers and listeners (before Start, typically in init())
event.Register("trace", traceHandler, event.MaxWorkers(512), event.ReservedWorkers(20))
event.Register("job", jobHandler)
event.Listen("trace.*", traceListener)

// 2. Start
event.Start()

// 3. Use (from any goroutine)
event.Push(ctx, "trace.add", payload, event.Queue(traceQueueID))
id, data, err := event.Call(ctx, "trace.get", req, event.Queue(traceQueueID))

// 4. Stop (during shutdown)
event.Stop(ctx)
```

## Handler

Implement `types.Handler`:

```go
type TraceHandler struct{}

func (h *TraceHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
    var p TracePayload
    if err := ev.Should(&p); err != nil {
        if ev.IsCall { resp <- types.Result{Err: err} }
        return
    }
    // ... business logic ...
    if ev.IsCall {
        resp <- types.Result{Data: result}
    }
}

func (h *TraceHandler) Shutdown(ctx context.Context) error { return nil }
```

- `ctx`: non-cancellable for Push; caller's context for Call.
- `resp`: always non-nil. Write exactly once for Call; ignore for Push.
- `ev.Should(&target)`: type-safe payload extraction.
- Panics are recovered automatically; `ErrHandlerPanic` is returned to Call.

## Queue

```go
queueID, err := event.QueueCreate("trace")           // auto-generated ID
queueID, err := event.QueueCreate("trace", "my-id")  // custom ID

event.Push(ctx, "trace.add", data, event.Queue(queueID))    // serial
event.Call(ctx, "trace.get", req, event.Queue(queueID))      // serial, same queue

event.QueueRelease(queueID)  // graceful: drain pending, reject new
event.QueueAbort(queueID)    // forceful: discard pending, reject new
```

## Listener

Implement `types.Listener`:

```go
type MailListener struct{}
func (l *MailListener) OnEvent(ev *types.Event) { /* ... */ }
func (l *MailListener) Shutdown(ctx context.Context) error { return nil }

// Register before Start
event.Listen("mail.*", &MailListener{}, event.Filter(fn), event.BufferSize(4096))
```

- Each listener runs in its own goroutine.
- Non-blocking: if buffer full, event is skipped (logged as warning).

## Subscriber

```go
ch := make(chan *types.Event, 256)
subID := event.Subscribe("trace.*", ch, event.Filter(fn))
defer event.Unsubscribe(subID)

for ev := range ch {
    // push to SSE / WebSocket
}
```

- Non-blocking: if `ch` full, event is skipped silently.
- Call `Unsubscribe` when client disconnects.

## Context Propagation

```go
ctx = event.WithSID(ctx, sessionID)
ctx = event.WithAuth(ctx, &types.AuthorizedInfo{UserID: "u-1"})

// Inside handler:
sid := ev.SID
auth := ev.Auth  // may be nil
```

SID and Auth are extracted from `ctx` automatically when calling `Push`/`Call`.

## Pattern Matching

Used by `Listen` and `Subscribe`:

| Pattern | Matches |
|---|---|
| `"*"` | Everything |
| `"trace.*"` | `"trace.add"`, `"trace.get"`, etc. |
| `"trace.add"` | Exact match only |

## Handler Options

| Option | Default | Description |
|---|---|---|
| `MaxWorkers(n)` | 512 | Max concurrent goroutines for this handler |
| `ReservedWorkers(n)` | 10 | Slots reserved for Call (Push can use Max−Reserved) |
| `QueueSize(n)` | 8192 | Per-queue buffered channel capacity |

## Errors

| Error | When |
|---|---|
| `ErrNotStarted` | Push/Call before Start or after Stop |
| `ErrNoHandler` | No handler registered for event prefix |
| `ErrQueueFull` | Queue buffer at capacity |
| `ErrQueueNotFound` | Queue ID never created |
| `ErrQueueReleased` | Queue already released/aborted |
| `ErrQueueExists` | QueueCreate with duplicate ID |
| `ErrHandlerPanic` | Handler panicked (recovered) |

## Performance (M2 Max, 12 cores)

| Metric | Value |
|---|---|
| Push (no queue) | ~860K ops/sec, 456 B/op |
| Call (no queue) | ~1.2M ops/sec, 440 B/op |
| Push (with queue) | ~2.9M ops/sec, 341 B/op |
| 1000-user scenario (2000 queues, 27K events) | ~100K events/sec, 280ms total |
| Steady-state memory (1000 users) | ~27 MB |
| Goroutine leaks | Zero |

## File Structure

```
event/
├── types/
│   ├── types.go        # Event, Result, HandlerEntry, FilterEntry, options
│   └── interfaces.go   # Handler, Listener interfaces
├── service.go          # Register, Start, Stop, Reload, global state
├── bus.go              # Push, Call, QueueCreate/Release/Abort
├── queue.go            # FIFO queue + queue manager
├── worker.go           # Worker pool (two-tier semaphore)
├── listener.go         # Listener manager + pattern matching
├── sub.go              # Subscriber manager
├── option.go           # Option functions
└── README.md
```
