package event

import (
	"context"
	"sync"

	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/event/types"
)

// workerPool manages goroutine-based workers for a single Handler.
// Workers are fire-and-forget: each goroutine processes one task then exits.
// MaxWorkers limits total concurrent goroutines.
// ReservedWorkers reserves slots for Call events so Push cannot starve them.
type workerPool struct {
	handler types.Handler

	// semTotal is a buffered channel of size MaxWorkers.
	semTotal chan struct{}

	// semPush is a buffered channel of size (MaxWorkers - ReservedWorkers).
	// Push events must acquire from both semPush and semTotal.
	// Call events only acquire from semTotal.
	semPush chan struct{}

	wg sync.WaitGroup
}

func newWorkerPool(entry *types.HandlerEntry) *workerPool {
	pushSlots := entry.MaxWorkers - entry.ReservedWorkers
	if pushSlots < 1 {
		pushSlots = 1
	}
	return &workerPool{
		handler:  entry.Handler,
		semTotal: make(chan struct{}, entry.MaxWorkers),
		semPush:  make(chan struct{}, pushSlots),
	}
}

// dispatch runs the handler for one event in a new goroutine.
// Returns a done channel that is closed when the handler finishes.
// Blocks until a worker slot is available or ctx is cancelled.
func (wp *workerPool) dispatch(ctx context.Context, ev *types.Event, resp chan<- types.Result) (done <-chan struct{}, err error) {
	isPush := !ev.IsCall

	if isPush {
		select {
		case wp.semPush <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	select {
	case wp.semTotal <- struct{}{}:
	case <-ctx.Done():
		if isPush {
			<-wp.semPush
		}
		return nil, ctx.Err()
	}

	ch := make(chan struct{})
	wp.wg.Add(1)
	go func() {
		defer close(ch)
		defer wp.wg.Done()
		defer func() { <-wp.semTotal }()
		if isPush {
			defer func() { <-wp.semPush }()
		}
		defer wp.recoverPanic(ev, resp)

		wp.handler.Handle(ctx, ev, resp)
	}()

	return ch, nil
}

func (wp *workerPool) recoverPanic(ev *types.Event, resp chan<- types.Result) {
	if r := recover(); r != nil {
		log.Error("event worker panic: type=%s id=%s err=%v", ev.Type, ev.ID, r)
		select {
		case resp <- types.Result{Err: ErrHandlerPanic}:
		default:
		}
	}
}

// wait blocks until all active workers finish. Used during Stop.
func (wp *workerPool) wait() {
	wp.wg.Wait()
}
