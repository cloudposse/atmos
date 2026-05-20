# DAG Concurrent Execution Rollout Tracker

This tracker coordinates the DAG concurrent execution rollout as a sequence of small, reviewable PRs. The primary design source remains `docs/prd/dag-concurrent-execution.md`.

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

## Next Step Prompt

Start from `codex/dag-process-io-foundation` and implement PR 2 only:

- Add `pkg/scheduler` with `Node`, `Status`, `NodeResult`, `AggregateResult`, `Dispatcher`, `Scheduler`, and `Orchestrator`.
- Use a ready-queue scheduler with a bounded worker pool from the beginning.
- Keep scheduler nodes as scheduling data only; do not put execution closures on nodes.
- Keep the scheduler generic with no Terraform coupling.
- Add deterministic result ordering for JSON summaries.
- Cover linear chain, diamond, fan-out, fan-in, fail-fast, keep-going, and destroy reverse blocking semantics with unit tests.
- Do not change Terraform CLI routing, add Terraform adapters, or expose concurrency flags in PR 2.
