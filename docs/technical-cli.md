# 7. CLI 技术规范 (Technical CLI)

本文档定义 Argus CLI 的命令集、全局参数、退出码约定以及各场景下的输出格式。

## 7.1 命令分类

Argus CLI 命令分为两类，通过 `help` 命令的可见性进行区分：

- **外部命令**：供人类用户直接执行，主要用于生命周期管理和系统诊断。由 `argus help` 默认展示。
- **内部命令**：供 AI Agent、Hook 系统或自动化脚本调用。由 `argus help --all` 展示。

Argus 采用 "Agent 为核心交互入口" 的设计模式。外部命令保持最小集，仅包含生命周期管理（install/uninstall）、故障诊断（doctor）和基础信息（version/help）。

## 7.2 外部命令

| 命令 | 说明 | 退出码 |
| :--- | :--- | :--- |
| `argus install [--yes] [--json]` | 安装 Argus 到当前项目。创建 `.argus/` 目录、配置 Agent Hook、生成内置 Skills。具有幂等性。子目录安装时需确认（`--yes` 跳过确认，详见 workspace §10.3.1）。默认输出人类可读文本；`--json` 返回结构化结果。 | 0: 成功, 1: 失败 |
| `argus install --workspace <path> [--yes] [--json]` | 注册 Workspace 路径。安装全局 Hook 和全局 Skills。Workspace 下未初始化的项目会被引导执行安装。执行前需确认（`--yes` 跳过）。默认输出人类可读文本；`--json` 返回结构化结果。 | 0: 成功, 1: 失败 |
| `argus uninstall [--yes] [--json]` | 从当前项目卸载 Argus。删除整个 `.argus/` 目录（Git-tracked 内容可通过 `git checkout` 恢复）、删除 Argus 管理的项目级 Skill 文件（`.agents/skills/argus-*` 与 `.claude/skills/argus-*`，保留非 `argus-` 前缀的用户自定义 Skill）、移除 Agent Hook 配置。执行前需确认（`--yes` 跳过）。Agent Skill 调用时应先检查 git 状态，提示用户未提交的文件，并使用 `--yes` 参数。默认输出人类可读文本；`--json` 返回结构化结果。 | 0: 成功, 1: 失败 |
| `argus uninstall --workspace <path> [--yes] [--json]` | 移除指定的 Workspace 路径。当没有剩余 Workspace 时，同时移除全局 Hook 和全局 Skills。执行前需确认（`--yes` 跳过）。默认输出人类可读文本；`--json` 返回结构化结果。 | 0: 成功, 1: 失败 |
| `argus doctor [--json]` | 诊断 Argus 配置状态（安装完整性、Hook 配置、文件校验等）。只诊断不治疗。默认输出人类可读报告；`--json` 返回结构化结果。 | 0: 通过, 1: 发现问题 |
| `argus version [--json]` | 显示版本号。默认输出简短文本；`--json` 返回结构化结果。 | 始终 0 |
| `argus help [--all]` | 展示帮助信息。`--all` 参数展示所有内部和外部命令。 | 始终 0 |

## 7.3 内部命令

| 命令 | 说明 |
| :--- | :--- |
| `argus tick` | 协作式调度点。每次用户在 Agent 中输入时被动触发，用于检查状态、注入上下文。 |
| `argus trap` | 操作门控。根据 Pipeline 状态拦截不被允许的工具调用。第一期不实现，始终 exit 0 放行，保留 Hook 入口。 |
| `argus job-done [--fail] [--end-pipeline] [--message "..."] [--json]` | Agent 报告当前 Job 结束。`--fail` 标记失败，`--end-pipeline` 提前结束 Pipeline（默认为成功，与 `--fail` 组合则为失败）。`--message` 附带摘要（可选）。默认输出可读文本；`--json` 返回结构化结果。无活跃 Pipeline 时返回引导性提示。 |
| `argus status [--json]` | 主动查询项目综合概览（Pipeline 进度、Invariant 状态）。实时执行 invariant check。默认输出可读文本；`--json` 返回结构化结果。 |
| `argus workflow start <workflow-id> [--json]` | 启动指定的 Workflow。Phase 1 强制单活跃：若已有 `status: running` 的 Pipeline，直接报错并提示先完成或取消当前 Pipeline。默认输出可读文本；`--json` 返回结构化结果。 |
| `argus workflow list [--json]` | 列出当前项目可用的所有 Workflow。默认输出可读文本；`--json` 返回结构化结果。 |
| `argus workflow cancel [--json]` | 中止当前活跃的 Pipeline。若存在多个 running Pipeline（异常状态），取消所有。无活跃 Pipeline 时在 `--json` 模式下返回 error envelope（`{"status": "error", "message": "no active pipeline"}`），exit 1。默认输出可读文本。 |
| `argus workflow snooze --session <id> [--json]` | 在当前 Session 中暂时忽略活跃 Pipeline。`tick` 不再注入该 Pipeline 内容，直到新 Session 启动。无活跃 Pipeline 时在 `--json` 模式下返回 error envelope（同 cancel），exit 1。默认输出可读文本。 |
| `argus workflow inspect [dir] [--json]` | 校验 Workflows 目录的格式与跨文件一致性。默认校验 `.argus/workflows/`。默认输出可读文本；`--json` 返回结构化结果。 |
| `argus invariant check [id] [--json]` | 执行 Invariant Shell 检查。若未指定 ID 则运行所有关联的检查。默认输出可读文本；`--json` 返回结构化结果。 |
| `argus invariant list [--json]` | 列出当前项目定义的所有 Invariants。默认输出可读文本；`--json` 返回结构化结果。 |
| `argus invariant inspect [dir] [--json]` | 校验 Invariants 目录的格式与交叉引用。默认校验 `.argus/invariants/`。默认输出可读文本；`--json` 返回结构化结果。 |
| `argus toolbox <tool> [args]` | 内置常用工具集（如 `jq`、`yq`），减少对宿主机环境的依赖。 |

## 7.3.1 人类命令输出模式

除 `tick` / `trap` / `toolbox` 这类协议型或工具型命令外，面向人类直接执行的 Argus 命令统一采用双输出模式：

- **默认输出**：人类可读文本。格式可以保留 Markdown-like 的标题、列表与分段，既便于人类阅读，也便于 Agent 直接理解。
- **`--json` 输出**：返回结构化 JSON，供脚本、字段级解析或 `argus toolbox jq` 等场景消费。
- **错误输出**：默认模式写入 `stderr`；`--json` 模式继续返回统一 error envelope。
- **不再提供 `--markdown`**：原本的 Agent 友好文本输出已成为默认行为，避免在“给人看”和“给 Agent 看”之间人为分裂模式。

## 7.3.2 生命周期命令的 `--json` 成功输出

`install` / `uninstall` 家族命令在 `--json` 模式下成功时统一返回 JSON envelope：

- `status: "ok"`
- `message`: 成功摘要
- `root`: 仅项目级 install 返回
- `path`: 仅 workspace 级 install / uninstall 返回；为规范化后的 workspace 路径
- `changes`: 实际发生的文件系统变更，按 `created` / `updated` / `removed` 分组
- `affected_paths`: 该命令管理的路径摘要列表

其中：

- `changes` 只列本次命令实际写入/删除的路径摘要；幂等场景可为空数组
- `affected_paths` 是稳定的摘要输出，允许将多条真实文件路径合并为一个展示项（如 `.agents/skills/argus-*`）

## 7.4 已移除的命令与理由

在设计迭代中，以下命令因职责重叠或模型简化被移除：

- `job current`：功能被 `tick`（被动注入）和 `status`（主动查询）覆盖。
- `job done` / `job fail`：合并为 `job-done` 命令，通过参数区分结果。
- `info`：静态信息由 `doctor` 和 `version` 提供，运行时信息由 `status` 提供。
- `rules regenerate`：规则重新生成由专门的 Workflow 承担，符合 Workflow 驱动原则。
- `rules check`：规则新鲜度检查已被 `tick` 逻辑覆盖。
- `rules list`：Agent 具备直接读取 `.argus/rules/` 目录的能力。

## 7.4.1 toolbox 规范

`argus toolbox <tool> [args]` 提供内置工具集，采用 busybox 模式——单一二进制内嵌多个工具实现，减少对宿主机环境的依赖。

### Phase 1 工具清单

| 工具 | Go 实现库 | 用途 |
|------|----------|------|
| `jq` | `itchyny/gojq` | 解析 JSON 输出（如 argus 命令返回值） |
| `yq` | `mikefarah/yq` | 读取 YAML 文件（如 workflow/invariant 定义） |
| `touch-timestamp` | 内置实现 | 将当前 UTC 时间戳写入指定文件（compact 格式 `YYYYMMDDTHHMMSSZ`） |
| `sha256sum` | `crypto/sha256` | 计算文件或 stdin 的 SHA256 哈希值，兼容 coreutils `sha256sum` 输出格式 |

### 使用方式
```bash
argus toolbox jq '.status' pipeline.yaml
argus toolbox yq '.jobs[0].id' workflow.yaml
argus toolbox touch-timestamp .argus/data/lint-passed
```

### 设计决策
- **输出/错误处理**：直接透传底层工具的 stdout/stderr 和 exit code，argus 不做额外包装。
- **主要使用场景**：invariant 的 shell check 脚本以及 hook wrapper 中解析 argus 命令的输出。
- **扩展机制**：后续可按需添加新工具，无需修改用户侧的使用方式。

## 7.5 全局参数

| 参数 | 说明 |
| :--- | :--- |
| `--agent <name>[,<name>...]` | 指定目标 Agent（`claude-code`, `codex`, `opencode`）。适用命令：`tick`/`trap`（输入侧解析各 Agent 的 stdin JSON 格式）、`install`/`uninstall`（指定安装/卸载哪些 Agent 的 Hook 和配置）。多值语法用逗号分隔，未传时默认操作所有已知 Agent。 |
| `--global` | 仅用于 `tick` / `trap`。标识调用来源于全局 Hook。由 `install --workspace` 自动写入全局配置，用户无需手动输入。 |

## 7.6 退出码约定

### Hook 命令规则 (tick / trap)

- **始终 exit 0 (Fail Open)**：为了不阻塞 Agent 的正常运行，Hook 命令即使发生内部错误（如无法读取配置文件、数据库损坏）也必须返回 0。
- **内部错误处理**：错误信息应作为警告包含在输出的上下文文本中，不影响流程控制。
- **避免 exit 2**：在 Claude Code 和 Codex 中，退出码 2 具有特殊的"阻断操作"含义，Argus 严禁使用。
- **Trap 拦截逻辑**：`trap` 的阻断决策通过输出 JSON 中的 `permissionDecision: "deny"` 字段实现，而非退出码。

### `--json` 模式下的统一 Envelope

支持 `--json` 的命令在该模式下共享统一的外层结构：
- **成功**：`{"status": "ok", ...}` + 命令特定数据字段
- **业务错误**：`{"status": "error", "message": "引导性提示"}` + exit 1

具体命令的特定字段在实现时逐步定义，文档中已定义 `workflow start`、`job-done`、`status` 的详细 schema（见 8.2-8.4）。其余命令（`workflow list/cancel/snooze`、`invariant list/check/inspect`）遵循相同 envelope。

### 常用命令规则

- **支持 `--json` 的人类命令与内部命令**：0 表示成功，1 表示业务错误（如参数无效、状态不匹配）。
- **外部命令**：
    - `install` / `uninstall`：0 成功，1 失败。
    - `version` / `help`：始终 0。
    - `doctor`：0 表示所有检查通过，1 表示发现潜在问题（类似 `git diff --exit-code`）。

### Agent Hook 系统退出码处理参考

| 退出码 | Claude Code 行为 | Codex 行为 |
| :--- | :--- | :--- |
| 0 | 成功，解析 stdout | 成功，解析 stdout |
| 2 | 阻断操作，stderr 为原因 | 阻断操作，stderr 为原因 |
| 非 0 (非 2) | 非阻塞错误，继续执行 | 报错 |

# 8. 输出格式

## 8.1 tick 输出 (5 种场景)

`argus tick --agent <name>` 输出统一的文本格式。各 Agent 的 Hook/Plugin 层负责将文本注入到对应的上下文机制中（Claude Code/Codex 封装在 `additionalContext` 字段，OpenCode 封装在 `output.parts`）。以下为各场景的输出内容示例。

**兼容性约束（重点）**：tick 输出虽然是纯文本，但首个非空白字符不得为 `[` 或 `{`。当前 Codex 会把这两种前缀当作 JSON 候选，从而把旧的方括号前缀文本误判为非法 JSON。为保持所有 Agent 上的统一输出，tick 使用 `Argus:` 这类普通文本前缀。

### 场景 1：无活跃 Pipeline
当项目未启动任何流程时，每次 `tick` 会提示可用 Workflow。
```markdown
 Argus: 当前没有活跃的 Pipeline。

可用 Workflows：
- release: 发布流程
- argus-init: 初始化项目的 Argus 配置

启动方式：argus workflow start <workflow-id>
```

### 场景 2：活跃 Pipeline，状态变化 (完整上下文)
当进入新 Job 或启动新 Pipeline 时，注入完整指引。
```markdown
Argus: Pipeline: release-20240405T103000Z | Workflow: release | 进度: 2/5

当前 Job: run_tests
Prompt: 运行所有测试，确保通过后再继续
Skill: argus-run-tests

完成后请调用：argus job-done --message "执行结果摘要"
提前结束 pipeline：argus job-done --end-pipeline
标记失败：argus job-done --fail --message "失败原因"
暂时忽略：argus workflow snooze --session ses_abc123
```

### 场景 3：活跃 Pipeline，状态未变 (最小摘要)
当用户与 Agent 进行多轮对话且 Job 未切换时，保持静默提醒防止 Agent 遗忘。
```markdown
Argus: Workflow: release | Job: run_tests (2/5) | 完成后请调用 argus job-done
```

### 场景 4：已 Snooze
若当前 Pipeline 已被当前 Session 暂时忽略，输出等同于场景 1（不再展示 Pipeline 信息）。

### 场景 5：首次 tick + Invariant 检查失败
在上述场景内容之后，追加 Invariant 检查结果。
```markdown
Argus: Invariant 检查未通过：
- argus-init: 项目未完成初始化
  建议：启动 argus-init workflow (argus workflow start argus-init)
```

## 8.2 workflow start 输出

`workflow start` 的输出结构复用 `job-done` 成功时的格式，语义一致——都是"下发下一个 job 的信息"。

**默认文本**
```markdown
Argus: Pipeline release-20240405T103000Z 已启动 (1/5)

当前 Job: lint
Prompt: 运行 lint 检查并修复问题
Skill: argus-run-lint

完成后请调用：argus job-done --message "执行结果摘要"
```
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline_status": "running",
  "progress": "1/5",
  "next_job": {
    "id": "lint",
    "prompt": "运行 lint 检查并修复问题",
    "skill": "argus-run-lint"
  }
}
```
## 8.3 job-done 输出 (6 种场景)

### 场景 1：成功，有下一个 Job
**默认文本**
```markdown
Argus: Job run_tests 完成 (3/5)

下一个 Job: deploy
Prompt: 将构建产物部署到 staging 环境

完成后请调用：argus job-done --message "执行结果摘要"
```
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline_status": "running",
  "progress": "3/5",
  "next_job": {
    "id": "deploy",
    "prompt": "将构建产物部署到 staging 环境",
    "skill": null
  }
}
```
### 场景 2：成功，最后一个 Job (Pipeline 完成)
**默认文本**
```markdown
Argus: Job deploy 完成 (5/5)
Pipeline release-20240405T103000Z 已全部完成。
```
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline_status": "completed",
  "progress": "5/5",
  "next_job": null
}
```
### 场景 3：提前结束 (--end-pipeline)
**默认文本**
```markdown
Argus: Job run_tests 完成，Pipeline 提前结束 (2/5)。
```
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline_status": "completed",
  "progress": "2/5",
  "early_exit": true,
  "next_job": null
}
```
### 场景 4：失败 (--fail)
**默认文本**
```markdown
Argus: Job run_tests 标记为失败，Pipeline 已停止 (2/5)。

可用操作：
- 重新开始：argus workflow start release
- 取消：argus workflow cancel
```
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline_status": "failed",
  "progress": "2/5",
  "failed_job": "run_tests",
  "next_job": null
}
```
### 场景 5：无活跃 Pipeline
**默认文本**
```markdown
Argus: 当前没有活跃的 Pipeline。
可以使用 argus workflow start <workflow-id> 启动一个 workflow。
```
**JSON (--json)**
```json
{
  "status": "error",
  "message": "当前没有活跃的 Pipeline。可以使用 argus workflow start <workflow-id> 启动一个 workflow。"
}
```

**说明**：`job-done` 是内部命令（非 hook 命令），无活跃 Pipeline 属于业务错误，exit code 为 1。提示内容采用引导性设计，帮助 Agent 理解下一步操作。

### 场景 6：提前失败结束 (--fail --end-pipeline)
**默认文本**
```markdown
Argus: Job run_tests 标记为失败，Pipeline 提前结束 (2/5)。

可用操作：
- 重新开始：argus workflow start release
- 取消：argus workflow cancel
```
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline_status": "failed",
  "progress": "2/5",
  "early_exit": true,
  "failed_job": "run_tests",
  "next_job": null
}
```
## 8.4 status 输出

`argus status` 提供项目维度的综合概览，包含三个信息维度：Pipeline 状态、Invariant 状态、Doctor 提示。

### 有活跃 Pipeline 时
**默认文本**
```markdown
Argus: 项目状态

Pipeline: release-20240115T103000Z (running) - Workflow: release - 进度 2/5
  1. [done] lint - All lint checks passed
  2. [>>]   run_tests
  3. [ ]    build
  4. [ ]    deploy_staging
  5. [ ]    deploy_prod

Invariant: 2 passed, 1 failed
  [FAIL] lint-clean: 24 小时内 lint 检查通过

提示：运行 argus doctor 检查 Argus 安装状态
```
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline": {
    "workflow_id": "release",
    "status": "running",
    "current_job": "run_tests",
    "started_at": "20240115T103000Z",
    "ended_at": null,
    "progress": {
      "current": 2,
      "total": 5
    },
    "jobs": [
      {"id": "lint", "status": "completed", "message": "All lint checks passed"},
      {"id": "run_tests", "status": "in_progress", "message": null},
      {"id": "build", "status": "pending", "message": null},
      {"id": "deploy_staging", "status": "pending", "message": null},
      {"id": "deploy_prod", "status": "pending", "message": null}
    ]
  },
  "invariants": {
    "passed": 2,
    "failed": 1,
    "details": [
      {"id": "argus-init", "description": "项目已完成 Argus 初始化", "status": "passed"},
      {"id": "lint-clean", "description": "代码应通过 lint 检查", "status": "failed"},
      {"id": "gitignore-complete", "description": ".gitignore 应包含 Argus 临时文件", "status": "passed"}
    ]
  },
  "hints": [
    "Invariant 检查总耗时 3.2s，建议运行 argus doctor 排查慢检查项"
  ]
}
```

**说明**：
- `pipeline.jobs` 列表展示 workflow 定义中的**所有** job（不仅是已执行的），结合 pipeline 数据文件推导每个 job 的状态（completed/in_progress/pending）。
- `invariants.details` 列出**所有** `auto` 不为 `never` 的 invariant 的实时检查结果（`auto: never` 的不出现），包括 passed 和 failed。每项包含 `description`（取自 invariant YAML 的顶层 `description` 字段），便于 Agent 理解检查内容。若 `description` 为空，则将各 check step 的 shell 命令简单拼接作为 fallback（由 Agent 自行理解语义）。
- `hints` 为通用提示数组，承载各类辅助信息（如 doctor 建议、性能警告等）。无提示时为空数组或省略该字段。
- Agent 和用户可以通过完整的 job 列表了解整个流程和当前进度。

### 无活跃 Pipeline 时
**JSON (--json)**
```json
{
  "status": "ok",
  "pipeline": null,
  "invariants": {
    "passed": 3,
    "failed": 0,
    "details": [
      {"id": "argus-init", "description": "项目已完成 Argus 初始化", "status": "passed"},
      {"id": "lint-clean", "description": "代码应通过 lint 检查", "status": "passed"},
      {"id": "gitignore-complete", "description": ".gitignore 应包含 Argus 临时文件", "status": "passed"}
    ]
  },
  "hints": []
}
```

## 8.5 其他命令输出

`workflow list/inspect`、`invariant list/check/inspect`、`workflow cancel/snooze` 遵循统一模式：
- **默认**：返回可读文本，供人类与 Agent 直接消费。
- **`--json`**：返回结构化 JSON，顶层包含 `"status": "ok/error"` envelope。

Phase 1 这些命令的具体字段定义在实现时根据实际需要确定，文档在实现阶段补充。当前仅保证默认文本与 `--json` 两条路径的语义一致，以及 `--json` 的 envelope 和 exit code 语义稳定。
