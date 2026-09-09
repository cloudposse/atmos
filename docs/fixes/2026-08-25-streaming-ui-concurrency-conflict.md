# Fix: `--ui` now errors instead of silently corrupting output under `--max-concurrency > 1`

**Date:** 2026-08-25

## Summary

`atmos terraform <apply|plan|deploy|destroy|init> --all --ui` combined with an explicit
`--max-concurrency` greater than `1` had no guard: every concurrently-scheduled component
would independently launch its own full-screen streaming TUI session against the same real
terminal, with no coordination between them. This now fails fast with a clear
`ErrInvalidConfig` error before any component runs, matching the existing identity/
auto-approve validation already in place for concurrent runs.

## Context

The streaming UI dispatch (`internal/exec/terraform_streaming_ui.go`'s
`executeStreamingOrShell`) only ever checked whether streaming was requested (`--ui` /
`atmos.yaml`) and whether the environment could support it (TTY, not CI) — it had no
awareness of multi-component or concurrent execution. `tfui.ExecuteOptions` (the streaming
executor's input) has no output-writer redirection field, unlike the plain-output concurrent
path, which explicitly suppresses spinners (`suppressTerraformSpinners()`) and redirects
workdir output (`provWorkdir.WithOutputSuppressed`) specifically because concurrent runs
can't share a terminal cleanly. `validateTerraformConcurrentExecution` (the existing gate for
concurrent-specific constraints — non-interactive identity, `-auto-approve`) also didn't
check `--ui`. Found while answering a user question about `--ui --all` behavior; confirmed by
reading the dispatch/concurrency code paths, since the failure mode (garbled/corrupted
terminal from racing Bubbletea sessions) isn't something this headless environment can
reproduce interactively.

## Changes

- `pkg/terraform/ui/executor.go`: extracted `WouldAttemptStreamingUI(uiFlagSet, uiFlag,
  configEnabled bool) bool` — "requested and the environment could support it (TTY, not
  CI)", independent of which subcommand/phase is running. `ShouldUseStreamingUI` now
  composes this with the existing per-subcommand support check.
- `pkg/scheduler/adapters/terraform.go`: `validateTerraformConcurrentExecution` now also
  rejects the run when `tfui.WouldAttemptStreamingUI(...)` is true, via a new
  `validateTerraformUIConcurrency(wouldAttemptStreamingUI bool) error` — the boolean is
  computed by the caller and passed in, not derived internally, so the branch is testable
  without a real TTY (mirroring the existing documented TTY-testability limitation in
  `internal/exec/terraform_streaming_ui_test.go`).
- Docs: `website/docs/cli/configuration/components/terraform.mdx` and the `--ui` flag
  section of `terraform-apply.mdx`, `terraform-deploy.mdx`, `terraform-destroy.mdx`,
  `terraform-init.mdx`, `terraform-plan.mdx` now state that `--ui` errors (not silently
  falls back) when combined with `--max-concurrency > 1`.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/scheduler/adapters/... ./pkg/terraform/ui/... ./internal/exec/...` — all
  pass, including the new `TestValidateTerraformUIConcurrency` (both branches: streaming
  would/would not be attempted).
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
- `cd website && npm run build` — succeeds; pre-existing warnings/broken anchors are in
  files this change doesn't touch.
- Not validated: an actual live-terminal repro of the original corruption (this environment
  is headless/non-TTY, so the streaming UI path itself never activates here even without the
  fix — see `WouldAttemptStreamingUI`'s TTY gate and the precedent this fix's tests follow).

## Follow-ups

None.
