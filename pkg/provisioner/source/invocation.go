package source

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/auth/fingerprint"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

type invocationProvisioning struct {
	entries sync.Map
}
type invocationEntry struct {
	// The slot is a one-element semaphore rather than a mutex so a waiter can stop on cancellation.
	slot chan struct{}
	done bool
	key  string
}

var currentInvocation atomic.Pointer[invocationProvisioning]

// BeginInvocation scopes successful source reuse to one top-level CLI invocation.
// Direct library calls have no process-lifetime cache. The restore function supports
// nested CLI invocations and must be deferred by the owner.
func BeginInvocation() func() {
	defer perf.Track(nil, "source.BeginInvocation")()
	previous := currentInvocation.Swap(&invocationProvisioning{})
	return func() { currentInvocation.Store(previous) }
}

func provisioningKey(target *Target, componentType string, config map[string]any, spec *schema.VendorComponentSource, auth *schema.AuthContext) string {
	absolute, _ := filepath.Abs(target.Dir)
	// Key credentials with a process-local secret rather than retaining plain digests. Different
	// auth scopes, source selectors, filters and component instances never share a record.
	encoded, _ := json.Marshal([]any{absolute, componentType, config["atmos_stack"], config["atmos_component"], spec, auth, config["auth"]})
	return fingerprint.Key(encoded)
}

// beginProvision serializes duplicate callers within an explicitly established invocation.
// A failed attempt is never cached. Independent library calls still use publication locking.
func beginProvision(ctx context.Context, key, target string) (bool, func(bool), error) {
	invocation := currentInvocation.Load()
	if invocation == nil {
		return false, func(bool) {}, nil
	}
	value, _ := invocation.entries.LoadOrStore(target, &invocationEntry{slot: make(chan struct{}, 1)})
	entry := value.(*invocationEntry)
	select {
	case entry.slot <- struct{}{}:
	case <-ctx.Done():
		return false, nil, ctx.Err()
	}
	release := func() { <-entry.slot }
	// The slot and cancellation can become ready together; a cancelled caller never reports reuse.
	if err := ctx.Err(); err != nil {
		release()
		return false, nil, err
	}
	if entry.done && entry.key == key && isNonEmptyDir(target) {
		release()
		return true, func(bool) {}, nil
	}
	return false, func(success bool) {
		entry.done = success
		if success {
			entry.key = key
		}
		release()
	}, nil
}

// provisionedMarker is valid only in the invocation that created it.
type provisionedMarker struct {
	key        string
	invocation *invocationProvisioning
}
