package git

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootTagCacheResolvesOncePerDirectory(t *testing.T) {
	cache := &rootTagCache{}
	calls := 0
	resolve := func() (string, error) { calls++; return "root", nil }
	for range 26 {
		root, err := cache.resolve("first-directory", resolve)
		require.NoError(t, err)
		assert.Equal(t, "root", root)
	}
	assert.Equal(t, 1, calls)
	_, err := cache.resolve("second-directory", resolve)
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	cache.clear()
	_, err = cache.resolve("first-directory", resolve)
	require.NoError(t, err)
	assert.Equal(t, 3, calls, "new invocations must resolve again")
}

func TestRootTagCacheDoesNotCacheErrors(t *testing.T) {
	cache := &rootTagCache{}
	_, err := cache.resolve("directory", func() (string, error) { return "", errors.New("no repository") })
	require.Error(t, err)
	root, err := cache.resolve("directory", func() (string, error) { return "new repository", nil })
	require.NoError(t, err)
	assert.Equal(t, "new repository", root)
}

func TestRootTagCacheCoalescesConcurrentLookups(t *testing.T) {
	cache := &rootTagCache{}
	var calls atomic.Int32
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			root, err := cache.resolve("directory", func() (string, error) {
				calls.Add(1)
				return "root", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, "root", root)
		})
	}
	workers.Wait()
	assert.EqualValues(t, 1, calls.Load())
}

func TestRootTagsFollowWorkingDirectoryAndRepositoryCreation(t *testing.T) {
	ResetRootTagCache()
	t.Cleanup(ResetRootTagCache)
	for range 2 {
		dir := t.TempDir()
		t.Chdir(dir)
		fallback, err := ProcessTagRoot("!repo-root fallback")
		require.NoError(t, err)
		assert.Equal(t, "fallback", fallback)
		_, err = gogit.PlainInit(dir, false)
		require.NoError(t, err)
		expected, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		for _, tag := range []string{"!repo-root", "!git.root"} {
			actual, err := ProcessTagRoot(tag)
			require.NoError(t, err)
			assert.Equal(t, filepath.Clean(expected), filepath.Clean(actual))
		}
	}
}
