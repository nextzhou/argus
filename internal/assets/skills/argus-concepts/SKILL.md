---
name: argus-concepts
description: Introduction to Argus terminology, architecture, and core concepts
version: 0.1.0
---

# argus-concepts

Core concepts and terminology for the Argus workflow orchestration system.

## Key Concepts

- **Workflow**: A YAML definition describing a sequence of jobs (imperative process)
- **Invariant**: A YAML definition describing conditions that should be true (declarative checks)
- **Pipeline**: A running instance of a workflow
- **Job**: A single step within a workflow, executed by the AI Agent
- **Skill**: A SKILL.md file providing specialized instructions to Agents
- **Rule**: A constraint/specification for coding (not to be confused with Skill)
- **Hook**: Integration point where Argus injects context into Agent conversations

## Architecture

Argus is an orchestration layer. It does not execute commands or make decisions. It tells the Agent what to do and tracks progress.

## Directory Structure

- `.argus/` — Project-level Argus configuration
- `.agents/skills/` — Project-level skill files written by Argus for Codex and OpenCode discovery
- `.claude/skills/` — The same project-level skills mirrored for Claude Code; OpenCode also scans this path
- `.opencode/skills/` — Optional OpenCode-native skill path; Argus project install does not write here
- If the same skill name exists in multiple OpenCode-scanned paths, OpenCode keeps the first copy it discovers and ignores later duplicates
- `.claude/`, `.codex/`, `.opencode/` — Agent-specific hook configurations
