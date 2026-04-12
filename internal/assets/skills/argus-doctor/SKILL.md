---
name: argus-doctor
description: Diagnose Argus setup issues using file checks and shell commands
version: 0.1.0
---

# argus-doctor

Diagnose and troubleshoot Argus setup and configuration issues.

## When to Use

- Argus commands are failing
- Hook integration not working
- After setup to verify configuration

## Diagnostic Approach

1. Check core setup integrity: `.argus/` structure, expected built-in assets, and whether the local `argus` binary is present
2. Check workflow and invariant health: YAML validity, inspect failures, and whether invariant execution has been explicitly enabled
3. Check Agent integration: hook configuration for supported agents and workspace-global registration drift
4. Check skill integrity: project-visible lifecycle skills, managed global skills, and `.gitignore` coverage
5. Check runtime artifacts when relevant: logs, pipeline data, tmp directory permissions, and version-compatibility mismatches
6. Before recommending `argus doctor --check-invariants`, inspect automatic invariant shell commands for obvious side effects and explain the risk

## Without Argus Binary

This skill still provides value when the `argus` binary is unavailable. Use file inspection and basic shell commands to diagnose likely setup drift, but note that the `argus doctor` command itself is unavailable until the binary is restored.

## Output Mode

- `argus doctor` defaults to a readable report for both humans and agents.
- `argus doctor --check-invariants` is an opt-in deep diagnostic mode that executes invariant shell checks; recommend it only after assessing risk.
- Use `argus doctor --json` only when another tool needs structured results.

## About `--check-invariants`

### What It Does

- Runs the current scope's automatic invariants only: definitions with `auto != never`
- Executes the invariant shell checks exactly for diagnosis; it does not repair anything or start workflows
- Produces deeper timing diagnostics than default `argus doctor`, including:
  - total automatic-invariant runtime
  - per-invariant runtime
  - per-step runtime and step status
- Exists to explain slow automatic checks, not to replace ordinary setup/configuration diagnostics

### When to Recommend It

- `tick` or `status` indicates that invariant checks are slow
- Default `argus doctor` shows the automatic-invariant deep-diagnostics item as skipped and the user wants the actual slow source
- You need a timing breakdown across automatic invariants, not just pass/fail output for one invariant

### When Not to Recommend It

- The automatic invariant shell commands have obvious side effects that the user did not agree to run
- The invariant definitions look risky or unclear, for example they contain package installs, deploys, destructive file operations, database changes, or other stateful external commands
- The user only needs to validate one invariant or inspect a specific failure

### Relation to `argus invariant check`

- Use `argus invariant check` when you want pass/fail results for one invariant or for all invariants as a direct validation command
- Use `argus doctor --check-invariants` when you want deep diagnostic timing breakdowns for the automatic invariants that affect runtime behavior
- If the need is "which invariant is slow?", prefer `argus doctor --check-invariants`
- If the need is "did this invariant pass or fail?", prefer `argus invariant check`

## Common Issues

- Missing `.argus/` directory: run `argus setup`
- Hook not triggering: check Agent-specific config files
- Malformed workflow or invariant YAML: run the matching `argus workflow inspect` or `argus invariant inspect`
- Invariant failures: run `argus invariant check` for details
- Slow invariant checks: assess invariant risk first, then consider `argus doctor --check-invariants`
- Global or workspace drift: verify workspace registration plus the user-level hook and skill roots
