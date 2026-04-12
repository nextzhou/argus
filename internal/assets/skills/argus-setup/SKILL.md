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

- `argus setup [--yes] [--json]` — Set up project-level Argus in the current Git directory and refresh managed global skills
- `argus setup --workspace <path> [--yes] [--json]` — Register a workspace and set up or refresh global hooks, global skills, and global artifacts

## Important Scope Note

`argus setup` is not project-only in effect. It also refreshes the managed global Argus skills for the current user so the corresponding global-only skills remain available locally.

## Output Mode

- Default output is readable text and includes a summary plus affected paths.
- Use `--json` only when another command or script needs structured change data.

## What Setup Does

1. Creates `.argus/` directory structure (workflows, invariants, rules, pipelines, logs, data, tmp)
2. Releases the built-in `argus-project-init` workflow and invariant to `.argus/workflows/` and `.argus/invariants/`
3. Releases built-in project-level lifecycle skills to `.agents/skills/argus-*/` and mirrors them to `.claude/skills/argus-*/`; OpenCode discovers from these compatibility paths, so project setup does not create `.opencode/skills/`
4. Refreshes the managed global Argus skills under the user's Agent skill directories so global-only built-in skills are available locally
5. Configures Agent `tick` hooks (Claude Code, Codex, OpenCode)

This establishes the managed project scaffold. It does not yet survey the repository or build the project-specific collaboration model. That work belongs to the built-in `argus-project-init` workflow.

Workspace setup (`argus setup --workspace <path>`) instead:

1. Registers the normalized workspace path in `~/.config/argus/config.yaml`
2. Sets up global `tick` hooks for Claude Code, Codex, and OpenCode
3. Releases or refreshes the managed global Argus skills in Agent-level skill directories
4. Releases or refreshes the managed global artifacts, including the workspace bootstrap invariant
5. Does not set up project-level Argus in any repository yet

## Prerequisites

- Must be inside a Git repository
- No ancestor `.argus/` directory (prevents nested project-level setup)
- Workspace setup requires the target path to already exist and be a directory

## After Setup

Run `argus workflow start argus-project-init` to complete project initialization.

That workflow is expected to:

1. survey the repository and identify its real build, test, lint, CI, hook, and documentation entrypoints
2. draft a concrete initialization plan and wait for explicit user approval before applying high-impact changes
3. refresh project rules, then project-specific invariants, and then project-specific workflows so the approved target state is defined before the remediation paths
4. align repo-managed `.argus/` tracking, local entrypoints such as `.gitignore`, and git hooks with the approved plan while keeping runtime-only Argus state ignored
5. update only the collaboration-focused parts of README or CONTRIBUTING when needed

`argus-project-init` establishes the repo-managed collaboration model under `.argus/`. Agent integration directories created by `argus setup`, such as `.agents/`, `.claude/`, `.codex/`, and `.opencode/`, are setup artifacts rather than required project-init outputs unless the repository already chooses to track them.
