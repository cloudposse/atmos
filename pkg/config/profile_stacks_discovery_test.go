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

// TestInitCliConfig_ProfileMergedStacksSettingsUsedDuringDiscovery reproduces
// https://github.com/cloudposse/atmos/issues/2827: `atmos describe config --profile test`
// reports the profile-merged `stacks.base_path`/`included_paths`, but commands that
// discover stacks (processStacks=true, e.g. `atmos list dependencies`) failed to find
// any stack manifests under those very same profile-supplied settings.
func TestInitCliConfig_ProfileMergedStacksSettingsUsedDuringDiscovery(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))

	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "profiles", "test"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "profile-stacks", "deploy"), 0o755))

	// Base atmos.yaml intentionally defines NO `stacks:` section at all — every
	// stack-discovery setting comes from the profile, exactly as in the issue repro.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(`
base_path: .
profiles:
  base_path: profiles
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "profiles", "test", "atmos.yaml"), []byte(`
stacks:
  base_path: profile-stacks
  included_paths:
    - "deploy/**/*.yaml"
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "profile-stacks", "deploy", "dev.yaml"), []byte(`
vars:
  stage: dev

components:
  terraform:
    app:
      metadata:
        component: app
      dependencies:
        components:
          - component: database
    database:
      metadata:
        component: database
`), 0o644))

	// Mirrors `atmos describe config --profile test`: processStacks=false, never
	// exercises stack discovery.
	describeConfigInfo := schema.ConfigAndStacksInfo{ProfilesFromArg: []string{"test"}}
	describeCfg, err := InitCliConfig(describeConfigInfo, false)
	require.NoError(t, err)
	require.Equal(t, "profile-stacks", describeCfg.Stacks.BasePath,
		"precondition: describe config should report the profile-merged stacks.base_path")
	require.Equal(t, []string{"deploy/**/*.yaml"}, describeCfg.Stacks.IncludedPaths,
		"precondition: describe config should report the profile-merged stacks.included_paths")

	// Mirrors `atmos list dependencies --profile test --stack dev`: processStacks=true,
	// which must discover stack manifests using the same profile-merged settings.
	listDepsInfo := schema.ConfigAndStacksInfo{
		ProfilesFromArg: []string{"test"},
		Stack:           "dev",
	}
	listDepsCfg, err := InitCliConfig(listDepsInfo, true)
	require.NoError(t, err,
		"list dependencies should discover the stack using the same profile-merged stacks "+
			"config that describe config reports")
	require.NotEmpty(t, listDepsCfg.StackConfigFilesAbsolutePaths,
		"stack discovery should find dev.yaml under the profile-merged base_path/included_paths")
	assert.Contains(t, filepath.ToSlash(listDepsCfg.StackConfigFilesAbsolutePaths[0]),
		"profile-stacks/deploy/dev.yaml")
}
