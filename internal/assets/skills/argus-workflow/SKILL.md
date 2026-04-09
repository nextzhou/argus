---
name: argus-workflow
description: Start or manage Argus workflow executions
version: 0.1.0
---

# argus-workflow

Manage Argus workflow lifecycle.

## When to Use

- Starting a new workflow
- Listing available workflows
- Cancelling or snoozing active pipelines

## Commands

- `argus workflow list [--json]` — List available workflows
- `argus workflow start <id> [--json]` — Start a workflow pipeline
- `argus workflow cancel [--json]` — Cancel active pipeline
- `argus workflow snooze --session <id> [--json]` — Snooze pipeline for current session
- `argus job-done [--message "summary"] [--json]` — Complete the current job

## Output Mode

- Default output is readable text for both humans and agents.
- Use `--json` only when you need field-level parsing or script integration.

## Workflow Lifecycle

1. `workflow start` creates a pipeline
2. Each job is executed sequentially
3. Complete jobs with `argus job-done [--message "summary"]`
4. Pipeline completes when all jobs are done
