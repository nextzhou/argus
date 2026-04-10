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

1. Check `.argus/` directory structure exists
2. Verify workflow and invariant files are present
3. Check Agent hook configurations
4. Verify skill files are set up
5. Check `.gitignore` entries

## Without Argus Binary

This skill works even when the `argus` binary is unavailable. Use file reading and basic shell commands to diagnose.

## Output Mode

- `argus doctor` defaults to a readable report for both humans and agents.
- Use `argus doctor --json` only when another tool needs structured results.

## Common Issues

- Missing `.argus/` directory: run `argus setup`
- Hook not triggering: check Agent-specific config files
- Invariant failures: run `argus invariant check` for details
