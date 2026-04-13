# Workflow Specification (Technical Workflow)

This document defines the Argus workflow system, including the YAML schema, job model, execution flow, and validation rules.

---

## 1. System Positioning and Design Philosophy

Workflow is the imperative orchestration component in Argus. It answers “how to do this” by decomposing a complex task into a sequence of ordered jobs.

Following the architecture invariants from [technical-overview.md](technical-overview.md), workflows follow these principles:

- **Argus is the orchestration layer**: it tracks state and injects context, but does not execute business logic directly
- **Agent-driven execution**: all actual operations are executed by the agent through prompt and skill injection
- **Determinism versus adaptability**: by removing the `script` field, Argus gives execution control fully to the agent in exchange for better environment adaptability and better user visibility

---

## 2. Workflow YAML Schema

### 2.1 File-Level Fields

Workflow definitions live in `.argus/workflows/`.

Except for `_shared.yaml`, each workflow file name is part of the contract: the file must be named `<workflow-id>.yaml`.

| Field | Required | Meaning |
|------|----------|---------|
| `version` | Yes | Schema version, currently fixed at `v0.1.0` |
| `id` | Yes | Machine identifier. Used by pipeline data and CLI parameters. Must match `^[a-z0-9]+(-[a-z0-9]+)*$`. The `argus-` prefix is reserved for built-ins |
| `description` | No | Human-readable description |
| `jobs` | Yes | Ordered list of jobs |

Example:

```yaml
version: v0.1.0
id: release
description: "Standard release process"
jobs:
  - id: run_tests
    prompt: "Run `go test ./...` and report the results"
```

### 2.2 Job Fields

Job is the smallest executable unit of a workflow.

| Field | Required | Meaning |
|------|----------|---------|
| `id` | Yes* | Unique job identifier within the workflow. May be omitted when `ref` is used |
| `description` | No | Human-readable description |
| `ref` | No | Reference to a shared job in `_shared.yaml` |
| `prompt` | No** | Text injected into the agent. Supports template variables |
| `skill` | No** | Skill name to be executed by the agent |

\* If `ref` is used and `id` is omitted, the job inherits the `ref` key as its ID.  
\** `prompt` and `skill` may not both be empty.

Examples:

```yaml
# Prompt-only: the agent follows instructions directly
- id: run_tests
  prompt: "Run `go test ./...` and report the results"

# Skill-only: the agent uses a named skill
- id: generate_rules
  skill: argus-runtime

# Original script-style scenario expressed as a multi-line prompt
- id: lint_and_fix
  prompt: |
    Run `golangci-lint run ./...`.
    If lint errors appear, fix them.
    If lint passes, continue.
```

---

## 3. Job Model: Fully Agent-Driven

### 3.1 Why the `script` Field Was Removed

Early designs allowed a `script` field so Argus could execute bash commands directly. That design was intentionally abandoned.

#### Tradeoff Analysis

| Dimension | `script` design (rejected) | Agent prompt design (current) |
|------|-----------------------------|-------------------------------|
| Implementation cost | High: Argus would need process management, timeouts, environment injection | Low: Argus only manages state and context injection |
| Visibility | Poor: users cannot see blocked hook execution in the agent UI | Good: the agent’s execution process is visible to the user |
| Adaptability | Poor: hardcoded commands fail easily across environments and cannot self-repair | Strong: the agent can adapt commands and recover from failures |
| Execution model | Complex: two execution paths (Argus runs commands vs agent runs commands) | Simple: inject context -> agent acts -> `job-done` |

#### Cost of Removing `script`

- **Determinism**: exact command reproduction is no longer guaranteed, but prompts can still constrain the agent tightly when needed, for example “Run `git tag v1.0.0` and do not alter the command”
- **Cost and latency**: the extra LLM turn is not a bottleneck in workflow scenarios
- **Automation**: the agent must call `job-done`, but the workflow becomes more robust and observable

### 3.2 Execution Loop

Each job follows the same loop:

1. **Injection**: Argus injects `skill` and `prompt` into the agent
2. **Execution**: the agent performs the work using its own tools
3. **Completion**: the agent calls `job-done`, which advances the pipeline

---

## 4. Ref Syntax and `_shared.yaml`

Argus supports shared job definitions for reuse.

### 4.1 `_shared.yaml` Structure

Shared jobs live in `.argus/workflows/_shared.yaml`.

- no `version` field is needed
- all jobs are nested under `jobs:`
- shared job keys must follow the same naming rule as template-safe job IDs: `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`

Example:

```yaml
# .argus/workflows/_shared.yaml
jobs:
  lint:
    prompt: "Run `golangci-lint run ./...` and fix any errors"

  code_review:
    prompt: "Review changes for code quality and security"
```

### 4.2 Reference Mechanism

- use `ref` to reference a shared job
- optionally provide `id` to rename the instance
- any explicitly written field overrides the shared definition
- if the concrete job omits `id`, the resolved job ID defaults to the `ref` key itself, not to any `id` field that may be written inside the shared job body

Example:

```yaml
version: v0.1.0
id: release

jobs:
  - ref: lint

  - ref: code_review
    id: strict_review
    prompt: "Review with extra focus on security"

  - id: tag_release
    prompt: "Create a git tag {{ .env.version }} and push it"
```

Rejected alternatives:

- YAML anchors (`&` / `*`): do not work well across files
- GitLab-style `extends`: too verbose for simple job reuse

### Ref Merge Semantics

Ref resolution uses a **shallow merge**:

| Case | Behavior |
|------|----------|
| Field absent on the concrete job | Keep inherited value |
| Field explicitly set on the concrete job | Override inherited value |
| Field set to `null` or empty string | Explicitly clear the inherited value |

The `id` field has one extra rule because it participates in template lookup and pipeline state:

| Concrete job shape | Resolved job ID |
|------|----------|
| `ref: lint` | `lint` |
| `ref: lint` plus `id: strict_lint` | `strict_lint` |

Argus does not inherit `id` from the shared job body when the concrete job omits it. This keeps the instance identity stable and obvious from the workflow file itself.

Example:

```yaml
# _shared.yaml
jobs:
  standard_test:
    skill: argus-run-tests
    prompt: "Run the standard test suite"

# workflow.yaml
jobs:
  - id: custom_test
    ref: standard_test
    prompt: "Run custom tests, including integration tests"
    # skill is not present -> inherited from standard_test
```

---

## 5. Template Variables

Argus renders `prompt` with Go `text/template` before dispatching the job.

### 5.1 Variable Categories and Phase 1 Field Set

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

`pre_job` provides a shortcut to the immediate predecessor without requiring knowledge of its ID.

**Job ID naming constraint**: job IDs used through `.jobs.<job_id>.message` must match `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`. Hyphens are forbidden because Go `text/template` dot syntax would treat them as subtraction, and the identifier must start with a letter.

**Missing-variable handling**: Argus uses partial replacement. Known variables are rendered. Unknown variables remain as their original placeholders and also produce a warning on stderr. For the `jobs` category, references to non-existent jobs or empty messages are treated as missing values and are left untouched.

### 5.2 Validation Rules

- the `jobs` list may not be empty
- built-in workflows such as `argus-project-init` are released into `.argus/workflows/` and validated using the same mechanism as user-defined workflows

---

## 6. Execution Flow and State Progression

Workflows support two progression paths.

### 6.1 Dual Advancement Paths

| Path | Trigger | Typical scenario |
|------|---------|------------------|
| **`tick` path** | user input triggers a hook | resume execution, continue across sessions, inspect progress |
| **`job-done` return path** | agent calls `job-done` | uninterrupted execution; `job-done` returns the next job immediately |

### 6.2 Tick Injection Strategy

To avoid over-injecting during long conversations:

- **when state changes** (new job or new pipeline): inject full context, including prompt, skill, and action guidance
- **when state is unchanged**: inject a minimal summary with the current job ID and reminder so the agent does not forget to call `job-done`

The complete `tick` routing contract, including no-pipeline and invariant-driven outputs, is documented in [technical-tick.md](technical-tick.md).

### 6.3 Human in the Loop

Argus intentionally avoids fine-grained control fields:

- no `confirm` field
- no `auto` field on jobs
- confirmation requirements are written directly into prompts
- the agent decides when to pause and ask the user

---

## 7. Workflow Validation (`workflow inspect`)

### 7.1 Command

```bash
argus workflow inspect [dir] [--json]
```

- validation is always directory-based so cross-file references and duplicate IDs can be checked correctly
- the default target is `.argus/workflows/`
- default output is readable text; `--json` returns structured results

### 7.2 What Gets Validated

1. YAML syntax and required fields
2. logical consistency, including duplicate IDs and valid `ref` references
3. preventive checks such as unknown-key detection and template syntax validation
4. version compatibility
5. `ref` override compatibility with shared definitions
6. detection of init workflows shipped by the Argus binary
7. workflow ID format validation (`^[a-z0-9]+(-[a-z0-9]+)*$`)
8. file-name validation: every workflow file except `_shared.yaml` must be named `<id>.yaml`
9. reserved-namespace validation for the `argus-` prefix, while allowing built-in IDs embedded in the current Argus binary

### 7.3 JSON Output

`workflow inspect --json` returns an explicit `entries[]` array rather than a filename-keyed map. Each entry carries:

- `source`: `{kind, raw}` metadata for the inspected source
- `valid`: whether that source passed validation
- `findings[]`: structured validation problems with `code`, `message`, `source`, and optional `field_path`
- `workflow` or `shared`: metadata for valid workflow files or `_shared.yaml`

**Valid case**

```json
{
  "status": "ok",
  "valid": true,
  "entries": [
    {
      "source": {"kind": "file", "raw": "/repo/.argus/workflows/_shared.yaml"},
      "valid": true,
      "shared": {"jobs": ["lint", "code_review"]}
    },
    {
      "source": {"kind": "file", "raw": "/repo/.argus/workflows/release.yaml"},
      "valid": true,
      "workflow": {"id": "release", "jobs": 4}
    }
  ]
}
```

**Invalid case**

```json
{
  "status": "ok",
  "valid": false,
  "entries": [
    {
      "source": {"kind": "file", "raw": "/repo/.argus/workflows/release.yaml"},
      "valid": false,
      "findings": [
        {
          "code": "invalid_template",
          "message": "invalid template syntax: template: :1: unclosed action",
          "source": {"kind": "file", "raw": "/repo/.argus/workflows/release.yaml"},
          "field_path": "jobs[2].prompt"
        },
        {
          "code": "missing_ref",
          "message": "ref 'nonexistent' not found in _shared.yaml",
          "source": {"kind": "file", "raw": "/repo/.argus/workflows/release.yaml"},
          "field_path": "jobs[3].ref"
        }
      ]
    }
  ]
}
```

### 7.4 Recommended Agent Editing Workflow

When an agent edits workflow definitions, the recommended safe sequence is:

1. copy `.argus/workflows/` to a temporary directory such as `/tmp/argus-draft/`
2. edit files in that temporary directory
3. run `argus workflow inspect /tmp/argus-draft/`
4. after validation succeeds, replace the original directory atomically

---

## 8. Deferred Features

To keep Phase 1 intentionally small, the following are deferred:

- **Parallel DAG execution**: workflows are sequential only
- **Constraints field**: negative constraints are expressed directly in prompts
- **Trap gating rules in workflow YAML**: fine-grained tool-interception rules are not configurable in workflow YAML yet
- **`script` field**: permanently removed, as described in §3.1
