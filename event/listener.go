package event

import (
	"context"
	"strings"
	"sync"

	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/event/types"
)

// listenerEntry holds a registered listener with its filter configuration.
type listenerEntry struct {
	pattern    string
	listener   types.Listener
	filter     func(*types.Event) bool
	bufferSize int
	ch         chan *types.Event
	done       chan struct{}
}

// listenerManager manages all registered listeners.
type listenerManager struct {
	mu      sync.RWMutex
	entries []*listenerEntry
	started bool
}

func newListenerManager() *listenerManager {
	return &listenerManager{}
}

// register adds a listener. Must be called before start().
func (lm *listenerManager) register(pattern string, listener types.Listener, opts ...types.FilterOption) {
	fe := &types.FilterEntry{
		Pattern:    pattern,
		BufferSize: types.DefaultBufferSize,
	}
	for _, opt := range opts {
		opt(fe)
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.entries = append(lm.entries, &listenerEntry{
		pattern:    pattern,
		listener:   listener,
		filter:     fe.Filter,
		bufferSize: fe.BufferSize,
	})
}

// start creates channels and goroutines for each listener.
func (lm *listenerManager) start() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, entry := range lm.entries {
		entry.ch = make(chan *types.Event, entry.bufferSize)
		entry.done = make(chan struct{})
		go lm.consume(entry)
	}
	lm.started = true
}

// consume is the goroutine that reads from a listener's channel.
func (lm *listenerManager) consume(entry *listenerEntry) {
	defer close(entry.done)
	for ev := range entry.ch {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("event listener panic: pattern=%s type=%s err=%v", entry.pattern, ev.Type, r)
				}
			}()
			entry.listener.OnEvent(ev)
		}()
	}
}

// notify sends an event to all matching listeners (non-blocking).
func (lm *listenerManager) notify(ev *types.Event) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if !lm.started {
		return
	}

	for _, entry := range lm.entries {
		if !matchPattern(entry.pattern, ev.Type) {
			continue
		}
		if entry.filter != nil && !entry.filter(ev) {
			continue
		}
		select {
		case entry.ch <- ev:
		default:
			log.Warn("event listener buffer full: pattern=%s type=%s id=%s (skipped)", entry.pattern, ev.Type, ev.ID)
		}
	}
}

// stop shuts down all listeners.
func (lm *listenerManager) stop(ctx context.Context) {
	lm.mu.Lock()
	lm.started = false
	entries := lm.entries
	lm.mu.Unlock()

	for _, entry := range entries {
		close(entry.ch)
	}
	for _, entry := range entries {
		<-entry.done
		_ = entry.listener.Shutdown(ctx)
	}
}

// matchPattern matches an event type against a listener/subscriber pattern.
//   - "*" matches everything
//   - "foo.*" matches any type starting with "foo."
//   - "foo.bar" matches exactly "foo.bar"
func matchPattern(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(eventType, prefix)
	}
	return pattern == eventType
}

// Listen registers a persistent listener. Must be called before Start.
func Listen(pattern string, listener types.Listener, opts ...types.FilterOption) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.lmgr.register(pattern, listener, opts...)
}
