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
- `argus teardown --workspace <path> [--yes] [--json]` — Remove a workspace registration and, if it is the last one, tear down global hooks, global skills, global artifacts, and the managed `~/.config/argus/` root

## Output Mode

- Default output is readable text and includes a summary plus affected paths.
- Use `--json` only when another command or script needs structured change data.
- `--json` is non-interactive. If confirmation would otherwise be required, pass `--yes` as well.

## Confirmation

- `argus teardown` and `argus teardown --workspace <path>` require confirmation unless `--yes` is provided.
- In JSON mode, these commands return an error unless `--yes` is also provided.

## What Teardown Does

1. Removes the project-level `.argus/` directory
2. Removes `argus-*` prefixed skills from the project-level `.agents/skills/` and `.claude/skills/` paths
3. Removes Argus-managed `tick` hook configurations
4. Preserves non-argus user skills

Workspace teardown (`argus teardown --workspace <path>`) instead:

1. Removes the normalized workspace path from `~/.config/argus/config.yaml`
2. If it was the last registered workspace, removes global hooks, global skills, managed global artifacts, and the global `~/.config/argus/` root
3. Preserves unrelated user-managed skills and hook settings outside the Argus-managed entries

## Notes

- Git-tracked files can be restored via Git if needed
- Project-level teardown does not unregister workspaces
- Project-level teardown does not remove user-level global skill roots or managed global skills refreshed by `argus setup`
- Codex `config.toml` hook flag is preserved to avoid breaking other hooks
