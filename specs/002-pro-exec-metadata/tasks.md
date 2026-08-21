---

description: "Task list for Atmos Pro Command-Execution Metadata Upload — remaining work"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload

**Input**: Design documents from `/specs/002-pro-exec-metadata/`

**Prerequisites**: plan.md (ninth re-plan, 2026-08-20), spec.md, research.md, data-model.md,
contracts/interactions.md, quickstart.md

**Tests**: Included — the constitution's Test-First principle (III) is NON-NEGOTIABLE and
CLAUDE.md's Bug-Fixing Workflow requires a failing regression test before any behavior
change.

**Regenerated in full** (not patched) — the previous tasks.md (T001-T026, eighth re-plan)
covered masking/`has_changes`/`has_errors`/`errors`/`component`/`stack`/`version`/
`describe affected`/`list instances` structured data. All 26 of those tasks are marked done
and confirmed still correct in the current tree. None of that work is re-tasked here.

**This regeneration's scope** — five new deltas from a second 2026-08-20 `/speckit-clarify`
session, triggered by a real CI payload (`atmos-pro-qa-3` run 32412509172) showing
`errors: null`, all-zero `resource_counts`, and `outputs: {}` alongside `has_changes: true`:

1. Decision 26 — list-typed `Data` fields (`changes`/`warnings`/`errors`) must serialize as
   `[]`, never `null`, when empty.
2. Decision 27 — `TerraformExecData` gains an `exit_code` field: the terraform/tofu
   subprocess's own exit code, the authoritative pass/fail/parse-completeness signal.
3. Decision 28 — `exit_code` is per-component (reusing the already-existing
   `execNodeResult.ExitCode`) for multi-component runs, never a single aggregate value.
4. Decision 29 — `buildTerraformExecData` must still attach a minimal `Data` payload
   (`version`/`exit_code`/`component`/`stack` + defaulted fields) even when itemized parsing
   fails entirely, instead of returning `nil`.
5. **Decision 30, retracted during implementation → Decision 30r**: a `deploy`-specific
   two-phase `{plan, apply}` shape was originally planned, on the premise that `deploy` runs
   plan and apply as two separate terraform/tofu subprocess invocations. **That premise was
   discovered false while implementing T037/T040 below**: `internal/exec/terraform.go`'s
   `handleDeploySubcommand` rewrites `deploy` to `apply` in place *before* any subprocess
   runs, so `deploy` executes exactly one subprocess — there is no independent plan-phase
   output/exit-code to split out. Decision 30r reverts `deploy` to the identical
   `TerraformExecData` shape as `plan`/`apply` (picking up deltas 1-4 automatically, no
   `deploy`-specific code). The tasks below reflect this correction — deploy-two-phase tasks
   from the original regeneration were dropped/repurposed, not silently deleted (see the
   "Retracted" note in Phase 4).

**Spec-mapping note**: All deltas extend `TerraformExecData`/`FR-006`, which spec.md maps
entirely to **User Story 3** (P3). No new user story is needed.

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

No setup tasks — no new packages, no new external dependencies.

---

## Phase 2: Foundational (Blocking Prerequisites)

**None.** This regeneration's deltas are internal to `cmd/terraform/utils.go`'s already-
existing `TerraformExecData` construction (plus a closure-signature change in
`internal/exec/shell_utils.go`/`terraform.go` to thread `exit_code` through) — no shared
primitive is extracted, and `execNodeResult.ExitCode` (Decision 28's reuse target) already
exists.

---

## Phase 3: User Story 1 / User Story 2 (Priority: P1/P2)

**No remaining tasks.** Base envelope fields, delivery classification, and multi-component
aggregation mechanics are unaffected by this regeneration's deltas.

---

## Phase 4: User Story 3 - Structured infrastructure-change data for plan/apply/deploy (Priority: P3)

**Goal**: Close the gaps a real CI payload exposed in the already-shipped `TerraformExecData`
shape: `null` instead of `[]` for empty lists, no `exit_code` field, no regression guard for
its per-component scoping, and `Data` silently omitted when parsing fails entirely.
`terraform deploy` reuses this same shape unchanged (Decision 30r).

**Independent Test**: Run `atmos terraform plan <component> -s <stack>` against a component
with no warnings/errors/changes and confirm `data.changes`/`data.warnings`/`data.errors` are
`[]` (not `null`) and `data.exit_code` matches the terraform subprocess's own exit code
(quickstart.md steps 18-19). Run `atmos terraform deploy <component> -s <stack>` and confirm
`data` uses the identical `TerraformExecData` shape as `plan`/`apply`, now also carrying
`exit_code`.

### Tests for User Story 3 (this regeneration) ⚠️

- [X] T027 [P] [US3] Test in `cmd/terraform/utils_exec_metadata_test.go`
  (`TestBuildTerraformExecData_EmptyListsAreNotNull`): given output with no warnings, no
  errors, and no resource changes, `buildTerraformExecData`'s returned map's `"changes"`,
  `"warnings"`, `"errors"` values each marshal to JSON `[]`, never `null`.
- [X] T028 [P] [US3] Test in `cmd/terraform/utils_exec_metadata_test.go`
  (`TestBuildTerraformExecData_ExitCode`, folded into the extended
  `TestBuildTerraformExecData_ApplySuccess`/`_ApplyFailure`): `buildTerraformExecData`'s
  returned map's `"exit_code"` equals the `exitCode` argument, for both a `0` and a non-zero
  case.
- [X] T029 [P] [US3] Extended `TestTerraformNodeHooks_RecordExecResultAccumulates` in
  `cmd/terraform/utils_exec_metadata_test.go` (multi-component path) with a doc-comment
  callout that its existing two-different-`ExitCode`-values assertion is the regression guard
  for research.md Decision 28 (per-component `exit_code` scoping) — the assertions already
  existed and already proved this; only the explanatory comment was added.
- [X] T030 [P] [US3] Test in `cmd/terraform/utils_exec_metadata_test.go`
  (`TestBuildTerraformExecData_UnparseableOutputStillAttachesMinimalData`): given output
  `parseTerraformOutputMirror` cannot decode at all, `buildTerraformExecData` returns a
  non-nil map with `"version"`, `"exit_code"`, `"component"`/`"stack"` set, and every
  unparseable field defaulted (`resource_counts` all-zero, `outputs: {}`,
  `changes`/`warnings`/`errors: []`, `has_changes`/`has_errors: false`).
- [X] T031 **[RETRACTED — Decision 30r]** ~~Test for `buildTerraformDeployExecData`'s
  two-phase shape~~ — no longer applicable; `deploy` uses the identical single-object shape,
  already covered by the pre-existing `TestBuildTerraformExecData_DeployParsedAsApply`.
- [X] T032 [US3] Extended `TestExecMetadataParserFromOpts_RoundTrips`/
  `TestCaptureExecMetadataSync_CallsParserForSyncAllowlistedSingleComponent` (and sibling
  tests) in `internal/exec/` asserting the parser closure is invoked with the invocation's own
  exit code (via `errUtils.GetExitCode(params.Err)`), not a hardcoded `0` — call-site
  assertions updated for the new `func(subCommand string, exitCode int) any` closure
  signature.
- [X] T033 **[RETRACTED — Decision 30r]** ~~Test asserting `deploy`'s `RunE` supplies
  phase-separated plan/apply output and exit codes~~ — no longer applicable; `deploy` reaches
  the same closure `plan`/`apply` use, no phase separation exists or is needed.

### Implementation for User Story 3 (this regeneration)

- [X] T034 [US3] In `cmd/terraform/utils.go`'s `buildTerraformExecData`: `nonNilStrings`/
  `nonNilChanges` helpers ensure `changes`/`warnings`/`errors` are non-nil, zero-length slices
  before assignment into the returned map. Makes T027 pass.
- [X] T035 [US3] In `cmd/terraform/utils.go`: `buildTerraformExecData`'s signature is now
  `buildTerraformExecData(subCommand, output, component, stack string, exitCode int) any`;
  `"exit_code": exitCode` is in the returned map literal. Makes T028 pass.
- [X] T036 [US3] In `cmd/terraform/utils.go`: `buildTerraformExecData` no longer early-returns
  `nil` when `parseTerraformOutputMirror` fails for a *covered* subcommand (`plan`/`apply`,
  `deploy` parsed as `apply`) — it builds a defaulted map (`version`, `exit_code`,
  `component`/`stack` when non-empty, `resource_counts` all-zero, `outputs: map[string]any{}`,
  `changes`/`warnings`/`errors: []`, `has_changes`/`has_errors: false`) instead. Still returns
  `nil` for a subcommand outside `plan`/`apply`/`deploy` entirely (new
  `terraformCoveredSubcommand` helper). Makes T030 pass.
- [X] T037 **[RETRACTED — Decision 30r]** ~~Add `buildTerraformDeployExecData`~~ — not
  implemented; `deploy` reaches the same `buildTerraformExecData` call as `plan`/`apply`, no
  new function needed. This is where the false "two real subprocesses" premise was caught —
  reading `internal/exec/terraform.go`'s `handleDeploySubcommand` while designing this
  function's inputs (`planOutput`/`applyOutput`) surfaced that only one subprocess ever runs.
- [X] T038 [US3] `exit_code` is threaded to `buildTerraformExecData` via a closure-signature
  change, not a `terraformCaptureShellOpts` parameter (exit code isn't known when that closure
  is created — only later, where `internal/exec/terraform.go`'s `captureExecMetadataSync`
  already computes it). `WithExecMetadataParser`'s type
  (`internal/exec/shell_utils.go`) changes from `func(subCommand string) any` to
  `func(subCommand string, exitCode int) any`; `execMetadataParserFromOpts`,
  `execMetadataSyncParams.Parser`, and `terraformExecMetadataParserFunc`'s returned closure
  all updated to match. `captureExecMetadataSync`'s exit-code computation now uses
  `errUtils.GetExitCode(params.Err)` (previously a cruder `if params.Err != nil { exitCode =
  1 }`), consistent with `execNodeResult.ExitCode`'s existing derivation. Makes T032 pass.
- [X] T039 **[Superseded by T038]** No `plan.go`/`apply.go` `RunE` call-site change was
  needed — `terraformCaptureShellOpts(component, stack)`'s own signature is unchanged; the
  exit code flows in at closure-invocation time (`captureExecMetadataSync`), not at
  closure-creation time (`RunE`).
- [X] T040 **[RETRACTED — Decision 30r]** ~~Separate `deploy`'s captured stdout/stderr into
  plan-phase and apply-phase pairs~~ — not implemented; `deploy.go`'s capture is unchanged,
  since there is only one phase to capture.

**Checkpoint**: `TerraformExecData` now guarantees `[]` (never `null`) for empty list fields,
carries `exit_code` as the authoritative pass/fail/parse-completeness signal (per-component
for multi-component runs via the already-existing `execNodeResult.ExitCode`), and still
attaches a minimal payload when parsing fails entirely. `terraform deploy` picks up all of
this automatically via the shared code path — no `deploy`-specific shape or code exists
(Decision 30r).

---

## Phase 5: Polish & Cross-Cutting Concerns

- [X] T041 [P] Extended `pkg/pro/consumer_pact_test.go`'s terraform interactions (9 and 10)
  with an `exit_code` field per `contracts/interactions.md` (interaction 9: `exit_code: 2`,
  distinct from the envelope's own `exit_code: 0`, modeling a `-detailed-exitcode`-style
  "succeeded, changes present" signal; interaction 10: `exit_code: 0`). `errors: []` was
  already present in both fixtures (pre-existing), satisfying the null-vs-`[]` regression
  check. No new `deploy` interaction (interaction 16 retracted — Decision 30r). Regenerated
  via `rm pacts/atmos-AtmosPro.json && go test -tags pact ./pkg/pro/...`; `git diff
  pacts/atmos-AtmosPro.json` confirmed additive (`exit_code` field added to both terraform
  interactions, nothing else changed).
- [X] T042 [P] `go test -cover` run for `cmd/terraform` (all subpackages), `internal/exec`
  (targeted exec-metadata tests — the full-package run hits a pre-existing, confirmed
  environment flake unrelated to this change, see Phase 4 notes), and `pkg/pro` — all green,
  no new failures introduced.
- [X] T043 `atmos lint --changed` and `go build ./...` — clean (0 issues) after fixing 3
  `godot` (comment sentences starting with a lowercase identifier) and 1 `revive`
  `add-constant` (`"apply"` literal repeated ≥4 times — extracted
  `terraformSubCommandPlan`/`terraformSubCommandApply` constants) finding the linter surfaced.
- [X] T044 Quickstart step 21 corrected in place (see T046) to match Decision 30r — manual
  walk deferred to a real Pro-stub session (requires live/stubbed endpoint + CI env, not
  available in this implementation session); steps 18-20's underlying behavior is fully
  covered by the automated tests in T027-T030/T042.
- [X] T045 Docs check confirmed: `git diff --stat -- cmd/ internal/ pkg/` shows only
  `cmd/terraform/utils.go`, its test file, `internal/exec/shell_utils.go`/`terraform.go`,
  their test files, and `pkg/pro/consumer_pact_test.go` — no new CLI flag/command/config
  surface. No `website/docs/cli/commands/` changes required.
- [X] T046 [P] `quickstart.md` step 21 and its coverage-table row rewritten to drop the
  two-phase-shape framing (see quickstart.md's Decision 30r note); `contracts/
  interactions.md`'s interaction 16 already carries its own retraction note — consistent.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Empty.
- **Foundational (Phase 2)**: Empty.
- **US1/US2 (Phase 3)**: No remaining tasks.
- **US3 (Phase 4)**: Complete. T034 → T035 → T036 → T038 were sequenced in the same file(s)
  to avoid edit conflicts. T037/T039/T040 retracted mid-implementation once Decision 30's
  premise was found false (see Phase 4 notes) — no code delta was needed for them.
- **Polish (Phase 5)**: T041 depends on Phase 4 (done). T042/T043 depend on all prior phases.
  T044/T045/T046 have no code dependency and can run now.

### Parallel Opportunities

- T041-T042 (Polish) are `[P]` — independent concerns (Pact regeneration vs. coverage check).
- T044-T046 (Polish, docs/manual-walk) are `[P]` — independent doc-consistency tasks.

---

## Implementation Strategy

### MVP First / Incremental Delivery

Superseded by actual events — Phase 4 (Decisions 26-29) landed in one pass, and Decision 30's
two-phase `deploy` shape was retracted before any code was written for it (caught during
T037's design, before T040's implementation), so there was no separate "ship deploy's
two-phase shape later" increment to plan for. Remaining work is entirely Phase 5 (Polish):
Pact regeneration (T041), coverage/lint verification (T042-T043), and doc consistency
(T044-T046).
