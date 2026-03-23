package events

import (
	"context"
	"net/http"
	"time"

	"github.com/yaoapp/yao/event"
	eventtypes "github.com/yaoapp/yao/event/types"
)

func init() {
	event.Register("robot", &robotHandler{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	})
}

// robotHandler processes all robot.* events.
type robotHandler struct {
	httpClient *http.Client
}

// Handle dispatches robot events by type.
func (h *robotHandler) Handle(ctx context.Context, ev *eventtypes.Event, resp chan<- eventtypes.Result) {
	switch ev.Type {
	case Delivery:
		h.handleDelivery(ctx, ev, resp)
	case Message:
		h.handleMessage(ctx, ev, resp)
	default:
		log.Debug("robot handler: unhandled event type=%s id=%s", ev.Type, ev.ID)
	}
}

// Shutdown gracefully shuts down the robot handler.
func (h *robotHandler) Shutdown(ctx context.Context) error {
	return nil
}
