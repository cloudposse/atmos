# Fix: `--use-version` rejects release-candidate semver strings

**Date:** 2026-07-31

## Summary

`atmos --use-version=1.225.0-rc.3` failed with `invalid version output format` even though
`1.225.0-rc.3` is a spec-compliant semver string. `isValidSemver()` in
`pkg/toolchain/version_spec.go` now delegates to the already-vendored
`github.com/Masterminds/semver/v3` library instead of a hand-rolled digit-only check, so
pre-release (`-rc.3`) and build-metadata (`+build.5`) suffixes are accepted.

## Context

[Issue #2839](https://github.com/cloudposse/atmos/issues/2839): `isValidSemver()` split the
version string on `.` and required every part to be pure digits. `1.225.0-rc.3` splits into
`["1", "225", "0-rc", "3"]`, and `"0-rc"` fails the digit check, so `ParseVersionSpec` fell
through to its final "invalid format" branch (it also fails the SHA fallback check, since `.`
and `-` aren't hex characters).

Tracing the downstream flow (`pkg/version/reexec.go` → `pkg/toolchain/install.go` →
`RunInstall`/`InstallSingleTool` → the aqua-style asset installer) confirmed the parser was the
only place the bug lived: the prerelease-exclusion logic in `pkg/toolchain/set.go` only applies
when resolving the `"latest"` keyword, never to an explicitly pinned version, and the installer
already has a v-prefix 404-fallback for GitHub release tags. So no installer/download changes
were needed.

`github.com/Masterminds/semver/v3` is already a direct dependency and already used for semver
parsing elsewhere in the same package (`pkg/toolchain/get.go`, `pkg/toolchain/list.go`), so this
fix reuses it rather than hand-rolling prerelease/build-metadata grammar — which is genuinely
tricky to get right with a naive dot-split, since the prerelease field itself can contain dots
(e.g. `rc.3` in `1.225.0-rc.3`).

## Changes

- `pkg/toolchain/version_spec.go`: `isValidSemver()` now requires at least one `.` (so bare
  numbers/`v1` are still left to the other classifiers) and then delegates the rest of the
  validation to `semver.NewVersion()`.
- `pkg/toolchain/version_spec_test.go`: added cases for `1.225.0-rc.3`, `v1.225.0-rc.3`,
  `1.2.3-alpha.1`, `1.2.3+build.5`, `1.2.3-rc.1+build.5`, and invalid pre-release forms
  (`1.2.3-01`, `1.2.3-`). Corrected two pre-existing cases whose expectations were artifacts of
  the old naive splitter rather than real semver rules: `1.2-beta` (`false` → `true`, this was
  the exact bug class) and `1.2.3.4.5` (`true` → `false`, not valid semver beyond
  major.minor.patch). Added matching `ParseVersionSpec` cases for the RC input.
- `pkg/version/reexec_test.go`: added
  `TestFindOrInstallVersionWithConfig_ReleaseCandidate`, a resolver-level regression test for
  the cached-binary path used by `--use-version`.

## Validation

- `go build ./...` — passes.
- `go test ./pkg/toolchain/... ./pkg/version/...` — passes, including all new/updated cases.
- `atmos fix lint` (patch-scoped `custom-gcl` run against `origin/main`) — 0 issues.
- Not run: a live network install of an actual GitHub release-candidate tag (no RC currently
  published to install against in this environment); the fix is scoped to the format-validation
  layer, and the downstream install/download path was verified by code reading, not a live
  network call.

## Follow-ups

None.
