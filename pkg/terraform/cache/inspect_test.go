package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedObject writes a cache object and its sidecar under root.
func seedObject(t *testing.T, root, key string, size int, kind string, fetchedAt time.Time) {
	t.Helper()
	objPath := filepath.Join(root, filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(objPath), 0o755))
	require.NoError(t, os.WriteFile(objPath, make([]byte, size), 0o644))

	sc := map[string]any{
		"sha256":     "deadbeef",
		"created_at": fetchedAt.Format(time.RFC3339),
		"custom": map[string]any{
			"kind":       kind,
			"fetched_at": fetchedAt.Format(time.RFC3339),
		},
	}
	b, _ := json.MarshalIndent(sc, "", "  ")
	require.NoError(t, os.WriteFile(objPath+metadataSuffix, b, 0o644))
}

func TestListAndSummarize(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	seedObject(t, root, "providers/registry.terraform.io/hashicorp/aws/index.json", 100, "metadata", now)
	seedObject(t, root, "providers/registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.95.0_linux_amd64.zip", 5000, "artifact", now.Add(-2*time.Hour))
	seedObject(t, root, "modules/registry.terraform.io/cloudposse/vpc/aws/versions.json", 200, "metadata", now)

	entries, err := List(root)
	require.NoError(t, err)
	require.Len(t, entries, 3)

	summary, err := Summarize(root)
	require.NoError(t, err)
	assert.Equal(t, 3, summary.ObjectCount)
	assert.Equal(t, 2, summary.Providers)
	assert.Equal(t, 1, summary.Modules)
	assert.Equal(t, int64(5300), summary.TotalSize)
	require.NotNil(t, summary.Largest)
	assert.Equal(t, int64(5000), summary.Largest.Size)
}

// TestListAndSummarize_ExcludesNonArtifacts proves that proxy infrastructure
// co-located in the cache root (the TLS certificate under tls/, and any stray file
// outside providers/ and modules/) is not counted as a cached object. This guards the
// fix for the stats reporting the cert as an object and surfacing tls/proxy.pem as the
// "Oldest" entry.
func TestListAndSummarize_ExcludesNonArtifacts(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	// Real cached artifacts.
	seedObject(t, root, "providers/registry.terraform.io/hashicorp/aws/index.json", 100, "metadata", now)
	seedObject(t, root, "modules/registry.terraform.io/cloudposse/vpc/aws/versions.json", 200, "metadata", now)

	// Proxy infrastructure that lives in the same root but is NOT a cached artifact.
	// The cert is the oldest file on disk, so a naive walk would report it as "Oldest".
	seedObject(t, root, "tls/proxy.pem", 4096, "", now.Add(-72*time.Hour))
	seedObject(t, root, "tls/proxy-key.pem", 2048, "", now.Add(-72*time.Hour))
	seedObject(t, root, "objects/stray", 999, "", now.Add(-48*time.Hour))

	entries, err := List(root)
	require.NoError(t, err)
	require.Len(t, entries, 2, "only provider and module artifacts are listed")

	summary, err := Summarize(root)
	require.NoError(t, err)
	assert.Equal(t, 2, summary.ObjectCount)
	assert.Equal(t, 1, summary.Providers)
	assert.Equal(t, 1, summary.Modules)
	assert.Equal(t, int64(300), summary.TotalSize, "tls/ and stray files excluded from size")
	require.NotNil(t, summary.Oldest)
	assert.NotContains(t, summary.Oldest.Key, "tls/", "the TLS cert must not be reported as the oldest object")
}

func TestDelete(t *testing.T) {
	root := t.TempDir()
	key := "providers/registry.terraform.io/hashicorp/aws/index.json"
	seedObject(t, root, key, 100, "metadata", time.Now())

	require.NoError(t, Delete(root, key))
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(key)))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(root, filepath.FromSlash(key)) + metadataSuffix)
	assert.True(t, os.IsNotExist(err))
	// Idempotent.
	assert.NoError(t, Delete(root, key))
}

func TestPrune_RetainsArtifactsByDefault(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-1000 * time.Hour)
	seedObject(t, root, "providers/h/aws/index.json", 100, "metadata", old)
	seedObject(t, root, "providers/h/aws/terraform-provider-aws_5.95.0_linux_amd64.zip", 5000, "artifact", old)

	// Default prune removes only stale metadata, keeps immutable artifacts.
	pruned, err := Prune(root, 168*time.Hour, false, false)
	require.NoError(t, err)
	require.Len(t, pruned, 1)
	assert.Equal(t, "metadata", pruned[0].Kind)

	// With --all, the old artifact is pruned too.
	seedObject(t, root, "providers/h/aws/index.json", 100, "metadata", old)
	pruned, err = Prune(root, 168*time.Hour, true, false)
	require.NoError(t, err)
	assert.Len(t, pruned, 2)
}

func TestPrune_DryRunDeletesNothing(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-1000 * time.Hour)
	seedObject(t, root, "providers/h/aws/index.json", 100, "metadata", old)

	pruned, err := Prune(root, 168*time.Hour, false, true)
	require.NoError(t, err)
	require.Len(t, pruned, 1)
	_, statErr := os.Stat(filepath.Join(root, "providers", "h", "aws", "index.json"))
	assert.NoError(t, statErr, "dry-run must not delete")
}
