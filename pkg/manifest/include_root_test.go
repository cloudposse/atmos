package manifest

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const includeRootManifest = `apiVersion: atmos/v1
kind: AtmosTestConfig
metadata:
  name: x
spec:
  source: embedded
  values: !include ./greeting.yaml
`

func loadWithIncludeRoot(t *testing.T, dir, root string) error {
	t.Helper()
	registerTestKind(t)
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	_, err := Load[testSpec](testKind, []byte(includeRootManifest),
		WithIncludeResolution(atmosConfig, filepath.Join(dir, "manifest.yaml"), nil),
		WithIncludeRoot(root))
	return err
}

// A publication root that cannot be resolved is a manifest error naming the root, with the
// filesystem cause preserved.
func TestLoad_WithIncludeRoot_MissingRootIsManifestError(t *testing.T) {
	dir := t.TempDir()
	writeIncludeFixture(t, dir, "greeting.yaml", "hello: world\n")
	missing := filepath.Join(dir, "does-not-exist")

	err := loadWithIncludeRoot(t, dir, missing)

	require.ErrorIs(t, err, errUtils.ErrManifestIncludeRoot)
	assert.Contains(t, err.Error(), missing)
}

// A resolvable root admits includes inside it.
func TestLoad_WithIncludeRoot_AllowsIncludeInsideRoot(t *testing.T) {
	dir := t.TempDir()
	writeIncludeFixture(t, dir, "greeting.yaml", "hello: world\n")

	require.NoError(t, loadWithIncludeRoot(t, dir, dir))
}
