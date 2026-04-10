# Argus

Workflow orchestration for AI Agents. Argus sits on top of your AI coding agents — [Claude Code](https://docs.anthropic.com/en/docs/claude-code), [Codex](https://github.com/openai/codex), and [OpenCode](https://github.com/opencode-ai/opencode) — to coordinate multi-step workflows, enforce project invariants, and track pipeline progress across sessions.

## Why Argus?

AI coding agents are powerful, but they struggle with complex engineering tasks that span multiple steps, require persistent state tracking, or demand continuous adherence to project standards. Specifically:

- **No structured guidance** — A release workflow involves linting, testing, security review, docs update, and deployment. Without explicit step-by-step orchestration, agents skip steps or execute them in the wrong order.
- **No persistent state** — Agents are session-centric. They lose track of multi-step progress when conversations restart or team members switch.
- **No continuous guardrails** — Project standards (lint passes, docs up-to-date, configs present) drift silently. Agents don't proactively enforce them unless told to.

Argus solves these by acting as an **orchestration layer** — it does not replace your agent or make decisions for it. Argus injects context (what the agent should do next), tracks state (which step the workflow is on), and monitors project health (via lightweight shell checks). The agent does the actual work: running commands, editing files, and calling tools.

## Supported Agents

| Agent | tick (context injection) | trap (operation gating)\* |
|-------|:---:|:---:|
| **Claude Code** | Supported | Full (Bash, files, MCP) |
| **Codex** | Supported | Bash only |
| **OpenCode** | Supported | Full (Bash, files, MCP) |

\* **Trap is a reserved future capability.** In Phase 1, `argus setup` wires only `tick` hooks; no tool-use hooks are set up yet. The "Full" and "Bash only" columns describe each agent's underlying capability — what Argus will be able to gate once trap policies are implemented. Codex's hook system can only intercept Bash commands, not file edits or MCP tools.

All three agents share the same state on disk: pipeline progress in `.argus/pipelines/`, invariant definitions in `.argus/invariants/`, and freshness data in `.argus/data/`. You can switch agents mid-workflow — the new agent picks up where the previous one left off because the state lives on disk, not in the agent's memory.

## Quick Start

### Prerequisites

- Go 1.26+ (for building from source)
- Git (Argus requires a Git repository)
- At least one supported AI Agent installed (Claude Code, Codex, or OpenCode)

### Install

```bash
# Build from source
go install github.com/nextzhou/argus/cmd/argus@latest

# Or clone and build
git clone https://github.com/nextzhou/argus.git
cd argus
make build
# Binary is at ./bin/argus — move it to your PATH
```

### Initialize a Project

```bash
cd your-project

# Set up Argus in the project
argus setup

# Run the built-in initialization workflow
argus workflow start argus-project-init

# Verify everything is set up
argus doctor
```

`argus setup` creates the `.argus/` directory, releases built-in workflows/invariants/skills, and configures hooks for all supported agents. It establishes the project-level Argus scaffold and is idempotent — safe to run multiple times.

The `argus-project-init` workflow walks the agent through generating project rules, setting up git hooks, configuring `.gitignore`, and creating project-specific workflows and example invariants.

## Core Concepts

```
Workflow (blueprint)  ──starts──>  Pipeline (running instance)
    defines jobs                     tracks progress, per-job output
                                     supports cross-session resume

Invariant (guardrail)
    shell checks run automatically (frequency controlled by `auto` field)
    failures suggest remediation workflow
```

### Workflow

A **workflow** defines a sequence of jobs that guide the agent through a complex task. Each job contains a prompt (natural-language instructions) and/or a skill reference.

```yaml
# .argus/workflows/release.yaml
version: v0.1.0
id: release
description: "Standard release process"
jobs:
  - id: run_tests
    prompt: "Run `go test ./...` and report the results"
  - id: security_review
    prompt: "Review changes for security issues"
  - id: tag_release
    prompt: "Create a git tag {{ .env.VERSION }} and push it"
```

Argus does not execute these commands — it injects them as context to the agent, which uses its own tools to carry them out.

### Invariant

An **invariant** defines a condition that should always be true about your project. Checks are pure shell commands (no LLM involvement), executed by Argus during `tick` when no pipeline is actively being surfaced. How often each invariant is checked depends on its `auto` field: `always` (every tick on that path), `session_start` (once per session), or `never` (manual only).

```yaml
# .argus/invariants/lint-clean.yaml
version: v0.1.0
id: lint-clean
auto: session_start
check:
  - shell: "find .argus/data/lint-passed -mtime -1 | grep -q ."
prompt: "Lint check may be stale. Please verify the code still passes lint."
workflow: run-lint
```

When a check fails, Argus notifies the agent and suggests a remediation workflow.

### Pipeline

A **pipeline** is a running instance of a workflow. It tracks which job is current, stores per-job messages, and supports resuming across sessions. Only one pipeline can be active at a time.

```
Pipeline: release-20240405T103000Z
Workflow: release
Progress: 2/5
  1. [done] run_tests - All tests passed
  2. [>>]   security_review
  3. [ ]    build
  4. [ ]    tag_release
  5. [ ]    deploy
```

### Skills and Rules

A **skill** is a SKILL.md file that provides specialized instructions to agents — like a reference card for a specific operation. Workflow jobs can reference skills by name.

**Rules** are project-specific coding standards generated during `argus-project-init`, stored in `.argus/rules/`, and used to produce agent-native rule files (`CLAUDE.md`, `AGENTS.md`).

> To write your own workflows, invariants, or skills, see the [Reference Guide](docs/reference.md).

## How It Works

Argus integrates via each agent's hook system. It is purely reactive — it only runs when triggered by a hook or CLI call, never in the background.

```
User input → Agent Hook → argus tick → Check state + inject context → Agent proceeds
Future: Agent tool call → Agent Hook → argus trap → Gate operation → Allow / Deny
```

Each agent's hook configuration is different (Claude Code uses `.claude/settings.json`, Codex uses `.codex/hooks.json`, OpenCode uses a TypeScript plugin), but they all forward events to the same `argus` binary. The hook layer is intentionally thin — it only passes through the agent's raw JSON context via stdin. All logic lives in the Go binary.

### tick — Context Injection

Every time the user submits a message, the agent's hook calls `argus tick`. Argus follows a single precedence order and injects exactly one kind of context into the agent's conversation.

- **State changed** (new job or new pipeline): full context with prompt, skill, and guidance.
- **State unchanged** (ongoing conversation): minimal one-line reminder to prevent the agent from forgetting the active workflow.
- **No active pipeline + first failing invariant**: invariant-only remediation guidance.
- **No active pipeline + invariants pass + workflows available**: lists available workflows the agent can start.
- **No active pipeline + invariants pass + no workflows available**: injects nothing.

### trap — Operation Gating (Future)

Before the agent executes a tool (Bash command, file edit, etc.), `argus trap` can allow or deny the operation based on pipeline state.

> **Note:** In the current version, `trap` remains a reserved internal command. Phase 1 setup does not wire tool-use hooks yet; only `tick` is set up.

### job-done — Progress Advancement

When the agent completes a job, it calls `argus job-done` to advance the pipeline. This returns the next job's instructions immediately, so the agent can continue without waiting for user input.

```bash
argus job-done --message "All tests passed"      # complete current job
argus job-done --fail --message "3 tests failing" # mark as failed
argus job-done --end-pipeline --message "Done"    # end pipeline early
```

### End-to-End Example

Here's what happens during a typical workflow session:

```
1. User opens their agent and types: "Let's start the release process"

2. Agent hook fires `argus tick`
   → Argus: no active pipeline. Output lists available workflows.
   → Agent sees "release" workflow and runs: argus workflow start release

3. `workflow start` returns the first job:
   → Job: run_tests — "Run `go test ./...` and report the results"

4. Agent executes `go test ./...`, reviews results

5. Agent calls: argus job-done --message "All 42 tests passed"
   → Argus advances pipeline, returns next job:
   → Job: security_review — "Review changes for security issues"

6. Agent performs review, then calls job-done again
   → ...and so on until the pipeline completes

7. Meanwhile, on every user message, `argus tick` checks for an active pipeline first.
   If none is running, it evaluates auto invariants and surfaces the first failure as exclusive guidance.
```

## Project Structure

```
.argus/
  workflows/          # Workflow YAML definitions (git-tracked)
    _shared.yaml      # Shared job definitions
  invariants/         # Invariant YAML definitions (git-tracked)
  rules/              # Generated project rules (git-tracked)
  pipelines/          # Pipeline instance data (local-only, gitignored)
  logs/               # Hook execution logs (local-only, gitignored)
  data/               # Freshness timestamps, etc. (git-tracked)
  tmp/                # Temporary data (local-only, gitignored)

.agents/skills/argus-*/SKILL.md   # Skills for Codex and OpenCode
.claude/skills/argus-*/SKILL.md   # Skills mirrored for Claude Code
.claude/settings.json             # Claude Code hook config (git-tracked)
.codex/hooks.json                 # Codex hook config (git-tracked)
.opencode/plugins/argus.ts        # OpenCode plugin (git-tracked)
```

**Git-tracked** (team-shared): workflows, invariants, rules, data, skills, agent hook configs.
**Local-only** (per-machine): pipelines, logs, tmp.

## Workspace

For developers working across multiple projects, Argus supports workspace-level management:

```bash
# Register a workspace (can register multiple)
argus setup --workspace ~/work/company
argus setup --workspace ~/work/client-x

# Remove a workspace
argus teardown --workspace ~/work/client-x
```

A workspace is a registered parent directory. The `--workspace` flag does four things:

1. **Sets up global hooks** — writes `argus tick` into each agent's **user-level** (global) hook configuration, so it fires for projects inside registered workspaces, not just projects with project-level Argus set up.
2. **Sets up global skills** — releases the current managed global built-in skills (`argus-configure-invariant`, `argus-configure-workflow`, `argus-doctor`, `argus-setup`, `argus-intro`, and `argus-teardown`) to each agent's global skill directory.
3. **Sets up global bootstrap artifacts** — releases managed global-scope invariants and related artifacts under `~/.config/argus/` so workspace ticks can guide setup before a repository has project-level Argus.
4. **Records the workspace path** in `~/.config/argus/config.yaml`.

Re-running `argus setup --workspace <path>` for an already registered workspace refreshes those global hooks, skills, and bootstrap artifacts to match the current Argus binary.

When the global hook fires inside a workspace directory, Argus checks whether the current project has a `.argus/` directory. If not, it guides the agent to either set up Argus, explain what Argus is, or ignore the reminder and continue. The workspace itself doesn't manage projects or aggregate state — it's purely a discovery and onboarding mechanism.

Multiple workspaces can be registered. Remove one with `argus teardown --workspace <path>`.

## Documentation

- **[Reference Guide](docs/reference.md)** — Workflow/invariant/skill authoring, CLI commands, built-in content
- **[Technical Docs](docs/)** — Internal architecture and design documents
- **[AGENTS.md](AGENTS.md)** — Contributing guidelines and development conventions

## Community

- **[Contributing Guide](CONTRIBUTING.md)** — Development workflow, testing expectations, and pull request guidance
- **[Code of Conduct](CODE_OF_CONDUCT.md)** — Community participation expectations and enforcement policy
- **[Security Policy](SECURITY.md)** — Supported branches and how to report security issues
- **[Support](SUPPORT.md)** — Where to ask usage and troubleshooting questions

## Development

```bash
make build    # Build binary to ./bin/argus
make test     # Run all tests
make lint     # Run golangci-lint + biome
```

- **Language**: Go 1.26+
- **Key dependencies**: [cobra](https://github.com/spf13/cobra) (CLI), [gojq](https://github.com/itchyny/gojq) (JSON toolbox), [yq](https://github.com/mikefarah/yq) (YAML toolbox)
- **Commit format**: [Conventional Commits](https://www.conventionalcommits.org/) — `type(scope): description`

## License

[MIT](LICENSE)
