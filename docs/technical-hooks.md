# Agent Hook 集成方案 (Technical Hooks)

本文档详细说明了 Argus 如何通过各 AI Agent 的 Hook 系统实现深度集成。集成的核心目标是提供统一的编排入口，支持状态感知与操作门控。

---

## 9.1 统一集成策略 (Unified Strategy)

### 面临的问题
目前主流的三个 Agent 拥有完全不同的 Hook 机制。Claude Code 与 Codex 采用基于 Shell 命令和 JSON 的系统。OpenCode 则使用基于 JS/TS 模块的插件体系。这种异构性给跨 Agent 的逻辑复用带来了巨大挑战。

### 核心方案
Argus 采用"调用入口"转发策略。各 Agent 的 Hook 仅作为转发层，将事件上下文传递给 `argus tick` 或 `argus trap` 命令。所有的业务逻辑和状态检查均在 Go 编写的 CLI 程序中实现。

```text
Agent Hook Event --> Agent 特有入口 --> argus CLI 命令 --> Go 业务逻辑
```

这种架构确保了编排逻辑的单一事实来源。无论用户使用哪种 Agent，看到的进度和受到的约束都是一致的。

### 设计指导思想：Wrapper 最薄化

**核心原则**：Hook/Plugin wrapper 层应尽可能薄——仅负责收集 Agent 提供的原始信息并透传给 argus，所有业务逻辑收敛到 argus 二进制内部。

**驱动因素**：
- **升级便利**：Wrapper 不含业务逻辑，升级 argus 时只需替换二进制文件，无需修改各 Agent 的 hook/plugin 配置。
- **一致性**：所有 Agent 的判断逻辑在同一处维护，避免多处实现产生分歧。
- **可维护性**：各 Agent 的 Wrapper 格式差异大（Shell vs TypeScript），逻辑越少越不容易出错。

**具体表现**：
- Wrapper 不包含业务逻辑；仅允许最小限度的 Agent 特定输出适配（如 trap 返回值解析）
- Wrapper 唯一的"智能"行为是收集 Agent 特有的上下文信息（如 OpenCode 需要通过 SDK 查询 `parentID`），然后序列化到 stdin JSON 中交给 argus 处理

### 子 Agent 屏蔽

各 Agent 在派生子 agent（如 Claude Code 的 task delegation、OpenCode 的子 session）时，子 agent 也会触发 hook。如果不加区分，子 agent 会收到无关的 pipeline context 注入，甚至可能误操作 pipeline 状态。

**设计方案**：检测逻辑全部收敛到 argus 内部。各 Wrapper 负责将子 agent 相关信息透传给 argus，argus 统一判断并跳过注入。

| Agent | Wrapper 透传内容 | argus 检测方式 | 检测到子 agent 时的行为 |
|-------|-----------------|---------------|----------------------|
| Claude Code | stdin JSON 原样透传（已含 `agent_id` 字段） | `agent_id` 字段存在 → 子 agent | exit 0，无输出 |
| OpenCode | plugin 查询 `session.parentID`，写入 stdin JSON | `parentID` 字段存在 → 子 agent | exit 0，无输出 |
| Codex | stdin JSON 原样透传 | 暂无法检测（上游 Issue #16226 跟踪中） | 正常注入（Phase 1 已知限制） |

**Codex 后续**：当 Codex 实现 `agent_id` 字段后（跟踪 [Issue #16226](https://github.com/openai/codex/issues/16226)），argus 侧添加对应检测逻辑即可，无需修改 Wrapper。

### 输入标准化：管道透传 (Pipe Passthrough)
不同 Agent 提供的上下文 JSON 结构存在差异。Argus 采用管道透传方案处理输入。Agent 的原始 JSON 通过标准输入 (stdin) 完整传给 Argus。Argus 内部根据 `--agent` 参数识别来源并解析对应的结构。这样做避免了在 Hook 入口处进行复杂的参数提取。

排除的方案：
方案 B（参数标准化）：hook 入口负责提取关键字段，通过命令行参数传给 argus（如 `argus trap --agent claude-code --tool Bash --command "git push"`）。排除理由：增加 hook 入口复杂度，每个 Agent 需要不同的提取逻辑；方案 A 更简单，归一化逻辑统一在 Go 侧。

### 输出标准化：统一文本
`argus tick` 的输出统一为可读文本（Markdown-like），不按 `--agent` 区分输出格式。各 Agent 的 Hook/Plugin 层负责将文本注入到对应的上下文机制中：

- **Claude Code / Codex**：文本作为 `additionalContext` 字段值。
- **OpenCode**：文本作为 `Part` 对象的 `text` 字段 push 到 `output.parts`。

`--agent` 参数的核心作用在**输入侧**——决定如何解析各 Agent 传入的不同格式的 stdin JSON。Agent 具备较强的自适应能力，不需要为不同 Agent 定制输出格式。

---

## 9.2 tick 实现机制 (tick Implementation)

tick 是 Argus 的协作调度点。它在用户每次输入时被动触发。Argus 借此机会检查进度并注入必要的引导上下文。

### Claude Code 与 Codex
这两个 Agent 均支持 `UserPromptSubmit` 事件。

- **触发时机**：用户提交消息后，Agent 处理之前。
- **核心能力**：通过 `additionalContext` 字段向模型注入纯文本。

#### Claude Code 配置示例
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

#### Codex 配置示例
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

#### 输入输出示例 (Claude Code)
**输入 (stdin JSON)**:
```json
{
  "session_id": "abc-123",
  "cwd": "/project",
  "hook_event_name": "UserPromptSubmit",
  "prompt": "帮我运行测试"
}
```

**输出 (stdout)**:
argus tick 输出统一文本格式。Claude Code 的 Hook 系统会自动将 stdout 文本封装为 `additionalContext` 注入给模型。

**兼容性约束（重点）**：tick 虽然保持纯文本输出，但其首个非空白字符**不得**为 `[` 或 `{`。当前 Codex CLI（验证于 `codex-cli 0.118.0`, 2026-04-09）会把这两种前缀视为 JSON 候选；若文本实际上不是合法 JSON，就会报 `hook returned invalid user prompt submit JSON output`。为了在所有 Agent 上保持统一输出，Argus 统一使用 `Argus:` 这样的纯文本前缀，而不是旧的方括号前缀。

```text
Argus: 当前正在执行 Job: run_tests
Prompt: 运行所有测试，确保通过后再继续

完成后请调用：argus job-done --message "执行结果摘要"
```

### OpenCode
OpenCode 通过 `chat.message` 钩子提供更强的控制力。

- **触发时机**：新用户消息到达时。
- **核心能力**：可以直接修改用户消息内容，或者追加消息部分 (Message Parts)。

#### 实现示例
```typescript
"chat.message": async (input, output) => {
  try {
    const session = await client.session.get();
    const payload = JSON.stringify({ sessionID: input.sessionID, parentID: session.parentID });
    const result = await $`echo ${payload} | argus tick --agent opencode`.quiet().nothrow();
    if (result.exitCode === 0 && result.text().trim()) {
      output.parts.push({
        type: "text",
        text: result.text(),
      } as any);
    }
  } catch {
    // 失败时保持开启，不中断会话
  }
}
```

#### OpenCode 额外增强
**未来扩展方向（非 Phase 1）**：OpenCode 额外支持 `experimental.chat.system.transform` 和 `experimental.session.compacting`。前者可将状态持久注入系统提示词，后者确保上下文压缩后状态得以保留。Phase 1 仅需实现 `chat.message` 和 `tool.execute.before` 两个核心 Hook。

---

## 9.3 trap 实现机制 (trap Implementation)

trap 是 Argus 的操作门控。它根据工作流规则拦截特定的工具调用。

**注意：在 Phase 1 阶段，trap 逻辑暂未在内部实现。命令目前默认返回退出码 0，即直接放行。但为了保持前瞻性，安装过程仍会配置对应的入口。**

**Phase 1 放行输出**：虽然 Phase 1 不实现拦截逻辑，但 trap 命令的放行输出不能再假设所有 Agent 都接受同一种格式：

- **Claude Code / OpenCode**：仍返回稳定的 allow JSON，便于与未来 deny 输出保持同构。
- **Codex**：放行时必须保持空 stdout。当前 Codex CLI（验证于 `codex-cli 0.118.0`, 2026-04-09）的 `PreToolUse` 只接受阻断输出；`permissionDecision: "allow"`、`permissionDecision: "ask"` 会直接报 unsupported 并 fail open。

Claude Code / OpenCode 的 allow 输出如下：

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow"
  }
}
```

**职责边界**：Git commit 前的 lint/test 检查应由 Git pre-commit hook 承担，不属于 trap 的职责。trap 的定位是基于 Pipeline 状态的操作门控，而非通用的代码质量守卫。

### Claude Code
通过 `PreToolUse` 事件实现。它能拦截所有类型的工具，包括 Bash 命令、文件编辑和写入。

#### 配置示例
```json
{
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "argus trap --agent claude-code",
            "timeout": 10,
            "statusMessage": "Argus"
          }
        ]
      }
    ]
  }
}
```

Claude Code 支持通过 `permissionDecision` 字段返回 `deny` (拒绝)、`allow` (放行) 或 `ask` (询问用户)。

#### 阻断输出示例
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "Argus: 当前阶段不允许执行此操作"
  }
}
```

### Codex
同样支持 `PreToolUse`，但目前仅限于拦截 Bash 工具。它无法有效阻止文件编辑。这被视为一种有用的警示，不应被当作严密的执行边界。

Codex trap 的额外限制：
- 无 `if` 字段，命令过滤需要 argus 内部处理
- Agent 可通过写脚本文件再执行的方式绕过 Bash 拦截
- 放行时不接受 `permissionDecision: "allow"`；Argus 必须输出空 stdout，仅在阻断时返回 `deny` / `block`

#### 配置示例
```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "argus trap --agent codex",
            "timeout": 10,
            "statusMessage": "Argus"
          }
        ]
      }
    ]
  }
}
```

### OpenCode
OpenCode 提供了两层拦截机制。`tool.execute.before` 允许在执行前修改工具参数。`permission.ask` 则能更优雅地控制权限决策。

#### 实现示例
```typescript
"tool.execute.before": async (input, output) => {
  try {
    const payload = JSON.stringify({ tool: input.tool, args: output.args });
    const result = await $`echo ${payload} | argus trap --agent opencode`.quiet().nothrow();
    if (result.exitCode !== 0) {
      // argus 命令异常，fail open（放行）
      return;
    }
    const trapData = JSON.parse(result.text());
    if (trapData.hookSpecificOutput?.permissionDecision === "deny") {
      throw new Error(trapData.hookSpecificOutput.permissionDecisionReason ?? "Argus: 操作被拒绝");
    }
  } catch (e: any) {
    if (typeof e?.message === "string" && e.message.startsWith("Argus:")) throw e;
    // JSON 解析失败或其它异常，fail open（放行）
  }
}
```

---

## 9.4 安装与卸载逻辑 (Install / Uninstall)

`argus install` 命令负责在不同 Agent 的配置路径中注入 Hook。

### 写入位置
- **Claude Code**：写入 `.claude/settings.json`。该文件建议提交到仓库以实现团队共享。安装程序会合并配置并保留现有的非 Argus 钩子。
- **Codex**：创建 `.codex/hooks.json`。同时确保用户级配置 `~/.codex/config.toml` 中开启了 `codex_hooks = true` 标识。卸载时不关闭该标识，避免影响用户可能存在的其他自定义 Hook。
- **OpenCode**：在 `.opencode/plugins/` 目录下生成 `argus.ts` 插件文件。

### 团队协作兼容性
Hook wrapper 通过 PATH 查找 argus 二进制（优先 `command -v argus`），覆盖 GOPATH/bin 等常见安装路径。如果二进制未找到：
- Shell wrapper（Claude Code / Codex）：静默放行（exit 0），并在输出中提示安装。
- TS Plugin（OpenCode）：检查二进制路径是否存在，不存在时 push 安装提示 Part。

**安装提示内容**：该字符串非规范的一部分，Phase 1 输出通用安装提示即可（如 `Please install Argus CLI. See project README for instructions.`）。待项目发布后替换为具体安装命令。

### 卸载逻辑
`argus uninstall` 执行逆向操作。在 Codex 中，出于安全考虑，`config.toml` 中的功能开关在卸载后会继续保留，以防破坏用户可能存在的其它自定义钩子。

### Hook 条目识别
`install` / `uninstall` 需要在 Agent 配置中合并或移除 argus 条目。识别策略：

- **Claude Code / Codex**：按 hook command 内容匹配。检查 command 字段是否包含 `argus tick` 或 `argus trap`（注意用户可能使用绝对路径，如 `/home/user/go/bin/argus tick`，需做子串匹配而非完全匹配）。
- **OpenCode**：按文件名识别。argus 的 plugin 文件固定为 `.opencode/plugins/argus.ts`，直接按文件名操作。

---

## 9.5 Hook 运行日志 (Hook Logging)

为了方便排查集成问题，Argus 在项目目录下维护统一的日志文件：`.argus/logs/hook.log`。

日志写入并不依赖 Argus 二进制文件。各 Agent 的 Hook 入口会使用原生脚本或代码直接写入该文件。日志采用以下标准格式：

`{COMPACT_UTC} [{COMMAND}] {OK|ERROR} {DETAILS}`

其中 `{COMPACT_UTC}` 使用全局统一的 compact UTC 格式（如 `20240115T103000Z`），参见 [overview §3.3](technical-overview.md)。每次 Hook 调用写入一条日志记录。

**OK/ERROR 判定范围**：`ERROR` 仅限于 **wrapper/执行层面的失败**，包括：
- Argus 二进制缺失（PATH 查找失败）
- 命令执行超时
- JSON 解析失败（如 stdin 输入格式异常）
- 日志文件写入失败

以下情况记为 `OK`（即使业务上存在问题）：
- argus 命令正常执行并返回了 error envelope（如 invariant 检查失败、无活跃 Pipeline）
- 全局 Hook 正常 skip（不在 workspace 内、已有 `.argus/`）
- tick/trap 正常完成（无论业务结果如何）

这确保了 `doctor` 扫描 ERROR 日志时只关注集成层面的异常，不会被业务性的检查失败所干扰。

**Pre-install 日志策略**：当 `.argus/logs/` 目录不存在时（如 Workspace 全局 Hook 在未初始化项目中触发），日志 fallback 写入用户级目录 `~/.config/argus/logs/hook.log`。这保持了 Workspace 非侵入原则——不为记日志而创建项目级 `.argus/` 目录。

所有的 Hook 处理逻辑均采用内联方式，不使用额外的包装脚本。这减少了文件查找开销并提高了系统稳定性。

---

## 9.6 能力矩阵 (Capability Matrix)

下表对比了各 Agent 在 Hook 层面提供的功能支持度。

| 能力类别 | 功能特性 | Claude Code | Codex | OpenCode |
| :--- | :--- | :---: | :---: | :---: |
| **基础触发** | tick (上下文注入) | 支持 | 支持 | **极佳** |
| | trap (操作门控) | **极佳** | 仅限 Bash | **极佳** |
| **拦截范围** | Bash 命令拦截 | 支持 | 支持 | 支持 |
| | 文件读写拦截 | 支持 | 不支持 | 支持 |
| | MCP 工具拦截 | 支持 | 不支持 | 支持 |
| **控制深度** | 权限决策 (Deny/Allow/Ask) | 支持 | 仅限 Deny | 支持 |
| | 命令行精细过滤 (Matcher/if) | 支持 | 弱 | 代码实现 |
| | 修改工具参数 | 不支持 | 不支持 | 支持 |
| | 修改工具输出 | 不支持 | 不支持 | 支持 |
| | 修改工具定义 | 不支持 | 不支持 | 支持 |
| **高级扩展** | 修改 LLM 推理参数 | 不支持 | 不支持 | 支持 |
| | 环境变量注入 | 不支持 | 不支持 | 支持 |
| | 自定义原生工具 | 需要 MCP | 不支持 | 支持 |
| **上下文管理** | 修改系统提示词 | 不支持 | 不支持 | 支持 |
| | 修改完整消息历史 | 不支持 | 不支持 | 支持 |
| | 定制上下文压缩逻辑 | 不支持 | 不支持 | 支持 |
| | 自动继续 (Stop event) | 支持 | 支持 | 支持 |

---

## 9.7 局限性与规避方案 (Limitations)

### 拦截范围限制
Codex 的 `PreToolUse` 无法拦截文件编辑动作。如果工作流需要强制锁定某些文件，在 Codex 上只能通过 `tick` 阶段注入软性约束。Agent 可能会违背这种约束。

Codex 拦截限制的规避方案：
- 通过 PostToolUse 事后检测（但无法阻止）
- 通过 tick 在上下文中强调约束（软约束）
- 等待 Codex 扩展 PreToolUse 支持范围

### 运行时依赖
OpenCode 插件需要 JS/TS 运行时环境。幸运的是 OpenCode 自带了 Bun 运行时。Argus 通过生成 TS 包装层来调用 Go CLI，虽然增加了层级，但保障了功能的完整性。

### 格式处理
各 Agent 对退出码和标准输出的解析逻辑各异。Argus `tick` 命令统一输出**纯文本**格式，各 Agent 的转发层负责将文本注入到各自的上下文机制中（如 Claude Code 通过 `additionalContext` 文本字段注入，OpenCode 通过 Plugin 代码处理）。为兼容 Codex 的当前 JSON 猜测逻辑，tick 文本首个非空白字符不得为 `[` 或 `{`。`trap` 命令的阻断输出统一使用 JSON（包含 `permissionDecision` 等字段），但放行输出需要按 Agent 能力适配：Claude Code / OpenCode 可返回 allow JSON，Codex 放行时必须保持空 stdout。

---

## 9.8 Hook 超时配置 (Hook Timeout)

为了防止 Hook 进程挂起导致 Agent 假死，必须配置合理的超时时间。

| Agent | 默认超时 | 硬性上限 | 可配置性 | 超时行为 |
| :--- | :---: | :---: | :---: | :--- |
| Claude Code | 600s | 无 | 支持 (timeout) | 进程被终止，Hook 失败 |
| Codex | 600s | 无 | 支持 (timeout) | 进程被终止，不阻断 Agent |
| OpenCode | 无 | 无 | N/A | 持续等待函数返回 |

对于 Argus 而言，tick 操作仅涉及快速的状态检查，默认的超时设置完全能够满足需求。安装程序在写入配置时会显式声明超时参数，以增强系统的防御性。
