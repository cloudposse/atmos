# Proposal: Concurrent Component Provisioning with Dependency-Aware Orchestration

## Summary

Add a dependency-aware execution mode that can provision multiple Atmos component instances in parallel from a single `atmos` invocation.

The intent is to keep Atmos' existing dependency semantics, but replace strictly sequential bulk execution with graph-based scheduling:

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

However, current bulk execution is effectively sequential. That leaves two gaps:

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

The repo already contains most of the building blocks needed for this feature:

- PR #1876 introduced isolated Terraform component workdirs and positioned them as an enabler for concurrent component execution
- `settings.depends_on` is part of the stack schema
- `describe dependents` can already discover reverse edges
- `terraform --affected` already executes in dependency order, but sequentially
- Terraform workdirs are explicitly positioned as enabling isolated concurrent execution

At the same time, there are important constraints:

- Config loading is not concurrency-safe today
- Current workflow execution is documented and implemented as sequential
- Some execution structs are mutated in-place and therefore are not safe to share between goroutines

That suggests the right boundary is:

- Build the execution plan sequentially
- Execute the plan concurrently using immutable per-node inputs

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

Introduce a graph-based orchestration layer above single-component execution.

Conceptually, the flow is:

1. Select target component instances from one command
2. Expand dependency and dependent relationships as needed
3. Build a directed acyclic graph of executable nodes
4. Topologically schedule ready nodes in bounded parallel batches
5. Aggregate results and stop or continue based on failure policy

This is best introduced as an extension of the existing multi-component Terraform execution path, not as an extension of current workflows.

### Why Terraform Bulk Execution Is the Best Landing Point

The most natural place for the first version is the existing Terraform bulk executor because it already supports:

- Target selection
- Affected detection
- Dependency-aware ordering
- Per-component execution

By contrast, workflows are list-based and sequential by design. They are a poor fit for dynamic graph scheduling unless their model is substantially expanded.

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

### 2. Build the Dependency Graph

Each selected node becomes a vertex in a graph.

Edges come from `settings.depends_on`.

That graph should support:

- dependencies within a stack
- dependencies across stacks
- transitive dependencies
- reverse expansion for `--include-dependents`

The graph build phase should also validate:

- missing referenced components
- ambiguous references
- cycles

If the graph is invalid, Atmos should fail before executing anything.

### 3. Topological Scheduling

Once the graph is built, the scheduler should:

- identify all nodes with no unresolved prerequisites
- dispatch them concurrently
- mark them complete or failed
- unlock downstream nodes whose prerequisites are satisfied

At a high level, this is a standard DAG scheduler with a bounded worker pool.

The user-facing concurrency control could be a flag such as:

- `--parallelism N`
- or `--max-concurrency N`

The exact flag name can be decided later, but the meaning should be "maximum number of component instances Atmos may execute at once."

### 4. Failure Handling

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
atmos terraform apply --affected --include-dependents --parallelism 8
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

This proposal should be treated as the orchestration layer that builds on top of the workdir foundation from PR #1876, not as a replacement for that work.

### Phase 1

Add concurrent orchestration for existing Terraform multi-component commands only.

Suggested initial surface:

- `terraform apply`
- `terraform plan`
- possibly `terraform deploy` if it already follows the same bulk path

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
- the bulk Terraform path already has dependency information and target selectors

### 3. Run Everything in Parallel Without Dependency Awareness

This is simpler, but incorrect for real infrastructure graphs.

Rejected because it would break one of the main existing guarantees users expect from `settings.depends_on`.

## Open Questions

The maintainers should decide the following before implementation:

1. Should the first version be Terraform-only, or should the abstraction be generic from day one?
2. What should the user-facing concurrency flag be called?
3. Should default failure policy be "continue independent branches" or hard fail-fast?
4. Should the feature require workdirs, or simply recommend them?
5. Should the execution summary be human-only at first, or JSON-capable from the start?
6. Do we want a dedicated command name such as `orchestrate`, or should this remain an execution mode on existing commands?

## Recommendation

Proceed with a Terraform-first orchestration layer that:

- reuses `settings.depends_on`
- integrates with existing multi-component selection modes
- plans sequentially
- executes ready nodes concurrently with bounded parallelism
- produces CI-friendly aggregated results

That gives Atmos a practical, high-value improvement without forcing a redesign of workflows or introducing a second dependency model.
