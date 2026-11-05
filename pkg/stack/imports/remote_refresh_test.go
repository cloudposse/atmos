package imports

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRemoteImporter_Resolve_GitSubdir_RefreshesCachedFiles verifies that a Git subdirectory import
// picks up a new commit on the next run: the cached per-file copy must not outlive a re-clone of
// the source. With a fresh ttl the source is not re-cloned, so the cached content is correctly kept.
func TestRemoteImporter_Resolve_GitSubdir_RefreshesCachedFiles(t *testing.T) {
	const relPath = "stacks/catalog/vpc.yaml"

	// resolve runs one "invocation": a new importer sharing the on-disk cache directory.
	resolve := func(t *testing.T, cacheDir, ttl, uri string) string {
		t.Helper()

		testCache, err := cache.NewFileCache("test", cache.WithBaseDir(cacheDir))
		require.NoError(t, err)
		importer, err := NewRemoteImporter(
			&schema.AtmosConfiguration{Imports: schema.ImportsSettings{TTL: ttl}},
			WithCache(testCache),
		)
		require.NoError(t, err)

		matches, err := importer.Resolve(uri)
		require.NoError(t, err)
		require.Len(t, matches, 1)
		return readMatch(t, matches[0])
	}

	// publish commits new content for the imported file.
	publish := func(t *testing.T, repoDir, content string) {
		t.Helper()

		require.NoError(t, os.WriteFile(filepath.Join(repoDir, filepath.FromSlash(relPath)), []byte(content), 0o644))
		runGit(t, repoDir, "commit", "-am", "update")
	}

	t.Run("no ttl re-clones and serves the new commit", func(t *testing.T) {
		repoDir := initGitRepo(t, map[string]string{relPath: "v1"})
		uri := fmt.Sprintf("git::%s//%s?ref=main", gitFileURI(repoDir), relPath)
		cacheDir := t.TempDir()

		assert.Equal(t, "v1", resolve(t, cacheDir, "", uri))
		publish(t, repoDir, "v2")
		assert.Equal(t, "v2", resolve(t, cacheDir, "", uri))
	})

	t.Run("fresh ttl keeps the cached content without re-cloning", func(t *testing.T) {
		repoDir := initGitRepo(t, map[string]string{relPath: "v1"})
		uri := fmt.Sprintf("git::%s//%s?ref=main", gitFileURI(repoDir), relPath)
		cacheDir := t.TempDir()

		assert.Equal(t, "v1", resolve(t, cacheDir, "1h", uri))
		publish(t, repoDir, "v2")
		assert.Equal(t, "v1", resolve(t, cacheDir, "1h", uri))
	})
}
