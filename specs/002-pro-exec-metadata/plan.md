# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is an eighth re-plan. `ExecutionID`, the `Data` inline-or-blob-URL redesign,
and the multi-component aggregation are already implemented and present in the current tree.
The seventh re-plan's three terraform-side deltas (output masking, `has_changes`/
`has_errors`/`errors`, `component`/`stack`) are designed (research.md Decisions 19-21) but
**not yet implemented** — `cmd/terraform/utils.go` still has no `maskSensitiveOutputs`
helper and `terraformOutputResultMirror` still only decodes `Data`, confirmed by re-reading
the file this session. This revision folds those three together with four more clarifications
from the same 2026-08-20 session, none of which are implemented or previously planned:

5. **FR-006b — `describe affected` structured data**: `describe affected` already calls
   `proexec.CaptureSync` (`internal/exec/describe_affected.go:379-382`, `Execute`), but with
   `Data` hardcoded to nothing — the doc comment on `Execute` literally states "Data is passed
   as nil — describe affected has no defined structured-data extension." The `[]schema.Affected`
   list this needs is already computed unconditionally by `executeInner` (every invocation, not
   only `--upload` ones) but is never returned to `Execute`, which only sees `error`.
6. **FR-006c — `list instances` structured data**: `pkg/list/list_instances.go` has **no**
   exec-metadata wiring at all today (confirmed: no `proexec` import in that file) — it relies
   entirely on the generic async default path (`cmd/root.go`'s `proexec.CaptureAsync(cmd, err)`,
   which always builds `Data: nil` since it has no command-specific knowledge). Attaching
   `list instances`' own `Data`, gated on `--upload`, requires a new hand-off from
   `list_instances.go` to the generic async hook — there is no existing mechanism for a
   non-sync-allowlisted command to attach its own `Data` to the async path.
7. **FR-005a — per-shape `version` field**: none of the three structured-`Data` shapes
   (`TerraformExecData`, and the two new ones above) currently has a `version` field.

## Summary

Three parts, landing together since two of them (describe affected, list instances) establish
the pattern the third (`version`) then applies uniformly across all shapes:

1. **`describe affected`** (`internal/exec/describe_affected.go`): `executeInner` gains a
   return value carrying the computed `[]schema.Affected` (or the struct is extended with an
   out-field — see Constraints) so `Execute` can build
   `Data: map[string]any{"version": 1, "stacks": affected}` and pass it to the existing
   `proexec.CaptureSync` call, replacing the current always-nil `Data`. No gating flag — the
   list is already computed on every invocation, matching `describe affected`'s existing
   unconditional-compute behavior (unlike `list instances`, below).
2. **`list instances`** (`pkg/list/list_instances.go`): a new `proexec.SetPendingAsyncData(data
   any)` package-level setter (mirroring the existing `SetAtmosConfig`/`currentAtmosConfig`
   pattern already in `pkg/proexec/async.go`) is called by `ExecuteListInstancesCmd` only when
   `--upload` was passed and the `[]UploadInstance` list was already built for
   `POST /api/v1/instances`, wrapping it as `map[string]any{"version": 1, "instances":
   instances}`. `CaptureAsync` (`pkg/proexec/async.go`) reads and clears this pending value when
   assembling its `ExecRecordInput`, defaulting to `nil` (today's behavior) when nothing was set.
3. **`version` field** (all three shapes): `describe affected` and `list instances` share an
   identical shape — a single array wrapped under one key (`stacks`/`instances`) — so a shared
   helper, `proexec.VersionedData(version int, key string, payload any) map[string]any` in
   `pkg/proexec`, builds `{"version": N, key: payload}` for both of those two call sites.
   `TerraformExecData` is structurally different (multiple top-level keys — `resource_counts`,
   `outputs`, `warnings`, `changes`, `has_changes`, `has_errors`, `errors`, and optionally
   `component`/`stack` — not one wrapped array), so `buildTerraformExecData` simply adds
   `"version": 1` as one more key in its own existing map literal directly, rather than being
   forced through a single-key wrapper that doesn't fit its shape (see Constraints/Alternatives).

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `cmd/terraform/utils.go` (existing, shipped, still unimplemented per Decisions 19-21) —
  unchanged from the seventh re-plan's design, plus one more key: `buildTerraformExecData`'s
  returned `map[string]any{...}` literal gains `"version": 1` directly alongside its existing
  keys (`resource_counts`, `outputs`, `warnings`, `changes`, `has_changes`, `has_errors`,
  `errors`, `component`, `stack`) — not via `VersionedData` (see Summary), since that shape has
  no single wrapped payload to key on.
- `internal/exec/describe_affected.go` (existing, shipped) —
  - `executeInner(a *DescribeAffectedCmdArgs) error` → `executeInner(a
    *DescribeAffectedCmdArgs) ([]schema.Affected, error)`: returns the already-computed
    `affected` slice (present whether or not `--upload` fired) alongside its existing error,
    so `Execute` can use it without recomputing or storing extra state.
  - `Execute(a *DescribeAffectedCmdArgs) error`: captures `executeInner`'s new `[]schema.Affected`
    return value and passes `Data: proexec.VersionedData(1, "stacks", affected)` into the
    existing `ExecRecordInput{Command: "describe affected", ...}` in place of the implicit nil.
- `pkg/proexec` (existing, shipped) —
  - `VersionedData(version int, key string, payload any) map[string]any` (new): returns
    `map[string]any{"version": version, key: payload}` — the one place the `{"version": N,
    <key>: ...}` wrapping convention is implemented.
  - `SetPendingAsyncData(data any)` / an unexported `pendingAsyncData` package-level var (new,
    `pkg/proexec/async.go`, mirroring the file's existing `SetAtmosConfig`/`currentAtmosConfig`
    pair): lets a command outside the synchronous allowlist attach its own `Data` before the
    generic `CaptureAsync(cmd, err)` hook (`cmd/root.go`) runs. `CaptureAsync` reads and clears
    it (`data := pendingAsyncData; pendingAsyncData = nil`) when building its `ExecRecordInput`,
    so a value set by one invocation can never leak into a later one within the same test
    process — required for `cmd.NewTestKit(t)`-style test isolation (CLAUDE.md, MANDATORY).
- `pkg/list/list_instances.go` (existing, shipped) — `ExecuteListInstancesCmd`, at the point
  `--upload`'s `apiClient.UploadInstances(&req)` call already has `req.Instances` built, calls
  `proexec.SetPendingAsyncData(proexec.VersionedData(1, "instances", req.Instances))` — only
  inside the existing `if opts.Upload { ... }` branch, never unconditionally.
- No new external dependency, no new package (`VersionedData`/`SetPendingAsyncData` land in the
  already-existing `pkg/proexec`, consistent with constitution Principle V — no new
  purpose-built package for three small additions to an already-purpose-built package).

**Storage**: N/A — unchanged.

**Testing**: `atmos test` (unit, table-driven). New/changed cases needed, in addition to the
seventh re-plan's still-outstanding terraform-side test list (masking, shape completeness,
`component`/`stack`):
- `proexec.VersionedData`: returns `{"version": N, key: payload}` for representative
  version/key/payload combinations, including a nil payload (still wraps, doesn't panic).
- `proexec.SetPendingAsyncData`/`CaptureAsync`: setting a value causes the next `CaptureAsync`
  call to use it as `Data`; a second `CaptureAsync` call (simulating a later command in the
  same test run, matching how `cmd.NewTestKit(t)` isolates state) sees `nil` again — proves the
  clear-on-read behavior prevents cross-test/cross-invocation leakage. Not calling
  `SetPendingAsyncData` at all (today's behavior for every command except `list instances`)
  continues to produce `Data: nil`.
- `describe_affected_test.go`: `executeInner`'s new `[]schema.Affected` return value matches
  what it already computes (extend existing test assertions, no new computation to test);
  `Execute` passes a non-nil `Data` shaped `{"version": 1, "stacks": [...]}` to `CaptureSync`
  (mock/spy `proexec.CaptureSync` the same way existing `describe_affected_test.go` cases
  already do), for both `--upload` and non-`--upload` invocations (the list is unconditional).
- `list_instances_test.go`: `SetPendingAsyncData` is called with `{"version": 1, "instances":
  [...]}` only when `--upload` is set; not called at all when `--upload` is absent (assert via
  a test double / by resetting `pendingAsyncData` before and inspecting it after, matching the
  test style already used for `currentAtmosConfig` in this package's existing tests).
- `pkg/proexec/envelope_test.go`: extend the masking-layer-independence test (already planned
  in the seventh re-plan) to also confirm a `version` field survives both the
  `maskSensitiveOutputs`/structural pass and the Gitleaks `maskedDataJSON` pass unchanged (an
  integer is never a masking target, but the field's presence end-to-end is worth asserting
  once, e.g. in `TestBuildRecord_SecretMaskingAppliedToData`).

**Target Platform**: Linux, macOS, Windows — unchanged; all deltas are pure data-plumbing with
no OS-specific behavior.

**Project Type**: CLI feature — targeted additions to three already-shipped files plus one new
pair of small exported functions in an already-shipped package. No new packages. All three
structured-`Data` shapes gain a `version` field (additive) and two commands
(`describe affected`, `list instances`) gain a `Data` shape where none existed before
(additive — `FR-005`'s existing "absent Data is valid" behavior is what they had until now).
`contracts/interactions.md` and the Pact contract need regenerating for all of this, including
the new 3-shapes-times-2-modes coverage (Assumptions/FR-013 extension).

**Performance Goals**: Unchanged for `describe affected` (the `[]schema.Affected` list it
attaches is already computed unconditionally today — this delta adds zero new computation,
only a slice being returned instead of discarded). For `list instances`, `Data` attachment is
strictly conditional on `--upload` already having built the list — the plain, non-uploading
invocation (the common case) pays no new cost, matching FR-006c's explicit requirement.

**Constraints**:
- `describe affected`'s `Data` MUST NOT be gated on `--upload` — FR-006b does not condition it
  the way FR-006c conditions `list instances`, because the underlying `[]schema.Affected` list
  is already computed for every invocation (it's the command's entire purpose), unlike
  `list instances`' `[]UploadInstance` list, which is only built when `--upload` triggers it.
  This asymmetry is intentional, not an inconsistency to "fix" — see Alternatives.
- `executeInner`'s new `[]schema.Affected` return value MUST be the same slice `Execute`
  already indirectly triggers computation of today (via `executeInner`'s existing internal
  call chain) — no second resolution pass, no risk of a second run producing a different
  answer (e.g. due to non-determinism in stack resolution) than what was actually reported to
  `--upload`/the user's own table/JSON output.
- `SetPendingAsyncData`/`pendingAsyncData` MUST be cleared on read by `CaptureAsync`, not left
  to a caller's discretion — an uncleared value would leak into the next command's execution
  record within the same process (relevant for tests, and for any future in-process
  multi-command execution path), silently misattributing `list instances`' data to an unrelated
  later command.
- `version` MUST be a plain `int` literal at each of the three sites (`1`, `1`, `1` today,
  whether via `VersionedData`'s parameter or `buildTerraformExecData`'s own map literal), not
  derived from any external source (Atmos release version, build metadata) — matches FR-005a's
  explicit "not tied to the Atmos release version" rule.
- No new user-facing configuration surface for any of the seven deltas in this plan — all are
  corrections/extensions to already-specified behavior (FR-005a/FR-006b/FR-006c plus the
  seventh re-plan's three), not new capabilities needing a flag.
- `internal/exec` MUST NOT gain a new import of `pkg/ci/plugins/terraform`/
  `pkg/ci/internal/plugin` (Decision 18's constraint, unaffected — none of this plan's four new
  deltas touch that import boundary).

**Scale/Scope** (this delta, on top of the still-outstanding seventh re-plan scope): 2 new
exported functions in `pkg/proexec` (`VersionedData`, `SetPendingAsyncData`) plus 1 new
unexported package-level var, 1 changed function signature
(`describe_affected.go:executeInner`), 2 call-site changes (`describe_affected.go:Execute`,
`list_instances.go:ExecuteListInstancesCmd`), 1 read-and-clear addition in
`pkg/proexec/async.go:CaptureAsync`. 0 new packages, 0 new non-test files, 0 breaking
wire-shape changes (additive only — a `version` field added to an already-additive shape, and
`Data` populated for two commands that previously always sent `Data: nil`, which FR-005
already documents as a valid, unremarkable state).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags/endpoints. `SetPendingAsyncData` is a narrow, single-purpose hand-off (one setter, one reader, one package) — not a new extensibility mechanism competing with the registry pattern; it mirrors an already-established precedent (`SetAtmosConfig`/`currentAtmosConfig`) in the same file rather than inventing a new one. |
| II. Interface-Driven Design with DI | ✅ Pass | `VersionedData` is a pure function; `SetPendingAsyncData`/`CaptureAsync`'s package-level var mirrors the existing `currentAtmosConfig` pattern exactly (same file, same rationale: a hook called at a `(cmd, err)`-only call site needs out-of-band access to data only available earlier in a different package). No interface needed since there is exactly one producer (`list_instances.go`) and one consumer (`CaptureAsync`) today. |
| III. Test-First with 80% Coverage | ✅ Pass | New tests planned for every new/changed function before implementation (see Testing above), including the clear-on-read leak-prevention case. |
| IV. Separated I/O and UI Architecture | ✅ Pass | No new user-visible output. |
| V. Simplicity and No Over-Engineering | ✅ Pass | Rejected a general-purpose "structured-data provider registry" (e.g. a `map[string]func() any` keyed by command path, or a new interface every command type implements) in favor of two narrow, purpose-fit mechanisms — a direct return-value plumb for `describe affected` (already synchronous, already has the data in scope) and a minimal set/clear hand-off for `list instances` (async, generic hook, no data in scope at the hook site) — because today there are exactly two producers with two different shapes of "not having the data available at the generic call site," and a registry abstracting over a set of two, with no third planned, is premature per YAGNI. `VersionedData` was chosen over two copied literals (`describe affected`, `list instances` — the only two sites that genuinely share the identical single-key-wrap shape) because that duplication is the "three similar lines" the constitution tolerates edging into "would benefit from one non-duplicated helper"; `TerraformExecData`'s structurally different multi-key shape was deliberately kept OUT of `VersionedData` rather than warping the helper (e.g. a variadic/merge-based signature) to accommodate a third, differently-shaped caller — a narrower helper serving its two actual matching callers beats a more "flexible" one serving three mismatched ones. |

**Post-design re-check**: Pending Phase 1 completion (this plan updates `data-model.md`,
`contracts/interactions.md`, and `quickstart.md` in the same pass). No new violations
anticipated — all four new deltas are additive extensions consistent with patterns this
feature has already established and accepted (Decisions 12-21).

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md                 # This file — eighth re-plan: describe affected + list instances Data, version field
├── research.md              # Phase 0 output — Decisions 19-21 (7th re-plan, still unimplemented)
│                              plus new Decisions 22 (describe affected), 23 (list instances +
│                              SetPendingAsyncData), 24 (VersionedData/version field), 25 (Pact
│                              per-shape coverage)
├── data-model.md            # Phase 1 output — new AffectedStacksExecData/InstancesExecData
│                              sections, version field noted on all three Data shapes, Delivery
│                              Classification table's `describe affected`/list-instances-adjacent
│                              rows updated
├── quickstart.md            # Phase 1 output — new steps verifying describe affected's Data,
│                              list instances' Data (with/without --upload), version field
├── contracts/
│   └── interactions.md      # Updated — two new Data shapes' fields documented, version field
│                              added to every `data` example, 6-interaction-total Pact coverage
│                              requirement (3 shapes x 2 modes) documented with interaction
│                              numbering extended past 11
└── tasks.md                  # Phase 2 output (/speckit-tasks) — NEEDS REGENERATION for all
                                seven still-unimplemented deltas across the 7th and 8th re-plans
```

### Source Code (repository root)

```text
pkg/proexec/async.go
├── pendingAsyncData      # New unexported package-level var — mirrors currentAtmosConfig
├── SetPendingAsyncData    # New — sets pendingAsyncData; called by list_instances.go
└── CaptureAsync           # Modified — reads and clears pendingAsyncData for ExecRecordInput.Data

pkg/proexec/ (new file or added to an existing small file, e.g. envelope.go)
└── VersionedData          # New — map[string]any{"version": version, key: payload}

internal/exec/describe_affected.go
├── executeInner            # Modified — returns ([]schema.Affected, error) instead of error
└── Execute                 # Modified — builds Data via proexec.VersionedData(1, "stacks", affected)

pkg/list/list_instances.go
└── ExecuteListInstancesCmd  # Modified — inside the existing --upload branch, calls
                               proexec.SetPendingAsyncData(proexec.VersionedData(1, "instances", req.Instances))

cmd/terraform/utils.go
└── buildTerraformExecData   # Modified (still from 7th re-plan, now also) — adds "version": 1
                               directly to its own map[string]any{...} literal, alongside
                               resource_counts/outputs/warnings/changes/has_changes/has_errors/
                               errors/component/stack — does NOT use proexec.VersionedData,
                               since that shape has multiple top-level keys, not one wrapped
                               payload (see Summary/Constraints).
```

**Structure Decision**: No new packages, no new non-test files beyond the modified files above.
`VersionedData`/`SetPendingAsyncData` land inside the already-existing `pkg/proexec`, not a new
package, consistent with constitution Principle V and this feature's established pattern
(Decisions 12/17/18/19-21) of preferring the smallest change that closes a confirmed gap.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
