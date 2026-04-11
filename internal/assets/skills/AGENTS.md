# AGENTS.md — internal/assets/skills

## Scope

This directory contains the embedded source assets for built-in Argus skills.

- A skill existing under `internal/assets/skills/` means it is available to the Argus binary as an embedded asset.
- It does **not** mean the skill must be released in every scope.
- Scope-specific release policy is owned by lifecycle setup logic. Keep this document, the lifecycle release sets, and the technical workspace documentation aligned.

## Core Assumptions

### Global skills may assume a local Argus installation

Global skills are installed only by local Argus setup. If a global skill is visible, it is reasonable to assume:

- the machine has an Argus binary available
- Argus-managed global hooks and skill roots may exist
- binary-dependent commands such as runtime inspection or schema validation are legitimate entry points
- the current user may have refreshed those managed global skills via either `argus setup` or `argus setup --workspace`

### Project skills must assume collaborators may not have Argus installed

Project-level skills are git-tracked artifacts and may be visible immediately after a repository clone or pull.

- Do not assume the collaborator has installed Argus
- Do not assume any global skills exist on that machine
- Do not make a project-visible skill a dead entry point that only says to run `argus ...`

Project-visible skills must still provide value when the Argus binary is missing, such as explanation, setup guidance, teardown guidance, or diagnosis with fallback behavior.

## Distribution Rules

### 1. Project scope is for standalone lifecycle skills only

A project-distributed skill must remain useful for collaborators who only have the repository contents.

Good project-scope skills:

- explain what Argus is and why the repository contains it
- guide setup or teardown decisions
- diagnose repository state and clearly distinguish binary-dependent checks from file-based checks

Do not release a skill to project scope when its main value depends on a working Argus installation.

### 2. Global scope may include binary-dependent operational and reference skills

Runtime control, schema authoring helpers, and other binary-dependent skills belong in global scope, where local installation is already an established prerequisite.

This includes skills whose normal use depends on commands such as:

- `argus status`
- `argus workflow ...`
- `argus invariant ...`
- `argus workflow inspect`
- `argus invariant inspect`

### 3. Workflow-local guidance should prefer job prompts over standalone skills

If instructions exist only to support one built-in workflow job, inline that guidance into the workflow job prompt instead of introducing another globally named skill.

Create a standalone skill only when the guidance is reusable across workflows or user-initiated tasks.

## Target Distribution Model

The target split for built-in skills is:

- Project scope: standalone lifecycle skills only, currently `argus-intro`, `argus-setup`, `argus-teardown`, and `argus-doctor`
- Global scope: the project-scope skills plus binary-dependent reference and runtime skills such as `argus-configure-workflow`, `argus-configure-invariant`, and `argus-runtime`
- Single-workflow companion guidance should be in workflow prompts rather than separate skills

When simplifying the skill surface, prefer removing narrow companion skills before collapsing standalone lifecycle skills into a single catch-all entry point.

## Maintenance Rules

- Before adding a new built-in skill, decide whether it is useful to a collaborator who only sees the checked-in project files
- If the answer is no, do not release that skill to project scope
- If a skill is meaningful only after local Argus installation, classify it as global-only from the start
- `argus setup` may refresh managed global skills even when it is run from a project root; project teardown still must not remove those user-level global skills
- When a skill's guidance can be expressed directly in a built-in workflow job without losing reuse, prefer the workflow prompt
- Do not rely on duplicated project/global copies of the same binary-dependent skill to solve discoverability; use intentional scope policy instead

## Safe-Write Guidance Rules

- Safe-write instructions stage only user-managed definitions
- Built-in `argus-*` workflows and invariants remain managed by `argus setup`; skills must not imply those files are copied into staging, edited there, or synced back from staging
- If `_shared.yaml` or another user-managed shared file participates in the edit flow, say so explicitly
- If validation depends on live directories outside staging, the skill must say that directly so the agent does not assume a fully isolated temp tree
