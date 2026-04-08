# 技术运营与安全性设计

本文件详细说明 Argus 的系统诊断工具、安全性方案以及各阶段的功能迭代规划。

## 12. Doctor 检查项

### 12.1 设计原则

Argus 遵循架构原则 #4：诊断工具只做诊断，不进行修复。`doctor` 命令仅报告发现的问题并提供操作建议，严禁自动执行修复逻辑或启动任何 Workflow。

*   **退出码规范**：遵循 `git diff --exit-code` 风格。所有检查通过返回 0，发现任何异常项则返回 1。
*   **双重入口**：
    *   `argus doctor`：CLI 命令行工具，作为主要的系统化诊断入口。
    *   `argus-doctor` Agent Skill：在 Claude Code 中通过 `/argus-doctor` 调用，在 Codex 中通过 `$argus-doctor` 或 `/use argus-doctor` 调用，在 OpenCode 中通过 `skill` 工具调用。该 Skill 主要用于 Argus 二进制文件损坏或未安装时的离线诊断。
*   **二进制缺失时的降级**：`argus-doctor` Skill 在 Argus 二进制不可用时，将依赖二进制的检查项（如 `argus version`、`workflow inspect`、`invariant inspect`、内置 Invariant 执行）标记为 **skipped**（附注"argus binary not found"），其余不依赖二进制的检查项（文件存在性、目录结构、Hook 配置、`.gitignore` 等）正常执行。最终报告汇总：N passed / M failed / K skipped。

### 12.2 完整检查清单

Doctor 诊断涵盖以下 13 个维度的内容：

#### 1. Argus 安装完整性
*   `.argus/` 根目录是否存在。
*   `.argus/workflows/` 目录是否存在。
*   `.argus/invariants/` 目录是否存在。
*   `argus version` 报告的版本号是否可读。

#### 2. Hook 配置校验
*   **Agent 范围规则**：Doctor 只校验当前已存在 Argus Hook 产物的 Agent。即：扫描各 Agent 的配置文件路径（如 `.claude/settings.json`、`.codex/hooks.json`、`.opencode/plugins/argus.ts`），仅对已存在的配置文件校验其中是否包含正确的 `argus tick` 和 `argus trap` 条目。配置文件不存在的 Agent 视为未启用，跳过不报错。
*   确认 `argus` 二进制文件已在系统的 PATH 环境变量中。

#### 3. Workflow 文件验证
*   调用内部命令 `argus workflow inspect`。
*   校验 YAML 语法、Schema 格式以及跨文件的 Job 引用一致性。

#### 4. Invariant 文件验证
*   调用内部命令 `argus invariant inspect`。
*   校验 Invariant 定义的格式完整性。

#### 5. 内置 Invariant 检查
*   执行所有以 `argus-` 为前缀的内置 Invariant 的 shell check 脚本。
*   注意：为了保持诊断的可预测性，`doctor` 不会运行用户自定义的 Invariant 检查。

#### 6. Skill 文件完整性
*   检查 Argus 管理的项目级 Skill 文件是否存在且路径符合规范：`.agents/skills/argus-*/SKILL.md` 与 `.claude/skills/argus-*/SKILL.md`（两者均由 Argus 写入）。

#### 7. .gitignore 配置
*   确认本地专用路径已正确列入 `.gitignore` 中。
*   必须包含：`.argus/pipelines/`, `.argus/logs/`, `.argus/tmp/`。
*   注意：`.argus/data/` 是团队共享的 Git-tracked 数据目录，**不应**列入 `.gitignore`。

#### 8. 日志健康状况
*   优先读取项目级日志 `.argus/logs/hook.log` 的**全部**记录；若项目级日志不存在，fallback 读取用户级日志 `~/.config/argus/logs/hook.log`（由 Workspace 全局 Hook 在未初始化项目中写入）。
*   检查是否存在 ERROR 级别的异常记录。
*   Doctor 作为低频诊断命令，可接受较长的执行耗时（1 分钟以内），无需采样。

#### 9. 版本兼容性检查
*   提取 Workflow、Invariant 和 Pipeline 文件中的 `version` 字段。
*   与当前 Argus 二进制版本进行主版本号（Major Version）匹配校验。

#### 10. 临时目录权限
*   确认 `/tmp/argus/` 目录对当前用户可写。
*   验证方式：尝试创建一个临时文件并随后将其删除。

#### 11. Pipeline 数据完整性
*   识别所有状态为 `running` 的活跃 Pipeline。
*   确认其对应的 `workflow_id` 在 workflows 目录中存在。
*   确保 Pipeline 数据文件没有损坏，能够被正常解析为 YAML。

#### 12. Shell 环境检查
*   检测用户默认 shell（`$SHELL`）是否为 bash。
*   若不是 bash，输出警告：Invariant shell check 固定使用 bash 执行，用户默认 shell 中配置的环境变量（如 PATH、alias）在 bash 中可能不可用。建议确保 invariant check 引用的工具在 bash 环境下可用。

#### 13. Workspace 相关检查（仅在配置了 Workspace 时）
*   列出所有已注册的 Workspace。
*   检查 Workspace 对应的物理路径是否仍然存在。
*   验证全局 Hook 配置是否正确。
*   检查用户级配置目录 `~/.config/argus/` 的访问权限。

## 13. Security

### 13.1 路径构造输入验证

为了防御路径遍历攻击，所有用于构造文件路径的外部输入在 Go 代码层级必须先进行严格验证。

| 输入字段 | 使用场景 | 验证规则 |
| :--- | :--- | :--- |
| session_id | `/tmp/argus/<safe-id>.yaml` | 优先校验 UUID 格式 `^[0-9a-fA-F-]+$`；不符合时对原值做 SHA256 hash 取前 16 位作为安全文件名（`safe-id`） |
| workflow_id | `.argus/pipelines/<workflow-id>-<timestamp>.yaml` | 命名规范：`^[a-z0-9]+(-[a-z0-9]+)*$` |
| invariant id | `.argus/invariants/<id>.yaml` | 与 workflow_id 相同的命名规范 |

**兜底逻辑**：在构造出完整路径后，必须通过 `filepath.Rel` 验证解析后的绝对路径依然位于预期的基准目录（如项目根目录或临时目录）之下。

### 13.2 命名空间预留

`argus-` 前缀被严格预留给系统内置的 Workflow、Invariant 和 Skill。Argus 在执行 `install` 或 `inspect` 操作时会强制检查该前缀。普通用户定义的任何项若带有此前缀，系统将报错并终止操作。

## 13.3 模板变量规范

Workflow Job 的 `prompt` 字段支持模板变量替换。

### 变量类别与 Phase 1 字段清单

Phase 1 必须实现的字段列表如下（后续版本可在各类别下按需扩展新字段）：

| 类别 | 字段 | 说明 |
|------|------|------|
| `workflow` | `{{ .workflow.id }}` | 当前 Workflow 的 ID |
| `workflow` | `{{ .workflow.description }}` | 当前 Workflow 的描述 |
| `job` | `{{ .job.id }}` | 当前 Job 的 ID |
| `job` | `{{ .job.index }}` | 当前 Job 在列表中的序号（0-based） |
| `pre_job` | `{{ .pre_job.id }}` | 前一个 Job 的 ID（首个 Job 时为空字符串） |
| `pre_job` | `{{ .pre_job.message }}` | 前一个 Job 的 message 输出（首个 Job 时为空字符串） |
| `git` | `{{ .git.branch }}` | 当前 Git 分支名 |
| `project` | `{{ .project.root }}` | 项目根目录绝对路径 |
| `env` | `{{ .env.XXX }}` | 环境变量透传，按变量名按需访问 |
| `jobs` | `{{ .jobs.run_tests.message }}` | 已完成 Job 的 message 输出（通过 Job ID 访问） |

`pre_job` 提供对直接前驱 Job 的快捷访问，无需知道具体 Job ID。首个 Job 执行时 `pre_job` 各字段均为空字符串。

**Job ID 命名约束**：Job ID 必须以小写字母开头，仅允许小写字母、数字和下划线，正则为 `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`。禁止连字符和数字开头（Go `text/template` 点语法限制）。详见 workflow §5.1。

### 缺失变量处理

模板采用**部分替换**策略：已知变量正常替换，未知变量（如引用了未定义的类别或字段）**保留原始 `{{ .xxx }}` 占位符不替换**，同时在 stderr 输出警告。不报错、不阻断流程。理由：模板可能引用了后期版本才添加的变量，不应阻断当前流程。对于 `jobs` 类别：引用的 Job 不存在或 message 为空时，均视为缺失变量，保留占位符。

## 14. Phase 1 Deferred Features

以下功能已列入后续迭代计划，在 Phase 1 中暂不实现：

| 功能名称 | 功能描述 | 延期理由 |
| :--- | :--- | :--- |
| Pipeline Resume | 支持从失败的 Job 处继续执行。 | 重新开始通常已足够。Agent 的自愈能力能处理大部分中断。 |
| 异步 Invariant | 后台执行的长耗时 Invariant 校验。 | 涉及复杂的触发时机和频率控制逻辑。 |
| Trap 规则引擎 | 基于 Workflow 定义的工具调用拦截规则。 | 目前暂无必须硬拦截的业务场景，提示词引导已满足需求。 |
| Pipeline 自动清理 | 自动清理过期的 Pipeline 状态文件。 | 初始阶段优先保证数据的可回溯性。后期再加入清理策略。 |
| DAG 并行执行 | 支持 Job 间的依赖关系图并并行运行。 | 第一阶段仅支持顺序执行。目前大多数 Agent 工作流仍为串行。 |
| 历史记录用例 | Pipeline 历史数据的具体消费场景。 | 待明确具体的诊断和展示需求后再行开发。 |


### 14.1 Removed Early Ideas

以下方案在设计过程中已被替代或因不再需要而被废弃：

*   `.argus/meta/rules-meta.json`：原本用于存储 Rule 的元数据（生成时间、来源等）。现已被 Invariant 的新鲜度检查机制（通过 mark 文件时间戳判断）所取代。
*   `.argus/deps/dependencies.json`：用于记录跨项目依赖。目前项目重心在于单项目内的编排，跨项目协同由 Agent 本身处理。
*   `.argus/prompts/`：原本设计的 Prompt 覆盖层。考虑到用户可以直接编辑 Workflow 文件中的 `prompt` 字段，引入额外的覆盖层会增加复杂度。
*   `.argus/config.json`：项目级配置文件。经分析发现，Agent 类型可通过 flag 指定，日志级别可通过环境变量控制，目前没有必须存在于项目配置中的条目。Workspace 相关的配置则存放于用户级的 `~/.config/argus/`。
