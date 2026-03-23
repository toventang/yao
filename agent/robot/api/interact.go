package api

import (
	"fmt"

	agentcontext "github.com/yaoapp/yao/agent/context"
	"github.com/yaoapp/yao/agent/robot/executor/standard"
	"github.com/yaoapp/yao/agent/robot/manager"
	"github.com/yaoapp/yao/agent/robot/types"
)

// InteractRequest represents a unified interaction with a robot.
type InteractRequest struct {
	ExecutionID string               `json:"execution_id,omitempty"`
	TaskID      string               `json:"task_id,omitempty"`
	Source      types.InteractSource `json:"source,omitempty"`
	Message     string               `json:"message"`
	Action      string               `json:"action,omitempty"`
}

// InteractResult is the response from an interaction.
type InteractResult struct {
	ExecutionID string `json:"execution_id,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
	Reply       string `json:"reply,omitempty"`
	WaitForMore bool   `json:"wait_for_more,omitempty"`
}

// Interact handles all human-robot interactions through a unified entry point.
//
// Routing logic:
//   - If manager is running, delegate to Manager.HandleInteract (full V2 flow with Host Agent)
//   - Otherwise, use legacy direct-executor path for backward compatibility
func Interact(ctx *types.Context, memberID string, req *InteractRequest) (*InteractResult, error) {
	if memberID == "" {
		return nil, fmt.Errorf("member_id is required")
	}
	if req == nil {
		return nil, fmt.Errorf("interact request is required")
	}

	// Try V2 path via manager
	mgr, err := getManager()
	if err == nil && mgr != nil {
		return managerInteract(ctx, mgr, memberID, req)
	}

	// V1 fallback: require execution_id for direct resume
	if req.ExecutionID == "" {
		return nil, fmt.Errorf("execution_id is required for current version (Host Agent deferred)")
	}

	return legacyResume(ctx, req)
}

// managerInteract delegates to the manager's HandleInteract.
func managerInteract(ctx *types.Context, mgr *manager.Manager, memberID string, req *InteractRequest) (*InteractResult, error) {
	mgrReq := &manager.InteractRequest{
		ExecutionID: req.ExecutionID,
		TaskID:      req.TaskID,
		Source:      req.Source,
		Message:     req.Message,
		Action:      req.Action,
	}

	resp, err := mgr.HandleInteract(ctx, memberID, mgrReq)
	if err != nil {
		return nil, err
	}

	return &InteractResult{
		ExecutionID: resp.ExecutionID,
		Status:      resp.Status,
		Message:     resp.Message,
		ChatID:      resp.ChatID,
		Reply:       resp.Reply,
		WaitForMore: resp.WaitForMore,
	}, nil
}

// legacyResume handles the direct executor resume path (backward compatible).
func legacyResume(ctx *types.Context, req *InteractRequest) (*InteractResult, error) {
	executor := standard.New()
	err := executor.Resume(ctx, req.ExecutionID, req.Message)
	if err != nil {
		if err == types.ErrExecutionSuspended {
			return &InteractResult{
				ExecutionID: req.ExecutionID,
				Status:      "waiting",
				Message:     "Execution suspended again: needs more input",
			}, nil
		}
		return nil, fmt.Errorf("failed to resume execution: %w", err)
	}

	return &InteractResult{
		ExecutionID: req.ExecutionID,
		Status:      "resumed",
		Message:     "Execution resumed and completed successfully",
	}, nil
}

// Reply is a semantic shortcut for replying to a specific waiting task.
func Reply(ctx *types.Context, memberID string, execID string, taskID string, message string) (*InteractResult, error) {
	return Interact(ctx, memberID, &InteractRequest{
		ExecutionID: execID,
		TaskID:      taskID,
		Source:      types.InteractSourceUI,
		Message:     message,
	})
}

// Confirm is a semantic shortcut for confirming a pending execution.
func Confirm(ctx *types.Context, memberID string, execID string, message string) (*InteractResult, error) {
	return Interact(ctx, memberID, &InteractRequest{
		ExecutionID: execID,
		Source:      types.InteractSourceUI,
		Message:     message,
		Action:      "confirm",
	})
}

// InteractStream is the streaming version of Interact.
// It streams Host Agent text tokens via streamFn while still returning the final InteractResult.
// V1 fallback does not support streaming and returns an error.
func InteractStream(ctx *types.Context, memberID string, req *InteractRequest, streamFn standard.StreamCallback) (*InteractResult, error) {
	if memberID == "" {
		return nil, fmt.Errorf("member_id is required")
	}
	if req == nil {
		return nil, fmt.Errorf("interact request is required")
	}

	mgr, err := getManager()
	if err != nil || mgr == nil {
		return nil, fmt.Errorf("streaming requires V2 manager (not available)")
	}

	mgrReq := &manager.InteractRequest{
		ExecutionID: req.ExecutionID,
		TaskID:      req.TaskID,
		Source:      req.Source,
		Message:     req.Message,
		Action:      req.Action,
	}

	resp, err := mgr.HandleInteractStream(ctx, memberID, mgrReq, streamFn)
	if err != nil {
		return nil, err
	}

	return &InteractResult{
		ExecutionID: resp.ExecutionID,
		Status:      resp.Status,
		Message:     resp.Message,
		ChatID:      resp.ChatID,
		Reply:       resp.Reply,
		WaitForMore: resp.WaitForMore,
	}, nil
}

// InteractStreamRaw is the CUI-protocol-aligned streaming version of Interact.
// It passes raw message.Message objects to the onMessage callback, preserving all CUI
// protocol fields for direct SSE passthrough to the frontend.
func InteractStreamRaw(ctx *types.Context, memberID string, req *InteractRequest, onMessage agentcontext.OnMessageFunc) (*InteractResult, error) {
	if memberID == "" {
		return nil, fmt.Errorf("member_id is required")
	}
	if req == nil {
		return nil, fmt.Errorf("interact request is required")
	}

	mgr, err := getManager()
	if err != nil || mgr == nil {
		return nil, fmt.Errorf("raw streaming requires V2 manager (not available)")
	}

	mgrReq := &manager.InteractRequest{
		ExecutionID: req.ExecutionID,
		TaskID:      req.TaskID,
		Source:      req.Source,
		Message:     req.Message,
		Action:      req.Action,
	}

	resp, err := mgr.HandleInteractStreamRaw(ctx, memberID, mgrReq, onMessage)
	if err != nil {
		return nil, err
	}

	return &InteractResult{
		ExecutionID: resp.ExecutionID,
		Status:      resp.Status,
		Message:     resp.Message,
		ChatID:      resp.ChatID,
		Reply:       resp.Reply,
		WaitForMore: resp.WaitForMore,
	}, nil
}

// CancelExecution cancels a waiting/confirming execution via the manager.
func CancelExecution(ctx *types.Context, execID string) error {
	mgr, err := getManager()
	if err != nil {
		return fmt.Errorf("cancel not available: %w", err)
	}
	return mgr.CancelExecution(ctx, execID)
}
