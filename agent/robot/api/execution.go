package api

import (
	"context"
	"fmt"
	"sync"

	"github.com/yaoapp/yao/agent/robot/store"
	"github.com/yaoapp/yao/agent/robot/types"
)

// executionStore singleton
var (
	execStore     *store.ExecutionStore
	execStoreOnce sync.Once
)

// getExecutionStore returns the singleton execution store
func getExecutionStore() *store.ExecutionStore {
	execStoreOnce.Do(func() {
		execStore = store.NewExecutionStore()
	})
	return execStore
}

// ResetExecutionStore resets the singleton for testing purposes
// This should only be called in tests
func ResetExecutionStore() {
	execStoreOnce = sync.Once{}
	execStore = nil
}

// ==================== Execution Query API ====================
// These functions query and manage execution history

// GetExecution returns a specific execution by ID
func GetExecution(ctx *types.Context, execID string) (*types.Execution, error) {
	if execID == "" {
		return nil, fmt.Errorf("execution_id is required")
	}

	// Try to get from execution store
	record, err := getExecutionStore().Get(context.Background(), execID)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("execution not found: %s", execID)
	}

	return record.ToExecution(), nil
}

// ListExecutions returns execution history for a robot
func ListExecutions(ctx *types.Context, memberID string, query *ExecutionQuery) (*ExecutionResult, error) {
	if memberID == "" {
		return nil, fmt.Errorf("member_id is required")
	}

	if query == nil {
		query = &ExecutionQuery{}
	}
	query.applyDefaults()

	opts := &store.ListOptions{
		MemberID: memberID,
		Page:     query.Page,
		PageSize: query.PageSize,
		OrderBy:  "start_time desc",
	}

	if query.Status != "" {
		opts.Status = query.Status
	}
	if len(query.ExcludeStatuses) > 0 {
		opts.ExcludeStatuses = query.ExcludeStatuses
	}
	if query.Trigger != "" {
		opts.TriggerType = query.Trigger
	}

	result, err := getExecutionStore().List(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}

	executions := make([]*types.Execution, 0, len(result.Data))
	for _, record := range result.Data {
		executions = append(executions, record.ToExecution())
	}

	return &ExecutionResult{
		Data:     executions,
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
	}, nil
}

// ==================== Execution Control API ====================
// These functions control running executions

// PauseExecution pauses a running execution
func PauseExecution(ctx *types.Context, execID string) error {
	if execID == "" {
		return fmt.Errorf("execution_id is required")
	}

	mgr, err := getManager()
	if err != nil {
		return err
	}

	if err := mgr.PauseExecution(ctx, execID); err != nil {
		return err
	}

	// Update database status to paused
	return getExecutionStore().UpdateStatus(context.Background(), execID, types.ExecPaused, "")
}

// ResumeExecution resumes a paused execution
func ResumeExecution(ctx *types.Context, execID string) error {
	if execID == "" {
		return fmt.Errorf("execution_id is required")
	}

	mgr, err := getManager()
	if err != nil {
		return err
	}

	if err := mgr.ResumeExecution(ctx, execID); err != nil {
		return err
	}

	// Update database status back to running
	return getExecutionStore().UpdateStatus(context.Background(), execID, types.ExecRunning, "")
}

// StopExecution stops a running execution
func StopExecution(ctx *types.Context, execID string) error {
	if execID == "" {
		return fmt.Errorf("execution_id is required")
	}

	mgr, err := getManager()
	if err != nil {
		return err
	}

	if err := mgr.StopExecution(ctx, execID); err != nil {
		return err
	}

	// Update database status to cancelled
	return getExecutionStore().UpdateStatus(context.Background(), execID, types.ExecCancelled, "User cancelled")
}

// ==================== Execution Status API ====================

// GetExecutionStatus returns the current status of an execution
// This combines stored data with runtime state
func GetExecutionStatus(ctx *types.Context, execID string) (*types.Execution, error) {
	if execID == "" {
		return nil, fmt.Errorf("execution_id is required")
	}

	// Get from store first
	exec, err := GetExecution(ctx, execID)
	if err != nil {
		return nil, err
	}

	// If manager is running, check for runtime state
	mgr, mgrErr := getManager()
	if mgrErr == nil {
		// Check if execution is being tracked (running)
		ctrlExec, ctrlErr := mgr.GetExecutionStatus(execID)
		if ctrlErr == nil && ctrlExec != nil {
			// Update with runtime state
			exec.Status = ctrlExec.Status
			exec.Phase = ctrlExec.Phase
		}
	}

	return exec, nil
}
