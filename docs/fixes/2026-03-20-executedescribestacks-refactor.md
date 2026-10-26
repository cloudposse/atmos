# `ExecuteDescribeStacks` Refactor Audit — Findings and Fixes

**Date:** 2026-03-21

**Related PR:** #2204 — `refactor: reduce ExecuteDescribeStacks cyclomatic complexity 247→10`

**Severity:** High (3 bugs), Medium (5 issues), Low (5 items)

---

## What PR #2204 Did

PR #2204 refactored `ExecuteDescribeStacks` — the highest-cyclomatic-complexity function in
Atmos (cyclomatic 247 → 10, max across all extracted functions: cyclomatic 20, cognitive 22
in `processComponentEntry`) — from a ~1100-line monolith into 19 focused helper functions
with 47+ unit tests. The refactor decomposed the function into:

- **`describeStacksProcessor`** — stateful processor struct holding config, filters, and the
  result map.
- **`processStackFile`** — per-stack-file orchestrator: extracts manifest name, pre-creates
  empty entries when needed, iterates over component types.
- **`processComponentTypeSection`** — per-component-type iterator: filters by component name,
  resolves component sections, delegates to `processComponentEntry`.
- **`processComponentEntry`** — per-component pipeline: extracts sections, builds info,
  resolves stack name, processes templates/YAML functions, applies metadata inheritance,
  writes to destination map.
- **Pure helpers** — `extractDescribeComponentSections`, `buildConfigAndStacksInfo`,
  `resolveStackName`, `shouldFilterByStack`, `ensureComponentEntryInMap`,
  `getComponentDestMap`, `stackHasNonEmptyComponents`, `filterEmptyFinalStacks`, etc.

### Files

| File | Lines | Responsibility |
|---|---|---|
| `describe_stacks.go` | 17 additions | Orchestrator: delegates to `describeStacksProcessor` |
| `describe_stacks_component_processor.go` | ~590 | All helper functions and processor struct |
| `describe_stacks_component_processor_test.go` | ~1500 | Unit tests (table-driven and individual) |
| `describe_stacks_test.go` | ~730 | Integration, edge-case, and ghost-entry tests |

---

## Audit Findings

### Finding 1 (High): Ghost entry for name_template/name_pattern stacks

When `stackManifestName == ""` and `NameTemplate` or `NamePattern` is set, `processStackFile`
pre-created an entry under `stackFileName` (e.g., `"stacks/prod.yaml"`). After
`resolveStackName` evaluated the template/pattern per component, component data was written
under the resolved name (e.g., `"prod-us-east-1"`), leaving a ghost entry under the file
name with empty components.

The ghost entry survived because `filterEmptyFinalStacks` is a no-op when
`includeEmptyStacks=true`.

An additional vector: when `filterByStack` is active and the stack doesn't match, the
pre-created entry was never populated but never cleaned up.

**Fix:** Skip pre-creation when `NameTemplate` or `NamePattern` is set and no manifest name
is defined. Also add a `shouldFilterByStack` early return before pre-creation:

```go
canResolveNameEarly := stackManifestName != "" ||
    (p.atmosConfig.Stacks.NameTemplate == "" && GetStackNamePattern(p.atmosConfig) == "")

if p.includeEmptyStacks && canResolveNameEarly {
    // ... also check shouldFilterByStack before creating entry
}
```

### Finding 2 (High): stackHasNonEmptyComponents section whitelist

The function checked only 5 hardcoded sections: `vars`, `metadata`, `settings`, `env`,
`workspace`. Components with only `backend`, `providers`, `hooks`, `overrides`, or `auth`
sections were incorrectly treated as empty and deleted from output.

This was a new behavioral regression — the original monolith had no such section-name
whitelist; it only checked whether the component map entry itself was non-empty.

**Fix:** Check `len(compContent) > 0` instead of matching specific section names.

### Finding 3 (High): Unguarded type assertions

Two locations had unguarded type assertions that could panic:

**3a:** `processComponentEntry` had a single-line chain of 4 unguarded type assertions to
reach the component destination map.

**Fix:** Extracted `getComponentDestMap` helper with `ok` guards at every level, returning
`(nil, false)` on type mismatch.

**3b:** `ensureComponentEntryInMap` had 3 unguarded type assertions.

**Fix:** Added `ok` guards to all three assertions, returning early on type mismatch.

### Finding 4 (Medium): Shared cache mutation via componentSection

`processComponentTypeSection` received `componentSection` as a direct reference from the
shared `FindStacksMap` cache. Mutations (setting component defaults, metadata inheritance
key deletions) permanently modified the cached data. Filtered-out components could change
what subsequent `ExecuteDescribeStacks` calls see.

**Fix:** Shallow-clone `componentSection` before any mutations so the shared cache is
not modified.

### Finding 5 (Medium): info.ComponentSection stale after template processing

After `processComponentSectionTemplates`, the local `componentSection` was updated but
`info.ComponentSection` still held the pre-template version. YAML functions that read
`info.ComponentSection` (e.g., `!terraform.output`, `!terraform.state`) would see
un-rendered template strings like `"{{ .vars.region }}"` instead of rendered values.

**Fix:** Added `info.ComponentSection = componentSection` after template processing.

### Finding 6 (Medium): delete(stackMap, "imports") mutates live stacksMap

`processStackFile` receives `stackMap` as a `map[string]any` reference from the `stacksMap`
returned by `FindStacksMap`. The `delete(stackMap, "imports")` permanently removes the key
from the underlying data structure.

**Status:** Pre-existing behavior from the original monolith. Not a regression. Reordered
to read manifest name before deleting as a defensive measure.

### Finding 7 (Medium): resolveStackName called O(N×K) per component

`resolveStackName` is called inside `processComponentEntry` — once per component per stack.
For `name_template` with external datasources, this increases external calls.

**Status:** Pre-existing behavior. The original monolith also resolved stack names per
component because the template may reference per-component `vars`.

### Finding 8 (Medium): pkg/describe wrapper diverges from internal/exec

`pkg/describe/describe_stacks.go` is a thin wrapper with hardcoded defaults for
`processTemplates`, `processYamlFunctions`, `skip`, and `authManager`.

**Status:** Intentional design. The public API has fewer parameters with sensible defaults.

### Finding 9 (Low): TestExecuteDescribeStacks_IncludeEmptyStacks was tautological

The test only asserted `NoError` and `NotNil` — it would pass even if `includeEmptyStacks`
had no effect.

**Fix:** Compare results against an `includeEmptyStacks=false` call, assert >= stacks.

### Finding 10 (Low): filterEmptyFinalStacks mutates map before returning error

**Status:** The mutation (deleting empty-key entries) is always correct behavior. Acceptable.

### Finding 11 (Low): Error format change not asserted

**Status:** Substring matching in tests is sufficient for preventing format drift.

### Finding 12 (Low): No golden-file snapshot test

**Status:** Deferred. Requires complete fixture scenarios.

### Finding 13 (Low): debug_stacks_test.go probe file

A debug-only test file with no assertions, ignored errors, and only `fmt.Printf` output.

**Fix:** Deleted `pkg/describe/debug_stacks_test.go`.

### Finding 14 (Low): hasStackExplicitComponents tests not table-driven

8 individual test functions with identical structure.

**Fix:** Consolidated into a single table-driven `TestHasStackExplicitComponents` test.

### Finding 15 (Low): Unguarded type assertions in test assertions

Direct `.(map[string]any)` assertions in tests would panic on type mismatch instead of
producing informative failure messages.

**Fix:** Replaced with `ok`-guarded assertions using `require.True(t, ok)`.

### Finding 16 (Low): Blog post complexity numbers misaligned

Blog said cognitive complexity 12 for `ExecuteDescribeStacks` while roadmap said 22.

**Fix:** Clarified that 22 is from `processComponentEntry` (the most complex extracted
helper), not the orchestrator itself.

---

## Changes Made

### `internal/exec/describe_stacks_component_processor.go`

- Fix ghost entry (#1): skip pre-creation when `NameTemplate` or `NamePattern` is set and
  manifest name is empty. Add `filterByStack` early return before pre-creation.
- Fix `stackHasNonEmptyComponents` (#2): replace 5-section whitelist with
  `len(compContent) > 0`.
- Fix unguarded type assertion chain (#3a): extract `getComponentDestMap` helper with `ok`
  guards at every level.
- Fix `ensureComponentEntryInMap` (#3b): add `ok` guards to all three type assertions.
- Fix shared cache mutation (#4): shallow-clone `componentSection` before any mutations.
- Fix info.ComponentSection staleness (#5): sync `info.ComponentSection = componentSection`
  after template processing.
- Reorder `processStackFile` (#6): read manifest name before `delete(stackMap, "imports")`.
- Add `nolint` directives for pre-existing lint issues:
  - `err113`: dynamic error messages (debugging context needed).
  - `gocognit`/`cyclop`/`funlen`: `processComponentEntry` orchestrator.
  - `gocritic/hugeParam`: `secs` and `info` passed by value (read-only snapshots).
  - `nestif`: inheritance processing block.
  - `revive`: constructor argument limit, function/file length, cyclomatic complexity.

### `internal/exec/describe_stacks_component_processor_test.go`

- Consolidate `TestHasStackExplicitComponents` into table-driven test (#14).
- Replace unguarded type assertions with `require.True(t, ok)` checks (#15).
- Split `TestStackHasNonEmptyComponents_NoRelevantSections` into
  `_NonStandardSections` (true for backend-only) and `_EmptyComponentContent` (false).
- Update `TestFilterEmptyFinalStacks_RemovesEmpty` for new non-empty check.
- Update `TestProcessComponentTypeSection_DefaultsComponentKey` to verify original map
  is NOT mutated (clone protection).
- Fix `gocritic/commentedOutCode` — reformat inline parameter comments.

### `internal/exec/describe_stacks_test.go`

- Add `TestGetComponentDestMap_*` — 5 tests covering all traversal paths (#3a).
- Add `TestEnsureComponentEntryInMap_Invalid*` — 2 tests for ok guard edge cases (#3b).
- Add `TestProcessStackFile_NoGhostEntry_NameTemplate` (#1).
- Add `TestProcessStackFile_NoGhostEntry_NamePattern` (#1).
- Add `TestProcessStackFile_NoGhostEntry_FilterByStack` (#1).
- Strengthen `TestExecuteDescribeStacks_IncludeEmptyStacks` — compare against false call (#9).
- Fix duplicate `config` import alias (#15).

### `pkg/describe/debug_stacks_test.go`

- Deleted (#13).

### `website/blog/2026-03-15-describe-stacks-complexity-reduction.mdx`

- Align complexity numbers with roadmap (#16).

---

## References

- PR #2204: `refactor: reduce ExecuteDescribeStacks cyclomatic complexity 247→10`
- `internal/exec/describe_stacks.go` — refactored orchestrator
- `internal/exec/describe_stacks_component_processor.go` — component processor + helpers
- `internal/exec/describe_stacks_component_processor_test.go` — unit tests
- `internal/exec/describe_stacks_test.go` — integration and edge-case tests
