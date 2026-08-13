# Fix: deferred YAML functions executed twice during Stage 3 resolution

**Date:** 2026-08-13

## Summary

- Reviewing PR #2892 (the fix for #2888, deferred YAML functions losing data on merge) surfaced a
  behavior regression not covered by that PR's own tests: with `--process-functions=true`, every
  deferred YAML function was **executed twice** per component resolution.
- Live reproduction: a component with a single, non-colliding `vars.foo: !exec '...'` (a shell
  command that appends to a counter file) ran the shell **2 times** on the PR branch versus **1
  time** on `origin/main` — same command, same fixture. The resolved value was identical (`hello`
  in both), so no output changed; the regression was purely a doubling of function *executions*.
- Impact by function type:
  - `!terraform.output` / `!terraform.state` — unaffected in practice: results are memoized
    (`skipCache`/`tfoutput.GetOutput`), so the second call is cheap and consistent.
  - `!template`, `!labels`, `!tags`, `!env` — harmless: pure and deterministic, just wasted CPU.
  - **`!exec`** — the real problem: `utils.ProcessTagExec` has no cache, so the shell command ran
    twice. That doubles side effects and latency, and for a non-deterministic command the emitted
    value came from the *second* run.
  - **`!store`** — minor: an extra (usually idempotent) backend read per resolution.
- Root cause: `internal/exec.processStacks` runs two passes when `processYamlFunctions` is true —
  first the document-wide `ProcessCustomYamlTags` (Stage 2 in the pipeline vocabulary; this already
  resolves every function string present in the merged component section), then the new
  `resolveDeferredYamlFunctions` (Stage 3, added by PR #2892). Stage 3 re-resolved **every** deferred
  path from its saved `DeferredMergeContext` unconditionally — including no-collision paths whose
  single surviving function `ProcessCustomYamlTags` had already resolved a moment earlier.
- Fix: in `pkg/merge.ApplyDeferredMerges`, when a deferred path has exactly one contributing layer
  (no concrete override and no competing function to deep-merge against) and `result` already holds
  a fully resolved value there, reuse that value instead of re-invoking the processor. Guarded so it
  changes only the Stage 3 pass and never a caller that has not already resolved.

## Context

The double-execution was found while analyzing PR #2892 at the user's request ("does this affect
existing functionality / could it break customers"). Existing output is *not* affected — the
resolved values are byte-identical to `main`. The concern is uncached, side-effecting functions
(`!exec`, `!store`) running twice.

Two passes run in `internal/exec/utils.go`'s `processStacks` under `if processYamlFunctions`:

1. `ProcessCustomYamlTags` — walks the whole (freshly rebuilt) component section and resolves any
  Atmos YAML-function string it finds. Because `mergeComponentConfigurations` writes each deferred
  path's *literal function string* back into the section via a nil-processor `ApplyDeferredMerges`,
  this pass sees and resolves the surviving (highest-precedence) function at every no-collision
  path — exactly as Atmos did before PR #2892.
2. `resolveDeferredYamlFunctions` (Stage 3) — resolves deferred functions from the saved context
  with a template- and auth-aware processor and deep-merges the result against any concrete
  override at the same path. This is what fixes #2888.

For a genuine collision where the concrete override wins the structural merge (the headline #2888
case, e.g. `vars.tags: !labels` overridden by a component's own `tags` map), the function string
never survives into the section, so `ProcessCustomYamlTags` never runs it and Stage 3 is the sole
resolver — no doubling. The doubling only affected paths whose single surviving contribution was a
function that both passes resolved.

## Changes

- `pkg/merge/merge_yaml_functions.go` — `ApplyDeferredMerges` now, for a single-contribution path
  (`len(deferredValues) == 1`) with a non-nil processor, reads the current value at that path via
  `GetValueAtPath` and, if it is already resolved (`isResolvedNonFunctionValue`: not a nil
  placeholder and not a still-unresolved function string), skips the path — leaving the earlier
  pass's value untouched — instead of re-invoking the processor. New helper
  `isResolvedNonFunctionValue`. Collision paths (`len > 1`) are unchanged, so the #2888 deep-merge
  is fully preserved. Skipping performs no `SetValueAtPath`, so it cannot interact with the
  ancestor-before-descendant write ordering added on 2026-08-07.
- `pkg/merge/merge_deferred_apply_test.go` — new `TestApplyDeferredMerges_SingleContributionReuse`
  with a call-counting mock processor: (1) an already-resolved single path resolves 0 additional
  times and preserves the value; (2) a nil placeholder still resolves exactly once; (3) a raw
  function string is not mistaken for a resolved value and still resolves once; (4) a genuine
  collision still resolves the function and deep-merges against the concrete override (guards the
  #2888 behavior).
- `internal/exec/{deferred_contexts,utils,stack_processor_process_stacks,stack_processor_utils}.go`
  — introduced named types `StackComponentDeferredContexts` and `AllStacksDeferredContexts` for the
  previously raw `map[string]map[string]…ComponentDeferredContexts` signatures (readability only; no
  behavior change).

## Validation

- Live before/after against locally rebuilt binaries — the decisive check. Fixture:
  `vars.foo: !exec 'echo run >> <counter>; echo hello'` (a single, non-colliding function), run via
  `atmos describe component mycomp -s test --process-templates=true --process-functions=true`:
  - `origin/main` (pre-#2892): shell executed **1** time.
  - PR #2892 branch, pre-fix: shell executed **2** times (the regression).
  - PR #2892 branch, post-fix: shell executed **1** time, resolved value unchanged (`hello`).
- `go test ./pkg/merge/ -count=1` — pass, includes the new
  `TestApplyDeferredMerges_SingleContributionReuse` (all 4 sub-cases) and the pre-existing
  cache-safety and parent/child ordering regression tests.
- `go test ./tests/ -run 'TestYAMLFunctionsDeferredMerge|TestDeferredMergeTypeConflictResolution'`
  — pass (confirms the #2888 collision deep-merge is intact).
- `go test ./internal/exec/ -run 'Deferred|YamlFunc|YAMLFunc|StackProcessorMerge|ProcessStacks|FindStacksMap|ProcessYAMLConfigFiles' -count=1`
  — pass (exercises the renamed `StackComponentDeferredContexts`/`AllStacksDeferredContexts` types
  and the Stage 3 wiring).
- `go vet ./internal/exec/ ./pkg/merge/` — clean.
- `gofumpt -l` on all changed files — clean.
- `atmos lint --changed` (patch-scoped `custom-gcl`) — **0 issues**.
- Coverage of the touched merge functions (`go tool cover`): `isResolvedNonFunctionValue` 100%,
  `ApplyDeferredMerges` 90.9%, `processDeferredField` 90.9%.

## Follow-ups

- Residual (uncommon): when the *higher*-precedence layer at a colliding path is itself a function
  (a function override sitting above a concrete base value, or two functions competing at the same
  path), the surviving function is still resolved once by `ProcessCustomYamlTags` and again by
  Stage 3's merge. These `len > 1` cases are far rarer than the no-collision case fixed here and
  still produce correct output. Fully deduplicating them would mean having Stage 3 reuse the
  highest-precedence surviving value from the section map rather than re-invoking the processor;
  deferred because it interacts with the ancestor/descendant write ordering and warrants its own
  focused change if it proves to matter in practice.
