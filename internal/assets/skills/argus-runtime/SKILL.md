---
name: argus-runtime
description: Use Argus runtime commands for pipeline progress, workflow control, and invariant checks
version: 0.1.0
---

# argus-runtime

Use Argus runtime commands to inspect state, drive active pipelines, and run validation commands in the current scope.

## When to Use

- Check what workflow or job is currently running
- Start, cancel, or snooze workflows
- Advance the current job with `job-done`
- Run or inspect invariants during troubleshooting

## Scope

- Runtime commands operate on the resolved Argus scope for the current directory.
- If no Argus scope applies, runtime commands fail instead of inferring ad-hoc local state.

## State Inspection

- `argus status [--json]` — Show the active pipeline, current job, progress, invariant summary, and next-step hints
- `argus workflow list [--json]` — List available workflows in the current scope
- `argus invariant list [--json]` — List invariants in the current scope

## Pipeline Control

- `argus workflow start <id> [--json]` — Start a workflow pipeline
- `argus workflow cancel [--json]` — Cancel the active pipeline
- `argus workflow snooze --session <id> [--json]` — Snooze the active pipeline for the current session; `--session` is required
- `argus job-done [--message "summary"] [--fail] [--end-pipeline] [--json]` — Complete or terminate the current job

## Validation and Diagnostics

- `argus invariant check [id] [--json]` — Run all invariant checks or a single invariant by ID
- `argus invariant inspect [dir] [--json]` — Validate invariant definitions
- `argus workflow inspect [dir] [--json]` — Validate workflow definitions

## Output Mode

- Default output is readable text for both humans and agents.
- Use `--json` only when another tool needs stable structured fields.

## Runtime Guidance

- Argus supports one active pipeline at a time in the current scope.
- Use `argus status` first when you need the high-level picture of pipeline plus invariant state.
- Use `argus job-done` to advance the pipeline after completing the injected job.
- Use `argus invariant check` when you need detailed pass/fail output for a specific invariant or remediation step.
- Invariant checks diagnose drift; they do not auto-fix or auto-start remediation workflows.
- Prefer the current scope's built-in workflow and invariant definitions over ad-hoc shell state when diagnosing orchestration issues.
