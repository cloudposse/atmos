# Fix: Terraform streaming UI (`--ui`) — Ctrl-C hang, duplicate-plan-then-fail, silent no-ops, and dead render config

**Date:** 2026-08-25

## Summary

A `/field-test` DX pass of PR #1908's new terraform streaming UI (`--ui`) found five confirmed
defects, fixed here in five phases: (1) `atmos` could hang indefinitely after Ctrl-C during a
streaming apply/init, (2) redirected stdin caused a full plan to run twice before failing, (3)
`destroy --ui` and `refresh --ui` silently fell back to plain output with no explanation, (4)
`components.terraform.ui.{compact,show_attribute_bar,max_lines}` were parsed and documented but
never actually read at render time, and (5) several docs described behavior that didn't match
the code (now corrected, since Phases 1-4 changed what "matches" means for two of them).

## Context

Found via a `/field-test` pass on branch `osterman/terraform-streaming-ui` (PR #1908), using a
new disposable local-only fixture (`tests/fixtures/scenarios/streaming-ui-manual/` -
`null_resource`/`local_file`/`time_sleep`, no cloud credentials) built for hands-on repro and
kept afterward for regression use. Each phase below was root-caused by reading the actual
implementation and live-verified before/after against a real terraform binary in a real pty -
not just unit-tested in isolation, since the field test's whole premise was that the streaming
UI's real-subprocess/real-TTY paths sat at 0% coverage in the original PR.

## Changes

### Phase 1 - Ctrl-C hangs `atmos` indefinitely

`Model.Update()`'s `tea.KeyMsg` handler (Ctrl-C/`q`) and its `doneMsg` handler (terraform
finishing on its own) both just set `done = true` and returned `tea.Quit` - indistinguishable to
the caller. `runTUIProgram()` only killed `cmd.Process` when `p.Run()` returned an error; a clean
Ctrl-C quit returns `nil`, so `Execute()`/`ExecuteInit()` fell into an unconditional, blocking
`cmd.Wait()` on a subprocess nobody told to stop. A live repro (pty apply against a 20s
`time_sleep`, Ctrl-C at 4s) hung over 10 hours before being force-killed, with no terraform
subprocess left alive and no state written - ruling out "finishes in the background" as the
explanation.

- `pkg/terraform/ui/model.go`, `init_model.go`: added `cancelled bool` + `Cancelled() bool` to
  both `Model` and `InitModel`, set only by the quit-key path.
- `pkg/terraform/ui/executor.go`: `runTUIProgram()` now kills `cmd.Process` when the final model
  reports `Cancelled()` (via a `cancellableModel` interface and two extracted, independently
  testable helpers `modelWasCancelled`/`killIfCancelled`). `Execute()`/`ExecuteInit()` return
  `errUtils.ExitCodeError{Code: cancelledExitCode}` (130) instead of blocking further. Extracted
  `finalizeExecuteInitResult` to keep `ExecuteInit`'s cyclomatic complexity under the lint limit.
- `pkg/terraform/ui/model_render.go`, `init_model.go`: both final views now show a "cancelled"
  message instead of falsely flashing "completed" on early quit.
- `pkg/ui/formatter.go`: added `FormatWarning`/`FormatWarningf` (mirrors `FormatSuccess`/
  `FormatError`), used by the above.
- `pkg/terraform/ui/testmain_test.go` (new): `TestMain` supporting `_ATMOS_TEST_SLEEP_SECONDS` so
  the test binary itself can stand in for a long-running subprocess (cross-platform, per
  CLAUDE.md - no reliance on a platform-specific `sleep`).

### Phase 2 - stdin/stdout TTY split duplicates a full plan before failing

`ShouldUseStreamingUI` only checked stdout TTY; `ConfirmApply`/`ConfirmDestroy` also require
stdin TTY. With stdout attached to a TTY but stdin redirected, the full streaming plan phase ran
(real work), confirmation then failed, and the caller reran the *entire* plan again via the plain
fallback - live-verified: real duplicate `terraform plan` output followed by a hard failure once
terraform's own prompt also hit stdin EOF.

- `pkg/terraform/ui/executor.go`: added `checkConfirmationPreconditions()` (checks stdin TTY),
  called at the top of `ExecuteApply`/`ExecuteDestroy` (after the `-auto-approve` bypass) so a
  non-interactive stdin fails fast, before the plan phase runs at all, instead of after it. Live
  re-verified: now only one plan runs (the plain fallback), not two.

### Phase 3 - `destroy --ui` and `refresh --ui` silently no-op

- **`destroy --ui`**: `cmd/terraform/destroy.go` called `ParseTerraformRunOptions(v)` without the
  variadic `cmd` parameter needed for `--ui` tri-state detection (`cmd.Flags().Changed("ui")`),
  so `UIFlagSet` never became `true` even when the user passed `--ui` - the exact bug class the
  PR's own `TestWorkspacePassthroughLeafPropagatesUIFlag` regression test caught for
  `workspace.go`, just missed on `destroy.go`. Fixed the one-line call site. This then exposed a
  second, previously-unreachable bug: `buildArgsWithJSON`'s `isJSONSubCommand` whitelist omitted
  `"destroy"`, so `-json`/`-compact-warnings` were prepended *before* `destroy` instead of after
  it - confirmed live that terraform rejects this outright (`Invalid flags before the
  subcommand`), and because that error text isn't JSON, the parser silently dropped it, so the
  TUI showed a false "✓ Destroy completed" immediately followed by an unrelated "exited with code
  1". Fixed by adding `destroy` to `isJSONSubCommand`. Verified `terraform destroy -json
  -compact-warnings ...` (flags in the right place) streams and destroys correctly - `destroy`
  fully supports the streaming UI, this was purely a flag-plumbing and whitelist bug, not a
  Terraform limitation.
- **`refresh --ui`**: genuinely unsupported (Terraform's `refresh` has no `-json` streaming
  output), and the PRD/blog already documented it as such - but the CLI silently fell back with
  no explanation, and two other doc pages (`terraform-refresh.mdx`, `terraform.mdx`) incorrectly
  claimed it worked. Rather than building a whole second non-JSON parser for `refresh` (the
  bigger lift), added `UIRequestedButUnsupported()` and a one-line `ui.Warning(...)` in
  `executeStreamingOrShell` so the user sees *why* it fell back, and corrected the two doc pages
  to match the PRD's original (correct) claim.
- `cmd/terraform/subcommands_test.go`: added `TestDestroyCommandPropagatesUIFlag` (mirrors the
  workspace regression test). `pkg/terraform/ui/executor_test.go`: added
  `TestBuildArgsWithJSON_AddsFlagForDestroy`, `TestUIRequestedButUnsupported`,
  `TestExecuteApply_FailsFastWhenStdinNotTTY`, `TestExecuteDestroy_FailsFastWhenStdinNotTTY`
  (Phase 2's fix).

### Phase 4 - dead render config

`components.terraform.ui.{compact,show_attribute_bar,max_lines}` were fully wired into
`pkg/schema/schema.go` and the JSON Schema, but every `RenderTree()` call site hardcoded
`RenderTreeWithConfig(nil)` - the atmos.yaml settings were parsed and documented but never read.
The doc also claimed `compact` defaults to `true`, but since it was never wired, the real
(zero-value) default was `false`.

- `pkg/terraform/ui/tree_render.go`: added `BuildRenderConfig(schema.TerraformUI) *RenderConfig`,
  applying the documented defaults (`compact=true`, `show_attribute_bar=false`) when the
  `*bool` config fields are unset.
- `pkg/terraform/ui/executor.go`: added `RenderConfig *RenderConfig` to `ExecuteOptions`; the
  three `RenderTree()` call sites now call `RenderTreeWithConfig(opts.RenderConfig)`.
- `internal/exec/terraform_streaming_ui.go`: populates `execOpts.RenderConfig` via
  `tfui.BuildRenderConfig(...)` - a one-line call site, keeping the actual translation logic in
  `pkg/terraform/ui` per this repo's package-organization convention rather than adding new
  logic to `internal/exec`.
- Live-verified both directions: default (unset) now renders compact (no blank lines, matching
  the doc); explicit `ui.compact: false` genuinely restores blank lines between resources.
- `pkg/terraform/ui/tree_test.go`: added `TestBuildRenderConfig_DefaultsWhenUnset`,
  `TestBuildRenderConfig_HonorsExplicitValues`.

### Phase 5 - doc/skill corrections

- `website/docs/cli/commands/terraform/terraform-refresh.mdx`: replaced the false `--ui` flag
  section with a note explaining `refresh` doesn't support it and why.
- `website/docs/cli/configuration/components/terraform.mdx`: corrected the supported-command
  list (removed `refresh`, added `deploy`/`destroy`) and the completion-message example (was
  showing a fictional `+3 ~1 -0` tally and lowercase `apply`; actual rendered output is
  `✓ Apply plat-ue2-dev/vpc completed (15.2s)` - capitalized, no tally - matching the blog post's
  already-correct example).
- `website/docs/cli/commands/terraform/terraform-destroy.mdx`: added the `--ui` flag section
  (correctly omitted in the original PR since destroy's streaming path didn't actually work yet;
  now it does, per Phase 3).
- `pkg/ui/theme/icons.go`: removed `IconRefresh` - defined but never referenced anywhere; replace
  actions already render via a colored dot (`colorizedActionSymbol`'s `replaceStyle`), not this
  glyph.
- `agent-skills/skills/atmos-terraform/SKILL.md`: added a "Streaming UI (`--ui`)" section - previously
  no skill anywhere documented this feature.

## Validation

- `go build ./...` - clean.
- `gofumpt -l` on all changed files - clean.
- `atmos lint --changed` (patch-scoped `custom-gcl` against `origin/main`) - 0 issues, after
  fixing complexity/repeated-literal/nolintlint/godot findings surfaced along the way.
- `go test ./pkg/terraform/ui/... ./pkg/ui/... ./internal/exec/... ./cmd/terraform/...` - all
  pass, including every test named above.
- Manual live verification for every phase, using the `streaming-ui-manual` fixture and a real
  `terraform` binary in a real pty (not just captured/piped output, per the field-test skill's
  "don't rely solely on mocked/dry-run paths" guidance):
  - Phase 1: 5/5 runs hung indefinitely before the fix (one confirmed alive 10+ hours); 5/5
    completed in 4-6s after.
  - Phase 2: before, a duplicate full plan ran then failed; after, only the single plain-fallback
    plan runs.
  - Phase 3: `destroy --ui` now streams a full tree/progress UI and genuinely destroys resources
    (confirmed via `terraform show` before/after); `refresh --ui` now prints an explicit warning
    and falls back cleanly; `refresh` without `--ui` and `plan --ui` (a supported command) both
    stay silent (no false-positive warnings).
  - Phase 4: default rendering is now compact (verified no blank lines); `ui.compact: false`
    genuinely restores blank lines.
- Not run: `atmos test --full` and the website build (`cd website && npm run build` failed on a
  pre-existing, unrelated environment issue - `@resvg/resvg-js` missing from `node_modules`,
  reproducible on a clean checkout before any of these changes; not caused by or related to the
  `.mdx` edits here).

## Follow-ups

None required for the confirmed defects - all five phases are fixed, tested, and live-verified.
Two items are worth flagging for your decision rather than being blocking issues:

- `tests/fixtures/scenarios/streaming-ui-manual/` is untracked and has lasting regression value;
  left uncommitted pending your call on whether to add it to the PR.
- Full JSON-streaming support for `refresh` (building a second, non-JSON-based progress model
  like `InitModel`'s) was considered and deliberately deferred in favor of the explicit-warning
  fix in Phase 3, since Terraform's `refresh` has no `-json` output to stream from - this would
  be new-feature work, not a bug fix, if ever pursued.
