---
name: argus-configure-invariant
description: Reference documentation for Argus invariant YAML syntax and authoring workflow
version: 0.1.0
---

# argus-configure-invariant

YAML authoring reference for Argus invariant definitions.

## File Location

Invariant files go in `.argus/invariants/` with `.yaml` extension. Each file must be named `<invariant-id>.yaml`.

## Schema

```yaml
version: v0.1.0
id: my-invariant          # lowercase letters, digits, hyphens
description: "Optional human-readable description"
auto: never               # when to run: always | session_start | never

check:
  - shell: "test -f some-file.txt"
    description: "Optional description of what this step verifies"
  - shell: "grep -q expected-pattern config.yaml"
    description: "Config contains expected pattern"

prompt: "Remediation instructions for the agent when check fails"
workflow: remediation-workflow-id
```

## Fields

### `version` (required)

Must be `v0.1.0`. Argus checks major version compatibility at parse time.

### `id` (required)

Unique invariant identifier.

- Regex: `^[a-z0-9]+(-[a-z0-9]+)*$`
- Lowercase letters, digits, and hyphens only
- Must not start or end with a hyphen
- The file name must match the ID exactly: `<id>.yaml`
- **The `argus-` prefix is reserved for built-in invariants.** User-defined invariants must not use this prefix.

### `description` (optional)

Human-readable description of what the invariant checks.

### `auto` (optional)

Controls when the invariant is automatically checked during `tick`.

| Value | Behavior |
|-------|----------|
| `always` | Checked on every tick when no pipeline is active |
| `session_start` | Checked once per session |
| `never` | Only checked manually (default if omitted or empty) |

### `check` (required)

At least one check step is required. Each step has:

- `shell` (required): Shell command to execute. Exit code 0 = pass, non-zero = fail.
- `description` (optional): Describes what this step verifies.

Checks are pure shell commands — no LLM involvement. Keep them fast and deterministic. For complex semantic checks, convert to timestamp-based freshness checks (e.g., `find .argus/data/reviewed -mtime -7 | grep -q .`).

### `prompt` and `workflow` (at least one required)

Must have `prompt` or `workflow` or both. These define what happens when the check fails:

- `prompt`: Remediation instructions injected to the agent.
- `workflow`: ID of a workflow to suggest for remediation.

## Template

Copy-pasteable starting point:

```yaml
version: v0.1.0
id: my-check
description: "Describe what should be true"
auto: session_start

check:
  - shell: "test -f required-file.txt"
    description: "Required file exists"

prompt: "The required file is missing. Please create it with the expected content."
workflow: setup-project
```

## Validation

Validate invariant definitions before applying:

```
argus invariant inspect [dir] [--json]
```

When `[dir]` is omitted, it validates `.argus/invariants/` by default.

Validation also enforces the `<id>.yaml` file-name contract.

## Safe-Write Flow

When creating or editing invariant files, use a staging directory to avoid corrupting the live definitions:

1. **Prepare staging directory.** Clean `.argus/tmp/invariants/` if it exists, then create it fresh.

2. **Copy existing user files.** Copy non-`argus-*` YAML files from `.argus/invariants/` to `.argus/tmp/invariants/`. Built-in `argus-*` files are managed by `argus setup` — do not copy or edit them. If creating from scratch and no user files exist, just create the empty `.argus/tmp/invariants/` directory.

3. **Make all changes in staging.** Create, edit, or delete files only in `.argus/tmp/invariants/`.

4. **Validate.** Run `argus invariant inspect .argus/tmp/invariants/` and confirm all definitions pass.

5. **Apply.** If valid: replace non-`argus-*` files in `.argus/invariants/` with the contents of `.argus/tmp/invariants/`. Do not touch `argus-*` built-in files. Ensure user file deletions are synced (remove files from `.argus/invariants/` that no longer exist in staging).

6. **On failure.** If validation fails: fix errors in `.argus/tmp/invariants/` and re-validate. Do not touch the original `.argus/invariants/` directory until validation passes.
