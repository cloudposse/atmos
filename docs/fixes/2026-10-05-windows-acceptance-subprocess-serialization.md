# Mitigation: serialize Windows acceptance harness subprocesses

## Failure

PR #3277's Windows acceptance shard 3 failed in
[job 112005518816](https://github.com/cloudposse/atmos/actions/runs/37380640146/job/112005518816).
The Go 1.26.8 runtime terminated `internal/ci/acceptance` with
`fatal error: found pointer to free object` in `runtime.(*mspan).reportZombies`.
The active goroutine was starting a fixture build through `syscall.envSorted`;
another goroutine was waiting for a subprocess launched by `Verify`.

This matches the crash signature documented in the September 11 and September 13
Windows subprocess fixes. The limit of two concurrent subprocesses still permits
the overlapping execution observed in this failure. The stack alone does not
establish the underlying runtime defect, and this change is a mitigation.

## Change

Reduce the Windows-only subprocess limit from two to one. Every `commandRunner.run`
and `commandRunner.output` invocation holds the shared slot until its subprocess
exits. Waiting callers continue to respect context cancellation. Other platforms
retain their existing concurrency. Windows harness tests may take longer.

Add deterministic tests that a second caller waits for the first to release its
slot, proceeds after release, and can cancel while waiting. Both tests fail with
the previous two-slot limit. They enforce serialization; they do not reproduce
the Go runtime crash.

## Validation

- Run the Windows semaphore implementation and its tests directly on the host:
  `go test -race internal/ci/acceptance/subprocess_cap_windows.go internal/ci/acceptance/subprocess_cap_windows_test.go -count=10 -timeout=30s`.
- Run the complete host harness suite:
  `go test -race ./internal/ci/acceptance -count=1 -timeout=5m`.
- Cross-compile the Windows harness tests with `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c`.
- Lint the affected package for the host and Windows targets.

The runtime crash occurred on Windows; host tests and cross-compilation cannot
prove it is eliminated. The next Windows CI run exercises the mitigation on that
platform.
