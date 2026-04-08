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

- `argus install [--yes]` — Install Argus in current Git project
- `argus install --workspace` — Install workspace-level configuration (future)

## What Install Does

1. Creates `.argus/` directory structure (workflows, invariants, rules, pipelines, logs, data, tmp)
2. Releases built-in workflows and invariants to `.argus/workflows/` and `.argus/invariants/`
3. Releases built-in project-level skills to `.agents/skills/argus-*/` and mirrors them to `.claude/skills/argus-*/`; OpenCode discovers from these compatibility paths, so project install does not create `.opencode/skills/`
4. Configures Agent hooks (Claude Code, Codex, OpenCode)

## Prerequisites

- Must be inside a Git repository
- No ancestor `.argus/` directory (prevents nested installs)

## After Install

Run `argus workflow start argus-init` to complete project initialization.
