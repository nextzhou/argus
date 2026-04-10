# Agent Hook Integration (Technical Hooks)

This document explains how Argus integrates with supported AI agents through their hook or plugin systems. The current goal is to provide one orchestration entry point for state-aware context injection, while keeping a reserved internal surface for future operation gating.

---

## 9.1 Unified Integration Strategy

### The Problem

The three primary agents use materially different integration models:

- Claude Code and Codex rely on shell commands plus JSON hook payloads
- OpenCode uses a JavaScript or TypeScript plugin system

This heterogeneity makes it easy for logic to drift if orchestration behavior is implemented separately per agent.

### Core Approach

Argus uses a forwarding model. Each configured agent-specific hook acts only as a wrapper that forwards event context to `argus tick`. `argus trap` remains a reserved internal command, but `argus setup` does not wire tool-use hooks in Phase 1. All business logic and state evaluation live in the Go CLI.

```text
Agent Hook Event -> Agent-specific wrapper -> argus CLI command -> Go business logic
```

This keeps orchestration logic as a single source of truth. Regardless of which agent the user is running, progress and guardrails should be interpreted the same way.

### Design Principle: Keep Wrappers as Thin as Possible

**Core rule**: Hook and plugin wrappers should be as thin as possible. They collect raw agent-specific input and pass it to `argus`. Business logic belongs inside the `argus` binary.

**Why**:

- **Easy upgrades**: Upgrading Argus should usually mean replacing the binary, not rewriting per-agent wrappers.
- **Consistency**: Decision logic stays in one place.
- **Maintainability**: Shell wrappers and TypeScript plugins already differ enough. The less logic they contain, the fewer ways they can diverge.

**Practical implications**:

- Wrappers should not contain orchestration logic.
- Minimal agent-specific output adaptation is allowed when the host requires it, for example OpenCode appending a text part or a future `trap` rollout formatting allow/deny responses.
- The only wrapper-side “smart” behavior should be collecting agent-native context that Argus cannot infer by itself, such as OpenCode fetching `parentID` through its SDK.

### Sub-Agent Suppression

Some agents spawn sub-agents or child sessions. Those children can also trigger hooks. If Argus treats them like the primary session, child agents may receive unrelated pipeline context or mutate pipeline state incorrectly.

Argus centralizes the detection decision. Wrappers only pass through enough information for Argus to identify child sessions and skip injection.

| Agent | Wrapper passes through | Detection in Argus | Behavior when child session is detected |
|------|-------------------------|--------------------|-----------------------------------------|
| Claude Code | Original stdin JSON including `agent_id` | Presence of `agent_id` | Exit 0 with no output |
| OpenCode | Plugin queries `session.parentID` and serializes it | Presence of `parentID` | Exit 0 with no output |
| Codex | Original stdin JSON | Currently unavailable | Normal injection; known Phase 1 limitation |

When Codex eventually exposes an agent identifier (tracked in upstream issue `#16226`), support can be added inside Argus without changing the wrapper model.

### Input Normalization: Pipe Passthrough

Different agents emit different JSON shapes. Argus uses pipe passthrough: the original JSON is forwarded over stdin and parsed inside Argus according to `--agent`.

Rejected alternative:

- **Argument normalization in the wrapper**: the wrapper could extract fields and pass them as many CLI flags instead of forwarding the original JSON payload. This was rejected because it makes each wrapper more complex and spreads parsing logic across agent-specific code. Passthrough keeps normalization centralized in Go.

### Output Normalization: One Text Format for `tick`

`argus tick` emits human-readable text and does not customize the payload shape per agent. Each wrapper adapts that text to the hosting agent:

- **Claude Code / Codex**: the text becomes `additionalContext`
- **OpenCode**: the text is appended as a message part

The primary role of `--agent` is therefore on the **input side**, not the output side.

---

## 9.2 `tick` Implementation

`tick` is the collaborative scheduling point. It is triggered passively whenever the user sends input to an agent.

### Bootstrap and Scope Discovery

`tick --global` is the entry point used by global hooks. Unlike a project-local hook, it must first determine which scope applies:

1. **Scope detection**: use current working directory and registered workspaces to resolve either **project scope** (`.argus/`) or **global scope** (`~/.config/argus/`).
2. **Shared orchestration semantics**: once scope is resolved, the orchestration engine is the same across scopes. `--global` identifies the source of the hook call, not a separate “discovery-only” mode.
3. **Arbitration**: project scope wins when both project scope and workspace scope are applicable.
4. **Fail open**: if the environment matches no known scope, Argus exits successfully and injects nothing.

### Claude Code and Codex

Claude Code and Codex both support a `UserPromptSubmit` event.

- **Trigger point**: after the user submits a message, before the model processes it
- **Core capability**: inject additional plain-text context

#### Example Claude Code Configuration

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "argus tick --agent claude-code",
            "timeout": 10,
            "statusMessage": "Argus"
          }
        ]
      }
    ]
  }
}
```

#### Example Codex Configuration

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "argus tick --agent codex",
            "timeout": 10,
            "statusMessage": "Argus"
          }
        ]
      }
    ]
  }
}
```

#### Example Input and Output

**Input (stdin JSON)**:

```json
{
  "session_id": "abc-123",
  "cwd": "/project",
  "hook_event_name": "UserPromptSubmit",
  "prompt": "Run the test suite"
}
```

**Output (stdout)**:

`tick` emits plain text. Claude Code will wrap that stdout as model context.

**Compatibility constraint**: although `tick` uses plain text, its first non-whitespace character must **not** be `[` or `{`. Current Codex CLI behavior (verified against `codex-cli 0.118.0` on April 9, 2026) treats those prefixes as candidate JSON. If the output is not valid JSON, Codex reports `hook returned invalid user prompt submit JSON output`. To keep one shared output contract across agents, Argus uses a text prefix such as `Argus:` rather than a JSON-like or bracket-style prefix.

Example full-context output:

```text
Argus: Pipeline: release-20240405T103000Z | Workflow: release | Progress: 2/5

Current Job: run_tests
Skill: argus-run-tests

Run all tests and only continue if they pass.

When done: argus job-done [--message "summary"]
To snooze: argus workflow snooze --session abc-123
To cancel: argus workflow cancel
```

### OpenCode

OpenCode exposes stronger hooks through `chat.message`, plus `experimental.chat.messages.transform` when a plugin needs to insert a synthetic context part safely.

- **Trigger point**: when a new user message arrives
- **Core capability**: modify message contents directly or append message parts

#### Example Implementation

```typescript
import type { Plugin } from "@opencode-ai/plugin"

export const ArgusPlugin: Plugin = async ({ $, client, directory }) => {
  const pendingInjections = new Map<string, string>()

  return {
    "chat.message": async (input) => {
      try {
        const session = await client.session.get({ path: { id: input.sessionID } })
        const payload = JSON.stringify({
          sessionID: input.sessionID,
          parentID: session.data?.parentID,
          cwd: directory,
        })
        const result = await $`echo ${payload} | argus tick --agent opencode`
          .cwd(directory)
          .quiet()
          .nothrow()
        const text = result.text().trim()
        if (result.exitCode === 0 && text) {
          pendingInjections.set(input.sessionID, text)
        }
      } catch {
        // Fail open and keep the session usable
      }
    },
    "experimental.chat.messages.transform": async (_input, output) => {
      const lastUserMessage = output.messages.findLast((message) => message.info.role === "user")
      const sessionID = lastUserMessage?.info.sessionID ?? ""
      const injectedText = pendingInjections.get(sessionID)
      if (!lastUserMessage || !sessionID || !injectedText) {
        return
      }
      pendingInjections.delete(sessionID)

      const textPartIndex = lastUserMessage.parts.findIndex(
        (part) => part.type === "text" && typeof part.text === "string",
      )
      if (textPartIndex === -1) {
        return
      }

      lastUserMessage.parts.splice(textPartIndex, 0, {
        id: `synthetic_hook_${Date.now()}`,
        messageID: lastUserMessage.info.id,
        sessionID,
        type: "text",
        text: injectedText,
        synthetic: true,
      } as any)
    },
  }
}
```

#### OpenCode-Specific Future Enhancements

Future versions may benefit from:

- `experimental.chat.system.transform` for persistent state injection into the system prompt
- `experimental.session.compacting` to preserve state across context compaction

Phase 1 uses `chat.message` to compute the pending Argus guidance and `experimental.chat.messages.transform` to inject that guidance as a synthetic text part with the metadata OpenCode expects.

#### Maintenance Notes

When debugging agent hook integrations, prefer this order of evidence:

1. The host agent's runtime logs
2. The currently installed SDK or type definitions on the machine
3. The actual installed hook or plugin artifact
4. Repository templates and official docs

This order matters because host integration APIs can change before older templates or prose examples are updated. After changing a hook template, remember that user machines will continue running the previously installed artifact until `argus setup` or `argus setup --workspace` refreshes it.

For OpenCode specifically, synthetic context insertion should happen in `experimental.chat.messages.transform`, not by treating `chat.message` as a generic append-only surface. The runtime validates inserted message parts more strictly than the minimal `chat.message` output type suggests, so synthetic parts should be created in the transform stage with the message metadata that OpenCode expects.

---

## 9.3 `trap` Implementation

`trap` is the reserved operation-gating entry point. It stays in the CLI as a hidden internal command, but `argus setup` and `argus setup --workspace` do not wire tool-use hooks in Phase 1.

**Phase 1 note**: gating logic is not enforced yet. The command defaults to allow. Argus deliberately avoids wiring a no-op gate into agent configs, because that would add hook surface area without changing behavior.

### Allow Output in Phase 1

Even when `trap` is effectively pass-through, allow output still has to respect agent-specific expectations:

- **Claude Code / OpenCode**: may return a stable allow JSON structure so the shape matches future deny responses
- **Codex**: must keep stdout empty on allow. Current Codex CLI behavior rejects `permissionDecision: "allow"` and `permissionDecision: "ask"` for `PreToolUse`, then fails open

Allow output used by Claude Code and OpenCode:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow"
  }
}
```

### Responsibility Boundary

Pre-commit quality checks such as lint or test enforcement belong in Git hooks, not in `trap`. `trap` is meant for pipeline-state-aware operation gating, not as a generic code-quality gate.

### Why Keep the Command

Keeping `trap` as a stable internal command preserves the CLI surface and its output contract while the actual gating policy is still empty. That lets future versions add real tool gating without redesigning the command boundary itself.

### Legacy Cleanup

Older Argus setups may already contain `PreToolUse` or equivalent `argus trap` entries. Re-running `argus setup` and `argus teardown` continues to recognize and remove those legacy entries so repositories converge toward the new tick-only configuration.

---

## 9.4 Setup and Teardown

`argus setup` injects context-injection (`tick`) hook configuration into each agent-specific location.

### Write Locations

- **Claude Code**: `.claude/settings.json`. This file should be committed to the repository so the team shares the configuration. The setup flow merges configuration and preserves existing non-Argus hooks.
- **Codex**: `.codex/hooks.json`, plus ensure `codex_hooks = true` in `~/.codex/config.toml`
- **OpenCode**: `.opencode/plugins/argus.ts`

### Team Collaboration Compatibility

Wrappers locate the Argus binary through `PATH`, preferring `command -v argus` and covering common Go installation locations such as `GOPATH/bin`.

If the binary is missing:

- **Shell wrappers (Claude Code / Codex)**: fail open with exit code 0 and print an installation hint
- **TypeScript plugin (OpenCode)**: check the binary path and push an installation hint part when missing

The exact CLI installation hint string is not a protocol contract in Phase 1. A generic message such as `Please install Argus CLI. See project README for instructions.` is sufficient for now.

### Teardown Behavior

`argus teardown` performs the inverse operation.

For Codex, Argus intentionally **does not** disable the global `codex_hooks` toggle during teardown, because that could break unrelated custom hooks managed by the user.

### Identifying Argus-Owned Hook Entries

Setup and teardown must merge or remove Argus-owned entries safely.

- **Claude Code / Codex**: identify entries by matching hook command content. The command field should be checked for `argus tick` or legacy `argus trap`, using substring matching rather than exact-match equality because users may install `argus` via absolute paths.
- **OpenCode**: identify by filename. Argus owns `.opencode/plugins/argus.ts`.

---

## 9.5 Hook Logging

Argus maintains a unified log file named `hook.log`.

- project scope: `.argus/logs/hook.log`
- global scope: `~/.config/argus/logs/hook.log`

Logging does not depend on the Argus binary itself. Hook wrappers may write log lines directly with native shell or plugin code.

Log format:

```text
{COMPACT_UTC} [{COMMAND}] {OK|ERROR} {DETAILS}
```

Where `{COMPACT_UTC}` uses the shared compact UTC format, for example `20240115T103000Z`.

### What Counts as `ERROR`

`ERROR` is reserved for wrapper or execution-layer failures:

- Argus binary not found on `PATH`
- command timeout
- JSON parsing failures on hook input
- log write failure

The following still count as `OK` even if the business result is negative:

- Argus returned an error envelope such as invariant failure or “no active pipeline”
- a global hook skipped correctly because no applicable scope was found
- `tick` or `trap` completed normally regardless of business result

This keeps `doctor` focused on integration-layer problems instead of conflating them with normal business failures.

### Pre-Install Logging Policy

When `.argus/logs/` does not exist yet, for example when a workspace global hook fires inside a project without project-level Argus set up, logging falls back to `~/.config/argus/logs/hook.log`. This avoids creating project-level `.argus/` only for logging and preserves the non-intrusive workspace model.

Argus prefers inline hook logic over extra wrapper scripts to reduce file lookup overhead and keep integration behavior stable.

---

## 9.6 Capability Matrix

Current setup flows wire only `tick`. The interception-oriented rows below describe host-agent capabilities that matter to a future `trap` rollout; they are not active Argus integrations in Phase 1.

| Capability Area | Feature | Claude Code | Codex | OpenCode |
| :--- | :--- | :---: | :---: | :---: |
| **Basic triggers** | `tick` (context injection) | Yes | Yes | Excellent |
| | Reserved `trap` rollout | Supported | Bash only | Supported |
| **Interception scope** | Bash command interception | Yes | Yes | Yes |
| | File read/write interception | Yes | No | Yes |
| | MCP tool interception | Yes | No | Yes |
| **Decision depth** | Deny / Allow / Ask | Yes | Deny only | Yes |
| | Fine-grained command matching | Yes | Weak | Implemented in code |
| | Modify tool arguments | No | No | Yes |
| | Modify tool output | No | No | Yes |
| | Modify tool definitions | No | No | Yes |
| **Advanced extension** | Modify LLM inference parameters | No | No | Yes |
| | Inject environment variables | No | No | Yes |
| | Define custom native tools | Via MCP | No | Yes |
| **Context management** | Modify system prompt | No | No | Yes |
| | Modify full message history | No | No | Yes |
| | Customize context compaction | No | No | Yes |
| | Automatic continue on stop events | Yes | Yes | Yes |

---

## 9.7 Limitations and Workarounds

### Interception Limits

If Argus later enables `trap`, Codex `PreToolUse` still cannot intercept file edits. A workflow that requires hard protection over certain files would therefore need additional enforcement beyond Codex tool hooks alone.

Possible workarounds:

- detect violations after the fact in `PostToolUse`
- emphasize constraints in `tick`
- wait for broader Codex hook coverage upstream

### Runtime Dependencies

OpenCode plugins require a JS or TS runtime. OpenCode ships with Bun, so Argus can generate a TypeScript wrapper that delegates to the Go CLI. This adds a layer, but preserves capability.

### Output Handling

Each agent interprets stdout and exit codes differently. Argus keeps:

- **`tick`**: plain text across all agents
- **deny responses in a future `trap` rollout**: JSON with fields such as `permissionDecision`
- **allow responses in a future `trap` rollout**: agent-specific, because Codex requires empty stdout while Claude Code and OpenCode can accept allow JSON

---

## 9.8 Hook Timeouts

Hooks must have explicit timeouts to prevent the agent UI from appearing hung.

| Agent | Default timeout | Hard limit | Configurable | Timeout behavior |
| :--- | :---: | :---: | :---: | :--- |
| Claude Code | 600s | None | Yes (`timeout`) | Process is terminated and the hook fails |
| Codex | 600s | None | Yes (`timeout`) | Process is terminated, agent continues |
| OpenCode | None | None | N/A | Waits until the function returns |

For Argus, `tick` should stay fast because it only performs state inspection and lightweight checks. Installers should write explicit timeout settings into generated configuration for defensive robustness.
