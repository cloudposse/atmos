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
}
