# Technical Overview

This document is the entry point for Argus technical documentation. It explains the role of Argus, the architectural invariants that shape the implementation, and the repository layout used by the runtime.

---

## 1. Positioning

Argus is an AI agent workflow orchestrator. It is a Go CLI that integrates with agent hook systems such as Claude Code, Codex, and OpenCode. Argus does not replace the agent. It coordinates state, injects context, and keeps project-level guardrails visible over time.

### 1.1 Architecture Invariants

Argus is built around the following invariants:

1. **Argus is an orchestration layer, not an agent replacement.**
   Argus tracks progress and injects context. The agent performs the actual work.
2. **Artifacts are the ground truth.**
   Prefer checking real files and real outputs over boolean markers such as `initialized: true`.
3. **Workflow and Invariant are complementary.**
   Workflow answers "how to proceed". Invariant answers "what must be true".
4. **Diagnostic tools diagnose only.**
   `doctor`, `inspect`, and invariant checks report problems. They do not auto-fix or auto-start workflows.
5. **Invariant checks are shell-only.**
   They must be deterministic and cheap enough to run silently during `tick`.
6. **Semantic checks become freshness checks when needed.**
   If a rule cannot be checked deterministically, represent it as a timestamp-backed review requirement.
7. **All path-construction inputs are validated.**
   Inputs such as `session_id`, `workflow_id`, and `invariant_id` must be validated before they affect filesystem paths.
8. **The repository is public by default.**
   Git-tracked content and embedded assets must be safe to publish without a cleanup pass.
9. **Scopes change configuration roots, not orchestration semantics.**
   Project scope and global/workspace scope share the same orchestration model.

### 1.2 Why Rules Are Not a Core Argus Primitive

Rules are generated and consumed through agent-native systems such as `AGENTS.md` or `CLAUDE.md`. Argus orchestrates the generation of those files through workflows and skills, but does not introduce a separate runtime rule engine of its own.

### 1.3 Terminology

| Term | Meaning | Not to be confused with |
|------|---------|-------------------------|
| Pipeline | A running instance of a workflow | GitLab pipeline |
| Workflow | The imperative blueprint of jobs | A pipeline instance |
| Invariant | A condition that should always hold | A one-off check command |
| Rule | A coding or architecture constraint | A skill |
| Argus command | A CLI subcommand such as `argus setup` | A slash command exposed by an agent |

`Invariant` is the chosen term because it expresses "a condition that should remain true" more precisely than alternatives like `check`, `guard`, or `policy`.

---

## 2. Repository Layout and State Model

### 2.1 Project-Level Layout

```text
.argus/
  workflows/         # Workflow YAML definitions (git-tracked)
    _shared.yaml     # Shared job definitions
  invariants/        # Invariant YAML definitions (git-tracked)
  rules/             # Generated project rules (git-tracked)
  pipelines/         # Pipeline instance data (local-only)
  logs/              # Hook execution logs (local-only)
  data/              # Shared data such as freshness timestamps (git-tracked)
  tmp/               # Other temporary data (local-only)
.agents/skills/argus-*/SKILL.md
.claude/skills/argus-*/SKILL.md
```

### 2.2 State Inventory

Argus keeps a strict boundary between git-tracked team state and local runtime state.

#### Git-tracked

| Artifact | Path | Managed by |
|----------|------|------------|
| Workflow definitions | `.argus/workflows/*.yaml` | `setup` plus user or agent edits |
| Shared job definitions | `.argus/workflows/_shared.yaml` | User or agent |
| Invariant definitions | `.argus/invariants/*.yaml` | `setup` plus user edits |
| Generated rules | `.argus/rules/` | Agent via workflow |
| Skills | `.agents/skills/argus-*`, `.claude/skills/argus-*` | `setup` |
| Agent-native rules | `AGENTS.md`, `CLAUDE.md`, etc. | Agent |
| Hook configuration | `.claude/settings.json`, `.codex/hooks.json`, `.opencode/plugins/argus.ts` | `setup` |
| Git hook config | `.husky/pre-commit`, `.lefthook.yml`, `.pre-commit-config.yaml`, or fallback `.git/hooks/*` | Agent or user. `.git/hooks/*` itself is local-only and cannot be committed; team projects should prefer a git-tracked framework such as husky |
| Shared data files | `.argus/data/` | Workflow and invariant logic |

#### Local-only

| Artifact | Path | Purpose |
|----------|------|---------|
| Pipeline state | `.argus/pipelines/*.yaml` | Current and historical pipeline progress |
| Hook logs | `.argus/logs/hook.log` | Integration diagnostics |
| Session state | `/tmp/argus/<safe-id>.yaml` | Snooze state and tick snapshot state |

### 2.3 Embedded Assets

Argus embeds several classes of assets under `internal/assets/`:

```text
internal/assets/
  skills/                        # Released by setup
    argus-doctor/SKILL.md
    argus-intro/SKILL.md
    argus-setup/SKILL.md
    ...
  workflows/                     # Released by setup
    argus-project-init.yaml
  invariants/                    # Released by setup
    argus-project-init.yaml
    argus-project-setup.yaml
  prompts/                       # Runtime-only templates
    tick-full-context.md.tmpl
    tick-minimal.md.tmpl
    ...
  hooks/                         # Runtime-only hook wrapper templates
```

```go
//go:embed skills workflows invariants prompts hooks
var embedded embed.FS
```

Usage rules:

- `skills/`, `workflows/`, and `invariants/` are released into project or global scope during setup. At project scope, skills are released to `.agents/skills/` and `.claude/skills/`. OpenCode discovers skills through compatible path scanning, so no separate `.opencode/skills/` is generated. At global scope (`setup --workspace`), Argus releases the current managed global built-in skill set to each agent's global skill directory and refreshes those managed resources on repeat setup (see [§11.5](technical-workspace.md)).
- `prompts/` and `hooks/` are runtime assets read internally by the Argus binary for template rendering (tick injection, job-done output, hook wrapper generation). They are not released as project files.

---

## 3. Scopes and Setup

### 3.1 Project Scope

- Root: `.argus/` inside a repository
- Stores project-specific workflows, invariants, rules, and pipeline data
- Takes precedence whenever the current working tree has project-level Argus set up

### 3.2 Global / Workspace Scope

- Root: `~/.config/argus/`
- Stores global artifacts used when a directory is inside a registered workspace but does not yet have project-level Argus set up
- Supports setup guidance through global invariants such as `argus-project-setup`

### 3.3 Unified Orchestration Model

Commands such as `tick`, `job-done`, and `status` keep the same semantics across scopes. Scope resolution changes where Argus loads artifacts from and where it writes runtime state, but it does not introduce a different orchestration model.

### 3.4 Setup Layers

1. Install the Argus binary globally.
2. Run `argus setup` inside a repository for project-level artifacts.
3. Run `argus setup --workspace <path>` to register a workspace and enable global scope behavior.

---

## 4. Naming and Versioning

### 4.1 Reserved Namespace

The `argus-` prefix is reserved for built-in workflows, invariants, and skills.

### 4.2 Identifier Rules

- Workflow IDs and invariant IDs: `^[a-z0-9]+(-[a-z0-9]+)*$`
- Skill names: `^[a-z0-9]+(-[a-z0-9]+)*$`
- Job IDs used in template lookups: `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`

### 4.3 Version Field

Independent schema files use `version: v0.1.0`. Argus validates major-version compatibility when loading them.

### 4.4 Timestamp Format

Persisted timestamps use compact UTC format:

```text
YYYYMMDDTHHMMSSZ
```

This format is filename-safe, sortable, and used consistently across pipelines, session files, tool output, and hook logs.

---

## 5. Path Safety

External inputs that affect path construction must be validated before use:

| Input | Target path pattern | Validation |
|------|----------------------|------------|
| `session_id` | `/tmp/argus/<safe-id>.yaml` | Prefer UUID format (`^[0-9a-fA-F-]+$`); non-conforming values are SHA256-hashed and the first 16 hex characters are used as `safe-id` |
| `workflow_id` | `.argus/pipelines/<workflow-id>-<timestamp>.yaml` | Validate against workflow ID naming rules |
| `invariant_id` | `.argus/invariants/<id>.yaml` | Same as workflow ID |

After constructing the final path, Argus should still verify with `filepath.Rel` that the resolved path remains under the intended root.

---

## 6. Navigation

Use the following documents for subsystem-specific details:

- [technical-cli.md](technical-cli.md): CLI command model, output contracts, and flags
- [technical-hooks.md](technical-hooks.md): hook integration for Claude Code, Codex, and OpenCode
- [technical-workflow.md](technical-workflow.md): workflow YAML schema and execution model
- [technical-invariant.md](technical-invariant.md): invariant schema and check semantics
- [technical-pipeline.md](technical-pipeline.md): pipeline state model and session tracking
- [technical-workspace.md](technical-workspace.md): workspace registration and global scope behavior
- [technical-operations.md](technical-operations.md): diagnostics, security, and deferred work
