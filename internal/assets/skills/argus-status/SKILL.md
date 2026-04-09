---
name: argus-status
description: Query current pipeline or job progress and detailed status
version: 0.1.0
---

# argus-status

Check the current state of Argus pipelines and jobs.

## When to Use

- Check what pipeline is running
- See current job progress
- Review pipeline history

## Commands

- `argus status` — Show current pipeline status with progress in readable text
- `argus status --json` — Return structured status data for parsing

## Output Mode

- Default output is readable text for both humans and agents.
- Use `--json` only when a script or workflow needs stable structured fields.

## Output

Shows pipeline ID, workflow ID, current job, progress (e.g., 2/5), and status.
