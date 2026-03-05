# Native CI Integration PRDs

This directory contains focused Product Requirement Documents for the Atmos Native CI Integration feature set. Each document covers a distinct subsystem.

## Documents

| Document | Description |
|----------|-------------|
| [overview.md](./overview.md) | Executive summary, problem statement, desired state, NFRs, success criteria, migration, FAQ |
| [ci-detection.md](./ci-detection.md) | CI environment detection, `--ci` flag, command parity, lifecycle hooks design |
| [job-summaries.md](./job-summaries.md) | `$GITHUB_STEP_SUMMARY` integration, rich markdown summaries, badges |
| [ci-outputs.md](./ci-outputs.md) | `$GITHUB_OUTPUT` integration, terraform outputs export, output variables |
| [status-checks.md](./status-checks.md) | Check runs, `atmos ci status` command, example output |
| [artifact-storage.md](./artifact-storage.md) | Generic `artifact.Store` interface, backends, registry, metadata |
| [planfile-storage.md](./planfile-storage.md) | Planfile adapter, CLI commands, planfile flags |
| [plan-verification.md](./plan-verification.md) | `--verify-plan` flag, plan-diff semantic comparison |
| [pr-comments.md](./pr-comments.md) | tfcmt-inspired PR comments, upsert behavior |
| [github-provider.md](./github-provider.md) | Provider/Context/Status interfaces, API endpoints, permissions, command registry |
| [describe-affected-matrix.md](./describe-affected-matrix.md) | `--format=matrix` for GitHub Actions matrix strategy |
| [configuration.md](./configuration.md) | Full `atmos.yaml` schema for planfiles and CI sections |
| [implementation-status.md](./implementation-status.md) | Phases, files to create/modify, sentinel errors, status table, changelog |
| [hooks-integration.md](./hooks-integration.md) | CI hook commands, lifecycle integration |

## Original Documents

These PRDs were split from two monolithic documents:
- `docs/prd/native-ci-integration.md` (now a redirect stub)
- `docs/prd/native-ci-artifact-storage.md` (now a redirect stub)
