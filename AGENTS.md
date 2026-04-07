# AGENTS.md — Argus

## Project Overview

Argus is an AI Agent workflow orchestration tool. It provides a CLI binary (Go) that integrates with multiple AI Agents (Claude Code, Codex, OpenCode) via their hook systems, orchestrating workflows, invariants, and project state.

**Current phase**: Final document verification. All design decisions recorded, technical documents written, eight rounds of Oracle verification completed. No implementation code exists yet.

## Key Module Navigation

```
.argus/                          # Project-level Argus directory
  workflows/                     # Workflow YAML definitions (git-tracked)
    _shared.yaml                 # Shared job definitions
  invariants/                    # Invariant YAML definitions (git-tracked)
  rules/                         # Generated project rules (git-tracked)
  pipelines/                     # Pipeline instance data files (local-only)
  logs/                          # Hook execution logs (local-only)
  data/                          # General-purpose data files, e.g. freshness timestamps (git-tracked)
  tmp/                           # Temporary data (local-only)
.agents/skills/argus-*/SKILL.md  # Skills distributed via Agent Skills standard (git-tracked)
assets/                          # Embedded assets in source code (Go embed)
  skills/                        # Built-in skills (released on install)
  workflows/                     # Built-in workflow definitions (released on install)
  invariants/                    # Built-in invariant definitions (released on install)
  prompts/                       # Runtime output templates (not released, internal use only)
```

## Architecture Invariants

### 1. Argus is orchestration layer, not Agent replacement

Argus does not execute shell commands or make decisions. It only tells the Agent what to do and tracks progress. Anything the Agent can do, let the Agent do it.

- All jobs are Agent-driven: inject context -> Agent executes -> job-done
- No `script` field in workflow jobs (removed by design, see `workflow-yaml-schema.md` section 3)
- tick only does state check + context injection, returns fast

### 2. Artifacts as ground truth

Prefer checking actual file/artifact existence over boolean flags or recorded state.

- No `initialized: true` marker. Use Invariant system to check actual artifacts instead.
- Pipeline state lives in dedicated per-instance data files, not a centralized `state.yaml`
- State classification: every piece of persistent data is either git-tracked (team-shared) or local-only (machine-specific), never ambiguous

### 3. Workflow (imperative) + Invariant (declarative) are complementary

- Workflow answers "how" (process steps). Invariant answers "what should be true" (conditions).
- Same goal, different dimensions: Workflow ensures correct process, Invariant catches drift regardless of cause.

### 4. Diagnostic tools only diagnose, never treat

`inspect`, `invariant check`, `doctor` only report problems and suggest solutions. They never auto-fix or auto-start workflows. The Agent guides the user to decide.

### 5. Invariant check is shell-only

No prompt check (LLM-based evaluation) in invariant definitions. Deep verification goes into the remediation workflow's first job. Rationale: shell check's core value is silent pass without interrupting the user.

### 6. Semantic checks become freshness checks

Complex semantic checks (e.g., "is documentation up-to-date") are converted to timestamp-based checks (e.g., "has documentation been reviewed in the last N days"). This keeps invariant checks fast, deterministic, and shell-only.

### 7. Path construction input validation

All external inputs used to construct file paths must be validated in Go before use, to prevent path traversal attacks.

| Input | Used in path | Validation |
|-------|-------------|------------|
| session_id | `/tmp/argus/<safe-id>.yaml` | UUID format preferred: `^[0-9a-fA-F-]+$`; non-conforming values are SHA256-hashed (first 16 chars) to produce a safe filename (`safe-id`) |
| workflow_id | `.argus/pipelines/<workflow-id>-<timestamp>.yaml` | Naming convention: `^[a-z0-9]+(-[a-z0-9]+)*$` |
| invariant id | `.argus/invariants/<id>.yaml` | Same as workflow_id |

Fallback: after constructing the path, verify via `filepath.Rel` that the resolved path is still under the expected directory.

## Naming Conventions

### Namespace reservation

`argus-` prefix is reserved for built-in workflows, invariants, and skills. Users cannot use this prefix for their own definitions. Enforced at inspect/install time.

### Skill naming

- Lowercase letters, digits, hyphens only. Max 64 characters.
- Regex: `^[a-z0-9]+(-[a-z0-9]+)*$`
- No `:` character (colon namespacing is Claude Code Plugin system, not skill names)
- Directory name must match `name` field in SKILL.md

### Key terminology

| Term | Meaning | NOT to be confused with |
|------|---------|------------------------|
| Pipeline | An instance of a workflow execution in Argus | GitLab Pipeline (different concept) |
| Rule | Constraint/specification for coding | Skill (capability/procedure) |
| Argus Command | CLI subcommand (`argus install`) | Slash Command (`/argus-doctor`) |

## Design Conventions

### Version field

All independent schema files (workflow YAML, invariant YAML, pipeline data) include `version: v0.1.0`. Argus checks major version compatibility at parse time. Dependent files (`_shared.yaml`) do not have their own version.

### Pipeline data model

- Single active pipeline (phase 1), sequential job execution
- `current_job` global field instead of per-job status (status is derivable from position relative to current_job + pipeline status)
- `current_job: null` when pipeline is completed
- Per-job data only records runtime outputs (`message`, `started_at`, `ended_at`), not status

### tick injection strategy

- State changed (new job, new pipeline): inject full context (prompt, skill, detailed guidance)
- State unchanged: inject minimal summary (current job id + status + job-done reminder)
- Minimal summary is necessary: prevents Agent from forgetting to call job-done after multiple conversation turns

### job-done dual progression

- tick path: user input triggers tick, for recovery/resume scenarios
- job-done return path: job-done returns next job info, Agent auto-advances without waiting for user input

### Human-in-the-loop

No `confirm` / `auto` fields. Write confirmation requirements directly in job prompt. Agent judges based on context.

## Development Workflow

### Discussion-first approach

All technical topics were discussed thoroughly before implementation. Design decisions, tradeoffs, and rationale are recorded in the final technical documents (`docs/technical-*.md`). Draft discussion records have been archived (available in git history).

### Communication rules

- Communicate in Chinese
- Do not proactively diverge into new topics before being asked
- Record tradeoff analysis for important design decisions to guide future design and prevent drift

### After context compaction

Read these key files to restore context:

1. `AGENTS.md` (this file) -- architecture invariants, conventions, and current phase
2. `docs/technical-overview.md` -- entry point of the final technical specification

Other final technical documents (read on demand):
- `docs/technical-workflow.md`, `docs/technical-invariant.md`, `docs/technical-pipeline.md`
- `docs/technical-cli.md`, `docs/technical-hooks.md`, `docs/technical-workspace.md`, `docs/technical-operations.md`

Draft discussion records have been cleaned up (available in git history).

## Project Phase

### Current phase: Technical specification complete

All design topics have been discussed. Final technical documents (8 files) have been written in `docs/technical-*.md`. Ten rounds of Oracle verification completed, all issues resolved. Draft discussion records cleaned up (available in git history).

**Next phase**: Implementation.

### Completed milestones

1. **Design discussions** (all topics): CLI, hooks, workflow schema, invariant, pipeline/state, output format, exit codes, doctor, workspace, skill distribution, security
2. **Final technical documents written** (8 files in `docs/technical-*.md`)
3. **Verification Round 1**: coverage (17 items → all fixed), conflict (2 items → both fixed), clarity (23 items → recorded in pending-discussions.md)
4. **Round 1 auto-fixes**: overview nav link + .git/hooks classification, workflow examples + ref rationale + inspect items, invariant inspect typo + 6 missing items, hooks OpenCode exit code + input rationale + Codex limitations, workspace Plugin/MCP rationale, pipeline Agent retry note
5. **Pending discussions resolved** (23 items): P1-P6 pipeline details, C1-C5 CLI details, H1-H5 hooks details, W1-W4 workspace details, I1-I2 invariant details, O1-O4 other details — all confirmed and updated to final docs
6. **Verification Round 2**: coverage (2 items → fixed), conflict (8 items → all fixed), clarity (8 BLOCKING + 2 MINOR → all resolved)
   - A-class fixes (10): pipeline field table, YAML example, session ID format, _shared.yaml example, template syntax, tick output, --agent description, job-done exit code, prompt+workflow coexistence, trap boundary
   - B-class decisions (8): workflow start output, status real-time invariant, multi-pipeline cancel-all, no concurrent handling, install releases to disk, zero-job illegal, session_id hash, install TODO
7. **Verification Round 3**: coverage (complete), conflict (5 items → all fixed), clarity (4 BLOCKING → all resolved)
   - C-class fixes (5): session_id hash propagation, --global wording, hooks text output example, pipeline ID Z suffix, CLI section numbering
   - D-class decisions (4): CLI command envelope, .argus/data/ + touch-timestamp + compact timestamp, global invariant/workflow paths, corrupt YAML handling
8. **Additional design decisions**: embedded assets directory (assets/ with Go embed), toolbox sha256sum, doctor shell check, sub-agent hook shielding (wrapper thinning principle), skill format cross-agent compatibility
9. **Verification Round 4**: coverage (2 items → fixed), conflict (covered by other dimensions), clarity (6 BLOCKING + 3 MINOR → all resolved)
   - E-class fixes (2): global compact timestamp convention in overview §3.3 + hooks log format, .argus/data/ removed from doctor .gitignore check
   - F-class decisions (8): template variable Phase 1 field table + pre_job + partial substitution, invariant shell execution semantics (bash -c non-login + tradeoff rationale), workspace path normalization algorithm (4-step with $HOME→~ compression), subdirectory install confirmation mechanism (--yes flag + non-TTY detection), job-done consistent error path on workflow modification, status JSON details with description + hints array, pipeline instance ID canonical form (stem without .yaml), doctor reads full log (no sampling)
10. **Verification Round 5**: coverage (1 item → fixed), conflict (10 items → all fixed), clarity (3 BLOCKING + 1 MINOR → all resolved)
    - G-class auto-fixes (7): status JSON examples (remove agents-md-fresh + fix description), session file path unified to safe-id, CLI Pipeline label correction, workspace config trailing slash, hooks §9.7 JSON contradiction, doctor dimension count 12→13, section numbering gaps (pipeline §6 parent + operations 13.5→13.3)
    - G-class decisions (6): overview §2.3 split skill vs workflow/invariant release paths, global hook skip based on .argus/ existence, Job ID prohibit hyphens (underscore only for text/template compatibility + absent job = preserved placeholder), workspace bootstrap simplified to Skill-only guidance (no global invariant/workflow files), pre-install log fallback to ~/.config/argus/logs/hook.log, install hint TODO marked as non-normative
11. **Verification Round 6**: coverage (2 items → fixed), conflict (5 items → all fixed), clarity (0 BLOCKING + 4 MINOR → all resolved)
    - H-class auto-fixes (7): overview path validation table safe-id, CLI failed_job run_tests, pipeline session path safe-id, Claude Code rules CLAUDE.md, workflow shared key code_review + naming rule, CLI toolbox yq .id, AGENTS.md round count
    - H-class decisions (4): invariant inspect [dir] workflow reference always resolves to current project, uninstall deletes .agents/skills/argus-* (preserves user skills), cancel/snooze no-pipeline = error envelope exit 1, hook log ERROR = wrapper/execution failures only (business failures = OK)
12. **Verification Round 7**: coverage (complete), conflict (7 items → all fixed), clarity (1 BLOCKING + 3 MINOR → all resolved)
    - I-class auto-fixes (5): AGENTS.md path table safe-id, CLI status JSON envelope + Markdown count, OpenCode tick example parentID, hooks wrapper principle softened
    - I-class decisions (6): argus-init §4.8 as canonical (add Skill check, §4.3 marked abbreviated), Doctor fallback to user-level log, Job ID regex tightened to letter-first `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`, non-Git install = error exit 1, --agent kept global multi-value (tick/trap/install/uninstall), other command output schemas deferred to implementation
13. **Verification Round 8**: coverage (complete), conflict (7 items → all fixed), clarity (1 BLOCKING + 2 MINOR → all resolved)
    - J-class auto-fixes (3): CLI status JSON duplicate status key removed, overview workflows description corrected, invariant §4.8 step 6 generate-rules→generate_rules
    - J-class decisions (7): workflow inspect examples wrapped in status envelope, argus-init workflow check changed to mark file (.argus/data/init_workflows_generated), git hooks path classification annotated (local-only fallback), section numbering unification deferred, Doctor only checks agents with existing hook artifacts, status best-effort display on missing current_job, OpenCode experimental hooks marked non-Phase-1
14. **Verification Round 9**: coverage (5 BLOCKING + 1 MINOR), conflict (2 BLOCKING + 2 MINOR), clarity (5 BLOCKING + 3 MINOR → all resolved)
    - K-class auto-fixes (3): invariant §4.4 phantom reference I2, workflow §2.2 YAML indentation, timestamp format z→Z
    - K-class decisions (15): workflow/invariant ID regex added to respective docs, argus-invariant-check added to skill list, doctor slow check description weakened, pipelines dir auto-creation added to pipeline doc, tick error output unified to text, argus-init gitignore check expanded to 3 paths, git hook detection expanded to 4 frameworks, workflow start parameter unified to workflow-id, trap Phase 1 allow JSON defined, check: [] forbidden (must be non-empty), current_job on cancel/failed preserved (only completed=null), snooze in abnormal state = snooze all, global skill path clarified, install hint cadence = invariant runs once per session (no change needed)
15. **Verification Round 10**: conflict (CLEAN), clarity (4 BLOCKING → 3 fixed + 1 no change needed)
    - L-class fixes (3): snooze-all post-tick priority rule (snooze wins, tick silent), workspace install path validation + uninstall normalization matching, global tick decision tree clarified (non-git directories silently skipped)
    - L-class decisions (1): --agent + --workspace combination = natural behavior (agent scopes hooks/skills, workspace registration is agent-agnostic), no change needed
16. **Cleanup**: Draft discussion records and the early spec document were removed. All content is preserved in git history.
