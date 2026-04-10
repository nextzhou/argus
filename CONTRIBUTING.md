# Contributing to Argus

Thanks for considering a contribution.

## Before You Start

- Read [README.md](README.md) for the product overview and quick start
- Read [AGENTS.md](AGENTS.md) for development rules and architectural invariants
- Read [docs/technical-overview.md](docs/technical-overview.md) before changing core behavior
- Open or join an issue first for large features, architecture changes, or behavior changes that may affect workflows or invariants

## Development Workflow

1. Fork the repository and create a topic branch from `main`.
2. Keep changes scoped to a single problem.
3. Add or update tests before changing behavior when practical.
4. Run the required local checks:

   ```bash
   make build
   make test
   make lint
   ```

5. Update documentation when behavior, CLI output, or repository workflows change.
6. Open a pull request with a clear summary, test evidence, and any follow-up work.

## Commit Messages

Use Conventional Commits:

```text
type(scope): description
```

Examples:

- `feat(cli): add workflow inspect command`
- `fix(hook): keep tick output mutually exclusive`
- `docs(project): clarify workspace behavior`

## Pull Request Expectations

- Explain the problem and the chosen approach
- Link the relevant issue when one exists
- Include test results or explain why a check could not be run
- Keep unrelated refactors out of the same pull request

## Design Expectations

Argus is an orchestration layer for AI agents, not an agent replacement. Changes
should preserve the project invariants documented in [AGENTS.md](AGENTS.md),
especially around artifact-driven state, scope consistency, and shell-only
invariant checks.
