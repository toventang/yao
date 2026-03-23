# DESIGN-V2 Line-by-Line Review Findings

**Review Date:** 2026-02-25  
**Files Reviewed:** `runner.go`, `run.go`

---

## File 1: runner.go

### 1. ExecuteTask: Is it truly single-call? No retry loop? No validation?

**✅ PASS** — Lines 65–108

- Non-assistant: single call to `executeNonAssistantTask` (L74), no loop
- Assistant: single call to `executeAssistantTask` (L89), no loop
- No `validator` import or call anywhere in the file
- Comment at L62–63: "V2 simplified: single call, no validation loop"

---

### 2. Does it correctly split assistant vs non-assistant at the top?

**✅ PASS** — Lines 72–86 vs 88–107

- L72: `if task.ExecutorType != robottypes.ExecutorAssistant` — non-assistant branch first
- L88+: assistant branch follows
- Clear split at the top of the function

---

### 3. For non-assistant: does it call executeNonAssistantTask which handles MCP and Process?

**✅ PASS** — Lines 74, 110–119

- L74: `output, err := r.executeNonAssistantTask(task, taskCtx)`
- `executeNonAssistantTask` (L110–119): switch on `ExecutorMCP` and `ExecutorProcess`, delegates to `ExecuteMCPTask` and `ExecuteProcessTask`

---

### 4. For assistant: does it call executeAssistantTask which returns (output, *CallResult, error)?

**✅ PASS** — Lines 89, 123–145

- L89: `output, callResult, err := r.executeAssistantTask(task, taskCtx)`
- L123: `func (r *Runner) executeAssistantTask(...) (interface{}, *CallResult, error)`
- L144: `return output, turnResult.Result, nil`

---

### 5. Does executeAssistantTask use conv.Turn() (single turn, not multi-turn)?

**✅ PASS** — Lines 126, 138

- L126: `conv := NewConversation(task.ExecutorID, chatID, 1)` — maxTurns=1
- L138: `turnResult, err := conv.Turn(r.ctx, input)` — single `Turn` call, no loop

---

### 6. Does detectNeedMoreInfo properly check result.Next for map with "status" == "need_input"?

**✅ PASS** — Lines 149–166

- L150–151: nil checks for `result` and `result.Next`
- L154: type assertion to `map[string]interface{}`
- L157–159: `status, _ := m["status"].(string); if status != "need_input" return false`
- Matches DESIGN §16.5 protocol

---

### 7. Does it extract "question" from the map? What happens if question is empty?

**✅ PASS** — Lines 161–165

- L161: `question, _ := m["question"].(string)`
- L163–164: `if question == "" { question = result.GetText() }` — fallback to `CallResult` text
- Empty question handled via fallback

---

### 8. Are result.NeedInput and result.InputQuestion set correctly?

**✅ PASS** — Lines 102–105

- L102–104: `if needInput, question := detectNeedMoreInfo(callResult); needInput { result.NeedInput = true; result.InputQuestion = question }`
- Set only when `detectNeedMoreInfo` returns true

---

### 9. Does result.Duration get set in all paths (success and failure)?

**✅ PASS** — Lines 77, 84, 92, 98

- Non-assistant error: L77
- Non-assistant success: L84
- Assistant error: L92
- Assistant success: L98
- All paths set `result.Duration = time.Since(startTime).Milliseconds()`

---

### 10. Does buildResult helper exist or is result construction inline?

**✅ PASS (inline)** — Lines 68–107

- No `buildResult` helper; construction is inline
- DESIGN §16.4 pseudocode uses `buildResult`; inline construction is acceptable and used here

---

### 11. Any edge cases: what if task.ExecutorType is empty or unknown?

**✅ PASS** — Lines 72, 112–118

- Empty/unknown: `!= ExecutorAssistant` is true → non-assistant branch
- L116–117: `default` returns `fmt.Errorf("unsupported executor type: %s (expected mcp or process)", task.ExecutorType)`
- Error returned and propagated; no silent failure

---

### Additional Finding (runner.go)

**⚠️ Minor:** DESIGN §9.1 shows `event.Push("robot.task.failed", ...)` inside `ExecuteTask` when `err != nil`. Implementation pushes `TaskFailed` from `run.go` (L113–120) when `result.Success` is false. Behavior is equivalent; only location differs.

---

## File 2: run.go

### 1. DefaultRunConfig — ContinueOnFailure defaults to true?

**✅ PASS** — Lines 21–26

- L23–25: `return &RunConfig{ ContinueOnFailure: true }`
- Matches DESIGN §6.3

---

### 2. RunExecution — does it check exec.ResumeContext for startIndex and PreviousResults?

**✅ PASS** — Lines 60–66

- L61–64: `if exec.ResumeContext != nil { startIndex = exec.ResumeContext.TaskIndex; exec.Results = exec.ResumeContext.PreviousResults }`
- L72: loop starts at `startIndex`
- Matches DESIGN §9.2, §16.3

---

### 3. Does it NOT reset Results when ResumeContext is present?

**✅ PASS** — Lines 62–65

- When `ResumeContext != nil`: `exec.Results = exec.ResumeContext.PreviousResults` — restores, does not reset
- When `ResumeContext == nil`: `exec.Results = make(...)` — fresh slice

---

### 4. Does it set task.Status to TaskRunning before execution?

**✅ PASS** — Lines 86–89

- L87: `task.Status = robottypes.TaskRunning`
- L88–89: `task.StartTime = &now`
- Set before `ExecuteTask` (L98)

---

### 5. Does it call e.updateTasksState to persist running state?

**✅ PASS** — Line 92

- L92: `e.updateTasksState(ctx, exec)` immediately after setting task status
- Persists running state before execution

---

### 6. Does result.NeedInput trigger e.Suspend(ctx, exec, i, result.InputQuestion)?

**✅ PASS** — Lines 100–103

- L100–102: `if result.NeedInput { return e.Suspend(ctx, exec, i, result.InputQuestion) }`
- Correct parameters and early return

---

### 7. Is the result NOT appended before Suspend (avoiding duplicate results per §16.15)?

**✅ PASS** — Lines 100–103, 124

- L100–102: `NeedInput` branch returns before any append
- L124: `exec.Results = append(exec.Results, *result)` is after the `NeedInput` check
- No append on suspend; matches DESIGN §16.15

---

### 8. Does it push event.Push for TaskFailed when a task fails?

**✅ PASS** — Lines 113–120

- L113–120: `event.Push(ctx.Context, robotevents.TaskFailed, robotevents.NeedInputPayload{...})` when `!result.Success`
- Event is pushed on task failure

**⚠️ Minor:** Uses `NeedInputPayload` with `Question: result.Error`. DESIGN §7.2 does not define a TaskFailed payload. `ExecPayload` (with `Error`) might be more appropriate; `NeedInputPayload.Question` is reused for the error message. Functionally acceptable but semantically odd.

---

### 9. Does it skip remaining tasks when ContinueOnFailure is false?

**✅ PASS** — Lines 129–137

- L129: `if !result.Success && !config.ContinueOnFailure`
- L131–134: marks remaining tasks as `TaskSkipped`
- L136: `return fmt.Errorf(...)` — stops execution
- Matches DESIGN §9.2

---

### 10. Does it clear exec.Current and exec.ResumeContext after completion?

**✅ PASS** — Lines 141–143

- L142–143: `exec.Current = nil; exec.ResumeContext = nil` after loop completes
- Only on normal completion (no early return from Suspend or failure)

---

### 11. Is there any event.Push for TaskCompleted?

**❌ FINDING** — run.go

- No `event.Push(robotevents.TaskCompleted, ...)` when a task succeeds
- DESIGN §7.2 defines `EventTaskCompleted = "robot.task.completed"`
- DESIGN-V2 §20.4 (B5) notes missing TaskCompleted event constant; implementation also does not push it
- **Recommendation:** Add `event.Push(ctx.Context, robotevents.TaskCompleted, payload)` when `result.Success` (e.g. after L110)

---

### 12. Does getRunConfig properly handle nil data?

**✅ PASS** — Lines 50–55

- No separate `getRunConfig`; config is obtained inline
- L51–55: `if cfg, ok := data.(*RunConfig); ok && cfg != nil { config = cfg } else { config = DefaultRunConfig() }`
- Handles: `data == nil`, wrong type, `cfg == nil` → falls back to `DefaultRunConfig()`

---

### Additional Finding (run.go)

**⚠️ Order of operations:** Task status update (L109–120) and result append (L124) occur *after* the NeedInput check. Flow is correct: NeedInput → Suspend (return) → no append, no status update for that task.

---

## Summary

| Category | runner.go | run.go |
|----------|-----------|--------|
| **PASS** | 11/11 | 11/12 |
| **Minor** | 1 | 1 |
| **Finding** | 0 | 1 (TaskCompleted not pushed) |

### Action Items

1. **run.go L109–110:** Add `event.Push(ctx.Context, robotevents.TaskCompleted, payload)` when `result.Success` for per-task completion events.
2. **run.go L113:** Consider introducing a `TaskFailedPayload` (or using `ExecPayload` with `Error`) instead of `NeedInputPayload` for TaskFailed events.
