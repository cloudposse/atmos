package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// A profile deep-merges over the base declaration: the merged package keeps source and version and
// gains path, and the loaded configuration retains the profile names it applied.
func TestProfileAddsPathBesideTheReleaseDeclaration(t *testing.T) {
	setupTestAdapters()
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "profiles", "kljasdljkhbaslhkdhlkjsd"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "profiles", "sandbox"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(`
base_path: .
profiles:
  base_path: profiles
plugins:
  packages:
    corporate:
      source: oci://registry.example.com/plugins/oidc
      version: ^0.1.0
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "profiles", "kljasdljkhbaslhkdhlkjsd", "atmos.yaml"), []byte(`
plugins:
  packages:
    corporate:
      path: build/corporate-oidc
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "profiles", "sandbox", "atmos.yaml"), []byte("logs:\n  level: Info\n"), 0o644))

	active, err := InitCliConfig(schema.ConfigAndStacksInfo{ProfilesFromArg: []string{"kljasdljkhbaslhkdhlkjsd", "sandbox", "kljasdljkhbaslhkdhlkjsd"}}, false)
	require.NoError(t, err)
	corporate := active.Plugins.Packages["corporate"]
	assert.Equal(t, "build/corporate-oidc", corporate.Path)
	assert.Equal(t, "oci://registry.example.com/plugins/oidc", corporate.Source, "source stays in the merged configuration")
	assert.Equal(t, "^0.1.0", corporate.Version, "version stays in the merged configuration")
	assert.Equal(t, []string{"kljasdljkhbaslhkdhlkjsd", "sandbox"}, active.ActiveProfiles, "names as typed, de-duplicated, in activation order")

	// Negative: no profile, no path, and no profile names.
	viper.Reset()
	inactive, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	assert.Empty(t, inactive.Plugins.Packages["corporate"].Path)
	assert.Equal(t, "oci://registry.example.com/plugins/oidc", inactive.Plugins.Packages["corporate"].Source)
	assert.Empty(t, inactive.ActiveProfiles)
}

func TestUniqueProfileNames(t *testing.T) {
	assert.Equal(t, []string{}, uniqueProfileNames(nil))
	assert.Equal(t, []string{"b", "a", "B"}, uniqueProfileNames([]string{"b", "", "a", "b", "B", "a"}))
}
