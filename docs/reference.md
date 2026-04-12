# Argus Reference

This document covers everything you need to write custom workflows, invariants, and skills, plus a complete CLI command reference. For an introduction to Argus concepts and architecture, see the [README](../README.md).

## Table of Contents

- [Naming Conventions](#naming-conventions)
- [Writing Workflows](#writing-workflows)
- [Writing Invariants](#writing-invariants)
- [Writing Skills](#writing-skills)
- [CLI Reference](#cli-reference)
- [Built-in Content](#built-in-content)

## Naming Conventions

All user-defined identifiers follow the same base pattern: lowercase letters, digits, and hyphens.

| Identifier | Regex | Notes |
|-----------|-------|-------|
| Workflow ID | `^[a-z0-9]+(-[a-z0-9]+)*$` | `argus-` prefix reserved for built-in |
| Invariant ID | `^[a-z0-9]+(-[a-z0-9]+)*$` | `argus-` prefix reserved for built-in |
| Skill name | `^[a-z0-9]+(-[a-z0-9]+)*$` | Max 64 characters; `argus-` prefix reserved |
| Job ID | `^[a-z][a-z0-9]*(_[a-z0-9]+)*$` | Underscores, not hyphens (required for template variable access) |

## Writing Workflows

Workflow files live in `.argus/workflows/` as YAML files. Except for `_shared.yaml`, each file must be named `<workflow-id>.yaml`.

### Schema

```yaml
version: v0.1.0
id: my-workflow                         # lowercase, digits, hyphens
description: "What this workflow does"  # optional

jobs:
  - id: step_one
    prompt: "Instructions for the agent"
  - id: step_two
    skill: some-skill-name              # reference a SKILL.md
  - ref: lint                           # reference a shared job from _shared.yaml
```

### Fields

- **`version`**: must be `v0.1.0` (major version compatibility checked at parse time).
- **`id`**: unique workflow identifier. See [Naming Conventions](#naming-conventions).
- **`description`**: optional, human-readable.
- **`jobs`**: ordered list of steps (at least one required).

Each job needs at least one of:
- **`prompt`**: natural-language instructions for the agent.
- **`skill`**: name of a SKILL.md to load (see [Writing Skills](#writing-skills)).
- **`ref`**: reference to a shared job definition (see [Shared Jobs](#shared-jobs)).

`prompt` and `skill` can coexist on the same job. `prompt` and `skill` cannot both be empty (unless `ref` is used).

### Shared Jobs

Define reusable jobs in `.argus/workflows/_shared.yaml`:

```yaml
# .argus/workflows/_shared.yaml
jobs:
  lint:
    prompt: "Run linter and fix any errors"
  code_review:
    prompt: "Review changes for code quality"
```

Reference them with `ref`:

```yaml
jobs:
  - ref: lint                           # id defaults to "lint"
  - ref: code_review
    id: strict_review                   # rename the instance
    prompt: "Review with extra focus on security"  # override prompt
```

When using `ref`, you can override `id`, `prompt`, and `skill` from the shared definition.

### Template Variables

Job prompts support Go `text/template` syntax:

| Variable | Description |
|----------|-------------|
| `{{ .workflow.id }}` | Current workflow ID |
| `{{ .job.id }}` | Current job ID |
| `{{ .pre_job.message }}` | Previous job's output message |
| `{{ .jobs.<job_id>.message }}` | Output from a specific completed job |
| `{{ .git.branch }}` | Current Git branch |
| `{{ .project.root }}` | Project root directory |
| `{{ .env.XXX }}` | Environment variable |

Example using template variables:

```yaml
jobs:
  - id: run_tests
    prompt: "Run the test suite and report results"
  - id: summarize
    prompt: |
      Tests completed with result: {{ .pre_job.message }}
      Create a summary for branch {{ .git.branch }}.
```

### Validation

```bash
argus workflow inspect                    # validate .argus/workflows/
argus workflow inspect /path/to/dir       # validate a specific directory
```

`workflow inspect` also verifies the `<id>.yaml` file-name contract and allows reserved `argus-*` IDs only when they belong to built-in workflows embedded in the current Argus binary.

## Writing Invariants

Invariant files live in `.argus/invariants/` as YAML files. Each file must be named `<invariant-id>.yaml`.

### Schema

```yaml
version: v0.1.0
id: my-check                   # lowercase, digits, hyphens (no argus- prefix)
order: 10
description: "Human-readable description"
auto: session_start             # when to check: always | session_start | never

check:
  - shell: "test -f .env.example"
    description: "Example env file exists"
  - shell: "grep -q 'DATABASE_URL' .env.example"
    description: "DATABASE_URL is documented"

prompt: "Please create .env.example with required variables"
workflow: setup-env             # optional: suggest a remediation workflow
```

### Fields

- **`version`**: must be `v0.1.0`.
- **`id`**: unique invariant identifier. See [Naming Conventions](#naming-conventions).
- **`order`**: required positive integer. Lower numbers run first. Must be unique within the current invariant directory.
- **`description`**: optional, human-readable.
- **`auto`**: when to run checks automatically during `tick`:
  - `always` — every tick (use sparingly for fast checks).
  - `session_start` — once per session.
  - `never` — manual only, via `argus invariant check`.
  - Default: `never`.
- **`check`**: ordered list of shell checks (at least one required).
  - **`shell`**: Bash command. Exit code 0 = pass. Each step runs in its own process.
  - **`description`**: optional, shown in check reports.
  - Steps execute in order and short-circuit on first failure.
- **`prompt`**: text injected to the agent when checks fail.
- **`workflow`**: ID of a remediation workflow to suggest on failure.

`prompt` and `workflow` cannot both be empty — at least one must be provided. Both can coexist.

### Multi-line Shell

Each check step runs in its own process, but a single step's multi-line shell shares execution context:

```yaml
check:
  - shell: |
      cd .argus/rules
      test -f security.yaml
      test -f coding-style.yaml
    description: "Rule files are complete"
```

### Semantic Checks as Freshness Checks

For checks that require LLM-level understanding (e.g., "is the documentation up to date?"), convert them to timestamp-based freshness checks:

```yaml
check:
  - shell: "find .argus/data/docs-reviewed -mtime -7 | grep -q ."
    description: "Documentation reviewed within 7 days"

workflow: review-docs
```

This keeps invariant checks fast, deterministic, and shell-only. Use `argus toolbox touch-timestamp` to update freshness markers.

### Validation

```bash
argus invariant inspect                   # validate .argus/invariants/
argus invariant inspect /path/to/dir      # validate a specific directory
```

`invariant inspect` also verifies the `<id>.yaml` file-name contract, `order` uniqueness, and allows reserved `argus-*` IDs only when they belong to built-in invariants embedded in the current Argus binary.

Runtime note: `argus tick`, `argus status`, `argus invariant list`, and `argus invariant check` continue operating on valid invariants even when some files are invalid. Their JSON success payloads include an `invalid_invariants` array describing malformed or conflicting definitions.

## Writing Skills

A skill is a SKILL.md file that provides specialized instructions to agents. Workflow jobs reference skills by name via the `skill` field.

### File Location

Skills must be placed in two directories (for cross-agent compatibility):

```
.agents/skills/<skill-name>/SKILL.md    # discovered by Codex and OpenCode
.claude/skills/<skill-name>/SKILL.md    # discovered by Claude Code (also scanned by OpenCode)
```

The directory name must match the `name` field in the SKILL.md frontmatter.

### Format

A SKILL.md file uses YAML frontmatter followed by Markdown content:

```markdown
---
name: run-tests
description: Run project test suite and report results
version: 0.1.0
---

# run-tests

Run the project's test suite.

## When to Use

- Before committing changes
- As part of a release workflow

## Steps

1. Run `go test ./...`
2. Report pass/fail summary
3. If tests fail, suggest fixes
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Skill name, must match directory name |
| `description` | Yes | Short description of the skill |
| `version` | Yes | Semantic version (e.g., `0.1.0`) |

### Naming

Skill names follow the same convention as other identifiers: lowercase letters, digits, and hyphens (`^[a-z0-9]+(-[a-z0-9]+)*$`, max 64 characters). The `argus-` prefix is reserved for built-in skills.

## CLI Reference

### Lifecycle Commands

Run these directly in your terminal:

These commands default to human-readable text. Add `--json` when you need structured output for scripts or field-level parsing.

| Command | Description |
|---------|-------------|
| `argus setup [--yes] [--json]` | Set up project-level Argus in the current directory and refresh the managed global skills for the current user |
| `argus setup --workspace <path> [--yes] [--json]` | Register a workspace and set up or refresh global hooks, skills, and global artifacts |
| `argus teardown [--yes] [--json]` | Remove project-level Argus setup from the current directory; managed user-level global skills are left in place |
| `argus teardown --workspace <path> [--yes] [--json]` | Remove a registered workspace; if it is the last one, also remove managed global hooks, skills, global artifacts, and the `~/.config/argus/` root |
| `argus doctor [--check-invariants] [--json]` | Diagnose setup and configuration issues; `--check-invariants` opts into invariant shell checks for deeper diagnostics |
| `argus version [--json]` | Show version |

### Workflow and Pipeline Commands

| Command | Description |
|---------|-------------|
| `argus workflow start <id> [--json]` | Start a workflow (creates a new pipeline) |
| `argus workflow list [--json]` | List available workflows |
| `argus workflow cancel [--json]` | Cancel the active pipeline |
| `argus workflow snooze --session <id> [--json]` | Temporarily ignore the active pipeline in this session |
| `argus workflow inspect [dir] [--json]` | Validate workflow YAML definitions |
| `argus job-done [flags] [--json]` | Mark current job as done and advance the pipeline |
| `argus status [--json]` | Query project status (pipeline + invariants) |

`job-done` flags:

| Flag | Description |
|------|-------------|
| `--message "text"` | Summary of what was done in this job |
| `--fail` | Mark the job as failed |
| `--end-pipeline` | End the pipeline early (skip remaining jobs) |

### Invariant Commands

| Command | Description |
|---------|-------------|
| `argus invariant check [id] [--json]` | Run invariant checks (all, or a specific one) |
| `argus invariant list [--json]` | List defined invariants |
| `argus invariant inspect [dir] [--json]` | Validate invariant YAML definitions |

### Toolbox

Built-in portable tools (no external dependencies required):

| Command | Description |
|---------|-------------|
| `argus toolbox jq <expression> [file]` | JSON query |
| `argus toolbox yq <expression> [file]` | YAML query |
| `argus toolbox touch-timestamp <path>` | Create/update a freshness marker file |
| `argus toolbox sha256sum <file>` | Compute SHA-256 hash |

### Hook Commands

Called automatically by configured agent hooks, or reserved for internal integration:

| Command | Description |
|---------|-------------|
| `argus tick --agent <name>` | Context injection (on every user message) |
| `argus trap --agent <name>` | Reserved operation-gating entry point; not wired by default in Phase 1 |

## Built-in Content

Everything prefixed with `argus-` is built-in and reserved. Users cannot create custom workflows, invariants, or skills with the `argus-` prefix.

### Built-in Workflow

| ID | Description |
|----|-------------|
| `argus-project-init` | Project initialization: bootstrap the local Argus runtime, generate rules, set up git hooks, configure .gitignore, create workflows, and create example invariants |

### Built-in Invariant

| ID | Description |
|----|-------------|
| `argus-project-init` | Checks that the project has completed initialization (rules exist, skills are set up, git hooks are configured, .gitignore is set up, workflows are generated, example invariants are created) |
| `argus-project-setup` | Workspace-scope bootstrap reminder that checks whether project-level Argus has been set up and, if not, guides the agent to present setup / explain / ignore choices |

### Built-in Skills

Project scope (`argus setup`) releases these lifecycle skills into `.agents/skills/` and `.claude/skills/`:

| Skill | Description |
|-------|-------------|
| `argus-doctor` | Diagnostic troubleshooting |
| `argus-intro` | Bootstrap explanation of what Argus is and what setup changes |
| `argus-setup` | Project setup and upgrade guidance |
| `argus-teardown` | Teardown guidance |

Managed global scope (`argus setup` and `argus setup --workspace`) also refreshes these global-only skills for the current user:

| Skill | Description |
|-------|-------------|
| `argus-configure-invariant` | YAML authoring reference for writing invariants with validation and safe-write flow |
| `argus-configure-workflow` | YAML authoring reference for writing workflows with validation and safe-write flow |
| `argus-runtime` | Runtime commands for workflow control, pipeline progress, and invariant operations |
