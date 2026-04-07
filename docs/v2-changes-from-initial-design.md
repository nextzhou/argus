# Argus 正式版相对初稿的变化说明

本文基于 Argus 最初的 M1 技术规格初稿与当前的正式版方案整理，重点说明正式版相对初稿发生了哪些结构性变化。

需要说明的是，初稿资料以技术规格为主，而正式版的入口资料更偏产品概述，因此两侧材料的粒度并不完全对称。为避免把“产品概述没有展开”误判为“设计已删除”，本文在必要处结合了正式版的技术规范，用来确认当前已经明确下来的正式约束。

还需要特别强调一点：本文所说的“变化”，主要是指平台内核的职责边界、抽象层次和默认表达方式发生了变化，不等于正式版失去了对应能力。像规则生成、GitLab CI 集成、K8s 部署引导、飞书通知、跨项目协同等场景，在正式版中仍然可以实现，只是更多通过通用的 Workflow、Skill、Invariant、Hook 和 Workspace 机制来承载，而不是都在平台内核里为每一类场景单独设计一套专门子系统。

## 一句话总结

Argus 从“把很多能力作为平台内建专门机制来描述的工作流平台”，演进成了“以通用编排原语来承载多种场景的 AI Agent 编排层”。正式版强化了边界、状态模型和声明式约束，把具体执行重新交还给 Agent，把诊断与修复严格分离，并把许多原本按场景展开的能力重新抽象到更通用的一层。

## 1. 核心变化总览

| 维度 | 初稿（M1） | 正式版 | 变化含义 |
| --- | --- | --- | --- |
| 产品定位 | 工作流引擎 + 规则生成系统 + 外部系统集成枢纽 | AI Agent 编排层 | 平台边界更薄，职责更聚焦 |
| 核心模型 | 以 Workflow 为中心 | Workflow + Invariant + Pipeline 三元模型 | 从“只管流程”变成“流程 + 持续约束 + 实例状态” |
| Job 执行 | Script Job 由 Argus 执行；Agent Job 由 Agent 执行 | 所有 Job 都由 Agent 执行 | Argus 不再直接跑业务命令 |
| Workflow 结构 | 单个 `workflow.yaml`，强调固定阶段和 Job 开关 | 多个 workflow 文件，直接定义顺序 Job 列表 | 配置更模块化，抽象更简单 |
| 状态存储 | `.argus/state.yaml` 中心化记录 | `.argus/pipelines/*.yaml` + `/tmp/argus/*.yaml` | 从中心状态切到按实例存储 |
| 规则能力表达 | 规则生成、元数据、自迭代以平台内建子系统表达 | 规则生成由通用 Workflow/Skill 编排承载 | 能力仍在，但抽象从专用子系统转向通用编排 |
| 约束方式 | `tick` / `trap` / `job done` 为主，偏流程门控 | 在此基础上引入 Invariant，强调持续检查和偏离发现 | 从过程约束扩展到状态约束 |
| 自动修复 | 旧设计强调流程推进、规则再生成、Hook 约束 | 诊断工具只诊断，不自动修复或自动启动 Workflow | Human-in-the-loop 更强 |
| Agent 集成 | 设计表述以 Claude Code Hook 为主，其他 Agent 是兼容性方向 | Claude Code、Codex、OpenCode 都是原生支持对象 | 跨 Agent 成为一等能力 |
| 外部场景承载方式 | GitLab CI、K8s、跨项目协同、内部 CLI wrapper 等以专门方案进入主规格 | 以编排内核、Hooks、Workspace、Workflow、Skill 这些通用机制承载 | 不是能力消失，而是表达方式更统一 |

## 2. 详细变化

### 2.1 从“操作系统式平台”调整为“薄编排层”

初稿把 Argus 描述为类似操作系统的内核，不仅调度 Agent，也直接执行 Script Job，并把规则生成、GitLab/K8s/飞书等外部系统对接作为平台内部的重要组成部分来描述。

正式版保留了“Argus 不是 Agent 替代品”这个主线，但进一步收紧边界：Argus 主要负责状态跟踪、上下文注入、约束检查与建议，不直接承担业务执行逻辑。

这里的变化不是说正式版不能再生成规则或对接外部系统，而是说这些能力更多通过上层编排来实现，而不是继续沉淀为平台内核里的专门执行模块。

这意味着正式版更像 orchestrator，而不是 workflow runtime、tool runner、rule engine 的组合体。

### 2.2 Workflow 模型被简化为纯 Agent 驱动

初稿的关键抽象是两类 Job：

- Script Job：Argus 自己跑命令
- Agent Job：Agent 做完后调用 `job done` / `job fail`

正式版彻底移除了 `script` 字段。所有 Job 都统一为“Argus 注入 prompt/skill -> Agent 执行 -> Agent 调用 `job-done`”。

这带来三点实质变化：

1. Argus 不需要维护两套执行路径。
2. 执行过程对用户完全可见。
3. 环境差异和失败恢复更多交给 Agent 自适应处理。

代价是确定性下降，但正式版明确接受这个取舍，认为这比平台内直接执行 Shell 更符合 AI Agent 的实际工作方式。

### 2.3 Workflow 配置从“阶段配置”转向“直接定义 Job 列表”

初稿里，Workflow 由单个 `.argus/workflow.yaml` 和 `.argus/config.json` 共同驱动，强调的是固定阶段、预置 Job、按项目类型启停阶段与 Job。

正式版则把 Workflow 简化为 `.argus/workflows/*.yaml` 下的一组独立定义文件，每个 Workflow 直接给出顺序 Job 列表；共享片段通过 `_shared.yaml` + `ref` 复用。

这说明正式版不再强调“阶段编排层 + Job 编排层”这类更重的配置结构，转而采用更直接的模型：

- Workflow 只表达一条任务路径
- Job 是唯一的运行时单位
- 复用通过共享 Job 定义完成，而不是通过阶段开关和大配置文件完成

### 2.4 从“Workflow 中心”扩展为“Workflow + Invariant + Pipeline”

初稿虽然也有 Pipeline，但整体仍是 Workflow Engine 视角：把流程跑完是核心。

正式版明确把三者拆开：

- Workflow：回答“怎么做”
- Invariant：回答“应该是什么状态”
- Pipeline：记录一次 Workflow 执行实例

其中 Invariant 是最重要的新能力。它补上了初稿难以覆盖的一类问题：即便流程曾经跑过，项目状态仍可能因手工修改、时间流逝、环境漂移而失真。正式版不再只依赖“流程是否执行过”，而是持续检查“结果是否仍然成立”。

### 2.5 状态管理从中心化文件改为“产物即真相”

初稿用 `.argus/state.yaml` 跟踪当前 Pipeline、各 Job 状态和指针。

正式版改用以下方式：

- `.argus/pipelines/*.yaml`：每个 Pipeline 一个实例文件
- `/tmp/argus/<safe-id>.yaml`：Session 级临时状态
- `.argus/data/`：通用数据，如 freshness timestamp

配套变化包括：

- 不再使用 `initialized: true` 这类标记
- 不再用一个总表维护所有运行态
- 以真实文件和产物是否存在作为 ground truth

这使正式版的状态更接近“可观察事实”，也减轻了中心状态文件损坏后的恢复问题。

### 2.6 规则能力从“专用内建子系统”转为“通用编排能力承载”

初稿中，规则是 Argus 的核心内建能力之一：

- `.argus/rules/` 存规则
- `rules-meta.json` 存 TTL、依赖、生成来源
- 领域探测、联网查询、规则生成、规则自迭代都是主规格的一部分
- `.argus/deps/dependencies.json` 用来描述跨项目依赖

正式版对这部分做了重新归位：

- 不再把 Rules 当成需要单独扩展的一套平台原生抽象
- 改为通过 Workflow/Skill 编排 Agent 去生成各 Agent 原生规则文件
- rules-meta、deps、TTL 这一类机制不再作为正式版平台内核的主线

换句话说，初稿把“规则系统”当成平台里的专门子系统；正式版则把“生成规则”视为一种可以被通用编排机制承载的任务类型。能力本身仍然存在，只是平台对它的建模方式更通用。

### 2.7 新增“不变量”，并把“诊断”和“修复”明确分开

初稿的强制力主要来自 Hook 体系：`tick`、`trap`、`job done` / `job fail`。

正式版保留这套机制，但增加了一个新的上层约束体系：Invariant。与初稿相比，正式版在这部分有几个关键新原则：

- Invariant 检查必须是 shell-only
- 复杂语义检查要转化为 freshness check
- `inspect`、`doctor`、`invariant check` 只报告问题，不自动修复
- 修复是否执行、何时执行，由 Agent 和用户决定

这说明正式版不再追求“平台自动推进一切”，而是更强调“平台负责发现偏离，人类和 Agent 负责决定如何纠偏”。

### 2.8 多 Agent 的含义发生了变化

初稿里，“多 Agent”更多是一个未来的运行时扩展方向，语义接近“多进程调度”或“多 Sub-Agent 并行执行”。

正式版产品层面强调的“支持多 Agent”，则是另一层含义：Claude Code、Codex、OpenCode 三种 Agent 工具都作为一等集成对象被支持，并尽量共享同一套 Workflow、Invariant 和 Pipeline 体验。

也就是说，正式版的重点已经从“调度多个 Agent 一起工作”转为“让不同 Agent 工具都能接入同一个编排内核”。

### 2.9 外部系统能力从“专门方案展开”转向“通用能力承载”

初稿把很多场景纳入了主规格：

- GitLab CI 集成
- K8s / kubefiles
- SLS / Grafana / tapsvc / 飞书 wrapper
- 跨项目契约检查
- 老项目渐进接入

这些内容在正式版的产品概述中不再是核心叙事，当前技术文档的重点也明显转向：

- Claude Code / Codex / OpenCode 的统一 Hook 集成
- Workflow / Invariant / Pipeline 的通用运行模型
- Workspace 级的多项目发现与引导

这里更准确的理解不是“正式版不做这些能力”，而是“正式版不再优先为这些能力各自定义专门内核机制”。正式版先把“跨 Agent 可复用的编排内核”做扎实，再通过具体 Workflow、Skill、Hook 和 Workspace 去承载 CI、部署、跨项目协同等场景。

### 2.10 人类在环比初稿更强

初稿虽然也没有把 Argus 定义成全自动系统，但整体仍偏向“通过引擎推进流程”。

正式版则显式强化了以下边界：

- 不自动修复
- 不自动启动 Workflow
- 不在 CLI 中内嵌复杂业务决策
- 需要确认的事情写进 prompt，让 Agent 自己判断何时询问用户

这意味着正式版把“是否继续”“如何修复”“是否接受建议”的决定权更明确地还给了用户和 Agent。

## 3. 没变的主线

尽管变化很大，下面几条主线在正式版中其实被保留并强化了：

- Argus 仍然不是 Agent 的替代品，而是其上层编排机制。
- Workflow 和 Pipeline 仍然是核心概念，只是建模更干净。
- Hook 驱动的运行方式仍然存在，Argus 依旧以 CLI + Hook 的方式嵌入 Agent 工作流。
- 项目级 `.argus/` 目录仍然是核心载体，只是内部结构和职责重新划分了。

## 4. 结论

如果用一句话概括这次演进：

Argus 正式版不是简单地在初稿上继续“加功能”，而是在做一次明确的抽象重组。它不再优先把“平台直接执行脚本、原生维护规则系统、为外部平台分别设计专门机制”作为核心路线，而是确立一个更薄、更稳、更通用的内核：用 Workflow 管过程，用 Invariant 守状态，用 Pipeline 记实例，再把具体执行交还给 AI Agent。

这种变化让正式版更容易跨 Agent 复用，也更符合“Argus 是编排层”的根本定位。同时，初稿中设想的很多具体场景能力并没有被否定，而是被上移到更通用的编排能力之上。
