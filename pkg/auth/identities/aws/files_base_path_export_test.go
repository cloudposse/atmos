package aws

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFilesBasePath_ResolvesLikeBuiltInIdentities pins the exported helper that plugin and mock
// AWS identities use to the same resolution the built-in identities use.
func TestFilesBasePath_ResolvesLikeBuiltInIdentities(t *testing.T) {
	basePath := filepath.Join(t.TempDir(), "custom-aws")
	assert.Equal(t, basePath, FilesBasePath(basePathManager(t, basePath), "dev"))
	assert.Empty(t, FilesBasePath(nil, "dev"), "no manager keeps the default location")
}
