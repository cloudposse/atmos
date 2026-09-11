# Fix: Windows GC/allocator crash from concurrent subprocess launches in acceptance-test orchestration

**Date:** 2026-09-11

## Summary

`internal/ci/acceptance`'s own acceptance-test suite runs ~90 `t.Parallel()` subtests that each shell out to
`go build`/`go test -c`/`go test`/precompiled `*.test.exe` binaries. On a wide Windows CI runner, letting that
many real `go` toolchain subprocesses launch fully concurrently triggered a Windows-only Go runtime
GC/allocator crash. Fixed by capping concurrent subprocess launches to 4 on Windows, then moving that
Windows-only logic into a build-tagged file (`//go:build windows` / `//go:build !windows`) to match this
codebase's established platform-split convention instead of a runtime `GOOS` check in shared code.

## Context

This package's acceptance tests each independently allocate and syscall heavily when they shell out. On a
wide/many-core CI runner with no cap, that produced a runtime-fatal GC/allocator crash ("fatal error: found
pointer to free object" / "marked free object in span") in `mcache`/`mgcsweep` during a concurrent `os/exec`
process launch — a long-standing, still-recurring class of Go runtime race under heavy concurrent
allocation+syscall pressure on Windows (golang/go#44900, #45364, #47415, #54247; every report is
Windows-only), not anything specific to the command being run. Seen bouncing PR #3115 from the merge queue
twice for unrelated reasons; this specific crash was on the first attempt (windows, shard 3/10, run
34558935380).

The first fix (commit `66e50d45f5`) added the cap for every platform. A quick follow-up (commit
`72319350ae`) scoped it to Windows only, since the race has never been reported on Linux/macOS and capping
them too just serializes work for no benefit — confirmed locally: this package's test suite dropped from
~15s to ~11.7s on macOS once the cap was scoped out. That follow-up implemented the scoping as an inline
`if runtime.GOOS != "windows" { return no-op }` check inside `command.go`, which left Windows-only semaphore
machinery (a const, a package var, and most of a function body) compiled into every platform's build, gated
only by a runtime branch. This codebase has an established convention for exactly this shape of
platform-specific code — grepped and found 13+ existing `*_windows.go` / `*_other.go` (or `*_unix.go`)
build-tag file pairs (e.g. `pkg/terraform/cache/trust_install_windows.go` +
`pkg/terraform/cache/trust_install_other.go`), each splitting the platform-specific implementation into its
own file with `//go:build windows` / `//go:build !windows` instead of a runtime check. This fix brings
`internal/ci/acceptance` in line with that convention.

## Changes

- `internal/ci/acceptance/command.go`: removed the `maxConcurrentSubprocesses` const, the `subprocessSlots`
  package var, and the `acquireSubprocessSlot` function (previously gated by a `runtime.GOOS != "windows"`
  check); dropped the now-unused `runtime` import. The `run`/`output` call sites that invoke
  `acquireSubprocessSlot` are unchanged.
- New `internal/ci/acceptance/subprocess_cap_windows.go` (`//go:build windows`): the real
  `maxConcurrentSubprocesses`/`subprocessSlots`/`acquireSubprocessSlot` implementation, moved verbatim (minus
  the now-unnecessary `runtime.GOOS` check, since the build tag handles that).
- New `internal/ci/acceptance/subprocess_cap_other.go` (`//go:build !windows`): a single no-op
  `acquireSubprocessSlot` returning an empty release func and `nil` error, matching the minimal-stub pattern
  used by `trust_install_other.go`.

(Prior rounds of this fix, unchanged by this doc: `66e50d45f5` added the semaphore and cap; `72319350ae`
scoped it to Windows via the runtime check this doc's changes now replace with build tags.)

## Validation

- `go build ./...` — clean.
- `GOOS=windows go build ./internal/ci/acceptance/...` — clean (confirms the windows-tagged file compiles;
  this sandbox can't run a Windows binary, so this is a cross-compile check only, not an execution check).
- `go vet ./internal/ci/acceptance/...` — clean.
- `gofumpt -l internal/ci/acceptance/*.go` — no output.
- `go test ./internal/ci/acceptance/...` — `ok`, 62.8s.
- `./custom-gcl run --new-from-rev=origin/main` — skipped: no prebuilt `custom-gcl` binary exists in this
  worktree (per this repo's own convention, pre-commit hooks run a prebuilt binary rather than building it
  in-hook, and none was available here to run standalone either).
- Not independently re-confirmed here: the `66e50d45f5`/`72319350ae` commit messages' own stated local
  Windows-crash reproduction and the ~15s→~11.7s macOS timing comparison — those are prior, already-landed
  validation, not re-run for this file-split refactor since it doesn't change runtime behavior on any
  platform (same semaphore, same cap, same no-op condition, only the compilation boundary moved).
- The actual Windows-crash fix (as opposed to this refactor's compile/test correctness) can only be confirmed
  by the next real Windows CI run, same caveat as other Windows-only race fixes in this directory (e.g.
  `2026-08-19-terraform-registry-cache-windows-ci-timeout.md`).

## Follow-ups

None.
