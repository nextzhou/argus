---
name: argus-install
description: Guide Argus installation, project initialization, and version upgrades
version: 0.1.0
---

# argus-install

Install and initialize Argus in a project.

## When to Use

- Fresh Argus installation in a new project
- Re-installing after uninstall
- Upgrading Argus version

## Commands

- `argus install [--yes] [--json]` — Install Argus in current Git project
- `argus install --workspace <path> [--yes] [--json]` — Register a workspace and install global hooks and global skills

## Output Mode

- Default output is readable text and includes a summary plus affected paths.
- Use `--json` only when another command or script needs structured change data.

## What Install Does

1. Creates `.argus/` directory structure (workflows, invariants, rules, pipelines, logs, data, tmp)
2. Releases built-in workflows and invariants to `.argus/workflows/` and `.argus/invariants/`
3. Releases built-in project-level skills to `.agents/skills/argus-*/` and mirrors them to `.claude/skills/argus-*/`; OpenCode discovers from these compatibility paths, so project install does not create `.opencode/skills/`
4. Configures Agent hooks (Claude Code, Codex, OpenCode)

Workspace install (`argus install --workspace <path>`) instead:

1. Registers the normalized workspace path in `~/.config/argus/config.yaml`
2. Installs global hooks for Claude Code, Codex, and OpenCode
3. Releases global independent skills to Agent-level skill directories
4. Does not install Argus into any project yet

## Prerequisites

- Must be inside a Git repository
- No ancestor `.argus/` directory (prevents nested installs)
- Workspace install requires the target path to already exist and be a directory

## After Install

Run `argus workflow start argus-init` to complete project initialization.
