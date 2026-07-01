# PRD: Parallel, Matrix, and Background Workflow Steps

## Overview

Atmos workflows execute steps sequentially by default. This PRD covers the concurrency story for
workflow steps in two parts:

- **Shipped** — the `parallel` and `matrix` control steps that run sibling work concurrently through a
  bounded, dependency-aware scheduler with parent-owned output and explicit failure semantics.
- **Implemented (v1)** — a `background: true` container step plus `wait` / `wait-all` / `cancel` action
  steps that add *imperative* async: start a long-running container service, keep going, then synchronize
  on its readiness or tear it down later. Readiness reuses the existing container `healthcheck:`.

Together these give Atmos workflows real orchestration — structured fan-out *and* service lifecycle —
without dropping into shell glue.

> **Why this PRD exists:** the `parallel`/`matrix` control steps shipped (branch
> `osterman/parallel-step-type`) straight to a blog post, step-type docs, and `examples/parallel-steps/`
> without a PRD. This document backfills that record and folds in the `background`/`wait`/`cancel`
> design as one coherent picture. The proposed half was independently in design before GitHub's
> [2026-06-25 "Actions steps can now be run in parallel"](https://github.blog/changelog/2026-06-25-actions-steps-can-now-be-run-in-parallel/)
> announcement, which formalized the `background` + `wait`/`wait-all`/`cancel` vocabulary we adopt here.

## Problem Statement

Workflows encode the operational knowledge that should not live in someone's shell history: run the
checks, build the thing, deploy dependencies, summarize what happened. When every step is sequential, a
workflow with four independent checks takes the sum of all four runtimes.

The usual workaround is shell scripting — background jobs (`&`), `wait`, temp files, and hand-rolled log
prefixes. That works until it doesn't:

- Output from concurrent commands interleaves into unreadable logs.
- Failure behavior is implicit and different in every script.
- Dependency relationships are hidden in shell control flow.
- Local workflows and CI matrices drift apart.

Infrastructure automation should not force a choice between "simple but slow" and "fast but fragile."

## Goals

1. Run independent workflow steps concurrently with bounded parallelism. *(Shipped)*
2. Express dependencies between concurrent steps declaratively via `needs`. *(Shipped)*
3. Keep concurrent output readable with parent-owned rendering. *(Shipped)*
4. Make failure behavior part of the workflow contract, not buried in scripts. *(Shipped)*
5. Fan a single step template across a matrix of axes. *(Shipped)*
6. Start a step in the background, continue the workflow, and later synchronize on it (`wait`) or
   tear it down (`cancel`) — enabling long-running local services (emulators, registries, k3s,
   devcontainers) inside a workflow. *(Proposed)*

## Non-Goals

1. **GitHub Actions `parallel`-as-sugar.** Atmos `parallel` is a structured DAG block, not syntactic
   sugar for "background a group + wait-all." See [Relationship to GitHub Actions](#relationship-to-github-actions).
2. **A new readiness mechanism.** Container readiness reuses the existing `healthcheck:` +
   `container.WaitHealthy`; v1 adds no `ready:` field. A non-Docker readiness probe (tcp/http/log) is
   deferred until the non-container (shell/atmos) background `Runner` lands.
3. **Interactive child steps inside concurrent groups.** Prompts, pagers, spinners, editors, and
   terminal-owning renderers stay outside concurrent groups for now.
4. **Conditional branching.** No if/else in workflows (use `when` / shell).

---

## Shipped: `parallel` control step

**Status: Shipped.**

The `parallel` step runs its child steps concurrently instead of one after another, with bounded
concurrency, sibling dependencies, configurable failure behavior, and parent-owned output. Child steps
must be non-interactive command steps — `shell`, `atmos`, or `sleep`.

```yaml
steps:
  - name: checks
    type: parallel
    max_concurrency: 2
    fail:
      mode: wait_all
    output:
      mode: grouped
      order: completion
      show_summary: true
      prefix: "{{ .step.name }}"
    steps:
      - name: lint
        type: shell
        command: make lint

      - name: unit
        type: shell
        command: make test

      - name: package
        type: shell
        needs: [lint, unit]
        command: ./scripts/package.sh
```

### Fields

- **`steps`** (required) — child steps to run concurrently; each must be `shell`, `atmos`, or `sleep`.
- **`max_concurrency`** — maximum children running at once. Defaults to unbounded (all eligible
  children start together).
- **`fail`** — failure behavior for the group (see below).
- **`output`** — how the parent renders child output (see below).
- Child **`needs`** — list of sibling names that must complete successfully before the child starts. A
  child whose dependency fails or is skipped is itself skipped.

### Failure behavior

```yaml
fail:
  mode: wait_all       # wait_all (default) | fail_fast | best_effort
  max_failures: 0      # 0 (default) means no limit
```

- **`wait_all`** (default) — let running children finish, skip dependents of failed children, then
  fail the parent if any child failed.
- **`fail_fast`** — cancel pending and running children once the failure threshold is reached.
- **`best_effort`** — record failures and skip dependents, but let the parent succeed.
- **`max_failures`** — failures tolerated before `fail_fast` cancels the group; `0` means unlimited.

### Output

The parent owns rendering for the whole group so concurrent output stays readable.

```yaml
output:
  mode: grouped        # grouped (default) | prefixed | none
  order: completion    # completion | definition (grouped only)
  show_summary: true   # print success/failed/skipped/canceled counts
  prefix: "{{ .step.name }}"
```

- **`grouped`** (default) — capture each child's output and print it as a labeled block when the child
  finishes. `order` is `completion` (as children finish) or `definition` (declared order).
- **`prefixed`** — stream child output live, prefixing every complete line with the child's label.
- **`none`** — suppress child output (metadata still captured).
- **`show_summary`** — print a summary line, e.g.
  `[checks] summary: 2 succeeded, 0 failed, 0 skipped, 0 canceled`.
- **`prefix`** — Go template for the per-child label, evaluated with the child step context.

---

## Shipped: `matrix` control step

**Status: Shipped.**

The `matrix` step expands a set of axes into a Cartesian product and schedules the generated child
steps through the same scheduler as `parallel`. It accepts `steps`, `max_concurrency`, `fail`, and
`output` exactly as `parallel` does, plus the axes:

```yaml
steps:
  - name: test-matrix
    type: matrix
    max_concurrency: 3
    output:
      mode: grouped
      order: definition
    matrix:
      os: [linux, darwin]
      go: ["1.22", "1.23"]
    steps:
      - name: test
        type: shell
        command: make test OS={{ .matrix.os }} GO_VERSION={{ .matrix.go }}
```

- **`matrix`** (required) — map of axis name → list of string values. Atmos builds the Cartesian
  product and generates one set of child steps per combination.
- Each generated child references its combination via the `matrix` template namespace
  (`{{ .matrix.<axis> }}`) in `command`, `stack`, `env`, and `timeout`.
- Generated steps are named after the parent and their axis values so they stay distinct in output and
  summaries. Steps within a single generated combination run in declared order unless they use `needs`.

---

## Shipped: Architecture

**Status: Shipped.**

- **Scheduler** — `pkg/scheduler/` provides a ready-queue worker pool (`sync.WaitGroup` + workers,
  bounded by `max_concurrency`) with context cancellation for fail-fast.
- **Orchestration** — `pkg/workflow/control.go`, `control_executor.go`, and `control_matrix.go` build
  the dependency graph (and expand the matrix), run children through the scheduler, aggregate results,
  and render parent-owned output.
- **Registration** — `pkg/runner/step/parallel.go` registers the `parallel`/`matrix` types but
  intentionally returns an error from `Execute`: concurrent execution requires the workflow executor
  context, so the workflow layer owns it.
- **Schema** — `pkg/schema/workflow.go`. Control fields on `WorkflowStep` (≈ lines 318–322): `Steps`,
  `MaxConcurrency`, `Matrix`, `Fail`. Structured output is `ParallelOutputConfig` (`Mode`, `Order`,
  `ShowSummary`, `Prefix`) and failure is `ParallelFailConfig` (`Mode`, `MaxFailures`).
- **Validation** — `pkg/schema/task_validate.go`: `validateConcurrentChild` enforces the
  non-interactive `shell`/`atmos`/`sleep` constraint; `validateNeedsGraph` detects cycles.

---

## Implemented (v1): Background container services (`background: true`)

**Status: Implemented (v1 — container services only).**

A container step with `background: true` starts detached and the workflow **immediately continues** to
the next step. The container runtime (Docker/Podman) supervises the process; Atmos reuses its existing
long-lived container lifecycle (`container.Up`/`WaitHealthy`/`Down`) rather than a new supervisor. When
the step declares a `healthcheck:` (under `with:`), Atmos blocks until it is healthy before the next
step (the **implicit readiness gate**). A later `wait`/`wait-all` re-checks readiness; `cancel` tears it
down; anything still running at the end of the workflow is auto-torn-down.

```yaml
steps:
  - name: emulator
    type: container
    action: run
    background: true                 # start detached, keep going
    with:
      image: localstack/localstack
      ports: [{ host: 4566, container: 4566 }]
      healthcheck:                   # readiness gate — reuses the existing container healthcheck
        test: ["CMD", "curl", "-f", "http://localhost:4566/_localstack/health"]
        interval: 5s
        retries: 10
        start_period: 30s
  # implicit gate: because the emulator has a healthcheck, Atmos blocks here until it is HEALTHY
  - name: apply
    type: atmos
    command: terraform apply vpc -s dev

  - type: cancel                     # graceful teardown → container.Down (stop+remove)
    for: emulator
```

v1 supports `background: true` only on `type: container`; shell/atmos background (a goroutine-supervised
process with a non-Docker readiness probe) is a deliberate follow-up behind the `pkg/background`
`Runner`/`Handle` seam, requiring no change to the workflow orchestration.

### Naming decision: `background`, not `async`

We use **`background`** — the most intuitive word and now the ecosystem-standard term (GitHub Actions
uses `background: true`).

`WorkflowStep` already has a `Background string` field meaning **style/color** (`pkg/schema/workflow.go:250`).
That is **not** a reason to avoid the name. Property names do not need to be globally unique across step
types — the apparent clash is an artifact of every step type sharing one flat `WorkflowStep` struct, not
a naming rule. Style/color `background` applies to style/output steps; execution `background` applies to
command steps; they never carry both meanings on the same step.

Atmos already resolves exactly this class of overlap: the `output` key is polymorphic today.
`WorkflowStep.UnmarshalYAML` (`workflow.go:330`) routes `output:` to either a scalar string mode or a
structured `ParallelOutputConfig` mapping via `decodeWorkflowStepOutput` (`workflow.go:339`). The same
approach disambiguates `background` by value type:

- `background: true` (bool) → async execution marker.
- `background: blue` (string) → style color.

**Rejected alternative — `async: true`.** Avoids the overlap by sidestepping it, one word, no
underscore. Rejected because it diverges from the now-standard ecosystem term for no real benefit once
the polymorphic-unmarshal precedent removes the only objection to `background`. Other rejected names:
`detach`/`detached` (wrong — implies Atmos stops supervising), `daemon` (too service-specific),
`wait: false` (negative boolean; and `wait` belongs to an action step).

**Cost to manage:** bool-vs-string polymorphism under one key is more fragile than `output`'s
scalar-vs-mapping split, so validation must assert that a given step type honors only one meaning.

### Semantics

- Start the step's command, continue scheduling subsequent steps.
- Keep the process under Atmos supervision (reap/cancel before the enclosing workflow exits).
- A background step's declared `Outputs` become visible to the `Variables` map only **after** a matching
  `wait` / `wait-all` (or `cancel`) resolves.

---

## Implemented (v1): `wait`, `wait-all`, and `cancel` action steps

**Status: Implemented (v1).**

These are **action steps** (new step types `wait`/`wait-all`/`cancel`), not properties — matching the
intuition that "wait reads like a step that waits on a process group, not a flag," and aligning with
GitHub's vocabulary. They name their target background step(s) with a dedicated **`for:`** key (scalar
or list). `needs` is left untouched for work-ordering.

```yaml
- type: wait
  for: [emulator]           # block until the named background step(s) are READY (healthy)
- type: wait-all            # block until all background steps in scope are ready
- type: cancel
  for: emulator             # gracefully tear down (container.Down: stop+remove)
```

- **`wait` semantics are lifecycle-aware:** for a service, `wait`/`wait-all` block until **ready
  (healthy)** — *never* "until exit" (a service never exits on its own). The readiness check reuses
  `container.WaitHealthy`, which fails fast on a terminal `unhealthy` state.
- **`cancel`** is the teardown signal: `container.Down` (SIGTERM → timeout → SIGKILL, then remove). It
  is how a service ends.
- **End-of-scope = auto-teardown.** Any background step still running when the workflow ends is
  automatically stopped (`StopAll`) — *not* implicitly waited (a service would hang forever). An
  explicit `cancel` retires the step first so it is not stopped twice.

---

## Implemented (v1): Service lifecycle & readiness (reuses existing health checks)

**Status: Implemented (v1).**

The high-value pattern is a supervised container service whose lifetime spans several later steps —
start the dependency (emulator, registry, k3s, Postgres), run work against it, tear it down. Readiness
is **not a new mechanism**: it reuses the existing container **`healthcheck:`** (`ContainerHealthCheck`,
the Docker-Compose shape) plus **`container.WaitHealthy`** (`pkg/container/wait.go`). A backgrounded
container with a healthcheck blocks until healthy before the next step; without one, "started" is
treated as "ready". This is exactly the Atmos emulator/devcontainer/registry-cache story, declarative
and identical locally and in CI.

```yaml
steps:
  - name: localstack
    type: container
    action: run
    background: true
    with:
      image: localstack/localstack
      healthcheck:
        test: ["CMD", "curl", "-f", "http://localhost:4566/_localstack/health"]
        interval: 5s
        retries: 10
        start_period: 30s
  - name: apply
    type: atmos
    command: terraform apply vpc -s dev   # safe: emulator is already healthy
  - type: cancel
    for: localstack
```

**Deferred to a future version (not v1):** a readiness probe for **non-container** background steps
(shell/atmos starting a bare process, which has no Docker healthcheck) — e.g. a tcp/http/log probe. That
arrives with the shell/atmos `Runner` implementation, designed against the existing health-check
machinery rather than as a parallel `ready:` mechanism.

---

## Relationship to GitHub Actions

GitHub's parallel-steps feature introduced four keywords:

| GitHub | Atmos mapping |
| --- | --- |
| `background: true` (property) | `background: true` (property) — adopted |
| `wait` / `wait-all` (keywords) | `wait` / `wait-all` (action steps) — adopted |
| `cancel` (keyword) | `cancel` (action step) — adopted |
| `parallel` (sugar for background-group + wait-all) | **Not adopted as sugar.** Atmos `parallel` already means a richer structured DAG block. |

**Watch the same-word trap.** GitHub's `parallel` is syntactic sugar; Atmos `parallel` is a structured
concurrent group with `needs` (DAG), `matrix` expansion, and `fail` modes — capabilities GitHub's
`parallel` does not have. The `background`/`wait`/`cancel` trio is the *imperative complement* that
sits in a normal sequential step list; it does not redefine the `parallel` block. Keep the two distinct.

---

## Implemented (v1): Validation rules

**Status: Implemented** in `pkg/schema/task_validate.go` (`validateBackgroundSteps`):

- `background: true` is supported only on `type: container` in v1; rejected (with a "shell/atmos
  background is planned" hint) on other types.
- Background steps must be non-interactive (no `tty`/`interactive`).
- `wait`/`cancel` `for:` targets must reference a `background` step declared **earlier** in the workflow.
- Double-`cancel` (and `wait` after `cancel`) is rejected: a cancelled target is retired from the live set.
- The polymorphic `background` key is disambiguated by value type in `WorkflowStep.UnmarshalYAML`
  (bool → async marker, string → style color), so a step never carries both meanings.

## Implemented (v1): Architecture notes

**Status: Implemented.** The scheduler is *not* reused (it is blocking/join-all and cannot supervise an
unjoined, individually-cancellable task). Instead:

- `pkg/background` — generic `Runner`/`Handle` interfaces + a run-scoped `Registry` (`StopAll` for
  end-of-scope teardown).
- `pkg/workflow/background_container.go` — the v1 `ContainerRunner`/`containerHandle`, which reuse
  `container.Up` (detached start), `container.WaitHealthy` (readiness), and `container.Down` (teardown).
  No goroutine is needed — the container runtime is the supervisor.
- `pkg/workflow/background.go` — `StartBackground` (start + implicit readiness gate),
  `WaitBackground`/`WaitAllBackground`, `CancelBackground`.
- `pkg/runner/step/background_steps.go` — registers the `wait`/`wait-all`/`cancel` step types.
- `internal/exec/workflow_utils.go` — inline call sites only: a run-scoped `runCtx` + `background.Registry`,
  dispatch in the step loop, and a deferred `StopAll` for auto-teardown (on success and on error).

## Resolved decisions

- **End-of-scope policy:** auto-teardown (`StopAll`), not implicit wait-all — a service never exits, so
  waiting would hang. `wait`/`wait-all` is the explicit opt-in to block for readiness.
- **Readiness:** reuses the existing container `healthcheck:` + `container.WaitHealthy`; no new `ready:`
  mechanism.
- **`background` scope (v1):** top-level sequential steps. Background **inside** `parallel`/`matrix`
  groups, cross-scope `wait`/`cancel`, and a non-container (shell/atmos) `Runner` with its own readiness
  probe are follow-ups behind the same seam.

## Open Questions (follow-ups)

- Background steps inside control groups (group-boundary auto-teardown) and cross-scope `wait`/`cancel`.
- Output rendering for a long-lived background container that outlives many subsequent steps.
- A non-Docker readiness probe (tcp/http/log) for the future shell/atmos `Runner`.

## References

- Shipped step docs: `website/docs/workflows/workflows/workflow/steps/type/parallel.mdx`,
  `.../type/matrix.mdx`.
- Shipped changelog: `website/blog/2026-06-20-parallel-matrix-workflow-steps.mdx`.
- Runnable example: `examples/parallel-steps/`.
- Scheduler/concurrency PRD (component-level DAG): `docs/prd/dag-concurrent-execution.md`.
- Workflow step types PRD: `docs/prd/workflow-step-types.md`.
- GitHub Actions announcement:
  https://github.blog/changelog/2026-06-25-actions-steps-can-now-be-run-in-parallel/
