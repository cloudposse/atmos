# DAG Concurrent Execution Rollout Tracker

This tracker coordinates the DAG concurrent execution rollout as a sequence of small, reviewable PRs. The primary design source remains `docs/prd/dag-concurrent-execution.md`; this file describes how the work is split, what each PR owns, and which behavior must remain unchanged until the correct layer is ready.

## Implementation Strategy

The rollout is intentionally stacked. Each PR introduces one architectural layer or one routing change, with tests at that layer, before the next PR depends on it. This keeps review focused and prevents concurrency behavior from being mixed with unrelated Terraform routing, auth, store, or CI lifecycle changes.

The implementation layers are:

1. `pkg/process` and `pkg/io` foundations: subprocess lifecycle, stream injection, masking, and per-node stream composition.
2. `pkg/scheduler` core: generic DAG scheduling with ready queues, bounded workers, deterministic results, and no Terraform coupling.
3. `pkg/scheduler/adapters`: Terraform-first adapter code that translates Atmos component execution into scheduler-dispatched work.
4. Terraform bulk routing consolidation: move `--all`, `--components`, and `--query` onto one graph-backed path while keeping execution sequential.
5. Concurrency enablement: turn on `--max-concurrency` first for plan, then apply/destroy after safety and finalize semantics are implemented.
6. Selector unification and expansion: move `--affected` onto the shared path, then add mixed-type adapters.
7. Operational enhancements: diagnostics, per-type pools, resumability, richer progress UX, and graph visualization.

Concurrency must not become user-visible until the Terraform bulk routing path is consolidated. The current general CLI path still uses `ExecuteTerraformQuery()` for multi-component execution, so enabling the scheduler too early would create a parallel implementation that is hard to reach, hard to validate, and likely to miss auth/store behavior from the query path.

## Cross-Cutting Rules

- Use `dependencies.components` as the primary dependency source. Use `settings.depends_on` only as a compatibility fallback.
- Keep `pkg/dependency` as graph/data infrastructure. Do not put orchestration behavior there.
- Keep scheduler nodes as scheduling data. Execution work belongs behind dispatchers/adapters, not closures embedded in nodes.
- Use ready-queue scheduling from the first scheduler PR. Do not add a level-based interim scheduler.
- Preserve `--max-concurrency 1` as equivalent to current sequential behavior.
- Preserve native CI plan/apply/deploy capture and finalize behavior from the existing CI integration.
- Do not add new files under `internal/exec`; use existing files there only as shims or integration points.
- Concurrent apply/destroy must be non-interactive and require `-auto-approve`.
- Terraform node completion means subprocess execution, hooks, store writes, and CI finalize work have all completed.
- Keep each PR reviewable: tests should prove the layer being introduced, and later PR behavior should not leak into earlier PRs.

## Branching

- PR 1 starts from `upstream/main`.
- PR 2 starts from `codex/dag-process-io-foundation`.
- Each later PR starts from the previous rollout branch until the stack is ready to merge or rebase.
- Keep each PR self-contained and avoid enabling concurrency before the graph-backed Terraform bulk path is consolidated.

## Rollout

| Step | Branch | PR | Scope | Status |
| --- | --- | --- | --- | --- |
| PR 1 | `codex/dag-process-io-foundation` | cloudposse/atmos#2459 | `pkg/process`, `pkg/io` stream foundations, shell wrapper integration, `runTerraformShow` stdout injection | Open |
| PR 2 | `codex/dag-scheduler-core` | TBD | Generic `pkg/scheduler` ready-queue scheduler, orchestrator, deterministic aggregate results, unit tests | Next |
| PR 3 | `codex/dag-terraform-graph-bulk-path` | TBD | Terraform adapter and graph-backed sequential consolidation for `--all`, `--components`, and `--query` | Planned |
| PR 4 | `codex/dag-concurrent-terraform-plan` | TBD | `--max-concurrency` for concurrent Terraform plan on the consolidated path | Planned |
| PR 5 | `codex/dag-concurrent-terraform-apply-destroy` | TBD | Concurrent apply/destroy safety, auto-approve validation, finalize semantics, reverse destroy blocking | Planned |
| PR 6 | `codex/dag-affected-scheduler-routing` | TBD | Route `--affected` onto the shared scheduler-backed executor | Planned |
| PR 7 | `codex/dag-mixed-type-adapters` | TBD | Packer and provider-backed component adapters for mixed-type DAGs | Planned |
| PR 8 | `codex/dag-scheduler-diagnostics` | TBD | Additive scheduling diagnostics, progress UX, pools, resumability, and graph visualization as justified | Planned |

## PR Details

### PR 1: Process and I/O Foundation

Owns subprocess and stream primitives only. `ExecuteShellCommand()` remains backward compatible but delegates to `pkg/process`, and `runTerraformShow()` stops mutating global `os.Stdout`. This is the safety prerequisite for later concurrent workers because subprocess output cannot rely on shared global stdout/stderr.

Review focus:

- Exit code preservation for Terraform detailed exit codes.
- Context-aware process execution and cancellation reporting.
- Existing CI stdout/stderr capture remains a tee, not a redirect.
- Per-node stream primitives can label and fan out to terminal, file, and capture sinks.

### PR 2: Scheduler Core

Adds the generic scheduler without touching Terraform routing. The scheduler owns dependency-aware orchestration, ready queues, bounded workers, fail-fast and keep-going behavior, destroy reverse blocking semantics, and deterministic aggregate result ordering.

Review focus:

- No Terraform imports or command assumptions.
- Nodes remain data only.
- Dispatcher is the execution boundary.
- Tests cover graph shapes and failure behavior in isolation.

### PR 3: Terraform Graph-Backed Bulk Path Consolidation

Introduces the Terraform adapter and moves non-affected bulk selectors onto one graph-backed executor while keeping concurrency fixed at one worker. This PR is where existing query-path behavior must be preserved: auth setup, store resolver bridging, YAML function processing, per-component command construction, hooks, and current sequential UX.

Review focus:

- `--all`, `--components`, and `--query` share one path.
- Dependency ordering prefers `dependencies.components`.
- Existing auth/store behavior from `ExecuteTerraformQuery()` remains intact.
- No visible concurrency or `--max-concurrency` behavior is introduced.

### PR 4: Concurrent Terraform Plan

Adds `--max-concurrency` for the consolidated Terraform bulk path and enables concurrent `plan` only. This is the first user-visible concurrency PR because plan is the safest Terraform command surface.

Review focus:

- Workdir and non-interactive auth preflight validation.
- Per-node output labeling, log files, and capture remain isolated.
- Plan exit codes preserve `0`, `2`, and failure semantics.
- `--max-concurrency 1` remains behaviorally sequential.

### PR 5: Concurrent Terraform Apply and Destroy

Extends concurrency to mutating Terraform commands after safety checks are in place. Apply/destroy require non-interactive execution and `-auto-approve` when concurrency is greater than one. Destroy uses reverse dependency blocking so prerequisites are not destroyed after dependent failure.

Review focus:

- Dependents release only after full node finalize completion.
- CI finalize and store writes are part of node completion.
- Signal handling cancels gracefully.
- Unsupported interactive combinations fail before any execution starts.

### PR 6: Affected Routing

Routes `--affected` through the same scheduler-backed executor while preserving the existing DescribeAffected selector behavior and `--include-dependents` semantics.

Review focus:

- Affected-set semantics remain unchanged.
- Dependency closure is explicit and tested.
- Fail-fast and keep-going behavior is consistent with other selectors.

### PR 7: Mixed-Type DAGs

Adds Packer and provider-backed component adapters after Terraform concurrency is correct. This makes the scheduler component-type agnostic without changing scheduler internals.

Review focus:

- Mixed Terraform + Packer DAGs work.
- Mixed Terraform + provider-backed DAGs work.
- `dependencies.components.kind` drives cross-type graph construction.

### PR 8: Advanced Scheduling and Diagnostics

Adds operational features only after the core path is correct. Candidate features include per-type concurrency pools, resumability, richer progress UX, graph visualization, and critical-path prioritization if profiling justifies it.

Review focus:

- Features are additive.
- Core scheduler semantics do not regress.
- Diagnostics explain graph and execution state without changing outcomes.

## Review and Merge Model

The PRs can be reviewed as a stack, but each PR should have its own clear validation. If the stack becomes too large, rebase later PRs as earlier PRs merge. Avoid combining PR 3 and PR 4 unless there is no practical alternative, because routing consolidation and first concurrency enablement exercise different risk areas.

Before moving from one PR to the next, confirm:

- The previous PR's package-level tests pass.
- New abstractions are used only by their intended integration point.
- No out-of-scope CLI behavior has changed.
- The next PR branches from the previous rollout branch unless the stack has been merged and rebased onto `upstream/main`.

## Next Step Prompt

Start from `codex/dag-process-io-foundation` and implement PR 2 only:

- Add `pkg/scheduler` with `Node`, `Status`, `NodeResult`, `AggregateResult`, `Dispatcher`, `Scheduler`, and `Orchestrator`.
- Use a ready-queue scheduler with a bounded worker pool from the beginning.
- Keep scheduler nodes as scheduling data only; do not put execution closures on nodes.
- Keep the scheduler generic with no Terraform coupling.
- Add deterministic result ordering for JSON summaries.
- Cover linear chain, diamond, fan-out, fan-in, fail-fast, keep-going, and destroy reverse blocking semantics with unit tests.
- Do not change Terraform CLI routing, add Terraform adapters, or expose concurrency flags in PR 2.
