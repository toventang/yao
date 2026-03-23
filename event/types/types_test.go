package types_test

import (
	"fmt"
	"testing"

	"github.com/yaoapp/yao/event/types"
)

// samplePayload is a test-only struct with no business semantics.
type samplePayload struct {
	Name  string
	Value int
	Tags  []string
}

// --- Should: basic struct assignment ---

func TestShould_StructValue(t *testing.T) {
	ev := &types.Event{
		Payload: samplePayload{Name: "alpha", Value: 1, Tags: []string{"a", "b"}},
	}

	var got samplePayload
	if err := ev.Should(&got); err != nil {
		t.Fatalf("Should returned error: %v", err)
	}
	if got.Name != "alpha" || got.Value != 1 || len(got.Tags) != 2 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

// --- Should: pointer payload ---

func TestShould_PointerPayload(t *testing.T) {
	ev := &types.Event{
		Payload: &samplePayload{Name: "beta", Value: 2},
	}

	var got samplePayload
	if err := ev.Should(&got); err != nil {
		t.Fatalf("Should returned error: %v", err)
	}
	if got.Name != "beta" || got.Value != 2 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

// --- Should: primitive payloads ---

func TestShould_StringPayload(t *testing.T) {
	ev := &types.Event{
		Payload: "hello world",
	}

	var got string
	if err := ev.Should(&got); err != nil {
		t.Fatalf("Should returned error: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("unexpected string: %s", got)
	}
}

func TestShould_IntPayload(t *testing.T) {
	ev := &types.Event{
		Payload: 42,
	}

	var got int
	if err := ev.Should(&got); err != nil {
		t.Fatalf("Should returned error: %v", err)
	}
	if got != 42 {
		t.Fatalf("unexpected int: %d", got)
	}
}

// --- Should: error cases ---

func TestShould_NilTarget(t *testing.T) {
	ev := &types.Event{Payload: "data"}
	if err := ev.Should(nil); err == nil {
		t.Fatal("expected error for nil target")
	}
}

func TestShould_NonPointerTarget(t *testing.T) {
	ev := &types.Event{Payload: "data"}
	var s string
	if err := ev.Should(s); err == nil {
		t.Fatal("expected error for non-pointer target")
	}
}

func TestShould_NilPayload(t *testing.T) {
	ev := &types.Event{Payload: nil}
	var got string
	if err := ev.Should(&got); err == nil {
		t.Fatal("expected error for nil payload")
	}
}

func TestShould_NilPointerPayload(t *testing.T) {
	ev := &types.Event{Payload: (*samplePayload)(nil)}
	var got samplePayload
	if err := ev.Should(&got); err == nil {
		t.Fatal("expected error for nil pointer payload")
	}
}

func TestShould_TypeMismatch(t *testing.T) {
	ev := &types.Event{
		Payload: "wrong type",
	}
	var got samplePayload
	if err := ev.Should(&got); err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

// --- Event fields ---

func TestEvent_NilAuth(t *testing.T) {
	ev := &types.Event{
		Type: "x.y",
		ID:   "ev-100",
		Auth: nil,
	}
	if ev.Auth != nil {
		t.Fatal("Auth should be nil")
	}
	if ev.Type != "x.y" || ev.ID != "ev-100" {
		t.Fatalf("unexpected Type/ID: %s/%s", ev.Type, ev.ID)
	}
}

func TestEvent_WithAuth(t *testing.T) {
	ev := &types.Event{
		Type: "x.y",
		ID:   "ev-101",
		SID:  "sess-abc",
		Auth: &types.AuthorizedInfo{
			UserID: "u-1",
			TeamID: "t-1",
		},
	}
	if ev.Type != "x.y" || ev.ID != "ev-101" {
		t.Fatalf("unexpected Type/ID: %s/%s", ev.Type, ev.ID)
	}
	if ev.SID != "sess-abc" {
		t.Fatalf("unexpected SID: %s", ev.SID)
	}
	if ev.Auth.UserID != "u-1" || ev.Auth.TeamID != "t-1" {
		t.Fatalf("unexpected Auth: %+v", ev.Auth)
	}
}

func TestEvent_QueueAndIsCall(t *testing.T) {
	push := &types.Event{Queue: "q-1", IsCall: false}
	call := &types.Event{Queue: "q-1", IsCall: true}

	if push.IsCall {
		t.Fatal("Push event should not be IsCall")
	}
	if !call.IsCall {
		t.Fatal("Call event should be IsCall")
	}
	if push.Queue != "q-1" || call.Queue != "q-1" {
		t.Fatal("Queue key mismatch")
	}
}

// --- Result ---

func TestResult_Success(t *testing.T) {
	r := types.Result{Data: map[string]string{"k": "v"}, Err: nil}
	if r.Err != nil {
		t.Fatal("expected nil error")
	}
	m, ok := r.Data.(map[string]string)
	if !ok || m["k"] != "v" {
		t.Fatal("unexpected result data")
	}
}

func TestResult_Error(t *testing.T) {
	r := types.Result{Data: nil, Err: fmt.Errorf("something failed")}
	if r.Err == nil {
		t.Fatal("expected error")
	}
	if r.Err.Error() != "something failed" {
		t.Fatalf("unexpected error message: %s", r.Err.Error())
	}
	if r.Data != nil {
		t.Fatal("expected nil data")
	}
}

// --- HandlerEntry defaults ---

func TestHandlerEntry_Defaults(t *testing.T) {
	entry := types.HandlerEntry{}
	if entry.MaxWorkers != 0 {
		t.Fatal("zero value should be 0 before applying options")
	}

	if entry.MaxWorkers == 0 {
		entry.MaxWorkers = types.DefaultMaxWorkers
	}
	if entry.ReservedWorkers == 0 {
		entry.ReservedWorkers = types.DefaultReservedWorkers
	}
	if entry.QueueSize == 0 {
		entry.QueueSize = types.DefaultQueueSize
	}

	if entry.MaxWorkers != 512 {
		t.Fatalf("expected MaxWorkers 512, got %d", entry.MaxWorkers)
	}
	if entry.ReservedWorkers != 10 {
		t.Fatalf("expected ReservedWorkers 10, got %d", entry.ReservedWorkers)
	}
	if entry.QueueSize != 8192 {
		t.Fatalf("expected QueueSize 8192, got %d", entry.QueueSize)
	}
}

// --- FilterEntry ---

func TestFilterEntry_WithFilter(t *testing.T) {
	called := false
	entry := types.FilterEntry{
		Pattern: "x.*",
		Filter: func(ev *types.Event) bool {
			called = true
			return ev.Type == "x.hit"
		},
		BufferSize: 4096,
	}

	if entry.Pattern != "x.*" {
		t.Fatalf("unexpected Pattern: %s", entry.Pattern)
	}
	if !entry.Filter(&types.Event{Type: "x.hit"}) {
		t.Fatal("filter should match x.hit")
	}
	if !called {
		t.Fatal("filter was not called")
	}
	if entry.Filter(&types.Event{Type: "x.miss"}) {
		t.Fatal("filter should not match x.miss")
	}
	if entry.BufferSize != 4096 {
		t.Fatalf("unexpected BufferSize: %d", entry.BufferSize)
	}
}

func TestFilterEntry_NilFilter(t *testing.T) {
	entry := types.FilterEntry{Pattern: "y.*"}
	if entry.Pattern != "y.*" {
		t.Fatalf("unexpected Pattern: %s", entry.Pattern)
	}
	if entry.Filter != nil {
		t.Fatal("Filter should be nil when not set")
	}
}
