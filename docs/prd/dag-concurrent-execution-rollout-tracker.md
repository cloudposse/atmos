# DAG Concurrent Execution Rollout Tracker

This tracker coordinates the DAG concurrent execution rollout as a sequence of small, reviewable PRs. The primary design source remains `docs/prd/dag-concurrent-execution.md`; this file describes current rollout state, what each PR owns, and which behavior must remain unchanged while output and diagnostics work continues.

## Implementation Strategy

The execution foundation and GitHub CI verification path are now merged into `main`. PR 1 through PR 7 established process stream injection, the generic scheduler, graph-backed Terraform routing, plan/apply/destroy concurrency, affected routing, and aggregate GitHub Actions output for concurrent Terraform plans. Remaining work should focus on making concurrent execution understandable from local terminals and extending DAG support without changing scheduler semantics.

Remaining near-term layers are:

1. TUI/interactive output for non-CI Terraform plan/apply/destroy runs.
2. Mixed-type DAG adapter support after Terraform-only execution and output UX are stable.
3. Advanced scheduling diagnostics and operator troubleshooting aids.

## Cross-Cutting Rules

- Use `dependencies.components` as the primary dependency source. Use `settings.depends_on` only as a compatibility fallback.
- Keep `pkg/dependency` as graph/data infrastructure. Do not put orchestration behavior there.
- Keep scheduler nodes as scheduling data. Execution work belongs behind dispatchers/adapters.
- Preserve `--max-concurrency 1` as equivalent to current sequential behavior.
- Preserve native CI plan/apply/deploy capture, hooks, summaries, outputs, status contexts, comments, and planfile behavior unless a PR explicitly changes that surface.
- Do not add new files under `internal/exec`; use existing files there only as shims or integration points.
- Terraform node completion means subprocess execution, hooks, store writes, CI finalize work, and per-node output capture have all completed.
- Keep each PR reviewable: tests should prove the layer being introduced, and later PR behavior should not leak into earlier PRs.

## Active PRs

| Step | PR | Status | Branch | Scope |
| --- | --- | --- | --- | --- |
| PR 1 | [cloudposse/atmos#2464](https://github.com/cloudposse/atmos/pull/2464) | Merged into `main` | `codex/dag-process-io-foundation` | Process runner, stream injection, shell wrapper integration, `runTerraformShow` stdout capture fix |
| PR 2 | [cloudposse/atmos#2465](https://github.com/cloudposse/atmos/pull/2465) | Merged into `main` | `codex/dag-scheduler-core` | Generic `pkg/scheduler` ready-queue scheduler with deterministic aggregate results |
| PR 3 | [cloudposse/atmos#2466](https://github.com/cloudposse/atmos/pull/2466) | Merged into `main` | `codex/dag-terraform-graph-bulk-path` | Terraform `--all`, `--components`, and `--query` graph-backed routing with effective concurrency fixed at `1`; includes discovered auth safety prerequisite |
| PR 4 | [cloudposse/atmos#2468](https://github.com/cloudposse/atmos/pull/2468) | Merged into `main` | `codex/dag-terraform-plan-concurrency` | Plan-only `--max-concurrency`, grouped/stream output, per-node logs, no-change hiding, and execution summaries |
| PR 5 | [cloudposse/atmos#2474](https://github.com/cloudposse/atmos/pull/2474) | Merged into `main` | `codex/dag-terraform-apply-destroy-concurrency` | Concurrent Terraform `apply`/`destroy` safety model with auto-approve gates, cancellation propagation, and reverse destroy ordering |
| PR 6 | [cloudposse/atmos#2519](https://github.com/cloudposse/atmos/pull/2519) | Merged into `main` | `codex/dag-terraform-affected-scheduler` | Routes Terraform `--affected` through the scheduler and adds `--fail-fast`/`--keep-going` controls |
| PR 7 | [cloudposse/atmos#2577](https://github.com/cloudposse/atmos/pull/2577) | Merged into `main` | `codex/dag-github-concurrent-output` | GitHub Actions-oriented output for concurrent Terraform plans, aggregate CI summaries, output variables, optional status contexts, and optional PR comments |
| Tracking | [cloudposse/atmos#2467](https://github.com/cloudposse/atmos/pull/2467) | Open draft | `codex/dag-concurrent-execution-tracker` | Rollout plan, findings, and PR coordination document |

## Planned PRs

| Step | Intended base | Proposed branch | Scope | Key gates / non-goals |
| --- | --- | --- | --- | --- |
| PR 8 | `main` after PR 7 merge | `codex/dag-terraform-interactive-output` | Add TUI/interactive output for non-CI Terraform plan/apply/destroy runs so concurrent execution is usable from a terminal. | Applies outside CI. Preserve GitHub CI rendering and native CI hooks. Do not introduce new execution semantics. |
| PR 9 | PR 8 | `codex/dag-mixed-type-adapters` | Add mixed-type DAG adapter support after Terraform-only execution and output UX are stable. Include explicit adapter boundaries for Terraform and future component types. | Requires repo-owner agreement on mixed-type ordering semantics. Do not rewrite unrelated executors. |
| PR 10 | PR 9 | `codex/dag-scheduling-diagnostics` | Add advanced scheduling diagnostics, graph/debug output, and operator-facing troubleshooting aids. | Should not change scheduling semantics. Keep diagnostics opt-in and safe for CI logs. |

## Current Findings

- The implementation stack PRs 1 through 7 have merged into `main`. PR 8 should start from current `main` and focus on local interactive/TUI output.
- `--ci` uses native CI provider detection. GitHub Actions auto-detects through `GITHUB_ACTIONS=true`; explicit `--ci` forces CI mode and falls back to the generic provider when no platform is detected.
- GitHub Actions output is written through provider output writers to `$GITHUB_STEP_SUMMARY` and `$GITHUB_OUTPUT`. Concurrent Terraform plan CI now writes aggregate artifacts once after the scheduler finishes to avoid concurrent writes to those files.
- Terraform CI plugin behavior remains compatibility-critical for single-component plan/apply/deploy. Multi-component plan CI uses the aggregate collector path; single-component and non-plan behavior continue through the existing hook path.
- Multi-component Terraform plan/apply/deploy suppress the global PostRun hook and install `Info.PerComponentHook` for per-component attribution. PR 7 centralizes CI artifact writes only for multi-component plan CI; later PRs should preserve that separation.
- The GitHub provider currently uses the commit Status API for status contexts. PR 7 preserved that path; later CI work should not depend on richer Checks API annotations or output without a separate design.
- Aggregate PR comments use a distinct marker from per-component comments (`<!-- atmos:ci:<command>:<component>:<stack> -->`) so they do not collide with existing component comments.
- The Terraform scheduler adapter captures per-node stdout/stderr, exit code, changed state, status, timings, and log file paths. CI aggregation should continue to use those structured outputs instead of scraping interleaved GitHub logs.
- GitHub Actions step summaries have a practical size limit of 1 MiB. Bulk operations should keep summaries bounded and rely on component logs, plan output files, and generated artifacts for full detail.
- PR 7 hardening found and fixed concurrency issues around identity-backed secret resolution, credential materialization, Terraform/OpenTofu provider cache access, and long CI output truncation. These findings should stay part of the DAG rollout risk model.

## PR 7: GitHub Actions Output For Concurrent Terraform Plans

Merged PR: [cloudposse/atmos#2577](https://github.com/cloudposse/atmos/pull/2577)

PR 7 makes concurrent Terraform plan verification usable in GitHub Actions with minimal friction.

Merged scope:

- Keep existing public config and flags: no new CLI flag or `atmos.yaml` setting for v1.
- Add an aggregate CI path for multi-component `plan` runs only. Single-component plan/apply/deploy behavior remains unchanged.
- Collect per-node plan output/results during DAG execution, then write GitHub CI artifacts once after the scheduler finishes.
- Write one deterministic job summary with component counts, aggregate resource counts, failed/changed/no-change/skipped grouping, per-component rows, and details for failed/changed components.
- Write aggregate output variables once: `has_changes`, `has_errors`, `exit_code`, `resources_to_create`, `resources_to_change`, `resources_to_replace`, `resources_to_destroy`, `components_total`, `components_succeeded`, `components_failed`, `components_changed`, `components_no_changes`, `components_skipped`, `summary`, `command`, `stack`, and `component`.
- Define aggregate exit-code semantics as `1` if any component failed, else `2` if any component changed, else `0`.
- Preserve optional PR comments and status contexts behind existing `ci.comments` and `ci.checks` settings, with deterministic serialized writes.
- Avoid concurrent writes to `$GITHUB_OUTPUT` and `$GITHUB_STEP_SUMMARY`.

Validation focus preserved for regressions:

- Unit-test aggregate Terraform CI rendering with mixed no-change, changed, failed, skipped, and output-only results.
- Unit-test aggregate output variables and exit-code rules.
- Unit-test command wiring so multi-component plan CI uses aggregate output while single-component plan still uses the existing hook path.
- Unit-test optional comments/statuses with mock providers to prove deterministic serialized calls and existing config gates.
- Regression-test `$GITHUB_STEP_SUMMARY` and `$GITHUB_OUTPUT` temp files under `GITHUB_ACTIONS=true`.
- Race-test the aggregate collector and relevant scheduler adapter paths.

## PR 8: TUI/Interactive Terraform Output

PR 8 follows the merged GitHub CI output work. It should focus on non-CI interactive plan/apply/destroy rendering without changing CI output behavior.

Review focus:

- Keep GitHub CI rendering unchanged.
- Preserve scheduler semantics, hooks, summaries, logs, and sequential UX.
- Make local terminal output understandable for concurrent plan/apply/destroy runs.
- Do not introduce new execution semantics or mixed-type behavior.

## Review and Merge Model

The first seven rollout PRs have merged into `main`, and PR 8 should branch from current `main`. If GitHub stacked PRs later become available for this repository, the remaining planned branches can be linked into official stack metadata, but this tracker should continue to represent the work in normal PR terms.

Before moving from one PR to the next, confirm:

- The previous PR's package-level tests pass.
- New abstractions are used only by their intended integration point.
- No out-of-scope CLI behavior has changed.
- The next PR branches from `main` unless the prior planned branch is intentionally still open.

## Next Step Prompt

Start PR 8 from current `main` on `codex/dag-terraform-interactive-output`:

- Build TUI/interactive Terraform output for non-CI plan/apply/destroy runs.
- Preserve merged GitHub CI rendering and native CI hooks.
- Keep scheduler semantics unchanged: fail-fast, keep-going, skipped dependents, concurrency limits, workdir locking, destroy reverse ordering, and plan exit-code handling must not change.
- Do not introduce mixed-type DAG adapter behavior in PR 8; that remains PR 9.
