---
name: argus-doctor
description: Diagnose Argus installation issues using file checks and shell commands
version: 0.1.0
---

# argus-doctor

Diagnose and troubleshoot Argus installation and configuration issues.

## When to Use

- Argus commands are failing
- Hook integration not working
- After installation to verify setup

## Diagnostic Approach

1. Check `.argus/` directory structure exists
2. Verify workflow and invariant files are present
3. Check Agent hook configurations
4. Verify skill files are installed
5. Check `.gitignore` entries

## Without Argus Binary

This skill works even when the `argus` binary is unavailable. Use file reading and basic shell commands to diagnose.

## Common Issues

- Missing `.argus/` directory: run `argus install`
- Hook not triggering: check Agent-specific config files
- Invariant failures: run `argus invariant check` for details
