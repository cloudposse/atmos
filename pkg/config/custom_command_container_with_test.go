package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandContainerBuildStepDecodesWithBlock reproduces
// https://github.com/cloudposse/atmos/issues/2876: a custom command's
// `type: container, action: build` step with a `with:` block, defined in
// commands.yaml exactly as a user would write it, must decode `with:` into
// the typed `Build` struct -- not silently drop it.
//
// This loads the config through the real production path (InitCliConfig,
// which drives Viper's merge + mapstructure decode via atmosDecodeHook/
// TasksDecodeHook), not by constructing a schema.Task/ContainerBuildStep
// literal in Go. Workflow files loaded via pkg/utils.UnmarshalYAMLFromFile
// go through go-yaml's node.Decode, which invokes Task.UnmarshalYAML and
// correctly promotes `with:` into Build/Run/Push/Inspect. Custom commands
// merged into atmos.yaml's Viper config tree never hit that yaml.Unmarshaler
// method -- they decode via TasksDecodeHook -> decodeTaskFromMap, a separate
// mapstructure-based path that (before this fix) had no equivalent
// promotion, leaving Build nil regardless of what `with:` contained.
func TestCustomCommandContainerBuildStepDecodesWithBlock(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	atmosYAML := `
base_path: "."
commands:
  - name: build
    description: Build the application image
    steps:
      - name: build
        type: container
        action: build
        provider: docker
        with:
          engine: buildx
          context: app
          dockerfile: Dockerfile
          tags:
            - "example.invalid/demo:sha-test"
          driver:
            name: atmos-native-ci
            provider: docker-container
            opts:
              image: mirror.gcr.io/moby/buildkit:buildx-stable-1
          cache:
            from:
              - type: registry
                ref: "example.invalid/demo:buildcache"
            to:
              - type: registry
                ref: "example.invalid/demo:buildcache"
                mode: max
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tempDir)
	configInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:      tempDir,
		AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
	}
	cfg, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)

	require.Len(t, cfg.Commands, 1)
	require.Len(t, cfg.Commands[0].Steps, 1)
	step := cfg.Commands[0].Steps[0]

	require.NotNil(t, step.Build,
		"with: block must decode into the typed Build struct, not be silently dropped")
	assert.Equal(t, "buildx", step.Build.Engine)
	assert.Equal(t, "app", step.Build.Context)
	assert.Equal(t, "Dockerfile", step.Build.Dockerfile)
	assert.Equal(t, []string{"example.invalid/demo:sha-test"}, step.Build.Tags)
	require.NotNil(t, step.Build.Driver)
	assert.Equal(t, "atmos-native-ci", step.Build.Driver.Name)
	assert.Equal(t, "docker-container", step.Build.Driver.Provider)
	assert.Equal(t, "mirror.gcr.io/moby/buildkit:buildx-stable-1", step.Build.Driver.Opts["image"])
	require.NotNil(t, step.Build.Cache)
	require.Len(t, step.Build.Cache.From, 1)
	assert.Equal(t, "example.invalid/demo:buildcache", step.Build.Cache.From[0]["ref"])
	require.Len(t, step.Build.Cache.To, 1)
	assert.Equal(t, "max", step.Build.Cache.To[0]["mode"])
}
