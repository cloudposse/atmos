# Fix: Deterministic Starlark output assertions and final coverage validation

**Date:** 2026-10-04

## Summary

Make the Starlark output-channel regression test work with both plain and colored
terminal output, correct an invalid-flag test that bypassed flag parsing, and add
direct behavioral coverage for the extracted standard-library modules. The full
step package passed 20 race-enabled repetitions. The final scoped profile covers
86.30% of executable added lines against the PR's runtime base.

## Context

The race CI shard failed `TestStarlarkOutputUsesMaskedIOChannels` in its raw and
log cases. Its output contained ANSI sequences between `ui-channel:` and the
masked value, so the expected contiguous text was absent even though the secret
was masked correctly. This was an assertion failure, not a race-detector report.
Forcing color reproduced both failures locally.

The expanded command suite also exposed `TestDescribeConfigCmd_Error`, which
called `RunE` directly with an invalid flag. That bypassed Cobra's parser and had
passed because an inherited flag was uninitialized, producing an unrelated error.
The command catalog initializes inherited flags, exposing the incorrect test.
Calling `InheritedFlags` before the old test reproduced the failure in isolation.

The separate intermittent sleep/subprocess race report did not reproduce. The
first full 20-repetition run reached Go's default ten-minute timeout; increasing
the test timeout allowed all repetitions to complete.

## Changes

- Test raw, log, and none output with color explicitly enabled and disabled.
  Strip ANSI only when comparing displayed UI text. Retain checks that raw
  captures contain the original value, displayed streams never contain the
  secret, and data/UI messages stay on their respective channels.
- Test invalid command flags through `ParseFlags`, require the actual unknown-flag
  diagnostic, and isolate command state with `NewTestKit`.
- Exercise the Atmos module directly: literal argv construction, registered
  shorthand and inherited flag spelling, subcommands supplied through `args`,
  invocation policies, detailed-plan flag precedence, and invalid calls rejected
  before the host runner is invoked.
- Exercise logger levels, structured values, context overrides, default logging,
  and secret masking; exercise regex searches, Unicode matches, literal
  replacement, and invalid patterns/arguments through the exported modules.
- Execute standalone scripts through the command dispatch and real embedded
  interpreter in unit tests. Cover sibling imports from a different working
  directory, paths containing spaces, script-owned arguments resembling Atmos
  flags, print-only scripts, empty output, interpreter failures, and missing
  files both during detection and execution. Restore command and I/O state
  between cases through the existing command test helpers.

## Validation

- `go test -race -count=20 -timeout=30m ./pkg/runner/step` passed in 812.525 seconds,
  with no race reports or test failures. Passing repetitions do not prove that an
  intermittent race is impossible; the original report remains unconfirmed.
- The same command passed again against the final production tree after the
  registry refactor and field-test fixes, in 780.362 seconds, with no race reports
  or test failures.
- `go test -race -count=2 -run '^TestStarlarkOutputUsesMaskedIOChannels$' ./pkg/runner/step`
  passed after the explicit color reproduction failed before the assertion fix.
- The full step suite passed with the failing CI shard's shuffle seed:
  `go test -race -shuffle=1791120785209689222 -parallel=4 -timeout=5m ./pkg/runner/step`
  (40.184 seconds).
- The corrected invalid-flag test passed in isolation, then the full `cmd` package
  passed with coverage (125.693 seconds).
- Direct standard-library race tests passed with statement coverage of 99.3% for
  Atmos, 100% for logging, and 100% for regex. Shared conversion tests passed at
  100%. The standard-library lint run reported zero issues.
- All touched root-module packages passed with `-tags mage -timeout=55m` and
  package-local coverage: `cmd`, `cmd/internal`, `internal/exec`, `pkg/flags`,
  `pkg/hooks`, `pkg/runner/step`, `pkg/script/starlark/...`, `pkg/toolchain`,
  `pkg/workflow`, and `pkg/yaml/includescope`. Fresh profiles for `cmd`, Starlark,
  and include expansion cover their final edits; the passing `tools/noticegen`
  nested-module profile is included. The toolchain profile was refreshed after
  the Windows path fix (96.597 seconds, 82.7% package statement coverage).
- Intersecting covered source-line ranges with added lines against merge base
  `c6fc1bf85a1dfdca994e775b7da6ccb2fb7f6ed2` of
  `origin/osterman/starlark-runtime` gives **674 / 781 = 86.30%**.
  This excludes test files and the registry/standard-library bindings moved
  into the runtime PR. The earlier combined integration patch measured 90.08%;
  moving those highly covered bindings down changes this PR denominator. This is scoped to
  touched packages' own tests, not full-suite `-coverpkg=./...` breadth: it is a
  local approximation, and CI's full-suite Codecov upload remains authoritative.

- After incorporating the latest target branch and review fixes, the full touched
  package run passed again with `-tags mage -p 4 -count=1 -timeout=40m`. The refreshed
  profile still measures **674 / 781 = 86.30%** for this integration layer.
- The step, Starlark, process, logger, and toolchain packages passed another
  race-enabled run after the review fixes; the full toolchain package took
  504.281 seconds.
- Codecov's completed core report for `b4577133addcff5a374bc7103749286d2e2a72ea`
  against runtime `c6fc1bf85a1dfdca994e775b7da6ccb2fb7f6ed2` reports
  **764 / 910 = 83.95%**, below the required 85% despite a green check. Its
  comparison API identified 26 missed lines in `cmd/standalone_script.go`;
  existing CLI scenarios did not instrument the separately built binary.
- `go test -race -count=2 -run '^TestStandaloneScript' -coverprofile=../core-standalone-coverage.out ./cmd`
  passed in 4.375 seconds. The profile covers 100% of dispatch statements and
  94.1% of execution statements. Intersecting its covered ranges with the
  authoritative comparison finds 26 previously missed or partial lines covered,
  projecting **790 / 910 = 86.81%** if the other baseline coverage remains
  unchanged. This projection is not a replacement for the next CI upload.
- `../../custom-gcl run ./cmd/... --new-from-rev=HEAD` reported zero issues.
- `go test -count=1 -timeout=10m ./cmd` passed the full command package in
  122.567 seconds with the new standalone cases included.

## Follow-ups

None.
