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

- `argus workflow list` — List available workflows
- `argus workflow start <id>` — Start a workflow pipeline
- `argus workflow cancel` — Cancel active pipeline
- `argus workflow snooze --session <id>` — Snooze pipeline for current session

## Workflow Lifecycle

1. `workflow start` creates a pipeline
2. Each job is executed sequentially
3. Complete jobs with `argus job-done [--message "summary"]`
4. Pipeline completes when all jobs are done
