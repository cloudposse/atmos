package manifest

import (
	"os"
	"path/filepath"
	"strings"
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

func writeOutsideInclude(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, "scaffold")
	require.NoError(t, os.Mkdir(dir, 0o755))
	writeIncludeFixture(t, parent, "greeting.yaml", "hello: world\n")
	return dir
}

func loadIncludeRootWith(t *testing.T, dir, includePath string) error {
	t.Helper()
	registerTestKind(t)
	data := []byte(strings.Replace(includeRootManifest, "./greeting.yaml", includePath, 1))
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	_, err := Load[testSpec](testKind, data,
		WithIncludeResolution(atmosConfig, filepath.Join(dir, "manifest.yaml"), nil),
		WithIncludeRoot(dir))
	return err
}

// An include that climbs out of the publication root is rejected as an invalid scaffold.
func TestLoad_WithIncludeRoot_RejectsIncludeOutsideRoot(t *testing.T) {
	err := loadIncludeRootWith(t, writeOutsideInclude(t), "../greeting.yaml")

	require.ErrorIs(t, err, errUtils.ErrArtifactScaffoldInvalid)
	assert.Contains(t, err.Error(), "outside the scaffold publication root")
}

// A symlink inside the root that points outside it is judged by its target.
func TestLoad_WithIncludeRoot_RejectsSymlinkEscapingRoot(t *testing.T) {
	dir := writeOutsideInclude(t)
	if err := os.Symlink(filepath.Join(filepath.Dir(dir), "greeting.yaml"), filepath.Join(dir, "greeting.yaml")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	err := loadWithIncludeRoot(t, dir, dir)

	require.ErrorIs(t, err, errUtils.ErrArtifactScaffoldInvalid)
}

// A forced local go-getter source cannot bypass the boundary.
func TestLoad_WithIncludeRoot_RejectsLocalGetterInclude(t *testing.T) {
	err := loadIncludeRootWith(t, t.TempDir(), "file::./greeting.yaml")

	require.ErrorIs(t, err, errUtils.ErrArtifactScaffoldInvalid)
	assert.Contains(t, err.Error(), "local getter include")
}

func TestOutsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"root itself", root, false},
		{"child", filepath.Join(root, "a", "b.yaml"), false},
		{"sibling", filepath.Join(filepath.Dir(root), "other"), true},
		{"parent", filepath.Dir(root), true},
		{"prefix lookalike", root + "-x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, outsideRoot(root, tt.path))
		})
	}
}
