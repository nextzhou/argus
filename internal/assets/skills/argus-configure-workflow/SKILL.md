---
name: argus-configure-workflow
description: Reference documentation for Argus workflow YAML syntax and authoring workflow
version: 0.1.0
---

# argus-configure-workflow

YAML authoring reference for Argus workflow definitions.

## File Location

Workflow files go in `.argus/workflows/` with `.yaml` extension. Except for `_shared.yaml`, each file must be named `<workflow-id>.yaml`.

## Schema

```yaml
version: v0.1.0
id: my-workflow             # lowercase letters, digits, hyphens
description: "Optional human-readable description"

jobs:
  - id: first_step          # optional but recommended; starts with letter, underscore-separated
    prompt: "Instructions for the agent to execute this job"
    skill: optional-skill-name
    description: "Optional description of this job"
  - id: second_step
    prompt: "Instructions for the next job"
  - ref: _shared.reusable_job
```

## Fields

### `version` (required)

Must be `v0.1.0`. Argus checks major version compatibility at parse time.

### `id` (required)

Unique workflow identifier.

- Regex: `^[a-z0-9]+(-[a-z0-9]+)*$`
- Lowercase letters, digits, and hyphens only
- Must not start or end with a hyphen
- The file name must match the ID exactly: `<id>.yaml`
- **The `argus-` prefix is reserved for built-in workflows.** User-defined workflows must not use this prefix.

### `description` (optional)

Human-readable description of what the workflow does.

### `jobs` (required)

Ordered list of jobs to execute sequentially. At least one job is required.

Each job has the following fields:

#### `id` (optional but recommended)

Job identifier within the workflow.

- Regex: `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`
- Starts with a lowercase letter
- Lowercase letters, digits, and underscores only
- **Note:** Job IDs use underscores (`_`), not hyphens — this differs from workflow IDs which use hyphens (`-`).

#### `prompt` (conditionally required)

Natural-language instructions for the agent. The agent receives this text as context and uses its own tools to carry out the work.

#### `skill` (optional)

Name of an Argus skill to load alongside this job. The skill's SKILL.md content is injected as additional context for the agent.

#### `description` (optional)

Human-readable description of the job's purpose.

#### `ref` (optional)

Reference to a shared job defined in `_shared.yaml`. Format: `_shared.<job_id>`.

**Resolution rule:** Each job must have at least one of `prompt`, `skill`, or `ref`. A job with only `ref` inherits all fields from the shared definition. A job can combine `ref` with local fields — local fields override the shared definition.

## Shared Jobs

Define reusable jobs in `.argus/workflows/_shared.yaml` and reference them with `ref: _shared.<job_id>`.

```yaml
# .argus/workflows/_shared.yaml
jobs:
  - id: run_tests
    prompt: "Run the full test suite and report results"
  - id: lint_check
    prompt: "Run linters and fix any issues"
```

Reference shared jobs in any workflow:

```yaml
version: v0.1.0
id: release
jobs:
  - ref: _shared.run_tests
  - ref: _shared.lint_check
  - id: tag_release
    prompt: "Create a git tag and push it"
```

## Template

Copy-pasteable starting point:

```yaml
version: v0.1.0
id: my-workflow
description: "Describe the workflow's purpose"

jobs:
  - id: step_one
    prompt: "Instructions for the first step"
  - id: step_two
    prompt: "Instructions for the second step"
    skill: relevant-skill-name
  - id: step_three
    prompt: "Instructions for the final step"
```

## Validation

Validate workflow definitions before applying:

```
argus workflow inspect [dir] [--json]
```

When `[dir]` is omitted, it validates `.argus/workflows/` by default.

Validation also enforces the `<id>.yaml` file-name contract. The only exception is `_shared.yaml`.

## Safe-Write Flow

When creating or editing workflow files, use a staging directory to avoid corrupting the live definitions:

1. **Prepare staging directory.** Clean `.argus/tmp/workflows/` if it exists, then create it fresh.

2. **Copy existing user files.** Copy non-`argus-*` YAML files from `.argus/workflows/` to `.argus/tmp/workflows/`. Built-in `argus-*` files are managed by `argus setup` — do not copy or edit them. If creating from scratch and no user files exist, just create the empty `.argus/tmp/workflows/` directory.

3. **Make all changes in staging.** Create, edit, or delete files only in `.argus/tmp/workflows/`.

4. **Validate.** Run `argus workflow inspect .argus/tmp/workflows/` and confirm all definitions pass.

5. **Apply.** If valid: replace non-`argus-*` files in `.argus/workflows/` with the contents of `.argus/tmp/workflows/`. Do not touch `argus-*` built-in files. Ensure user file deletions are synced (remove files from `.argus/workflows/` that no longer exist in staging).

6. **On failure.** If validation fails: fix errors in `.argus/tmp/workflows/` and re-validate. Do not touch the original `.argus/workflows/` directory until validation passes.
