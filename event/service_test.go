package event_test

import (
	"context"
	"testing"

	"github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/event/types"
)

// stubHandler is a minimal Handler for testing registration and lifecycle.
type stubHandler struct {
	shutdownCalled bool
}

func (h *stubHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {}

func (h *stubHandler) Shutdown(ctx context.Context) error {
	h.shutdownCalled = true
	return nil
}

// --- Register + Start/Stop lifecycle ---

func TestStartStop_Basic(t *testing.T) {
	event.Reset()
	defer event.Reset()

	if event.IsStarted() {
		t.Fatal("service should not be started initially")
	}

	if err := event.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !event.IsStarted() {
		t.Fatal("service should be started after Start")
	}

	if err := event.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if event.IsStarted() {
		t.Fatal("service should not be started after Stop")
	}
}

func TestStart_Double(t *testing.T) {
	event.Reset()
	defer event.Reset()

	if err := event.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err := event.Start()
	if err != event.ErrAlreadyStart {
		t.Fatalf("expected ErrAlreadyStart, got: %v", err)
	}

	_ = event.Stop(context.Background())
}

func TestStop_WhenNotStarted(t *testing.T) {
	event.Reset()
	defer event.Reset()

	if err := event.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on non-started service should succeed, got: %v", err)
	}
}

func TestReload_WhenNotStarted(t *testing.T) {
	event.Reset()
	defer event.Reset()

	err := event.Reload()
	if err != event.ErrNotStarted {
		t.Fatalf("expected ErrNotStarted, got: %v", err)
	}
}

func TestReload_WhenStarted(t *testing.T) {
	event.Reset()
	defer event.Reset()

	_ = event.Start()
	if err := event.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	_ = event.Stop(context.Background())
}

// --- Register + options ---

func TestRegister_DefaultOptions(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &stubHandler{}
	event.Register("test", h)

	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	if !event.IsStarted() {
		t.Fatal("service should be started")
	}
}

func TestRegister_CustomOptions(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &stubHandler{}
	event.Register("test", h,
		event.MaxWorkers(128),
		event.ReservedWorkers(5),
		event.QueueSize(2048),
	)

	_ = event.Start()
	defer func() { _ = event.Stop(context.Background()) }()

	if !event.IsStarted() {
		t.Fatal("service should be started")
	}
}

// --- Stop calls Shutdown on handlers ---

func TestStop_CallsHandlerShutdown(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h := &stubHandler{}
	event.Register("test", h)
	_ = event.Start()

	if err := event.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if !h.shutdownCalled {
		t.Fatal("Handler.Shutdown should have been called on Stop")
	}
}

func TestStop_MultipleHandlersShutdown(t *testing.T) {
	event.Reset()
	defer event.Reset()

	h1 := &stubHandler{}
	h2 := &stubHandler{}
	event.Register("alpha", h1)
	event.Register("bravo", h2)
	_ = event.Start()

	if err := event.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if !h1.shutdownCalled || !h2.shutdownCalled {
		t.Fatal("all handlers should have been shut down")
	}
}

// --- Context SID/Auth propagation ---

func TestWithSID_SIDFrom(t *testing.T) {
	ctx := event.WithSID(context.Background(), "sess-123")
	got := event.SIDFrom(ctx)
	if got != "sess-123" {
		t.Fatalf("expected sess-123, got %s", got)
	}
}

func TestSIDFrom_Empty(t *testing.T) {
	got := event.SIDFrom(context.Background())
	if got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestWithAuth_AuthFrom(t *testing.T) {
	auth := &types.AuthorizedInfo{UserID: "u-1", TeamID: "t-1"}
	ctx := event.WithAuth(context.Background(), auth)
	got := event.AuthFrom(ctx)
	if got == nil {
		t.Fatal("expected non-nil auth")
	}
	if got.UserID != "u-1" || got.TeamID != "t-1" {
		t.Fatalf("unexpected auth: %+v", got)
	}
}

func TestAuthFrom_Nil(t *testing.T) {
	got := event.AuthFrom(context.Background())
	if got != nil {
		t.Fatal("expected nil auth from bare context")
	}
}

func TestWithSIDAndAuth_Combined(t *testing.T) {
	auth := &types.AuthorizedInfo{UserID: "u-2"}
	ctx := event.WithSID(context.Background(), "sess-456")
	ctx = event.WithAuth(ctx, auth)

	if event.SIDFrom(ctx) != "sess-456" {
		t.Fatal("SID mismatch")
	}
	if event.AuthFrom(ctx).UserID != "u-2" {
		t.Fatal("Auth mismatch")
	}
}

// --- Reset ---

func TestReset_ClearsState(t *testing.T) {
	event.Reset()

	h := &stubHandler{}
	event.Register("test", h)
	_ = event.Start()

	event.Reset()

	if event.IsStarted() {
		t.Fatal("service should not be started after Reset")
	}
}
