package exec

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupSharedCacheFixture chdirs into the stack-manifest-name-template fixture
// (component `vpc` defined in two stacks: `my-explicit-stack` and `prod-ue2`),
// initializes the CLI config, and resets the FindStacksMap cache around the test.
func setupSharedCacheFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()

	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "my-explicit-stack",
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)

	return atmosConfig
}

// TestProcessStacksDoesNotMutateSharedStacksMapCache verifies that ProcessStacks
// never writes into the map tree owned by the shared FindStacksMap cache.
//
// FindStacksMap returns its cached maps by reference, and DAG-scheduled bulk
// commands (`terraform <cmd> --all/--affected/--query`) run ProcessStacks
// concurrently against that shared tree. Any in-place mutation of the cached
// maps is therefore a data race (`fatal error: concurrent map iteration and map
// write`) as well as cross-command cache pollution. This test enforces the
// invariant deterministically, without requiring the race detector.
func TestProcessStacksDoesNotMutateSharedStacksMapCache(t *testing.T) {
	atmosConfig := setupSharedCacheFixture(t)

	// Warm the cache and snapshot the shared tree.
	stacksMap, _, _, err := FindStacksMap(&atmosConfig, false)
	require.NoError(t, err)
	snapshot, err := m.DeepCopyMap(stacksMap)
	require.NoError(t, err)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "my-explicit-stack",
		ComponentType:    cfg.TerraformComponentType,
	}
	_, err = ProcessStacks(&atmosConfig, info, true, false, false, nil, nil)
	require.NoError(t, err)

	after, _, _, err := FindStacksMap(&atmosConfig, false)
	require.NoError(t, err)
	// Normalize the "after" tree through the same deep copy as the snapshot so the
	// comparison is insensitive to DeepCopyMap's type normalization (e.g. []string
	// vs []any) and only real mutations of the cached tree register.
	afterNormalized, err := m.DeepCopyMap(after)
	require.NoError(t, err)
	require.Equal(t, snapshot, afterNormalized,
		"ProcessStacks must not mutate the shared FindStacksMap cache")
}

// TestProcessStacksConcurrentSharedCacheAccess reproduces the production
// interleaving behind the reported `fatal error: concurrent map iteration and
// map write`: multiple DAG workers process the same component in different
// stacks, so each worker's findComponentInStacks iterates the other stack's
// component section (from the shared FindStacksMap cache) while the other
// worker writes into it. Run with `go test -race` to detect the race; without
// the fix the unsynchronized access is reported (and can crash fatally even in
// plain mode).
func TestProcessStacksConcurrentSharedCacheAccess(t *testing.T) {
	atmosConfig := setupSharedCacheFixture(t)

	// Warm the cache once so every goroutine below gets a cache hit and shares
	// the same map tree, exactly as concurrent DAG workers do.
	_, _, _, err := FindStacksMap(&atmosConfig, false)
	require.NoError(t, err)

	// Two stacks that both define the `vpc` component.
	stacks := []string{"my-explicit-stack", "prod-ue2"}
	const workers = 16
	const iterations = 10

	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations)
	for w := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range iterations {
				info := schema.ConfigAndStacksInfo{
					ComponentFromArg: "vpc",
					Stack:            stacks[(worker+i)%len(stacks)],
					ComponentType:    cfg.TerraformComponentType,
				}
				if _, err := ProcessStacks(&atmosConfig, info, true, false, false, nil, nil); err != nil {
					errCh <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}
