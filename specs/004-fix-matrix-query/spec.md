# Feature Specification: `--filter` for `describe affected` and No Silent `--query` with Matrix Output

**Feature Branch**: `1200-fix-matrix-query`

**Created**: 2026-10-30

**Status**: Draft

**Input**: User description: "`atmos describe affected --format=matrix` silently ignores `--query`, so CI matrices include components that the query should have filtered out. Instead of making `--query` reshape matrix output, add a `--filter` flag (like the one on `list instances`) that selects which affected components are kept, for every output format, including matrix. Using `--query` with `--format=matrix` must fail with a hint pointing to `--filter`."

## Clarifications

### Session 2026-10-30

- Q: Fix `--query` for matrix output, add `--filter`, or both? → A: Add `--filter` only. `--query` combined with `--format=matrix` fails with a hint pointing to `--filter`. The release is `minor`.
- Q: Single-object `--query` results for matrix output? → A: No longer applicable. `--query` is not accepted with matrix output.
- Q: What is `--filter` evaluated against when dependents are included? → A: Every item, including dependents at every level. A filtered-out item is dropped together with all of its dependents, even dependents that would satisfy the filter on their own.
- Q: How does `--filter` behave with `--upload`? → A: Mirror `--tags`/`--labels`. Passing `--filter` as a flag with `--upload` fails with an error. A filter that comes only from the environment variable is ignored with a warning, because the upload is always unfiltered.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Filter the CI matrix by component properties (Priority: P1)

A platform engineer runs `atmos describe affected` in a CI workflow and feeds the result into a job matrix. They want to keep only components that opted in (for example, those whose `settings.github.actions_enabled` is true). They pass `--filter` with a condition and `--format=matrix`. Components that don't satisfy the condition are left out of the matrix, so no CI jobs run for them.

**Why this priority**: This is the original need behind the bug report. Without it, jobs run for components that were deliberately excluded.

**Independent Test**: With one affected component whose `settings.github.actions_enabled` is `false`, run `describe affected --include-settings --filter '<actions_enabled is true>' --format=matrix`. The output is `{"include":[]}`.

**Acceptance Scenarios**:

1. **Given** one affected component with `actions_enabled: false`, **When** the user runs `describe affected --include-settings --filter '.settings.github.actions_enabled // false' --format=matrix`, **Then** the output is `{"include":[]}`.
2. **Given** two affected components, one enabled and one not, **When** the same command runs, **Then** the matrix contains exactly one entry, for the enabled component, with the same fields as an unfiltered entry (stack, component, component path, component type).
3. **Given** a filter that is true for every affected component, **When** the command runs with `--format=matrix`, **Then** the matrix is identical to the one produced without `--filter`.

---

### User Story 2 - The same filter works for every output format (Priority: P1)

A user applies `--filter` with `--format=json` or `--format=yaml` and gets the same set of components that the matrix would contain. The items keep their full shape and only the selection changes.

**Why this priority**: The filter must behave the same everywhere, or it will recreate the same class of silent bug.

**Independent Test**: Run the same `--filter` with `json`, `yaml`, and `matrix`, and confirm the three outputs list the same components.

**Acceptance Scenarios**:

1. **Given** affected components and a filter, **When** the user runs with `--format=json`, **Then** the output lists only the components that satisfy the filter, unchanged in shape.
2. **Given** the same inputs, **When** the user runs with `--format=yaml` and `--format=matrix`, **Then** the set of components is identical across all three formats.
3. **Given** a filter that matches nothing, **When** the command runs with json or yaml, **Then** it succeeds and outputs an empty list.

---

### User Story 3 - `--query` with matrix output fails loudly (Priority: P1)

A user who still passes `--query` with `--format=matrix` (as in the original report) no longer gets a silently unfiltered matrix. The command stops with an error that explains `--query` is not supported for matrix output and points to `--filter`.

**Why this priority**: This closes the silent-failure hole in the original bug. Existing pipelines learn about the problem immediately instead of running unintended jobs.

**Independent Test**: Run `describe affected --format=matrix --query '<any>'` and confirm it exits non-zero, writes no matrix, and the message names `--filter`.

**Acceptance Scenarios**:

1. **Given** affected components, **When** the user runs with `--format=matrix` and `--query`, **Then** the command fails with an error that says `--query` can't be used with matrix output and suggests `--filter`, and no matrix is written.
2. **Given** `--query` with any non-matrix format, **When** the command runs, **Then** it behaves exactly as before.

---

### User Story 4 - Existing behavior is unchanged without the new flag (Priority: P1)

A user who passes neither `--filter` nor `--query` with matrix output gets exactly the same output as before, including when the matrix is written to a file or the CI output variable.

**Why this priority**: The new flag must not regress existing pipelines.

**Independent Test**: Run `describe affected --format=matrix` without `--filter` and compare the output, byte for byte, with the pre-change output.

**Acceptance Scenarios**:

1. **Given** affected components and no `--filter`, **When** the user runs `describe affected --format=matrix`, **Then** the output is identical to the pre-change output.
2. **Given** a CI environment with an output file configured (explicit flag or auto-detected), **When** the command runs with `--filter`, **Then** the filtered matrix is written to that file in the usual format.

---

### User Story 5 - `--filter` combines with existing selectors and options (Priority: P2)

A user combines `--filter` with the existing `--tags` and `--labels` selectors. The result is the components that satisfy both. Combining `--filter` with `--upload` follows the same rule as `--tags` and `--labels`: the upload is never filtered, so a `--filter` flag with `--upload` is rejected with an error, and a filter set only through the environment variable is ignored with a warning, instead of silently filtering only the printed output.

**Why this priority**: It keeps the filtering behavior of the command coherent. It matters less than the core fix.

**Independent Test**: Run with `--filter` and `--tags` together and confirm only components satisfying both remain.

**Acceptance Scenarios**:

1. **Given** affected components, **When** the user passes `--filter` and `--tags`, **Then** only components that match both are returned.
2. **Given** the `--filter` flag together with `--upload`, **When** the command runs, **Then** it fails with an error explaining that the upload is always unfiltered, and nothing is uploaded.
3. **Given** a filter set only through the environment variable together with `--upload`, **When** the command runs, **Then** the filter is ignored with a warning and the full affected set is uploaded.

---

### Edge Cases

- A filter matches nothing: output is the valid empty matrix `{"include":[]}` for matrix format, and an empty list for json/yaml. This is never an error and never the unfiltered list.
- A filter has a syntax error: reported as a clear error, and no output is written.
- A filter evaluates to a non-boolean value for an item (for example, a string): the item is kept only when the value is truthy. Null, false, and empty count as not kept (see Assumptions).
- A filter references a setting that isn't loaded because `--include-settings` was not passed: the value is missing, so the item is not kept when the expression defaults to false. This is the same behavior as other formats.
- An empty or unset `--filter` is a no-op.
- Dependents are included and a dependent at any level fails the filter: that dependent and everything beneath it are dropped, while its siblings that pass the filter are kept.
- Dependents are included and a parent fails the filter but one of its dependents would pass on its own: the dependent is dropped too, because it hangs under a filtered-out item.
- Dependents are flattened into the top-level list: the filter is applied to each item with the same pruning rule, evaluated before flattening, so a dependent whose ancestor was filtered out does not appear.
- `--query` and `--filter` are both set with a non-matrix format: the filter selects items first, then the query runs on the filtered list.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `describe affected` MUST accept a `--filter` option holding an expression that decides, for each affected component, whether it is kept. This follows the filter on `list instances`.
- **FR-002**: `--filter` MUST apply to every output format, including matrix, and the set of components MUST be the same across formats.
- **FR-003**: Filtering MUST NOT change the shape of kept items. Each kept item is identical to what it would be without the filter.
- **FR-004**: When no component satisfies the filter, the matrix output MUST be `{"include":[]}` and the command MUST exit successfully. For json and yaml the output is an empty list.
- **FR-005**: A kept component's matrix entry MUST contain the same fields as an unfiltered matrix entry. The matrix format itself is unchanged.
- **FR-006**: Using `--query` together with `--format=matrix` MUST fail with a clear error that explains the combination is unsupported and recommends `--filter`. No matrix may be written.
- **FR-007**: `--query` behavior for non-matrix formats MUST remain unchanged.
- **FR-008**: Matrix output without `--filter` and without `--query` MUST remain byte-identical to the current output, including writing to an explicit output file or the auto-detected CI output file.
- **FR-009**: `--filter` MUST be combinable with `--tags` and `--labels` (a component must satisfy all), and its use with `--upload` MUST follow the same rule as those selectors: the `--filter` flag with `--upload` MUST fail with a clear error, and a filter coming only from the environment variable MUST be ignored with a warning. The upload is always unfiltered.
- **FR-009a**: When dependents are included, `--filter` MUST be evaluated on every item at every dependency level. An item that fails the filter MUST be removed together with all of its dependents, regardless of whether those dependents would satisfy the filter themselves. This also holds when dependents are flattened into the top-level list.
- **FR-010**: An invalid filter expression MUST produce a clear error and no output.
- **FR-011**: `--filter` MUST be configurable through an environment variable, consistent with other flags, and MUST follow the standard precedence (flag, then environment, then config, then default).
- **FR-012**: User documentation for `describe affected` MUST document `--filter`, the `--query` restriction for matrix output, and an example that filters a CI matrix by a component setting.
- **FR-013**: The change MUST be covered by automated tests, including a regression test that reproduces the original report (a `--query` with `--format=matrix` that previously produced an unfiltered matrix, and now fails with the hint). Tests also cover: filter matching none, a subset, and all; the same result across json, yaml, and matrix; no filter (unchanged); invalid expression; filter combined with tags/labels; dependents pruned under a filtered-out item at the first and deeper levels (including when a dependent would pass on its own, and when flattened); and filtered output written to a file.
- **FR-014**: Configuration schemas and CLI help for the command MUST be updated for the new flag.
- **FR-015**: The release MUST be recorded per repository conventions: a fix or feature record, a `minor` label, a blog post, and a roadmap update.

### Key Entities

- **Affected component**: A component in a stack that is affected by a change. It has a stack name, component name, component path, component type, and optionally its settings.
- **Matrix entry**: The reduced form of an affected component used by CI job matrices: stack, component, component path, and component type.
- **Filter**: A user-supplied condition evaluated per affected component. It selects which components to keep and never changes their shape.
- **Query**: A user-supplied expression that can reshape the whole result. It remains available for non-matrix formats only.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In the reported scenario, the filtered matrix output is `{"include":[]}`, and 0 jobs are created for the filtered-out component.
- **SC-002**: For 100% of tested filters (no match, subset, all), the set of components is identical across json, yaml, and matrix output.
- **SC-003**: Matrix output without `--filter` and `--query` is byte-identical before and after the change across the existing test suite, with 0 regressions.
- **SC-004**: 100% of runs that combine `--query` with `--format=matrix` exit non-zero with a message naming `--filter`, and 0 such runs write a matrix.
- **SC-005**: The regression test for the original report fails on the pre-change code and passes after the change.
- **SC-006**: The documentation contains at least one working example of a filtered CI matrix using `--filter`.

## Assumptions

- The matrix JSON schema (the `include` list and the fields in each entry) is unchanged.
- The filter is written in the same expression language as the filter on `list instances` and the existing `--query`, so users don't need to learn a new syntax. It is evaluated against each affected component, and the component is kept when the result is truthy (not false, null, empty, or zero).
- Settings are only visible to a filter when the user passes `--include-settings`, as for `--query`.
- Existing behavior of the matrix-only output flag for non-matrix formats (rejecting it) stays as it is.
- The environment variable name and the shared flag wiring are decided in planning.
- Other commands that offer matrix output are reviewed during planning for the same silent-ignore problem, and any such case is made to fail loudly or support `--filter`.
- The spec directory and git branch are still named `fix-matrix-query`. Renaming them is optional and does not affect the content.
