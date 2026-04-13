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
- `tick-snoozed.md.tmpl`
- `tick-invariant-failed.md.tmpl`
- `tick-active-pipeline-issue.md.tmpl`
- `tick-multiple-active-pipelines.md.tmpl`

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
   - multiple active pipelines detected -> dedicated anomaly guidance output
   - global hook with no applicable scope -> empty output
2. **Top-level pipeline split**
   - active pipeline present -> active-pipeline routing
   - no active pipeline -> no-pipeline routing
3. **Active-pipeline routing**
   - snoozed in this session -> snoozed output
   - invalid current job state, workflow load failure, or workflow mismatch -> active-pipeline-issue output
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

Use workflow-first judgment for the current request:
- If one workflow is a uniquely strong match, briefly say why and start it.
- If several workflows are strong matches, let the user choose.
- Otherwise continue the current task quietly.
```

Typical triggers:

- no active pipeline, automatic invariants passed, workflows are available
- no active pipeline is being surfaced and Argus wants workflow availability to influence the agent's next move

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
- explicitly tell the agent to continue the current job by default
- include action guidance for `job-done`, snooze, and cancel

### 4.3 Minimal Reminder

Use this family when an active pipeline is visible but the current pipeline/job snapshot matches `last_tick`.

Representative example:

```text
Argus: Workflow release | Job run_tests | Progress 2/5 | Continue this job. When done: argus job-done
```

Semantic requirements:

- keep the reminder short
- preserve the current workflow/job/progress context
- preserve the `argus job-done` affordance so the agent does not lose the completion path during long conversations

### 4.4 Snoozed

Use this family when an active pipeline exists but has been snoozed in the current session.

Representative example:

```text
Argus: The active pipeline is snoozed for this session.

Do not resurface or resume it unless the user asks to return to it.
Do not auto-start another workflow just because one is available.
```

Semantic requirements:

- preserve snooze as a real suppression mechanism, not as a disguised no-pipeline view
- keep the guidance quiet and conservative
- avoid encouraging unrelated workflow starts while a pipeline is merely hidden

### 4.5 Invariant Failed

Use this family when no active pipeline is being surfaced and the first valid automatic invariant fails.

Representative example:

```text
Argus: Pause the current task for a moment. Invariant check failed, and I found a project issue that should be handled before you continue.
Invariant: argus-project-init
The overall expectation for this project is: The project has completed Argus initialization

Argus says this check is not currently satisfied: Rules directory exists. It checked this by running `test -d .argus/rules`.
Argus concluded that this check failed because the command exited with code 1 and reported failure kind `exit`.

The recommended way to fix it is: Generate workflow files for this project under .argus/workflows/.
If you want to handle it through Argus, the primary workflow is `argus workflow start argus-project-init`.

Before answering the user's original request or continuing the current task, stop and ask the user how to proceed.
Prefer the agent's structured choice input tool when available.
```

Semantic requirements:

- only the first failing automatic invariant is surfaced
- invariant failure is mutually exclusive with the other primary output families
- use a spoken flow: pause -> overall expectation -> unmet check expectation -> observed evidence -> remediation -> user options
- include the invariant goal plus the unmet check expectation and observed evidence when available, including the original shell command when it helps explain how Argus checked it
- feed the template with internal invariant data plus the failed step's runtime result, instead of a separate presentation-only fact model
- treat invariant `prompt` and `workflow` as remediation inputs, not as the final user-choice UX
- tell the agent to stop and collect a user choice before continuing the original task or starting remediation
- prefer the host agent's structured choice input when available
- tell the agent to present a concise choice set instead of copying low-quality invariant wording verbatim
- if the user chooses to ignore the reminder, continue normally and do not resurface the same reminder again in the same conversation unless the user asks about it

### 4.6 Active Pipeline Issue

Use this family when a running pipeline exists but cannot be continued safely, for example because the current job is missing, the workflow definition cannot be loaded, or the current job no longer exists in the workflow.

Representative example:

```text
Argus: Active pipeline needs attention.
Pipeline: release-20240405T103000Z | Workflow: release

Issue: current job deploy was not found in workflow release

Guide the user with a concise choice set before taking action:
1. Investigate first with `argus status`.
2. Cancel the broken pipeline with `argus workflow cancel`.
3. Snooze this pipeline for the current session with `argus workflow snooze --session <id>`.
```

Semantic requirements:

- do not fall back to the normal no-pipeline workflow-start guidance
- prefer real dismiss actions such as `snooze` when they exist
- choose the investigate command based on the likely solution path, for example `status` for runtime state and `doctor` for structural drift

### 4.7 Multiple Active Pipelines

Use this family when directory scanning finds more than one running pipeline, which is an anomaly in the Phase 1 model.

Semantic requirements:

- list the conflicting pipeline instance IDs
- direct the agent toward resolving ambiguity before continuing work
- surface concrete actions such as `workflow cancel`, `workflow snooze --session`, and `doctor`

## 5. Secondary Warnings and Empty Output

Secondary warnings are appended as short plain-text lines prefixed with `Argus warning:`.

Examples of warning sources:

- hook input parsing failure
- scope resolution failure
- invalid invariant definitions excluded from automatic evaluation
- slow automatic invariant checks
- hook log write failure
- low-level hook/runtime failures that do not produce a richer dedicated guidance template

If there is primary output, warnings are appended after it. If there is no primary output, warnings may become the only output.

`tick` may also emit nothing at all. Representative empty-output cases:

- sub-agent invocation
- global hook with no matching scope
- no active pipeline, no failing automatic invariant, no available workflows, and no warning to surface

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
