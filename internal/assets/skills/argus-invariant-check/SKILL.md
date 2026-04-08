---
name: argus-invariant-check
description: Manually trigger invariant checks and view results
version: 0.1.0
---

# argus-invariant-check

Run invariant checks to verify project state compliance.

## When to Use

- Verify project meets defined invariants
- Debug failing invariant checks
- After making project configuration changes

## Commands

- `argus invariant check` — Run all invariant checks
- `argus invariant list` — List defined invariants

## Invariant System

Invariants define "what should be true" about a project. Each invariant has shell-based checks that run without LLM involvement. Failed invariants suggest remediation workflows.
