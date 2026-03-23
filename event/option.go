package event

import "github.com/yaoapp/yao/event/types"

// MaxWorkers sets the max concurrent worker goroutines for a Handler.
// Default is 512. Workers are fire-and-forget (goroutine ends after task).
func MaxWorkers(n int) types.HandlerOption {
	return func(e *types.HandlerEntry) {
		e.MaxWorkers = n
	}
}

// ReservedWorkers sets the number of workers reserved for Call events.
// Default is 10. Push can use MaxWorkers - Reserved; Call can use MaxWorkers.
func ReservedWorkers(n int) types.HandlerOption {
	return func(e *types.HandlerEntry) {
		e.ReservedWorkers = n
	}
}

// QueueSize sets the per-queue capacity. Default is 8192.
// When a queue is full, Push/Call returns ErrQueueFull immediately.
func QueueSize(n int) types.HandlerOption {
	return func(e *types.HandlerEntry) {
		e.QueueSize = n
	}
}

// Queue sets the queue key for a Push/Call invocation.
// Events with the same queue key are processed serially (FIFO).
func Queue(key string) types.PushOption {
	return func(ev *types.Event) {
		ev.Queue = key
	}
}

// Filter sets a custom filter function for Listen or Subscribe.
// Events that do not pass the filter are skipped.
func Filter(fn func(*types.Event) bool) types.FilterOption {
	return func(e *types.FilterEntry) {
		e.Filter = fn
	}
}

// BufferSize sets the Listener channel buffer size. Default is 8192.
// Only effective for Listen; ignored by Subscribe.
func BufferSize(n int) types.FilterOption {
	return func(e *types.FilterEntry) {
		e.BufferSize = n
	}
}
