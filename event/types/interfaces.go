package types

import "context"

// Handler processes events for a given prefix (registered at startup, one per prefix).
//
// Handle is invoked by the WorkerPool.
//   - ctx: for Call, this carries the caller's deadline/cancellation; for Push, a non-cancellable context.
//   - resp is always non-nil. For Push the framework passes a discard channel; for Call it waits for a read.
//     Use ev.IsCall to decide whether to write a meaningful result.
type Handler interface {
	Handle(ctx context.Context, ev *Event, resp chan<- Result)
	Shutdown(ctx context.Context) error
}

// Listener receives matched events in a dedicated goroutine (registered at startup).
//
// OnEvent is called in the Listener's own goroutine; it does not block other
// Listeners or Subscribers.
type Listener interface {
	OnEvent(ev *Event)
	Shutdown(ctx context.Context) error
}
