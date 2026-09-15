//go:build windows

package acceptance

import "context"

// maxConcurrentSubprocesses caps how many subprocesses (go build/go test/go tool
// covdata, plus each precompiled *.test.exe) this package launches at once. This
// package's own acceptance tests run dozens of t.Parallel() subtests that each shell
// out, so without a cap, a wide/many-core Windows CI runner lets that many real `go`
// toolchain invocations (each independently allocating and syscalling heavily) run
// fully concurrently. That has produced a runtime-fatal GC/allocator crash ("fatal
// error: found pointer to free object" / "marked free object in span") in
// mcache/mgcsweep during a concurrent os/exec process launch -- a long-standing,
// still-recurring class of Go runtime race under heavy concurrent allocation+syscall
// pressure specifically on Windows (see e.g. golang/go#44900, #45364, #47415, #54247;
// the reports are Windows-only), not anything specific to the command being run.
// Serializing actual subprocess launches (while still letting the surrounding Go test
// logic run in parallel) avoids the trigger condition without giving up test
// parallelism where it doesn't involve a real subprocess. See subprocess_cap_other.go
// for why this cap doesn't exist on other platforms.
//
// A cap of 4 (set in commit 72319350ae, see
// docs/fixes/2026-09-11-windows-acceptance-subprocess-race.md) was still insufficient:
// the same crash signature recurred four more times on Windows shard 3 on 2026-09-12,
// across two unrelated PRs -- #3107 (run 34699763477) and #3122 (runs 34703060789 and
// 34726124320, twice in the same PR). Lowered to 2 so fewer real `go` toolchain
// subprocesses can ever be in flight at once on Windows; revisit upward only with new
// evidence that 2 is unnecessarily conservative.
const maxConcurrentSubprocesses = 2

// subprocessSlots limits how many commandRunner.run/output calls -- across every
// commandRunner instance, since each caller constructs its own -- may have a real
// `cmd.Run()` in flight at once. See maxConcurrentSubprocesses.
var subprocessSlots = make(chan struct{}, maxConcurrentSubprocesses)

// acquireSubprocessSlot blocks until a subprocess slot is free or ctx is done,
// returning a release func to call (typically deferred) once the subprocess exits.
func acquireSubprocessSlot(ctx context.Context) (func(), error) {
	select {
	case subprocessSlots <- struct{}{}:
		return func() { <-subprocessSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
