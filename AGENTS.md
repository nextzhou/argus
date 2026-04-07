# AGENTS.md — Argus

## Project Overview

Argus is an AI Agent workflow orchestration tool. It provides a CLI binary (Go) that integrates with multiple AI Agents (Claude Code, Codex, OpenCode) via their hook systems, orchestrating workflows, invariants, and project state.

## Key Module Navigation

```
.argus/                          # Project-level Argus directory
  workflows/                     # Workflow YAML definitions (git-tracked)
    _shared.yaml                 # Shared job definitions
  invariants/                    # Invariant YAML definitions (git-tracked)
  rules/                         # Generated project rules (git-tracked)
  pipelines/                     # Pipeline instance data files (local-only)
  logs/                          # Hook execution logs (local-only)
  data/                          # General-purpose data files, e.g. freshness timestamps (git-tracked)
  tmp/                           # Temporary data (local-only)
.agents/skills/argus-*/SKILL.md  # Skills distributed via Agent Skills standard (git-tracked)
assets/                          # Embedded assets in source code (Go embed)
  skills/                        # Built-in skills (released on install)
  workflows/                     # Built-in workflow definitions (released on install)
  invariants/                    # Built-in invariant definitions (released on install)
  prompts/                       # Runtime output templates (not released, internal use only)
```

## Architecture Invariants

### 1. Argus is orchestration layer, not Agent replacement

Argus does not execute shell commands or make decisions. It only tells the Agent what to do and tracks progress. Anything the Agent can do, let the Agent do it.

- All jobs are Agent-driven: inject context -> Agent executes -> job-done
- No `script` field in workflow jobs; workflow execution is fully Agent-driven by design
- tick only does state check + context injection, returns fast

### 2. Artifacts as ground truth

Prefer checking actual file/artifact existence over boolean flags or recorded state.

- No `initialized: true` marker. Use Invariant system to check actual artifacts instead.
- Pipeline state lives in dedicated per-instance data files, not a centralized `state.yaml`
- State classification: every piece of persistent data is either git-tracked (team-shared) or local-only (machine-specific), never ambiguous

### 3. Workflow (imperative) + Invariant (declarative) are complementary

- Workflow answers "how" (process steps). Invariant answers "what should be true" (conditions).
- Same goal, different dimensions: Workflow ensures correct process, Invariant catches drift regardless of cause.

### 4. Diagnostic tools only diagnose, never treat

`inspect`, `invariant check`, `doctor` only report problems and suggest solutions. They never auto-fix or auto-start workflows. The Agent guides the user to decide.

### 5. Invariant check is shell-only

No prompt check (LLM-based evaluation) in invariant definitions. Deep verification goes into the remediation workflow's first job. Rationale: shell check's core value is silent pass without interrupting the user.

### 6. Semantic checks become freshness checks

Complex semantic checks (e.g., "is documentation up-to-date") are converted to timestamp-based checks (e.g., "has documentation been reviewed in the last N days"). This keeps invariant checks fast, deterministic, and shell-only.

### 7. Path construction input validation

All external inputs used to construct file paths must be validated in Go before use, to prevent path traversal attacks.

| Input | Used in path | Validation |
|-------|-------------|------------|
| session_id | `/tmp/argus/<safe-id>.yaml` | UUID format preferred: `^[0-9a-fA-F-]+$`; non-conforming values are SHA256-hashed (first 16 chars) to produce a safe filename (`safe-id`) |
| workflow_id | `.argus/pipelines/<workflow-id>-<timestamp>.yaml` | Naming convention: `^[a-z0-9]+(-[a-z0-9]+)*$` |
| invariant id | `.argus/invariants/<id>.yaml` | Same as workflow_id |

Fallback: after constructing the path, verify via `filepath.Rel` that the resolved path is still under the expected directory.

## Naming Conventions

### Namespace reservation

`argus-` prefix is reserved for built-in workflows, invariants, and skills. Users cannot use this prefix for their own definitions. Enforced at inspect/install time.

### Skill naming

- Lowercase letters, digits, hyphens only. Max 64 characters.
- Regex: `^[a-z0-9]+(-[a-z0-9]+)*$`
- No `:` character (colon namespacing is Claude Code Plugin system, not skill names)
- Directory name must match `name` field in SKILL.md

### Key terminology

| Term | Meaning | NOT to be confused with |
|------|---------|------------------------|
| Pipeline | An instance of a workflow execution in Argus | GitLab Pipeline (different concept) |
| Rule | Constraint/specification for coding | Skill (capability/procedure) |
| Argus Command | CLI subcommand (`argus install`) | Slash Command (`/argus-doctor`) |

## Design Conventions

### Version field

All independent schema files (workflow YAML, invariant YAML, pipeline data) include `version: v0.1.0`. Argus checks major version compatibility at parse time. Dependent files (`_shared.yaml`) do not have their own version.

### Pipeline data model

- Single active pipeline (phase 1), sequential job execution
- `current_job` global field instead of per-job status (status is derivable from position relative to current_job + pipeline status)
- `current_job: null` when pipeline is completed
- Per-job data only records runtime outputs (`message`, `started_at`, `ended_at`), not status

### tick injection strategy

- State changed (new job, new pipeline): inject full context (prompt, skill, detailed guidance)
- State unchanged: inject minimal summary (current job id + status + job-done reminder)
- Minimal summary is necessary: prevents Agent from forgetting to call job-done after multiple conversation turns

### job-done dual progression

- tick path: user input triggers tick, for recovery/resume scenarios
- job-done return path: job-done returns next job info, Agent auto-advances without waiting for user input

### Human-in-the-loop

No `confirm` / `auto` fields. Write confirmation requirements directly in job prompt. Agent judges based on context.

## Working Conventions

- Record tradeoff analysis for important design decisions to guide future design and prevent drift.
- To restore context quickly, read `AGENTS.md` first, then `docs/technical-overview.md`; read other `docs/technical-*.md` on demand.
