# Workflow 系统规范 (Technical Workflow)

本文档定义了 Argus 的 Workflow 系统规范，涵盖 YAML Schema、Job 模型、执行流程及校验机制。

---

## 1. 系统定位与设计哲学

Workflow 是 Argus 的命令式（Imperative）编排组件，负责定义"怎么做"（过程保障）。它通过将复杂任务拆解为一系列顺序执行的 Job，引导 AI Agent 完成预定的工作流。

根据 [技术概览 (technical-overview.md)](technical-overview.md) 中的架构不变量，Workflow 遵循以下核心原则：
- **编排层定位**：Argus 只负责状态跟踪和上下文注入，不直接执行业务逻辑。
- **Agent 驱动**：所有的操作通过注入 Prompt 或 Skill 交给 Agent 执行。
- **确定性 vs 自适应**：通过取消 `script` 字段，将执行权完全交给 Agent，换取更好的环境自适应能力和用户可见性。

---

## 2. Workflow YAML Schema

### 2.1 文件级字段

Workflow 定义文件存储在 `.argus/workflows/` 目录下。

| 字段 | 必填 | 说明 |
|------|------|------|
| `version` | 是 | Schema 版本。当前固定为 `v0.1.0`。 |
| `id` | 是 | 机器标识符。用于引用、关联 Pipeline 数据和 CLI 参数。命名规则：`^[a-z0-9]+(-[a-z0-9]+)*$`；`argus-` 前缀保留给内置 Workflow，用户不可使用。 |
| `description` | 否 | 人类可读的描述。 |
| `jobs` | 是 | Job 定义列表。 |

**示例：**

```yaml
version: v0.1.0
id: release
description: "标准发布流程"
jobs:
  - id: run_tests
    prompt: "Run `go test ./...` and report the results"
```

### 2.2 Job 字段定义

Job 是 Workflow 执行的最小单元。

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | 是* | Job 标识符。单文件内必须唯一。若使用 `ref` 则可省略。 |
| `description` | 否 | 人类可读描述。 |
| `ref` | 否 | 引用 `_shared.yaml` 中的 Job 定义。 |
| `prompt` | 否** | 注入给 Agent 的文本。支持模板变量。 |
| `skill` | 否** | 让 Agent 执行的 Skill 名称。 |

\* 若使用 `ref` 且未提供 `id`，则 `id` 默认继承 `ref` 的值。
\*\* `prompt` 和 `skill` 不能同时为空。

**Job 示例：**

```yaml
# 纯 prompt：Agent 按指令执行
- id: run_tests
  prompt: "Run `go test ./...` and report the results"

# Pure skill: Agent executes specified skill
- id: generate_rules
  skill: argus-generate-rules

# Original script scenario expressed as multi-line prompt
- id: lint_and_fix
  prompt: |
     Run `golangci-lint run ./...`.
     If there are lint errors, fix them.
     If lint passes, proceed.
```

---

## 3. Job 模型：纯 Agent 驱动

### 3.1 移除 script 字段的决策

在设计初期，Job 曾包含 `script` 字段，允许 Argus 直接执行 Bash 命令。经过深度讨论，我们决定彻底移除该字段。

**权衡分析 (Tradeoff Analysis)：**

| 维度 | script 方案 (已放弃) | Agent Prompt 方案 (当前) |
|------|-------------------|-----------------------|
| **实现成本** | 高。需要 Go 实现进程管理、超时控制、环境变量注入。 | 低。Argus 只负责状态机和上下文注入。 |
| **可见性** | 差。用户在 Agent UI 上看不到 Hook 阻塞执行的过程。 | 好。Agent 执行命令的过程对用户完全透明。 |
| **自适应性** | 差。命令写死，环境微差即导致失败，无法自我修复。 | 强。Agent 可根据环境调整命令，并在失败时自主尝试修复。 |
| **执行模型** | 复杂。需维护两套执行路径（Argus 执行 vs Agent 执行）。 | 简单。模型统一为"注入上下文 -> Agent 执行 -> job-done"。 |

**放弃的代价与评估：**
- **确定性**：虽然无法保证命令被 100% 精确执行，但可以通过 Prompt 明确约束（例如："执行 `git tag v1.0.0`，不要修改命令"）。
- **性能/成本**：Workflow 场景下额外的一次 LLM 调用不是瓶颈。
- **自动化**：Agent 需要手动或自动调用 `job-done`，虽然多了一步，但换取了流程的稳健性。

### 3.2 驱动逻辑

每个 Job 的执行遵循以下循环：
1. **注入**：Argus 将 `skill`（如有）和 `prompt`（如有）合并注入给 Agent。
2. **执行**：Agent 接收指令，利用其工具链（Bash, Read, Edit 等）完成任务。
3. **完成**：Agent 执行完毕后，调用 `job-done` 标记 Job 完成并推进流程。

---

## 4. Ref 语法与 _shared.yaml

为了实现 Job 的复用，Argus 支持引用共享定义。

### 4.1 _shared.yaml 结构

共享 Job 定义存储在 `.argus/workflows/_shared.yaml` 中。
- 不需要 `version` 字段。
- 所有 Job 嵌套在 `jobs:` 键下。
- **shared job key 命名**：shared job 的 key 必须遵循与 Job ID 相同的命名规范（`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`），因为未显式指定 `id` 时 `ref` 的 key 会默认作为 Job ID 继承。

**示例：**

```yaml
# .argus/workflows/_shared.yaml
jobs:
  lint:
    prompt: "Run `golangci-lint run ./...` and fix any errors"

  code_review:
    prompt: "Review changes for code quality and security"
```

### 4.2 引用机制

- 使用 `ref` 引用共享 Job。
- 可选提供 `id` 进行实例重命名。
- 其他字段（如 `prompt`）会直接覆盖（Override）共享定义中的相应字段。

**示例：**

```yaml
# .argus/workflows/release.yaml
version: v0.1.0
id: release

jobs:
   - ref: lint                        # 直接引用，id 默认为 "lint"

   - ref: code_review                 # 引用 + 覆盖内容 + 重命名
     id: strict_review
     prompt: "Review with extra focus on security"

   - id: tag_release                  # 直接定义
     prompt: "Create a git tag {{ .env.version }} and push it"
```

**排除的方案：**
- YAML 原生 anchor（`&` / `*`）：不支持跨文件引用，排除
- GitLab `extends` 风格：即使不覆盖也必须写 `id` + `extends` 两行，过于冗余

### Ref 合并语义

Job 通过 `ref` 引用 `_shared.yaml` 中的共享定义时，采用**浅合并（Shallow Merge）**策略：

| 情况 | 行为 |
|------|------|
| Job 中未出现的字段 | 保留 ref 的继承值 |
| Job 中显式写入的字段 | 覆盖 ref 的继承值 |
| Job 中写入 `null` 或空字符串 | 视为显式清空，继承值被移除 |

**示例**：
```yaml
# _shared.yaml
jobs:
  standard_test:
    skill: argus-run-tests
    prompt: "运行标准测试套件"

# workflow.yaml
jobs:
   - id: custom_test
     ref: standard_test
     prompt: "运行自定义测试，包括集成测试"  # 覆盖 ref 的 prompt
     # skill 未出现 → 保留继承值 argus-run-tests
```

---

## 5. 模板变量 (Template Variables)

Argus 使用 Go `text/template` 语法在派发 Job 前渲染 `prompt` 字段。

### 5.1 变量类别与 Phase 1 字段清单

模板引擎支持以下类别的变量。Phase 1 必须实现的字段列表如下（后续版本可在各类别下按需扩展新字段）：

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

**Job ID 命名约束**：Job ID 必须以小写字母开头，仅允许小写字母、数字和下划线，正则为 `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`。禁止使用连字符 `-`（Go `text/template` 点语法将其解析为减法），且必须字母开头（Go `text/template` 标识符要求）。这确保了 `{{ .jobs.<job_id>.message }}` 始终是合法的模板表达式。

**缺失变量处理**：模板采用部分替换策略——已知变量正常替换，未知变量（如引用了未定义的类别或字段）**保留原始 `{{ .xxx }}` 占位符不替换**，同时在 stderr 输出警告。不报错、不阻断流程。理由：模板可能引用了后期版本才添加的变量，不应阻断当前流程。对于 `jobs` 类别：引用的 Job 不存在（如引用了后续 Job）或 message 字段为空时，均视为缺失变量，保留占位符。

### 5.2 校验规则

- Workflow 的 `jobs` 列表不可为空。空 job 列表在 `workflow inspect` 阶段报错。
- 内置 Workflow（如 `argus-init`）由 `argus install` 释出到 `.argus/workflows/` 目录，与用户自定义 Workflow 采用相同的存储和校验机制。

---

## 6. 执行流程与状态推进

Workflow 支持双推进路径，确保在不同场景下都能流畅执行。

### 6.1 双推进路径

| 路径 | 触发条件 | 典型场景 |
|------|---------|----------|
| **tick 路径** | 用户输入触发 Hook | 恢复执行、新 Session 接续、检查进度。 |
| **job-done 返回路径** | Agent 调用 `job-done` | 自动连续执行。`job-done` 会返回下一个 Job 的内容，Agent 无需等待用户输入即可继续。 |

### 6.2 tick 注入策略

为了避免在多轮对话中过度干扰用户，`tick` 采用差异化注入策略：
- **状态变化时**（切换到新 Job 或新 Pipeline）：注入完整上下文，包括 `prompt`、`skill` 和详细的操作引导。
- **状态未变时**：注入极简摘要（当前 Job ID + 状态），持续提醒 Agent 仍处于工作流中，防止其忘记调用 `job-done`。

### 6.3 Human-in-the-loop (人工干预)

Argus 遵循"不做细粒度控制"的原则：
- 不提供 `confirm` 或 `auto` 字段。
- 需要用户确认的操作直接写在 Job Prompt 中。
- Agent 根据上下文和项目规则，自主决定何时暂停并询问用户。

---

## 7. Workflow 校验 (workflow inspect)

### 7.1 命令用法

```bash
argus workflow inspect [dir] [--json]
```
- 始终以**目录**为单位进行校验，以确保跨文件引用（如 `_shared.yaml`）和 ID 冲突检测的准确性。
- 默认校验 `.argus/workflows/`。
- 默认输出可读文本；传入 `--json` 时返回结构化结果。

### 7.2 校验内容

1. **结构合法性**：YAML 语法、必填字段检查。
2. **逻辑一致性**：无重复 ID、`ref` 引用存在且有效。
3. **预防性检查**：未知 Key 检测（防止 typo）、模板语法合法性。
4. **版本兼容性**：检查 `version` 字段是否受当前 Argus 版本支持。
5. **ref 兼容性**：ref 覆盖字段与共享定义的兼容性校验。
6. **init workflow 识别**：检测目录中是否存在 init workflow（内置于 argus 二进制），在报告中标注。
7. **ID 格式校验**：Workflow `id` 必须匹配 `^[a-z0-9]+(-[a-z0-9]+)*$`。
8. **命名空间校验**：`argus-` 前缀保留给内置 Workflow，用户定义的 Workflow 不可使用。

### 7.3 输出格式 (JSON)

**校验通过：**

```json
{
  "status": "ok",
  "valid": true,
  "files": {
    "_shared.yaml": {"valid": true, "jobs": ["lint", "code_review"]},
    "release.yaml": {"valid": true, "workflow": {"id": "release", "jobs": 4}}
  }
}
```

**校验失败：**

```json
{
  "status": "ok",
  "valid": false,
  "files": {
    "release.yaml": {
      "valid": false,
      "errors": [
        {"path": "jobs[2]", "message": "prompt and skill are both empty"},
        {"path": "jobs[3].ref", "message": "ref 'nonexistent' not found in _shared.yaml"}
      ]
    }
  }
}
```

### 7.4 Agent 编辑流程建议

当 Agent 需要修改 Workflow 定义时，建议遵循以下原子化流程：
1. 将 `.argus/workflows/` 复制到临时目录（如 `/tmp/argus-draft/`）。
2. 在临时目录中执行编辑操作。
3. 运行 `argus workflow inspect /tmp/argus-draft/`。
4. 校验通过后，使用临时目录内容**整体替换**原目录（`rm -r` + `cp -r`）。

---

## 8. 延期交付的功能 (Deferred Features)

为了保持第一期的精简，以下功能暂不实现：
- **Parallel DAG**：目前仅支持顺序串行执行。
- **Constraints 字段**：负向约束直接通过 Prompt 表达。
- **Trap 门控规则**：Workflow YAML 中暂不支持配置细粒度的工具调用拦截规则，安全防护目前依赖基础设施权限和 Agent 自律。
- **Script 字段**：已彻底移除，详见第 3.1 节。
