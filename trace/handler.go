package trace

import (
	"context"

	"github.com/yaoapp/yao/event"
	eventTypes "github.com/yaoapp/yao/event/types"
)

// traceHandler processes trace events dispatched through the event service.
// It enables event.Push routing for trace.* events (used by addUpdateAndBroadcast).
type traceHandler struct{}

func (h *traceHandler) Handle(ctx context.Context, ev *eventTypes.Event, resp chan<- eventTypes.Result) {
	resp <- eventTypes.Result{}
}

func (h *traceHandler) Shutdown(ctx context.Context) error {
	return nil
}

func init() {
	event.Register("trace", &traceHandler{},
		event.MaxWorkers(256),
		event.ReservedWorkers(32),
		event.QueueSize(4096),
	)
}
