---
name: argus-intro
description: Explain what Argus is, why its bootstrap reminders appear, and what installation changes
version: 0.1.0
---

# argus-intro

Explain Argus to a user who has not installed it in the current project yet.

## When to Use

- A global bootstrap invariant asks the user whether to install Argus
- The user asks what Argus is or why this reminder appeared
- The user wants to understand what `argus install` will change before deciding

## Core Message

Argus is a workflow orchestration layer for AI coding agents. It does not replace the agent or execute the work itself. Instead, it:

- injects the next-step context for multi-step workflows
- tracks progress on disk so work survives across sessions
- checks lightweight project invariants and surfaces drift

If useful, explain that Argus exists because agents are strong at local execution but weak at long, stateful, multi-step engineering flows without explicit orchestration.

## Concepts to Explain Briefly

- **Workflow**: the blueprint of jobs the agent should follow
- **Pipeline**: one running instance of a workflow
- **Invariant**: a shell-check guardrail describing what should remain true
- **Skill**: a reusable instruction card for a specific Argus task

Keep this conceptual section short unless the user explicitly asks for more depth.

## Why the Reminder Appeared

Explain this specific situation:

1. The current directory is inside a registered Argus workspace.
2. Global Argus hooks are active for workspace projects.
3. This repository does not have project-level Argus installed yet because `.argus/` is missing.
4. The global invariant `argus-project-init` failed, so Argus surfaced this bootstrap reminder.

## What Installation Changes

### `argus install`

- creates `.argus/` with workflows, invariants, rules, pipeline state dirs, logs, data, and tmp
- releases built-in project-level skills into `.agents/skills/` and `.claude/skills/`
- configures project-level hooks for supported agents

### `argus install --workspace <path>`

- registers the workspace in `~/.config/argus/config.yaml`
- installs or refreshes user-level hooks
- installs or refreshes global bootstrap skills and bootstrap artifacts
- does not install Argus into every repository under that workspace

## Decision Guidance

- Installing now is useful if the user wants workflow orchestration, project invariants, and persistent cross-session progress in this repo.
- Ignoring for now is reasonable if the user only wants to continue the current task without adding Argus to the project yet.
- If the user is unsure, explain the changes above first, then restate the install / ignore choice clearly.

## Response Style

- Keep the explanation brief and decision-oriented
- Start with a plain-language summary, then give only the minimum concepts needed for the current question
- Prefer concrete filesystem effects over abstract architecture when the user is deciding whether to install
- End by restating the available choices: install now, ignore for now, or ask a follow-up question
- Do not install Argus automatically; wait for explicit user confirmation
