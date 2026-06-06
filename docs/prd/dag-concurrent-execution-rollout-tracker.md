# DAG Concurrent Execution Rollout Tracker

This tracker coordinates the DAG concurrent execution rollout as a sequence of small, reviewable PRs. The primary design source remains `docs/prd/dag-concurrent-execution.md`; this file describes current rollout state, what each PR owns, and which behavior must remain unchanged while output and diagnostics work continues.

## Implementation Strategy

The execution foundation is now merged into `main`. PR 1 through PR 6 established process stream injection, the generic scheduler, graph-backed Terraform routing, plan/apply/destroy concurrency, and affected routing. Remaining work should focus on making concurrent execution understandable and easy to verify without changing scheduler semantics.

Near-term layers are:

1. GitHub Actions CI output for concurrent Terraform plan verification.
2. TUI/interactive output for non-CI Terraform plan/apply/destroy runs.
3. Mixed-type DAG adapter support after Terraform-only execution and output UX are stable.
4. Advanced scheduling diagnostics and operator troubleshooting aids.

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
| PR 7 | [cloudposse/atmos#2577](https://github.com/cloudposse/atmos/pull/2577) | Open draft | `codex/dag-github-concurrent-output` | GitHub Actions-oriented output for concurrent Terraform plans, aggregate CI summaries, output variables, optional status contexts, and optional PR comments |
| Tracking | [cloudposse/atmos#2467](https://github.com/cloudposse/atmos/pull/2467) | Open draft | `codex/dag-concurrent-execution-tracker` | Rollout plan, findings, and PR coordination document |

## Planned PRs

| Step | Intended base | Proposed branch | Scope | Key gates / non-goals |
| --- | --- | --- | --- | --- |
| PR 8 | PR 7 / `main` after PR 7 merges | `codex/dag-terraform-interactive-output` | Add TUI/interactive output for non-CI Terraform plan/apply/destroy runs so concurrent execution is usable from a terminal. | Applies outside CI. Preserve GitHub CI rendering and native CI hooks. Do not introduce new execution semantics. |
| PR 9 | PR 8 | `codex/dag-mixed-type-adapters` | Add mixed-type DAG adapter support after Terraform-only execution and output UX are stable. Include explicit adapter boundaries for Terraform and future component types. | Requires repo-owner agreement on mixed-type ordering semantics. Do not rewrite unrelated executors. |
| PR 10 | PR 9 | `codex/dag-scheduling-diagnostics` | Add advanced scheduling diagnostics, graph/debug output, and operator-facing troubleshooting aids. | Should not change scheduling semantics. Keep diagnostics opt-in and safe for CI logs. |

## Current Findings

- The implementation stack PRs 1 through 6 have merged into `main`, and PR 7 is now open as draft [cloudposse/atmos#2577](https://github.com/cloudposse/atmos/pull/2577) from current `main`.
- `--ci` uses native CI provider detection. GitHub Actions auto-detects through `GITHUB_ACTIONS=true`; explicit `--ci` forces CI mode and falls back to the generic provider when no platform is detected.
- GitHub Actions output is currently written through provider output writers to `$GITHUB_STEP_SUMMARY` and `$GITHUB_OUTPUT`.
- Terraform CI plugin behavior is single-component shaped today: after-plan hooks parse one component's captured output, append one rendered summary, write output variables, optionally upload one planfile, optionally update one status context, and optionally post or update one PR comment.
- Multi-component Terraform plan/apply/deploy currently suppress the global PostRun hook and install `Info.PerComponentHook`, which runs inside each scheduled component. That keeps single-component attribution correct, but concurrent plan mode can produce nondeterministic GitHub output unless writes are centralized or serialized.
- The GitHub provider currently uses the commit Status API for status contexts. PR 7 should not depend on richer Checks API annotations or output.
- PR comment markers are per component (`<!-- atmos:ci:<command>:<component>:<stack> -->`). Aggregate comments, if added, need a distinct aggregate marker so they do not collide with existing component comments.
- The Terraform scheduler adapter already captures per-node stdout/stderr, exit code, changed state, status, timings, and log file paths; PR 7 should build CI aggregation from those existing outputs instead of scraping interleaved GitHub logs.

## PR 7: GitHub Actions Output For Concurrent Terraform Plans

Draft PR: [cloudposse/atmos#2577](https://github.com/cloudposse/atmos/pull/2577)

PR 7 makes concurrent Terraform plan verification usable in GitHub Actions with minimal friction.

Review focus:

- Keep existing public config and flags: no new CLI flag or `atmos.yaml` setting for v1.
- Add an aggregate CI path for multi-component `plan` runs only. Single-component plan/apply/deploy behavior remains unchanged.
- Collect per-node plan output/results during DAG execution, then write GitHub CI artifacts once after the scheduler finishes.
- Write one deterministic job summary with component counts, aggregate resource counts, failed/changed/no-change/skipped grouping, per-component rows, and details for failed/changed components.
- Write aggregate output variables once: `has_changes`, `has_errors`, `exit_code`, `resources_to_create`, `resources_to_change`, `resources_to_replace`, `resources_to_destroy`, `components_total`, `components_succeeded`, `components_failed`, `components_changed`, `components_no_changes`, `components_skipped`, `summary`, `command`, `stack`, and `component`.
- Define aggregate exit-code semantics as `1` if any component failed, else `2` if any component changed, else `0`.
- Preserve optional PR comments and status contexts behind existing `ci.comments` and `ci.checks` settings, with deterministic serialized writes.
- Avoid concurrent writes to `$GITHUB_OUTPUT` and `$GITHUB_STEP_SUMMARY`.

Validation focus:

- Unit-test aggregate Terraform CI rendering with mixed no-change, changed, failed, skipped, and output-only results.
- Unit-test aggregate output variables and exit-code rules.
- Unit-test command wiring so multi-component plan CI uses aggregate output while single-component plan still uses the existing hook path.
- Unit-test optional comments/statuses with mock providers to prove deterministic serialized calls and existing config gates.
- Regression-test `$GITHUB_STEP_SUMMARY` and `$GITHUB_OUTPUT` temp files under `GITHUB_ACTIONS=true`.
- Race-test the aggregate collector and relevant scheduler adapter paths.

## PR 8: TUI/Interactive Terraform Output

PR 8 moves after GitHub CI output so developer verification in CI is stable first. It should focus on non-CI interactive plan/apply/destroy rendering without changing CI output behavior.

Review focus:

- Keep GitHub CI rendering unchanged.
- Preserve scheduler semantics, hooks, summaries, logs, and sequential UX.
- Make local terminal output understandable for concurrent plan/apply/destroy runs.
- Do not introduce new execution semantics or mixed-type behavior.

## Review and Merge Model

The first six rollout PRs have merged into `main`, and PR 7 is now open as a draft against `main`. If GitHub stacked PRs later become available for this repository, the remaining planned branches can be linked into official stack metadata, but this tracker should continue to represent the work in normal PR terms.

Before moving from one PR to the next, confirm:

- The previous PR's package-level tests pass.
- New abstractions are used only by their intended integration point.
- No out-of-scope CLI behavior has changed.
- The next PR branches from `main` unless the prior planned branch is intentionally still open.

## Next Step Prompt

Continue PR 7 review and hardening from draft [cloudposse/atmos#2577](https://github.com/cloudposse/atmos/pull/2577):

- Verify the GitHub Actions aggregate summary/output shape against a real concurrent Terraform plan job.
- Confirm optional `ci.comments` and `ci.checks` behavior remains gated and deterministic.
- Keep scheduler semantics unchanged: fail-fast, keep-going, skipped dependents, concurrency limits, workdir locking, and plan exit-code handling must not change.
- Do not change interactive/TUI output in PR 7; that remains PR 8.
