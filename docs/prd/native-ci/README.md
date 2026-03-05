# Native CI Integration PRDs

This directory contains focused Product Requirement Documents for the Atmos Native CI Integration feature set, organized into three workstreams.

## Documents

### Root

| Document | Description |
|----------|-------------|
| [overview.md](./overview.md) | Executive summary, problem statement, desired state, NFRs, success criteria, migration, FAQ |
| [clarifications.md](./clarifications.md) | Resolved ambiguities, architecture decisions, open questions, and implementation specs for underspecified areas |

### [Framework](./framework/) — Core CI Infrastructure

| Document | Description |
|----------|-------------|
| [interfaces.md](./framework/interfaces.md) | Core interfaces: Provider, OutputWriter, Context, Status, Artifact Store, Planfile Store |
| [ci-detection.md](./framework/ci-detection.md) | CI environment detection, `--ci` flag, command parity, lifecycle hooks design |
| [artifact-storage.md](./framework/artifact-storage.md) | Generic `artifact.Store` interface, backends, registry, metadata |
| [hooks-integration.md](./framework/hooks-integration.md) | CI hook commands, lifecycle integration |
| [configuration.md](./framework/configuration.md) | Full `atmos.yaml` schema for planfiles and CI sections |
| [implementation-status.md](./framework/implementation-status.md) | Phases, files to create/modify, sentinel errors, status table, changelog |

### [Providers](./providers/) — CI Provider Implementations

| Document | Description |
|----------|-------------|
| [generic.md](./providers/generic.md) | Generic CI provider fallback |
| [provider.md](./providers/github/provider.md) | GitHub Actions permissions, API endpoints, command registry |
| [status-checks.md](./providers/github/status-checks.md) | Check runs, `atmos ci status` command |
| [job-summaries.md](./providers/github/job-summaries.md) | `$GITHUB_STEP_SUMMARY` integration, markdown summaries |
| [ci-outputs.md](./providers/github/ci-outputs.md) | `$GITHUB_OUTPUT` integration, terraform outputs export |
| [pr-comments.md](./providers/github/pr-comments.md) | tfcmt-inspired PR comments, upsert behavior |

### [Terraform Plugin](./terraform-plugin/) — Terraform-Specific CI Commands

| Document | Description |
|----------|-------------|
| [planfile-storage.md](./terraform-plugin/planfile-storage.md) | Planfile adapter, CLI commands, planfile flags |
| [plan-verification.md](./terraform-plugin/plan-verification.md) | `--verify-plan` flag, plan-diff semantic comparison |
| [describe-affected-matrix.md](./terraform-plugin/describe-affected-matrix.md) | `--format=matrix` for GitHub Actions matrix strategy |

## Original Documents

These PRDs were split from two monolithic documents:
- `docs/prd/native-ci-integration.md` (now a redirect stub)
- `docs/prd/native-ci-artifact-storage.md` (now a redirect stub)
