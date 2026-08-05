package exec

import (
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// lastAuth holds the auth context and manager from the most recent
// ExecuteTerraform call. PostRunE hooks (e.g. store hooks) need this so
// the terraform output subprocess can access backends that require role
// assumption. Reset on each ExecuteTerraform call.
var lastAuth struct {
	mu          sync.Mutex
	authContext *schema.AuthContext
	authManager any
}

// SetLastAuthContext persists the auth context and manager from the
// current terraform execution so PostRunE hooks can reuse them.
func SetLastAuthContext(authContext *schema.AuthContext, authManager any) {
	defer perf.Track(nil, "exec.SetLastAuthContext")()

	lastAuth.mu.Lock()
	defer lastAuth.mu.Unlock()
	lastAuth.authContext = authContext
	lastAuth.authManager = authManager
}

// GetLastAuthContext returns the auth context and manager from the most
// recent ExecuteTerraform call.
func GetLastAuthContext() (*schema.AuthContext, any) {
	defer perf.Track(nil, "exec.GetLastAuthContext")()

	lastAuth.mu.Lock()
	defer lastAuth.mu.Unlock()
	return lastAuth.authContext, lastAuth.authManager
}

// ClearLastAuthContext resets the persisted auth context. Called by tests.
func ClearLastAuthContext() {
	defer perf.Track(nil, "exec.ClearLastAuthContext")()

	lastAuth.mu.Lock()
	defer lastAuth.mu.Unlock()
	lastAuth.authContext = nil
	lastAuth.authManager = nil
}
