---
name: argus-setup
description: Guide Argus setup, project initialization, and version upgrades
version: 0.1.0
---

# argus-setup

Set up project-level Argus in a repository and guide the follow-up initialization flow.

## When to Use

- Fresh project-level Argus setup in a new repository
- Re-running setup after teardown
- Upgrading Argus version

## Commands

- `argus setup [--yes] [--json]` — Set up project-level Argus in the current Git directory
- `argus setup --workspace <path> [--yes] [--json]` — Register a workspace and set up or refresh global hooks, global skills, and global bootstrap artifacts

## Output Mode

- Default output is readable text and includes a summary plus affected paths.
- Use `--json` only when another command or script needs structured change data.

## What Setup Does

1. Creates `.argus/` directory structure (workflows, invariants, rules, pipelines, logs, data, tmp)
2. Releases the built-in `argus-project-init` workflow and invariant to `.argus/workflows/` and `.argus/invariants/`
3. Releases built-in project-level skills to `.agents/skills/argus-*/` and mirrors them to `.claude/skills/argus-*/`; OpenCode discovers from these compatibility paths, so project setup does not create `.opencode/skills/`
4. Configures Agent `tick` hooks (Claude Code, Codex, OpenCode)

This establishes the managed project scaffold. It does not complete project-specific initialization tasks such as generating custom rules, workflows, and example invariants.

Workspace setup (`argus setup --workspace <path>`) instead:

1. Registers the normalized workspace path in `~/.config/argus/config.yaml`
2. Sets up global `tick` hooks for Claude Code, Codex, and OpenCode
3. Releases global bootstrap skills to Agent-level skill directories
4. Refreshes those global resources when the workspace is already registered
5. Does not set up project-level Argus in any repository yet

## Prerequisites

- Must be inside a Git repository
- No ancestor `.argus/` directory (prevents nested project-level setup)
- Workspace setup requires the target path to already exist and be a directory

## After Setup

Run `argus workflow start argus-project-init` to complete project initialization.
