package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerfileInstallsPython3Runtime(t *testing.T) {
	content, err := os.ReadFile("../Dockerfile")
	require.NoError(t, err)

	assert.Contains(t, string(content), "--no-install-recommends curl git gnupg ca-certificates docker.io python3")
	assert.NotContains(t, string(content), "python3-pip")
	assert.NotContains(t, string(content), "python3-venv")
}

// TestAtmosCacheActionValidatesMetadataBeforeActionsCache guards the
// invariant that actions/cache derives its cache metadata before either
// terminal cache step runs. Key/path validation itself (empty key, empty
// paths, CR/LF rejection, delimiter-collision-safe multiline encoding) lives
// in Go now -- see cmd/ci/cache/paths.go's emitGitHubCachePaths and its own
// tests (TestEmitGitHubCachePaths_EmptyKey/_EmptyPaths) -- so this only
// checks structure/ordering, not bash string literals that no longer exist.
func TestAtmosCacheActionValidatesMetadataBeforeActionsCache(t *testing.T) {
	content, err := os.ReadFile("../actions/cache/action.yml")
	require.NoError(t, err)
	action := string(content)

	metaIdx := strings.Index(action, "atmos ci cache paths --format=github")
	require.NotEqual(t, -1, metaIdx, "action.yml must derive cache metadata via `atmos ci cache paths --format=github`")

	cacheIdx := strings.Index(action, "id: cache\n")
	require.NotEqual(t, -1, cacheIdx, "action.yml must have a `cache` step")

	cacheRestoreIdx := strings.Index(action, "id: cache-restore\n")
	require.NotEqual(t, -1, cacheRestoreIdx, "action.yml must have a `cache-restore` step")

	assert.Less(t, metaIdx, cacheIdx, "cache metadata must be derived before the cache step runs")
	assert.Less(t, metaIdx, cacheRestoreIdx, "cache metadata must be derived before the cache-restore step runs")
	// The released bootstrap CLI can predate the GITHUB_ENV protocol and write
	// cache metadata only to this composite step's outputs. Keep each cache
	// action compatible with both it and the current CLI, which additionally
	// exports ATMOS_CACHE_* for nested composite post steps.
	fallbacks := []string{
		"env.ATMOS_CACHE_KEY || steps.meta.outputs.key",
		"env.ATMOS_CACHE_PATH || steps.meta.outputs.path",
		"env.ATMOS_CACHE_RESTORE_KEYS || steps.meta.outputs.restore-keys",
	}
	blocks := []struct {
		name    string
		content string
	}{
		{name: "cache", content: action[cacheIdx:cacheRestoreIdx]},
		{name: "cache-restore", content: action[cacheRestoreIdx:]},
	}
	for _, block := range blocks {
		t.Run(block.name, func(t *testing.T) {
			for _, fallback := range fallbacks {
				assert.Contains(t, block.content, fallback)
			}
		})
	}
}
