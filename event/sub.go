package event

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/yaoapp/yao/event/types"
)

var subIDCounter atomic.Uint64

func nextSubID() string {
	id := subIDCounter.Add(1)
	return fmt.Sprintf("sub-%d", id)
}

// subEntry holds a dynamic subscriber registration.
type subEntry struct {
	id      string
	pattern string
	filter  func(*types.Event) bool
	ch      chan<- *types.Event
}

// subManager manages dynamic subscribers.
type subManager struct {
	mu      sync.RWMutex
	entries map[string]*subEntry // id -> entry
}

func newSubManager() *subManager {
	return &subManager{
		entries: make(map[string]*subEntry),
	}
}

// subscribe adds a dynamic subscriber. Returns the subscription ID.
func (sm *subManager) subscribe(pattern string, ch chan<- *types.Event, opts ...types.FilterOption) string {
	fe := &types.FilterEntry{Pattern: pattern}
	for _, opt := range opts {
		opt(fe)
	}

	id := nextSubID()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.entries[id] = &subEntry{
		id:      id,
		pattern: pattern,
		filter:  fe.Filter,
		ch:      ch,
	}
	return id
}

// unsubscribe removes a subscriber by ID and closes its channel
// so that any goroutine blocked on `range ch` will unblock and exit.
func (sm *subManager) unsubscribe(id string) {
	sm.mu.Lock()
	entry, ok := sm.entries[id]
	delete(sm.entries, id)
	sm.mu.Unlock()

	if ok && entry.ch != nil {
		func() {
			defer func() { recover() }()
			close(entry.ch)
		}()
	}
}

// notify sends an event to all matching subscribers (non-blocking).
// Recovers from send-on-closed-channel panics that may occur if
// unsubscribe closes a channel concurrently.
func (sm *subManager) notify(ev *types.Event) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, entry := range sm.entries {
		if !matchPattern(entry.pattern, ev.Type) {
			continue
		}
		if entry.filter != nil && !entry.filter(ev) {
			continue
		}
		func() {
			defer func() { recover() }()
			select {
			case entry.ch <- ev:
			default:
			}
		}()
	}
}

// clear removes all subscribers and closes their channels. Used during Stop.
func (sm *subManager) clear() {
	sm.mu.Lock()
	old := sm.entries
	sm.entries = make(map[string]*subEntry)
	sm.mu.Unlock()

	for _, entry := range old {
		if entry.ch != nil {
			func() {
				defer func() { recover() }()
				close(entry.ch)
			}()
		}
	}
}

// Subscribe dynamically subscribes to events matching the given pattern.
// Returns the subscription ID for later unsubscription.
// Event delivery is non-blocking: if ch is full, the event is skipped.
func Subscribe(pattern string, ch chan<- *types.Event, opts ...types.FilterOption) string {
	return svc.smgr.subscribe(pattern, ch, opts...)
}

// Unsubscribe removes a dynamic subscription by ID.
func Unsubscribe(id string) {
	svc.smgr.unsubscribe(id)
}
