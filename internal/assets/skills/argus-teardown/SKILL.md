---
name: argus-teardown
description: Guide Argus teardown including hook removal and config cleanup
version: 0.1.0
---

# argus-teardown

Remove project-level Argus setup from a repository.

## When to Use

- Removing project-level Argus setup from a repository
- Cleaning up before re-running setup

## Commands

- `argus teardown [--yes] [--json]` — Remove project-level Argus setup from the current directory
- `argus teardown --workspace <path> [--yes] [--json]` — Remove a workspace registration and, if it is the last one, tear down global hooks, global skills, global bootstrap artifacts, and the managed `~/.config/argus/` root

## Output Mode

- Default output is readable text and includes a summary plus affected paths.
- Use `--json` only when another command or script needs structured change data.

## What Teardown Does

1. Removes the project-level `.argus/` directory
2. Removes `argus-*` prefixed skills from the project-level `.agents/skills/` and `.claude/skills/` paths
3. Removes Argus-managed `tick` hook configurations
4. Preserves non-argus user skills

Workspace teardown (`argus teardown --workspace <path>`) instead:

1. Removes the normalized workspace path from `~/.config/argus/config.yaml`
2. If it was the last registered workspace, removes global hooks, global bootstrap skills, and the global `~/.config/argus/` root
3. Preserves unrelated user-managed skills and hook settings outside the Argus-managed entries

## Notes

- Git-tracked files can be restored via `git checkout`
- Project-level teardown does not remove future workspace/global skill roots
- Codex `config.toml` hook flag is preserved to avoid breaking other hooks
