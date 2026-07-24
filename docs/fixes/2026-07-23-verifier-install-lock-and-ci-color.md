# Fix: verifier install/execution race (`ETXTBSY`) and missing CI color detection

**Date:** 2026-07-23

## Summary

Bootstrap verifier installs (e.g. `cosign`) now hold their per-version lock through trust repair and
execution, closing a race where a concurrent installer could replace the binary while another process
was executing it. That subprocess execution is now also time-bounded when the caller supplies no deadline,
so a hung verifier can no longer hold the shared install lock forever. Structured error output also now
renders with ANSI color under the standard `CI` environment signal, matching the rest of Atmos's
color-detection behavior. That color change made several existing tests flaky in CI: they asserted literal
substrings against `Format()` output that now spans styled-text boundaries, so those assertions are fixed
to strip ANSI first.

## Context

`verifierCommandRunner.runBootstrapVerifier` called `Installer.Install`, which released its per-version
file lock as soon as the install/extract/replace transaction completed, and only afterward invoked
`runTrustedVerifier` to trust-repair and execute the binary. A second worker installing the same tool
version could reinstall (atomically replace) the binary in the window between the first worker's install
returning and its exec starting. Linux rejects executing a binary that is mid-replace with `ETXTBSY`
("text file busy"), which broke a real CI run:
https://github.com/cloudposse-examples/atmos-native-ci/actions/runs/30062334123/job/89386299463.

Holding that lock through execution also introduced a new risk CodeRabbit flagged on PR #2794: the context
feeding `runBootstrapVerifier` (from `verifyDownloadedAsset`'s `context.Background()`) carries no deadline,
so a hung verifier subprocess would hold the version lock indefinitely and stall every other install or
verification of that tool version, not just its own run.

Separately, `shouldUseColor()` in `errors/formatter.go` built its `lipgloss.EnvColorProfileParams` from
`NO_COLOR`, `CLICOLOR`, `CLICOLOR_FORCE`, and `FORCE_COLOR`, but never read `CI`. Structured error output
therefore lost ANSI styling in CI even though `CI` is Atmos's standard color-capable signal elsewhere.

Enabling color under `CI=true` had a fallout the initial fix didn't anticipate: several existing tests call
`errUtils.Format()` (or, inside the `errors` package itself, the unexported `Format()`) and then assert a
literal, multi-word substring against the result — e.g. a hint like `` `gh auth status` `` or a heading like
`## Example`. Once CI renders ANSI color, the renderer's word-wrapping and markdown styling can insert an
escape sequence between two plain-text runs that used to be contiguous, so a substring that used to match
now straddles a styled-text boundary and the `Contains`/`Index` check fails. This surfaced as CI failures on
PR #2794 in `internal/exec` (`TestBuildWorkflowStepError`), `pkg/github`
(`TestHandleGitHubAPIError_RateLimitHintBranches`), and `pkg/version`
(`TestFindOrInstallVersionWithConfig_InvalidVersionFormat`) — and, on a broader local `CI=true` sweep of the
whole repo, in `cmd` (`TestShowArgCountErrorAndExit_MessageContent`) and four tests inside `errors` itself
(`TestFormat_SectionOrder`, `TestFormat_ExampleAndHintsSeparation`, `TestFormat_ContextMarkdownTable`,
`TestFormat_VerboseStackTrace`) that would otherwise have failed the next time CI ran them.

Fixed on this branch for PR [#2794](https://github.com/cloudposse/atmos/pull/2794).

## Changes

- Added `Installer.installWithVersionLock(ctx, owner, repo, version, afterInstall)`, which runs the
  existing install/extract/replace transaction and, only while still holding the per-version file lock,
  invokes an optional `afterInstall(binaryPath)` callback. `Installer.Install` now delegates to it with a
  `nil` callback, so its public behavior is unchanged (`pkg/toolchain/installer/installer.go`).
- `runBootstrapVerifier` now calls `installWithVersionLock` directly and runs trust repair + verifier
  execution from inside the `afterInstall` callback, so the whole install → trust → exec lifecycle for one
  verifier version happens under a single held lock (`pkg/toolchain/installer/installer.go`).
- `runTrustedVerifier` no longer acquires its own separate `binaryPath+".run.lock"` — the caller already
  holds the version lock for the full lifecycle — and now just performs trust repair (at most once per
  binary, via the existing `.trusted` marker) and runs the verifier
  (`pkg/toolchain/installer/installer.go`).
- Added `boundedVerifierContext`, which derives a `verifierSubprocessTimeout` (5 minute) bounded context
  only when the incoming context has no deadline. `runBootstrapVerifier` applies it only around the
  `runVerifierCommand` subprocess call, leaving `installWithVersionLock` and trust repair on the caller's
  original context (`pkg/toolchain/installer/installer.go`).
- `shouldUseColor()` now includes `EnvCI: os.Getenv("CI") != ""` in the `lipgloss.EnvColorProfileParams`
  passed to lipgloss's environment-based color detection, so `CI=true` enables ANSI color in structured
  error output while `NO_COLOR`/`CLICOLOR_FORCE` continue to take precedence (`errors/formatter.go`).
- Fixed the CI-color fallout: five test files now strip ANSI from formatted output before asserting literal
  substrings, using the existing `pkg/ansi.Strip()` helper (aliased `atmosansi`, the same convention already
  used elsewhere, e.g. `internal/exec/terraform_test.go`) or, inside the `errors` package itself, the
  package-local `stripANSI()` — `cmd/cmd_utils_test.go`, `internal/exec/workflow_utils_test.go`,
  `pkg/github/client_test.go`, `pkg/version/reexec_test.go`, `errors/formatter_test.go`.

## Validation

- `go build ./...`
- `go test ./errors/... ./pkg/toolchain/installer/... -race` and, separately, `CI=true go test
  ./internal/exec/... ./pkg/github/... ./pkg/version/... ./cmd/... ./errors/...` to reproduce the CI
  environment locally and confirm every test named above now passes under `CI=true`.
- `CI=true go test $(go list ./... | grep -v '^github.com/cloudposse/atmos/tests$')` — a full-repo sweep
  under `CI=true` (378 packages) to catch any other tests with the same latent fallout; it passed clean
  (exit 0, zero `FAIL` lines). `github.com/cloudposse/atmos/tests` was excluded from this sweep only because
  one of its acceptance tests (`quick-start-advanced terraform plan --all dry-run fails before scheduling
  when its emulator is unavailable`) hangs in this sandbox with no Docker daemon available; that test and
  its Docker dependency predate this branch and are unrelated to this fix.
- `atmos lint --changed`
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh docs/fixes/2026-07-23-verifier-install-lock-and-ci-color.md`

## Follow-ups

None.
