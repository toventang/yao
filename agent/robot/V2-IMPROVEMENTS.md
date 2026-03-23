# Robot Agent V2 — Improvement Plan

> Generated: 2026-02-25
> Based on: DESIGN-V2.md deep review against implementation code
> Scope: Bug fixes, missing unit tests, code quality improvements

---

## Auth Context Clarification

Robot is a legitimate team member in `__yao.member`. Auth is always present:

| Trigger Path | Auth Source | Code |
|-------------|------------|------|
| Clock | `manager.buildRobotAuth(robot)` → `{UserID: robot.MemberID, TeamID: robot.TeamID}` | `manager.go:270` |
| Human / Event | Caller's `ctx.Auth` passthrough from HTTP middleware | `openapi/agent/robot/*.go` |
| Resume | Loaded from execution record → `buildRobotAuth` or caller passthrough | `executor.go:764+` |

The `openapi/agent/robot/` layer constructs `ctx := &robottypes.Context{}` without Auth — this is the **existing V1 pattern** across ALL openapi handlers (trigger.go, execution.go, list.go, etc). Auth checking is done via `authorized.GetInfo(c)` at the Gin middleware level; `robottypes.Context` is a downstream execution context.

**However**, `manager/interact.go:createConfirmingExecution` calls `ctx.UserID()` which returns `""` when openapi passes an empty Context. This is a V2-specific issue since V1 handlers don't need `ctx.UserID()`.

---

## 1. Bugs

### BUG-1 [P0] `advanceExecution` discards confirmed Goals/Tasks

**File**: `manager/interact.go:415-431`

**Problem**: After multi-round Host Agent confirmation (which may have generated Goals and Tasks stored in `record.Goals` / `record.Tasks`), `advanceExecution()` submits to Pool via `m.pool.SubmitWithID(...)`. The Pool Worker then calls `ExecuteWithControl()` which starts from P1 (Goals) and re-generates everything — the confirmed plan is lost.

**Design intent (§10.1)**: Confirmation → use confirmed Goals/Tasks → skip P1/P2 → directly execute P3.

**Fix**: `advanceExecution` must inject `record.Goals` and `record.Tasks` into the `TriggerInput` or use a dedicated `Resume`-like path that skips P1/P2 when Goals/Tasks already exist.

**Test required**:
- Confirm with pre-existing Goals/Tasks → verify P3 uses those Goals/Tasks, not re-generated ones
- Confirm without Goals/Tasks → verify normal P1→P2→P3 flow

---

### BUG-2 [P0] `standard.New()` creates orphan Executor instances

**Files**: `manager/interact.go:501, 525, 544`

**Problem**: `skipWaitingTask()`, `resumeWithContext()`, and `directResume()` all call `standard.New()`, creating a fresh Executor with independent counters. Consequences:
1. `currentCount` / `execCount` not shared — monitoring inaccurate
2. No `execController.Untrack()` after Resume completes — memory leak
3. Separate `store` / `robotStore` instances (less critical, stateless)

**Fix**: Manager should hold a reference to the live Executor (obtained via Pool) and expose a `Resume` method, or provide the Executor as a constructor parameter.

**Tests required**:
- Resume via `skipWaitingTask` → verify `execController.Untrack()` called
- Resume via `resumeWithContext` → verify executor `currentCount` incremented/decremented correctly

---

### BUG-3 [P1] `buildRobotStatusSnapshot` returns near-empty snapshot

**File**: `manager/interact.go:266-278`

**Problem**: Only populates `ActiveCount` and `MaxQuota`. Missing: `WaitingCount`, `QueuedCount`, `ActiveExecs`, `RecentExecs`. Host Agent cannot make informed decisions about robot workload.

**Fix**: Query `robot.Executions` to compute `WaitingCount`, collect `ActiveExecs` briefs, and optionally query recent completed executions from store.

**Tests required**:
- Robot with 2 running + 1 waiting execution → snapshot reflects correct counts
- Robot with no executions → all counts zero

---

### BUG-4 [P1] `openapi/agent/robot/interact.go` passes empty Context to Manager

**File**: `openapi/agent/robot/interact.go:67, 152, 209`

**Problem**: `ctx := &robottypes.Context{}` — no `Auth`, no `context.Context`. When `HandleInteract` → `createConfirmingExecution` calls `ctx.UserID()`, returns `""`. The `TriggerInput.UserID` in the DB record is empty.

**Note**: This is NOT about Robot's own Auth (which is always set via `buildRobotAuth` in execution paths). This is about tracking **which human user** initiated the interaction.

**Fix**: In V2 interact handlers, construct Context properly:
```go
ctx := robottypes.NewContext(c.Request.Context(), &oauthtypes.AuthorizedInfo{
    UserID: authInfo.UserID,
    TeamID: authInfo.TeamID,
})
```

**Tests required**:
- InteractRobot handler → verify ctx.UserID() returns the authenticated user's ID
- CreateConfirmingExecution → verify record.Input.UserID is populated

---

### BUG-5 [P2] `HostContext.Goals` type mismatch with design

**File**: `types/host.go:15`

**Problem**: Design §5.7 defines `Goals string`, implementation uses `*Goals` (struct with `Content` field). Host Agent receives `{"goals": {"content": "..."}}` instead of `{"goals": "..."}`.

**Fix**: Either update the Host Agent prompt to expect the struct format, or flatten to `string` in `buildHostContext`:
```go
if record.Goals != nil {
    hostCtx.GoalsContent = record.Goals.Content  // string
}
```

**Tests required**:
- `buildHostContext` with Goals → verify JSON output matches Host Agent prompt expectations

---

## 2. Missing Unit Tests

All tests should be **black-box** tests (test exported APIs only), must **verify return values and side effects**, and must **not require real LLM calls**.

### 2.1 `executor/standard/host.go` — CallHostAgent

**Current coverage**: 0 tests

| # | Test Case | Verify |
|---|-----------|--------|
| H1 | Robot is nil | Returns error "robot cannot be nil" |
| H2 | No Host Agent configured (empty Resources) | Returns error "no Host Agent configured" |
| H3 | Valid Host Agent call returns JSON | Parsed `HostOutput` with correct Action and Reply |
| H4 | Host Agent returns non-JSON text | Fallback to `HostActionConfirm` with text as Reply |
| H5 | Host Agent returns invalid JSON structure | Fallback to `HostActionConfirm` |
| H6 | Host Agent call fails (network error) | Returns wrapped error |
| H7 | Input marshalling (verify HostInput fields) | Correct JSON sent to agent |

**Status**: ✅ All tests implemented. H1-H2, H7 are pure unit tests. H3-H5 use real LLM integration via `yao-dev-app` test assistants (`tests.host-json`, `tests.host-plaintext`, `tests.host-badjson`). H6 uses real assistant framework.

---

### 2.2 `manager/interact.go` — processHostAction (all branches)

**Current coverage**: 2/7 branches (WaitForMore, default)

| # | Test Case | Action | Verify |
|---|-----------|--------|--------|
| PA1 | HostActionConfirm | `confirm` | `resp.Status == "confirmed"`, `advanceExecution` called |
| PA2 | HostActionAdjust with goals data | `adjust` | Record Goals updated, `resp.Status == "adjusted"` |
| PA3 | HostActionAdjust with tasks data | `adjust` | Record Tasks updated |
| PA4 | HostActionAdjust with nil data | `adjust` | No error, noop |
| PA5 | HostActionAddTask | `add_task` | New task appended to record.Tasks with generated ID |
| PA6 | HostActionAddTask with nil data | `add_task` | Returns error "task data is required" |
| PA7 | HostActionSkip with waiting task | `skip` | Task status = skipped |
| PA8 | HostActionSkip without waiting task | `skip` | Returns error "no task is waiting" |
| PA9 | HostActionInjectCtx with string reply | `inject_context` | Resume called with correct reply |
| PA10 | HostActionInjectCtx → re-suspend | `inject_context` | `resp.Status == "waiting"` |
| PA11 | HostActionCancel | `cancel` | Execution status = cancelled, event pushed |
| PA12 | WaitForMore = true | — | `resp.Status == "waiting_for_more"`, `resp.WaitForMore == true` |
| PA13 | Unknown action | — | `resp.Status == "acknowledged"` |

**Note**: PA1, PA7, PA9, PA11 require mocking Executor.Resume and Pool.SubmitWithID.

---

### 2.3 `manager/interact.go` — HandleInteract routing

**Current coverage**: Parameter validation only

| # | Test Case | Verify |
|---|-----------|--------|
| HI1 | No execution_id → creates confirming execution | Record saved with status=confirming, Host Agent called with "assign" |
| HI2 | execution_id with status=confirming | Host Agent called with "assign" scenario |
| HI3 | execution_id with status=waiting | Host Agent called with "clarify" scenario |
| HI4 | execution_id with status=running | Host Agent called with "guide" scenario |
| HI5 | execution_id with status=completed | Returns error "cannot interact" |
| HI6 | execution_id not found | Returns error "execution not found" |
| HI7 | Host Agent unavailable → direct assign fallback | Execution started without Host Agent |
| HI8 | Host Agent unavailable → direct resume fallback | Execution resumed directly |

---

### 2.4 `manager/interact.go` — CancelExecution

**Current coverage**: "manager not started" only

| # | Test Case | Verify |
|---|-----------|--------|
| CE1 | Cancel waiting execution | Status → cancelled, `Untrack` called, event pushed |
| CE2 | Cancel confirming execution | Status → cancelled |
| CE3 | Cancel running execution | Returns error (only waiting/confirming allowed) |
| CE4 | Cancel non-existent execution | Returns error "execution not found" |
| CE5 | Cancel already cancelled | Returns error |

---

### 2.5 `executor/standard/executor.go` — Resume method

**Current coverage**: Only via E2E tests (requires real LLM)

| # | Test Case | Verify |
|---|-----------|--------|
| R1 | Resume non-waiting execution | Returns error "not in waiting status" |
| R2 | Resume non-existent execution | Returns error "execution not found" |
| R3 | Resume with nil store | Returns error "store is required" |
| R4 | Resume injects reply into task messages | `exec.Tasks[i].Messages` contains `[Human reply]` prefixed message |
| R5 | Resume clears waiting fields | `WaitingTaskID`, `WaitingQuestion`, `WaitingSince` all empty after resume |
| R6 | Resume updates status to running | `exec.Status == ExecRunning` |
| R7 | Resume → re-suspend | Returns `ErrExecutionSuspended`, execution stays tracked |
| R8 | Resume → complete → P4 → P5 | Status == ExecCompleted, `ResumeContext` cleared |
| R9 | Resume → P3 error | Status == ExecFailed with error message |
| R10 | Resume maintains executor currentCount | `currentCount +1 before, -1 after` |

**Note**: R4-R10 require mocking store.Get, store.UpdateResumeState, RunExecution, runPhase.

---

### 2.6 `manager/interact.go` — Helper methods

**Current coverage**: buildRobotStatusSnapshot (3), findWaitingTask (3), buildHostContext (2)

| # | Missing Test Case | Verify |
|---|-------------------|--------|
| HL1 | `createConfirmingExecution` | Record has correct fields (execID, chatID, status=confirming, input) |
| HL2 | `adjustExecution` with goals string | `record.Goals.Content` updated |
| HL3 | `adjustExecution` with tasks array | `record.Tasks` replaced |
| HL4 | `adjustExecution` with non-map data | Graceful handling |
| HL5 | `injectTask` with valid task | Task appended with auto-generated ID |
| HL6 | `injectTask` preserves existing tasks | len(tasks) == original + 1 |
| HL7 | `callHostAgentForScenario` — no host agent | Returns error |
| HL8 | `directAssign` | Returns "confirmed" status |
| HL9 | `directResume` — re-suspend | Returns "waiting" status |
| HL10 | `directResume` — complete | Returns "resumed" status |

---

### 2.7 `api/interact.go` — Interact/Reply/Confirm/CancelExecution

**Current coverage**: 0 tests

| # | Test Case | Verify |
|---|-----------|--------|
| AI1 | `Interact` with manager available | Delegates to `managerInteract` |
| AI2 | `Interact` without manager, with execution_id | Delegates to `legacyResume` |
| AI3 | `Interact` without manager, without execution_id | Returns error |
| AI4 | `Interact` with empty member_id | Returns error |
| AI5 | `Interact` with nil request | Returns error |
| AI6 | `Reply` shortcut | Calls Interact with correct TaskID and Source |
| AI7 | `Confirm` shortcut | Calls Interact with correct Action |
| AI8 | `CancelExecution` with manager | Delegates correctly |
| AI9 | `CancelExecution` without manager | Returns error |
| AI10 | `legacyResume` → success | Returns "resumed" status |
| AI11 | `legacyResume` → re-suspend | Returns "waiting" status |
| AI12 | `legacyResume` → error | Returns wrapped error |

---

### 2.8 `events/events.go` + `events/handlers.go` — Event integration

**Current coverage**: DeliveryHandler basic (3 tests)

| # | Missing Test Case | Verify |
|---|-------------------|--------|
| EV1 | DeliveryHandler — payload deserialization | All fields (`ExecutionID`, `MemberID`, `Content`, `Preferences`) correctly parsed |
| EV2 | Verify event constants match design §7.2 | All 9 constants present and correctly named |
| EV3 | `NeedInputPayload` marshalling | Correct JSON roundtrip |
| EV4 | `TaskPayload` marshalling | Correct JSON roundtrip with optional Error field |
| EV5 | `ExecPayload` marshalling | Correct JSON roundtrip |

---

### 2.9 `openapi/agent/robot/interact.go` — HTTP handlers

**Current coverage**: 0 tests

| # | Test Case | Verify |
|---|-----------|--------|
| OH1 | `InteractRobot` — valid request | 200 with InteractResponse |
| OH2 | `InteractRobot` — missing robot ID | 400 error |
| OH3 | `InteractRobot` — missing message | 400 error |
| OH4 | `InteractRobot` — robot not found | 404 error |
| OH5 | `InteractRobot` — forbidden (no write permission) | 403 error |
| OH6 | `ReplyToTask` — valid request | 200 with response |
| OH7 | `ReplyToTask` — missing params | 400 error |
| OH8 | `ConfirmExecution` — valid request | 200 with response |
| OH9 | `ConfirmExecution` — empty body allowed | 200 (confirm without message) |

---

### 2.10 Event push verification in execution flow

**Current coverage**: 0 (events are pushed but never verified in tests)

| # | Test Case | File | Verify |
|---|-----------|------|--------|
| EP1 | Task completes → TaskCompleted event | `run.go:111` | Event type + payload fields |
| EP2 | Task fails → TaskFailed event | `run.go:120` | Event type + error in payload |
| EP3 | Execution suspends → ExecWaiting event | `executor.go:750` | Event type + question in payload |
| EP4 | Execution resumes → ExecResumed event | `executor.go:856` | Event type + chatID |
| EP5 | Execution completes → ExecCompleted event | `executor.go:287` | Event type + status |
| EP6 | Execution cancelled → ExecCancelled event | `manager/interact.go:66` | Event type + status |
| EP7 | Delivery → Delivery event | `delivery.go:102` | Content + Preferences in payload |

**Approach**: Use `event.Subscribe` in test to capture pushed events, or mock `event.Push`.

---

## 3. Code Quality Improvements

### CQ1 — Extract common Executor resume logic

`skipWaitingTask`, `resumeWithContext`, `directResume` all duplicate: create executor → call Resume → handle ErrExecutionSuspended. Extract to a private helper:

```go
func (m *Manager) executeResume(ctx *types.Context, execID, reply string) error {
    // Use shared executor reference, not standard.New()
    return m.getExecutor().Resume(ctx, execID, reply)
}
```

### CQ2 — `processHostAction` needs explicit `store.Save()` after Confirm

`advanceExecution` changes execution status but doesn't save the Goals/Tasks that may have been set during confirming flow. Needs explicit persist before Pool submit.

### CQ3 — `RobotStatusSnapshot` should include `MemberID` and `Status`

Add back `MemberID` and `Status` fields to match design §5.7. These help Host Agent identify which robot it's serving.

---

## 4. Implementation Priority

| Priority | Items | Est. Effort |
|----------|-------|-------------|
| **P0** | BUG-1 (advanceExecution), BUG-2 (standard.New) | 1 day |
| **P1** | BUG-3 (snapshot), BUG-4 (context auth) | 0.5 day |
| **P1** | Tests §2.2 (processHostAction), §2.3 (HandleInteract), §2.5 (Resume) | 1.5 days |
| **P2** | BUG-5 (Goals type), CQ1-CQ3 | 0.5 day |
| **P2** | Tests §2.1 (CallHostAgent), §2.4 (Cancel), §2.6-2.10 | 2 days |
| | **Total** | **~5.5 days** |

---

## 5. Test Infrastructure Notes

1. **Source env before test**: `source yao/env.local.sh`
2. **Test app**: `yao-dev-app` — all test assistants live there
3. **No recompile needed**: `yao-dev` runs from Go source directly
4. **Mock strategy**: For unit tests not requiring real LLM, create interfaces for `ConversationCaller`, `ExecutionStore`, `Pool` to enable mock injection. Alternatively, use `SkipPersistence: true` config + in-memory stubs.
5. **Event verification**: Wrap `event.Push` calls with a test interceptor or use `event.Subscribe` to capture events during test.
