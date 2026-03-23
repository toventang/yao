package event

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/yaoapp/yao/event/types"
)

var eventIDCounter atomic.Uint64

func nextEventID() string {
	id := eventIDCounter.Add(1)
	return fmt.Sprintf("ev-%d", id)
}

// prefixOf extracts the handler prefix from an event type.
// "trace.add" -> "trace", "job.progress" -> "job"
func prefixOf(typ string) string {
	if i := strings.IndexByte(typ, '.'); i >= 0 {
		return typ[:i]
	}
	return typ
}

// Push delivers an event asynchronously (fire-and-forget).
// SID and Auth are extracted from ctx automatically.
// Returns the auto-generated event ID.
func Push(ctx context.Context, typ string, payload any, opts ...types.PushOption) (string, error) {
	prefix := prefixOf(typ)
	entry, pool, err := getHandler(prefix)
	if err != nil {
		return "", err
	}
	_ = entry // used for queue config lookup

	ev := &types.Event{
		Type:    typ,
		ID:      nextEventID(),
		IsCall:  false,
		Payload: payload,
		SID:     SIDFrom(ctx),
		Auth:    AuthFrom(ctx),
	}
	for _, opt := range opts {
		opt(ev)
	}

	// Notify listeners and subscribers (non-blocking, before handler)
	svc.lmgr.notify(ev)
	svc.smgr.notify(ev)

	// Route to queue or direct dispatch
	if ev.Queue != "" {
		q, err := svc.queues.get(ev.Queue)
		if err != nil {
			return ev.ID, err
		}
		discard := make(chan types.Result, 1)
		if err := q.enqueue(ctx, ev, discard); err != nil {
			return ev.ID, err
		}
		return ev.ID, nil
	}

	// No queue: direct dispatch with discard channel
	discard := make(chan types.Result, 1)
	pushCtx := context.WithoutCancel(ctx)
	if _, err := pool.dispatch(pushCtx, ev, discard); err != nil {
		return ev.ID, fmt.Errorf("event push: worker unavailable: %w", err)
	}
	return ev.ID, nil
}

// Call delivers an event synchronously and blocks until the handler responds.
// SID and Auth are extracted from ctx automatically.
// Returns the auto-generated event ID and the handler's result.
func Call(ctx context.Context, typ string, payload any, opts ...types.PushOption) (string, any, error) {
	prefix := prefixOf(typ)
	_, pool, err := getHandler(prefix)
	if err != nil {
		return "", nil, err
	}

	ev := &types.Event{
		Type:    typ,
		ID:      nextEventID(),
		IsCall:  true,
		Payload: payload,
		SID:     SIDFrom(ctx),
		Auth:    AuthFrom(ctx),
	}
	for _, opt := range opts {
		opt(ev)
	}

	// Notify listeners and subscribers
	svc.lmgr.notify(ev)
	svc.smgr.notify(ev)

	resp := make(chan types.Result, 1)

	if ev.Queue != "" {
		q, err := svc.queues.get(ev.Queue)
		if err != nil {
			return ev.ID, nil, err
		}
		if err := q.enqueue(ctx, ev, resp); err != nil {
			return ev.ID, nil, err
		}
	} else {
		if _, err := pool.dispatch(ctx, ev, resp); err != nil {
			return ev.ID, nil, fmt.Errorf("event call: worker unavailable: %w", err)
		}
	}

	// Wait for handler result or context cancellation
	select {
	case result := <-resp:
		return ev.ID, result.Data, result.Err
	case <-ctx.Done():
		return ev.ID, nil, ctx.Err()
	}
}

// QueueCreate creates a new event queue bound to a handler prefix.
// Returns the queue ID. If no id is provided, one is auto-generated.
func QueueCreate(prefix string, id ...string) (string, error) {
	entry, pool, err := getHandler(prefix)
	if err != nil {
		return "", err
	}

	queueID := ""
	if len(id) > 0 && id[0] != "" {
		queueID = id[0]
	} else {
		queueID = fmt.Sprintf("q-%s-%d", prefix, eventIDCounter.Add(1))
	}

	if err := svc.queues.create(prefix, queueID, entry.QueueSize, pool); err != nil {
		return "", err
	}
	return queueID, nil
}

// QueueRelease gracefully releases a queue (async).
// Rejects new events immediately; existing events are drained internally.
func QueueRelease(queueID string) {
	svc.queues.release(queueID)
}

// QueueAbort forcefully releases a queue (async).
// Rejects new events, discards pending events, waits for in-flight to finish.
func QueueAbort(queueID string) {
	svc.queues.abortOne(queueID)
}
