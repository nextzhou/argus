---
name: argus-workflow-syntax
description: Reference documentation for Argus workflow YAML syntax
version: 0.1.0
---

# argus-workflow-syntax

YAML syntax reference for Argus workflow definitions.

## File Location

Workflow files go in `.argus/workflows/` with `.yaml` extension.

## Schema

```yaml
version: v0.1.0
id: my-workflow          # lowercase letters, digits, hyphens
description: "Optional description"

jobs:
  - id: job_name         # Optional but recommended
    prompt: "Instructions for the Agent"
    skill: optional-skill-name
  - id: another_job
    ref: _shared.job_id  # Reference shared job definition
```

## Fields

- `version`: Must be `v0.1.0` (major version compatibility checked)
- `id`: Unique workflow identifier (regex: `^[a-z0-9]+(-[a-z0-9]+)*$`)
- `jobs`: Ordered list of steps. Each job needs `prompt` or `skill` (or `ref`)
- `ref`: Reference to a job defined in `_shared.yaml`

## Shared Jobs

Define reusable jobs in `.argus/workflows/_shared.yaml` and reference them with `ref: _shared.job_id`.
