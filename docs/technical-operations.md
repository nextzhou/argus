# Technical Operations and Security Design

This document describes Argus diagnostics, security constraints, and deferred implementation work.

## 12. Doctor Checks

### 12.1 Design Principles

Argus follows architecture invariant #4: diagnostic tools diagnose only and never repair automatically. `doctor` reports problems and suggests actions, but must not execute repair logic or start workflows on its own.

- **Exit-code model**: follow a `git diff --exit-code` style. Return 0 when all checks pass and 1 when any issue is found
- **Two entry points**:
  - `argus doctor`: the main CLI diagnosis command
  - `argus-doctor`: an agent skill used as an alternate path, especially when the Argus binary is broken or missing. Invoked as `/argus-doctor` in Claude Code, `$argus-doctor` or `/use argus-doctor` in Codex, and through the `skill` tool in OpenCode
- **Graceful degradation without the binary**: when the Argus binary is unavailable, `argus-doctor` should mark binary-dependent checks as **skipped** (for example `argus version`, `workflow inspect`, `invariant inspect`, and built-in invariant execution), while still running file-presence, directory-structure, hook-config, and `.gitignore` checks. The final report should summarize `N passed / M failed / K skipped`

### 12.2 Full Checklist

Doctor covers the following 13 dimensions.

#### 1. Argus Installation Completeness

- whether `.argus/` exists
- whether `.argus/workflows/` exists
- whether `.argus/invariants/` exists
- whether `argus version` returns a readable version string

#### 2. Hook Configuration Validation

- **Agent scope rule**: only validate agents for which Argus-owned hook artifacts already exist. If an agent’s config file does not exist, treat that agent as not enabled and skip it without error
- verify that `argus tick` entries exist where expected
- verify that the `argus` binary is discoverable on `PATH`

#### 3. Workflow File Validation

- call `argus workflow inspect`
- validate YAML syntax, schema correctness, and cross-file job references

#### 4. Invariant File Validation

- call `argus invariant inspect`
- validate invariant syntax and schema correctness

#### 5. Built-In Invariant Checks

- execute shell checks for built-in invariants whose IDs begin with `argus-`
- do **not** execute user-defined invariants inside `doctor`; keeping the diagnostic surface predictable is more important than running arbitrary project-specific checks here

#### 6. Skill File Integrity

- verify that Argus-managed project-level skill files exist in `.agents/skills/argus-*/SKILL.md` and `.claude/skills/argus-*/SKILL.md`

#### 7. `.gitignore` Coverage

- confirm that local-only paths are ignored
- required entries: `.argus/pipelines/`, `.argus/logs/`, `.argus/tmp/`
- confirm that `.argus/data/` is **not** ignored, because it is a shared git-tracked data directory

#### 8. Log Health

- read the full project-level log from `.argus/logs/hook.log` when it exists
- otherwise fall back to `~/.config/argus/logs/hook.log`, which may contain pre-install global-hook records
- report `ERROR` records
- `doctor` is a low-frequency command, so it may read complete logs rather than sampling them

#### 9. Version Compatibility

- extract `version` from workflow, invariant, and pipeline files
- compare against the current Argus binary using major-version compatibility

#### 10. Temporary Directory Permissions

- verify that `/tmp/argus/` is writable by creating and deleting a temporary file

#### 11. Pipeline Data Integrity

- identify all `running` pipelines
- verify that each pipeline’s `workflow_id` exists under workflows
- ensure pipeline YAML is parseable

#### 12. Shell Environment Check

- inspect the user’s default shell from `$SHELL`
- if it is not bash, emit a warning that invariant shell checks are always executed with bash, so aliases or shell-specific initialization from another shell may not be available there

#### 13. Workspace-Specific Checks

When workspaces are configured:

- list registered workspaces
- verify that each registered path still exists
- verify global hook configuration
- verify access to `~/.config/argus/`

## 13. Security

### 13.1 Path-Construction Input Validation

To prevent path traversal, any external input used to build a file path must be validated in Go first.

| Input | Use case | Validation |
| :--- | :--- | :--- |
| `session_id` | `/tmp/argus/<safe-id>.yaml` | Prefer UUID format (`^[0-9a-fA-F-]+$`); non-conforming values are SHA256-hashed and the first 16 hex characters are used as `safe-id` |
| `workflow_id` | `.argus/pipelines/<workflow-id>-<timestamp>.yaml` | Must match `^[a-z0-9]+(-[a-z0-9]+)*$` |
| `invariant_id` | `.argus/invariants/<id>.yaml` | Same naming rule as workflow IDs |

**Final safety net**: after constructing the path, use `filepath.Rel` to verify that the resolved absolute path still sits under the intended base directory.

### 13.2 Namespace Reservation

The `argus-` prefix is reserved for built-in workflows, invariants, and skills. `install` and `inspect` must reject user-defined items using that prefix.

### 13.3 Template Variable Rules

Workflow job `prompt` fields support template variables.

#### Variable Categories and Phase 1 Field Set

| Category | Field | Meaning |
|------|------|------|
| `workflow` | `{{ .workflow.id }}` | Current workflow ID |
| `workflow` | `{{ .workflow.description }}` | Current workflow description |
| `job` | `{{ .job.id }}` | Current job ID |
| `job` | `{{ .job.index }}` | Current job index (0-based) |
| `pre_job` | `{{ .pre_job.id }}` | Previous job ID, or empty string for the first job |
| `pre_job` | `{{ .pre_job.message }}` | Previous job message, or empty string for the first job |
| `git` | `{{ .git.branch }}` | Current Git branch |
| `project` | `{{ .project.root }}` | Absolute project root |
| `env` | `{{ .env.XXX }}` | Environment-variable passthrough |
| `jobs` | `{{ .jobs.run_tests.message }}` | Message from a completed job by job ID |

`pre_job` exists so prompts can reference the immediate predecessor without knowing its explicit job ID.

**Job ID naming constraint**: job IDs referenced through `.jobs.<id>` must start with a lowercase letter and use only lowercase letters, digits, and underscores: `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`. Hyphens are forbidden because Go `text/template` dot syntax would interpret them as subtraction.

#### Missing Variables

Argus uses partial replacement. Known variables are rendered; unknown variables remain as their original `{{ .xxx }}` placeholders and also trigger a warning on stderr. This avoids blocking execution when a workflow references a variable introduced by a later version. Under the `jobs` category, non-existent jobs or empty messages are treated as missing values and left as placeholders.

## 14. Phase 1 Deferred Features

| Feature | Description | Why deferred |
| :--- | :--- | :--- |
| Pipeline resume | Continue from a failed job | Restart is sufficient in most cases and keeps the model simpler |
| Asynchronous invariants | Long-running invariant checks in the background | Requires more complex scheduling and frequency control |
| Trap rule engine | Workflow-defined tool gating rules | Hard enforcement is not yet required by current use cases |
| Automatic pipeline cleanup | Garbage-collect old pipeline files | Early phases prioritize traceability over cleanup |
| DAG parallel execution | Parallel jobs with dependency graphs | Phase 1 is intentionally sequential |
| Historical-reporting use cases | Richer uses of pipeline history | Wait until concrete scenarios are established |

### 14.1 Removed Early Ideas

These ideas were replaced or dropped during design:

- `.argus/meta/rules-meta.json`: replaced by freshness checks based on marker-file timestamps
- `.argus/deps/dependencies.json`: cross-project dependency tracking was deferred; agents can coordinate across projects without a built-in Argus dependency graph
- `.argus/prompts/`: a separate prompt override layer was removed because users can already edit workflow `prompt` fields directly
- `.argus/config.json`: a project-level config file was rejected because current requirements are already covered by flags, environment variables, or user-level workspace config
