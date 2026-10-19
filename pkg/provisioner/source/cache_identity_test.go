package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Metadata receipts redact query parameters, while the retained source declaration
// must still distinguish refs and allow an unchanged pinned source to hit the cache.
func TestSourceMetadataCacheRetainsQueryIdentity(t *testing.T) {
	target := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("# staged\n"), 0o644))
	spec := &schema.VendorComponentSource{
		Uri: "git::https://example.com/components.git//app?ref=v1", TTL: "1h",
	}
	require.NoError(t, writeWorkdirMetadata(target, "app", "dev", spec))
	needs, reason := needsProvisioning(target, spec, true)
	assert.False(t, needs, "an unchanged pinned source must reuse its workdir: %s", reason)
	spec.Uri = "git::https://example.com/components.git//app?ref=v2"
	needs, _ = needsProvisioning(target, spec, true)
	assert.True(t, needs, "a changed query ref must refresh the staged content")
}
