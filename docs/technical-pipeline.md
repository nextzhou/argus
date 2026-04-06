# Pipeline 与 Session 状态管理

本文档定义了 Argus V2 的 Pipeline 运行时数据模型、Session 管理机制以及状态跟踪逻辑。这些设计遵循 "产物作为 ground truth" 的核心原则，确保系统的健壮性与可预测性。

---

## 5.1 Pipeline 数据文件 Schema

Pipeline 运行时状态记录在独立的 YAML 文件中，而非中心化的状态库。

- **Instance ID**: 格式为 `<workflow-id>-<compact-UTC-timestamp>`（如 `release-20240115T103000Z`），**不含 `.yaml` 扩展名**。这是逻辑标识符，在 Session 文件、CLI 输出、日志等所有引用场景中统一使用此形式。
- **文件路径**: `.argus/pipelines/<instance-id>.yaml`（如 `release-20240115T103000Z.yaml`）。只在实际读写文件时拼接 `.yaml` 扩展名。
- **唯一性**: 创建时检测文件是否已存在，若存在则报错。Phase 1 强制单活跃 Pipeline（见 5.2），同一秒重复启动不会发生。
- **版本**: `version: v0.1.0`
- **设计决策**:
  - **不内嵌完整 Workflow 配置**: Pipeline 文件仅记录 `workflow_id`。运行时通过 ID 从 `.argus/workflows/` 读取定义。这避免了数据冗余，并确保定义变更能立即生效（暂不支持"临时 Workflow"快照）。
  - **运行中 Workflow 被修改**：Pipeline 每次 tick/job-done 时实时读取 workflow 定义文件。如果运行中 workflow 被修改（job 删除/改名/顺序变化），按新定义执行。若 `current_job` 在新定义中找不到，tick 输出错误提示文本（保持 tick 统一文本输出的约定），job-done 返回 error envelope（`{"status": "error", "message": "current_job '<id>' not found in workflow definition"}`），引导用户执行 `argus workflow cancel` 兜底处理。两者共用同一个"读取 workflow 定义 → 定位 current_job"的逻辑，保持一致的异常处理路径。`argus status` 在此场景下采用 best-effort 策略：正常返回 pipeline 基本信息（workflow_id、status、started_at 等）和 invariant 检查结果，但将 `current_job` 标记为 `null`，并在 `hints` 中提示 workflow 定义已变更。这是 Phase 1 的已知限制。
  - **全局 current_job 替代 per-job status**: 在顺序执行模型下，任务状态可根据其相对于 `current_job` 的位置推导。这简化了数据结构并消除了状态同步风险。

### YAML 示例

```yaml
version: v0.1.0
workflow_id: release
status: running                    # running | completed | failed | cancelled
current_job: run_tests             # 当前执行的 job id; 完成时为 null
started_at: "20240115T103000Z"
ended_at: null

jobs:
   lint:
     started_at: "20240115T103005Z"
     ended_at: "20240115T103100Z"
     message: "All lint checks passed"
   run_tests:
     started_at: "20240115T103105Z"
     ended_at: null
```

### 字段定义表

| 字段 | 类型 | 必填 | 可空 | 创建时机 | 更新时机 |
|------|------|------|------|----------|----------|
| `version` | string | 必填 | 不可空 | `workflow start` | 不更新 |
| `workflow_id` | string | 必填 | 不可空 | `workflow start` | 不更新 |
| `status` | enum(running/completed/failed/cancelled) | 必填 | 不可空 | `workflow start` (=running) | `job-done --end-pipeline/--fail`, `workflow cancel` |
| `current_job` | string | 必填 | 可空 | `workflow start` (=首个 job) | `job-done` 推进到下一个；completed 时设为 null；failed/cancelled 时保留当前值 |
| `started_at` | timestamp | 必填 | 不可空 | `workflow start` | 不更新 |
| `ended_at` | timestamp | 必填 | 可空 | `workflow start` (=null) | pipeline 结束时设置 |
| `jobs` | map | 必填 | 不可空 | `workflow start` (=包含首个 job entry) | 各 job 执行时按需添加 |
| `jobs.<id>.started_at` | timestamp | 必填 | 不可空 | 首个 job 由 `workflow start` 创建；后续 job 由 `job-done` 推进时创建 | 不更新 |
| `jobs.<id>.ended_at` | timestamp | 必填 | 可空 | job 开始时 (=null) | `job-done` 时设置 |
| `jobs.<id>.message` | string | 可选 | 可空 | — | `job-done --message` 时设置 |

**说明**: `jobs` 采用按需添加策略。`workflow start` 时仅创建首个 job 的 entry，后续 job 在 `job-done` 推进时才创建对应 entry。

### 状态判定逻辑
- **status: completed** + **current_job: null**: 所有任务圆满完成。
- **status: failed** + **current_job: <job_id>**: 任务在执行 `<job_id>` 时失败。
- **status: cancelled**: 流程被手动取消。

---

## 5.2 Pipeline 生命周期

### 启动流程
通过 `argus workflow start <workflow-id>` 命令创建。若 `.argus/pipelines/` 目录不存在，Argus 会自动创建。系统会生成新的 pipeline 数据文件，并返回第一个 Job 的指令信息。**Phase 1 强制单活跃**：如果当前已存在 `status: running` 的 Pipeline，`workflow start` 直接报错，提示用户先完成或取消当前 Pipeline。目录扫描发现多个 running Pipeline 视为异常状态，`doctor` 会报告。

### 推进机制
Agent 完成任务后调用 `argus job-done`。Argus 会更新当前 Job 的元数据，并将 `current_job` 推进至下一个定义的任务。

**后续扩展方向**：Phase 1 之后可支持多活跃 Pipeline。扩展时需要给 `workflow cancel`、`snooze`、`status`、`tick` 等命令添加 `--pipeline <instance-id>` 参数以指定目标 Pipeline。数据文件格式和目录扫描机制天然支持多活跃，无需迁移。

---

## 5.3 提前结束 (--end-pipeline)

在某些场景下，Agent 可能发现后续步骤已无必要（例如 release 流程中发现没有代码变更）。

- **命令**: `argus job-done --end-pipeline`
- **行为**: 完成当前 Job 记录，并将 Pipeline 状态标记为 `completed`，同时将 `current_job` 设为 `null`。
- **命名依据**: 曾考虑 `--finish`，但因其无法明确区分是结束 Job 还是结束整个 Pipeline 而被放弃。`--end-pipeline` 语义明确，指向流程层级。

---

## 5.4 Job 失败与恢复

### 失败上报
当 Agent 遇到无法自动修复或需要人工干预的问题时，调用 `argus job-done --fail --message "失败原因"`。
- **状态变更**: Pipeline `status` 变为 `failed`，`current_job` 保留在该 Job。

### 恢复选项
1. **忽略**: 允许 Pipeline 停留在失败状态，不影响其他新流程。
2. **取消**: 执行 `argus workflow cancel` 彻底终结。
3. **重新开始**: 再次执行 `argus workflow start`。新流程将从第一个 Job 重新跑起。

**注意**: 第一期不支持 `resume`（从失败处继续）。理由是重新启动足以覆盖大多数场景，且 Job Prompt 的设计应保证 Agent 能够识别并跳过已完成的工作，从而保持状态机简单。

### Agent 失败判断与自动恢复

> 注意：Agent 并非一遇到问题就调用 `--fail`。Agent 可以自行重试、向用户求助。只有 agent 判断确实无法继续时才报告失败。另外，如果 session 结束时 agent 没有调用 job-done，pipeline 保持 `running` 状态，下个 session 的 tick 会重新注入该 job 上下文，agent 自动重试。

---

## 5.4.1 状态迁移表

以下是各操作对 Pipeline 字段的完整影响：

| 操作 | status | current_job | ended_at | 当前 job.ended_at | 当前 job.message |
|------|--------|-------------|----------|-------------------|-----------------|
| `workflow start` | running | 首个 job | — | — | — |
| `job-done`（中间 job） | running | 下一个 job | — | 设置 | `--message` 值 |
| `job-done`（最后一个 job） | completed | null | 设置 | 设置 | `--message` 值 |
| `job-done --end-pipeline` | completed | null | 设置 | 设置 | `--message` 值 |
| `job-done --fail` | failed | 保留当前值 | 设置 | 设置 | `--message` 值 |
| `job-done --fail --end-pipeline` | failed | 保留当前值 | 设置 | 设置 | `--message` 值 |
| `workflow cancel` | cancelled | 保留当前值 | 设置 | 不设置 | 不设置 |

**说明**：
- `--end-pipeline` 表示提前结束，默认为成功（completed）。与 `--fail` 组合时为提前失败结束。
- `--fail` 时 `current_job` 保留当前值，便于定位失败的 job。
- `workflow cancel` 不影响当前 job 的记录，因为 cancel 是外部操作。

---

## 5.5 Snooze (Session 级别忽略)

当用户正在进行其他紧急任务时，可以通过 Snooze 暂时静默活跃的 Pipeline 提醒。

- **触发**: 用户表达"稍后再做"，Agent 调用内部命令 `argus workflow snooze --session <session-id>`。
- **存储**: Pipeline 实例 ID 被追加到 Session 文件（`/tmp/argus/<safe-id>.yaml`）中的 `snoozed_pipelines` 列表。`safe-id` 由 `session-id` 验证/哈希后得到（见 §6.1）。
- **效果**: Pipeline 在全局仍处于 `running` 状态，但本 Session 剩余的 tick 将跳过该流程的上下文注入。新的 Session 会重新唤起提醒。

---

## 5.6 取消 (Cancel)

通过 `argus workflow cancel` 主动终止。状态设为 `cancelled`，该流程不再被 tick 关注。

**异常状态处理**：如果目录扫描发现多个 `status: running` 的 Pipeline（Phase 1 不应发生），`workflow cancel` 会取消所有 running Pipeline；`workflow snooze` 会将所有 running Pipeline 加入 snooze 列表。其他命令（`tick`、`status`、`job-done`）在此异常状态下报错，引导用户使用 `workflow cancel` 或 `doctor` 恢复。

**snooze-all 后的优先级**：若当前 session 中所有 running Pipeline 都已在 `snoozed_pipelines` 列表中，后续 `tick` 按"已 Snooze"静默跳过（不注入上下文，不报错）；但 `status` 和 `job-done` 仍返回异常状态错误，因为它们需要明确的单一 Pipeline 目标。这使得用户可以通过 snooze 暂时消除 tick 的干扰，同时异常状态本身仍可通过 `workflow cancel` 或 `doctor` 正式解决。

---

## 5.7 活跃 Pipeline 识别

Argus 采用 **目录扫描** 机制来识别活跃流程。通过遍历 `.argus/pipelines/` 目录并读取每个文件的 `status` 字段，凡是 `status: running` 的均视为活跃。

**损坏文件处理**：扫描时如果遇到不可解析的 YAML 文件，跳过该文件并在输出中附带警告（提示运行 `argus doctor` 排查）。`doctor` 会列出所有损坏文件的路径和具体解析错误，并给出修复指引（手动修复内容或删除文件；running 状态的 pipeline 删除后需重新启动 workflow）。

### 方案权衡分析

| 方案 | 多活跃扩展 | 一致性风险 | 符合 Ground Truth | 性能 |
|------|-----------|-----------|------------------|------|
| A. 指针文件 (.active) | 需改格式 + 读改写 | 高（冗余索引） | 否 | O(1) |
| B. 符号链接 | 命名复杂 | 高（冗余链接） | 否 | O(1) |
| **C. 目录扫描** | **天然支持** | **无** | **是** | **O(n)** |
| D. 固定文件名 rename | 不可用 | 高 | 否 | O(1) |

**采用理由**: 目录扫描消除了派生索引带来的数据不一致风险，完全符合架构原则。在实际项目规模下，几千个文件的头部扫描性能损耗可忽略不计。

---

## 5.8 Pipeline 历史

- **统一存放**: 活跃与历史 Pipeline 均存放在同一目录，仅靠状态位区分。
- **清理策略**: 第一期不引入自动清理机制。所有流程记录均被保留用于后续的审计与诊断。

---

## 5.9 tick 注入策略与状态变化检测

为了避免在对话中重复注入相同指令，Argus 在 **Session 文件**中记录 `last_tick` 状态（而非 Pipeline 文件）。这确保了跨 Session 的正确行为——新 Session 的 Agent 没有上下文时，能收到完整的上下文注入。

- **字段**：
  - `pipeline`: 上次 tick 时的 Pipeline 实例 ID（不含 `.yaml` 扩展名）。
  - `job`: 上次 tick 时的 `current_job` ID。
  - `timestamp`: 上次 tick 的时间戳（诊断用）。

### 注入策略逻辑表

| 条件 | 判定 | 注入行为 |
|------|------|------|
| 无活跃 Pipeline | — | 展示可用 Workflow 列表及启动方式 |
| 活跃 Pipeline, `last_tick` 为空 | Session 首次 tick 或新流程启动 | 注入完整任务上下文与技能指导 |
| 活跃 Pipeline, 当前状态与 Session 的 `last_tick` 不一致 | 任务已推进 | 注入新 Job 的完整上下文 |
| 活跃 Pipeline, 当前状态与 Session 的 `last_tick` 一致 | 状态无变化 | 仅注入最小摘要（当前 Job ID + 状态提醒） |

---

## 6. Session 状态管理

### 6.1 Session 数据存储

Session 数据用于记录跨 Hook 执行的临时状态（如 Snooze 标记、首次启动检查等）。

- **路径**: `/tmp/argus/<safe-id>.yaml`。`<safe-id>` 来源于 Agent 提供的 session_id：符合 UUID 格式（`^[0-9a-fA-F-]+$`）时直接使用，不符合时对原值做 SHA256 hash 取前 16 位作为安全文件名。
- **清理**: 随操作系统的临时目录清理策略自动回收。
- **唯一性**: 通过 session_id 的唯一性保证（UUID 或 hash 映射），确保在多项目并发开发时不会发生冲突。

### 路径方案权衡分析

| 方案 | 路径示例 | 结论 |
|------|---------|------|
| 独立目录 | /tmp/argus-<safe-id>/data.yaml | 排除: 对单一文件而言结构过重 |
| 扁平文件 | /tmp/argus-session-<safe-id>.yaml | 排除: 散落在 /tmp 根目录不便管理 |
| **归组存放** | **/tmp/argus/<safe-id>.yaml** | **采用**: 集中管理，且便于手动一键清理 |

---

### 6.2 Session 数据内容

```yaml
# /tmp/argus/<safe-id>.yaml
snoozed_pipelines:
  - release-20240115T103000Z
last_tick:
   pipeline: release-20240115T103000Z
   job: run_tests
   timestamp: "20240115T103500Z"
```

- **snoozed_pipelines**: 列表形式，记录本 Session 需要忽略的 Pipeline 实例。
- **last_tick**: 记录本 Session 最近一次 tick 时的 Pipeline 状态快照，用于判断是否需要注入完整上下文。
- **invariant_checked 移除说明**: 早期设计的 `invariant_checked` 已删除。现在通过 Session 文件的 **存在性** 来判定 `auto: session_start` 的 Invariant 是否已执行。文件存在即代表在该 Session 启动时已进行过检查。

---

### 6.3 Session ID 来源

各 Agent 均在 Hook 输入中提供 Session ID。Go 侧通过 `--agent` 标志进行字段映射归一化：

- **Claude Code**: `session_id` (来自标准输入 JSON)
- **Codex**: `session_id` (来自标准输入 JSON)
- **OpenCode**: `sessionID` (来自 named hook 参数)

---

### 6.4 首次 Tick 检测

Argus 利用 Session 文件的存在性来区分 Session 的"首次进入"与"后续交互"。

### 检测流程图

```text
tick 触发 
  ↓
从 Hook 输入提取 session_id
  ↓
检查 /tmp/argus/<safe-id>.yaml 是否存在
  ↓
┌────────── 不存在 (首次 Tick) ──────────┐
│ 1. 执行所有 auto: session_start 的    │
│    Invariant 检查 (无论通过或失败)    │
│ 2. 创建 Session 数据文件               │
│ 3. 向 Agent 注入首次欢迎及环境检查信息  │
└──────────────────────────────────────┘
  ↓
┌─────────── 存在 (非首次 Tick) ──────────┐
│ 1. 跳过 session_start Invariant 检查  │
│ 2. 根据 Pipeline 状态进行正常上下文注入  │
└───────────────────────────────────────┘
```

**安全性设计**: Session 文件在 Invariant 检查 **执行完成后** 创建（无论检查结果为通过还是失败）。如果检查过程中断（如进程被 kill），下次 tick 将重新触发完整检查流程，确保环境不变量得到有效验证。
