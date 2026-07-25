# `--labels`/`--tags` and `list dependencies` Evaluate Every Stack Before Filtering

**Date:** 2026-07-25
**Severity:** High — on a monorepo spanning multiple AWS accounts (or any repo with an
inaccessible/not-yet-provisioned stack), `atmos terraform ... --all --labels=...`/`--tags=...` and
`atmos list dependencies --stack <stack>` fail on an unrelated stack's backend/auth before the
requested selector is ever consulted
**Reported:** Customer Slack thread (Cloud Posse support), 2026-07-20 through 2026-07-25 — a
multi-account monorepo where one login cannot authenticate to another account's Terraform state
backend at all
**Reproducer:** `internal/exec/describe_stacks_component_processor_scope_test.go`
(`TestProcessComponentEntry_TagsLabelsOutOfScopeSkipsAuth`); `cmd/list/dependencies_scoped_test.go`
(`TestExecuteListDependenciesCmd_ScopedEvaluationAvoidsUnrelatedStack`)

______________________________________________________________________

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` bug fix generalizing an existing scoping mechanism (see Prior Art below) to two more
callers. There is no new command or flag — `--labels`/`--tags` and `list dependencies` already existed;
this only corrects when they evaluate stacks relative to when they filter. Per the repo's label
decision tree that makes it a `patch`, which does not require a `website/blog/` post or a roadmap
milestone.

## Prior art

`docs/fixes/2026-06-22-describe-stacks-scope-and-cache-per-component-auth.md` fixed the identical shape
of bug for `-s`/`--stack`: `shouldFilterByStack` now runs in `processComponentEntry`
(`internal/exec/describe_stacks_component_processor.go`) before `resolveComponentAuthManager`/
template/YAML-function processing, so an out-of-scope stack never authenticates. That fix did not cover
`--labels`/`--tags` (which have no early filter at all) or `atmos list dependencies` (which
deliberately disables even the `-s` early filter, to preserve cross-stack edge resolution).

## Symptom

1. `atmos terraform apply --all --labels=team=platform` (or `--tags=...`) in a repo where `--all` is
   given without `-s`: every stack in the repo is fully evaluated (templates, YAML functions, auth,
   backend config) before label/tag matching is ever consulted, because
   `filterTerraformGraphByTagsAndLabels` (`pkg/scheduler/adapters/terraform.go`) only runs on the graph
   built from an already-fully-described stack set. Labels/tags gave the customer a way to *express*
   the selection they wanted, but not a way to avoid evaluating everything else first — the exact
   failure mode `-s` alone does not have.
1. `atmos list dependencies --stack <stack>` fails when *any other* stack in the repo has an
   inaccessible backend, unresolved `!terraform.state`, or (in the reported case) a physically
   unreachable AWS account. `cmd/list/dependencies.go`'s `describeStacksForDependencies` hardcoded the
   stack filter to `""` when calling describe-stacks, with a comment explaining this was deliberate —
   cross-stack dependency edges need every stack described to resolve. `opts.Stack` was only applied
   afterward, for display filtering.

## Root Cause

Both callers had a real reason for their scope-widening: labels/tags are a component attribute, and
you can't know a component matches a label without describing it; cross-stack dependency edges need
other stacks' identities to resolve. Neither caller had a *cheap* way to get that information — both
reached for a full evaluation (auth, templates, YAML functions) when only structural/static metadata
(`metadata.tags`, `metadata.labels`, `dependencies.components`/`settings.depends_on`) was actually
needed to decide scope.

## Fix

Two independent extensions of the same idea — decide scope from cheap, already-available data before
paying for auth/template/YAML-function evaluation — landed together since they share the same root
cause:

### 1. `--labels`/`--tags` early-skip (`internal/exec/describe_stacks_component_processor.go`)

Added `scopeDecision`/`inScopeByTagsAndLabels`, checked in `processComponentEntry` immediately after
the existing `shouldFilterByStack` gate (i.e. still before `resolveComponentAuthManager`). It mirrors
`pkg/scheduler/adapters/terraform.go`'s `matchesTerraformTagsAndLabels` post-filter exactly (same
`pkg/tags.MatchesTags`/`MatchesLabels` semantics: tags any-match, labels all-match), so the early gate
and that later, still-authoritative post-filter can never disagree — the post-filter remains the safety
net for anything the early gate cannot decide.

`info.Tags`/`info.Labels` are threaded through a new `ExecuteDescribeStacksWithMocks(...,
tagsFilter, labelsFilter)` pair of trailing parameters, populated only by `ExecuteTerraformAllWithContext`
and `ExecuteTerraformQueryWithContext`. Every other `ExecuteDescribeStacks*` wrapper is unchanged and
implicitly passes empty filters, so single-component and non-selector callers see zero behavior change.

**Templated metadata safety net.** `metadata.tags`/`metadata.labels` could in principle be Go
templates. Grepping this repo's own `examples/`/`tests/` found zero such occurrences, but the code does
not assume that: `isMetadataSelectorTemplated` detects a `{{` marker in the raw value and returns
`decidable=false`, which forces the component through full evaluation instead of being silently
skipped — correctness is preserved (only the perf optimization is forfeited for that one component). A
`settings.describe.settings.eager_evaluation` / `ATMOS_...`-style config escape hatch
(`schema.DescribeSettings.EagerEvaluation`, read via `GetEagerEvaluationSetting`) forces this path for
every component, as an instant rollback.

### 2. `list dependencies` closure-scoped evaluation (`cmd/list/dependencies.go`, `pkg/list/dependencies/closure.go`)

When `--stack`/`--component` bounds the request, evaluation is now split:

- **Phase A (lightweight, whole repo):** describe every stack with templates/YAML functions/auth all
  off — cheap, since dependency edges and tags/labels are structural. Build the graph
  (`dependencies.BuildGraph`).
- **Phase B (closure):** `dependencies.ReachableClosure` (new; a thin wrapper over the dependency
  package's existing `Graph.Filter(IncludeDependencies/IncludeDependents)` traversal — no new graph-walk
  logic was needed) computes the nodes reachable from the `--stack`/`--component` roots in the
  requested direction. `dependencies.StackNames` reduces that to the set of stacks actually touched.
- **Phase C (resolve, closure-scoped only):** re-describes *only* those stacks with the caller's real
  `--process-templates`/`--process-functions` settings, then **recomputes** the closure against the
  resolved graph (`resolveClosure`) rather than reusing Phase B's node set. This matters: this repo's
  own `dependencies-components-inheritance` fixture has a same-stack templated dependency target
  (`stack: "{{ .vars.stage }}"`), which Phase A's unrendered pass cannot resolve into a real edge.
  Recomputing against the resolved graph picks it up correctly (verified — see below); if a resolved
  pass reveals a *new* stack the lightweight pass could not see at all, `resolveClosure` describes that
  stack too and loops until the touched-stack set stabilizes (bounded by the repo's total stack count).

Without a `--stack`/`--component` filter there is no root to bound a closure to, so behavior is
unchanged: a single full-repo describe, exactly as before.

## Verification

- `go test ./internal/exec/... ./cmd/list/... ./pkg/list/... ./pkg/scheduler/... ./pkg/tags/...
  ./pkg/dependency/...` — full pass, including every pre-existing test (no snapshot/behavior
  regressions for callers that don't use `--labels`/`--tags`/bounded `list dependencies`).
- `TestProcessComponentEntry_TagsLabelsOutOfScopeSkipsAuth` — the injectable auth resolver is invoked
  **zero** times for a component excluded by `--tags`/`--labels`, mirroring the `-s` regression test
  from the prior-art fix.
- `TestProcessComponentEntry_TagsLabelsTemplatedMetadataFallsThroughToAuth` /
  `TestProcessComponentEntry_TagsLabelsEagerEvaluationOverride` — the safety net and rollback both
  force full evaluation instead of skipping.
- New fixture `tests/fixtures/scenarios/dependencies-scoped-evaluation` (two unrelated stacks; one
  component unconditionally fails template rendering the moment it's evaluated, standing in for an
  unreachable backend without needing real cloud credentials in CI):
  - `TestExecuteListDependenciesCmd_ScopedEvaluationAvoidsUnrelatedStack` — `list dependencies --stack
    app-a` with full evaluation enabled succeeds even though app-b's component always errors when
    evaluated.
  - `TestExecuteListDependenciesCmd_UnboundedStillEvaluatesEverything` — documents the known boundary:
    with no `--stack`/`--component` filter, app-b's error still surfaces (matches historical behavior,
    not a regression).
- `TestExecuteListDependenciesCmd_CoverageIntegration` (existing, `dependencies-components-inheritance`
  fixture) — after this fix, `vpc --stack dev`'s `required_by` output correctly includes `eks`, whose
  dependency on `vpc` is declared via the same-stack templated `stack: "{{ .vars.stage }}"` — direct
  evidence the Phase C re-closure (rather than reusing Phase A/B's unrendered edges) is necessary and
  correct.

## Recommendations

- **`--all -s <stack>` still does not expand to cross-stack prerequisites.** `IncludeDependencies:
  false` remains hardcoded in `pkg/scheduler/adapters/terraform.go` — selecting one stack orders its
  in-scope cross-stack edges but does not pull in prerequisite components from other stacks. This fix's
  lightweight-graph-plus-closure machinery (`dependencies.ReachableClosure`) is directly reusable for
  that follow-up (compute the closure, feed its node IDs as a `TerraformSelection`, flip
  `IncludeDependencies` for the selected roots) but implementing it was out of scope here. Tracked
  separately.
- **Templated cross-stack dependency targets remain a residual gap.** `resolveClosure`'s convergence
  loop only discovers a new stack once a *previously-included* stack's resolved pass reveals an edge to
  it. A dependency target templated to point at a stack with **no** structural path from the root at
  all (e.g., driven by a remote store lookup) cannot be discovered this way. `settings.describe.settings.eager_evaluation`
  is the escape hatch for a repo that needs it.
