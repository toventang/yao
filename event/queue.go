package event

import (
	"context"
	"sync"

	"github.com/yaoapp/yao/event/types"
)

// queueItem wraps an event with its execution context and response channel.
type queueItem struct {
	ctx  context.Context
	ev   *types.Event
	resp chan<- types.Result
}

// eventQueue is a single FIFO queue bound to a specific handler prefix.
// Events are enqueued and consumed serially by a dedicated goroutine.
type eventQueue struct {
	id       string
	prefix   string
	ch       chan queueItem
	released bool
	aborted  bool
	mu       sync.Mutex
	done     chan struct{} // closed when consumer goroutine exits
}

// enqueue adds an event to the queue. Returns error if full, released, or aborted.
// The send to q.ch is performed while holding q.mu to prevent a race with
// release()/abort() closing the channel between the flag check and the send.
func (q *eventQueue) enqueue(ctx context.Context, ev *types.Event, resp chan<- types.Result) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.released || q.aborted {
		return ErrQueueReleased
	}

	select {
	case q.ch <- queueItem{ctx: ctx, ev: ev, resp: resp}:
		return nil
	default:
		return ErrQueueFull
	}
}

// release gracefully stops the queue: rejects new events, drains existing ones.
func (q *eventQueue) release() {
	q.mu.Lock()
	if q.released || q.aborted {
		q.mu.Unlock()
		return
	}
	q.released = true
	close(q.ch)
	q.mu.Unlock()
}

// abort forcefully stops the queue: rejects new events, discards pending.
// The consumer goroutine detects the aborted flag and skips remaining items.
func (q *eventQueue) abort() {
	q.mu.Lock()
	if q.aborted {
		q.mu.Unlock()
		return
	}
	wasReleased := q.released
	q.aborted = true
	q.released = true
	if !wasReleased {
		close(q.ch)
	}
	q.mu.Unlock()
}

// consumer is the goroutine that processes queued events serially.
func (q *eventQueue) consumer(pool *workerPool) {
	defer close(q.done)
	for item := range q.ch {
		q.mu.Lock()
		aborted := q.aborted
		q.mu.Unlock()
		if aborted {
			continue
		}

		// For Push events, use a non-cancellable context so that queued
		// fire-and-forget events are not dropped when the caller's ctx expires.
		// For Call events, preserve the caller's ctx for deadline/cancellation.
		dispatchCtx := item.ctx
		if !item.ev.IsCall {
			dispatchCtx = context.WithoutCancel(item.ctx)
		}

		done, err := pool.dispatch(dispatchCtx, item.ev, item.resp)
		if err != nil {
			select {
			case item.resp <- types.Result{Err: err}:
			default:
			}
			continue
		}
		<-done
	}
}

// queueManager manages all active queues.
type queueManager struct {
	mu       sync.RWMutex
	queues   map[string]*eventQueue
	released map[string]struct{} // tracks IDs that have been released/aborted
}

func newQueueManager() *queueManager {
	return &queueManager{
		queues:   make(map[string]*eventQueue),
		released: make(map[string]struct{}),
	}
}

// create creates a new queue bound to a handler prefix.
func (qm *queueManager) create(prefix string, queueID string, queueSize int, pool *workerPool) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if _, exists := qm.queues[queueID]; exists {
		return ErrQueueExists
	}

	q := &eventQueue{
		id:     queueID,
		prefix: prefix,
		ch:     make(chan queueItem, queueSize),
		done:   make(chan struct{}),
	}
	qm.queues[queueID] = q
	go q.consumer(pool)
	return nil
}

// get returns a queue by ID.
// Returns ErrQueueNotFound if the queue was never created,
// or ErrQueueReleased if it has been released/aborted.
func (qm *queueManager) get(queueID string) (*eventQueue, error) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	q, ok := qm.queues[queueID]
	if !ok {
		if _, wasReleased := qm.released[queueID]; wasReleased {
			return nil, ErrQueueReleased
		}
		return nil, ErrQueueNotFound
	}
	return q, nil
}

// release gracefully releases a queue.
func (qm *queueManager) release(queueID string) {
	qm.mu.Lock()
	q, ok := qm.queues[queueID]
	if !ok {
		qm.mu.Unlock()
		return
	}
	delete(qm.queues, queueID)
	qm.released[queueID] = struct{}{}
	qm.mu.Unlock()

	q.release()
	go func() { <-q.done }()
}

// abortOne forcefully releases a single queue.
func (qm *queueManager) abortOne(queueID string) {
	qm.mu.Lock()
	q, ok := qm.queues[queueID]
	if !ok {
		qm.mu.Unlock()
		return
	}
	delete(qm.queues, queueID)
	qm.released[queueID] = struct{}{}
	qm.mu.Unlock()

	q.abort()
	go func() { <-q.done }()
}

// abortAll forcefully releases all queues. Used during Stop.
func (qm *queueManager) abortAll() {
	qm.mu.Lock()
	queues := make([]*eventQueue, 0, len(qm.queues))
	for id, q := range qm.queues {
		queues = append(queues, q)
		qm.released[id] = struct{}{}
	}
	qm.queues = make(map[string]*eventQueue)
	qm.mu.Unlock()

	for _, q := range queues {
		q.abort()
	}
	for _, q := range queues {
		<-q.done
	}
}
