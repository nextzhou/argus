---
name: argus-uninstall
description: Guide Argus uninstallation including hook removal and config cleanup
version: 0.1.0
---

# argus-uninstall

Remove Argus from a project.

## When to Use

- Removing Argus from a project
- Cleaning up before reinstall

## Commands

- `argus uninstall [--yes] [--json]` — Remove Argus from current project
- `argus uninstall --workspace <path> [--yes] [--json]` — Remove a workspace registration and, if it is the last one, clean up global hooks and global skills

## Output Mode

- Default output is readable text and includes a summary plus affected paths.
- Use `--json` only when another command or script needs structured change data.

## What Uninstall Does

1. Removes `.argus/` directory
2. Removes `argus-*` prefixed skills from the project-level `.agents/skills/` and `.claude/skills/` paths
3. Removes Argus-managed `tick` hook configurations
4. Preserves non-argus user skills

Workspace uninstall (`argus uninstall --workspace <path>`) instead:

1. Removes the normalized workspace path from `~/.config/argus/config.yaml`
2. If it was the last registered workspace, removes global hooks and global independent skills
3. Preserves unrelated user-managed skills and hook settings outside the Argus-managed entries

## Notes

- Git-tracked files can be restored via `git checkout`
- Project-level uninstall does not remove future workspace/global skill roots
- Codex `config.toml` hook flag is preserved to avoid breaking other hooks
