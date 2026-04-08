---
name: argus-generate-rules
description: Guide rule generation for Agent-native rules systems
version: 0.1.0
---

# argus-generate-rules

Generate project rules for Agent-native rules systems.

## When to Use

This skill is loaded during the `generate_rules` job of the `argus-init` workflow.

## What to Generate

Analyze the project and create rules covering:
1. **Technical architecture** — Languages, frameworks, directory structure
2. **Coding standards** — Style, patterns, testing conventions
3. **Project domain** — Business logic, terminology, constraints

## Output Targets

- Claude Code: `CLAUDE.md`
- OpenCode: `AGENTS.md`
- Rules directory: `.argus/rules/`

## Guidelines

- Keep rules concise and actionable
- Reference existing configs (linter, formatter) rather than duplicating
- Focus on what cannot be derived from code alone
