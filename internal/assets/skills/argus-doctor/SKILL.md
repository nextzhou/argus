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
2. Check workflow and invariant health: YAML validity, inspect failures, and built-in invariant availability
3. Check Agent integration: hook configuration for supported agents and workspace-global registration drift
4. Check skill integrity: project-visible lifecycle skills, managed global skills, and `.gitignore` coverage
5. Check runtime artifacts when relevant: logs, pipeline data, tmp directory permissions, and version-compatibility mismatches

## Without Argus Binary

This skill still provides value when the `argus` binary is unavailable. Use file inspection and basic shell commands to diagnose likely setup drift, but note that the `argus doctor` command itself is unavailable until the binary is restored.

## Output Mode

- `argus doctor` defaults to a readable report for both humans and agents.
- Use `argus doctor --json` only when another tool needs structured results.

## Common Issues

- Missing `.argus/` directory: run `argus setup`
- Hook not triggering: check Agent-specific config files
- Malformed workflow or invariant YAML: run the matching `argus workflow inspect` or `argus invariant inspect`
- Invariant failures: run `argus invariant check` for details
- Global or workspace drift: verify workspace registration plus the user-level hook and skill roots
