# Tick Runtime Control Surface

This document defines `argus tick` as Argus's main runtime control surface. It explains how Argus maps runtime state to injected agent guidance and which parts of the output are treated as contract.

---

## 1. Role and Contract Boundary

`tick` is the passive scheduling point that runs whenever the user sends input to an agent. Argus does not execute the work itself. Instead, it decides what the agent should be reminded of right now and injects that guidance as plain text.

This contract is primarily **semantic**, not byte-for-byte:

- **Stable contract**: output family, routing priority, field meaning, warning behavior, and action affordances such as `argus job-done`
- **Flexible implementation detail**: exact wording inside a template, as long as the same user-visible meaning is preserved

Changes to `tick` routing, output ordering, or the meaning of the injected text are orchestration behavior changes, not mere copy edits.

The current primary templates live under `internal/assets/prompts/`:

- `tick-no-pipeline.md.tmpl`
- `tick-full-context.md.tmpl`
- `tick-minimal.md.tmpl`
- `tick-invariant-failed.md.tmpl`

## 2. Output Model

`tick` always emits plain text when it emits anything at all.

Compatibility rule:

- the first non-whitespace character must not be `[` or `{`

Current Codex hook behavior may treat those prefixes as JSON candidates and reject otherwise valid text output.

`tick` has two output lanes:

- **Primary output**: the main orchestration guidance Argus wants the agent to act on
- **Secondary warnings**: short non-blocking warning lines appended after the primary output when needed

Possible overall outcomes:

- primary output only
- primary output plus one or more warning lines
- warning-only output
- empty output

## 3. Decision Order

The runtime decision order is:

1. **Early skip and fail-open checks**
   - sub-agent input -> empty output
   - hook input parse failure -> warning-only output
   - project-local hook outside an Argus project -> warning-only output
   - scope resolution or pipeline scan failure -> warning-only output
   - multiple active pipelines detected -> warning-only output
   - global hook with no applicable scope -> empty output
2. **Top-level pipeline split**
   - active pipeline present -> active-pipeline routing
   - no active pipeline -> no-pipeline routing
3. **Active-pipeline routing**
   - snoozed in this session -> behave like the no-pipeline view
   - invalid current job state or workflow mismatch -> degrade to the no-pipeline view with warning text
   - state changed since `last_tick` -> full-context output
   - state unchanged -> minimal output
4. **No-pipeline routing**
   - run valid automatic invariants
   - first failing invariant wins and becomes the exclusive primary output
   - if no invariant fails and workflows are available -> no-pipeline output
   - if no invariant fails and no workflows are available -> empty output unless warnings need to be surfaced

Automatic invariant participation:

- first tick in a session: run `auto: always` and `auto: session_start`
- later ticks in the same session: run only `auto: always`

## 4. Primary Output Families

### 4.1 No Pipeline

Use this family when Argus wants the agent to treat the current session as having no visible active pipeline.

Representative example:

```text
Argus: No active pipeline.

Available workflows:
  - release: Standard release process
  - argus-project-init: Initialize Argus for the project

To start: argus workflow start <workflow-id>
```

Typical triggers:

- no active pipeline, automatic invariants passed, workflows are available
- active pipeline exists but is snoozed in the current session
- degraded fallback from an active-pipeline anomaly where Argus still wants to surface start guidance

The snoozed case is a semantic alias of the no-pipeline view, not a separate output family.

### 4.2 Full Context

Use this family when an active pipeline is visible and the current pipeline/job snapshot differs from the previous `last_tick` snapshot.

Representative example:

```text
Argus: Pipeline: release-20240405T103000Z | Workflow: release | Progress: 2/5

Current Job: run_tests
Skill: argus-run-tests

Run all tests and only continue if they pass.

When done: argus job-done [--message "summary"]
To snooze: argus workflow snooze --session abc-123
To cancel: argus workflow cancel
```

Semantic requirements:

- include pipeline identity, workflow identity, progress, and current job
- include the rendered job prompt
- include the job skill only when non-empty
- include action guidance for `job-done`, snooze, and cancel

### 4.3 Minimal Reminder

Use this family when an active pipeline is visible but the current pipeline/job snapshot matches `last_tick`.

Representative example:

```text
Argus: release | Job: run_tests | Progress: 2/5 — When done: argus job-done
```

Semantic requirements:

- keep the reminder short
- preserve the current workflow/job/progress context
- preserve the `argus job-done` affordance so the agent does not lose the completion path during long conversations

### 4.4 Invariant Failed

Use this family when no active pipeline is being surfaced and the first valid automatic invariant fails.

Representative example:

```text
Argus: Invariant check failed:
  - argus-project-init: The project has completed Argus initialization
    Prompt: Generate workflow files for this project under .argus/workflows/.
    Workflow: Start the remediation workflow with `argus workflow start argus-project-init`
```

Semantic requirements:

- only the first failing automatic invariant is surfaced
- invariant failure is mutually exclusive with the other primary output families
- `Prompt:` and `Workflow:` may both appear
- when both appear, `Prompt:` comes before `Workflow:`

## 5. Secondary Warnings and Empty Output

Secondary warnings are appended as short plain-text lines prefixed with `Argus warning:`.

Examples of warning sources:

- hook input parsing failure
- scope resolution failure
- multiple active pipelines detected
- invalid invariant definitions excluded from automatic evaluation
- slow automatic invariant checks
- hook log write failure
- degraded active-pipeline fallback such as missing workflow or missing current job

If there is primary output, warnings are appended after it. If there is no primary output, warnings may become the only output.

`tick` may also emit nothing at all. Representative empty-output cases:

- sub-agent invocation
- global hook with no matching scope
- no active pipeline, no failing automatic invariant, no available workflows, and no warning to surface
- snoozed active pipeline with no workflow list to show

## 6. Mock Mode

`argus tick --mock` reuses the normal `tick` routing but bypasses hook stdin parsing.

When Argus auto-generates a mock session id, it prints an extra debug prefix before the normal `tick` output:

```text
Argus: Mock session: <generated-session-id>
```

This mock-session prefix is not part of the four primary template families. It exists only to make repeated local debugging easier.

`--mock` behavior:

- `--agent` may be omitted
- `--mock-session-id <id>` reuses session state across repeated debug runs
- `--global` exercises the same global-scope routing used by installed global hooks

## 7. Related Documents

- [technical-cli.md](technical-cli.md): command surface, flags, and concise output summary
- [technical-hooks.md](technical-hooks.md): hook transport and host-agent integration details
- [technical-pipeline.md](technical-pipeline.md): `last_tick`, snooze, and session state mechanics
- [technical-invariant.md](technical-invariant.md): automatic invariant evaluation rules and remediation semantics
- [technical-workflow.md](technical-workflow.md): relationship between the `tick` path and the `job-done` return path
