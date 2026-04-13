# Pipeline and Session State Management

This document defines the Pipeline runtime data model, session management, and state-tracking behavior. These designs follow the “artifacts as ground truth” principle so the system stays predictable and resilient.

---

## 5.1 Pipeline Data File Schema

Pipeline runtime state is recorded in standalone YAML files rather than in a centralized state database.

- **Instance ID**: `<workflow-id>-<compact-UTC-timestamp>` such as `release-20240115T103000Z`, **without** the `.yaml` suffix. This is the logical identifier used in session files, CLI output, and logs.
- **File path**: `.argus/pipelines/<instance-id>.yaml`, for example `release-20240115T103000Z.yaml`. The `.yaml` suffix is only used for filesystem access.
- **Uniqueness**: Argus checks whether the file already exists before creation. Under Phase 1 single-active-pipeline enforcement, duplicate starts in the same second should not happen.
- **Version**: `version: v0.1.0`

### Design Decisions

- **Do not embed full workflow definitions in pipeline files**: the file stores only `workflow_id`. Argus loads the workflow definition from `.argus/workflows/` at runtime. This avoids duplication and makes workflow definition changes visible immediately. Temporary workflow snapshots are not supported in Phase 1.
- **Workflow modified during execution**: every `tick` and `job-done` reloads the workflow definition. If jobs are deleted, renamed, or reordered while a pipeline is running, the new definition becomes authoritative. If `current_job` no longer exists, `tick` emits a text error, `job-done` returns an error envelope, and the user is guided to use `argus workflow cancel`. `status` uses best-effort behavior: it still returns pipeline basics and invariant results, but sets `current_job` to `null` and adds a hint that the workflow definition may have changed. This is a known Phase 1 limitation.
- **Use one global `current_job` instead of persistent per-job statuses**: in a sequential execution model, job status can be derived from the current pointer. This reduces state duplication and synchronization risk.

### YAML Example

```yaml
version: v0.1.0
workflow_id: release
status: running                    # running | completed | failed | cancelled
current_job: run_tests             # current job id; null when completed
started_at: "20240115T103000Z"
ended_at: null

jobs:
  lint:
    started_at: "20240115T103005Z"
    ended_at: "20240115T103100Z"
    message: "All lint checks passed"
  run_tests:
    started_at: "20240115T103105Z"
    ended_at: null
```

### Field Definition Table

| Field | Type | Required | Nullable | Created by | Updated by |
|------|------|----------|----------|------------|------------|
| `version` | string | Yes | No | `workflow start` | Never |
| `workflow_id` | string | Yes | No | `workflow start` | Never |
| `status` | enum | Yes | No | `workflow start` (`running`) | `job-done`, `workflow cancel` |
| `current_job` | string | Yes | Yes | `workflow start` (first job) | `job-done`; set to `null` on completion |
| `started_at` | timestamp | Yes | No | `workflow start` | Never |
| `ended_at` | timestamp | Yes | Yes | `workflow start` (`null`) | set when pipeline ends |
| `jobs` | map | Yes | No | `workflow start` | appended as jobs begin |
| `jobs.<id>.started_at` | timestamp | Yes | No | when the job begins | Never |
| `jobs.<id>.ended_at` | timestamp | Yes | Yes | job start (`null`) | `job-done` |
| `jobs.<id>.message` | string | No | Yes | — | `job-done --message` |

`jobs` uses an append-on-demand strategy. `workflow start` creates only the first job entry. Later job entries are created when `job-done` advances the pipeline.

### Status Interpretation

- **`status: completed` + `current_job: null`**: all jobs completed successfully
- **`status: failed` + `current_job: <job-id>`**: execution failed while on that job
- **`status: cancelled`**: the pipeline was stopped externally

---

## 5.2 Pipeline Lifecycle

### Start

`argus workflow start <workflow-id>` creates a pipeline. If `.argus/pipelines/` does not exist, Argus creates it automatically. A new pipeline data file is written and the first job is returned.

**Phase 1 single-active rule**: if another pipeline already has `status: running`, `workflow start` returns an error and asks the caller to finish or cancel the current pipeline first. If directory scanning finds multiple running pipelines, that is treated as an anomaly and should be reported by `doctor`.

### Progression

After a job is finished, the agent calls `argus job-done`. Argus updates job metadata and advances `current_job` to the next workflow step.

**Future direction**: after Phase 1, multiple active pipelines could be supported by adding `--pipeline <instance-id>` selectors to commands such as `workflow cancel`, `snooze`, `status`, and `tick`. The file format and scanning model already support that future expansion without migration.

---

## 5.3 Early Termination (`--end-pipeline`)

Sometimes the agent may determine that later jobs are no longer necessary, for example when there are no actual code changes in a release flow.

- **Command**: `argus job-done --end-pipeline`
- **Behavior**: record the current job as finished, mark the pipeline as `completed`, and set `current_job` to `null`
- **Naming rationale**: `--finish` was considered but rejected because it could mean either “finish this job” or “finish the whole pipeline”. `--end-pipeline` is explicit at the workflow-instance level.

---

## 5.4 Job Failure and Recovery

### Reporting Failure

When the agent encounters a problem it cannot safely resolve, it calls:

```bash
argus job-done --fail --message "reason for failure"
```

- **State change**: pipeline `status` becomes `failed`
- `current_job` remains on the failed job

### Recovery Options

1. **Leave it failed**: a failed pipeline may remain in that state without blocking new work forever
2. **Cancel it**: use `argus workflow cancel`
3. **Restart from the beginning**: run `argus workflow start` again

**Phase 1 does not support `resume`**. Restart is considered sufficient for the early model, and job prompts should be written so the agent can recognize already-completed work and skip unnecessary repetition.

### Agent Judgment and Automatic Recovery

The agent should not immediately call `--fail` for every obstacle. It may retry or ask the user for help. Failure should be reported only when the agent decides it cannot safely proceed.

If a session ends without a `job-done` call, the pipeline remains `running`. On the next session, `tick` reinjects the current job context and the agent can continue or retry naturally.

---

## 5.4.1 State Transition Table

| Operation | `status` | `current_job` | `ended_at` | current job `ended_at` | current job `message` |
|----------|----------|---------------|------------|-------------------------|-----------------------|
| `workflow start` | `running` | first job | — | — | — |
| `job-done` (middle job) | `running` | next job | — | set | `--message` value |
| `job-done` (last job) | `completed` | `null` | set | set | `--message` value |
| `job-done --end-pipeline` | `completed` | `null` | set | set | `--message` value |
| `job-done --fail` | `failed` | keep current value | set | set | `--message` value |
| `job-done --fail --end-pipeline` | `failed` | keep current value | set | set | `--message` value |
| `workflow cancel` | `cancelled` | keep current value | set | not set | not set |

Notes:

- `--end-pipeline` means an early end. By default that means success (`completed`). Combined with `--fail`, it becomes an early failed termination.
- When `--fail` is used, `current_job` is intentionally preserved to identify where the failure happened.
- `workflow cancel` is an external stop operation and does not rewrite the current job record.

---

## 5.5 Snooze (Session-Level Ignore)

When the user needs to work on something else, an active pipeline can be temporarily silenced through snooze.

- **Trigger**: the agent calls `argus workflow snooze --session <session-id>`
- **Storage**: the pipeline instance ID is appended to `snoozed_pipelines` inside `/tmp/argus/<safe-id>.yaml`
- **Effect**: the pipeline remains globally `running`, but `tick` skips reinjection for the rest of the current session. A new session will surface it again.

---

## 5.6 Cancel

`argus workflow cancel` terminates the current active pipeline by marking it `cancelled`.

### Anomaly Handling

If directory scanning finds multiple `status: running` pipeline files, which should not happen in Phase 1:

- `workflow cancel` cancels all running pipelines
- `workflow snooze` adds all running pipelines to the snoozed list
- `tick`, `status`, and `job-done` should report the anomaly and direct the caller toward `workflow cancel` or `doctor`

### Priority After Snoozing All Running Pipelines

If all running pipelines in the current session are already listed in `snoozed_pipelines`, later `tick` calls should skip quietly without injecting context. `status` and `job-done` should still return anomaly errors because they require a single unambiguous target pipeline. This lets snooze suppress interruption without pretending the anomaly is resolved.

---

## 5.7 Active Pipeline Discovery

Argus discovers active pipelines by **directory scanning**. It traverses `.argus/pipelines/`, reads each file’s `status`, and treats every file with `status: running` as active.

### Corrupt File Handling

If scanning hits a YAML file that cannot be parsed, Argus should skip it and emit a warning in output, recommending `argus doctor`. `doctor` should list the corrupted file path, the parse error, and a recovery suggestion such as manually fixing or deleting the file. If a deleted file represented a running pipeline, the corresponding workflow must be restarted.

### Tradeoff Analysis

| Option | Extends to multiple active pipelines | Consistency risk | Fits artifacts-as-ground-truth | Performance |
|------|--------------------------------------|------------------|-------------------------------|-------------|
| Pointer file (`.active`) | Requires a format change and read-modify-write logic | High | No | `O(1)` |
| Symbolic link | Naming is awkward | High | No | `O(1)` |
| **Directory scan** | **Natural fit** | **None** | **Yes** | **`O(n)`** |
| Fixed-filename rename | Not viable | High | No | `O(1)` |

**Why scanning wins**: it avoids redundant indexes, preserves artifacts as the source of truth, and the cost of scanning a reasonable number of pipeline files is negligible in practice.

---

## 5.8 Pipeline History

- **Shared directory**: active and historical pipelines live in the same directory and differ only by state
- **Retention policy**: Phase 1 keeps all pipeline files. Automatic cleanup is deferred in favor of auditability and debugging value

---

## 5.9 `tick` State-Change Inputs

To avoid repeating identical instructions on every conversation turn, Argus stores `last_tick` in the **session file**, not in the pipeline file. That way a new session still receives full context even when the pipeline itself did not change.

`last_tick` fields:

- `pipeline`: pipeline instance ID without `.yaml`
- `job`: the `current_job` at the previous tick
- `timestamp`: last tick timestamp, mainly for diagnostics

### Injection Logic

| Condition | Interpretation | Injection behavior |
|----------|----------------|--------------------|
| Active pipeline, `last_tick` missing | First tick in session or newly started pipeline | Inject full job context and guidance |
| Active pipeline, state differs from `last_tick` | Job advanced | Inject the new job’s full context |
| Active pipeline, state matches `last_tick` | No change | Inject only a minimal reminder |

This section covers only the session and pipeline inputs that influence the active-pipeline path. The full `tick` routing contract, including no-pipeline, invariant-failure, warning-only, and empty-output cases, is documented in [technical-tick.md](technical-tick.md).

---

## 6. Session State Management

### 6.1 Session Data Storage

Session files store temporary cross-hook state such as snooze markers and first-session checks.

- **Path**: `/tmp/argus/<safe-id>.yaml`
- **`safe-id` rule**: use the agent-provided `session_id` directly if it matches a UUID-like format; otherwise use the first 16 characters of its SHA256 hash
- **Cleanup**: rely on the operating system’s temp-directory cleanup behavior
- **Uniqueness**: UUID or hash-derived names prevent collisions across concurrent sessions and projects

### Path Strategy Tradeoff

| Option | Example | Result |
|------|---------|--------|
| Dedicated directory | `/tmp/argus-<safe-id>/data.yaml` | Rejected: too heavy for a single file |
| Flat file in `/tmp` | `/tmp/argus-session-<safe-id>.yaml` | Rejected: clutters the root temp directory |
| **Grouped directory** | **`/tmp/argus/<safe-id>.yaml`** | **Chosen**: centralized and easy to clean manually |

---

### 6.2 Session Data Contents

```yaml
# /tmp/argus/<safe-id>.yaml
snoozed_pipelines:
  - release-20240115T103000Z
last_tick:
  pipeline: release-20240115T103000Z
  job: run_tests
  timestamp: "20240115T103500Z"
```

- **`snoozed_pipelines`**: list of pipeline instances to suppress during the current session
- **`last_tick`**: the latest pipeline snapshot used to decide whether full reinjection is needed
- **Removed field note**: an earlier `invariant_checked` field was removed. `auto: session_start` execution is now determined by whether the session file exists

---

### 6.3 Session ID Sources

Each supported agent provides a session identifier, and Argus normalizes them through `--agent`:

- **Claude Code**: `session_id`
- **Codex**: `session_id`
- **OpenCode**: `sessionID`

---

### 6.4 First-Tick Detection

Argus distinguishes the first tick in a session from later ticks by checking whether the session file already exists.

```text
tick triggered
  ↓
extract session_id from hook input
  ↓
check whether /tmp/argus/<safe-id>.yaml exists
  ↓
┌────────── missing (first tick) ──────────┐
│ 1. If no active pipeline exists, run     │
│    auto invariants including             │
│    session_start until the first failure │
│ 2. Create the session data file          │
│ 3. Inject pipeline, invariant, workflow, │
│    or no output based on precedence      │
└──────────────────────────────────────────┘
  ↓
┌────────── present (later tick) ──────────┐
│ 1. Skip session_start checks             │
│ 2. Inject pipeline, invariant, workflow, │
│    or no output based on precedence      │
└──────────────────────────────────────────┘
```

### Safety Detail

The session file is created **after** any required invariant checks finish, or immediately on the active-pipeline path where no invariant checks are needed. If the process is interrupted during no-pipeline checking, the next tick will run the full first-session logic again. This avoids accidentally skipping first-session checks because a previous attempt was cut off midway.
