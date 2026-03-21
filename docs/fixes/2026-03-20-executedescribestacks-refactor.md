# `ExecuteDescribeStacks` Refactor Audit — Findings and Fixes

**Date:** 2026-03-20

**Related PR:** #2204 — `refactor: reduce ExecuteDescribeStacks cyclomatic complexity 247→10`

**Severity:** High (2 bugs), Medium (4 issues), Low (6 items)

---

## What PR #2204 Did

PR #2204 refactored `ExecuteDescribeStacks` — the highest-cyclomatic-complexity function in
Atmos (247 → 10, cognitive 1252 → 22) — from a ~1100-line monolith into 19 focused helper
functions with 47 unit tests. The refactor decomposed the function into:

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
  `stackHasNonEmptyComponents`, `filterEmptyFinalStacks`, etc.

### Files

| File | Lines | Responsibility |
|---|---|---|
| `describe_stacks.go` | 17 additions | Orchestrator: delegates to `describeStacksProcessor` |
| `describe_stacks_component_processor.go` | ~570 | All helper functions and processor struct |
| `describe_stacks_component_processor_test.go` | ~1550 | 47 unit tests |
| `describe_stacks_test.go` | ~600 | Integration and edge-case tests |

---

## Audit Findings

### Finding 1 (High): Ghost entry for name_template stacks with includeEmptyStacks=true

When `stackManifestName == ""` and `NameTemplate != ""`, `processStackFile` pre-created an
entry under `stackFileName` (e.g., `"stacks/prod.yaml"`). After `resolveStackName` evaluated
the template per component, component data was written under the template-resolved name (e.g.,
`"prod-us-east-1"`), leaving a ghost entry under the file name with empty components.

The ghost entry survived because `filterEmptyFinalStacks` is a no-op when
`includeEmptyStacks=true`.

**Fix:** Skip pre-creation when `NameTemplate` is set and no manifest name is defined. The
real stack name won't be known until template evaluation per component:

```go
if p.includeEmptyStacks && (stackManifestName != "" || p.atmosConfig.Stacks.NameTemplate == "") {
```

### Finding 2 (High): stackHasNonEmptyComponents section whitelist

The function checked only 5 hardcoded sections: `vars`, `metadata`, `settings`, `env`,
`workspace`. Components with only `backend`, `providers`, `hooks`, `overrides`, or `auth`
sections were incorrectly treated as empty and deleted from output.

This was a new behavioral regression — the original monolith had no such section-name
whitelist; it only checked whether the component map entry itself was non-empty.

**Fix:** Check `len(compContent) > 0` instead of matching specific section names:

```go
if len(compContent) > 0 {
    return true
}
```

### Finding 3 (High): Unguarded 4-level type assertion chain at line 291

`processComponentEntry` had a single-line chain of 4 unguarded type assertions to reach the
component destination map:

```go
destMap := p.finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[typeName].(map[string]any)[componentName].(map[string]any)
```

While `ensureComponentEntryInMap` pre-creates all levels 35 lines above, any future code path
bypassing that call would get a panic instead of a returned error.

**Fix:** Extracted `getComponentDestMap` helper with `ok` guards at every level, returning
`(nil, false)` on type mismatch. The caller now returns a descriptive error instead of
panicking.

### Finding 3b (Medium): delete(stackMap, "imports") mutates live stacksMap

`processStackFile` receives `stackMap` as a `map[string]any` reference from the `stacksMap`
returned by `FindStacksMap`. The `delete(stackMap, "imports")` permanently removes the key
from the underlying data structure.

**Status:** Pre-existing behavior from the original monolith. Not a regression. If
`FindStacksMap` is cached, subsequent calls see stacks with their `imports` key removed.

### Finding 4 (Medium): resolveStackName called O(N×K) per component

In the new code, `resolveStackName` is called inside `processComponentEntry` — once per
component per stack — making it O(N stacks × K components). For `name_template` with
external datasources, this increases external calls.

**Status:** Pre-existing behavior. The original monolith also resolved stack names per
component because the template may reference per-component `vars` (e.g.,
`{{ .vars.region }}`).

### Finding 5 (Medium): info.ComponentSection stale after template processing

After `processComponentSectionTemplates`, the local `componentSection` is updated but
`info.ComponentSection` still holds the pre-template version. YAML functions that read
`info.ComponentSection` (e.g., `!terraform.output`, `!terraform.state`) would see
un-rendered template strings like `"{{ .vars.region }}"` instead of rendered values.

**Fix:** Added `info.ComponentSection = componentSection` after template processing to sync
the rendered values before YAML function processing.

### Finding 6 (Medium): Unguarded type assertion in ensureComponentEntryInMap

Three unguarded type assertions (`finalStacksMap[stackName].(map[string]any)`) could panic
if the map contained unexpected types.

**Fix:** Added `ok` guards to all three assertions, returning early on type mismatch:

```go
stackEntry, ok := finalStacksMap[stackName].(map[string]any)
if !ok {
    return
}
```

### Finding 7 (Low): delete("imports") ordering creates implicit assumption

`delete(stackMap, "imports")` was called before `getStackManifestName(stackMap)`.

**Status:** Invalid as a bug — `getStackManifestName` reads `"name"`, not `"imports"`.
However, reordered as a defensive measure to keep reads before mutations.

### Finding 8 (Medium): pkg/describe wrapper diverges from internal/exec

`pkg/describe/describe_stacks.go` is a thin wrapper that calls `ExecuteDescribeStacks` with
hardcoded defaults (`processTemplates=true`, `processYamlFunctions=true`, `skip=nil`,
`authManager=nil`).

**Status:** Intentional design. The public API has fewer parameters with sensible defaults.
Not a bug.

### Finding 9 (Medium): No test for name_template + includeEmptyStacks=true

No test existed that combined `NameTemplate` with `includeEmptyStacks=true` to verify ghost
entries under `stackFileName` were absent.

**Fix:** Added `TestProcessStackFile_NameTemplate_NoGhostEntry` that verifies no entry
exists under `stackFileName` when `NameTemplate` is set with `includeEmptyStacks=true`.

### Finding 10 (Low): filterEmptyFinalStacks mutates map before returning error

When an invalid entry is found, the function has already deleted `""` entries and potentially
modified other stack entries in earlier loop iterations.

**Status:** The mutation (deleting empty-key entries) is always correct behavior. The error
case indicates corrupted data, and the caller would discard the result anyway.

### Finding 11 (Low): Error format change not asserted

`filterEmptyFinalStacks` returns a new error format with dynamic stack names. Existing tests
check `assert.Contains(err.Error(), "invalid stack entry type")`.

**Status:** Substring matching is sufficient for preventing format drift.

### Finding 12 (Low): No golden-file snapshot test

No test captures the full `ExecuteDescribeStacks` output and compares it against a committed
golden file.

**Status:** Documented. Golden-file tests require complete fixture scenarios with all
component types, inheritance chains, and template rendering. Deferred to a follow-up.

---

## Changes Made

### `internal/exec/describe_stacks_component_processor.go`

- Fix unguarded type assertion chain (#3): extract `getComponentDestMap` helper with `ok`
  guards at every level — returns error instead of panicking.
- Fix info.ComponentSection staleness (#5): sync `info.ComponentSection = componentSection`
  after template processing so YAML functions see rendered values.
- Fix ghost entry (#1): skip pre-creation when `NameTemplate` is set and manifest name is
  empty — prevents orphaned entries under `stackFileName`.
- Fix `stackHasNonEmptyComponents` (#2): replace 5-section whitelist with
  `len(compContent) > 0` — components with `backend`, `providers`, `hooks`, etc. are no
  longer silently dropped.
- Fix `ensureComponentEntryInMap` (#6): add `ok` guards to all three type assertions —
  prevents panics on unexpected map types.
- Reorder `processStackFile` (#7): read manifest name before `delete(stackMap, "imports")`.
- Add `nolint` directives for pre-existing lint issues from the Copilot-authored PR:
  - `err113`: 3 dynamic error messages (need context for debugging).
  - `gocognit`/`cyclop`/`funlen`: `processComponentEntry` orchestrator (51 stmts, unavoidable).
  - `gocritic/hugeParam`: `secs` and `info` passed by value (read-only snapshots).
  - `nestif`: inheritance processing block.
  - `revive`: constructor argument limit, function length, file length, cyclomatic complexity.

### `internal/exec/describe_stacks_component_processor_test.go`

- Update `TestStackHasNonEmptyComponents_NoRelevantSections` → split into two tests:
  - `TestStackHasNonEmptyComponents_NonStandardSections` — verifies `backend`-only component
    returns `true` (previously returned `false` due to whitelist).
  - `TestStackHasNonEmptyComponents_EmptyComponentContent` — verifies empty content map
    returns `false`.
- Update `TestFilterEmptyFinalStacks_RemovesEmpty` — use empty component content (not
  non-whitelisted sections) to exercise the removal path.
- Fix `gocritic/commentedOutCode` — reformat inline parameter comments.

### `internal/exec/describe_stacks_test.go`

- Add `TestEnsureComponentEntryInMap_InvalidStackEntryType` — verifies no panic when
  `finalStacksMap[stackName]` is not a `map[string]any`.
- Add `TestEnsureComponentEntryInMap_InvalidComponentsType` — verifies no panic when
  `components` section is not a `map[string]any`.
- Add `TestGetComponentDestMap_ValidPath` — happy path traversal.
- Add `TestGetComponentDestMap_MissingStack` — returns false when stack absent.
- Add `TestGetComponentDestMap_InvalidStackType` — returns false for non-map stack.
- Add `TestGetComponentDestMap_MissingComponentsSection` — returns false for missing section.
- Add `TestGetComponentDestMap_MissingComponentName` — returns false for missing component.
- Add `TestProcessStackFile_NameTemplate_NoGhostEntry` — verifies no ghost entry under
  `stackFileName` when `NameTemplate` is set with `includeEmptyStacks=true` (#10).
- Fix `staticcheck/ST1019` — remove duplicate `config` import, unify to `cfg` alias.

---

## References

- PR #2204: `refactor: reduce ExecuteDescribeStacks cyclomatic complexity 247→10`
- `internal/exec/describe_stacks.go` — refactored orchestrator
- `internal/exec/describe_stacks_component_processor.go` — component processor + helpers
- `internal/exec/describe_stacks_component_processor_test.go` — 47 unit tests
- `internal/exec/describe_stacks_test.go` — integration and edge-case tests
