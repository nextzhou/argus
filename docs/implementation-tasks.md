# Argus V2 实施任务拆解

## 1. 前言

本文档面向后续执行实现工作的 AI Agents。
它不是技术规范的替代品，而是基于既有规范整理出的里程碑式落地计划。
目标是把 Argus V2 的 Phase 1 能力拆解成可串行推进、可并行协作、可逐任务提交的执行清单。
执行时必须始终把 `docs/technical-*.md` 作为规范真源；本文件只回答“先做什么、后做什么、每一步的完成定义是什么”。

### 1.1 已确认技术决策

| 决策项 | 选择 |
|--------|------|
| Go 版本 | 1.24 (latest) |
| Module Path | `github.com/nextzhou/argus` |
| CLI 框架 | cobra |
| 测试库 | testify/assert + require |
| 日志 | log/slog |
| YAML | gopkg.in/yaml.v3 |
| 错误处理 | Sentinel errors + Custom types, %w wrapping |
| 依赖哲学 | Stdlib-first (jq/yq toolbox is an intentional exception) |
| 包组织 | 按领域 (internal/core, internal/workflow, internal/invariant, internal/pipeline, internal/session, internal/hook, internal/workspace, internal/install, internal/doctor, internal/toolbox) |
| 版本注入 | ldflags (-X main.version=...) |
| Commit 规范 | Conventional Commits (type(scope): description) |
| Git Hooks | lefthook |
| Lint | golangci-lint (strict) |

### 1.2 通用执行约定

- 每个任务对应一个原子提交；不要把多个任务合并到同一个 commit。
- 默认采用 TDD 顺序：先写测试，再写实现，再做必要重构，最后提交。
- 所有实现必须尊重技术文档中的架构不变量，尤其是“Argus 是编排层，不直接替代 Agent 执行逻辑”。
- 每个 Milestone 收尾前都要跑通 `make build && make test && make lint`。
- 任务引用格式统一为 `M{n}-T{m}`，例如 `M2-T3`。
- 文档中的依赖声明是执行下限，不代表必须等待同 Milestone 内全部任务完成；若依赖满足，可按任务粒度并行。
- JSON 输出、错误 envelope、时间戳格式、路径校验等横切约束，必须尽量在基础模块阶段一次性定型，避免后续命令各自漂移。
- `tick` 与 `trap` 是 Hook 命令，必须遵循 fail-open；非 Hook 命令则遵循常规 exit code 语义。
- 运行时模板、内置 workflow / invariant / skills、hook wrapper 模板都以嵌入资源或内置模板为准，不引入额外动态下载机制。

### 1.3 范围声明

- IN：`docs/technical-*.md` 中定义的全部 Phase 1 功能。
- OUT：Deferred Features（Pipeline Resume、async Invariant、Trap rules engine、DAG parallel）、`cmd/argus-server/`、OpenCode experimental hooks（`chat.system.transform`、`session.compacting`）。
- NOTE：仓库里即使存在 `cmd/argus-server/` 目录，也不在本实施计划范围内。

### 1.4 执行方式建议

- 先做 M0-M1，建立工具链、命令框架和核心基础能力，再进入 schema / parser / store 层。
- M2 是 Phase 1 的“数据与定义基线”，完成前不要开始命令级编排逻辑。
- M3-M5 形成“启动 / 推进 / 检查 / Hook 注入”的闭环，是系统首次可用的关键阶段。
- M6-M7 负责安装、资源释出、Workspace 与 Doctor，属于交付层完善。
- M8 只做整体验证，不承载新设计决策。
- 若实现中发现规范歧义，应回到对应技术文档定位，而不是在代码里自行发明新行为。

## 2. 依赖概览

```text
Critical Path: M0 → M1 → M2 → M3 → M4 → M5 → M6 → M7 → M8
```

里程碑级并行提示：

- M2：T1、T5、T7、T8、T9、T10 可并行推进（都只依赖 M1 完成）。
- M3：T1 和 T2 可并行（都依赖 M2-T1 与 M2-T7）。
- M4：T1 和 T2 可并行。
- M6：T2、T3、T4 可并行（都依赖 M6-T1）。

| Milestone | 完成后可获得的能力 | 主要风险点 |
|-----------|--------------------|------------|
| M0 | Go 项目可编译、可测试、可 lint、可提交 | 工具链不稳定会拖慢全部后续任务 |
| M1 | 核心基础设施（ID、版本、路径、输出）就位 | 横切约束若做错会造成全局返工 |
| M2 | Workflow / Invariant / Pipeline / Session / Workspace 的静态数据模型与校验具备 | Schema 与校验不一致会污染后续所有命令 |
| M3 | Workflow 启动、推进、取消、状态查询首次闭环 | 状态机和模板引擎是最容易引入系统性缺陷的部分 |
| M4 | Invariant 运行、Session 状态控制、snooze、status 增强就位 | shell check 超时、短路和 session 首次进入逻辑容易被忽略 |
| M5 | Hook 输入输出与 tick/trap 主流程可用 | fail-open、global/project root 判断、子 agent 跳过是关键边界 |
| M6 | install/uninstall 与嵌入资源形成完整项目级交付 | 幂等性、模板释出和多 Agent hook 合并复杂度高 |
| M7 | Workspace 与 Doctor 完成交付级能力闭环 | 全局路径、去重、清理与诊断覆盖面广 |
| M8 | 端到端集成覆盖完成，Phase 1 可发布验收 | 测试 fixture 与环境隔离不足会造成不稳定 |

## 3. 里程碑 M0-M8

### M0: 项目骨架与开发工具链

**目标**: 完成最小可编译 Go 项目、统一构建命令、lint / hook / 编辑器规范与面向后续 Agent 的开发约定。完成后，仓库应具备稳定的 build/test/lint/commit 基线，后续任务可以围绕统一工具链推进。

**NOT IN SCOPE**: 不实现任何业务模块；不设计 CLI 子命令细节；不引入 Workflow / Invariant / Hook 的运行时逻辑；不处理 `cmd/argus-server/`。

#### M0-T1: Go 项目初始化

| 字段 | 内容 |
|------|------|
| 依赖 | 无 |
| 参考 | `AGENTS.md`；`technical-overview.md §2.3`；`technical-cli.md §7.2` |
| 产出 | `go.mod`, `go.sum`, `cmd/argus/main.go` |
| Commit | `build(project): initialize Go module and main entry point` |

**实现内容**:
- 初始化 module path 为 `github.com/nextzhou/argus`。
- 创建 `cmd/argus/main.go`，只保留最小可编译入口。
- 入口程序当前只负责输出版本信息或基础 help，不提前实现复杂参数解析。
- 确保目录命名与未来 `cmd/argus` 主二进制路径保持一致。
- 在任务说明中显式注明 `cmd/argus-server/` 不属于本项目范围，避免未来误接入。

**测试要求**:
- 先通过 `go test ./...` 验证空项目结构可通过。
- 验证 `go build ./cmd/argus` 能产出可执行文件。
- 验证最小入口运行后不会 panic。

---

#### M0-T2: Makefile

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1 |
| 参考 | `technical-overview.md §3.3`；`technical-cli.md §7.2` |
| 产出 | `Makefile` |
| Commit | `build(make): add Makefile with build/test/lint/fmt/clean targets` |

**实现内容**:
- 增加 `build` target，使用 ldflags 注入版本号到 `main.version`。
- `build` 输出固定到 `./bin/argus`，避免后续文档与测试路径不一致。
- 增加 `test` target，执行 `go test ./...`。
- 增加 `lint` target，执行 `golangci-lint run`。
- 增加 `fmt` target，执行 `goimports -w .`。
- 增加 `clean` target，清理 `bin/` 目录。

**测试要求**:
- 先写简单 smoke test 或命令验证说明，确保各 target 可独立执行。
- 验证 ldflags 版本注入在 `./bin/argus` 中可观察到效果。
- 验证 `clean` 不会误删非构建产物。

---

#### M0-T3: golangci-lint 配置

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1 |
| 参考 | `AGENTS.md`；`technical-overview.md §3` |
| 产出 | `.golangci.yml` |
| Commit | `build(lint): add golangci-lint configuration` |

**实现内容**:
- 创建严格模式 `.golangci.yml`。
- Go 版本固定为 1.24。
- 启用 `goimports`, `errcheck`, `govet`, `staticcheck`, `ineffassign`, `gocritic`, `revive`, `misspell`, `nolintlint`, `unconvert`, `unparam`, `gochecknoinits`, `forbidigo`。
- 对测试文件放宽必要规则，但不能掩盖错误处理与命名问题。
- 明确禁止 `init()` 和滥用 `fmt.Print*` 等不符合长期维护要求的写法。

**测试要求**:
- 先用最小示例验证配置文件能被 `golangci-lint` 正确加载。
- 验证 `gochecknoinits` 与 `errcheck` 等关键规则真正生效。
- 验证测试文件的排除或放宽规则符合预期，不出现误杀。

---

#### M0-T4: lefthook 配置

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T2, M0-T3 |
| 参考 | `technical-cli.md §7.2`；`AGENTS.md` |
| 产出 | `.lefthook.yml` |
| Commit | `build(hooks): add lefthook for pre-commit lint and commit-msg validation` |

**实现内容**:
- 配置 pre-commit 执行 `golangci-lint run --fix && go test ./...`。
- 配置 commit-msg 校验 Conventional Commits 正则。
- 提交信息长度限制与作用域语法遵循既定规范。
- 保持配置足够简单，不在 hook 层实现业务逻辑。
- 为后续团队协作提供默认 Git hook 框架基线。

**测试要求**:
- 先验证 hook 配置可被 lefthook 正确识别。
- 用合法和非法 commit message 样例验证正则边界。
- 验证 pre-commit 失败时会阻断提交，成功时不额外污染输出。

---

#### M0-T5: .gitignore 和 .editorconfig

| 字段 | 内容 |
|------|------|
| 依赖 | 无 |
| 参考 | `technical-overview.md §2.2`；`technical-operations.md §12.2(7)` |
| 产出 | `.gitignore`, `.editorconfig` |
| Commit | `chore(project): expand .gitignore and add .editorconfig` |

**实现内容**:
- 扩展现有 `.gitignore`，增加 Go 构建产物与本地编辑器目录。
- 增加 Argus local-only 目录：`.argus/pipelines/`, `.argus/logs/`, `.argus/tmp/`。
- 明确不忽略 `.argus/data/`，因为它是 Git-tracked 数据目录。
- 创建 `.editorconfig`，为 Go 文件使用 tab 缩进、UTF-8、LF。
- 保持编辑器规范尽量简洁，不覆盖已有语言工具默认最佳实践。

**测试要求**:
- 先用路径样例确认 `.gitignore` 条目命中预期文件。
- 验证 `.editorconfig` 可被常见编辑器识别。
- 验证 Argus 共享目录未被误加入忽略列表。

---

#### M0-T6: AGENTS.md 开发规范

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5 |
| 参考 | `AGENTS.md`；全部 `technical-*.md` |
| 产出 | `AGENTS.md` |
| Commit | `docs(agents): add development guidelines section` |

**实现内容**:
- 只追加新板块 `## Development Guidelines`，不得修改既有内容。
- 汇总标准工作流：读任务、读对应技术章节、先测试、后实现、再重构、最后提交。
- 写明 Go best practices：sentinel + custom error + `%w`、`slog`、table-driven tests、小接口、Go 1.24 能力。
- 写明反模式：禁止 `init()`、禁止全局可变状态、禁止业务逻辑中 `panic`、禁止滥用 `any`、禁止忽略 error。
- 写明包组织与依赖方向：`cmd -> internal/*`，并强调避免循环依赖。
- 写明测试和 commit 规范，给出 scope 示例。

**测试要求**:
- 先人工检查追加位置，确保不破坏原有 AGENTS 结构。
- 验证新增内容与技术文档不冲突。
- 验证新增规范可为后续 AI Agent 提供稳定实现约束。

---

#### M0 验收标准

```bash
make build
make test
make lint
./bin/argus
```

### M1: 基础设施与横切约束

**目标**: 完成 CLI 根命令、错误模型、ID 校验、时间戳、路径安全、版本兼容与统一输出等基础设施。完成后，所有上层模块都可以复用统一的横切能力，不再各自实现重复逻辑。

**NOT IN SCOPE**: 不解析 Workflow / Invariant YAML；不创建 pipeline 或 session 文件；不实现任何 install / hook / doctor 业务逻辑。

#### M1-T1: cobra CLI 框架搭建 + version/help 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5, M0-T6 |
| 参考 | `technical-cli.md §7.1`, `technical-cli.md §7.2` |
| 产出 | `cmd/argus/main.go`, `cmd/argus/root.go`, `cmd/argus/cmd_version.go` |
| Commit | `feat(cli): add cobra framework with version and help commands` |

**实现内容**:
- 引入 cobra，重构最小入口为根命令执行模式。
- 在根命令中区分 external commands 与 internal commands 的默认展示行为。
- 实现 `argus version`，从 ldflags 注入值读取版本号。
- 实现 `argus help --all` 展示所有命令的机制。
- 为后续子命令注册保留清晰的目录和初始化结构。

**测试要求**:
- 先写 CLI smoke tests，覆盖 `version`、`help`、`help --all`。
- 验证默认 help 不暴露内部命令。
- 验证 `--all` 能显示 internal commands 分组。

---

#### M1-T2: 错误类型与模式

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5, M0-T6 |
| 参考 | `technical-cli.md §7.6`；`AGENTS.md` |
| 产出 | `internal/core/errors.go`, `internal/core/errors_test.go` |
| Commit | `feat(core): add sentinel errors and ValidationError type` |

**实现内容**:
- 定义 sentinel errors：`ErrNotFound`, `ErrInvalidID`, `ErrVersionMismatch`, `ErrNoActivePipeline`, `ErrActivePipelineExists`。
- 定义 `ValidationError{Field, Message}` 并实现 `error` 接口。
- 固化推荐 wrapping 模式：`fmt.Errorf("loading workflow: %w", err)`。
- 保证该错误模型能同时服务 parser、store、CLI 和 install 模块。
- 不在此任务扩展过多业务错误类型，避免早期过度设计。

**测试要求**:
- 先写 `errors.Is` / `errors.As` 场景测试。
- 验证 `ValidationError` 可被识别并保留字段信息。
- 验证 wrapping 后 sentinel 仍可向上匹配。

---

#### M1-T3: ID 验证函数

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T2 |
| 参考 | `technical-overview.md §3.1`；`technical-workflow.md §2.1`；`technical-invariant.md §4.3` |
| 产出 | `internal/core/ids.go`, `internal/core/ids_test.go` |
| Commit | `feat(core): add ID validation functions` |

**实现内容**:
- 实现 `ValidateWorkflowID`，遵循 workflow / invariant 统一 ID 规范。
- 实现 `ValidateJobID`，遵循模板引擎要求的下划线命名规则。
- 实现 `ValidateSkillName`，遵循 Agent Skills 命名规范。
- 实现 `IsArgusReserved`，统一处理 `argus-` 保留前缀。
- 把长度限制、字符集限制和前缀限制收敛在 core 层。

**测试要求**:
- 先写 table-driven tests，覆盖 valid / invalid / edge cases。
- 验证空字符串、超长、非法字符、保留前缀等情况。
- 验证 Job ID 的字母开头与下划线规则被正确执行。

---

#### M1-T4: 时间戳工具

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5, M0-T6 |
| 参考 | `technical-overview.md §3.3` |
| 产出 | `internal/core/timestamp.go`, `internal/core/timestamp_test.go` |
| Commit | `feat(core): add compact UTC timestamp format/parse utilities` |

**实现内容**:
- 提供 `FormatTimestamp(t time.Time) string`。
- 提供 `ParseTimestamp(s string) (time.Time, error)`。
- 固定输出 `YYYYMMDDTHHMMSSZ` compact UTC 格式。
- 为 pipeline、session、log、toolbox 共享使用场景打通公共实现。
- 避免各模块手写格式字符串导致漂移。

**测试要求**:
- 先写 round-trip 测试：format 后 parse 应等价。
- 验证非 UTC 输入会被规范化为 UTC 输出。
- 验证非法字符串会返回可识别错误。

---

#### M1-T5: 路径安全工具

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T3, M1-T4 |
| 参考 | `technical-overview.md §1.2(7)`；`technical-operations.md §13.1` |
| 产出 | `internal/core/paths.go`, `internal/core/paths_test.go` |
| Commit | `feat(core): add path safety utilities with traversal protection` |

**实现内容**:
- 实现 `SessionIDToSafeID(sessionID string) string`，UUID 直接透传，非 UUID 做 SHA256 截断。
- 实现 `ValidatePath(base, target string) error`，通过 `filepath.Rel` 做目录逃逸校验。
- 固化 pipeline / session / invariant 三类文件路径构造规则。
- 为后续 store、install、doctor 提供统一安全入口。
- 特别覆盖路径遍历、空路径、相对路径混用等边界情形。

**测试要求**:
- 先写 UUID 直通与 hash 回退测试。
- 验证 `../` 等路径逃逸尝试会失败。
- 验证合法目标路径不会被误判为非法。

---

#### M1-T6: 版本兼容性检查

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T2 |
| 参考 | `technical-overview.md §3.2` |
| 产出 | `internal/core/version.go`, `internal/core/version_test.go` |
| Commit | `feat(core): add schema version compatibility checker` |

**实现内容**:
- 定义 `const SchemaVersion = "v0.1.0"`。
- 实现 `CheckCompatibility(fileVersion string) error`，按 major version 匹配。
- 处理 malformed version 输入，不允许 silently accept。
- 保持实现足够通用，供 workflow / invariant / pipeline 三类文件复用。
- 明确该模块做 schema 兼容，不做 CLI 自身版本升级策略。

**测试要求**:
- 先写 compatible / incompatible / malformed 三类测试。
- 验证 `v0.x.y` 兼容规则稳定。
- 验证 major mismatch 返回 `ErrVersionMismatch`。

---

#### M1-T7: 输出格式化工具

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T2 |
| 参考 | `technical-cli.md §7.6`；`technical-cli.md §8` |
| 产出 | `internal/core/output.go`, `internal/core/output_test.go` |
| Commit | `feat(core): add unified JSON envelope and output formatting utilities` |

**实现内容**:
- 实现 `OKEnvelope(data any) ([]byte, error)`。
- 实现 `ErrorEnvelope(msg string) ([]byte, error)`。
- 实现 `WriteJSON(w io.Writer, data any)` 等输出辅助方法。
- 为未来支持 `--markdown` 的命令预留渲染辅助结构。
- 统一所有内部命令的 envelope 语义，避免每个命令自定义顶层 JSON。

**测试要求**:
- 先写 JSON 序列化测试，验证 `status` 字段位置与内容。
- 验证错误 envelope 字段最小且稳定。
- 验证传入 struct / map 时不会丢失关键字段。

---

#### M1 验收标准

```bash
make build && ./bin/argus version
./bin/argus help
./bin/argus help --all
go test ./internal/core/...
make lint
```

### M2: Schema、Parser、Store 与静态校验

**目标**: 完成 Workflow / Invariant / Pipeline / Session / Workspace / Toolbox 的基础数据结构、解析器、校验器和持久化能力。完成后，Argus 具备“能读定义、能写运行态、能做静态检查”的底层能力，但还未进入完整编排闭环。

**NOT IN SCOPE**: 不启动 workflow；不推进 job 状态机；不运行 invariant shell check；不处理 tick / trap / install / doctor 主流程。

#### M2-T1: Workflow YAML Schema & Parser

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-workflow.md §2` |
| 产出 | `internal/workflow/schema.go`, `internal/workflow/parser.go`, `internal/workflow/parser_test.go` |
| Commit | `feat(workflow): add YAML schema definition and parser` |

**实现内容**:
- 定义 `Workflow` 与 `Job` 结构体，覆盖文件级与 job 级字段。
- 用 `gopkg.in/yaml.v3` 实现严格 parser。
- 打开 KnownFields strict mode，拒绝 unknown keys。
- 检查 required fields 与 `prompt` / `skill` 不能同时为空的基础规则。
- 保持 parser 只负责单文件解析与基础字段校验，不提前承担跨文件 inspect 职责。

**测试要求**:
- 先写有效 YAML fixture，再逐步补错误案例。
- 覆盖缺失 `id`、缺失 `jobs`、空 job 列表、unknown key、prompt+skill 同空。
- 验证错误返回中保留足够字段上下文，方便 CLI 层展示。

---

#### M2-T2: _shared.yaml & Ref 解析

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T1 |
| 参考 | `technical-workflow.md §4` |
| 产出 | `internal/workflow/ref.go`, `internal/workflow/ref_test.go` |
| Commit | `feat(workflow): add _shared.yaml support with shallow merge ref resolution` |

**实现内容**:
- 定义 shared jobs 结构体与加载逻辑。
- 实现 `LoadShared(path)` 与 `ResolveRef(job, shared)`。
- 严格落实浅合并语义：缺失字段继承、显式值覆盖、`null` 或空字符串视为清空。
- 明确 `id` 继承 / 重命名行为。
- 不把 ref 展开逻辑耦合进 parser，以便 inspect 与 runtime 复用。

**测试要求**:
- 先写 `ref found` 正向测试。
- 覆盖 `ref not found`、prompt 覆盖、skill 继承、null 清空、id 继承。
- 验证共享 job key 命名限制与 job id 兼容。

---

#### M2-T3: Workflow Inspect 校验逻辑

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T2 |
| 参考 | `technical-workflow.md §7`；`technical-workflow.md §5.1` |
| 产出 | `internal/workflow/validate.go`, `internal/workflow/validate_test.go` |
| Commit | `feat(workflow): add multi-file inspect validation with 8 checks` |

**实现内容**:
- 实现 `InspectDirectory(dir string)`，以目录为单位检查 workflow 集合。
- 覆盖 8 项检查：YAML 语法、必填字段、重复 ID、ref 存在、unknown keys、模板语法、版本兼容、ID/namespace 规则。
- 识别 `_shared.yaml` 的特殊地位，不把它当作普通 workflow 文件输出。
- 模板检查只做 parse，不做 render。
- 输出结构要便于 CLI 原样包装成 JSON / Markdown。

**测试要求**:
- 先按检查项拆 fixture，避免单 fixture 同时覆盖过多错误。
- 覆盖跨文件 duplicate IDs 与 ref 跨文件解析。
- 验证 inspect 失败时不会中断整个目录扫描，而是汇总结果。

---

#### M2-T4: workflow inspect CLI 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T3 |
| 参考 | `technical-cli.md §7.3`；`technical-workflow.md §7.1-§7.3` |
| 产出 | `cmd/argus/cmd_workflow_inspect.go` |
| Commit | `feat(cli): add workflow inspect command` |

**实现内容**:
- 注册 `argus workflow inspect [dir] [--markdown]`。
- 默认目录为 `.argus/workflows/`。
- JSON 输出遵循统一 envelope。
- `--markdown` 输出为人类可读摘要，不要求完全复刻 JSON 字段名。
- 命令只报告检查结果，不自动修复文件。

**测试要求**:
- 先写命令级测试，覆盖默认目录和显式目录两种调用。
- 验证 JSON / Markdown 双输出路径。
- 验证校验失败时退出码与 envelope 语义一致。

---

#### M2-T5: Invariant YAML Schema & Parser

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-invariant.md §4.3`；`technical-invariant.md §4.7` |
| 产出 | `internal/invariant/schema.go`, `internal/invariant/parser.go`, `internal/invariant/validate.go`, `internal/invariant/schema_test.go`, `internal/invariant/parser_test.go`, `internal/invariant/validate_test.go` |
| Commit | `feat(invariant): add YAML schema, parser, and 10-check validator` |

**实现内容**:
- 定义 `Invariant` 与 `CheckStep` 结构体。
- 实现 parser 与目录级 validator。
- 覆盖 10 项校验：语法、必填字段、unknown keys、auto enum、跨文件重复 ID、namespace、workflow 引用、版本兼容、ID 格式、check 非空。
- 允许 `prompt` 和 `workflow` 共存，但不允许同时为空。
- 为后续 runtime shell runner 保留 step 级 description 和 shell 原文。

**测试要求**:
- 先写正常案例，再覆盖各类非法 auto、空 check、重复 ID。
- 验证 workflow 引用检查逻辑可独立工作。
- 验证 `never` / `always` / `session_start` 三种 auto 值都被正确识别。

---

#### M2-T6: invariant inspect CLI 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T5, M2-T1 |
| 参考 | `technical-cli.md §7.3`；`technical-invariant.md §4.7` |
| 产出 | `cmd/argus/cmd_invariant_inspect.go` |
| Commit | `feat(cli): add invariant inspect command` |

**实现内容**:
- 注册 `argus invariant inspect [dir] [--markdown]`。
- 默认目录为 `.argus/invariants/`。
- 维持 workflow 引用始终指向当前项目 `.argus/workflows/` 的约束。
- JSON 与 Markdown 输出行为与 workflow inspect 保持风格一致。
- 保持命令职责为静态检查，不引入 runtime shell 执行。

**测试要求**:
- 先写跨目录引用测试，确保 `[dir]` 不影响 workflow 查找根。
- 验证输出 envelope 与退出码。
- 验证 invalid invariant 目录也能产出结构化结果。

---

#### M2-T7: Pipeline 数据文件 Schema & Store

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-pipeline.md §5.1`；`technical-pipeline.md §5.7` |
| 产出 | `internal/pipeline/schema.go`, `internal/pipeline/store.go`, `internal/pipeline/schema_test.go`, `internal/pipeline/store_test.go` |
| Commit | `feat(pipeline): add pipeline data schema and YAML store` |

**实现内容**:
- 定义 `Pipeline` 与 `JobData` 结构。
- 实现 `NewInstanceID`, `LoadPipeline`, `SavePipeline`, `ScanActivePipelines`。
- `pipelines/` 目录缺失时自动创建。
- 扫描时跳过损坏 YAML，并把 warning 返回给调用方。
- 统一 instance ID 与文件路径的区别：逻辑 ID 不带 `.yaml`。

**测试要求**:
- 先写 create / load / save round-trip 测试。
- 覆盖 active pipeline 扫描与 corrupt file skip。
- 验证 instance ID 格式稳定且带 compact UTC 时间戳。

---

#### M2-T8: Session 数据文件 Schema & Store

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-pipeline.md §6` |
| 产出 | `internal/session/schema.go`, `internal/session/store.go`, `internal/session/schema_test.go`, `internal/session/store_test.go` |
| Commit | `feat(session): add session data schema and store` |

**实现内容**:
- 定义 `Session` 与 `LastTickState` 结构体。
- 实现 `LoadSession`, `SaveSession`, `SessionExists`。
- 自动创建 `/tmp/argus/` 目录。
- 使用 safe-id 路径规则，统一所有 session 文件读写位置。
- 保持 session store 只做存取，不做业务状态演算。

**测试要求**:
- 先写 session 不存在场景测试。
- 覆盖新建、读回、覆写与 safe-id 路径分支。
- 验证 session 文件内容与 schema 一致。

---

#### M2-T9: Workspace 配置 Schema & 路径工具

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-workspace.md §10.2` |
| 产出 | `internal/workspace/config.go`, `internal/workspace/paths.go`, `internal/workspace/config_test.go`, `internal/workspace/paths_test.go` |
| Commit | `feat(workspace): add config schema and 4-step path normalization` |

**实现内容**:
- 定义 `Config{Workspaces []string}`。
- 实现 4 步路径规范化算法：绝对化、clean、`$HOME -> ~`、存储。
- 实现 `LoadConfig`, `SaveConfig`, `IsInWorkspace`。
- 使用路径分段前缀匹配，避免 `/work/co` 误匹配 `/work/company`。
- 处理重复路径去重与嵌套 workspace 的稳定存储。

**测试要求**:
- 先按输入形式写 table-driven tests：相对路径、绝对路径、`~`、尾斜杠。
- 覆盖 prefix mismatch 反例。
- 验证去重后配置顺序和存储结果可预测。

---

#### M2-T10: toolbox 子命令

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-cli.md §7.4.1`；`technical-overview.md §3.3` |
| 产出 | `internal/toolbox/jq.go`, `internal/toolbox/yq.go`, `internal/toolbox/timestamp.go`, `internal/toolbox/sha256.go`, `cmd/argus/cmd_toolbox.go`, `internal/toolbox/toolbox_test.go` |
| Commit | `feat(toolbox): add jq/yq/touch-timestamp/sha256sum builtins` |

**实现内容**:
- 集成 `itchyny/gojq` 与 `mikefarah/yq`，作为 stdlib-first 的受控例外。
- 实现 `touch-timestamp`，写入 compact UTC 时间戳。
- 实现 `sha256sum`，兼容 coreutils 输出格式。
- 注册 `argus toolbox <tool> [args]` 并透传 stdout / stderr / exit code。
- 保持工具子命令职责单一，不附加 Argus 业务 envelope。

**测试要求**:
- 先为 `touch-timestamp` 与 `sha256sum` 编写稳定测试。
- 对 `jq` / `yq` 至少准备一个基本解析样例。
- 验证错误时 exit code 透传而不是被吞掉。

---

#### M2 验收标准

```bash
go test ./internal/workflow/... ./internal/invariant/... ./internal/pipeline/... ./internal/session/... ./internal/workspace/...
./bin/argus workflow inspect /path/to/test/workflows
./bin/argus invariant inspect /path/to/test/invariants
./bin/argus toolbox touch-timestamp /tmp/test-ts
make lint
```

### M3: Workflow 执行闭环

**目标**: 完成模板渲染、pipeline 状态机、workflow start、job-done、workflow list/cancel 与 status 的 pipeline 部分。完成后，系统首次具备“定义 workflow → 启动 → 顺序推进 → 完成 / 失败 / 取消 → 查询状态”的完整闭环。

**NOT IN SCOPE**: 不运行 invariant shell check；不处理 session_start 行为；不处理 tick / trap Hook；不实现 install / uninstall。

#### M3-T1: 模板引擎

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T1, M2-T7 |
| 参考 | `technical-workflow.md §5`；`technical-operations.md §13.3` |
| 产出 | `internal/workflow/template.go`, `internal/workflow/template_test.go` |
| Commit | `feat(workflow): add template engine with partial substitution strategy` |

**实现内容**:
- 定义 `TemplateContext`，覆盖 `workflow`, `job`, `pre_job`, `git`, `project`, `env`, `jobs` 字段。
- 实现 `RenderPrompt(prompt, ctx)`。
- 实现 `BuildContext(pipeline, workflow, jobIdx)`。
- 落实“部分替换”策略：已知变量替换、未知变量保留原始占位符、stderr 输出 warning。
- 明确处理 `jobs.<id>.message` 不存在、前序 job 为空、首个 job 没有 `pre_job` 等边界。

**测试要求**:
- 先写已知变量替换成功案例。
- 覆盖未知变量保留原文、git 分支填充、环境变量透传、前驱 job message 读取。
- 验证缺失 `jobs` 引用不会导致模板执行失败。

---

#### M3-T2: Pipeline 状态机

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T1, M2-T7 |
| 参考 | `technical-pipeline.md §5.1-§5.6`；`technical-pipeline.md §5.4.1` |
| 产出 | `internal/pipeline/engine.go`, `internal/pipeline/engine_test.go` |
| Commit | `feat(pipeline): add pipeline state machine with 9 transitions` |

**实现内容**:
- 实现 `CreatePipeline`，包括单活跃检查和首个 job 初始化。
- 实现 `AdvanceJob`，覆盖成功推进、完成、失败、提前结束等场景。
- 实现 `CancelPipeline` 与 `DeriveJobStatus`。
- 落实运行中 workflow 被修改时的统一检测逻辑。
- 严格依据状态迁移表更新 `status`, `current_job`, `ended_at`, `jobs.<id>` 字段。

**测试要求**:
- 先按状态迁移表逐项写测试，一项操作对应一条断言集合。
- 覆盖单活跃约束与异常状态处理。
- 验证 workflow 修改导致的 `current_job` 缺失能被正确识别。

---

#### M3-T3: workflow start 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M3-T1, M3-T2 |
| 参考 | `technical-cli.md §8.2`；`technical-pipeline.md §5.2` |
| 产出 | `cmd/argus/cmd_workflow_start.go` |
| Commit | `feat(cli): add workflow start command` |

**实现内容**:
- 注册 `argus workflow start <workflow-id>`。
- 从 `.argus/workflows/<id>.yaml` 读取定义并创建 pipeline。
- 在启动成功后渲染首个 job prompt。
- JSON / Markdown 输出与文档示例保持一致语义。
- 对已有活跃 pipeline 直接返回错误，不尝试自动取消或替换。

**测试要求**:
- 先写命令级测试，覆盖正常启动和已有活跃 pipeline 场景。
- 验证输出中的 progress、next_job、pipeline_status。
- 验证首个 job 的模板变量在启动时已被渲染。

---

#### M3-T4: job-done 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M3-T1, M3-T2 |
| 参考 | `technical-cli.md §8.3`；`technical-pipeline.md §5.3-§5.4` |
| 产出 | `cmd/argus/cmd_job_done.go` |
| Commit | `feat(cli): add job-done command with 6 completion scenarios` |

**实现内容**:
- 注册 `argus job-done [--fail] [--end-pipeline] [--message "..."] [--markdown]`。
- 实现 6 种返回场景：成功推进、成功完成、提前结束、失败、无活跃 pipeline、提前失败结束。
- 成功推进时返回下一 job 的渲染结果。
- 无活跃 pipeline 时返回 error envelope，exit 1。
- 与状态机共享逻辑，不在命令层手写状态更新。

**测试要求**:
- 先按 6 个场景各写一组测试。
- 验证 message 落库与 output schema 一致。
- 验证 `--fail` 与 `--end-pipeline` 组合行为符合规范。

---

#### M3-T5: workflow list / cancel 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M3-T2 |
| 参考 | `technical-cli.md §7.3`；`technical-pipeline.md §5.6` |
| 产出 | `cmd/argus/cmd_workflow_list.go`, `cmd/argus/cmd_workflow_cancel.go` |
| Commit | `feat(cli): add workflow list and cancel commands` |

**实现内容**:
- `workflow list` 读取 `.argus/workflows/` 并列出所有可用 workflow。
- `workflow cancel` 取消当前活跃 pipeline。
- 异常状态下若存在多个 running pipeline，则全部取消。
- 无活跃 pipeline 时返回 error envelope，exit 1。
- 保持 cancel 是外部控制动作，不修改当前 job 的 message / ended_at 记录。

**测试要求**:
- 先写 list 的目录扫描测试，忽略 `_shared.yaml`。
- 覆盖正常 cancel、多个 running cancel-all、无活跃 pipeline 报错。
- 验证取消后 pipeline 状态与结束时间更新正确。

---

#### M3-T6: status 命令 (pipeline part)

| 字段 | 内容 |
|------|------|
| 依赖 | M3-T2 |
| 参考 | `technical-cli.md §8.4`；`technical-pipeline.md §5.1` |
| 产出 | `cmd/argus/cmd_status.go` |
| Commit | `feat(cli): add status command with pipeline state display` |

**实现内容**:
- 注册 `argus status [--markdown]`。
- 先只实现 pipeline 部分，invariants 字段先返回空结构或空数组占位。
- 通过 workflow 定义 + pipeline 数据推导所有 job 的 completed / in_progress / pending 状态。
- 处理 workflow 被修改导致 current_job 丢失的 best-effort 场景。
- 统一 hints[] 字段，为后续 invariant 和 doctor 提示预留位置。

**测试要求**:
- 先写有活跃 pipeline 和无活跃 pipeline 两类输出测试。
- 覆盖 workflow 修改后的 best-effort 输出。
- 验证 JSON / Markdown 都能稳定展示 job 列表顺序。

---

#### M3 验收标准

```bash
./bin/argus workflow start test-workflow
./bin/argus status
./bin/argus job-done --message "step 1"
./bin/argus job-done --end-pipeline
./bin/argus workflow start test-workflow
./bin/argus workflow cancel
go test ./internal/pipeline/... ./internal/workflow/...
make lint
```

### M4: Invariant 运行与 Session 行为

**目标**: 完成 invariant shell runner、session manager、invariant check/list、workflow snooze，以及 status 的 invariant 实时展示。完成后，Argus 具备“检测声明式约束并把结果反馈给命令调用者”的能力，同时具备 session 级 snooze 与首次 tick 状态跟踪所需的数据能力。

**NOT IN SCOPE**: 不实现 Hook 输入解析与 tick 主流程；不处理 install / uninstall；不做 doctor 汇总。

#### M4-T1: Invariant Shell 检查引擎

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T5 |
| 参考 | `technical-invariant.md §4.4` |
| 产出 | `internal/invariant/runner.go`, `internal/invariant/runner_test.go` |
| Commit | `feat(invariant): add shell check runner with 5s timeout and short-circuit` |

**实现内容**:
- 实现 `RunCheck(inv, projectRoot)`。
- 使用 `/usr/bin/env bash -c "<script>"` 执行，保持 non-login / non-interactive 语义。
- 单 step 5 秒超时，前序失败后后续 step 标记 `skip`。
- 捕获 stdout / stderr 作为失败诊断。
- 注入 `ARGUS_PROJECT_ROOT` 环境变量，并记录总耗时超过 2 秒的 warning 信息。

**测试要求**:
- 先写 pass / fail / timeout 三类测试。
- 覆盖短路行为，确保 fail 后后续步骤不执行。
- 验证 shell 脚本内可读取 `ARGUS_PROJECT_ROOT`。

---

#### M4-T2: Session 管理器

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T8, M2-T7 |
| 参考 | `technical-pipeline.md §5.5`；`technical-pipeline.md §5.9`；`technical-pipeline.md §6` |
| 产出 | `internal/session/manager.go`, `internal/session/manager_test.go` |
| Commit | `feat(session): add session manager for snooze and tick state tracking` |

**实现内容**:
- 实现 `IsFirstTick`, `AddSnooze`, `IsSnoozed`, `SnoozeAll`, `UpdateLastTick`, `HasStateChanged`。
- 用 session 文件存在性表达 `session_start` 是否已检查。
- 明确 session 文件创建时机：在 invariant 检查完成之后。
- 对多个 running pipeline 异常状态支持 snooze-all。
- 把 state change 判断逻辑固定在 session 层，避免 tick 自己拼装比较。

**测试要求**:
- 先写 first tick 检测与 session file 不存在场景。
- 覆盖 snooze 单个 / 全部、last_tick 更新与状态变更判断。
- 验证“检查完成后才创建 session 文件”的时序要求。

---

#### M4-T3: invariant check / list 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M4-T1 |
| 参考 | `technical-cli.md §7.3`；`technical-invariant.md §4.9` |
| 产出 | `cmd/argus/cmd_invariant_check.go`, `cmd/argus/cmd_invariant_list.go` |
| Commit | `feat(cli): add invariant check and list commands` |

**实现内容**:
- 实现 `argus invariant check [id]`，支持运行全部或单个 invariant。
- 失败时输出关联 `workflow` / `prompt` 建议。
- 实现 `argus invariant list`，列出当前项目定义的 invariants。
- JSON 输出遵循 envelope，不在命令层自动触发修复 workflow。
- 对 `auto: never` 的 invariant，手动检查时也必须可运行。

**测试要求**:
- 先写 check all / check one 的命令测试。
- 覆盖 check fail 时建议信息输出。
- 验证 list 输出与目录中的定义一致。

---

#### M4-T4: workflow snooze 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M4-T2, M3-T2 |
| 参考 | `technical-cli.md §7.3`；`technical-pipeline.md §5.5`；`technical-pipeline.md §5.6` |
| 产出 | `cmd/argus/cmd_workflow_snooze.go` |
| Commit | `feat(cli): add workflow snooze command for session-level pipeline suppression` |

**实现内容**:
- 注册 `argus workflow snooze --session <id>`。
- 将当前活跃 pipeline 写入 session 的 `snoozed_pipelines`。
- 无活跃 pipeline 时返回 error envelope。
- 多个 running pipeline 异常状态时执行 snooze-all。
- 保持 snooze 只影响当前 session，不修改 pipeline 自身状态。

**测试要求**:
- 先写正常 snooze 路径测试。
- 覆盖无活跃 pipeline 报错与多个 running pipeline 异常状态。
- 验证相同 pipeline 重复 snooze 不会造成不稳定数据。

---

#### M4-T5: status 命令补充 Invariant 部分

| 字段 | 内容 |
|------|------|
| 依赖 | M4-T1, M3-T6 |
| 参考 | `technical-cli.md §8.4` |
| 产出 | `cmd/argus/cmd_status.go` |
| Commit | `feat(cli): add real-time invariant results to status command` |

**实现内容**:
- 扩展 `status`，实时运行所有 `auto != never` 的 invariants。
- 填充 `invariants.passed`, `invariants.failed`, `invariants.details[]`。
- 将慢检查 warning 通过 `hints[]` 暴露。
- 保持 pipeline 部分逻辑不回退、不重复实现。
- status 仍然只做展示，不启动 workflow 修复流程。

**测试要求**:
- 先写全部通过、部分失败、无活跃 pipeline 三类状态测试。
- 覆盖 invariant 无 description 时的 fallback 行为。
- 验证慢检查 warning 会进入 hints[]。

---

#### M4 验收标准

```bash
./bin/argus invariant check
./bin/argus invariant check test-inv
./bin/argus invariant list
./bin/argus status
go test ./internal/invariant/... ./internal/session/...
make lint
```

### M5: Hook 基础设施与 tick/trap 主流程

**目标**: 完成项目根发现、Hook stdin 解析、Hook 文本输出、tick 主流程、trap 占位实现与 Hook 日志。完成后，Argus 可以被 Claude Code / Codex / OpenCode 的 Hook 层稳定调用，并根据项目状态向 Agent 注入统一文本上下文。

**NOT IN SCOPE**: 不实现 trap 规则引擎；不安装 Hook 配置；不释放内置资源；不执行 doctor 汇总诊断。

#### M5-T1: 项目根发现

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-workspace.md §10.3.1` |
| 产出 | `internal/workspace/project.go`, `internal/workspace/project_test.go` |
| Commit | `feat(workspace): add project root discovery with .argus/.git upward traversal` |

**实现内容**:
- 实现 `FindProjectRoot(cwd)`，优先向上查找 `.argus/`，找不到再查 `.git/`。
- 定义 `ProjectRoot{Path, HasArgus, HasGit}`。
- 实现 `IsSubdirectory` 与 `IsGitRoot` 等辅助函数。
- 为 global tick 场景保留“非 Git 目录静默跳过”的判断基础。
- 明确 `.argus/` 优先级高于 `.git/`。

**测试要求**:
- 先写 `.argus/` 命中、`.git/` fallback、都不存在三类测试。
- 覆盖子目录与仓库根目录两种路径关系。
- 验证路径比较不会误判相似前缀目录。

---

#### M5-T2: tick 输入解析

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-hooks.md §9.2`；`technical-pipeline.md §6.3` |
| 产出 | `internal/hook/input.go`, `internal/hook/input_test.go` |
| Commit | `feat(hook): add multi-agent stdin JSON parser with sub-agent detection` |

**实现内容**:
- 定义统一的 `AgentInput`。
- 实现 `ParseInput(r, agent)`，映射 claude-code、codex、opencode 的字段差异。
- 实现 `IsSubAgent(input)`，基于 `agent_id` 或 `parentID` 检测子 agent。
- 把 Agent 差异屏蔽在输入层，后续 tick 逻辑只依赖统一结构。
- 保持 Codex 当前无法检测子 agent 的已知限制。

**测试要求**:
- 先按 agent 类型写三组解析测试。
- 覆盖 OpenCode `parentID` 与 Claude Code `agent_id` 的子 agent 检测。
- 验证缺失字段时返回可诊断错误，而不是 silent zero-value。

---

#### M5-T3: tick 输出格式化

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-cli.md §8.1` |
| 产出 | `internal/hook/output.go`, `internal/hook/output_test.go` |
| Commit | `feat(hook): add tick output formatters for 5 scenarios` |

**实现内容**:
- 实现 `FormatNoPipeline`, `FormatFullContext`, `FormatMinimalSummary`, `FormatSnoozed`, `AppendInvariantFailed`。
- 输出统一使用 Markdown text。
- 形成 5 种场景的文本模板语义基线。
- 兼容后续接入运行时 prompt 模板文件时的内容结构。
- 强调 tick 输出是文本，不是 JSON。

**测试要求**:
- 先为 5 种场景分别写 snapshot 风格测试。
- 覆盖 invariant failure 追加内容。
- 验证输出中命令示例与文档约定一致。

---

#### M5-T4: tick 命令核心逻辑

| 字段 | 内容 |
|------|------|
| 依赖 | M5-T1, M5-T2, M5-T3, M4-T2, M4-T1, M3-T2, M3-T1 |
| 参考 | `technical-hooks.md §9.2`；`technical-pipeline.md §5.9`；`technical-workspace.md §10.3.1` |
| 产出 | `cmd/argus/cmd_tick.go`, `internal/hook/tick.go`, `internal/hook/tick_test.go` |
| Commit | `feat(cli): add tick command as passive workflow orchestration hook` |

**实现内容**:
- 实现 `HandleTick(agent, global, stdin)` 主流程。
- 流程包括：解析输入、跳过子 agent、定位项目根、读取 pipeline / session、首次 tick 执行 `session_start` 检查、创建 session 文件、选择注入策略、更新 `last_tick`。
- 始终 exit 0，内部错误只输出警告文本。
- 正确处理 5 种 tick 场景与 snooze 优先级。
- 保持逻辑集中在 Go 内部，Hook wrapper 不承载业务判断。

**测试要求**:
- 先写 5 种输出场景测试，再补错误路径测试。
- 覆盖子 agent 跳过、异常状态 fail-open、session 首次进入创建时机。
- 验证多个 running pipeline 且全部 snoozed 时会静默跳过。

---

#### M5-T5: trap 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-hooks.md §9.3` |
| 产出 | `cmd/argus/cmd_trap.go` |
| Commit | `feat(cli): add trap command (Phase 1: always allow)` |

**实现内容**:
- 注册 `argus trap --agent <name> [--global]`。
- Phase 1 固定返回 `permissionDecision: allow` 的 JSON。
- 命令始终 exit 0。
- 保留统一输出结构，便于后续扩展 deny / ask 决策。
- 不在本阶段实现任何工具调用拦截规则。

**测试要求**:
- 先写输出 JSON 结构测试。
- 验证不同 agent 参数不会改变 Phase 1 放行结果。
- 验证命令在空 stdin 下也能安全返回。

---

#### M5-T6: Hook 日志管理

| 字段 | 内容 |
|------|------|
| 依赖 | M5-T1 |
| 参考 | `technical-hooks.md §9.5`；`technical-overview.md §3.3` |
| 产出 | `internal/hook/logger.go`, `internal/hook/logger_test.go` |
| Commit | `feat(hook): add hook execution logger with project/global fallback` |

**实现内容**:
- 实现 `LogHookExecution(projectRoot, command, success, details)`。
- 日志格式固定为 `{COMPACT_UTC} [{COMMAND}] {OK|ERROR} {DETAILS}`。
- 优先写项目级 `.argus/logs/hook.log`，不存在则 fallback 到 `~/.config/argus/logs/hook.log`。
- 只把 wrapper / execution 级失败记为 ERROR。
- 为 Doctor 提供稳定可读的原始诊断数据。

**测试要求**:
- 先写项目级日志写入测试。
- 覆盖 fallback 路径、ERROR / OK 判定、时间戳格式。
- 验证目录不存在时能自动创建必要父目录。

---

#### M5 验收标准

```bash
echo '{"session_id":"abc-123","cwd":"/tmp"}' | ./bin/argus tick --agent claude-code
echo '{"sessionID":"abc","parentID":"parent-abc"}' | ./bin/argus tick --agent opencode
echo '{}' | ./bin/argus trap --agent claude-code
go test ./internal/hook/... ./internal/workspace/...
make lint
```

### M6: 项目级安装与内置资源释出

**目标**: 完成嵌入资源目录、内置 workflow / invariant / skills / prompts、hook 模板、Agent hook 配置安装器，以及项目级 install / uninstall。完成后，Argus 可以把自身 Phase 1 所需的资源和配置幂等地安装到一个 Git 项目中。

**NOT IN SCOPE**: 不实现 workspace 全局安装；不实现 doctor；不扩展 OpenCode 实验性 hooks；不实现 trap 规则引擎。

#### M6-T1: 嵌入资源目录 & go:embed 配置

| 字段 | 内容 |
|------|------|
| 依赖 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| 参考 | `technical-overview.md §2.3` |
| 产出 | `assets/skills/`, `assets/workflows/`, `assets/invariants/`, `assets/prompts/`, `assets/hooks/`, `internal/assets/embed.go` |
| Commit | `build(assets): add embedded assets directory with go:embed configuration` |

**实现内容**:
- 建立 `assets/` 目录结构。
- 在 `internal/assets/embed.go` 中配置 `//go:embed`。
- 提供 `ReadAsset` 与 `ListAssets` 等辅助函数。
- 统一内置资源读取入口，避免 install 模块直接拼 embed 路径。
- 为后续 workflow / invariant / skills / prompts 释出打底。

**测试要求**:
- 先写 embed smoke test，验证资源可列举、可读取。
- 覆盖不存在资源路径的错误返回。
- 验证资源目录命名与技术文档一致。

---

#### M6-T2: 内置 Workflow / Invariant 定义

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T1 |
| 参考 | `technical-invariant.md §4.8` |
| 产出 | `assets/workflows/argus-init.yaml`, `assets/invariants/argus-init.yaml` |
| Commit | `feat(assets): add argus-init built-in workflow and invariant definitions` |

**实现内容**:
- 按 `technical-invariant.md §4.8` 的 canonical 定义，逐字落实 `argus-init` workflow。
- 同样按 canonical 定义编写 `argus-init` invariant。
- 不做二次解释或“优化版”重写，避免与规范漂移。
- 确保 workflow 的 job 顺序、mark 文件路径、gitignore 检查条目与文档一致。
- 确保 invariant 的 7 个 check step 顺序和描述与文档一致。

**测试要求**:
- 先做静态 fixture 校验，保证 YAML 可被 parser 读入。
- 用 workflow / invariant inspect 验证内置定义通过。
- 验证 `argus-` 前缀与 canonical 内容未被误改。

---

#### M6-T3: 内置 Skill 文件

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T1 |
| 参考 | `technical-workspace.md §11.2-§11.4` |
| 产出 | `assets/skills/argus-install/SKILL.md`, `assets/skills/argus-uninstall/SKILL.md`, `assets/skills/argus-doctor/SKILL.md`, `assets/skills/argus-status/SKILL.md`, `assets/skills/argus-workflow/SKILL.md`, `assets/skills/argus-invariant-check/SKILL.md`, `assets/skills/argus-generate-rules/SKILL.md`, `assets/skills/argus-concepts/SKILL.md`, `assets/skills/argus-workflow-syntax/SKILL.md` |
| Commit | `feat(assets): add 9 built-in skill SKILL.md files` |

**实现内容**:
- 为 9 个内置 skills 编写 `SKILL.md`。
- 每个文件都包含 YAML frontmatter：`name`, `description`, `version`。
- 内容按“独立型 / 依赖型 / 参考型 / 任务配套型”角色组织，避免混淆定位。
- 目录名必须与 `name` 字段一致。
- 只写 skill 指南，不在这里复制技术规范全文。

**测试要求**:
- 先检查目录名与 frontmatter 一致。
- 验证所有 skill 名称符合命名规则并使用 `argus-` 前缀。
- 用 install 侧的释出逻辑 smoke test 验证文件可被正确复制。

---

#### M6-T4: Prompt 模板

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T1 |
| 参考 | `technical-cli.md §8.1`；`technical-workspace.md §10.4` |
| 产出 | `assets/prompts/tick-no-pipeline.md.tmpl`, `assets/prompts/tick-full-context.md.tmpl`, `assets/prompts/tick-minimal.md.tmpl`, `assets/prompts/tick-invariant-failed.md.tmpl`, `assets/prompts/workspace-guide.md.tmpl` |
| Commit | `feat(assets): add tick and workspace prompt templates` |

**实现内容**:
- 将 tick 各场景与 workspace install 引导文本沉淀为模板文件。
- 使用 Go `text/template` 语法，变量命名与运行时上下文对齐。
- 保证模板只负责文案组织，不携带业务逻辑。
- 未来如需多语言或文案演进，可在不改主逻辑的前提下替换模板。
- 与 `internal/hook/output.go` 的文本结构保持兼容。

**测试要求**:
- 先渲染至少一组样例数据验证模板可执行。
- 覆盖空列表、空提示、失败建议追加等场景。
- 验证模板文件路径可被 embed 读取。

---

#### M6-T5: Hook Wrapper 脚本

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T1 |
| 参考 | `technical-hooks.md §9.2`；`technical-hooks.md §9.3`；`technical-hooks.md §9.4` |
| 产出 | `internal/install/hook_templates.go` |
| Commit | `feat(install): add hook wrapper templates for claude-code/codex/opencode` |

**实现内容**:
- 将 Claude Code、Codex、OpenCode 的 Hook 配置模板写成 Go 字符串或模板。
- 模板中包含 `argus tick` / `argus trap` 命令、超时设置、必要的 agent 参数。
- OpenCode 模板只覆盖 `chat.message` 与 `tool.execute.before` 两个 Phase 1 hook。
- 不把这些 wrapper 作为独立 assets 文件释出。
- 保持 wrapper 最薄化，只收集上下文并转发给 argus。

**测试要求**:
- 先对模板渲染输出做 snapshot 测试。
- 验证命令中正确带入 `--agent` 与需要时的 `--global`。
- 验证模板不包含非 Phase 1 的 experimental hook。

---

#### M6-T6: Agent Hook 配置生成器

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T5 |
| 参考 | `technical-hooks.md §9.4`；`technical-workspace.md §11.5` |
| 产出 | `internal/install/hooks.go`, `internal/install/hooks_test.go` |
| Commit | `feat(install): add agent hook configuration installer/uninstaller` |

**实现内容**:
- 实现 `InstallHooks(projectRoot, agents)` 与 `UninstallHooks(projectRoot, agents)`。
- Claude Code：合并 `.claude/settings.json` 中的 argus 条目并保留非 argus 条目。
- Codex：写 `.codex/hooks.json`，并确保 `~/.codex/config.toml` 开启 `codex_hooks = true`。
- OpenCode：写 `.opencode/plugins/argus.ts`，卸载时按文件名删除。
- Argus 条目识别依赖 `command` 包含 `argus tick` 或 `argus trap` 的子串。

**测试要求**:
- 先写 install / uninstall round-trip 测试。
- 覆盖已有非 argus hook 条目保留。
- 验证重复安装幂等，不造成配置重复追加。

---

#### M6-T7: install 命令 (项目级)

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T1, M6-T2, M6-T3, M6-T4, M6-T5, M6-T6, M5-T1 |
| 参考 | `technical-cli.md §7.2`；`technical-workspace.md §10.3.1` |
| 产出 | `cmd/argus/cmd_install.go`, `internal/install/installer.go`, `internal/install/installer_test.go` |
| Commit | `feat(cli): add install command with idempotent project setup` |

**实现内容**:
- 注册 `argus install [--yes]`。
- 先检查 Git 仓库存在性；无 `.git/` 直接报错。
- 若祖先目录已有 `.argus/`，报错防止嵌套安装。
- 若当前目录不是 git root，则给出警告与确认机制；`--yes` 可跳过。
- 成功时创建 `.argus/{workflows,invariants,rules,pipelines,logs,data,tmp}`，释出内置资源和项目级 skills，并安装 Hook。

**测试要求**:
- 先写非 Git 目录失败测试。
- 覆盖幂等安装、子目录确认、祖先 `.argus/` 冲突。
- 验证资源释出路径与 hook 安装结果正确。

---

#### M6-T8: uninstall 命令 (项目级)

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T6 |
| 参考 | `technical-cli.md §7.2`；`technical-workspace.md §11.5` |
| 产出 | `cmd/argus/cmd_uninstall.go` |
| Commit | `feat(cli): add uninstall command with selective skill removal` |

**实现内容**:
- 注册 `argus uninstall [--yes]`。
- 提供交互确认与 `--yes` 跳过逻辑。
- 删除 `.argus/` 目录。
- 删除 `.agents/skills/argus-*`，保留非 `argus-` 用户自定义 skills。
- 调用 Hook 卸载逻辑移除项目级 Agent 配置。

**测试要求**:
- 先写确认拒绝与 `--yes` 直接执行测试。
- 覆盖仅删除 `argus-*` skills 的选择性清理。
- 验证 uninstall 后再次 install 仍能恢复到正确状态。

---

#### M6 验收标准

```bash
cd /tmp && mkdir test-project && cd test-project && git init
/path/to/argus_v2/bin/argus install --yes
test -d .argus/workflows
test -d .argus/invariants
test -f .agents/skills/argus-doctor/SKILL.md
/path/to/argus_v2/bin/argus uninstall --yes
test ! -d .argus
go test ./internal/install/...
make lint
```

### M7: Workspace 与 Doctor

**目标**: 完成 workspace 注册/卸载、global tick 决策树，以及 13 维度 doctor 诊断。完成后，Argus 不仅能在单项目中运行，还能在工作区层面提供安装引导，并通过 doctor 给出完整的离线/在线诊断能力。

**NOT IN SCOPE**: 不新增项目级核心编排能力；不实现 deferred features；不扩展 install/uninstall 之外的新资源类型。

#### M7-T1: install --workspace

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T7, M2-T9 |
| 参考 | `technical-workspace.md §10.2`；`technical-workspace.md §11.5` |
| 产出 | `cmd/argus/cmd_install.go`, `internal/install/workspace.go`, `internal/install/workspace_test.go` |
| Commit | `feat(cli): add install --workspace for global hook and skill distribution` |

**实现内容**:
- 扩展 `argus install --workspace <path>`。
- 先校验路径存在且为目录，再做 4 步规范化。
- 写入全局 Hook 配置，命令中带 `--global`。
- 释出全局 Skills 到各 Agent 的全局 Skill 目录。
- 更新 `~/.config/argus/config.yaml`，重复注册仅 stderr 提示，不视为错误。

**测试要求**:
- 先写路径不存在与非目录报错测试。
- 覆盖重复注册、嵌套路径、全局 hook 写入。
- 验证 config.yaml 中存储的是规范化后路径。

---

#### M7-T2: uninstall --workspace

| 字段 | 内容 |
|------|------|
| 依赖 | M6-T8, M2-T9 |
| 参考 | `technical-workspace.md §10.2` |
| 产出 | `cmd/argus/cmd_uninstall.go` |
| Commit | `feat(cli): add uninstall --workspace with global cleanup` |

**实现内容**:
- 扩展 `argus uninstall --workspace <path>`。
- 对输入路径应用与 install 相同的规范化算法。
- 未找到匹配项时返回 exit 1。
- 从 config.yaml 中删除对应 workspace。
- 当最后一个 workspace 被移除时，一并清理全局 Hook 和全局 Skills。

**测试要求**:
- 先写 install 后再 uninstall 的 round-trip 测试。
- 覆盖未注册 workspace 报错与最后一个 workspace 清理逻辑。
- 验证路径规范化前后等价输入能命中同一注册项。

---

#### M7-T3: 全局 tick 逻辑

| 字段 | 内容 |
|------|------|
| 依赖 | M5-T4, M5-T1, M2-T9 |
| 参考 | `technical-workspace.md §10.3`；`technical-workspace.md §10.4` |
| 产出 | `internal/hook/global.go`, `internal/hook/global_test.go` |
| Commit | `feat(hook): add global tick decision tree for workspace-guided install` |

**实现内容**:
- 实现 `HandleGlobalTick(cwd, config)`。
- 决策树顺序：找项目根；找不到则静默退出；存在 `.argus/` 则跳过；不在 workspace 内则静默退出；在 workspace 内则返回安装引导文本。
- 非 Git 目录在 global tick 中静默跳过。
- 只做安装引导，不创建 `.argus/`，不启动 pipeline。
- 与项目级 tick 共享输入输出基础，但不复用错误语义。

**测试要求**:
- 先按决策树分支逐一写测试。
- 覆盖 workspace 匹配与非匹配、HasArgus 跳过、非 Git 目录跳过。
- 验证返回文本确实指向 `argus-install` 引导而非 workflow 启动。

---

#### M7-T4: Doctor 命令

| 字段 | 内容 |
|------|------|
| 依赖 | M2-T3, M2-T5, M5-T6, M6-T7, M5-T1 |
| 参考 | `technical-operations.md §12` |
| 产出 | `cmd/argus/cmd_doctor.go`, `internal/doctor/checks.go`, `internal/doctor/checks_test.go` |
| Commit | `feat(cli): add doctor command with 13 diagnostic dimensions` |

**实现内容**:
- 实现 `argus doctor` 及 13 个独立检查函数。
- 覆盖：安装完整性、Hook 配置、Workflow 校验、Invariant 校验、内置 Invariant、Skill 完整性、`.gitignore`、日志健康、版本兼容、临时目录权限、Pipeline 数据完整性、Shell 环境、Workspace 配置。
- 仅检查已有 Hook 产物的 Agent，不强制所有 Agent 都配置。
- 日志检查读取全部记录，不采样。
- 严格遵守“只诊断不治疗”。

**测试要求**:
- 先以函数级别为 13 个维度分别写 fixture 测试。
- 覆盖项目级日志缺失时 fallback 到用户级日志。
- 验证 doctor 汇总退出码：全通过为 0，有问题为 1。

---

#### M7 验收标准

```bash
./bin/argus install --workspace ~/test-workspace-argus
cat ~/.config/argus/config.yaml | grep test-workspace-argus
./bin/argus uninstall --workspace ~/test-workspace-argus
./bin/argus doctor
go test ./internal/doctor/... ./internal/install/...
make lint
```

### M8: 集成测试与发布前验证

**目标**: 基于前面所有模块，补齐端到端、异常路径和 workspace 场景的集成测试。完成后，Phase 1 的核心实现将具备可重复执行的系统级验证闭环，可作为发布前的最终验证层。

**NOT IN SCOPE**: 不新增产品功能；不改设计；不引入非测试辅助用的新模块；不把集成测试当成修补规范缺口的地方。

#### M8-T1: 端到端集成测试

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1 ~ M7-T4 |
| 参考 | `technical-cli.md §7-§8`；`technical-hooks.md §9`；`technical-workspace.md §10-§11` |
| 产出 | `tests/integration/e2e_test.go` |
| Commit | `test(integration): add end-to-end workflow lifecycle test` |

**实现内容**:
- 在临时 Git 仓库中执行完整场景：install、写 test workflow、workflow start、tick、job-done、status、invariant check、uninstall。
- 模拟各 Agent stdin 输入格式。
- 验证从“项目安装”到“工作流完成”的端到端链路。
- 确保测试不依赖开发机已有项目状态。
- 让集成测试覆盖最关键的 happy path。

**测试要求**:
- 先封装临时仓库与二进制调用 helper。
- 断言关键命令的 stdout、exit code、文件系统副作用。
- 验证 uninstall 后关键目录与 hook 产物被移除。

---

#### M8-T2: 异常路径测试

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1 ~ M7-T4 |
| 参考 | `technical-pipeline.md §5.7`；`technical-cli.md §7.6`；`technical-operations.md §12` |
| 产出 | `tests/integration/error_test.go` |
| Commit | `test(integration): add error path and edge case tests` |

**实现内容**:
- 覆盖损坏 YAML graceful skip + warn。
- 覆盖 workflow 文件缺失、重复 workflow start、无活跃 pipeline 的 job-done。
- 覆盖异常状态下 doctor 报告多个 running pipelines。
- 验证 error envelope 与 fail-open 命令的差异。
- 补齐最容易在真实使用中出现的边界条件。

**测试要求**:
- 先按错误类别拆测试函数，避免单个测试承担过多 setup。
- 断言警告文本、错误 envelope、doctor 退出码。
- 验证错误出现后系统仍可恢复到可继续操作状态。

---

#### M8-T3: Workspace 场景测试

| 字段 | 内容 |
|------|------|
| 依赖 | M0-T1 ~ M7-T4 |
| 参考 | `technical-workspace.md §10.2-§10.5`；`technical-hooks.md §9.2` |
| 产出 | `tests/integration/workspace_test.go` |
| Commit | `test(integration): add workspace discovery and global tick tests` |

**实现内容**:
- 覆盖 `install --workspace`、global tick 引导、项目内 install、项目级 tick、`uninstall --workspace` 全链路。
- 验证未初始化项目在 workspace 内会收到安装引导。
- 验证项目安装后 global tick 自动跳过、项目级 tick 正常工作。
- 验证 workspace 卸载后不再收到全局引导。
- 让 workspace 功能具备可回归验证的系统级测试。

**测试要求**:
- 先搭建临时 HOME 与用户级配置目录隔离环境。
- 覆盖 config.yaml、全局 skills、全局 hooks 的文件系统副作用。
- 验证 global tick 的静默跳过与引导输出分支。

---

#### M8 验收标准

```bash
go test ./tests/integration/... -v -timeout 120s
make build && make test && make lint
```

## 4. 任务依赖矩阵明细

| Task | Depends On |
|------|------------|
| M0-T1 | (none) |
| M0-T2 | M0-T1 |
| M0-T3 | M0-T1 |
| M0-T4 | M0-T2, M0-T3 |
| M0-T5 | (none) |
| M0-T6 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5 |
| M1-T1 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5, M0-T6 |
| M1-T2 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5, M0-T6 |
| M1-T3 | M1-T2 |
| M1-T4 | M0-T1, M0-T2, M0-T3, M0-T4, M0-T5, M0-T6 |
| M1-T5 | M1-T3, M1-T4 |
| M1-T6 | M1-T2 |
| M1-T7 | M1-T2 |
| M2-T1 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M2-T2 | M2-T1 |
| M2-T3 | M2-T2 |
| M2-T4 | M2-T3 |
| M2-T5 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M2-T6 | M2-T5, M2-T1 |
| M2-T7 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M2-T8 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M2-T9 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M2-T10 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M3-T1 | M2-T1, M2-T7 |
| M3-T2 | M2-T1, M2-T7 |
| M3-T3 | M3-T1, M3-T2 |
| M3-T4 | M3-T1, M3-T2 |
| M3-T5 | M3-T2 |
| M3-T6 | M3-T2 |
| M4-T1 | M2-T5 |
| M4-T2 | M2-T8, M2-T7 |
| M4-T3 | M4-T1 |
| M4-T4 | M4-T2, M3-T2 |
| M4-T5 | M4-T1, M3-T6 |
| M5-T1 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M5-T2 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M5-T3 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M5-T4 | M5-T1, M5-T2, M5-T3, M4-T2, M4-T1, M3-T2, M3-T1 |
| M5-T5 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M5-T6 | M5-T1 |
| M6-T1 | M1-T1, M1-T2, M1-T3, M1-T4, M1-T5, M1-T6, M1-T7 |
| M6-T2 | M6-T1 |
| M6-T3 | M6-T1 |
| M6-T4 | M6-T1 |
| M6-T5 | M6-T1 |
| M6-T6 | M6-T5 |
| M6-T7 | M6-T1, M6-T2, M6-T3, M6-T4, M6-T5, M6-T6, M5-T1 |
| M6-T8 | M6-T6 |
| M7-T1 | M6-T7, M2-T9 |
| M7-T2 | M6-T8, M2-T9 |
| M7-T3 | M5-T4, M5-T1, M2-T9 |
| M7-T4 | M2-T3, M2-T5, M5-T6, M6-T7, M5-T1 |
| M8-T1 | M0-T1 ~ M7-T4 |
| M8-T2 | M0-T1 ~ M7-T4 |
| M8-T3 | M0-T1 ~ M7-T4 |

## 附加说明

- 本文刻意避免在任务说明中直接嵌入 Go 代码片段；实现细节应回到对应技术文档与测试中完成。
- 若执行过程中发现某任务需要拆分更多 commit，可在保持原子性的前提下增加“内部子步骤”，但不得改变对外任务编号。
- 若某任务提前暴露技术文档冲突，应先回溯规范来源，再决定是否需要补充设计讨论，不应在代码实现层面私自定义新语义。
- M5-T4、M6-T7、M7-T4 是整条实施链上最复杂的三个任务，建议单独预留更完整的测试时间。
- 所有命令验收都默认在开发期通过 `./bin/argus` 调用，不依赖系统 PATH 中已有同名二进制。
