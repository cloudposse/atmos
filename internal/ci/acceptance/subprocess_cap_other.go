//go:build !windows

package acceptance

import "context"

// acquireSubprocessSlot is a no-op on every platform but Windows -- the GC/allocator
// race the real cap guards against (see subprocess_cap_windows.go) has only been
// observed there, so Linux/macOS runs keep their full, uncapped concurrency.
func acquireSubprocessSlot(_ context.Context) (func(), error) {
	return func() {}, nil
}
