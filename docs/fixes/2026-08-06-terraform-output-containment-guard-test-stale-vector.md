# Fix: containment-guard regression test retargeted at the still-open traversal vector

**Date:** 2026-08-06

## Summary

`TestExtractComponentPath_ContainmentGuard` (`pkg/terraform/output/config_test.go`)
started failing on all three CI platforms (linux, windows, macos) after an
unrelated, earlier fix closed the specific traversal vector the test
exercised. No production code was broken — the test's chosen attack vector
just stopped being reachable, so the containment guard it was checking
correctly stopped firing. Retargeted the test at the vector that's still
open instead of the one that's now closed.

## Context

`docs/fixes/2026-08-05-workdir-nested-component-path-depth.md` made
`workdir.BuildPath` sanitize `/` out of the *component* name before
formatting the workdir directory name. That sanitization is a side effect
that also happens to neutralize `..`-based traversal via the component
argument: a component name like `../../../../evil` now collapses into the
single literal segment `..-..-..-..-evil` (no `/` left for the OS to
interpret as a parent-directory reference), so it can no longer escape
`BasePath` through `filepath.Join`.

`extractComponentPath`'s containment guard
(`pkg/terraform/output/config.go`) exists precisely to catch a derived
workdir path that escapes `BasePath` and fall back to `componentPath`
instead. `TestExtractComponentPath_ContainmentGuard` proved that guard by
injecting the traversal string via the *component* argument — the exact
vector the workdir fix closed. Once that fix landed, the guard's
`strings.HasPrefix(workdirPath, absBase+sep)` check no longer had anything
to reject for that input, so the test's own assertion (`path` must not
contain `.workdir`, proving the fallback fired) failed on every platform:
`config_test.go:406: ... should not contain ".workdir"`.

The guard itself was never broken. The *stack* argument is concatenated
into the workdir directory name the same way the component name is
(`fmt.Sprintf("%s-%s", stack, componentName)`), but `BuildPath` never
sanitizes it — so stack-name traversal remains a live vector the guard
still needs to catch, and the pre-fix test simply wasn't exercising it.

## Changes

- `pkg/terraform/output/config_test.go`: `TestExtractComponentPath_ContainmentGuard`
  now injects the traversal string via the `stack` argument instead of the
  `component` argument, with an updated comment explaining why the
  component vector is closed and the stack vector is what still needs
  coverage. No production code changed — `extractComponentPath`'s guard
  logic is untouched.

## Validation

- `go test ./pkg/terraform/output/... -run TestExtractComponentPath_ContainmentGuard -v`
  — both `TestExtractComponentPath_ContainmentGuard` and
  `TestExtractComponentPath_ContainmentGuard_AcceptsLegitimate` pass.
- `go build ./...` — clean.
- `go test ./pkg/terraform/output/...` (full package) — all pass.
- `atmos lint --changed` — 0 issues.
- Confirmed via the linux/windows/macos Acceptance Tests CI logs that this
  was the *only* failing test across all three platforms before this fix,
  and that no other package was affected.

## Follow-ups

None.
