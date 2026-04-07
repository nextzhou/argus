# 技术概览 (Technical Overview)

本文档是 Argus 的技术规范入口，涵盖了系统的核心定位、设计原则、架构约束及项目组织结构。

---

## 1. 系统定位与核心概念

### 1.1 Argus 定位
Argus 是一个 **AI Agent 工作流编排工具**。它提供一个 Go 编写的 CLI 二进制工具，通过各 AI Agent（Claude Code, Codex, OpenCode）提供的 Hook 系统进行集成，实现对复杂任务流、项目不变量（Invariants）和项目状态的自动化编排与管理。

### 1.2 设计原则 (Architecture Invariants)
Argus 的设计遵循以下 7 个核心原则：

1. **Argus 是编排层，而非 Agent 的替代品**：
   Argus 不直接执行具体的业务逻辑（如运行测试、提交代码）。所有的具体操作都通过注入上下文（Prompt/Skill）交给 Agent 执行。Workflow Job 中不包含 `script` 字段，`tick` 只负责状态检查和上下文注入。

2. **产物即真相 (Artifacts as ground truth)**：
   优先检查实际的文件或产物是否存在，而非依赖布尔标记。系统中没有中心化的 `state.yaml` 或 `initialized: true` 标记。Pipeline 的运行状态存储在独立的数据文件中。

3. **工作流 (Workflow) 与不变量 (Invariant) 互补**：
   Workflow 是命令式的（Imperative），解决"怎么做"（过程保障）；Invariant 是声明式的（Declarative），解决"应该是什么样"（结果保障）。两者结合，既能引导 Agent 正确执行，也能在状态偏离（如手动修改）时及时发现并纠偏。

4. **诊断工具只诊断，不治疗**：
   `inspect`、`invariant check`、`doctor` 等命令只报告问题并建议解决方案，绝不自动修复或自动启动 Workflow。修复决策始终保留在 Agent 和用户手中。

5. **不变量检查仅限 Shell (Shell-only)**：
   Invariant 的 `check` 步骤必须是纯 Shell 命令（exit code 0 为通过）。不支持 Prompt 检查（即调用 LLM 评估），以保障检查的确定性、速度以及静默通过的能力。复杂的语义检查应转化为时效性检查（见原则 6）。

6. **语义检查转化为时效性检查 (Freshness checks)**：
   对于无法通过 Shell 直接判断的语义要求（如"文档是否最新"），通过检查"最近 N 天内是否执行过对应的评审 Workflow"来替代。这保持了 Invariant 检查的高效与确定。

7. **路径构建的输入验证**：
   所有用于构建文件路径的外部输入必须经过验证，以防止路径遍历攻击。
   | 输入 | 预期用途 | 验证规则 |
   |------|----------|----------|
   | `session_id` | `/tmp/argus/<safe-id>.yaml` | 优先校验 UUID 格式 `^[0-9a-fA-F-]+$`；不符合时 SHA256 hash 取前 16 位得到 `safe-id` |
   | `workflow_id` | `.argus/pipelines/<id>-<ts>.yaml` | 命名规范: `^[a-z0-9]+(-[a-z0-9]+)*$` |
   | `invariant_id` | `.argus/invariants/<id>.yaml` | 与 `workflow_id` 相同 |
   *回退方案：构建路径后，始终使用 `filepath.Rel` 验证解析后的路径是否仍处于预期的父目录下。*

### 1.3 Rules 定位
**Rules 不是 Argus 的原生功能**。Argus 仅通过 Workflow Job 编排规则的生成过程。
- 各 Agent 使用各自原生的规则系统（如 Claude Code 的 `CLAUDE.md`，OpenCode 的 `AGENTS.md`）。
- Argus 提供 `argus-generate-rules` Skill 引导 Agent 为这些原生系统生成内容。
- 这样做避免了引入新的抽象层，同时充分利用了各 Agent 现有的能力。

### 1.4 术语表

| 术语 | 含义 | 易混淆概念 |
|------|------|------------|
| **Pipeline** | Workflow 执行的一个实例 | GitLab Pipeline |
| **Rule** | 编码或架构的约束规范 | **Skill** (指具体的操作能力或流程) |
| **Argus Command** | Argus CLI 的子命令 (如 `argus install`) | **Slash Command** (Agent 中的 `/argus-doctor`) |
| **Invariant** | 项目中应始终满足的条件 | **Check** (仅强调验证动作，不强调状态维持) |

**为什么选择 "Invariant"？**
选择该词是为了精确表达"应该始终为真的条件"这一语义。相比 `check`（动作感过强）、`guard`（偏防御，缺少纠偏含义）、`policy`（偏治理/权限）、`assertion`（偏测试断言）或 `rule`（已在 Agent 原生系统中使用），`invariant` 更好地契合了声明式系统（如 Kubernetes、Terraform）中期望状态的概念，强调了该条件的不变性。

---

## 2. 目录结构与状态管理

### 2.1 项目级目录结构
```text
.argus/
  workflows/         # Workflow YAML 定义 (Git 跟踪)
    _shared.yaml     # 跨 Workflow 共享的 Job 定义
  invariants/        # Invariant YAML 定义 (Git 跟踪)
  rules/             # 由 Agent 生成的项目规则文件 (Git 跟踪)
  pipelines/         # Pipeline 实例运行数据 (Local-only, 忽略)
  logs/              # Hook 执行日志 (Local-only, 忽略)
  data/              # 通用数据目录，如时效性检查的时间戳标记 (Git 跟踪)
  tmp/               # 其它临时数据 (Local-only, 忽略)
.agents/skills/argus-*/SKILL.md  # 随项目分发的 Skills (Git 跟踪)
```

### 2.2 状态清单 (State Inventory)
Argus 严格区分受 Git 跟踪的共享状态和仅本机有效的本地状态。

#### Git-tracked (团队共享)

| 产物 | 路径 | 生成/管理方式 |
|------|------|---------|
| Workflow 定义 | `.argus/workflows/*.yaml` | `install` 生成内置 + 用户/Agent 创建 |
| 共享 Job 定义 | `.argus/workflows/_shared.yaml` | 用户或 Agent 创建 |
| Invariant 定义 | `.argus/invariants/*.yaml` | `install` 生成内置 + 用户创建 |
| 项目规则 | `.argus/rules/` | Agent 生成 (由 Init Workflow 驱动) |
| Skills | `.agents/skills/argus-*/SKILL.md` | `install` 生成 |
| Agent 原生 Rules | `CLAUDE.md`, `AGENTS.md` 等 | Agent 生成 |
| Hook 配置文件 | 如 `.claude/settings.json`, `.codex/hooks.json` | `install` 生成 |
| Git Hooks | `.husky/pre-commit` 或其他框架配置 | Agent 配置（Hook 框架配置 Git-tracked；`.git/hooks/*` 本身是 local-only 不可提交，仅作为无框架时的 fallback。团队项目应优先使用 husky 等 Git-tracked 框架） |
| `.gitignore` | `.gitignore` | Agent 添加 |
| 通用数据文件 | `.argus/data/` | Workflow/Invariant 使用的数据文件（如时效性检查的时间戳标记） |

#### Local-only (本机有效)

| 状态/数据 | 路径 | 说明 |
|----------|------|------|
| Pipeline 数据文件 | `.argus/pipelines/*.yaml` | 记录 Pipeline 实例的运行进度与状态 |
| Hook 日志 | `.argus/logs/hook.log` | 所有 Agent 的 Hook 执行日志，用于诊断 |
| Session 临时文件 | `/tmp/argus/<safe-id>.yaml` | 存储本 Session 内的临时标记 (如 Snooze 状态)。`safe-id` 由 session_id 验证/哈希后得到（见 pipeline §6.1） |

### 2.3 源码内嵌资源 (Embedded Assets)

Argus 二进制通过 Go `//go:embed` 内嵌多种资源文件，统一存放在源码的 `assets/` 目录下：

```text
assets/
  skills/                        # 内置 Skills（install 时释出）
    argus-doctor/SKILL.md
    argus-install/SKILL.md
  workflows/                     # 内置 Workflow 定义（install 时释出）
    argus-init.yaml
  invariants/                    # 内置 Invariant 定义（install 时释出）
    argus-init.yaml
  prompts/                       # 运行时输出模板（仅运行时读取，不释出）
    tick-full-context.md.tmpl
    tick-minimal.md.tmpl
    ...
```

```go
//go:embed assets/*
var Assets embed.FS
```

**使用方式**：
- `assets/workflows/`、`assets/invariants/`：`argus install`（项目级）时释出到项目目录 `.argus/workflows/`、`.argus/invariants/`。
- `assets/skills/`：`argus install`（项目级）时释出到 `.agents/skills/`；`argus install --workspace` 时释出到各 Agent 的全局 Skill 目录（详见 workspace §11.5）。
- `assets/prompts/`：仅在运行时由 argus 内部读取，用于 tick 注入、job-done 输出、错误引导等场景的文本模板渲染（Go `text/template`）。不释出到文件系统。

### 2.4 安装层级
1. **全局二进制**：Argus CLI 二进制文件安装在用户全局路径（如 `~/.local/bin/argus`）。
2. **PATH 查找**：Agent Hook 配置直接使用 `argus` 命令，不依赖项目内的相对路径。
3. **项目配置**：每个项目的 `.argus/` 目录存放该项目特有的编排配置和运行数据。
4. **用户级配置**：`~/.config/argus/config.yaml` 用于记录 Workspace 路径等全局偏好。`~/.config/argus/logs/` 存放全局 Hook 日志。

### 2.5 临时数据管理
Argus 在 `/tmp/argus/` 目录下管理 Session 相关数据。
- 文件名格式为 `<safe-id>.yaml`。`safe-id` 由 Agent 提供的 `session_id` 经验证后得到：符合 UUID 格式时直接使用，否则取 SHA256 前 16 位（详见 pipeline §6.1）。
- Session 文件记录了本 Session 的临时状态，包括已忽略 (Snoozed) 的 Pipeline 列表和 `last_tick` 状态快照（用于判断 tick 注入策略）。

---

## 3. 规范与约定

### 3.1 命名空间与标识符
- **命名空间保留**：`argus-` 前缀保留给系统内置的 Workflow、Invariant 和 Skill。用户定义的内容不得使用该前缀。
- **Skill 命名**：仅限小写字母、数字和连字符。正则表达式为 `^[a-z0-9]+(-[a-z0-9]+)*$`。最大长度 64 字符。
- **ID 格式**：Workflow 和 Invariant 的 ID 遵循同样的命名规范，作为唯一的机器标识符。

### 3.2 版本字段
所有独立的 Schema 文件（Workflow YAML, Invariant YAML, Pipeline Data）必须包含 `version` 字段。
- 当前版本：`v0.1.0`。
- Argus 在解析时会检查主版本（Major version）的兼容性。

### 3.3 时间戳格式
Argus 持久化的所有时间戳统一使用 **compact UTC 格式**：`YYYYMMDDTHHMMSSZ`（示例：`20240115T103000Z`）。

适用范围：
- Pipeline 数据文件中的 `started_at`、`ended_at` 等字段
- Pipeline 实例 ID 中的时间戳部分
- Session 文件中的 `last_tick` 时间戳
- `argus toolbox touch-timestamp` 写入的时间戳内容
- Hook 日志 (`hook.log`) 中的时间戳

该格式不含冒号和连字符，既可直接用于文件名，又便于排序和解析。

---

## 4. 导航

本文档是技术规范的起点。欲深入了解具体模块的设计细节，请参考以下文件：

- [CLI 与命令设计 (technical-cli.md)](technical-cli.md)：Argus CLI 的详细命令定义、参数说明及退出码约定。
- [Agent Hook 集成方案 (technical-hooks.md)](technical-hooks.md)：针对 Claude Code、Codex 和 OpenCode 的 Hook 集成细节。
- [Workflow YAML 规范 (technical-workflow.md)](technical-workflow.md)：Workflow 的 YAML Schema 定义、Job 模型及模板变量参考。
- [Invariant 不变量系统 (technical-invariant.md)](technical-invariant.md)：不变量的设计哲学、检查机制及初始化 (Init) 流程。
- [Pipeline 与状态管理 (technical-pipeline.md)](technical-pipeline.md)：Pipeline 的运行数据结构、Session 管理及状态持久化方案。
- [Workspace 发现机制 (technical-workspace.md)](technical-workspace.md)：跨项目 Workspace 的注册、引导及全局 Hook 的发现逻辑。
