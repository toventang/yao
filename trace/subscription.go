package trace

import (
	"fmt"
	"sync"

	"github.com/yaoapp/yao/event"
	eventTypes "github.com/yaoapp/yao/event/types"
	"github.com/yaoapp/yao/trace/types"
)

func dedupKey(u *types.TraceUpdate) string {
	return fmt.Sprintf("%s:%s:%d", u.Type, u.NodeID, u.Timestamp)
}

// Subscribe creates a new subscription for trace updates (replays all historical events from the beginning).
// Returns the update channel and a cancel function. The caller MUST call
// cancel when done (e.g., client disconnect) to release the goroutine.
func (m *manager) Subscribe() (<-chan *types.TraceUpdate, func(), error) {
	return m.subscribe(0)
}

// SubscribeFrom creates a subscription starting from a specific timestamp.
// Returns the update channel and a cancel function.
func (m *manager) SubscribeFrom(since int64) (<-chan *types.TraceUpdate, func(), error) {
	return m.subscribe(since)
}

// subscribe creates a subscription channel that first replays historical
// updates, then streams live events via the event service's Subscriber.
// The subscriber is registered BEFORE reading historical state to prevent
// missing events that occur between the state snapshot and subscriber setup.
//
// The returned cancel function triggers event.Unsubscribe which closes
// liveCh, causing the goroutine to exit via `for range liveCh`.
func (m *manager) subscribe(since int64) (<-chan *types.TraceUpdate, func(), error) {
	bufferSize := 1000

	out := make(chan *types.TraceUpdate, bufferSize)

	liveCh := make(chan *eventTypes.Event, bufferSize)
	traceID := m.traceID
	subID := event.Subscribe("trace.*", liveCh, event.Filter(func(ev *eventTypes.Event) bool {
		update, ok := ev.Payload.(*types.TraceUpdate)
		if !ok {
			return false
		}
		return update.TraceID == traceID
	}))

	historical := m.stateGetUpdates(since)

	histSeen := make(map[string]struct{}, len(historical))
	for _, u := range historical {
		histSeen[dedupKey(u)] = struct{}{}
	}

	var cancelOnce sync.Once
	cancel := func() {
		cancelOnce.Do(func() {
			event.Unsubscribe(subID)
		})
	}

	go func() {
		defer close(out)
		defer cancel()

		for _, update := range historical {
			out <- update
		}

		for ev := range liveCh {
			update, ok := ev.Payload.(*types.TraceUpdate)
			if !ok {
				continue
			}
			key := dedupKey(update)
			if _, dup := histSeen[key]; dup {
				delete(histSeen, key)
				continue
			}
			out <- update
			if update.Type == types.UpdateTypeComplete {
				return
			}
		}
	}()

	return out, cancel, nil
}
