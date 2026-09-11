# Implementation Plan: Full Component Name in Atmos Pro Uploads

**Branch**: `1199-fix-upload-component-name` | **Date**: 2026-09-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-fix-upload-component-name/spec.md`

**References**: GitHub issue [cloudposse/atmos#3102](https://github.com/cloudposse/atmos/issues/3102) (root-cause analysis corrected by code review — see research.md Decision 1)

## Summary

For components whose logical name contains slashes (e.g. `foo/bar/baz`), two Atmos Pro upload payloads report the truncated leaf (`baz`) instead of the full logical name, because they read `info.Component` (intentionally truncated for working-directory resolution) instead of `info.ComponentFromArg` (always the full logical name):

1. **Instance-status upload** (`--upload-status`): `uploadStatus` in `internal/exec/pro.go` (~line 254) builds `InstanceStatusUploadRequest{Component: info.Component}`. Affects **all** invocation shapes. This is the site the issue's single-component repro actually hits.
2. **Multi-component aggregate execution record**: `terraformNodeHooks.recordExecResult` in `cmd/terraform/utils.go` (~line 581) passes `info.Component` into `buildTerraformExecData`. Affects only `--affected`/`--all`/`--components`/`--query` runs.

The fix substitutes `info.ComponentFromArg` at both sites — nothing else changes. The name-splitting logic in `internal/exec/utils.go` (`ProcessStacks`, ~lines 1106–1116) and all working-directory consumers of `Component`/`ComponentFolderPrefix`/`FinalComponent` are untouched. Bug-first workflow: failing tests for both sites land before the fix.

## Technical Context

**Language/Version**: Go 1.26 (per `go.mod`; CI pins via `go-version-file`)

**Primary Dependencies**: existing only — `spf13/cobra`, `go.uber.org/mock` (mocks for `pro.AtmosProAPIClientInterface` already generated), `pact-go` (consumer contract tests). No new dependencies.

**Storage**: N/A (client-side payload construction; persistence is Atmos Pro server-side and out of scope)

**Testing**: `go test` via `atmos test` / `atmos test --full`; table-driven unit tests with existing mocks; pact consumer tests in `pkg/pro/consumer_pact_test.go`

**Target Platform**: Linux/macOS/Windows (no platform-sensitive code touched)

**Project Type**: CLI (existing Atmos codebase; bug fix within `internal/exec` and `cmd/terraform`)

**Performance Goals**: N/A — no hot-path change; two field substitutions in upload construction

**Constraints**: MUST NOT alter working-directory resolution semantics (`info.Component` / `ComponentFolderPrefix` / `FinalComponent` keep current values everywhere); MUST NOT change payload shape, only the value of the existing `component` field

**Scale/Scope**: 2 production lines changed across 2 files + tests; no schema, doc, flag, or command changes

## Constitution Check

*GATE: evaluated against Atmos Constitution v1.0.0.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | PASS (N/A) | No new commands or providers; existing call sites only. |
| II. Interface-Driven Design / DI | PASS | Both sites already testable: `uploadStatus` takes `pro.AtmosProAPIClientInterface` + `git.GitRepoInterface` (generated mocks exist); `recordExecResult` is a pure accumulator invoked directly in existing tests. No new interfaces needed (Principle V forbids adding any). |
| III. Test-First, 80% coverage (NON-NEGOTIABLE) | PASS (planned) | Failing tests reproducing both truncations land before the fix (quickstart.md gives the exact commands and expected failures). New tests assert full-name identity plus flat-name control cases. |
| IV. Separated I/O and UI | PASS (N/A) | No output-channel changes. |
| V. Simplicity / YAGNI | PASS | Minimal possible diff: two identifier substitutions. No abstraction introduced. |
| Dev standards (errors, flags, perf.Track, imports) | PASS (N/A) | No new errors, flags, or public functions. |
| Docs gate | PASS (N/A) | No new commands/flags → no Docusaurus changes. Release label `patch` → no blog post or roadmap entry required (per `pull-request` skill decision tree). |

**Post-Phase-1 re-check**: design adds no projects, interfaces, or abstractions — all gates still PASS. Complexity Tracking not required (no violations).

## Project Structure

### Documentation (this feature)

```text
specs/003-fix-upload-component-name/
├── spec.md              # Feature specification (complete, clarified)
├── plan.md              # This file
├── research.md          # Phase 0: consolidated code-review decisions
├── data-model.md        # Phase 1: identity fields & payload entities
├── quickstart.md        # Phase 1: repro, test-first commands, verification
├── contracts/
│   └── upload-component-identity.md  # Phase 1: component-identity contract per upload channel
├── checklists/
│   └── requirements.md  # Spec quality checklist (all pass)
└── tasks.md             # Phase 2 (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
internal/exec/
├── pro.go                          # FIX SITE 1: uploadStatus → info.ComponentFromArg (~line 254)
├── pro_test.go                     # NEW/EXTENDED: nested-name + flat-name uploadStatus cases
└── utils.go                        # READ-ONLY: split logic (~1106–1116) — MUST NOT change

cmd/terraform/
├── utils.go                        # FIX SITE 2: recordExecResult → info.ComponentFromArg (~line 581)
└── utils_exec_metadata_test.go     # EXTENDED: nested-name recordExecResult case; existing
                                    # fixtures already set Component == ComponentFromArg and keep passing

pkg/scheduler/adapters/terraform.go # READ-ONLY: verified nodeInfo.ComponentFromArg = node.Component
                                    # (full logical name) at ~lines 681–682 — guarantees the fix's input

pkg/pro/consumer_pact_test.go       # VERIFY-ONLY: fixtures use flat names ("vpc"); no changes expected
```

**Structure Decision**: Existing Atmos layout; this is a two-file surgical fix with co-located tests. No new packages (Constitution V) — purpose-built packages are for new functionality, which this is not.

## Phase 0: Research

All unknowns were resolved by direct code inspection during the review session; `research.md` consolidates those findings as numbered decisions (call-site inventory, field semantics, multi-node identity guarantee, deferral of the path-argument quirk, release labeling). No open questions remain.

## Phase 1: Design

- `data-model.md` — the three identity fields of `ConfigAndStacksInfo` (`ComponentFromArg`, `Component`, `ComponentFolderPrefix`) with their producers/consumers, and the two payload entities (`InstanceStatusUploadRequest`, TerraformExecData component entry) with the corrected `component` semantics.
- `contracts/upload-component-identity.md` — the cross-channel contract: every Atmos Pro upload channel identifies a component by its full logical stack-manifest key; per-channel current vs. required values.
- `quickstart.md` — bug-first workflow: exact failing tests to write, fix diff, verification commands (`atmos test`, targeted `go test` runs, pact suite), and the FR-008 follow-up-issue step.

## Release & PR Requirements (repo policy)

- **Label**: `patch` (bug fix, no new surface). No blog post, no roadmap update required.
- **FR-008 gate**: before merge, open a follow-up GitHub issue for the path-style-argument quirk (raw path reported as component identity in single-component execution records, `cmd/terraform/plan.go` ~91–95 capture-before-resolution ordering) and link it by number in the PR description.
- **Issue linkage**: PR closes cloudposse/atmos#3102; PR description should note the corrected root-cause analysis (two sites, not one) so the issue history stays accurate.

## Complexity Tracking

No constitution violations — table intentionally omitted.
