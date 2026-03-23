package event

import (
	"context"
	"errors"
	"sync"

	"github.com/yaoapp/yao/event/types"
)

// Sentinel errors.
var (
	ErrNotStarted    = errors.New("event: service not started")
	ErrAlreadyStart  = errors.New("event: service already started")
	ErrQueueFull     = errors.New("event: queue is full")
	ErrQueueNotFound = errors.New("event: queue not found")
	ErrQueueExists   = errors.New("event: queue already exists")
	ErrQueueReleased = errors.New("event: queue already released")
	ErrNoHandler     = errors.New("event: no handler registered for prefix")
	ErrHandlerPanic  = errors.New("event: handler panicked")
)

// Context keys for SID and Auth propagation.
type ctxKey int

const (
	ctxKeySID ctxKey = iota
	ctxKeyAuth
)

// WithSID returns a context carrying the given session ID.
func WithSID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, ctxKeySID, sid)
}

// SIDFrom extracts the session ID from ctx. Returns empty string if not set.
func SIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeySID).(string); ok {
		return v
	}
	return ""
}

// WithAuth returns a context carrying the given authorized info.
func WithAuth(ctx context.Context, auth *types.AuthorizedInfo) context.Context {
	return context.WithValue(ctx, ctxKeyAuth, auth)
}

// AuthFrom extracts the authorized info from ctx. Returns nil if not set.
func AuthFrom(ctx context.Context) *types.AuthorizedInfo {
	if v, ok := ctx.Value(ctxKeyAuth).(*types.AuthorizedInfo); ok {
		return v
	}
	return nil
}

// service holds all global state for the event bus.
type service struct {
	mu       sync.RWMutex
	started  bool
	handlers map[string]*types.HandlerEntry // prefix -> registration
	pools    map[string]*workerPool         // prefix -> worker pool
	queues   *queueManager                  // queue lifecycle
	lmgr     *listenerManager               // listener manager
	smgr     *subManager                    // subscriber manager
}

var svc = &service{}

func init() {
	svc.reset()
}

// Register registers a handler for the given prefix.
// Must be called before Start (typically in init()).
func Register(prefix string, handler types.Handler, opts ...types.HandlerOption) {
	entry := &types.HandlerEntry{
		Prefix:          prefix,
		Handler:         handler,
		MaxWorkers:      types.DefaultMaxWorkers,
		ReservedWorkers: types.DefaultReservedWorkers,
		QueueSize:       types.DefaultQueueSize,
	}
	for _, opt := range opts {
		opt(entry)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.handlers[prefix] = entry
}

// Start initializes and starts the event service.
// Called during engine startup, after runtime is ready.
func Start() error {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.started {
		return ErrAlreadyStart
	}

	// Create worker pools for each registered handler
	for prefix, entry := range svc.handlers {
		svc.pools[prefix] = newWorkerPool(entry)
	}

	// Start listener manager
	svc.lmgr.start()

	svc.started = true
	return nil
}

// Stop gracefully shuts down the event service.
// Waits for in-flight events to finish, discards pending queue items,
// and calls Shutdown on all handlers and listeners.
//
// The lock is released before waiting for workers so that in-flight handlers
// calling Push/Call (which acquire RLock via getHandler) do not deadlock.
// Once started=false, getHandler returns ErrNotStarted for any new calls.
func Stop(ctx context.Context) error {
	svc.mu.Lock()
	if !svc.started {
		svc.mu.Unlock()
		return nil
	}
	svc.started = false

	// Snapshot references under lock, then release.
	queues := svc.queues
	pools := make([]*workerPool, 0, len(svc.pools))
	for _, p := range svc.pools {
		pools = append(pools, p)
	}
	handlers := make([]*types.HandlerEntry, 0, len(svc.handlers))
	for _, e := range svc.handlers {
		handlers = append(handlers, e)
	}
	lmgr := svc.lmgr
	smgr := svc.smgr
	svc.mu.Unlock()

	// From here on, started=false prevents any new Push/Call/QueueCreate.
	// Existing in-flight workers may still call getHandler and get ErrNotStarted,
	// which is the correct behavior during shutdown.

	// Abort all queues (discard pending, wait for in-flight)
	queues.abortAll()

	// Wait for all worker pools to drain
	for _, pool := range pools {
		pool.wait()
	}

	// Shutdown all handlers
	for _, entry := range handlers {
		if entry.Handler != nil {
			_ = entry.Handler.Shutdown(ctx)
		}
	}

	// Stop listener manager
	lmgr.stop(ctx)

	// Clear subscribers
	smgr.clear()

	return nil
}

// Reload performs a hot-reload. Preserves queues and in-flight events,
// reloads dynamic configuration only.
func Reload() error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if !svc.started {
		return ErrNotStarted
	}
	return nil
}

// IsStarted reports whether the service is currently running.
func IsStarted() bool {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.started
}

// getHandler returns the handler entry and its worker pool for the given prefix.
func getHandler(prefix string) (*types.HandlerEntry, *workerPool, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if !svc.started {
		return nil, nil, ErrNotStarted
	}
	entry, ok := svc.handlers[prefix]
	if !ok {
		return nil, nil, ErrNoHandler
	}
	pool := svc.pools[prefix]
	return entry, pool, nil
}

// Reset clears all state. For testing only.
func Reset() {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.reset()
}

func (s *service) reset() {
	s.started = false
	s.handlers = make(map[string]*types.HandlerEntry)
	s.pools = make(map[string]*workerPool)
	s.queues = newQueueManager()
	s.lmgr = newListenerManager()
	s.smgr = newSubManager()
}
