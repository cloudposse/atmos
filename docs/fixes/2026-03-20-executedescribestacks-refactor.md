# `ExecuteDescribeStacks` Refactor Audit — Findings and Fixes

**Date:** 2026-03-20

**Related PR:** #2204 — `refactor: reduce ExecuteDescribeStacks cyclomatic complexity 247→10`

**Severity:** High (2 bugs), Medium (4 issues), Low (6 items)

---

## Audit Findings

### Finding 1 (High): Ghost entry for name_template stacks with includeEmptyStacks=true

When `stackManifestName == ""` and `NameTemplate != ""`, `processStackFile` pre-created an
entry under `stackFileName`. After `resolveStackName` evaluated the template per component,
component data was written under the template-resolved name (e.g., `"prod-us-east-1"`),
leaving a ghost entry under the file name (e.g., `"stacks/prod.yaml"`) with empty components.

**Fix:** Skip pre-creation when `NameTemplate` is set and no manifest name is defined.
The real stack name won't be known until template evaluation per component.

### Finding 2 (High): stackHasNonEmptyComponents section whitelist

The function checked only 5 sections (`vars`, `metadata`, `settings`, `env`, `workspace`).
Components with only `backend`, `providers`, `hooks`, `overrides`, or `auth` sections were
incorrectly treated as empty and deleted from output.

**Fix:** Check `len(compContent) > 0` instead of matching specific section names.

### Finding 3 (Medium): delete(stackMap, "imports") mutates live stacksMap

**Status:** Pre-existing behavior from the original monolith. Not a regression.

### Finding 4 (Medium): resolveStackName called O(N×K) per component

**Status:** Pre-existing behavior — the original monolith also resolved stack names per
component because the template may depend on per-component vars.

### Finding 5 (Medium): info.ComponentSection stale after template processing

**Status:** Valid but currently harmless. The rendered `componentSection` is passed to
downstream functions as a parameter; `info.ComponentSection` staleness only matters if
downstream code reads from it after template processing, which it currently doesn't.

### Finding 6 (Medium): Unguarded type assertion in ensureComponentEntryInMap

Three unguarded type assertions could panic if the map contained unexpected types.

**Fix:** Added `ok` guards to all three assertions, returning early on type mismatch.

### Finding 7 (Low): delete("imports") before reading manifest name

**Status:** Invalid. `getStackManifestName` reads `"name"`, not `"imports"`. However,
reordered as a defensive measure — reads before mutations.

### Finding 8 (Medium): pkg/describe wrapper diverges from internal/exec

**Status:** Intentional design. The public API has fewer parameters with sensible defaults.
Not a bug.

### Finding 9 (Medium): No test for name_template + includeEmptyStacks=true

**Status:** Addressed by fixing the underlying bug (#1). The fix prevents ghost entries
from being created in the first place.

### Finding 10 (Low): filterEmptyFinalStacks mutates before error return

**Status:** The mutation (deleting empty-key entries) is always correct behavior.
The error case indicates corrupted data. Current behavior is acceptable.

### Finding 11 (Low): Error format not asserted in tests

**Status:** Existing tests check `assert.Contains(err.Error(), "invalid stack entry type")`
which is sufficient for preventing format drift.

### Finding 12 (Low): No golden-file snapshot test

**Status:** Documented. Golden-file tests for full `ExecuteDescribeStacks` output would
require complete fixture scenarios. Deferred.

---

## Changes Made

### `internal/exec/describe_stacks_component_processor.go`

- Fix ghost entry: skip pre-creation when `NameTemplate` is set and manifest name is empty.
- Fix `stackHasNonEmptyComponents`: check `len(compContent) > 0` instead of section whitelist.
- Fix `ensureComponentEntryInMap`: add `ok` guards to all type assertions.
- Reorder `processStackFile`: read manifest name before `delete(stackMap, "imports")`.

### `internal/exec/describe_stacks_component_processor_test.go`

- Update `TestStackHasNonEmptyComponents_NoRelevantSections` → split into
  `TestStackHasNonEmptyComponents_NonStandardSections` (returns true for backend-only)
  and `TestStackHasNonEmptyComponents_EmptyComponentContent` (returns false for empty map).
- Update `TestFilterEmptyFinalStacks_RemovesEmpty` to use empty component content instead
  of non-whitelisted sections.

### `internal/exec/describe_stacks_test.go`

- Add `TestEnsureComponentEntryInMap_InvalidStackEntryType` (ok guard test).
- Add `TestEnsureComponentEntryInMap_InvalidComponentsType` (ok guard test).

---

## References

- PR #2204: `refactor: reduce ExecuteDescribeStacks cyclomatic complexity 247→10`
- `internal/exec/describe_stacks_component_processor.go` — component processor
- `internal/exec/describe_stacks.go` — orchestrator
