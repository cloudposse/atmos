package proexec

import (
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/cloudposse/atmos/pkg/perf"
)

// invocationID is process-scoped like currentAtmosConfig; components never mutate it.
var invocationID atomic.Pointer[string]

// BeginInvocation allocates the ID shared by an execution record and its exceptions.
// The returned restoration function supports nested invocations and test isolation.
func BeginInvocation() (string, func()) {
	defer perf.Track(nil, "proexec.BeginInvocation")()

	id := uuid.NewString()
	previous := invocationID.Swap(&id)
	return id, func() { invocationID.Store(previous) }
}

// ExecutionID returns the current invocation's correlation ID, or empty outside an invocation.
//
//nolint:lintroller // Trivial atomic getter; performance tracking would dominate the lookup.
func ExecutionID() string {
	if id := invocationID.Load(); id != nil {
		return *id
	}
	return ""
}
