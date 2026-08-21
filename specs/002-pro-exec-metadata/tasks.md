---

description: "Task list for the eleventh re-plan: FR-006e/f correctness fixes (plan-only `-detailed-exitcode` + local exit-code neutralization)"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload — Eleventh Re-Plan (exit_code / buffer-scoping)

**Input**: Design documents from `specs/002-pro-exec-metadata/` (plan.md eleventh re-plan, spec.md FR-006e/f, research.md Decisions 31-32/35-36, data-model.md provenance amendment)

**Prerequisites**: plan.md (eleventh re-plan), spec.md, research.md Decisions 31-32/35-36, data-model.md

**Tests**: Included — this repo's CLAUDE.md mandates a reproduce-first bug-fixing workflow (write a test that fails against current code, then fix), and each delta fixes a confirmed production bug.

**Organization**: All tasks serve **User Story 3** ("Structured infrastructure-change data for plan/apply/deploy", spec.md, Priority P3) — specifically its SC-007 accuracy guarantee. There is no US1/US2/US4 work in this re-plan; those stories' behavior is unaffected. Tasks are grouped by the two FR deltas within the US3 phase, since they are independent bug fixes that happen to share one user story. `pkg/ci/plugins/terraform` is not touched by this re-plan — a `-json`-stream parser rewrite (FR-006g/h) was scoped, then retracted before implementation (research.md Decisions 33-34); extraction stays regex-based, identical to what Native CI already does today.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[US3]**: All story-phase tasks belong to User Story 3
- File paths are exact, current locations confirmed against the tree at the time this list was generated — re-verify line numbers before editing, since surrounding code may have shifted slightly.

## Path Conventions

Single Go project at repository root: `internal/exec/`, `cmd/terraform/`, `pkg/pro/`.

---

## Phase 1: Setup

Not applicable — this re-plan extends already-shipped files only; no new project scaffolding, dependencies, or packages are introduced.

---

## Phase 2: Foundational

Not applicable — Groups A and B are independent bug fixes with no shared prerequisite; there is no blocking foundational work.

---

## Phase 3: User Story 3 — Structured infrastructure-change data accuracy (Priority: P3)

**Goal**: `TerraformExecData.exit_code` reflects the real terraform/tofu subprocess outcome (not silently `0`/uninformative), Atmos's own process exit code for `plan` is provably unaffected by that fix, and `resource_counts`/`outputs`/`changes` reflect only the actual plan/apply's own output (not poisoned by init/workspace-select noise) — satisfying SC-007's "100%" guarantee via the existing regex-based parser, correctly scoped.

**Independent Test**: Run `atmos terraform plan` against a component with pending changes, in a CI environment satisfying FR-001 but WITHOUT `ci.enabled: true` set in `atmos.yaml` (the common real-world case per research.md Decision 36), with Atmos Pro configured; confirm (a) the uploaded `TerraformExecData.exit_code` is the real non-zero pre-remap value, (b) Atmos's own process exit code for the `atmos terraform plan` invocation itself is unchanged (still 0 for a successful plan with changes, not 2), and (c) `resource_counts`/`outputs`/`changes` match the plan's actual output exactly, even when `terraform init` or `workspace select` emitted incidental "No changes."-shaped text earlier in the run.

### Group A — `exit_code` pre-remap capture (FR-006e, FR-006e/Decisions 31/35/36)

- [X] T001 [US3] Extend the `-detailed-exitcode` gate in `buildPlanSubcommandArgs` (`internal/exec/terraform_execute_helpers_args.go:38-40`) with an OR'd condition: add the flag whenever exec-metadata capture is active for this **`plan`** invocation (reuse the same `proexec.IsSyncCommand`-derived signal `captureExecMetadataSync` already checks, `internal/exec/terraform.go:210`), independent of `uploadStatusFlag`. Do **not** add `-detailed-exitcode` to `apply`/`deploy`'s internal `apply` invocation anywhere in this codebase (research.md Decision 35 — version-support risk on older pinned terraform/tofu binaries; `apply`/`deploy` keep plain 0/1 exit-code semantics)
- [X] T002 [P] [US3] Regression test in `internal/exec/terraform_execute_helpers_args_test.go`: `-detailed-exitcode` is present on `plan` when exec-metadata capture is active and `uploadStatusFlag` is `false`; still present (unchanged behavior) when `uploadStatusFlag` alone is `true` and exec-metadata capture is inactive; confirm the equivalent `apply`/`deploy` argument-building path never adds `-detailed-exitcode` under any combination of these flags
- [X] T003 [US3] Change `executeMainTerraformCommand` (`internal/exec/terraform_execute_helpers_exec.go:416-471`) to surface the pre-`mapCIExitCode` exit code to its caller — add a second return value or a field on an existing result type — without altering its existing `error`-return or CI-remap behavior (the `mapCIExitCode` call at line ~468-470 still determines what this function returns as `error`)
- [X] T004 [US3] Implement the local exit-2-neutralization guarantee (research.md Decision 36): at the `plan` call site in `executeMainTerraformCommand`/`executeCommandPipeline`, when `-detailed-exitcode` was added to this invocation specifically because T001's exec-metadata-capture trigger fired (not because `uploadStatusFlag` alone did, and not because `atmosConfig.CI.Enabled` already covers it via the existing `mapCIExitCode` path), and the resulting exit code is `2` ("changes detected"), remap it to a success-equivalent (`nil`/`0`) for **Atmos's own returned error/exit status only** — leave `atmosConfig.CI.Enabled` untouched (do NOT force-set it to `true`; do NOT widen `mapCIExitCode`'s own gate). This is a narrow, call-site-local remap, not a change to the global CI-mode switch or its other consumers (`pkg/ci.AnnotationsEnabled`/`ResultsEnabled`, container summaries, hooks CI-mode). Depends on T001, T003
- [X] T005 [US3] Regression test reproducing the exit-code-neutralization guarantee: with exec-metadata capture active (FR-001 gate true) and `ci.enabled` **NOT** set in the test's `atmos.yaml` fixture (the common real-world case, research.md Decision 36), a `plan` fixture with real pending changes (`-detailed-exitcode` → real exit 2) must still result in `atmos terraform plan`'s own process exit code being unchanged from today's baseline (0), while `TerraformExecData.exit_code` still reports the real `2` (per T003/T008). Also assert `atmosConfig.CI.Enabled` remains `false` after the invocation (guards against a regression that force-flips the global switch). Depends on T004
- [X] T006 [US3] Thread the pre-remap exit code from `executeMainTerraformCommand` through `executeCommandPipeline` (`internal/exec/terraform_execute_helpers_exec.go:164-220`) up to `ExecuteTerraform` (`internal/exec/terraform.go:92-204`) — depends on T004 (the local-neutralization change and the pre-remap-exit-code plumbing touch the same return path, so implement neutralization first, then thread the now-stable pre-remap value upward)
- [X] T007 [US3] Update `captureExecMetadataSync`'s call site in `ExecuteTerraform` (`internal/exec/terraform.go:200-204`) and the function itself (`terraform.go:243-281`) to pass the pre-remap exit code — not `errUtils.GetExitCode(params.Err)` (`terraform.go:254`), which reflects the post-remap/neutralized error — into `params.Parser(subCommand, exitCode)`. `ExecutionRecord.exit_code` (the base envelope, FR-003) is unaffected and continues to be sourced from the existing post-remap/neutralized `err` — depends on T006
- [X] T008 [US3] Regression test reproducing the original production bug in `internal/exec/terraform_test.go` (or a new adjacent test file): a fixture where `mapCIExitCode`/the T004 local neutralization together turn a real `ExitCodeError{Code: 2}` into a neutralized success for Atmos's own status, asserting `TerraformExecData.exit_code` still reports `2` while the base envelope's `exit_code` reports `0` — depends on T007

### Group B — buffer scoping to the main invocation only (FR-006f)

- [X] T009 [P] [US3] Move `terraformCaptureShellOpts` (`cmd/terraform/utils.go:855-864`) construction from once-per-pipeline to freshly invoked (fresh `bytes.Buffer`s) immediately before `executeMainTerraformCommand` inside `executeCommandPipeline` (`internal/exec/terraform_execute_helpers_exec.go:164-220`), so `terraform init`'s (line ~179) and `terraform workspace select`'s (line ~196, via `runWorkspaceSetup`) output are no longer tee'd into the same buffer the exec-metadata parser reads. Any other consumer of `terraform init`'s/workspace-select's captured output (e.g. logging) MUST be unaffected — only the exec-metadata parser's input buffer is rescoped
- [X] T010 [US3] Regression test reproducing the exact reported production bug: a fixture where the (now-separate) init-phase captured output alone contains a "No changes." lookalike string, but the main `plan` invocation's own captured output has a real `Plan: N to add, M to change, K to destroy.` summary — assert the parsed `resource_counts`/`has_changes` reflect the real summary, not an init-phase false match. Add to the nearest existing test file for `terraformExecMetadataParserFunc`/`terraformCaptureShellOpts` in `cmd/terraform/` — depends on T009

**Checkpoint**: User Story 3's `exit_code`/`resource_counts`/`outputs`/`changes` accuracy is fixed for both this feature and (transitively, since the underlying subprocess/buffer behavior is shared execution machinery) any other consumer of the same capture path — without touching `pkg/ci/plugins/terraform`, and without any observable change to Atmos's own process exit code or the global `ci.enabled` switch.

---

## Phase 4: Polish & Cross-Cutting Concerns

- [X] T011 Run `atmos test` and `atmos lint --changed` across all touched packages (`internal/exec`, `cmd/terraform`) — fix any findings before proceeding
- [X] T012 Grep `pkg/pro/consumer_pact_test.go` (and any other Pact fixtures) for a hardcoded `exit_code: 0` used as "expected" for a plan-with-changes scenario; update if found, otherwise explicitly note in the PR description that no Pact/wire-shape changes were needed (only extraction-source correctness and Atmos's own exit-code stability changed, not the JSON envelope Atmos Pro receives) — depends on T007
- [X] T013 [P] Re-read `specs/002-pro-exec-metadata/quickstart.md` and confirm no example values (if any reference `exit_code`/`resource_counts`) have gone stale as a result of these fixes; update only if an actual stale example is found (plan.md's Constitution Check already predicted no changes needed — verify that prediction held)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: N/A
- **Foundational (Phase 2)**: N/A
- **User Story 3 (Phase 3)**: Groups A and B have no dependency on each other and can run fully in parallel
- **Polish (Phase 4)**: Depends on both Group A and Group B being complete

### Within Group A (exit_code + neutralization)

T001 → T002 (test follows the gate change) · T003 → T004 → T005 (neutralization implemented, then verified) · T004 → T006 → T007 → T008 (strictly sequential — each step threads the value/behavior one layer further up the call stack, and T006's plumbing depends on T004's neutralization landing first so it threads a stable value)

### Within Group B (buffer scoping)

T009 → T010 (test follows the fix)

### Parallel Opportunities

- Group A (T001-T008) and Group B (T009-T010) can be implemented by two different people/sessions fully in parallel — different files, no shared state
- T002 is marked [P] — independent test file from the rest of Group A's sequential chain
- T009 is marked [P] relative to Group A — different files, no shared state
- T013 is marked [P] relative to T011/T012 — independent of the code changes, only checks documentation

---

## Parallel Example: Groups A and B (fully independent)

```bash
# Track 1 — exit_code fix + exit-code-neutralization guarantee (internal/exec/):
Task: "Extend -detailed-exitcode gate to plan only in terraform_execute_helpers_args.go (T001)"
Task: "Implement local exit-2-neutralization in executeMainTerraformCommand, independent of global ci.enabled (T004)"
Task: "Thread pre-remap exit code through executeCommandPipeline/captureExecMetadataSync (T006-T007)"

# Track 2 — buffer scoping fix (cmd/terraform/ + internal/exec/):
Task: "Relocate terraformCaptureShellOpts construction inside executeCommandPipeline (T009)"
```

---

## Implementation Strategy

### Recommended order

1. Start Groups A and B in parallel (two independent tracks, different files, no shared state)
2. Within Group A, land T001/T003 first (mechanical plumbing), then T004 (the neutralization guarantee — the highest-risk, most novel piece of this re-plan), verified immediately by T005, before threading the value further upward (T006-T008)
3. Each group closes one confirmed production bug on its own (exit_code reliability + exit-code stability; resource_counts/outputs accuracy) and can ship independently
4. Phase 4 (Polish) last, gating merge

### Incremental delivery

Each of Groups A and B closes a distinct, independently-verifiable bug (see plan.md's Decisions 31/35/36 and 32) and can ship as its own PR if preferred, though this repo's `pull-request` skill and CLAUDE.md conventions should be consulted for whether splitting is appropriate versus one bundled PR for this re-plan.

---

## Notes

- No new Pact/wire-shape changes are expected anywhere in this task list (T012 verifies this holds, doesn't introduce a change)
- The global `atmosConfig.CI.Enabled` switch (and everything else it gates — annotations, SARIF uploads, container summaries, hooks CI-mode) MUST remain untouched by this re-plan; T004/T005 exist specifically to guarantee and verify that
- Every new/changed public function needs `defer perf.Track(...)` per this repo's CLAUDE.md, if any new exported function is introduced during implementation (unlikely for these two targeted fixes, but apply the convention if one appears)
- Follow this repo's mandatory bug-fixing workflow for every task in Groups A/B (T002/T005/T008/T010): write the failing regression test first, confirm it fails against current code, then implement the fix
- Avoid platform-specific test fixtures (no hardcoded Unix paths, no shell-outs to `terraform`) — all new tests use inline fixtures per this repo's cross-platform testing conventions
