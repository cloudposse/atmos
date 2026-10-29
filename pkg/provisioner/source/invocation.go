package source

import (
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
	mu   sync.Mutex
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
func beginProvision(key, target string) (bool, func(bool)) {
	invocation := currentInvocation.Load()
	if invocation == nil {
		return false, func(bool) {}
	}
	value, _ := invocation.entries.LoadOrStore(target, &invocationEntry{})
	entry := value.(*invocationEntry)
	entry.mu.Lock()
	if entry.done && entry.key == key && isNonEmptyDir(target) {
		entry.mu.Unlock()
		return true, func(bool) {}
	}
	return false, func(success bool) {
		entry.done = success
		if success {
			entry.key = key
		}
		entry.mu.Unlock()
	}
}

// provisionedMarker is valid only in the invocation that created it.
type provisionedMarker struct {
	key        string
	invocation *invocationProvisioning
}
