package types

import (
	"fmt"
	"reflect"

	"github.com/yaoapp/gou/process"
)

// AuthorizedInfo is an alias for gou/process.AuthorizedInfo.
type AuthorizedInfo = process.AuthorizedInfo

// Event represents a single event in the event bus.
type Event struct {
	Type    string          // Event type, e.g. "trace.add", "job.progress"
	ID      string          // Auto-generated event ID
	Queue   string          // Queue key for serial processing; empty means no queue
	IsCall  bool            // true = synchronous Call, false = asynchronous Push
	Payload any             // Business data; concrete type is determined by event type
	SID     string          // Session ID, extracted from caller context
	Auth    *AuthorizedInfo // Authorized info, extracted from caller context; may be nil
}

// Should asserts the Payload to the target pointer type.
// target must be a non-nil pointer. Returns an error if the type does not match.
//
// Usage:
//
//	var p MyPayload
//	if err := ev.Should(&p); err != nil { ... }
func (ev *Event) Should(target any) error {
	if target == nil {
		return fmt.Errorf("event.Should: target must be a non-nil pointer")
	}

	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("event.Should: target must be a non-nil pointer, got %T", target)
	}

	if ev.Payload == nil {
		return fmt.Errorf("event.Should: payload is nil")
	}

	// Direct assignment: payload is already the expected pointer type
	payloadVal := reflect.ValueOf(ev.Payload)
	targetElem := rv.Elem()

	// If payload is a pointer, dereference it
	if payloadVal.Kind() == reflect.Ptr {
		if payloadVal.IsNil() {
			return fmt.Errorf("event.Should: payload is nil pointer")
		}
		payloadVal = payloadVal.Elem()
	}

	if !payloadVal.Type().AssignableTo(targetElem.Type()) {
		return fmt.Errorf("event.Should: payload type %T is not assignable to %s", ev.Payload, targetElem.Type())
	}

	targetElem.Set(payloadVal)
	return nil
}

// Result holds the response from a synchronous Call.
type Result struct {
	Data any
	Err  error
}

// HandlerOption configures a Handler registration.
type HandlerOption func(*HandlerEntry)

// HandlerEntry is the internal registration record for a Handler.
type HandlerEntry struct {
	Prefix          string
	Handler         Handler
	MaxWorkers      int // Max concurrent workers, default 512
	ReservedWorkers int // Workers reserved for Call, default 10
	QueueSize       int // Per-queue capacity, default 8192
}

// FilterOption configures a Listener or Subscriber registration.
type FilterOption func(*FilterEntry)

// FilterEntry is the internal registration record for a Listener/Subscriber.
type FilterEntry struct {
	Pattern    string
	Filter     func(*Event) bool // Custom filter function
	BufferSize int               // Listener chan buffer size, default 8192; only for Listen
}

// PushOption configures a Push or Call invocation.
type PushOption func(*Event)

// Default configuration values.
const (
	DefaultMaxWorkers      = 512
	DefaultReservedWorkers = 10
	DefaultQueueSize       = 8192
	DefaultBufferSize      = 8192
)
