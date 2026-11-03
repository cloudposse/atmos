package ci

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/viperguard"
)

// Reporter creation can overlap with theme initialization binding environment keys.
func TestCIModeConcurrentEnvironmentBinding(t *testing.T) {
	const iterations = 1000
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for range iterations {
			_ = forcedCIMode()
			_ = ModeEnabled(nil)
		}
	}()
	defer workers.Wait()
	for range iterations {
		require.NoError(t, viperguard.BindEnv("theme", "ATMOS_THEME"))
	}
}
