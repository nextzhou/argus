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

- `argus uninstall [--yes]` — Remove Argus from current project

## What Uninstall Does

1. Removes `.argus/` directory
2. Removes `argus-*` prefixed skills from the project-level `.agents/skills/` and `.claude/skills/` paths
3. Removes Agent hook configurations
4. Preserves non-argus user skills

## Notes

- Git-tracked files can be restored via `git checkout`
- Project-level uninstall does not remove future workspace/global skill roots
- Codex `config.toml` hook flag is preserved to avoid breaking other hooks
