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
.claude/skills/argus-*/SKILL.md  # Claude Code project-level skill mirror (also scanned by OpenCode)
assets/                          # Embedded assets in source code (Go embed)
                                 # Physical path: internal/assets/ (go:embed only refs package subdir)
  skills/                        # Built-in skills (released on setup)
  workflows/                     # Built-in workflow definitions (released on setup)
  invariants/                    # Built-in invariant definitions (released on setup)
  prompts/                       # Runtime output templates (not released, internal use only)
  hooks/                         # Hook wrapper templates (not released, internal use only)
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

`inspect`, `invariant check`, `doctor` only report problems and suggest solutions. They never auto-fix or auto-start workflows. `doctor` may run explicit opt-in diagnostic checks, including invariant shell checks, but still remains read-only. The Agent guides the user to decide.

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

### 9. Scopes change configuration, not orchestration semantics

Different Argus scopes (for example, project scope and user/workspace scope) are different configuration roots of the same orchestration model, not separate product modes.

- Prefer one shared orchestration mechanism across scopes. If behavior differs by scope, first ask whether the difference can be expressed by loading different invariants, workflows, prompts, or state roots.
- Scope-specific policy belongs in artifacts, not in hardcoded hook branches. Setup guidance, skip/remind decisions, and remediation flows should normally be modeled as invariants plus workflows.
- Hardcoded scope logic is acceptable only for bootstrap concerns that artifacts cannot decide by themselves, such as input parsing, sub-agent suppression, project/scope discovery, and fail-open handling when no scope applies.
- When a shortcut would create a second mental model for `tick`, `job-done`, or other orchestration entry points, treat that shortcut as design debt even if it is simpler to ship.
- Storage paths remain documented facts, but runtime collaboration should prefer artifact capabilities over raw directory contracts. Calling code should ask to load/save workflows, invariants, pipelines, or hook logs rather than passing `.argus/...` subdirectories across package boundaries.

## Naming Conventions

### Namespace reservation

`argus-` prefix is reserved for built-in workflows, invariants, and skills. Users cannot use this prefix for their own definitions. Enforced at inspect/setup time.

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
| Argus Command | CLI subcommand (`argus setup`) | Slash Command (`/argus-doctor`) |

## Design Conventions

### Version field

All independent schema files (workflow YAML, invariant YAML, pipeline data) include `version: v0.1.0`. Argus checks major version compatibility at parse time. Dependent files (`_shared.yaml`) do not have their own version.

### Pipeline data model

- Single active pipeline (phase 1), sequential job execution
- `current_job` global field instead of per-job status (status is derivable from position relative to current_job + pipeline status)
- `current_job: null` when pipeline is completed
- Per-job data only records runtime outputs (`message`, `started_at`, `ended_at`), not status

### tick injection strategy

- Primary output carries the current orchestration context: full job context, minimal reminder, no-pipeline workflow guidance, snoozed guidance, invariant remediation guidance, or active-pipeline anomaly guidance depending on the current route
- Secondary warning lane may append short non-blocking warnings after the primary output, for example invalid invariant summaries or slow automatic-check notices
- State changed (new job, new pipeline): primary output injects full context (prompt, skill, detailed guidance)
- State unchanged: primary output injects minimal summary (current job id + status + job-done reminder)
- Minimal summary is necessary: prevents Agent from forgetting to call job-done after multiple conversation turns
- Treat changes to `tick`'s user-visible routing or output semantics as orchestration behavior changes, not copy-only edits. Keep `docs/technical-tick.md` in sync when those contracts change.

### job-done dual progression

- tick path: user input triggers tick, for recovery/resume scenarios
- job-done return path: job-done returns next job info, Agent auto-advances without waiting for user input

### Human-in-the-loop

No `confirm` / `auto` fields. Write confirmation requirements directly in job prompt. Agent judges based on context.

## Working Conventions

- Record tradeoff analysis for important design decisions to guide future design and prevent drift.
- To restore context quickly, read `AGENTS.md` first, then `docs/technical-overview.md`; read other `docs/technical-*.md` on demand.
- When a function's correct usage depends on a specific call sequence (timing contract), document the sequence in the function's godoc comment. Callers should be able to understand usage constraints without reading architecture documents.
- When behavior depends on time, environment variables, current directory, git state, or external command execution, prefer package-private runtime seams while keeping the public API stable. Tests should inject those seams instead of depending on wall-clock time or the caller's ambient environment.
- When updating agent hook or plugin wrappers, verify behavior against the host agent's current runtime contract using the installed SDK or type definitions, runtime logs, and the actual installed hook artifact. Official docs and older templates may lag behind the host's real integration API.
- If a reviewer (including automated review) misreads code and files a false positive, treat it as a signal that the code's intent is not clear enough. Add comments, rename symbols, or restructure to eliminate the ambiguity — even though the behavior is already correct.
- When a local simplification conflicts with architectural consistency, prefer the design that preserves one reusable mechanism and artifact-driven policy. In this early stage, do not preserve bespoke behavior only because it already exists.
- Argus is still in an early development stage. Do not preserve APIs, file formats, or behaviors solely for backward compatibility or to avoid breaking changes. Prefer the most correct and maintainable design, even when that means changing existing behavior and discarding historical baggage.

## Development Guidelines

### Standard Workflow

For every implementation task:
1. Read `AGENTS.md`
2. Read `docs/technical-overview.md`
3. Read the relevant `docs/technical-*.md` chapters on demand
4. Review the current implementation and tests before changing behavior
5. Write tests first (TDD): define expected behavior before implementation
6. Implement the minimum code to make tests pass
7. Refactor for clarity and maintainability
8. Run `make build && make test && make lint` before committing
9. Commit with Conventional Commits format

### Go Best Practices

**Error Handling**:
- Use sentinel errors for known error conditions: `var ErrNotFound = errors.New("not found")`
- Define custom error types for structured error data
- Always wrap errors with context: `fmt.Errorf("reading config: %w", err)`
- Never ignore error return values
- When loading state files that may not exist yet (session, pipeline data), distinguish "file not found" (create fresh) from "file exists but malformed" (return error). Never silently swallow all load errors with a fallback.

**Logging**:
- Use `log/slog` for all structured logging
- Do not use `fmt.Print*` for output; use `os.Stdout.WriteString` or `slog` instead
- Log at appropriate levels: Debug for development, Info for operations, Error for failures

**Testing**:
- Use table-driven tests for coverage of multiple cases
- Test file naming: `foo_test.go` alongside `foo.go`
- Use `testing` standard library for M0; `testify/assert` and `testify/require` from M1+
- Test both happy path and error paths
- For CLI commands, prefer `cmd.InOrStdin()` / `cmd.OutOrStdout()` over direct `os.Stdin` / `os.Stdout` access. If interactivity depends on TTY detection, detect TTY from the injected input stream rather than from process-global stdin.
  Exception: hidden passthrough utility commands under `argus toolbox` may intentionally forward process-global stdin/stdout/stderr to preserve their busybox-style contract.
- For structured persisted state such as YAML or JSON, tests should assert by parsing the artifact into typed data rather than by substring matching on serialized text. String matching is acceptable only as a secondary check for human-facing output.
- Integration test helpers must make output mode explicit. Do not hide command contracts behind helpers that infer flags such as `--json` from the command shape.
- The default `go test ./...` baseline must not depend on optional third-party binaries being installed. Extra validation that shells out to external tools should be explicitly gated and must not be required for the default test suite.

**Interface Design**:
- Prefer small, single-method interfaces (Go interface segregation)
- Define interfaces at the point of use, not at the point of implementation
- Accept interfaces, return concrete types

**Go 1.26 Features**:
- Use `slices` and `maps` standard library packages where applicable
- Prefer `errors.Is` and `errors.As` for error inspection

**Context Propagation**:
- In cobra `RunE` functions, use `cmd.Context()` when calling context-aware APIs, not `context.Background()`. This maintains proper cancellation propagation even in CLI commands.

### Lint Policy

The authoritative enforced lint set lives in `.golangci.yml` and CI. Treat lint as a way to encode repository-wide engineering constraints, not as a collection of personal style preferences.

- Prefer linters that catch correctness bugs, contract violations, architectural boundary breaks, insecure defaults, deprecated APIs, or maintainability hazards that recur across the codebase.
- Prefer rules that express **"do not do this"** over brittle allowlists that merely snapshot today's import graph or coding shape.
- A linter is worth adding when it encodes a durable project invariant, produces mostly actionable findings, and does not require routine suppression churn to stay usable.
- Do not add or keep a linter solely because it is available. If a rule mostly creates local style noise, forces arbitrary rewrites, or repeatedly fights intentional Argus design, it is the wrong rule for this repository.
- When a repository convention and a linter disagree, first ask whether the convention should be tightened, the code should be clarified, or the linter should be reconfigured. Do not cargo-cult the linter output.

### `nolint` Usage

`nolint` is an escape hatch for narrow, justified exceptions. It is not a normal way to resolve lint findings.

- Prefer changing the code, tests, naming, comments, or helper structure so the lint issue disappears without suppression.
- Suppress only when the code is intentionally correct but the linter cannot reasonably model that fact.
- Typical acceptable cases:
  - product contracts that intentionally look risky to a generic linter, such as executing user-authored invariant shell checks
  - host-tool integration contracts that require patterns we normally avoid
  - passthrough utility commands under `argus toolbox`, where preserving the external utility contract is more important than normal CLI abstractions
  - narrowly-scoped false positives where a rewrite would make the code less clear
  - temporary migration exceptions, but only when the follow-up work is explicit and bounded
- Requirements for every suppression:
  - use the smallest possible scope, ideally one line
  - name the exact linter: `//nolint:<linter>`
  - explain why the code is safe or why the exception is intentional
  - keep the surrounding code readable enough that future reviewers can re-evaluate the suppression
- Unacceptable uses:
  - bare `//nolint`
  - file-wide or directory-wide suppression for convenience
  - suppressing a warning without understanding it
  - using suppression to preserve unclear code when a small refactor would remove the ambiguity

### Anti-Patterns (Prohibited)

The authoritative enforced lint set lives in `.golangci.yml` and CI. The rules below document the repository-level intent behind that stricter baseline and should stay aligned with the actual configuration without duplicating every linter toggle.

- **No `init()` functions**: Use explicit initialization in `main` or constructors (`gochecknoinits`)
- **No global mutable state**: Pass dependencies explicitly via function parameters or struct fields; package-level state should be immutable constants or carefully scoped runtime seams
- **No `panic` in business logic**: Use error returns; `panic` is reserved for unrecoverable programmer errors
- **No `any` abuse**: Use typed parameters and return values; avoid `interface{}` where a concrete type works
- **No ignored errors**: All error return values must be checked (`errcheck`)
- **No context-less external command execution in production paths**: Thread `context.Context` through call chains and use context-aware APIs such as `exec.CommandContext`
- **No insecure defaults for Argus-managed private state**: Files under Argus-managed config, state, log, and asset roots should default to owner-only permissions (`0o600` files, `0o700` directories) unless broader access is a deliberate documented requirement
- **No broad lint suppressions**: Prefer fixing code over suppression. When suppression is unavoidable, use line-scoped `//nolint:<linter> // reason`, name the exact linter, and explain why the code is safe. Do not use bare `//nolint` or directory-wide exclusions for convenience
- **No `fmt.Print*` in production code**: Use `os.Stdout.WriteString` or `log/slog` (`forbidigo`)
- **No raw `json.Marshal` for CLI output**: Use `core.OKEnvelope`/`core.ErrorEnvelope`/`core.WriteJSON` (they disable HTML escaping via `SetEscapeHTML(false)`; raw `json.Marshal` escapes `<>&` as `\uXXXX`)
- **No string manipulation for structured data**: Use proper parsing libraries for JSON, YAML, TOML, XML, etc.
  Never use regex, string split/replace, or line-level operations to read or modify structured formats.
  Already available: `encoding/json`, `gopkg.in/yaml.v3`, `pelletier/go-toml/v2`.

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
- `internal/lifecycle/`: Setup, teardown, and asset release logic
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

#### Sequence Tests (Cross-Command State Flow)

Sequence tests verify that state produced by one CLI command is correctly consumed by subsequent commands. They complement unit tests by validating cross-command contracts that individual tests cannot catch.

**When to write a sequence test** — apply ALL three criteria:
1. **Implicit state coupling**: Command B reads state written by Command A via the file system (e.g., pipeline YAML), with no compile-time contract
2. **Directional operation path**: The commands form an ordered sequence with a defined entry point (e.g., `start → job-done → status`)
3. **Cumulative effect / terminal state**: The sequence accumulates state changes and reaches a final condition that must be verified (e.g., pipeline status transitions from `running` to `completed`)

**Conventions**:
- File naming: `cmd_<theme>_test.go` (e.g., `cmd_pipeline_lifecycle_test.go`)
- Pipeline state must be produced by actual commands (`workflow start`), not by `writePipelineFixture`
- `t.Parallel()` is prohibited (os.Stdout capture is incompatible)
- Source files whose output participates in sequence tests must carry a `// SEQUENCE-TEST:` comment above the relevant command constructor, referencing the test file

#### Test Patterns and Conventions

The following patterns emerged from the test coverage backfill effort and should be followed for new tests:

- **Two output capture patterns**: Use `executeXxxCmd` with `os.Pipe` for JSON output (which redirects `os.Stdout` globally), and `cmd.SetOut(buf)` with `bytes.Buffer` for markdown or plain text output. Do not mix these: `os.Pipe` captures all writes to `os.Stdout`, while `SetOut` only captures output sent through cobra's `cmd.OutOrStdout()`.
- **Fixture helper reuse**: Fixture helpers (e.g., `writeWorkflowFixture`, `writePipelineFixture`, `writeInvariantFixture`) defined in any `_test.go` file within a package are accessible to all other test files in that same package. Do not redeclare these helpers.
- **Mechanism tests vs built-in asset tests**: General command, parser, engine, and hook tests should use small purpose-built fixtures and assert the reusable mechanism under test. Do not couple those tests to the internal job order, step names, prompt wording, or check count of built-in workflows or invariants such as `argus-project-init`. For built-in assets, keep assertions at the smoke-test level unless a behavior is itself a documented public contract: existence, parseability, reserved ID registration, release wiring, and explicit cross-asset references are appropriate; internal phase structure is not.
- **Untestable patterns to avoid**: Functions reading from `os.Stdin` directly are difficult to test; use an `io.Reader` parameter or `cmd.InOrStdin()` instead. Similarly, cobra commands using `Run` with `os.Exit` cannot be tested for error paths; use `RunE` for all new commands to allow returning errors to the test runner.
  Exception: the hidden `argus toolbox` passthrough subcommands may keep `Run` + direct exit-code forwarding because their contract is to proxy utility behavior rather than follow the normal Argus command envelope.
- **Test directory context**: Tests in `internal/hook/` typically use an explicit `projectRoot := t.TempDir()` and pass it to fixtures and functions. CLI tests in `cmd/argus/` prefer `t.Chdir(t.TempDir())` to establish an implicit directory context for the duration of the test.
- **t.Parallel() prohibition**: In addition to sequence tests, any test using the `os.Pipe` pattern to capture `os.Stdout` MUST NOT call `t.Parallel()`, as `os.Stdout` is a process-global resource.
- **Session test hygiene**: In `cmd/argus`, same-process session tests should use `sessiontest.NewSessionID(...)` with `sessiontest.NewMemoryStore()`. In `tests/integration`, tests that hit the default session files should use the shared session ID and cleanup helpers. Keep explicit session literals only when the literal itself is the assertion target.

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
- `lifecycle`: Setup, teardown, and asset release
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
