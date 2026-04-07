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

### 8. Public-by-default repository

Argus is designed so the repository can be made public at any time without a cleanup pass.

- All git-tracked files and embedded assets must be safe to publish and legally redistributable.
- The core project must not depend on private infrastructure, unpublished APIs, internal documents, or private credentials to build, test, install, or explain its main workflows.
- Organization-specific or proprietary integrations must be added as optional adapters, ideally in separate repositories, and must not introduce a reverse dependency from the core project to private code or private services.
- The core repository is responsible for stable extension points and public contracts; adapters may extend behavior, but the core project must remain buildable, testable, and understandable without them.
- Default logs, examples, fixtures, and templates must not assume internal context or include sensitive data.
- Any outbound data transfer, telemetry, or auto-update behavior must be explicit, documented, and not required for core local use.

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

## Development Guidelines

### Standard Workflow

For every implementation task:
1. Read the task specification in `docs/implementation-tasks.md`
2. Read the relevant technical chapter in `docs/technical-*.md`
3. Write tests first (TDD): define expected behavior before implementation
4. Implement the minimum code to make tests pass
5. Refactor for clarity and maintainability
6. Run `make build && make test && make lint` before committing
7. Commit with Conventional Commits format

### Go Best Practices

**Error Handling**:
- Use sentinel errors for known error conditions: `var ErrNotFound = errors.New("not found")`
- Define custom error types for structured error data
- Always wrap errors with context: `fmt.Errorf("reading config: %w", err)`
- Never ignore error return values

**Logging**:
- Use `log/slog` for all structured logging
- Do not use `fmt.Print*` for output; use `os.Stdout.WriteString` or `slog` instead
- Log at appropriate levels: Debug for development, Info for operations, Error for failures

**Testing**:
- Use table-driven tests for coverage of multiple cases
- Test file naming: `foo_test.go` alongside `foo.go`
- Use `testing` standard library for M0; `testify/assert` and `testify/require` from M1+
- Test both happy path and error paths

**Interface Design**:
- Prefer small, single-method interfaces (Go interface segregation)
- Define interfaces at the point of use, not at the point of implementation
- Accept interfaces, return concrete types

**Go 1.24 Features**:
- Use `slices` and `maps` standard library packages where applicable
- Prefer `errors.Is` and `errors.As` for error inspection

### Anti-Patterns (Prohibited)

The following patterns are prohibited and enforced by golangci-lint:

- **No `init()` functions**: Use explicit initialization in `main` or constructors (`gochecknoinits`)
- **No global mutable state**: Pass dependencies explicitly via function parameters or struct fields
- **No `panic` in business logic**: Use error returns; `panic` is reserved for unrecoverable programmer errors
- **No `any` abuse**: Use typed parameters and return values; avoid `interface{}` where a concrete type works
- **No ignored errors**: All error return values must be checked (`errcheck`)
- **No `fmt.Print*` in production code**: Use `os.Stdout.WriteString` or `log/slog` (`forbidigo`)

### Package Organization and Dependency Direction

The dependency graph flows strictly one way:

```
cmd -> internal/*
```

- `cmd/argus/`: Entry point only. Wires up dependencies, calls internal packages.
- `internal/core/`: Core domain types shared across packages (no external imports)
- `internal/workflow/`: Workflow parsing and execution logic
- `internal/invariant/`: Invariant definition parsing and check logic
- `internal/pipeline/`: Pipeline state management
- `internal/session/`: Session lifecycle management
- `internal/hook/`: Hook command handlers (tick, trap, job-done)
- `internal/workspace/`: Workspace discovery and management
- `internal/install/`: Install and asset release logic
- `internal/doctor/`: Diagnostic reporting (read-only)
- `internal/toolbox/`: Shared utilities (no dependencies on other internal packages)

Circular dependencies between `internal/*` packages are prohibited.

### Test Standards

- Follow TDD order: test -> implement -> refactor -> commit
- Use table-driven tests:
  ```go
  tests := []struct {
      name    string
      input   string
      want    string
      wantErr bool
  }{
      {"valid input", "foo", "bar", false},
      {"empty input", "", "", true},
  }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- Test file naming: `foo_test.go` in the same package as `foo.go`
- Each package must have at least one test file in M1+
- Integration tests go in `internal/<pkg>/<pkg>_integration_test.go`

### Commit Convention

Format: `type(scope): description`

**Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`

**Scope examples**:
- `cli`: CLI command changes
- `core`: Core domain types
- `workflow`: Workflow parsing/execution
- `invariant`: Invariant definition/checking
- `pipeline`: Pipeline state management
- `session`: Session lifecycle
- `hook`: Hook command handlers
- `workspace`: Workspace management
- `install`: Install and asset release
- `doctor`: Diagnostic tools
- `toolbox`: Shared utilities
- `project`: Project-level configuration
- `make`: Makefile changes
- `lint`: Linter configuration
- `hooks`: Git hook configuration
- `agents`: AGENTS.md or AI agent configuration

**Message rules**:
- Description: 1-72 characters, lowercase, no trailing period
- Scope is optional but recommended for non-trivial changes
- Breaking changes: append `!` after scope, e.g., `feat(core)!: redesign error types`
