package policy

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countSearchDirs resets the roots cache and counts root discovery for one test.
func countSearchDirs(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	previous := searchDirs
	searchDirs = func(cwd string, args []string) []string {
		calls.Add(1)
		return previous(cwd, args)
	}
	resetSearchRoots := func() {
		searchRootsCache.Lock()
		searchRootsCache.valid, searchRootsCache.key, searchRootsCache.roots = false, "", nil
		searchRootsCache.Unlock()
	}
	resetSearchRoots()
	t.Cleanup(func() {
		searchDirs = previous
		resetSearchRoots()
	})
	return &calls
}

// resolved returns path as an absolute path with symlinks resolved; t.Chdir may
// resolve macOS /var.
func resolved(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	require.NoError(t, err)
	result, err := filepath.EvalSymlinks(absolute)
	require.NoError(t, err)
	return result
}

func TestPolicyDirectoryReusesRootsForUnchangedInputs(t *testing.T) {
	isolateDiscovery(t)
	calls := countSearchDirs(t)
	t.Chdir(t.TempDir())

	for range 3 {
		directory, err := DirectoryFromEnvironment()
		require.NoError(t, err)
		require.Empty(t, directory)
	}
	assert.Equal(t, int32(1), calls.Load(), "unchanged inputs must reuse the discovered roots")

	// policy.d is checked on every call, so creating it is seen without rediscovery.
	require.NoError(t, os.Mkdir("policy.d", 0o700))
	directory, err := DirectoryFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, resolved(t, "policy.d"), resolved(t, directory))
	assert.Equal(t, int32(1), calls.Load())

	// Removing it is seen the same way.
	require.NoError(t, os.Remove("policy.d"))
	directory, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	assert.Empty(t, directory)
	assert.Equal(t, int32(1), calls.Load())
}

func TestPolicyDirectoryRecomputesRootsWhenInputsChange(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, elsewhere string)
		// found reports whether the change should select elsewhere/policy.d.
		found bool
	}{
		{
			name:   "working directory",
			change: func(t *testing.T, elsewhere string) { t.Chdir(elsewhere) },
			found:  true,
		},
		{
			name: "arguments",
			change: func(t *testing.T, elsewhere string) {
				setArgs(t, "atmos", "--config-path", elsewhere, "describe", "stacks")
			},
			found: true,
		},
		{
			name: "environment",
			change: func(t *testing.T, elsewhere string) {
				t.Setenv("ATMOS_CONFIG_PATH", elsewhere)
			},
			found: true,
		},
		{
			name: "unrelated environment variable",
			change: func(t *testing.T, _ string) {
				t.Setenv("ATMOS_POLICY_DISCOVERY_CACHE_TEST", "changed")
			},
			found: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateDiscovery(t)
			calls := countSearchDirs(t)
			elsewhere := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(elsewhere, "policy.d"), 0o755))
			t.Chdir(t.TempDir())

			directory, err := DirectoryFromEnvironment()
			require.NoError(t, err)
			require.Empty(t, directory)
			require.Equal(t, int32(1), calls.Load())

			tt.change(t, elsewhere)
			directory, err = DirectoryFromEnvironment()
			require.NoError(t, err)
			assert.Equal(t, int32(2), calls.Load(), "a changed input must rediscover the roots")
			if tt.found {
				assert.Equal(t, resolved(t, filepath.Join(elsewhere, "policy.d")), resolved(t, directory))
			} else {
				assert.Empty(t, directory)
			}
		})
	}
}

func TestPolicyDirectoryConcurrentDiscovery(t *testing.T) {
	isolateDiscovery(t)
	countSearchDirs(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("policy.d", 0o700))
	want := resolved(t, "policy.d")

	var wg sync.WaitGroup
	results := make([]string, 16)
	errs := make([]error, len(results))
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = DirectoryFromEnvironment()
		}()
	}
	wg.Wait()
	for i := range results {
		require.NoError(t, errs[i])
		assert.Equal(t, want, resolved(t, results[i]))
	}
}

func TestSearchRootsKeySeparatesFields(t *testing.T) {
	// Moving a value between fields must never produce the same key.
	assert.NotEqual(
		t,
		searchRootsKey("/a", []string{"b"}, nil),
		searchRootsKey("/a", nil, []string{"b"}),
	)
	assert.NotEqual(
		t,
		searchRootsKey("/a", []string{"b"}, nil),
		searchRootsKey("/a/b", nil, nil),
	)
}

// An ancestor that gains atmos.yaml while the process runs becomes a project
// root, so its policy.d is found without a change to the cwd, arguments or
// environment.
func TestPolicyDirectoryRecomputesRootsWhenAnAncestorGainsConfig(t *testing.T) {
	isolateDiscovery(t)
	calls := countSearchDirs(t)
	root := t.TempDir()
	child := filepath.Join(root, "stacks", "deploy")
	require.NoError(t, os.MkdirAll(child, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(root, "policy.d"), 0o700))
	t.Chdir(child)

	directory, err := DirectoryFromEnvironment()
	require.NoError(t, err)
	require.Empty(t, directory, "without atmos.yaml the ancestor is not a project root")

	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("base_path: .\n"), 0o600))
	directory, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, resolved(t, filepath.Join(root, "policy.d")), resolved(t, directory))
	assert.Equal(t, int32(2), calls.Load(), "the new configuration recomputes the roots once")

	// Negative path: with nothing else changed, the recomputed roots are reused.
	_, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

// A selected config path that changes from a file to a directory moves the
// project root from its parent to the directory itself, so the roots are
// recomputed and the directory's policy.d is found.
func TestPolicyDirectoryRecomputesRootsWhenASelectedConfigPathChangesType(t *testing.T) {
	isolateDiscovery(t)
	calls := countSearchDirs(t)
	root := t.TempDir()
	selected := filepath.Join(root, "config")
	require.NoError(t, os.WriteFile(selected, []byte("base_path: .\n"), 0o600))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", selected)
	t.Chdir(t.TempDir())

	directory, err := DirectoryFromEnvironment()
	require.NoError(t, err)
	require.Empty(t, directory, "a selected file makes its parent, which has no policy.d, the root")

	require.NoError(t, os.Remove(selected))
	require.NoError(t, os.MkdirAll(filepath.Join(selected, "policy.d"), 0o700))
	directory, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, resolved(t, filepath.Join(selected, "policy.d")), resolved(t, directory))
	assert.Equal(t, int32(2), calls.Load())
}
