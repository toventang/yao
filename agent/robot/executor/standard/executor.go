package standard

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	kunlog "github.com/yaoapp/kun/log"
	agentcontext "github.com/yaoapp/yao/agent/context"
	robotevents "github.com/yaoapp/yao/agent/robot/events"
	"github.com/yaoapp/yao/agent/robot/executor/types"
	"github.com/yaoapp/yao/agent/robot/store"
	robottypes "github.com/yaoapp/yao/agent/robot/types"
	"github.com/yaoapp/yao/agent/robot/utils"
	"github.com/yaoapp/yao/event"
)

// Executor implements the standard executor with real Agent calls
// This is the production executor that:
// - Persists execution history to database
// - Calls real Agents via Assistant.Stream()
// - Logs phase transitions and errors using kun/log
type Executor struct {
	config       types.Config
	store        *store.ExecutionStore
	robotStore   *store.RobotStore
	execCount    atomic.Int32
	currentCount atomic.Int32
	onStart      func()
	onEnd        func()
}

// New creates a new standard executor
func New() *Executor {
	return &Executor{
		store:      store.NewExecutionStore(),
		robotStore: store.NewRobotStore(),
	}
}

// NewWithConfig creates a new standard executor with configuration
func NewWithConfig(config types.Config) *Executor {
	return &Executor{
		config:     config,
		store:      store.NewExecutionStore(),
		robotStore: store.NewRobotStore(),
	}
}

// Execute runs a robot through all applicable phases with real Agent calls (auto-generates ID)
func (e *Executor) Execute(ctx *robottypes.Context, robot *robottypes.Robot, trigger robottypes.TriggerType, data interface{}) (*robottypes.Execution, error) {
	return e.ExecuteWithControl(ctx, robot, trigger, data, "", nil)
}

// ExecuteWithID runs a robot through all applicable phases with a pre-generated execution ID (no control)
func (e *Executor) ExecuteWithID(ctx *robottypes.Context, robot *robottypes.Robot, trigger robottypes.TriggerType, data interface{}, execID string) (*robottypes.Execution, error) {
	return e.ExecuteWithControl(ctx, robot, trigger, data, execID, nil)
}

// ExecuteWithControl runs a robot through all applicable phases with execution control
// control: optional, allows pause/resume functionality during execution
func (e *Executor) ExecuteWithControl(ctx *robottypes.Context, robot *robottypes.Robot, trigger robottypes.TriggerType, data interface{}, execID string, control robottypes.ExecutionControl) (*robottypes.Execution, error) {
	if robot == nil {
		return nil, fmt.Errorf("robot cannot be nil")
	}

	// Determine starting phase based on trigger type
	startPhaseIndex := 0
	if trigger == robottypes.TriggerHuman || trigger == robottypes.TriggerEvent {
		startPhaseIndex = 1 // Skip P0 (Inspiration)
	}

	// Use provided execID or generate new one
	if execID == "" {
		execID = utils.NewID()
	}

	// Create execution (Job system removed, using ExecutionStore only)
	input := types.BuildTriggerInput(trigger, data)
	exec := &robottypes.Execution{
		ID:          execID,
		MemberID:    robot.MemberID,
		TeamID:      robot.TeamID,
		TriggerType: trigger,
		StartTime:   time.Now(),
		Status:      robottypes.ExecPending,
		Phase:       robottypes.AllPhases[startPhaseIndex],
		Input:       input,
		ChatID:      fmt.Sprintf("robot_%s_%s", robot.MemberID, execID),
	}

	// Load pre-existing Goals/Tasks from store when resuming a confirmed execution.
	// RunGoals and RunTasks have skip logic when these are already populated.
	if execID != "" && !e.config.SkipPersistence && e.store != nil {
		if existing, err := e.store.Get(ctx.Context, execID); err == nil && existing != nil {
			exec.Goals = existing.Goals
			exec.Tasks = existing.Tasks
			if existing.Input != nil {
				exec.Input = existing.Input
			}
		}
	}

	// If goals are pre-confirmed (passed via Input.Data["goals"]), inject them directly.
	// RunGoals will skip LLM call when exec.Goals is already populated (§18.2).
	if exec.Goals == nil && input != nil && input.Data != nil {
		if goalsStr, ok := input.Data["goals"].(string); ok && goalsStr != "" {
			exec.Goals = &robottypes.Goals{Content: goalsStr}
		}
	}

	// Initialize UI display fields (with i18n support)
	exec.Name, exec.CurrentTaskName = e.initUIFields(trigger, input, robot)

	// Set robot reference for phase methods
	exec.SetRobot(robot)

	// Persist execution record to database
	// Robot is identified by member_id (globally unique in __yao.member table)
	if !e.config.SkipPersistence && e.store != nil {
		record := store.FromExecution(exec)
		if err := e.store.Save(ctx.Context, record); err != nil {
			// Log warning but don't fail execution
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"member_id":    exec.MemberID,
				"error":        err,
			}).Warn("Failed to persist execution record: %v", err)
		}

		// If goals were pre-injected, persist them and update the execution title
		if exec.Goals != nil && exec.Goals.Content != "" {
			if err := e.store.UpdatePhase(ctx.Context, exec.ID, robottypes.PhaseGoals, exec.Goals); err != nil {
				kunlog.With(kunlog.F{
					"execution_id": exec.ID,
					"member_id":    exec.MemberID,
					"error":        err,
				}).Warn("Failed to persist pre-confirmed goals: %v", err)
			}
			if goalName := extractGoalName(exec.Goals); goalName != "" {
				e.updateUIFields(ctx, exec, goalName, "")
			}
		}

	}

	// Acquire execution slot
	if !robot.TryAcquireSlot(exec) {
		kunlog.With(kunlog.F{
			"execution_id": exec.ID,
			"member_id":    exec.MemberID,
		}).Warn("Execution quota exceeded")
		return nil, robottypes.ErrQuotaExceeded
	}
	// Defer: remove execution from robot's tracking (unless suspended) and update robot status
	defer func() {
		// Suspended executions stay in tracking — they are still "alive"
		if exec.Status == robottypes.ExecWaiting {
			return
		}
		robot.RemoveExecution(exec.ID)
		// Update robot status to idle if no more running executions
		if robot.RunningCount() == 0 && !e.config.SkipPersistence && e.robotStore != nil {
			if err := e.robotStore.UpdateStatus(ctx.Context, robot.MemberID, robottypes.RobotIdle); err != nil {
				kunlog.With(kunlog.F{
					"member_id": robot.MemberID,
					"error":     err,
				}).Warn("Failed to update robot status to idle: %v", err)
			}
		}
	}()

	// Track execution count
	e.execCount.Add(1)
	e.currentCount.Add(1)
	defer e.currentCount.Add(-1)

	// Callbacks
	if e.onStart != nil {
		e.onStart()
	}
	if e.onEnd != nil {
		defer e.onEnd()
	}

	// Update status to running
	exec.Status = robottypes.ExecRunning
	kunlog.With(kunlog.F{
		"execution_id": exec.ID,
		"member_id":    exec.MemberID,
		"trigger_type": string(exec.TriggerType),
	}).Info("Execution started")

	// Persist running status
	if !e.config.SkipPersistence && e.store != nil {
		if err := e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecRunning, ""); err != nil {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"error":        err,
			}).Warn("Failed to persist running status: %v", err)
		}
	}

	// Update robot status to working (when execution starts)
	if !e.config.SkipPersistence && e.robotStore != nil {
		if err := e.robotStore.UpdateStatus(ctx.Context, robot.MemberID, robottypes.RobotWorking); err != nil {
			kunlog.With(kunlog.F{
				"member_id": robot.MemberID,
				"error":     err,
			}).Warn("Failed to update robot status to working: %v", err)
		}
	}

	// Check for simulated failure (for testing)
	if dataStr, ok := data.(string); ok && dataStr == "simulate_failure" {
		exec.Status = robottypes.ExecFailed
		exec.Error = "simulated failure"
		kunlog.With(kunlog.F{
			"execution_id": exec.ID,
			"member_id":    exec.MemberID,
		}).Warn("Simulated failure triggered")
		// Persist failed status
		if !e.config.SkipPersistence && e.store != nil {
			_ = e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecFailed, "simulated failure")
		}
		return exec, nil
	}

	// Determine locale for UI messages
	locale := getEffectiveLocale(robot, exec.Input)

	// Execute phases (PhaseHost is not part of the normal pipeline — it is only for Interact)
	phases := robottypes.AllPhases[startPhaseIndex:]
	for _, phase := range phases {
		if phase == robottypes.PhaseHost {
			continue
		}
		if err := e.runPhase(ctx, exec, phase, data, control); err != nil {
			// Check if execution was suspended (needs human input)
			if err == robottypes.ErrExecutionSuspended {
				kunlog.With(kunlog.F{
					"execution_id": exec.ID,
					"member_id":    exec.MemberID,
					"phase":        string(phase),
				}).Info("Execution suspended during phase %s", phase)
				return exec, robottypes.ErrExecutionSuspended
			}

			// Check if execution was cancelled
			if err == robottypes.ErrExecutionCancelled {
				exec.Status = robottypes.ExecCancelled
				exec.Error = "execution cancelled by user"
				now := time.Now()
				exec.EndTime = &now

				// Update UI field for cancellation with i18n
				e.updateUIFields(ctx, exec, "", getLocalizedMessage(locale, "cancelled"))

				kunlog.With(kunlog.F{
					"execution_id": exec.ID,
					"member_id":    exec.MemberID,
					"phase":        string(phase),
				}).Info("Execution cancelled by user")

				// Persist cancelled status
				if !e.config.SkipPersistence && e.store != nil {
					_ = e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecCancelled, "execution cancelled by user")
				}
				return exec, nil
			}

			// Normal failure case
			exec.Status = robottypes.ExecFailed
			exec.Error = err.Error()

			// Update UI field for failure with i18n
			failedPrefix := getLocalizedMessage(locale, "failed_prefix")
			phaseName := getLocalizedMessage(locale, "phase_"+string(phase))
			failureMsg := failedPrefix + phaseName
			e.updateUIFields(ctx, exec, "", failureMsg)

			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"member_id":    exec.MemberID,
				"phase":        string(phase),
				"error":        err.Error(),
			}).Error("Phase execution failed: %v", err)
			// Persist failed status
			if !e.config.SkipPersistence && e.store != nil {
				_ = e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecFailed, err.Error())
			}
			return exec, nil
		}
	}

	// Mark completed
	exec.Status = robottypes.ExecCompleted
	now := time.Now()
	exec.EndTime = &now

	// Update UI field for completion with i18n
	e.updateUIFields(ctx, exec, "", getLocalizedMessage(locale, "completed"))

	duration := now.Sub(exec.StartTime)
	kunlog.With(kunlog.F{
		"execution_id": exec.ID,
		"member_id":    exec.MemberID,
		"duration_ms":  duration.Milliseconds(),
	}).Info("Execution completed successfully")

	// Persist completed status
	if !e.config.SkipPersistence && e.store != nil {
		if err := e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecCompleted, ""); err != nil {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"error":        err,
			}).Warn("Failed to persist completed status: %v", err)
		}
	}

	event.Push(ctx.Context, robotevents.ExecCompleted, robotevents.ExecPayload{
		ExecutionID: exec.ID,
		MemberID:    exec.MemberID,
		TeamID:      exec.TeamID,
		Status:      string(robottypes.ExecCompleted),
		ChatID:      exec.ChatID,
	})

	return exec, nil
}

// runPhase executes a single phase
func (e *Executor) runPhase(ctx *robottypes.Context, exec *robottypes.Execution, phase robottypes.Phase, data interface{}, control robottypes.ExecutionControl) error {
	// Check if context is cancelled before starting this phase
	select {
	case <-ctx.Context.Done():
		return robottypes.ErrExecutionCancelled
	default:
	}

	// Wait if execution is paused (blocks until resumed or cancelled)
	if control != nil {
		if err := control.WaitIfPaused(); err != nil {
			return err // Returns ErrExecutionCancelled if cancelled while paused
		}
	}

	exec.Phase = phase

	kunlog.With(kunlog.F{
		"execution_id": exec.ID,
		"member_id":    exec.MemberID,
		"phase":        string(phase),
	}).Info("Phase started: %s", phase)

	// Persist phase change immediately (so frontend sees current phase)
	if !e.config.SkipPersistence && e.store != nil {
		if err := e.store.UpdatePhase(ctx.Context, exec.ID, phase, nil); err != nil {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"phase":        string(phase),
				"error":        err,
			}).Warn("Failed to persist phase start: %v", err)
		}
	}

	if e.config.OnPhaseStart != nil {
		e.config.OnPhaseStart(phase)
	}

	phaseStart := time.Now()

	// Execute phase-specific logic
	var err error
	switch phase {
	case robottypes.PhaseInspiration:
		err = e.RunInspiration(ctx, exec, data)
	case robottypes.PhaseGoals:
		err = e.RunGoals(ctx, exec, data)
	case robottypes.PhaseTasks:
		err = e.RunTasks(ctx, exec, data)
	case robottypes.PhaseRun:
		err = e.RunExecution(ctx, exec, data)
	case robottypes.PhaseDelivery:
		err = e.RunDelivery(ctx, exec, data)
	case robottypes.PhaseLearning:
		err = e.RunLearning(ctx, exec, data)
	}

	if err != nil {
		if err == robottypes.ErrExecutionSuspended {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"member_id":    exec.MemberID,
				"phase":        string(phase),
			}).Info("Phase suspended: %s (waiting for human input)", phase)
			return err
		}
		kunlog.With(kunlog.F{
			"execution_id": exec.ID,
			"member_id":    exec.MemberID,
			"phase":        string(phase),
			"error":        err.Error(),
		}).Error("Phase failed: %s - %v", phase, err)
		return err
	}

	// Persist phase output to database
	if !e.config.SkipPersistence && e.store != nil {
		phaseData := e.getPhaseData(exec, phase)
		if phaseData != nil {
			if err := e.store.UpdatePhase(ctx.Context, exec.ID, phase, phaseData); err != nil {
				// Log warning but don't fail execution
				kunlog.With(kunlog.F{
					"execution_id": exec.ID,
					"phase":        string(phase),
					"error":        err,
				}).Warn("Failed to persist phase %s data: %v", phase, err)
			}
		}
	}

	if e.config.OnPhaseEnd != nil {
		e.config.OnPhaseEnd(phase)
	}

	phaseDuration := time.Since(phaseStart).Milliseconds()
	kunlog.With(kunlog.F{
		"execution_id": exec.ID,
		"member_id":    exec.MemberID,
		"phase":        string(phase),
		"duration_ms":  phaseDuration,
	}).Info("Phase completed: %s (took %dms)", phase, phaseDuration)

	return nil
}

// getPhaseData extracts the output data for a specific phase from execution
func (e *Executor) getPhaseData(exec *robottypes.Execution, phase robottypes.Phase) interface{} {
	switch phase {
	case robottypes.PhaseInspiration:
		return exec.Inspiration
	case robottypes.PhaseGoals:
		return exec.Goals
	case robottypes.PhaseTasks:
		return exec.Tasks
	case robottypes.PhaseRun:
		return exec.Results
	case robottypes.PhaseDelivery:
		return exec.Delivery
	case robottypes.PhaseLearning:
		return exec.Learning
	default:
		return nil
	}
}

// ExecCount returns total execution count
func (e *Executor) ExecCount() int {
	return int(e.execCount.Load())
}

// CurrentCount returns currently running execution count
func (e *Executor) CurrentCount() int {
	return int(e.currentCount.Load())
}

// Reset resets the executor counters
func (e *Executor) Reset() {
	e.execCount.Store(0)
	e.currentCount.Store(0)
}

// DefaultStreamDelay is the simulated delay for Agent Stream calls
// This will be removed when real Agent calls are implemented
const DefaultStreamDelay = 50 * time.Millisecond

// simulateStreamDelay simulates the delay of an Agent Stream call
func (e *Executor) simulateStreamDelay() {
	time.Sleep(DefaultStreamDelay)
}

// initUIFields initializes UI display fields based on trigger type with i18n support
// Returns (name, currentTaskName)
func (e *Executor) initUIFields(trigger robottypes.TriggerType, input *robottypes.TriggerInput, robot *robottypes.Robot) (string, string) {
	// Determine locale for UI messages
	locale := getEffectiveLocale(robot, input)

	// Get localized default messages
	name := getLocalizedMessage(locale, "preparing")
	currentTaskName := getLocalizedMessage(locale, "starting")

	switch trigger {
	case robottypes.TriggerHuman:
		// For human trigger, extract name from first message
		if input != nil && len(input.Messages) > 0 {
			if content, ok := input.Messages[0].GetContentAsString(); ok && content != "" {
				// Use first 100 chars of message as name
				name = content
				if len(name) > 100 {
					name = name[:100] + "..."
				}
			}
		}
	case robottypes.TriggerClock:
		name = getLocalizedMessage(locale, "scheduled_execution")
	case robottypes.TriggerEvent:
		if input != nil && input.EventType != "" {
			name = getLocalizedMessage(locale, "event_prefix") + input.EventType
		} else {
			name = getLocalizedMessage(locale, "event_triggered")
		}
	}

	return name, currentTaskName
}

// getEffectiveLocale determines the locale for UI display
// Priority: input.Locale > robot.Config.DefaultLocale > "en"
func getEffectiveLocale(robot *robottypes.Robot, input *robottypes.TriggerInput) string {
	// 1. Human trigger with explicit locale
	if input != nil && input.Locale != "" {
		return input.Locale
	}
	// 2. Robot configured default
	if robot != nil && robot.Config != nil {
		return robot.Config.GetDefaultLocale()
	}
	// 3. System default
	return "en"
}

// i18n message maps for UI display fields
// Use simple locale codes (en, zh) as keys
var uiMessages = map[string]map[string]string{
	"en": {
		"preparing":           "Preparing...",
		"starting":            "Starting...",
		"scheduled_execution": "Scheduled execution",
		"event_prefix":        "Event: ",
		"event_triggered":     "Event triggered",
		"analyzing_context":   "Analyzing context...",
		"planning_goals":      "Planning goals...",
		"breaking_down_tasks": "Breaking down tasks...",
		"generating_delivery": "Generating delivery content...",
		"sending_delivery":    "Sending delivery...",
		"learning_from_exec":  "Learning from execution...",
		"completed":           "Completed",
		"cancelled":           "Cancelled",
		"failed_prefix":       "Failed at ",
		"task_prefix":         "Task",
		// Phase names for failure messages
		"phase_inspiration": "inspiration",
		"phase_goals":       "goals",
		"phase_tasks":       "tasks",
		"phase_run":         "execution",
		"phase_delivery":    "delivery",
		"phase_learning":    "learning",
	},
	"zh": {
		"preparing":           "准备中...",
		"starting":            "启动中...",
		"scheduled_execution": "定时执行",
		"event_prefix":        "事件: ",
		"event_triggered":     "事件触发",
		"analyzing_context":   "分析上下文...",
		"planning_goals":      "规划目标...",
		"breaking_down_tasks": "分解任务...",
		"generating_delivery": "生成交付内容...",
		"sending_delivery":    "正在发送...",
		"learning_from_exec":  "学习执行经验...",
		"completed":           "已完成",
		"cancelled":           "已取消",
		"failed_prefix":       "失败于",
		"task_prefix":         "任务",
		// Phase names for failure messages
		"phase_inspiration": "灵感阶段",
		"phase_goals":       "目标阶段",
		"phase_tasks":       "任务阶段",
		"phase_run":         "执行阶段",
		"phase_delivery":    "交付阶段",
		"phase_learning":    "学习阶段",
	},
}

// getLocalizedMessage returns a localized message for the given key
func getLocalizedMessage(locale string, key string) string {
	if messages, ok := uiMessages[locale]; ok {
		if msg, ok := messages[key]; ok {
			return msg
		}
	}
	// Fallback to English
	if messages, ok := uiMessages["en"]; ok {
		if msg, ok := messages[key]; ok {
			return msg
		}
	}
	return key // Return key as fallback
}

// updateUIFields updates UI display fields and persists to database
func (e *Executor) updateUIFields(ctx *robottypes.Context, exec *robottypes.Execution, name string, currentTaskName string) {
	// Update in-memory execution
	if name != "" {
		exec.Name = name
	}
	if currentTaskName != "" {
		exec.CurrentTaskName = currentTaskName
	}

	// Persist to database
	if !e.config.SkipPersistence && e.store != nil {
		if err := e.store.UpdateUIFields(ctx.Context, exec.ID, name, currentTaskName); err != nil {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"error":        err,
			}).Warn("Failed to update UI fields: %v", err)
		}
	}
}

// updateTasksState persists the current tasks array with status to database
// This should be called after each task status change for real-time UI updates
func (e *Executor) updateTasksState(ctx *robottypes.Context, exec *robottypes.Execution) {
	if e.config.SkipPersistence || e.store == nil {
		return
	}

	// Convert Current to store.CurrentState
	var current *store.CurrentState
	if exec.Current != nil {
		current = &store.CurrentState{
			TaskIndex: exec.Current.TaskIndex,
			Progress:  exec.Current.Progress,
		}
	}

	if err := e.store.UpdateTasks(ctx.Context, exec.ID, exec.Tasks, current); err != nil {
		kunlog.With(kunlog.F{
			"execution_id": exec.ID,
			"error":        err,
		}).Warn("Failed to update tasks state: %v", err)
	}
}

// extractGoalName extracts the execution name from goals output
func extractGoalName(goals *robottypes.Goals) string {
	if goals == nil || goals.Content == "" {
		return ""
	}

	// Extract first non-empty, non-markdown-header line as the goal name
	content := goals.Content
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip markdown headers (# ## ### etc.)
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Skip markdown horizontal rules (--- or ***)
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "***") {
			continue
		}
		// Found a content line - strip markdown formatting
		line = stripMarkdownFormatting(line)
		// Limit length
		if len(line) > 150 {
			line = line[:150] + "..."
		}
		return line
	}

	// Fallback: if all lines are headers, use first header without # prefix
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip leading # symbols
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		line = stripMarkdownFormatting(line)
		if line != "" {
			if len(line) > 150 {
				line = line[:150] + "..."
			}
			return line
		}
	}

	return ""
}

// stripMarkdownFormatting removes common markdown formatting from text
func stripMarkdownFormatting(s string) string {
	// Remove bold/italic markers
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "_", "")
	// Remove inline code
	s = strings.ReplaceAll(s, "`", "")
	// Remove link syntax [text](url) -> text
	// Simple approach: just remove brackets and parentheses content
	for {
		start := strings.Index(s, "[")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "]")
		if end == -1 {
			break
		}
		linkEnd := start + end
		// Check if followed by (url)
		if linkEnd+1 < len(s) && s[linkEnd+1] == '(' {
			parenEnd := strings.Index(s[linkEnd+1:], ")")
			if parenEnd != -1 {
				// Extract just the link text
				linkText := s[start+1 : linkEnd]
				s = s[:start] + linkText + s[linkEnd+1+parenEnd+1:]
				continue
			}
		}
		// Just remove brackets
		s = s[:start] + s[start+1:linkEnd] + s[linkEnd+1:]
	}
	return strings.TrimSpace(s)
}

// Suspend transitions the execution to waiting status, persists state, and returns
// ErrExecutionSuspended so the caller stops further phase processing.
func (e *Executor) Suspend(ctx *robottypes.Context, exec *robottypes.Execution, taskIndex int, question string) error {
	now := time.Now()
	taskID := ""
	if taskIndex >= 0 && taskIndex < len(exec.Tasks) {
		taskID = exec.Tasks[taskIndex].ID
		exec.Tasks[taskIndex].Status = robottypes.TaskWaitingInput
	}

	exec.Status = robottypes.ExecWaiting
	exec.WaitingTaskID = taskID
	exec.WaitingQuestion = question
	exec.WaitingSince = &now
	exec.ResumeContext = &robottypes.ResumeContext{
		TaskIndex:       taskIndex,
		PreviousResults: exec.Results,
	}

	if !e.config.SkipPersistence && e.store != nil {
		// Persist task state (waiting_input on the specific task)
		e.updateTasksState(ctx, exec)
		// Persist P3 results so UI can show completed tasks while waiting (§16.26)
		if err := e.store.UpdatePhase(ctx.Context, exec.ID, robottypes.PhaseRun, exec.Results); err != nil {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"error":        err,
			}).Warn("Failed to persist partial results on suspend: %v", err)
		}
		// Persist suspend state atomically
		if err := e.store.UpdateSuspendState(ctx.Context, exec.ID, taskID, question, exec.ResumeContext); err != nil {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"task_id":      taskID,
				"error":        err,
			}).Warn("Failed to persist suspend state: %v", err)
		}
	}

	kunlog.With(kunlog.F{
		"execution_id": exec.ID,
		"member_id":    exec.MemberID,
		"task_id":      taskID,
		"question":     question,
	}).Info("Execution suspended, waiting for human input")

	// Fire event (best-effort, errors are ignored)
	event.Push(ctx.Context, robotevents.ExecWaiting, robotevents.NeedInputPayload{
		ExecutionID: exec.ID,
		MemberID:    exec.MemberID,
		TeamID:      exec.TeamID,
		TaskID:      taskID,
		Question:    question,
		ChatID:      exec.ChatID,
	})

	return robottypes.ErrExecutionSuspended
}

// Resume resumes a suspended execution with human-provided input.
// Loads execution from DB, restores state, injects reply, and continues from the suspended task.
func (e *Executor) Resume(ctx *robottypes.Context, execID string, reply string) error {
	if ctx == nil {
		return fmt.Errorf("context is required for resume")
	}
	if execID == "" {
		return fmt.Errorf("execID cannot be empty")
	}
	if e.store == nil {
		return fmt.Errorf("store is required for resume")
	}

	// Load execution record from DB
	record, err := e.store.Get(ctx.Context, execID)
	if err != nil {
		return fmt.Errorf("failed to load execution: %w", err)
	}
	if record == nil {
		return fmt.Errorf("execution not found: %s", execID)
	}
	if record.Status != robottypes.ExecWaiting {
		return fmt.Errorf("execution %s is not in waiting status (current: %s)", execID, record.Status)
	}

	// Restore runtime execution from record
	exec := record.ToExecution()

	// Load robot from store
	if e.robotStore == nil {
		return fmt.Errorf("robot store is required for resume")
	}
	robotRecord, err := e.robotStore.Get(ctx.Context, exec.MemberID)
	if err != nil {
		return fmt.Errorf("failed to load robot: %w", err)
	}
	if robotRecord == nil {
		return fmt.Errorf("robot not found: %s", exec.MemberID)
	}
	robot, err := robotRecord.ToRobot()
	if err != nil {
		return fmt.Errorf("failed to convert robot record: %w", err)
	}
	exec.SetRobot(robot)

	// Re-add execution to robot's in-memory tracking (skips quota check per §16.30)
	robot.AddExecution(exec)

	// Maintain executor concurrency count (§16.21)
	e.currentCount.Add(1)
	defer e.currentCount.Add(-1)

	// Defer cleanup: mirror ExecuteWithControl's defer logic (§16.21)
	defer func() {
		if exec.Status == robottypes.ExecWaiting {
			return // re-suspended, keep tracking
		}
		robot.RemoveExecution(exec.ID)
		if robot.RunningCount() == 0 && !e.config.SkipPersistence && e.robotStore != nil {
			if err := e.robotStore.UpdateStatus(ctx.Context, robot.MemberID, robottypes.RobotIdle); err != nil {
				kunlog.With(kunlog.F{
					"member_id": robot.MemberID,
					"error":     err,
				}).Warn("Failed to update robot status to idle after resume: %v", err)
			}
		}
	}()

	// Handle __skip__: mark waiting task as skipped and advance to next task
	if reply == "__skip__" && exec.ResumeContext != nil {
		ti := exec.ResumeContext.TaskIndex
		if ti >= 0 && ti < len(exec.Tasks) {
			task := &exec.Tasks[ti]
			task.Status = robottypes.TaskSkipped
			exec.ResumeContext.PreviousResults = append(exec.ResumeContext.PreviousResults, robottypes.TaskResult{
				TaskID:   task.ID,
				Success:  false,
				Output:   "skipped",
				Duration: 0,
			})
			exec.ResumeContext.TaskIndex = ti + 1
			if !e.config.SkipPersistence && e.store != nil {
				e.updateTasksState(ctx, exec)
			}
		}
		reply = "" // Don't inject __skip__ as a message
	}

	// Inject reply into the waiting task's messages so the re-executed task gets context
	if exec.ResumeContext != nil {
		ti := exec.ResumeContext.TaskIndex
		if ti >= 0 && ti < len(exec.Tasks) && reply != "" {
			exec.Tasks[ti].Messages = append(exec.Tasks[ti].Messages, agentcontext.Message{
				Role:    agentcontext.RoleUser,
				Content: fmt.Sprintf("[Human reply] %s", reply),
			})
		}
	}

	// Clear waiting fields and transition back to running
	exec.Status = robottypes.ExecRunning
	exec.WaitingTaskID = ""
	exec.WaitingQuestion = ""
	exec.WaitingSince = nil

	if !e.config.SkipPersistence && e.store != nil {
		if err := e.store.UpdateResumeState(ctx.Context, exec.ID); err != nil {
			kunlog.With(kunlog.F{
				"execution_id": exec.ID,
				"error":        err,
			}).Warn("Failed to persist resume state: %v", err)
		}
	}

	kunlog.With(kunlog.F{
		"execution_id": exec.ID,
		"member_id":    exec.MemberID,
		"reply_len":    len(reply),
	}).Info("Execution resumed")

	event.Push(ctx.Context, robotevents.ExecResumed, robotevents.ExecPayload{
		ExecutionID: exec.ID,
		MemberID:    exec.MemberID,
		TeamID:      exec.TeamID,
		ChatID:      exec.ChatID,
	})

	// Continue P3 (Run) from where it was suspended
	if err := e.RunExecution(ctx, exec, nil); err != nil {
		if err == robottypes.ErrExecutionSuspended {
			return err
		}
		exec.Status = robottypes.ExecFailed
		exec.Error = err.Error()
		if !e.config.SkipPersistence && e.store != nil {
			_ = e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecFailed, err.Error())
		}
		return err
	}

	// Clear resume context after successful P3 completion
	exec.ResumeContext = nil

	// Continue with P4 (Delivery) and P5 (Learning)
	locale := getEffectiveLocale(robot, exec.Input)
	for _, phase := range []robottypes.Phase{robottypes.PhaseDelivery, robottypes.PhaseLearning} {
		if err := e.runPhase(ctx, exec, phase, nil, nil); err != nil {
			if err == robottypes.ErrExecutionSuspended {
				return err
			}
			exec.Status = robottypes.ExecFailed
			exec.Error = err.Error()
			failedPrefix := getLocalizedMessage(locale, "failed_prefix")
			phaseName := getLocalizedMessage(locale, "phase_"+string(phase))
			e.updateUIFields(ctx, exec, "", failedPrefix+phaseName)
			if !e.config.SkipPersistence && e.store != nil {
				_ = e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecFailed, err.Error())
			}
			return fmt.Errorf("resume phase %s failed: %w", phase, err)
		}
	}

	// Mark completed
	exec.Status = robottypes.ExecCompleted
	now := time.Now()
	exec.EndTime = &now
	e.updateUIFields(ctx, exec, "", getLocalizedMessage(locale, "completed"))
	if !e.config.SkipPersistence && e.store != nil {
		_ = e.store.UpdateStatus(ctx.Context, exec.ID, robottypes.ExecCompleted, "")
	}

	return nil
}

// Verify Executor implements types.Executor
var _ types.Executor = (*Executor)(nil)
