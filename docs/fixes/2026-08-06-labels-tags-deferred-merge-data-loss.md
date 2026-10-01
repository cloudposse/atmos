# Fix: `!labels`/`!tags` deferred-merge data loss — regression tests and completion plan (#2888)

**Date:** 2026-08-06

## Summary

- Confirmed [GitHub issue #2888](https://github.com/cloudposse/atmos/issues/2888) is a real bug,
  not working-as-designed: `!labels`/`!tags`/`!labels.keys`/`!labels.values` silently lose data
  when another config layer (e.g. `terraform.overrides.vars`) sets a conflicting value at the same
  `vars` path, instead of deep-merging.
- Confirmed, by patching and rebuilding locally, that the issue's own suggested one-line fix
  (adding those four prefixes to `postMergeFunctions` in `pkg/merge/merge_yaml_functions.go`) does
  **not** actually fix it.
- Discovered a broader, pre-existing gap: every deferred-merge YAML function that's already
  "recognized" (`!template`, `!terraform.output`, `!terraform.state`, `!store`, `!store.get`,
  `!exec`, `!env`) exhibits the identical silent data loss today, because every production call
  site in `internal/exec/stack_processor_merge.go` passes `processor = nil` to
  `merge.ApplyDeferredMerges` — the "resolve the deferred function, then deep-merge the result"
  half of the design was never wired up, only the "defer it so it doesn't crash the merge" half.
- Added regression tests that reproduce both failure modes end-to-end through the real production
  pipeline (`ExecuteDescribeComponent`) — intentionally red at the time they were added, since no
  code fix was included in this change. **Update:** the production fix landed later the same day in
  this PR (commit `4b832423e3`, "fix(merge): resolve deferred YAML functions and deep-merge with
  concrete overrides (#2888)"), and these regression tests now pass — see the
  [Validation](#validation) and [Follow-ups](#follow-ups) sections below.
- Corrected the stale "✅ Implemented and Tested" status in
  `docs/prd/deferred-yaml-functions-evaluation-in-merge.md` and replaced its stub "Next Steps for
  Full Integration" with a concrete, staged completion plan.

## Context

Investigating #2888 before accepting its prescribed fix (`/field-test` pass), then following up
with an explicit request to add regression tests and a fix plan. Two independent code-reading
passes traced the merge pipeline (`pkg/merge/merge_yaml_functions.go`, `pkg/merge/deferred.go`,
`internal/exec/stack_processor_merge.go`) and confirmed the root cause; empirical reproduction
against a locally built `atmos` binary confirmed both the bug itself and that the reporter's
proposed fix is insufficient. Two independent implementation-design passes (a narrow/per-function
workaround vs. a full pipeline-unification fix) were compared; the unification approach was
selected because the narrow approach adds special-case machinery on top of an already two-stage
pipeline instead of completing the mechanism this repo already built and partially shipped (see
the PRD's own "Next Steps for Full Integration" section, unchanged since 2025-11-29 until this
fix).

## Changes

- `tests/yaml_functions_integration_test.go`
  - Strengthened `TestYAMLFunctionsDeferredMerge/deep_merges_with_yaml_functions`: previously only
    asserted `vars.template_config` existed after merge; now asserts the full expected deep-merged
    map (keys unique to the `!template` result must survive alongside the component's own
    override).
  - Added `TestYAMLFunctionsDeferredMerge/deep_merges_with_labels_and_tags_functions_(regression_for_#2888)`,
    exercising `!labels`/`!tags` against a conflicting component-level override.
  - Added `test-labels-override` to the `"loads all test cases without errors"` component sweep.
- `tests/fixtures/scenarios/atmos-yaml-functions-merge/stacks/catalog/base.yaml` — added abstract
  component `base-component-labels` (`metadata.labels`/`metadata.tags` backing `vars.tags: !labels`
  / `vars.tag_list: !tags`).
- `tests/fixtures/scenarios/atmos-yaml-functions-merge/stacks/test-deferred-merge.yaml` — added
  `test-labels-override`, layering a conflicting literal map/list on top of the base component's
  `!labels`/`!tags` values.
- `docs/prd/deferred-yaml-functions-evaluation-in-merge.md` — corrected status claims throughout
  (header, "Implementation Status", "Test Results", "Benefits Delivered"); added a "Known Gap:
  #2888" section documenting the discovery; replaced "Next Steps for Full Integration" with a
  "Completion Plan: Wiring Post-Merge Resolution (Plan B)" section (staged as a plumbing-only PR 1
  with no behavior change, followed by a behavior-changing PR 2), including file:line citations,
  what must not regress, test plan, and rollout risk.

No production code changed in this pass — see Follow-ups for the production fix that landed
afterward in this same PR.

## Validation

- `go test ./tests/... -run TestYAMLFunctionsDeferredMerge -v` — at the time this pass was written,
  the two new/strengthened subtests **failed as expected** (documenting the confirmed, not-yet-fixed
  bug); all other subtests in the suite passed unchanged. **Update:** now that the production fix
  from commit `4b832423e3` has landed, all subtests — including
  `deep_merges_with_labels_and_tags_functions_(regression_for_#2888)` and
  `deep_merges_with_yaml_functions` — pass.
- `gofmt -l` / `gofumpt -l` on the modified Go test file — clean, no output.
- Manual empirical reproduction against a locally built `atmos` binary (`go build -o ./build/atmos
  .`) in a throwaway temp-dir fixture, matching the issue's own repro script: confirmed the base
  bug, confirmed the reporter's proposed `postMergeFunctions` patch (applied, rebuilt, tested, then
  reverted — `git status` clean afterward) does not fix it, and confirmed the same failure mode for
  `!template` on the unpatched binary (ruling out that the patch itself introduced the broader
  symptom).
- No golangci-lint/full test-suite run was performed for this change beyond the targeted test
  command above, since this is a test-and-docs-only change with no production code touched.

## Follow-ups

- The actual fix (implementing the PRD's "Completion Plan: Wiring Post-Merge Resolution (Plan B)")
  has since shipped on this same PR, commit `4b832423e3`
  ("fix(merge): resolve deferred YAML functions and deep-merge with concrete overrides (#2888)"):
  `resolveDeferredYamlFunctions` (`internal/exec/deferred_contexts.go`) now constructs a real
  `TemplateAwareYAMLProcessor` and passes it to `merge.ApplyDeferredMerges` at Stage 3, so deferred
  YAML functions are resolved and deep-merged against concrete overrides (including the
  mirror-precedence direction) instead of silently losing data. This closes out
  [#2888](https://github.com/cloudposse/atmos/issues/2888).
- This work is on PR [#2892](https://github.com/cloudposse/atmos/pull/2892) (labeled
  `no-release`). The regression tests added in this pass now pass against the shipped fix; the PR
  no longer carries an intentionally-failing check.
