# PRD: DAG-Based Concurrent Execution

**Status:** Draft
**Version:** 2.0
**Last Updated:** 2026-03-16
**Author:** Erik Osterman

---

## Problem Statement

As infrastructure architectures grow, the number of components that need to be provisioned in a single operation grows with them. Today, Atmos executes components sequentially — even when components have no dependency relationship and could safely run in parallel. For large deployments with dozens or hundreds of components, this serialization is the dominant bottleneck.

Atmos has had the concept of component dependencies (`settings.depends_on`) for some time, and PR #1516 added a proper dependency graph package (`pkg/dependency/`) with topological sorting. However, the execution engine still processes the sorted graph one node at a time. There have been partial attempts to introduce concurrency, but nothing that addresses the full problem: a scheduling model that is DAG-aware, component-type-agnostic, and safe for production use.

This PRD defines Atmos's approach to concurrent execution. The goal is a concurrency model that:

1. **Works across component types** — Terraform, Packer, Ansible, and custom registry components can all participate in the same execution DAG
2. **Works within a component type** — Multiple Terraform components at the same dependency depth run concurrently
3. **Respects the dependency graph** — Components only execute when all their dependencies have completed successfully
4. **Is safe by default** — Sequential execution (`--max-concurrency 1`) remains the default; concurrency is opt-in
5. **Handles the hard cases** — Diamond dependencies (fan-out/fan-in), asymmetric graphs, failure propagation, destroy ordering

In order to get here, there are several related concerns this PRD also addresses: output isolation under concurrency, the relationship between the scheduler and legacy built-in component types (which don't use the component registry), stream injection tradeoffs, DAG visualization for debugging, and configuration of concurrency defaults.

---

## Related PRs and PRDs
- **PR #1405** — Merged (2025-08-17). Redefined `settings.depends_on` with cross-stack support via `stack` attribute, context variables (`namespace`, `tenant`, `environment`, `stage`), and Go template support in dependency declarations
- **PR #1516** — Merged (2026-03-10). Added `pkg/dependency/` graph package + `ExecuteTerraformAll()` with sequential dependency-ordered execution. PRD: `docs/prd/terraform-dependency-order.md`
- **PR #1876** — Merged. Workdir isolation for Terraform components (enabler for concurrent execution)
- **PR #1891** — Open. CI hooks, planfile infrastructure, `--ci` flag
- **PR #2127** — Merged (2026-03-04). Propagated component-type level dependencies (`terraform.dependencies.tools`) through stack processor — 3-scope merge chain
- **PR #2159** — Open (fork: shirkevich). Proposal document for concurrent component provisioning
- **PR #2193** — Open. **PREREQUISITE — merge before starting this work.** Introduces `dependencies.components` format with cross-type dependencies (`kind` field for terraform/helmfile/packer/plugin), file/folder monitoring (`kind: file`/`kind: folder`), Go template support, and list-based syntax replacing the old `settings.depends_on` map format. This PR provides the dependency declaration format that the scheduler will consume.

### How This PRD Differs from PR #2159's Proposal

| Aspect | PR #2159 Proposal | This PRD |
|--------|------------------|----------|
| **Scheduling model** | Level-based (Phase 1), ready-queue (later phase) | **Ready-queue from the start** — level-based is a known anti-pattern that Terragrunt already abandoned |
| **Scope** | Terraform-only | **Component-type-agnostic** — scheduler works across Terraform, Packer, Ansible, custom registry types |
| **Architecture** | Extends `internal/exec/terraform_all.go` | **Three-layer design** — `pkg/scheduler/` (scheduling), `pkg/process/` (subprocess lifecycle), `pkg/scheduler/adapters/` (tool-specific) |
| **Industry grounding** | References Atmos internals | **Benchmarked against Terragrunt, TerraMate, Make, Ninja, Bazel, Buck2** — all converge on ready-queue |
| **Foundation PRs** | Two foundation PRs (stream injection, routing consolidation) before any concurrency | **Foundation packages first** — `pkg/process/` + `pkg/io/` extensions land as Phase 1, scheduler as Phase 2 |
| **Mixed-type DAGs** | Not addressed | **First-class concern** — Packer AMI → Terraform EC2 dependencies |
| **Component registry** | Not addressed | **Adapter pattern** — `ComponentProviderAdapter` wraps `ComponentProvider.Execute()` for registered types |

**Key disagreement: Level-based vs ready-queue.** PR #2159 proposes level-based scheduling for Phase 1, citing simplicity and debuggability. However:
- Terragrunt explicitly abandoned level-based for ready-queue ([#3629](https://github.com/gruntwork-io/terragrunt/issues/3629))
- Every major build system (Make, Ninja, Bazel, Buck2) uses ready-queue
- Ready-queue is ~100 lines of Go with `errgroup` — not materially more complex
- Level-based wastes worker time on asymmetric graphs (common in real infra)

**Key agreement with PR #2159:** Both proposals agree on `--max-concurrency`, fail-fast semantics, output isolation, non-interactive requirement for concurrent apply, signal handling, auth pre-bootstrap, and the JSON summary contract.

---

## Industry Survey: How Others Solved This

### Terragrunt — The Cautionary Tale of Level-Based Execution

Terragrunt's history is the most instructive for Atmos because they started with the exact approach PR #2159 proposes (level/group-based), used it for years, then abandoned it.

**The old model (group-based / level-based):**
Terragrunt originally organized units into "run groups" — sets of units at the same depth in the DAG. All units in a group ran concurrently, but the next group waited for the entire previous group to finish.

**Why they abandoned it ([RFC #3629](https://github.com/gruntwork-io/terragrunt/issues/3629), filed 2024-12-05):**

Two problems drove the change:

1. **The "slowest unit" problem.** From the RFC:
   > *"There is wasted time in a run, as groups execute when they have no dependent groups they are waiting on. A group dependent on another group will only start running when the slowest Unit in the dependency completes."*

2. **Failure blast radius.** From the RFC:
   > *"Individual Units failing during runs can cause entire groups, and dependent groups to fail, ultimately meaning that individual failing Units can cause widespread failure for a Stack."*

The RFC includes timing diagrams proving that the worst case for runner pool equals the best case for level-based — it can never be slower, only faster.

**The new model (runner pool / ready-queue):**
- [PR #4434](https://github.com/gruntwork-io/terragrunt/pull/4434) (merged 2025-07-03) — Implemented `StackRunner` interface with both old (configstack) and new (runnerpool) backends
- [PR #4855](https://github.com/gruntwork-io/terragrunt/pull/4855) (merged 2025-09-22) — Added benchmarks comparing the two models with real performance data
- **v0.89.0** (2025-10-06) — Shipped as GA, explicitly marked as a **Breaking Change**, replacing group-based as the default
- Each unit tracks a `blocked-by` list; transitions to `ready` when the list empties
- `--parallelism N` controls worker pool size (default: unlimited)

**Migration pain points (relevant to our design):**
- [Issue #5035](https://github.com/gruntwork-io/terragrunt/issues/5035) — Runner pool initially didn't reverse dependency order for `destroy` operations
- [Issue #5192](https://github.com/gruntwork-io/terragrunt/issues/5192) — `--queue-strict-include` broke because the runner pool requires dependencies to be resolved *within the current run*, unlike the old model which checked for existing state

Community response to the RFC was positive. One user commented:
> *"From my experience in projects where we built many small units, this would be a total game changer and potentially lead to a huge speed increase!"*

### TerraMate — Never Used Level-Based

TerraMate uses structural ordering (parent stacks before children) plus explicit `before`/`after` attributes. Their `--parallel N` flag enables concurrent execution of independent stacks.

Notably, TerraMate **never adopted level-based execution**. In [terramate-io/terramate#2069](https://github.com/terramate-io/terramate/issues/2069), a contributor explicitly rejected a numbered-group representation as insufficient, explaining:
> *"the DAG itself is the complete representation of the execution flow"*

This confirms they use per-node-ready scheduling internally, not wave-based grouping.

### Build Systems — Universal Convergence on Ready-Queue

Every major build system uses the same fundamental pattern: **DAG + ready queue + bounded worker pool**.

| System | Scheduling Model | Notable Addition |
|--------|-----------------|-----------------|
| **Make** (`-j N`) | Ready-queue | The original. 1976. |
| **Ninja** | Ready-queue | **Resource pools** — per-rule concurrency limits (e.g., max 2 link steps) |
| **Bazel** | Ready-queue | **Critical path scheduling** — prioritizes the longest dependency chain |
| **Buck2** | Ready-queue | **Unified graph (DICE)** — no phase boundaries, cross-phase parallelism |

None of these systems use level-based execution. The pattern has been settled for decades.

### Beyond IaC — Task Runners, Build Systems, and Workflow Engines

To confirm this isn't just an IaC-specific pattern, we surveyed 10 non-Terraform tools that solve DAG-based concurrent execution. **Every single one uses ready-queue scheduling. None use level-based.**

| Tool | Language | Domain | Scheduling Model | Concurrency Mechanism |
|------|----------|--------|------------------|-----------------------|
| **[Taskfile](https://taskfile.dev)** (go-task) | Go | Task runner | Ready-queue | `errgroup.Go()` + semaphore (`acquireConcurrencyLimit`) |
| **[Dagger](https://dagger.io)** | Go | CI/CD pipelines | Lazy DAG (BuildKit) | BuildKit auto-parallelizes independent nodes |
| **[Apache Airflow](https://airflow.apache.org)** | Python | Workflow orchestration | Polling ready-queue | 3-step scheduler: create DagRuns → find schedulable TaskInstances → enqueue with pool slots |
| **[Nx](https://nx.dev)** | TypeScript | Monorepo builds | Ready-queue | `--parallel=N`, cross-target parallelism automatic |
| **[Turborepo](https://turbo.build)** | Rust | Monorepo builds | Graph walker | petgraph `Walker` emits ready nodes, tokio `Semaphore::new(concurrency)` |
| **[Gradle](https://gradle.org)** | Java | Build system | Ready-queue | Worker thread pool pulling from `MergedQueues`, `--parallel` flag |
| **[Pants](https://www.pantsbuild.org)** | Rust/Python | Build system | Ready-queue | Tokio runtime, fine-grained rule graph, uses all cores by default |
| **[Temporal](https://temporal.io)** | Go | Workflow engine | Event-sourced task queue | Workers long-poll, parallel branches via user code (goroutines/Promise.all) |
| **[Luigi](https://luigi.readthedocs.io)** (Spotify) | Python | Data pipelines | Lock manager | Central scheduler prevents duplicates, parallelism via multiple workers |
| **[Concourse CI](https://concourse-ci.org)** | Go | CI/CD | Explicit `in_parallel` step | `limit` parameter as semaphore, `fail_fast` for early termination |

**Notable implementation patterns:**
- **Go tools** (Taskfile, Dagger, Temporal): `errgroup` + semaphore is the dominant pattern
- **Rust tools** (Turborepo, Pants): tokio runtime + `Semaphore` / `FuturesUnordered`
- **Java** (Gradle): Worker thread pool with project-level locks
- **Python** (Airflow): Database-backed scheduling with pool slot limits

This confirms that `errgroup` + ready-queue is not just the right approach — it's the idiomatic Go approach, already used by Taskfile (the most popular Go task runner).

### Summary: Why Ready-Queue Is the Industry Standard

| Criterion | Level-Based | Ready-Queue |
|-----------|------------|-------------|
| **Throughput** | Waits for slowest node per level | Starts work as soon as dependencies satisfied |
| **Worst case** | Same as ready-queue | Same as level-based |
| **Best case** | Slower (idle workers waiting for level completion) | Optimal (no unnecessary waiting) |
| **Implementation complexity** | Slightly simpler (iterate levels) | ~100 lines of Go with `errgroup` |
| **Debuggability** | Can log "Level N complete" | Can log "Node X ready, dependencies satisfied" |
| **Industry adoption** | Terragrunt (abandoned), no major build system | Make, Ninja, Bazel, Buck2, Terragrunt (current), TerraMate |

The implementation complexity difference is negligible. The performance difference is real and grows with graph asymmetry — which is common in infrastructure (a VPC takes seconds, a database takes minutes).

---

## The Canonical Pattern: Ready-Queue Scheduler

The industry standard is **modified Kahn's algorithm with a ready queue and worker pool**:

```
1. Compute in-degree for every node (count of unsatisfied dependencies)
2. Seed ready queue with all zero-in-degree nodes (roots)
3. Workers pull from ready queue (bounded by --max-concurrency)
4. On node completion:
   a. Atomically decrement in-degree of all dependents
   b. Any dependent reaching in-degree 0 enters the ready queue
5. Repeat until queue empty + all workers idle, OR error
```

### Why This Beats Level-Based Execution

Level-based groups nodes by depth and waits for the entire level to finish before starting the next. Consider:

```
A (1 min) → C (1 min)
B (10 min) → C
```

- **Level-based**: Level 0 = {A, B}. Wait 10 min. Level 1 = {C}. Total: 11 min.
- **Ready-queue**: A finishes at 1 min, but C still blocked (in-degree=1). B finishes at 10 min, C becomes ready. Total: 11 min.

Same in this case, but consider a longer chain:

```
A (1 min) → C (1 min) → E
B (10 min) → D (1 min) → E
```

- **Level-based**: {A,B}=10min, {C,D}=1min, {E}=1min. Total: 12 min.
- **Ready-queue**: A@1min→C@2min, B@10min→D@11min, E@11min. Total: 12 min.

The real difference shows with **asymmetric diamonds** where branches have different depths. Ready-queue always starts work as soon as dependencies are satisfied, never waiting for unrelated work.

### Diamond Dependencies (Fan-Out/Fan-In)

```
    A
   / \
  B   C
   \ /
    D
```

Handled naturally by in-degree counting:
- D starts with in-degree=2
- B completes → D's in-degree drops to 1 (still blocked)
- C completes → D's in-degree drops to 0 → enters ready queue

No special case needed. This is the elegance of the pattern.

---

## What Atmos Has Today

### Dependency Graph (`pkg/dependency/`)
Fully implemented and shipped (#1516):
- `graph.go` — Graph structure, `AddNode()`, `AddDependency()`, cycle detection, path finding
- `sort.go` — Kahn's topological sort, `GetExecutionLevels()` (level-based grouping)
- `builder.go` — `GraphBuilder` for constructing graphs
- `filter.go` — Filter by type, stack, component; connected components

### Dependency-Ordered Execution (`internal/exec/terraform_all.go`)
- `ExecuteTerraformAll()` — Builds DAG from `settings.depends_on`, executes in topological order
- `buildTerraformDependencyGraph()` — Constructs graph from stack configs
- `executeInDependencyOrder()` — **Currently sequential** (iterates sorted nodes one by one)
- Reverse order for `destroy`
- Cross-stack dependency support

### Component Registry (`pkg/component/`)
- `registry.go` — Thread-safe global registry with `Register()`, `GetProvider()`, `ListProviders()`
- `provider.go` — `ComponentProvider` interface with `Execute()`, `GetType()`, etc.
- Ansible registered via `init()` — follows the pattern
- Terraform/Packer — legacy built-in, NOT registered in the component registry

### Existing Concurrency
- `describe_affected_utils_parallel.go` — WaitGroup + channels for parallel stack processing
- `stack_processor_process_stacks.go` — WaitGroup for parallel stack processing
- No errgroup, no semaphore patterns currently used

### I/O Layer (`pkg/io/`)
The I/O package provides the stream isolation primitives that per-node output will build on:
- `interfaces.go` — Core contracts: `Context` (combines streams + masker), `Streams`, `Masker`, `Config`
- `streams.go` — `maskedWriter` wraps `io.Writer` to transform content on each `Write()` call; `dynamicMaskedWriter` resolves writers at write time
- `context.go` — `io.Context` implementation: `Write()` is the choke point where all output flows through masking
- `global.go` — Global singletons: `Data` (stdout) and `UI` (stderr) writers
- `WithStreams()` option already supports custom streams (used in tests) — this is the extension point for per-node stream isolation
- Two-channel design: **Data** (stdout, pipeable) and **UI** (stderr, human-readable)

### Task Runner (`pkg/runner/`)
- `CommandRunner` interface with `RunShell()` and `RunAtmos()` — task-level execution (shell commands, atmos sub-commands)
- `Run()` / `RunAll()` with timeout support via `context.WithTimeout()`
- Interface-driven with `//go:generate mockgen` — already mockable
- This is **task-level** (workflow tasks with stack/command semantics), not subprocess-level. The new `pkg/process/` package will be lower-level: raw subprocess lifecycle with injected streams.

### Subprocess Execution (`internal/exec/shell_utils.go`)
- `ExecuteShellCommand()` — the refactoring target. Currently hardcodes `cmd.Stdout = ioLayer.MaskWriter(os.Stdout)` and `cmd.Stderr = ioLayer.MaskWriter(os.Stderr)`
- Merges environment: system + `atmos.yaml` global env + command-specific env
- Preserves exit codes: `exec.ExitError` → `errUtils.ExitCodeError`
- Propagates TTY: injects `ATMOS_FORCE_TTY=true` when parent has TTY
- `terraform_plan_diff.go` swaps global `os.Stdout` to capture output — race condition under concurrency

### Routing Gap
- `--all` for Terraform goes through `ExecuteTerraformAll()` (dependency-aware, sequential)
- `--components`, `--query` still route through `ExecuteTerraformQuery()` (no DAG awareness)
- No `--all` equivalent exists for other component types

---

## Recommended Architecture: Three-Layer Design

A scheduler solves DAG ordering and concurrency, but it does not by itself solve: stream isolation, signal forwarding / graceful shutdown, stdin policy for interactive commands, per-node log capture and labeling, or child-process cleanup and exit handling. Those are process-management concerns that deserve a separate reusable package boundary.

### Layer 1: `pkg/scheduler/` — Pure Scheduling

The scheduler is a component-type-agnostic DAG executor. **Nodes are pure scheduling data — they do not know how to execute themselves.** Execution is delegated to a `Dispatcher` interface, which decouples scheduling logic from process management and tool-specific concerns.

```go
// Node is pure scheduling data.
// It does not know how to execute itself.
type Node struct {
    ID        string // unique DAG node ID: "terraform/tenant1-ue2-dev/vpc"
    Component string
    Stack     string
    Type      string // "terraform", "packer", "ansible", etc.
    Ref       any    // tool-specific payload prepared earlier
}

// Dispatcher is called by the scheduler to execute a node.
// The orchestrator implements this interface.
type Dispatcher interface {
    Dispatch(ctx context.Context, node *Node) (*Result, error)
}

// Scheduler manages concurrent DAG execution
type Scheduler struct {
    graph          *dependency.Graph
    dispatcher     Dispatcher
    maxConcurrency int
    failFast       bool
    onNodeStart    func(node *Node)
    onNodeComplete func(node *Node, result *Result, err error)
}

// Run executes the DAG with bounded concurrency
func (s *Scheduler) Run(ctx context.Context) *AggregateResult
```

### Layer 2: `pkg/process/` — Generic Subprocess Lifecycle

A reusable package for subprocess management. Not Terraform-specific — usable anywhere Atmos needs to run a child process with controlled I/O and signal handling.

```go
// TaskSpec is the generic process contract.
// Adapters translate tool-specific nodes into this shape.
type TaskSpec struct {
    Command     []string
    Dir         string
    Env         []string
    Interactive bool
}

// Streams are injected by the orchestrator/process layer.
// This is what lets concurrent nodes avoid shared stdio.
type Streams struct {
    Stdin  io.Reader
    Stdout io.Writer
    Stderr io.Writer
}

// Result is the generic execution result returned by the process layer.
type Result struct {
    ExitCode   int
    StartedAt  time.Time
    FinishedAt time.Time
    Err        error
}

// Runner owns subprocess lifecycle.
// This is the reusable process package, not Terraform-specific code.
type Runner interface {
    Run(ctx context.Context, spec *TaskSpec, streams *Streams) (*Result, error)
}
```

**Runner responsibilities:**
- Spawn child process with injected streams (stdin/stdout/stderr)
- Forward SIGINT/SIGTERM to child process with configurable grace period
- Wait for child process, capture exit code
- Clean up on context cancellation

**Relationship to existing `pkg/runner/`:** The existing `CommandRunner` interface handles workflow task execution (shell tasks, atmos sub-commands with stack/command semantics). `pkg/process/` is lower-level — it manages raw subprocess lifecycle with injected streams. They are complementary, not competing. `pkg/runner/` could eventually be refactored to use `pkg/process/` internally.

### Layer 3: `pkg/scheduler/adapters/` — Tool-Specific Adapters

Adapters translate tool-specific semantics into the generic `TaskSpec` / `Result` contract. Each adapter handles preparation (auth, hooks, env, command construction) and finalization (hooks, store writes, post-processing).

```go
// Adapter owns tool-specific execution semantics.
// Scheduler uses adapters; adapters use the process layer.
type Adapter interface {
    Prepare(ctx context.Context, node *scheduler.Node) (*process.TaskSpec, error)
    Finalize(ctx context.Context, node *scheduler.Node, result *process.Result) error
}

// Registry resolves adapters by node type.
type Registry interface {
    Get(nodeType string) (Adapter, bool)
    MustGet(nodeType string) Adapter
}
```

**Concrete adapters:**

```go
// TerraformAdapter — in pkg/scheduler/adapters/terraform.go
type TerraformAdapter struct{}

func (a *TerraformAdapter) Prepare(ctx context.Context, node *scheduler.Node) (*process.TaskSpec, error) {
    info := node.Ref.(*TerraformInfo)
    // Tool-specific preparation:
    // - validate workdir
    // - resolve auth context
    // - build terraform args
    // - run before-hooks
    // - decide whether interactive mode is allowed
    return &process.TaskSpec{
        Command: []string{"terraform", "apply", "-auto-approve"},
        Dir:     info.Dir,
        Env:     info.Env,
    }, nil
}

func (a *TerraformAdapter) Finalize(ctx context.Context, node *scheduler.Node, result *process.Result) error {
    // - run after-apply hooks
    // - update stores
    // - publish plan/apply metadata
    return nil
}

// PackerAdapter — in pkg/scheduler/adapters/packer.go
// AnsibleAdapter — in pkg/scheduler/adapters/ansible.go
// ComponentProviderAdapter — wraps ComponentProvider.Execute() for registered types
```

**Why adapters live in `pkg/scheduler/adapters/` (not `internal/exec/`):** The long-term goal is to eliminate `internal/exec/`. No new files should be added to exec. Adapters call into existing exec functions where needed, but the adapter code itself lives in its own package.

### Orchestrator: `pkg/scheduler/orchestrator.go`

The orchestrator combines all three layers. It implements `scheduler.Dispatcher` and is the integration point where scheduling, process management, and tool-specific logic come together.

```go
// Orchestrator implements Dispatcher by combining process runner + adapter registry.
type Orchestrator struct {
    runner   process.Runner
    registry adapters.Registry
}

// Dispatch shows the intended boundary:
// scheduler drives execution order,
// adapter translates tool semantics,
// process runner owns subprocess execution.
func (o *Orchestrator) Dispatch(ctx context.Context, node *Node) (*Result, error) {
    adapter := o.registry.MustGet(node.Type)

    // Tool-specific preparation:
    // auth/bootstrap, hooks, cwd/env resolution, command construction
    spec, err := adapter.Prepare(ctx, node)
    if err != nil {
        return nil, err
    }

    // Create per-node isolated streams (see "Output Under Concurrency")
    streams := io.NewNodeStreams(node.ID, ...)

    // Generic subprocess lifecycle:
    // stream isolation, signal forwarding, wait/kill, exit handling
    result, err := o.runner.Run(ctx, spec, streams)
    if err != nil {
        return nil, err
    }

    // Tool-specific completion semantics:
    // hooks, store writes, planfile handling, post-processing
    if err := adapter.Finalize(ctx, node, result); err != nil {
        return nil, err
    }

    return result, nil
}
```

### Scheduler Implementation (Ready-Queue + errgroup)

The scheduling algorithm is unchanged from v1 — the only difference is that `node.Execute(ctx)` becomes `s.dispatcher.Dispatch(ctx, node)`:

```go
func (s *Scheduler) Run(ctx context.Context) *AggregateResult {
    result := &AggregateResult{}
    inDegree := computeInDegrees(s.graph)
    ready := make(chan *Node, s.graph.Size())
    done := make(chan struct{})

    // Seed with roots
    for _, node := range s.graph.Roots() {
        ready <- node
    }

    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(s.maxConcurrency)

    var mu sync.Mutex
    completed := 0
    total := s.graph.Size()

    // Scheduler loop — select on ctx.Done() to avoid deadlock on fail-fast
    for {
        select {
        case <-ctx.Done():
            result.Err = g.Wait()
            return result
        case node := <-ready:
            g.Go(func() error {
                nodeResult, err := s.dispatcher.Dispatch(ctx, node)

                mu.Lock()
                result.Nodes = append(result.Nodes, nodeResult)
                completed++
                // Decrement dependents' in-degrees
                for _, dep := range s.graph.Dependents(node.ID) {
                    inDegree[dep]--
                    if inDegree[dep] == 0 {
                        // Context-aware send to avoid blocking on cancellation
                        select {
                        case ready <- s.graph.GetNode(dep):
                        case <-ctx.Done():
                        }
                    }
                }
                if completed == total {
                    close(done)
                }
                mu.Unlock()

                if err != nil && s.failFast {
                    return err // cancels context via errgroup
                }
                return nil
            })
        case <-done:
            result.Err = g.Wait()
            return result
        }
    }
}
```

### Key Design Decisions

| Decision | Recommendation | Rationale |
|----------|---------------|-----------|
| Scheduling | Ready-queue (not level-based) | Industry standard, optimal throughput |
| Concurrency control | `--max-concurrency N` | Like Terragrunt's `--parallelism` |
| Default concurrency | 1 (sequential) | Safe default, opt-in parallelism |
| Failure mode | `--fail-fast` (default) vs `--keep-going` | Like Make's behavior |
| Node execution | Dispatcher interface (not closures) | Decouples scheduling from execution; testable with mock dispatchers |
| Process management | Separate `pkg/process/` | Reusable beyond scheduler; clean subprocess lifecycle |
| Tool specifics | Adapter pattern (Prepare/Finalize) | Structured contract for tool-specific semantics; registry-driven |
| Stream isolation | Extend `pkg/io/` | Reuse existing masking infrastructure; add per-node prefixing |
| Destroy ordering | Reverse topological order | Already implemented in Atmos |
| No new exec files | Adapters in `pkg/scheduler/adapters/` | `internal/exec/` is being phased out — no new investment there |

### How It Spans Component Types (Without Migrating Legacy Built-ins)

The scheduler doesn't care about component types — it dispatches to the orchestrator, which resolves the appropriate adapter via the registry. **Legacy built-ins (Terraform, Packer) do NOT need to be migrated to the `ComponentProvider` interface** — they get their own adapters that call into existing execution functions:

```go
// TerraformAdapter.Prepare() calls into existing execution setup
// PackerAdapter.Prepare() calls into existing packer setup
// AnsibleAdapter wraps ComponentProvider.Execute() via ComponentProviderAdapter
// Custom types also go through ComponentProviderAdapter
```

**Why this works without migration:**
- Each adapter encapsulates the tool-specific preparation and finalization logic
- Adapters call into existing execution functions — no duplication, no deep coupling
- Legacy built-ins can be migrated to `ComponentProvider` later independently
- The adapter registry follows the same pattern as `pkg/component/` and `pkg/store/`

### Integration Path

1. **`pkg/process/`** — New package: `Runner`, `TaskSpec`, `Streams`, `Result`
2. **`pkg/scheduler/`** — New package: `Scheduler`, `Node`, `Dispatcher`, `Orchestrator`, `Result`
3. **`pkg/scheduler/adapters/`** — Tool-specific adapters: `TerraformAdapter`, `PackerAdapter`, etc.
4. **`pkg/io/`** — Extend with `prefixedWriter` and `NewNodeStreams()` factory
5. **`internal/exec/terraform_all.go`** — Wire orchestrator into `executeInDependencyOrder()` (modify existing file, no new files in exec)
6. **Unify routing** — `--all`, `--components`, `--query` all flow through the scheduler

---

## Go Libraries

**`golang.org/x/sync` is already in `go.mod` (v0.19.0).** No new dependencies needed.

| Library | Status | Usage |
|---------|--------|-------|
| `golang.org/x/sync/errgroup` | **Already available** | Bounded goroutine groups with error propagation — the scheduler foundation |
| `sync.WaitGroup` | **Already used** | Used in `describe_affected_utils_parallel.go`, `stack_processor_utils.go` |
| `golang.org/x/sync/semaphore` | **Already available** | Optional: for per-type concurrency limits (Phase 4) |

No third-party DAG libraries needed. Atmos already has the graph primitives in `pkg/dependency/`. The scheduler is ~100 lines on top of `errgroup`.

---

## Output Under Concurrency

### The Problem

Atmos currently runs Terraform subprocesses with hardcoded OS streams (`internal/exec/shell_utils.go:70-82`):

```go
cmd.Stdin = os.Stdin
cmd.Stdout = ioLayer.MaskWriter(os.Stdout)
cmd.Stderr = ioLayer.MaskWriter(os.Stderr)
```

Under concurrency, multiple subprocesses writing to the same `os.Stdout` produces interleaved, unreadable output.

Worse, there's a pattern in `terraform_plan_diff.go:265-308` that **swaps the global `os.Stdout`** to capture output:

```go
origStdout := os.Stdout
os.Stdout = w  // Replace global stdout with pipe!
defer func() { os.Stdout = origStdout }()
execErr := ExecuteTerraform(showInfo)
```

This is a race condition under concurrency — two goroutines would fight over the global `os.Stdout`.

### What "Stream Injectable" Means

PR #2159 calls this "stream-injectable subprocess execution." It means refactoring `ExecuteShellCommand()` to accept `io.Writer`/`io.Reader` parameters instead of using `os.Stdout`/`os.Stderr` directly. This allows each concurrent worker to have its own isolated output streams.

### Two Complementary Requirements

Concurrent execution requires **both** of these — they are not alternatives to each other:

1. **Live output streaming** — Users need to see what's happening in real time. Each node's output must be streamable to the terminal (prefixed/labeled per node) without interleaving. This requires stream injection so that each subprocess writes to its own `io.Writer`, and a multiplexer renders them to the terminal in a readable way.

2. **Per-node log files** — Each node's full stdout/stderr is captured to a deterministic file in the component workdir. This is essential for post-mortem debugging, CI artifact collection, and reviewing output from nodes that completed while the user was watching a different node's output.

### Existing Infrastructure We Can Leverage

Atmos already has the building blocks for labeled, per-node output:

**Logger prefixes** (`pkg/logger/log.go:176`, `pkg/logger/atmos_logger.go:120`):
```go
// Already supported — create a prefixed logger per node
componentLogger := logger.WithPrefix("[vpc/tenant1-ue2-dev]")
componentLogger.Info("Starting execution")
```

**Writer wrapping pattern** (`pkg/io/streams.go`): The `maskedWriter` already wraps `io.Writer` to transform content on each `Write()` call. A `prefixedWriter` follows the same pattern — prepend a label to each line of output.

**`io.Context`** (`pkg/io/context.go`): The `Write()` method is the choke point where all Atmos output flows. It already applies masking. Adding prefix support here labels all Atmos-generated output for a given node.

### Two Layers of Output to Label

Under concurrency, there are two distinct output streams per node that both need labeling:

1. **Atmos log messages** (status, progress, errors) — Use `logger.WithPrefix("[component/stack]")`. Already supported, no new code needed. Each scheduler worker gets a prefixed logger instance.

2. **Subprocess output** (Terraform's stdout/stderr) — Needs a `prefixedWriter` wrapping the `io.Writer` passed to `cmd.Stdout`/`cmd.Stderr`. This is new — currently subprocesses bypass the `io.Context` layer and write directly to `os.Stdout`.

### Implementation Approach: Extending `pkg/io/`

Stream injection is a prerequisite for the scheduler, not a future optimization. The approach extends `pkg/io/` — the package that already owns stream abstraction and secret masking:

**New types in `pkg/io/`:**

1. **`prefixedWriter`** — follows the exact same `maskedWriter` pattern in `pkg/io/streams.go` (lines 97-124). Wraps an `io.Writer` and prepends a configurable prefix (e.g., `[vpc/tenant1-ue2-dev]`) to each line of output. Line-prefixed output is a general I/O concern reusable beyond scheduling.

2. **`NewNodeStreams()`** — factory that composes the per-node output pipeline:
   ```go
   func NewNodeStreams(prefix string, masker Masker, terminal io.Writer, logFile io.Writer) *Streams
   ```
   Each node's stream pipeline:
   - `maskedWriter` → applies secret masking (shared global `Masker` — thread-safe, secrets are process-wide)
   - `prefixedWriter` → labels each line with the node ID
   - `io.MultiWriter` → tees to both the terminal (labeled) and the log file (raw)

**Note on `maskedWriter` vs `dynamicMaskedWriter`:** The `dynamicMaskedWriter` pattern (line 129 of `streams.go`) resolves writers at write time via `getWriter func() io.Writer`. This is needed for the global singletons but NOT for per-node streams — each node has fixed writers. Use the simpler `maskedWriter` directly.

**Refactoring existing code:**

3. Refactor `ExecuteShellCommand()` to accept optional `process.Streams` for stdout/stderr (backward-compatible: `nil` means use `os.Stdout`/`os.Stderr` as today)
4. The `os.Stdout` swap pattern in `terraform_plan_diff.go` is eliminated — replaced by passing a buffer writer as the stdout stream to the process runner, then reading the buffer after execution
5. Each worker gets a `logger.WithPrefix("[component/stack]")` for Atmos-level log messages
6. When `--max-concurrency 1`, output goes directly to stdout/stderr as today — no behavior change, no prefixes

---

## Default Concurrency: `--max-concurrency` and `atmos.yaml` Configuration

The default is `1` (sequential, backward-compatible). Users can override via:

1. **CLI flag**: `--max-concurrency N` (highest precedence)
2. **Environment variable**: `ATMOS_MAX_CONCURRENCY=N`
3. **`atmos.yaml` configuration**: (lowest precedence)

```yaml
# atmos.yaml
settings:
  scheduler:
    max_concurrency: 4
    fail_fast: true
```

This follows Atmos's existing precedence model: CLI flags > ENV vars > config files > defaults (Viper).

---

## What the Scheduler Replaces

The scheduler replaces the serial loop in `executeInDependencyOrder()` (`internal/exec/terraform_all.go:75-104`):

```go
// CURRENT (sequential — what gets replaced)
executionOrder, err := graph.TopologicalSort()
for i := range executionOrder {
    node := &executionOrder[i]
    if err := executeTerraformForNode(node, info); err != nil {
        return err  // stops on first error
    }
}

// NEW (three-layer — orchestrator drives scheduler + process runner + adapters)
orch := scheduler.NewOrchestrator(
    process.NewExecRunner(),
    adapters.NewRegistry(
        adapters.WithTerraform(&adapters.TerraformAdapter{}),
        adapters.WithPacker(&adapters.PackerAdapter{}),
    ),
)
sched := scheduler.New(graph, orch,
    scheduler.WithMaxConcurrency(maxConcurrency),
    scheduler.WithFailFast(true),
)
result := sched.Run(ctx)
```

The graph construction (`buildTerraformDependencyGraph()`, `DependencyParser`) is **unchanged**. The scheduler consumes the same `*dependency.Graph` that exists today.

---

## Package Organization

### Keep `pkg/dependency/` as-is (graph structure + algorithms)

The existing `pkg/dependency/` package is well-designed:
- `Graph`, `Node`, `GraphBuilder` — graph structure
- `TopologicalSort()`, `GetExecutionLevels()`, `HasCycles()` — algorithms
- `Filter()`, `FilterByType()`, `FilterByStack()` — queries
- `Clone()`, `FindPath()`, `IsReachable()` — utilities

This stays exactly as-is. It's a data structure package — no execution logic.

### New `pkg/scheduler/` (scheduling + orchestration)

The scheduler is a separate concern from the graph:
- `pkg/dependency/` = "what's the order?" (data)
- `pkg/scheduler/` = "execute in that order, concurrently" (execution)

```
pkg/scheduler/
├── scheduler.go       # Scheduler struct, Run(), Dispatcher interface
├── orchestrator.go    # Orchestrator: combines scheduler + process runner + adapter registry
├── node.go            # Node (pure scheduling data: ID, Type, Ref)
├── result.go          # AggregateResult, NodeResult, Status enum
├── options.go         # Functional options (WithMaxConcurrency, WithFailFast, etc.)
├── scheduler_test.go
├── orchestrator_test.go
└── adapters/
    ├── adapter.go     # Adapter interface, Registry interface
    ├── registry.go    # Default registry implementation
    ├── terraform.go   # TerraformAdapter (Prepare/Finalize)
    ├── packer.go      # PackerAdapter
    ├── ansible.go     # AnsibleAdapter (wraps ComponentProvider)
    └── adapter_test.go
```

### New `pkg/process/` (subprocess lifecycle)

Generic subprocess management — reusable beyond the scheduler:

```
pkg/process/
├── runner.go          # Runner interface, TaskSpec, Streams, Result
├── exec_runner.go     # Default implementation using os/exec
├── options.go         # Functional options (timeouts, signal grace period)
└── runner_test.go
```

### Extended `pkg/io/` (stream isolation)

New additions to the existing I/O package:

```
pkg/io/
├── ...                # existing files unchanged
├── prefixed_writer.go # prefixedWriter (follows maskedWriter pattern)
└── node_streams.go    # NewNodeStreams() factory for per-node output pipelines
```

### No new files in `internal/exec/` (MANDATORY)

The long-term goal is to eliminate `internal/exec/`. All new code goes into `pkg/` packages. Existing exec files are modified to wire in the orchestrator, but no new files are created there.

### Package Responsibility Summary

| Concern | Package | Responsibility |
|---------|---------|----------------|
| Graph structure | `pkg/dependency/` | Build, sort, filter, query (unchanged) |
| Scheduling | `pkg/scheduler/` | Ready-queue, worker pool, dispatch, results |
| Orchestration | `pkg/scheduler/` | Combines scheduler + process runner + adapters |
| Tool adapters | `pkg/scheduler/adapters/` | Prepare (→ TaskSpec) and Finalize per tool type |
| Subprocess lifecycle | `pkg/process/` | Spawn/wait/stop/kill, stream injection, signal handling |
| Stream isolation | `pkg/io/` | Per-node streams, prefix writers, masking (extended) |
| Component abstraction | `pkg/component/` | `ComponentProvider` interface + registry (unchanged) |

This follows Atmos's existing pattern of purpose-built packages (`pkg/store/`, `pkg/git/`, `pkg/component/`).

---

## DAG Visualization

Atmos already has Mermaid and Graphviz renderers in `pkg/auth/list/graph.go` (lines 29-257) for the auth identity graph. The same patterns can be reused for dependency graph visualization.

### Proposed: `atmos describe graph`

```shell
# Mermaid diagram to stdout
atmos describe graph --stack prod --format mermaid

# Graphviz DOT to file
atmos describe graph --stack prod --format dot --file deps.dot

# JSON adjacency list (machine-readable)
atmos describe graph --stack prod --format json

# Filter to specific component and its dependencies
atmos describe graph --stack prod --component eks
```

### Implementation

Reuse the existing renderer patterns from `pkg/auth/list/graph.go`:
- `RenderGraphviz()` → adapt for dependency nodes (component@stack labels)
- `RenderMermaid()` → adapt with direction arrows showing dependency flow
- `RenderMarkdown()` → Mermaid embedded in markdown (useful for PRs/docs)
- `sanitizeMermaidID()` → already handles special characters

The graph is already computed by `buildTerraformDependencyGraph()`. Visualization is just a different rendering of the same `*dependency.Graph`.

### Why this matters for the scheduler

When users enable `--max-concurrency > 1`, understanding the DAG is critical for debugging:
- "Why is component X waiting?" → visualize its dependencies
- "What can run in parallel?" → see the graph width at each level
- "Is my `depends_on` correct?" → spot missing or wrong edges

---

## Phased Rollout

### Phase 1: Foundation Packages
1. Create `pkg/process/` with `Runner` interface, `TaskSpec`, `Streams`, `Result`, default exec-based implementation
2. Extend `pkg/io/` with `prefixedWriter` and `NewNodeStreams()` factory (reuse existing `maskedWriter` pattern)
3. Refactor `ExecuteShellCommand()` to accept optional `process.Streams` parameter (backward-compatible: `nil` means current behavior)
4. Eliminate the `os.Stdout` swap pattern in `terraform_plan_diff.go` — replaced by stream injection

### Phase 2: Scheduler + Terraform Adapter
1. Create `pkg/scheduler/` with pure scheduling logic (`Scheduler`, `Node`, `Dispatcher` interface, `AggregateResult`)
2. Create `pkg/scheduler/orchestrator.go` combining scheduler + process runner + adapter registry
3. Create `pkg/scheduler/adapters/` with `TerraformAdapter` (Prepare/Finalize calling existing exec functions)
4. Wire orchestrator into `executeInDependencyOrder()` in `internal/exec/terraform_all.go` (modify existing file, no new files in exec)
5. Add `--max-concurrency N` flag (default: 1 = sequential, backward-compatible)
6. Per-node output: live labeled streaming to terminal + log files in component workdir
7. JSON summary output (`--output json`)
8. Require `-auto-approve` when `--max-concurrency > 1` for `apply`/`destroy`

### Phase 3: Routing Consolidation + Multi-Type DAGs
1. Converge `--components` and `--query` onto the DAG-backed executor (currently `ExecuteTerraformQuery`)
2. Unify `--affected` path to use the scheduler
3. Add `--fail-fast` / `--keep-going` flags
4. Create `PackerAdapter`, `AnsibleAdapter` (wraps `ComponentProvider`), `ComponentProviderAdapter` for registered types
5. Extend graph building to include Packer and Ansible nodes
6. Cross-type `depends_on` syntax (e.g., `component: ami-builder, type: packer`)

### Phase 4: Advanced Scheduling
1. Per-type concurrency limits (resource pools, like Ninja)
2. Critical-path scheduling (prioritize longest chain, like Bazel)
3. TUI progress display (live node status table)
4. Resumability (skip already-completed nodes on re-run)

---

## PR #2159 Proposal — Key Details for Reference

PR #2159's rollout plan (for comparison):
- **Foundation PR 1**: Stream-injectable subprocess execution, remove `runTerraformShow` stdout swap
- **Foundation PR 2**: Converge bulk Terraform CLI routing onto one graph-backed executor
- **Phase 1**: Level-based concurrent scheduling for `plan` with `--max-concurrency`
- **Phase 2**: Extend to `apply`/`destroy` with `-auto-approve`, signal handling, hook/store-aware completion
- **Phase 3**: Move `--affected` onto shared scheduler, add resumability, ready-queue scheduling

Notable details from PR #2159 that should be preserved:
- **Prerequisite closure**: Auto-include `depends_on` prerequisites; `--require-closure` to fail instead
- **Node completion semantics**: Node isn't "done" until hooks + store updates complete, not just Terraform exit 0
- **Exit code contract**: `plan` returns 1 (failure), 2 (changes detected), 0 (clean) — matches Terraform's own exit codes
- **Auth pre-bootstrap**: Authenticate during planning phase, pass authenticated context to workers
- **Signal handling**: SIGINT/SIGTERM stops scheduling new nodes, grace period for running nodes

---

## Open Questions

1. **Per-type concurrency limits?** — Like Ninja's resource pools: "max 3 Terraform applies, max 1 Packer build". Useful for API rate limits. Phase 4 concern.
2. **Progress UX** — TUI progress display for concurrent execution (which nodes running, which waiting, which done). Phase 4 concern.

### Resolved Questions
- **Package location**: `pkg/scheduler/` (separate from `pkg/dependency/`)
- **Scheduler vs process separation**: The scheduler is pure scheduling data + ready queue. The process runner (`pkg/process/`) owns subprocess lifecycle. They are connected via the `Dispatcher` interface and `Adapter` pattern, with the orchestrator as the integration point.
- **Legacy built-ins**: NOT migrated to `ComponentProvider` — they get dedicated adapters (`TerraformAdapter`, `PackerAdapter`) that call into existing execution functions
- **Stream injection**: Required as Phase 1 foundation. Extends `pkg/io/` with `prefixedWriter` and `NewNodeStreams()`. Process runner accepts injected `Streams` instead of hardcoding `os.Stdout`/`os.Stderr`. Both live streaming and per-node log files are needed — they are complementary, not alternatives.
- **`pkg/io/` reuse**: Per-node stream isolation extends the existing `pkg/io/` package. The shared global `Masker` instance handles secret masking (thread-safe, secrets are process-wide). `prefixedWriter` follows the existing `maskedWriter` pattern.
- **No new exec files**: Adapters and orchestrator live in `pkg/scheduler/` and `pkg/scheduler/adapters/`, NOT in `internal/exec/`. The long-term goal is to eliminate `internal/exec/`.
- **`pkg/process/` vs `pkg/runner/`**: `pkg/process/` is subprocess-level (spawn, streams, signals, exit codes). `pkg/runner/` is task-level (shell tasks, atmos sub-commands). Different abstraction levels, complementary.
- **Default concurrency**: `1` (sequential, backward-compatible), configurable via `atmos.yaml`, ENV, or CLI flag
- **Cross-type dependency syntax**: Solved by PR #2193 — new `dependencies.components` format with `kind` field for cross-type dependencies (terraform/helmfile/packer/plugin). The scheduler consumes this format via the graph builder.

---

## Verification

- Unit tests for scheduler: diamond DAG, linear chain, fan-out, fan-in, single node, error propagation, fail-fast vs keep-going
- Unit tests for `pkg/process/`: stream injection (stdout/stderr go to injected writers, not os.Stdout), signal forwarding (SIGINT/SIGTERM), exit code preservation
- Unit tests for adapters: `Prepare` returns correct `TaskSpec` (command, dir, env), `Finalize` handles hooks/stores
- Unit tests for `prefixedWriter`: correct line-by-line prefixing, partial writes, empty lines, concurrent writes
- Integration test: `atmos terraform plan --all --max-concurrency 4` on test fixtures with `depends_on`
- Integration test: mixed-type DAG (Terraform + Packer nodes) with adapter registry
- Benchmark: 100-node graph scheduling overhead < 100ms
- Backward compatibility: `--max-concurrency 1` produces identical output to current sequential execution
