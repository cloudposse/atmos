# Proposal: Concurrent Component Provisioning with Dependency-Aware Orchestration

## Summary

Add a concurrent execution mode on top of Atmos' dependency-ordered Terraform execution so that multiple ready component instances can run in parallel from a single `atmos` invocation.

The intent is to keep Atmos' existing dependency semantics and existing dependency graph work, but replace strictly sequential execution of the graph with bounded concurrent scheduling:

- Independent components run concurrently
- Dependent components wait until prerequisites complete
- Cross-stack dependencies are respected
- CI pipelines can execute a full dependency tree from one job instead of coordinating many matrix jobs

This proposal is intentionally design-only. It does not include implementation details beyond the level needed for maintainers to evaluate fit, scope, and risks.

## Problem

Atmos already supports bulk Terraform execution patterns such as:

- `--affected`
- `--all`
- `--components`
- `--query`

It also already models component dependencies through `settings.depends_on`.

PR #1516 answers a large part of the ordering problem by introducing reusable dependency graph logic, topological sorting, cross-stack dependency support, filtering, and dependency-ordered execution for bulk Terraform paths. However, PR #1516 is still open and unmerged as of this proposal revision, so it should be treated as a prerequisite rather than as behavior already present on `main`.

The remaining gap is that execution of the resolved graph is still sequential. That leaves two concrete problems:

1. Large deployments take longer than necessary because independent components are serialized
2. CI users who want to deploy a dependency tree from one command still need external orchestration, often via a matrix

This is most visible in scenarios such as:

- A platform stack with `vpc`, `dns`, and `iam` as independent roots
- Application stacks that depend on shared network or cluster components
- `--affected --include-dependents` runs where the dependency set is already known, but execution still proceeds one component at a time

## Motivation

The feature matters for both local and CI workflows.

### Local Use

- Faster feedback for large changes
- Simpler "apply the whole slice" workflows
- Better fit for repositories with many component instances

### CI Use

- One `atmos` process can own dependency discovery and execution
- No need to distribute stacks across matrix jobs just to express ordering
- Failures can be reported in graph terms: failed, blocked, skipped, completed

## Current State in the Repository

The repo already contains some of the building blocks needed for this feature, and PR #1516 introduces the rest:

- PR #1516 introduces a reusable dependency graph package, Kahn-based topological sorting, deterministic ordering, cycle detection, cross-stack support, filtering, and shared execution paths for `--all` and `--affected`
- PR #1876 introduced isolated Terraform component workdirs and positioned them as an enabler for concurrent component execution
- `settings.depends_on` is part of the stack schema
- `describe dependents` can already discover reverse edges
- current shell execution still binds subprocesses to shared `os.Stdin`, `os.Stdout`, and `os.Stderr`
- Terraform workdirs are explicitly positioned as enabling isolated concurrent execution

At the same time, there are important constraints:

- Config loading is not concurrency-safe today
- Current workflow execution is documented and implemented as sequential
- Some execution structs are mutated in-place and therefore are not safe to share between goroutines

That suggests the right boundary is:

- Build the execution plan sequentially
- Execute the plan concurrently using immutable per-node inputs

## Prerequisites

This proposal assumes the following prerequisite work:

- PR #1516 merges first, so Atmos has one graph-backed dependency-ordering foundation for Terraform bulk execution
- PR #1876 remains the execution-isolation foundation via workdirs

Without PR #1516, this proposal would have to solve both ordered graph execution and concurrent scheduling at the same time, which is not the intended scope.

## What PR #1516 Already Settles

This proposal should not reopen design questions that PR #1516 already addresses.

Those decisions include:

- using a reusable dependency graph package instead of keeping graph logic inside one command path
- using topological sorting to derive execution order
- detecting circular dependencies up front with actionable errors
- supporting cross-stack dependencies
- supporting existing bulk selectors and filters
- preserving deterministic ordering for reproducible runs
- handling destroy in reverse dependency order

The proposal in this document is therefore intentionally narrower:

- do not redesign graph construction
- do not redesign filter semantics
- do not redesign ordered `--all` or `--affected`
- add concurrent scheduling on top of the ordered graph that PR #1516 defines

## Goals

- Provision multiple component instances concurrently when they have no unresolved dependencies
- Reuse Atmos' existing dependency model instead of introducing a second ordering system
- Support cross-stack orchestration from one invocation
- Fit naturally into existing multi-component Terraform entry points
- Provide deterministic behavior and summary reporting suitable for CI

## Non-Goals

- Replacing Terraform's internal resource-level parallelism
- Replacing workflows as a general task runner
- Changing the meaning of `settings.depends_on`
- Solving every possible multi-tool orchestration case in the first version
- Implementing distributed execution across multiple machines

## Proposed Direction

Extend the graph-based Terraform executor from PR #1516 with concurrent scheduling of ready nodes.

Conceptually, the flow is:

1. Select target component instances from one command
2. Build and filter the dependency graph using the existing graph package
3. Derive execution batches or a ready queue from that graph
4. Schedule ready nodes with bounded concurrency
5. Aggregate results and stop or continue based on failure policy

This is best introduced as an extension of the existing multi-component Terraform execution path, not as an extension of current workflows.

### Why Terraform Bulk Execution Is the Best Landing Point

The most natural place for the first version is the existing Terraform bulk executor because it already supports:

- Target selection
- Affected detection
- Dependency-aware ordering
- Graph construction and filtering
- Per-component execution

By contrast, workflows are list-based and sequential by design. They are a poor fit for dynamic graph scheduling unless their model is substantially expanded.

### Architecture Sketch

```mermaid
flowchart TD
    subgraph planPhase ["Sequential Planning Phase"]
        A["CLI Input"] --> B["Target Selection"]
        B --> C["Config Resolution"]
        C --> D["Graph Construction via pkg/dependency"]
        D --> E["Cycle Detection"]
        E --> F["GetExecutionLevels()"]
    end

    subgraph execPhase ["Concurrent Execution Phase"]
        F --> G["Level 0"]
        G --> H{"Level Complete?"}
        H -->|Yes| I["Level 1"]
        I --> J{"Level Complete?"}
        J -->|Yes| K["Level N"]
        H -->|Failure| L["Mark downstream nodes blocked"]
    end

    subgraph reportPhase ["Reporting Phase"]
        K --> M["Aggregate Results"]
        L --> M
        M --> N["Summary Output"]
    end
```

## Execution Model

### 1. Build the Target Set

The orchestrator would accept the same high-level selectors users already understand:

- `--affected`
- `--all`
- `--components`
- `--query`

The result should be normalized into unique executable nodes keyed by:

- component type
- stack
- component instance name

Using the stack plus component instance is important because the same base component can appear many times across stacks.

This proposal should reuse the existing graph-building and filtering logic from PR #1516 rather than replace it.

### 2. Build and Filter the Dependency Graph

Each selected node already becomes a vertex in a graph under the PR #1516 design.

Edges come from `settings.depends_on`, and the graph layer already handles the major graph concerns:

- dependencies within a stack
- dependencies across stacks
- transitive prerequisite inclusion
- cycle detection
- deterministic ordering

If additional expansion is needed for `--include-dependents`, that should also happen as a graph operation rather than as a separate execution model.

### 3. Derive Execution Levels

PR #1516 already uses topological sorting and introduces `GetExecutionLevels()`. Phase 1 should use level-based scheduling as the default model:

- all nodes in level 0 are eligible to run first
- level 1 only starts after level 0 completes successfully
- level N only starts after level N-1 completes successfully

This is simpler, deterministic, easier to debug, and directly matches the graph API introduced by PR #1516.

A streaming ready-queue remains a valid Phase 2 optimization for unbalanced graphs, but it should not be the initial scheduler model.

### 4. Concurrent Scheduling

Once execution levels or ready nodes are known, the scheduler should:

- dispatch ready nodes concurrently
- cap concurrency with a worker-pool limit
- mark nodes complete, failed, blocked, or skipped
- unlock downstream nodes only when all prerequisites succeed

At a high level, this is a standard DAG scheduler layered on top of the graph that already exists.

The user-facing concurrency control should be opt-in in Phase 1, with default concurrency of 1 for backward-compatible behavior.

Recommended flag:

- `--max-concurrency N`

This avoids confusion with Terraform's own `-parallelism` flag, which controls resource-level parallelism inside a single component.

### 5. Failure Handling

Failure semantics must be explicit.

Recommended default behavior:

- If a node fails, do not run nodes that depend on it
- Continue running already-started or otherwise independent nodes
- Return a non-zero exit code at the end

This gives better CI behavior than either extreme:

- pure fail-fast, which wastes parallel work already available
- pure continue-on-error, which can produce confusing downstream failures

A later extension could add a stricter mode that stops scheduling new work after the first failure.

## CI Shape

One explicit motivation for this feature is to let CI run dependent stacks from a single Atmos command instead of a job matrix.

### Example Shape

Conceptually, a pipeline would be able to run something like:

```shell
atmos terraform apply --affected --include-dependents --max-concurrency 8 -- -auto-approve
```

That single invocation would:

- discover changed roots
- expand dependent component instances
- preserve dependency order
- apply independent branches concurrently
- report one aggregated result to CI

### Why This Is Better Than a Matrix for This Use Case

For dependency-aware infrastructure changes, the matrix model pushes graph logic outside Atmos.

That has several drawbacks:

- ordering is harder to express
- failures are fragmented across jobs
- skipped or blocked dependents are harder to reason about
- duplicated setup work increases CI cost

A single orchestrated invocation keeps dependency discovery, ordering, and execution policy in one place.

## Output and Reporting

Parallel execution introduces a usability problem: raw interleaved stdout becomes difficult to read.

The orchestration layer should therefore produce structured output at the node level.

Recommended reporting model:

- live progress line per component instance or a compact event stream
- per-node result states: `pending`, `running`, `completed`, `failed`, `blocked`, `skipped`
- final summary table or JSON output

This is especially important for CI, where users need to understand:

- what ran
- what failed
- what was blocked by upstream failures
- what never became eligible to run

## Output Isolation

Concurrent execution cannot reuse the current shell execution model unchanged because Atmos subprocess execution still binds each command to shared process stdio.

Phase 1 should therefore require per-node output isolation:

- capture stdout and stderr per component instance
- write per-node logs to a deterministic location such as the component workdir
- keep secret masking behavior consistent with the current `MaskWriter` path
- render live progress separately from raw Terraform output

Recommended Phase 1 model:

- live event stream or compact progress lines in the terminal
- per-component detailed logs on disk
- final summary that links each failed node to its captured output

## Interactive Approval

Concurrent `apply` and `destroy` cannot rely on shared `os.Stdin` for interactive approval.

Recommended Phase 1 behavior:

- require non-interactive execution when `--max-concurrency > 1`
- require `-auto-approve` for concurrent `apply` and `destroy`
- document this as an explicit behavioral constraint of concurrent mode

Concurrent `plan` remains safe without approval prompts, but its output still needs per-node capture and aggregation.

## Signal Handling and Cancellation

Signal behavior must be explicit before concurrent execution is introduced.

Recommended default:

- on `SIGINT` or `SIGTERM`, stop scheduling new nodes
- allow running nodes to finish gracefully within a timeout
- if the timeout expires, terminate remaining child processes
- return a non-zero exit code and mark unfinished downstream nodes appropriately

Implementation-wise, this strongly suggests moving the concurrent executor toward context-aware subprocess handling rather than directly calling subprocesses without cancellation semantics.

## Auth and Credential Lifecycle

Authentication should be treated as a shared resource, not as an incidental per-node side effect.

Recommended Phase 1 model:

- perform auth bootstrap during the planning phase
- create or resolve the required `AuthManager` instances before concurrent execution begins
- pass authenticated context into node execution instead of re-prompting per node
- serialize or otherwise guard credential refresh paths that are not safe under concurrent access

Browser- or device-code-based interactive auth flows are especially problematic under concurrency and should be documented as incompatible with high parallelism unless pre-authenticated.

## Execution Context Isolation

Each worker must receive an isolated execution context.

Recommended assumptions:

- deep-copy `ConfigAndStacksInfo` per node before mutation
- avoid shared mutable execution structs across goroutines
- prefer passing pre-resolved configuration and graph results into workers rather than repeating full config resolution in each worker where possible
- treat `AtmosConfiguration` as shared read-only configuration unless a specific subsystem requires cloning

This is necessary both for correctness and for reducing redundant stack/config processing during large runs.

## Node Completion Semantics

For scheduling purposes, a node should not be considered complete merely because the Terraform subprocess exited with code 0.

A node should be considered complete only after:

- the Terraform command finishes successfully
- any required post-command hooks complete successfully
- any required post-apply store updates complete successfully

This matters because downstream components may depend on side effects produced after `apply`, not only on Terraform state changes themselves.

## Rate Limits and Concurrency Defaults

Component-level concurrency multiplies Terraform's own resource-level concurrency and can easily amplify cloud API load.

Recommended Phase 1 defaults:

- keep concurrent mode opt-in
- use conservative examples and defaults
- document the interaction between component-level concurrency and Terraform `-parallelism`

Per-account or per-provider limits are valid future extensions, but a single global concurrency cap is sufficient for Phase 1.

## Remote State and Undeclared Dependencies

Declared dependencies are protected by the graph: prerequisites complete before dependents start, which avoids stale reads for properly-declared remote state relationships.

However, concurrent mode does not protect undeclared dependencies.

The proposal should explicitly document that:

- each component instance is assumed to own separate state
- declared `settings.depends_on` relationships are respected
- components that read remote state or store values without declaring dependencies may still observe unsafe ordering

## Destroy Semantics Under Concurrency

Destroy ordering should remain the reverse of apply ordering.

Under concurrent scheduling, this means:

- nodes with no dependents are eligible to destroy first
- reversed execution levels can run concurrently
- a node can only be destroyed after all of its dependents have completed destruction

This preserves the reverse-topological semantics already defined by PR #1516.

## Plan Output Aggregation

Concurrent `plan` is operationally different from concurrent `apply` because users need to review the results.

Phase 1 should include:

- per-component captured plan output
- a summary of nodes with changes, no changes, and errors
- a deterministic output directory or log location for detailed inspection

If machine-readable output is supported, plan summaries should be available there from the start for CI consumption.

## Resumability

Resumability can remain a later phase, but the proposal should define the shape now.

Recommended direction:

- write execution state atomically after each node completes
- persist node status as `completed`, `failed`, `blocked`, `skipped`, or `not-started`
- allow a future `--resume` mode to skip completed nodes and continue from the last known execution state

This is particularly valuable for large CI or operator-driven runs with many long-lived nodes.

## Concurrency and Safety Considerations

This feature should treat planning and execution differently.

### Plan Sequentially

Config loading and stack resolution should stay sequential in the first version.

The repository already documents that config loading is not concurrency-safe, so the safe model is:

- resolve config once
- build the execution graph once
- freeze the plan

### Execute Concurrently

Once the graph is frozen, each executable node should receive an isolated execution context.

That means:

- no shared mutable `ConfigAndStacksInfo` between goroutines
- no shared mutable per-run argument objects
- per-node environment preparation
- per-node logging context

### Workdir Isolation

Terraform workdirs already point in the right direction for this feature:

- isolated directories per component instance
- separation of generated files
- reduced risk of collisions between concurrent executions

This proposal should rely on workdir isolation rather than attempt to share a single component directory across concurrent runs.

## Scope Recommendation

The lowest-risk path is a staged rollout.

This proposal should be treated as the concurrency layer that builds on top of the dependency graph foundation from PR #1516 and the workdir foundation from PR #1876, not as a replacement for either.

### Phase 1

Add concurrent scheduling for the existing graph-backed Terraform bulk commands.

Suggested initial surface:

- `terraform apply --all`
- `terraform plan --all`
- `terraform destroy --all`
- `terraform --affected` once it is routed through the same scheduler path

Recommended Phase 1 constraints:

- opt-in only, with default concurrency of 1
- level-based scheduling only
- workdirs required when `--max-concurrency > 1`
- non-interactive apply and destroy only
- global concurrency cap only
- machine-readable summary output available from the start

### Phase 2

Add better reporting, resumability, and machine-readable execution summaries for CI.

### Phase 3

Evaluate whether the orchestration layer should be generalized for:

- Helmfile
- Packer
- future multi-tool DAG workflows

## Alternatives Considered

### 1. Keep Using CI Matrices

This works, but it keeps graph orchestration outside Atmos.

Rejected as the primary answer because it does not solve:

- dependency-aware ordering inside one run
- unified reporting
- reuse of existing Atmos dependency metadata

### 2. Extend Workflows Instead

This would require workflows to move from an ordered list model to a graph model.

Possible in the long term, but not a good first step because:

- current workflow behavior is sequential
- the bulk Terraform path already has dependency information, graph semantics, and execution order

### 3. Run Everything in Parallel Without Dependency Awareness

This is simpler, but incorrect for real infrastructure graphs.

Rejected because it would break one of the main existing guarantees users expect from `settings.depends_on`.

## Open Questions

The maintainers should still decide the following before implementation, but Phase 1 recommendations are included here to reduce ambiguity:

1. Opt-in vs default:
   Recommended Phase 1 position: opt-in, with default concurrency of 1.
2. Execution levels vs streaming ready-queue:
   Recommended Phase 1 position: execution levels only.
3. User-facing flag name:
   Recommended Phase 1 position: `--max-concurrency`.
4. Failure policy:
   Recommended Phase 1 position: continue independent branches by default, with a future `--fail-fast` mode.
5. Workdir requirement:
   Recommended Phase 1 position: require workdirs for concurrent mode.
6. Output format:
   Recommended Phase 1 position: support JSON summary output from the start.
7. Concurrency scoping:
   Recommended Phase 1 position: global concurrency cap only; per-stack or per-account limits later.

## Recommendation

Proceed with a phase-next Terraform concurrency enhancement that builds directly on PR #1516 and PR #1876:

- reuses the dependency graph and ordering semantics from PR #1516
- reuses workdir isolation from PR #1876
- keeps graph construction and filtering sequential
- uses level-based scheduling first, with bounded global concurrency
- requires explicit non-interactive execution for concurrent apply and destroy
- isolates per-node output and emits machine-readable summaries
- produces CI-friendly aggregated results

That gives Atmos a practical, high-value improvement without forcing a redesign of workflows, re-litigating graph design that PR #1516 already settled, or introducing a second dependency model.
