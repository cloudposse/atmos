# Feature Specification: Full Component Name in Atmos Pro Uploads

**Feature Branch**: `1199-fix-upload-component-name`

**Created**: 2026-09-10

**Status**: Draft

**Input**: User description: "Fix GitHub issue #3102 — atmos terraform plan/apply --upload-status reports a truncated component name for nested (path-style) components. Code review of the actual codebase found the issue's proposed single-call-site fix is incomplete: there are two distinct truncation sites, and the exact repro in the issue is caused by a different upload than the one the issue proposes to fix."

## Problem Statement

A component's logical name (the key under `components.terraform:` in a stack manifest) may contain slashes — e.g. `foo/bar/baz`. Atmos internally splits such a name to locate the Terraform working directory, keeping only the last path segment (`baz`) in one of its identity fields. Two of the payloads Atmos uploads to Atmos Pro read that truncated field instead of the full logical name:

1. **Instance-status upload** (`--upload-status`): the reported component is the truncated leaf in **all** invocations (single- and multi-component).
2. **Execution-record component entries** in **multi-component** runs (`--affected`, `--all`, `--components`, `--query`): each per-component entry reports the truncated leaf.

Single-component execution records already report the full logical name (taken from the CLI argument) and are not affected.

Because other identity channels (stack locks, affected-component uploads, single-component execution records) report the full logical name, any exact-name correlation on the receiving side fails for nested components. Concretely, the Atmos Pro approvals page's "previous plan" lookup finds no match, showing **"No previous plan found for this component"** even though the plan ran and uploaded successfully.

Components whose logical name contains no slash are unaffected.

## Clarifications

### Session 2026-09-10

- Q: How should the pre-existing path-style-argument quirk (raw filesystem path reported as component identity in single-component execution records) be handled in this feature? → A: Defer — open a follow-up GitHub issue now and link it; this feature fixes only the two truncation sites.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Single-component upload reports full name (Priority: P1)

An operator runs `atmos terraform plan "foo/bar/baz" -s <stack> --upload-status` for a nested component. The status uploaded to Atmos Pro identifies the component by its full logical name `foo/bar/baz`, so the approvals page correlates the plan with the pending approval and shows the previous plan.

**Why this priority**: This is the exact reported repro and the primary user-visible breakage (approvals page shows "No previous plan found"). It is the most common invocation shape in CI.

**Independent Test**: Run a plan with `--upload-status` for a slash-named component against a captured/mock upload sink and assert the reported component equals the full CLI argument.

**Acceptance Scenarios**:

1. **Given** a stack defining component `foo/bar/baz`, **When** the operator runs a plan with status upload enabled, **Then** the uploaded status identifies the component as `foo/bar/baz`, not `baz`.
2. **Given** a stack defining component `vpc` (no slash), **When** the operator runs a plan with status upload enabled, **Then** the uploaded status identifies the component as `vpc` (behavior unchanged).

---

### User Story 2 - Multi-component execution records report full names (Priority: P2)

An operator runs a multi-component invocation (`--affected` or `--all`) that includes nested components. The single aggregated execution record uploaded after the run identifies every component by its full logical name.

**Why this priority**: Same class of mismatch, but on the execution-record channel and only for bulk runs; less frequent than the single-component repro but silently corrupts per-component attribution in aggregate records.

**Independent Test**: Feed a per-component result for a slash-named component through the aggregation path and assert the recorded entry carries the full logical name.

**Acceptance Scenarios**:

1. **Given** a bulk run whose graph contains component `foo/bar/baz`, **When** its per-component result is recorded into the aggregate execution record, **Then** that entry's component identity is `foo/bar/baz`.
2. **Given** a bulk run with a mix of nested and flat component names, **When** the aggregate record is built, **Then** every entry carries the exact logical name used in the stack manifest.

---

### User Story 3 - Regression guard for already-correct channels (Priority: P3)

The channels that already report the full logical name — single-component execution records, stack lock/unlock keys, affected-component uploads — continue to do so, and Terraform working-directory resolution for nested components is unchanged.

**Why this priority**: The fix touches identity fields near heavily shared plumbing; the split that produces the truncated field is intentional and load-bearing for working-directory resolution and must not change.

**Independent Test**: Existing tests for working-directory resolution and single-component execution records pass unmodified; a new test asserts the single-component execution record keeps the full name.

**Acceptance Scenarios**:

1. **Given** a single-component plan of `foo/bar/baz`, **When** its execution record is built, **Then** the component entry is `foo/bar/baz` (as today).
2. **Given** a nested component, **When** any terraform command resolves its working directory, **Then** resolution behavior is byte-for-byte identical to before the fix.

### Edge Cases

- **Component with a `component:` attribute** (logical name differs from the Terraform directory): uploads must report the logical name (the stack-manifest key), never the Terraform directory name.
- **Path-style CLI argument** (`atmos terraform plan ./components/terraform/vpc`): the raw filesystem path is currently reported as the component identity in the single-component execution-record channel — a known pre-existing quirk, explicitly out of scope here and deferred to a linked follow-up GitHub issue (see FR-008).
- **No-slash components**: reported identity must be byte-identical to current behavior — no migration impact for the overwhelmingly common case.
- **Bulk run where the run-level component argument is empty** (placeholder-based dispatch): per-component identity must come from each graph node's own logical name, never from the run-level argument.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every upload to Atmos Pro that identifies a component MUST report the component's full logical name exactly as it appears as the stack-manifest key (e.g. `foo/bar/baz`), never a truncated segment of it.
- **FR-002**: The instance-status upload (`--upload-status`) MUST report the full logical name in both single-component and multi-component invocations.
- **FR-003**: In multi-component runs, each per-component entry of the aggregated execution record MUST report the full logical name of its graph node.
- **FR-004**: Single-component execution records MUST continue to report the full logical name (regression guard — this channel is correct today).
- **FR-005**: Terraform working-directory resolution for nested components MUST be unchanged; the internal split of a slash-containing name into folder prefix and leaf remains as-is.
- **FR-006**: Components whose logical name contains no slash MUST have byte-identical reported identity before and after the fix.
- **FR-007**: Tests MUST cover a nested (slash-containing) component name for each affected upload channel, plus a flat-name control case.
- **FR-008**: A follow-up GitHub issue for the path-style-argument quirk MUST be opened and linked by number in this feature's PR description before merge, per the repository's follow-up-tracking policy; no code change for that quirk lands in this feature.

### Key Entities

- **Logical component name**: The key under `components.terraform:` in a stack manifest; may contain slashes; the sole component identity Atmos Pro correlates on.
- **Instance-status upload**: The per-invocation status payload sent when `--upload-status` is enabled; carries stack, component, command, and exit code.
- **Execution record**: The per-invocation record capturing terraform plan/apply/deploy results; single-component runs produce one record with one component entry, bulk runs produce one aggregate record with one entry per graph node.
- **Working-directory identity (leaf + folder prefix)**: The internal split of a slash-containing name used to locate the Terraform component directory; intentionally different from the logical name and out of scope for change.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a nested component, the Atmos Pro approvals page finds the previous plan (no "No previous plan found for this component") — the uploaded component identity matches the approval's component name exactly, 100% of the time.
- **SC-002**: All Atmos Pro upload channels report identical component identity for the same invocation (instance status, execution records, locks, affected uploads) — zero cross-channel mismatches for nested components.
- **SC-003**: Zero behavior change for components without slashes in their name, verified by the existing test suite passing unmodified.
- **SC-004**: Working-directory resolution behavior is unchanged for all component name shapes, verified by existing tests passing unmodified.

## Assumptions

- The GitHub issue's proposed one-line fix is insufficient: code review established that the issue's repro (single-component `--upload-status`) is caused by the instance-status upload, while the issue's proposed change only fixes the multi-component execution-record channel. Both sites are in scope for this feature.
- The server-side identity shift is acceptable: Atmos Pro instances previously recorded under the truncated leaf name (`baz`) will appear under the full name (`foo/bar/baz`) after the fix. This is the intended outcome; reconciling historical rows is the server's concern and out of scope here.
- The pre-existing path-style-argument quirk (raw filesystem path reported as component identity in the single-component execution-record channel) is a sibling bug, explicitly out of scope: it is deferred to a follow-up GitHub issue opened and linked before this feature merges (FR-008).
- No consumer contract (pact) fixtures currently assume the truncated value (verified: existing fixtures use flat names like `vpc`); if any turn up during implementation they are updated to the full-name contract.
- Multi-component graph nodes always carry the full logical name as their node identity, so the full name is available at every point where uploads are built (verified in code review).
