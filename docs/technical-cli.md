# 7. Technical CLI

This document defines the Argus CLI surface: command groups, flags, exit-code conventions, and output formats.

## 7.1 Command Categories

Argus CLI commands fall into two groups, distinguished by how they appear in help output:

- **External commands**: intended for direct human use; mainly lifecycle management and diagnostics. These appear in `argus help`
- **Internal commands**: intended for AI agents, hook systems, or automation. These appear in `argus help --all`

### 7.1.1 Scope Awareness

All commands except explicitly scope-free ones are scope-aware. During execution, Argus resolves scope from `cwd` through `scope.ResolveScope(cwd)`:

- **Project scope**: when the current working directory belongs to an initialized project
- **Global/workspace scope**: when the current working directory is inside a registered workspace path
- **No scope**: when neither condition applies. In that case, most internal commands such as `status` or `workflow` respond with guidance appropriate to the situation

This model lets a single command set load the correct artifact root automatically.

Argus uses an agent-centric interaction model. The external command set stays deliberately small and focuses on install/uninstall, `doctor`, `version`, and `help`.

## 7.2 External Commands

| Command | Description | Exit codes |
| :--- | :--- | :--- |
| `argus install [--yes] [--json]` | Install Argus into the current project. Create `.argus/`, configure agent `tick` hooks, and release built-in skills. The command is idempotent. The installer merges configuration and preserves existing non-Argus hooks. When run from a subdirectory, confirmation may be required unless `--yes` is used (see [workspace §10.3.1](technical-workspace.md) for details). Default output is human-readable text; `--json` returns a structured result. | `0` success, `1` failure |
| `argus install --workspace <path> [--yes] [--json]` | Register a workspace path. Install global `tick` hooks, global skills, and global-scope artifacts under `~/.config/argus/`. See [technical-workspace.md](technical-workspace.md) for runtime orchestration semantics. Confirmation is required unless `--yes` is used. Default output is text; `--json` returns a structured result. | `0` success, `1` failure |
| `argus uninstall [--yes] [--json]` | Remove Argus from the current project. Delete `.argus/`, remove Argus-managed project-level skills (`.agents/skills/argus-*` and `.claude/skills/argus-*`; non-`argus-` prefixed user skills are preserved), and remove Argus hook configuration. Git-tracked files can be restored through Git if needed. Confirmation is required unless `--yes` is used. | `0` success, `1` failure |
| `argus uninstall --workspace <path> [--yes] [--json]` | Remove one registered workspace path. If no workspaces remain, also remove global hooks and global skills. Confirmation is required unless `--yes` is used. | `0` success, `1` failure |
| `argus doctor [--json]` | Diagnose Argus installation and configuration health. Reports only, never repairs. Default output is a human-readable report; `--json` returns structured data. | `0` all checks passed, `1` findings present |
| `argus version [--json]` | Show the current version. Default output is brief text; `--json` returns structured data. | Always `0` |
| `argus help [--all]` | Show help. `--all` includes internal commands. | Always `0` |

## 7.3 Internal Commands

| Command | Description |
| :--- | :--- |
| `argus tick` | Passive coordination point triggered whenever the user sends input to the agent. Checks state and injects context. |
| `argus trap` | Reserved operation-gating entry point. In Phase 1 it always allows operations and is not installed by `argus install`. |
| `argus job-done [--fail] [--end-pipeline] [--message "..."] [--json]` | Report that the current job is finished. `--fail` marks failure. `--end-pipeline` ends the pipeline early (defaults to success; combined with `--fail` it becomes an early failure). `--message` records an optional summary. Default output is readable text; `--json` returns structured data. |
| `argus status [--json]` | Show a project-level overview including pipeline progress and invariant status. Runs real-time invariant checks. |
| `argus workflow start <workflow-id> [--json]` | Start a workflow. Phase 1 enforces a single active pipeline: if another pipeline is already running, Argus returns an error and asks the caller to finish or cancel it first. |
| `argus workflow list [--json]` | List available workflows. |
| `argus workflow cancel [--json]` | Cancel the active pipeline. If multiple running pipelines are found (an anomaly), cancel all of them. When no active pipeline exists, `--json` returns an error envelope and exit code 1. |
| `argus workflow snooze --session <id> [--json]` | Temporarily ignore the active pipeline in the current session. Later `tick` calls skip its context until a new session begins. |
| `argus workflow inspect [dir] [--json]` | Validate workflow files and cross-file consistency. Defaults to `.argus/workflows/`. |
| `argus invariant check [id] [--json]` | Execute one or all invariant shell checks. |
| `argus invariant list [--json]` | List all invariants in the current project or scope. |
| `argus invariant inspect [dir] [--json]` | Validate invariant files and cross references. Defaults to `.argus/invariants/`. |
| `argus toolbox <tool> [args]` | Run bundled utility tools such as `jq`, `yq`, `touch-timestamp`, or `sha256sum`. |

## 7.3.1 Output Modes for Human-Facing Commands

Except for protocol-style or utility commands such as `tick`, `trap`, and `toolbox`, human-facing Argus commands follow a dual-output model:

- **Default output**: human-readable plain text. Markdown-like headings, lists, and sections are acceptable because they are also easy for agents to interpret
- **`--json` output**: structured JSON for scripts, field-level parsing, and automation
- **Errors**: in default mode, errors go to `stderr`; in `--json` mode, the command returns the standard error envelope
- **No `--markdown` mode**: the old agent-friendly text output is now the default behavior, so a separate markdown mode would only split the contract unnecessarily

## 7.3.2 Successful `--json` Output for Lifecycle Commands

`install` and `uninstall` return a common JSON envelope on success:

- `status: "ok"`
- `message`: success summary
- `root`: returned only by project-level install
- `path`: returned only by workspace install or uninstall; the normalized workspace path
- `changes`: grouped filesystem changes under `created`, `updated`, and `removed`
- `affected_paths`: a stable summary of managed paths

Notes:

- `changes` should list what actually changed in the current invocation, and may be empty in idempotent cases
- `affected_paths` is a stable summary and may merge multiple concrete filesystem paths into a single logical item such as `.agents/skills/argus-*`

## 7.4 Removed Commands and Why

The following commands were removed during design iteration:

- `job current`: superseded by `tick` for passive reminders and `status` for active inspection
- `job done` / `job fail`: unified as `job-done` with flags
- `info`: static information belongs to `doctor` and `version`; runtime information belongs to `status`
- `rules regenerate`: rule regeneration should happen through a workflow, not a dedicated subcommand
- `rules check`: freshness checks are covered by invariants and `tick`
- `rules list`: agents can already read `.argus/rules/` directly

## 7.4.1 Toolbox Specification

`argus toolbox <tool> [args]` is a small built-in utility suite in busybox style: one binary, multiple embedded tools, less dependency on host-machine tooling.

### Phase 1 Tool Set

| Tool | Go implementation | Purpose |
|------|-------------------|---------|
| `jq` | `itchyny/gojq` | Query JSON output such as Argus command results |
| `yq` | `mikefarah/yq` | Read YAML workflow or invariant definitions |
| `touch-timestamp` | built-in | Write the current compact UTC timestamp into a target file |
| `sha256sum` | `crypto/sha256` | Compute SHA256 for files or stdin using a coreutils-compatible output style |

### Usage Examples

```bash
argus toolbox jq '.status' pipeline.json
argus toolbox yq '.jobs[0].id' workflow.yaml
argus toolbox touch-timestamp .argus/data/lint-passed
```

### Design Choices

- stdout, stderr, and exit codes are forwarded directly from the underlying implementation
- primary use cases are invariant shell checks and hook wrappers parsing Argus output
- new tools can be added later without changing the user-facing invocation model

## 7.5 Global Flags

| Flag | Description |
| :--- | :--- |
| `--agent <name>[,<name>...]` | Select target agents (`claude-code`, `codex`, `opencode`). Used by `tick` and `trap` to parse incoming hook JSON, and by `install`/`uninstall` to select which agent configs to manage. Multiple values are comma-separated. When omitted, Argus operates on all known agents where applicable. |
| `--global` | Used only by `tick` and `trap`. Marks that the invocation came from a global hook configuration. Written automatically by `install --workspace`. |

## 7.6 Exit-Code Conventions

### Hook Commands (`tick` / `trap`)

- **Always exit 0 (fail open)**: hook commands must not block the agent merely because Argus encountered an internal error
- **Internal error handling**: surface problems as warning text inside the emitted context rather than using a failing exit code
- **Never use exit code 2**: in Claude Code and Codex, exit code 2 has special blocking semantics, so Argus must not use it accidentally
- **Trap blocking**: use JSON fields such as `permissionDecision: "deny"` rather than exit codes to block an operation

### Unified Envelope for `--json`

Commands supporting `--json` share a common outer structure:

- **Success**: `{"status":"ok", ...}`
- **Business error**: `{"status":"error","message":"..."}` plus exit code 1

Specific commands define their own inner fields (see §8.2 `workflow start`, §8.3 `job-done`, and §8.4 `status` for detailed schemas), but the envelope and exit-code meaning remain stable. Other commands (`workflow list/cancel/snooze`, `invariant list/check/inspect`) follow the same envelope.

### Common Command Rules

- Human-facing and internal commands that support `--json` return `0` on success and `1` on business errors such as invalid parameters or invalid state
- `install` / `uninstall`: `0` success, `1` failure
- `version` / `help`: always `0`
- `doctor`: `0` when all checks pass, `1` when findings exist

### Hook-System Exit-Code Reference

| Exit code | Claude Code behavior | Codex behavior |
| :--- | :--- | :--- |
| 0 | success, stdout parsed | success, stdout parsed |
| 2 | block operation, stderr shown as reason | block operation, stderr shown as reason |
| non-zero other than 2 | non-blocking hook error, agent continues | command error |

# 8. Output Formats

## 8.1 `tick` Output (Five Scenarios)

`argus tick --agent <name>` returns plain text. Wrappers then inject that text into each host agent’s own context mechanism.

**Compatibility rule**: the first non-whitespace character must not be `[` or `{`. Current Codex may interpret those prefixes as JSON candidates and reject otherwise valid text output.

### Scenario 1: No Active Pipeline

```markdown
Argus: No active pipeline.

Available workflows:
  - release: Standard release process
  - argus-init: Initialize Argus for the project

To start: argus workflow start <workflow-id>
```

### Scenario 2: Active Pipeline, State Changed (Full Context)

```markdown
Argus: Pipeline: release-20240405T103000Z | Workflow: release | Progress: 2/5

Current Job: run_tests
Skill: argus-run-tests

Run all tests and continue only if they pass.

When done: argus job-done [--message "summary"]
To snooze: argus workflow snooze --session ses_abc123
To cancel: argus workflow cancel
```

### Scenario 3: Active Pipeline, State Unchanged (Minimal Summary)

```markdown
Argus: release | Job: run_tests | Progress: 2/5 — When done: argus job-done
```

### Scenario 4: Snoozed

If the current pipeline has been snoozed in the current session, output becomes equivalent to the “No active pipeline” case.

### Scenario 5: First Tick Plus Failed Invariant

Append invariant failure information after the base scenario output:

```markdown
Argus: Invariant check failed:
  - argus-init: The project has completed Argus initialization
    Suggestion: Run argus workflow start argus-init
```

## 8.2 `workflow start` Output

`workflow start` reuses the same structure as a successful `job-done` with a next job: it is another way of delivering the next job payload.

**Default text**

```markdown
Argus: Pipeline release-20240405T103000Z started (1/5)

Current job: lint
Prompt: Run lint checks and fix any issues.
Skill: argus-run-lint

When complete, run: argus job-done --message "execution summary"
```

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline_status": "running",
  "progress": "1/5",
  "next_job": {
    "id": "lint",
    "prompt": "Run lint checks and fix any issues.",
    "skill": "argus-run-lint"
  }
}
```

## 8.3 `job-done` Output (Six Scenarios)

### Scenario 1: Success, Next Job Exists

**Default text**

```markdown
Argus: Job run_tests completed (3/5)

Next job: deploy
Prompt: Deploy the build artifacts to staging.

When complete, run: argus job-done --message "execution summary"
```

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline_status": "running",
  "progress": "3/5",
  "next_job": {
    "id": "deploy",
    "prompt": "Deploy the build artifacts to staging.",
    "skill": null
  }
}
```

### Scenario 2: Success, Last Job Completed

**Default text**

```markdown
Argus: Job deploy completed (5/5)
Pipeline release-20240405T103000Z is complete.
```

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline_status": "completed",
  "progress": "5/5",
  "next_job": null
}
```

### Scenario 3: Early Success (`--end-pipeline`)

**Default text**

```markdown
Argus: Job run_tests completed. Pipeline ended early (2/5).
```

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline_status": "completed",
  "progress": "2/5",
  "early_exit": true,
  "next_job": null
}
```

### Scenario 4: Failure (`--fail`)

**Default text**

```markdown
Argus: Job run_tests marked as failed. Pipeline stopped (2/5).

Available actions:
- Restart: argus workflow start release
- Cancel: argus workflow cancel
```

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline_status": "failed",
  "progress": "2/5",
  "failed_job": "run_tests",
  "next_job": null
}
```

### Scenario 5: No Active Pipeline

**Default text**

```markdown
Argus: No active pipeline.
Start one with argus workflow start <workflow-id>.
```

**JSON (`--json`)**

```json
{
  "status": "error",
  "message": "No active pipeline. Start one with argus workflow start <workflow-id>."
}
```

`job-done` is an internal command, not a hook command, so “no active pipeline” is a normal business error and returns exit code 1.

### Scenario 6: Early Failure (`--fail --end-pipeline`)

**Default text**

```markdown
Argus: Job run_tests marked as failed. Pipeline ended early (2/5).

Available actions:
- Restart: argus workflow start release
- Cancel: argus workflow cancel
```

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline_status": "failed",
  "progress": "2/5",
  "early_exit": true,
  "failed_job": "run_tests",
  "next_job": null
}
```

## 8.4 `status` Output

`argus status` provides a project-level overview with three dimensions:

- pipeline state
- invariant state
- general hints

### With an Active Pipeline

**Default text**

```markdown
Argus: Project status

Pipeline: release-20240115T103000Z (running) - Workflow: release - Progress 2/5
  1. [done] lint - All lint checks passed
  2. [>>]   run_tests
  3. [ ]    build
  4. [ ]    deploy_staging
  5. [ ]    deploy_prod

Invariants: 2 passed, 1 failed
  [FAIL] lint-clean: Lint passed within the last 24 hours
```

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline": {
    "workflow_id": "release",
    "status": "running",
    "current_job": "run_tests",
    "started_at": "20240115T103000Z",
    "ended_at": null,
    "progress": {
      "current": 2,
      "total": 5
    },
    "jobs": [
      {"id": "lint", "status": "completed", "message": "All lint checks passed"},
      {"id": "run_tests", "status": "in_progress", "message": null},
      {"id": "build", "status": "pending", "message": null},
      {"id": "deploy_staging", "status": "pending", "message": null},
      {"id": "deploy_prod", "status": "pending", "message": null}
    ]
  },
  "invariants": {
    "passed": 2,
    "failed": 1,
    "details": [
      {"id": "argus-init", "description": "The project has completed Argus initialization", "status": "passed"},
      {"id": "lint-clean", "description": "The codebase should pass lint", "status": "failed"},
      {"id": "gitignore-complete", "description": ".gitignore should include Argus temporary files", "status": "passed"}
    ]
  },
  "hints": [
    "Invariant checks took 3.2s total. Run argus doctor to investigate slow checks."
  ]
}
```

Notes:

- `pipeline.jobs` lists **all** jobs in the workflow definition, not only jobs that have already run
- `invariants.details` includes every invariant whose `auto` value is not `never`
- `description` comes from top-level invariant YAML `description`; if missing, Argus may fall back to a shell-summary string
- `hints` is a general-purpose array for performance warnings and other guidance

### With No Active Pipeline

**JSON (`--json`)**

```json
{
  "status": "ok",
  "pipeline": null,
  "invariants": {
    "passed": 3,
    "failed": 0,
    "details": [
      {"id": "argus-init", "description": "The project has completed Argus initialization", "status": "passed"},
      {"id": "lint-clean", "description": "The codebase should pass lint", "status": "passed"},
      {"id": "gitignore-complete", "description": ".gitignore should include Argus temporary files", "status": "passed"}
    ]
  },
  "hints": []
}
```

## 8.5 Other Command Outputs

`workflow list/inspect`, `invariant list/check/inspect`, and `workflow cancel/snooze` follow the same general rule:

- **Default**: readable plain text for both humans and agents
- **`--json`**: structured JSON with a top-level `"status": "ok"` or `"status": "error"` envelope

Phase 1 keeps some of these command-specific field definitions relatively lightweight in documentation, but the envelope and exit-code behavior must remain stable.
