package bootstrap

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
)

func TestConcurrentAvailableAuthConsumersRetainConstructorsUntilFinalRelease(t *testing.T) {
	const workers = 16
	atmos, requests := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	// Keep one consumer alive while all concurrent users register and release.
	retained, err := PrepareAvailable(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(retained)
	start := make(chan struct{})
	var consumers sync.WaitGroup
	for range workers {
		consumers.Go(func() {
			<-start
			cleanup, prepareErr := PrepareAvailable(t.Context(), atmos)
			if prepareErr != nil {
				t.Errorf("concurrent auth preparation failed: %v", prepareErr)
				return
			}
			cleanup()
			cleanup() // A repeated release must not consume another caller's lease.
			if !authregistry.HasProvider("test/bootstrap-provider") {
				t.Error("concurrent release removed constructors still in use")
			}
		})
	}
	close(start)
	consumers.Wait()
	require.True(t, authregistry.HasProvider("test/bootstrap-provider"))
	require.Zero(t, requests.Load(), "auth preparation must not download packages")
	retained()
	require.False(t, authregistry.HasProvider("test/bootstrap-provider"))
}
