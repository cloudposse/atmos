# Native CI Integration PRDs

This directory contains focused Product Requirement Documents for the Atmos Native CI Integration feature set, organized into three workstreams.

## Documents

### Root

| Document | Description |
|----------|-------------|
| [overview.md](./overview.md) | Executive summary, problem statement, desired state, NFRs, success criteria, migration, FAQ |

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
| [plan-verification.md](./terraform-plugin/plan-verification.md) | Plan verification on `deploy` command, plan-diff semantic comparison |
| [describe-affected-matrix.md](./terraform-plugin/describe-affected-matrix.md) | `--format=matrix` for GitHub Actions matrix strategy |

### [Phases](./phases/) — Incremental Implementation PRDs

| Document | Status | Description |
|----------|--------|-------------|
| [planfile-storage-validation.md](./phases/planfile-storage-validation.md) | **SHIPPED** | Git SHA fallback, KeyPattern integration, metadata validation |
| [planfile-metadata-embed-artifact.md](./phases/planfile-metadata-embed-artifact.md) | **SHIPPED** | `planfile.Metadata` embeds `artifact.Metadata`, simplified adapter |
| [planfile-bundle-with-lockfile.md](./phases/planfile-bundle-with-lockfile.md) | **SHIPPED** | Plan + lock file tar bundle, multi-file store interface |
| [unify-artifact-stores.md](./phases/unify-artifact-stores.md) | **SHIPPED** | Unified artifact store registry, deleted planfile local/registry |
| [planfile-cli-component-stack-addressing.md](./phases/planfile-cli-component-stack-addressing.md) | **SHIPPED** | CLI `<component> -s <stack>` pattern, SHA resolution, `--all` flag |
| [apply-command-parity.md](./phases/apply-command-parity.md) | **SHIPPED** | Apply/deploy full CI wiring (PreRunE, output capture, error defer, `--ci` flag) |
| [plan-verification-ci-integration.md](./phases/plan-verification-ci-integration.md) | Proposed | Deploy-based stored vs fresh plan comparison (download → plan → compare → apply) |
| [move-checkrun-store-to-provider.md](./phases/move-checkrun-store-to-provider.md) | Proposed | Move check run ID correlation from plugin to provider layer |
| [rename-artifact-store-types.md](./phases/rename-artifact-store-types.md) | Proposed | Rename store types to namespaced `{provider}/{backend}` convention |

## Original Documents

These PRDs were split from two monolithic documents:
- `docs/prd/native-ci-integration.md` (now a redirect stub)
- `docs/prd/native-ci-artifact-storage.md` (now a redirect stub)
