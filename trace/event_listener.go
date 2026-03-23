package trace

import (
	"context"

	"github.com/yaoapp/yao/event"
	eventTypes "github.com/yaoapp/yao/event/types"
)

// traceUpdateListener receives trace update events for cross-cutting concerns
// (e.g., audit logging, metrics). Trace updates are broadcast via event.Push
// and delivered to this listener and any dynamic subscribers.
type traceUpdateListener struct{}

func (l *traceUpdateListener) OnEvent(ev *eventTypes.Event) {}

func (l *traceUpdateListener) Shutdown(ctx context.Context) error {
	return nil
}

func init() {
	event.Listen("trace.*", &traceUpdateListener{},
		event.BufferSize(4096),
	)
}
