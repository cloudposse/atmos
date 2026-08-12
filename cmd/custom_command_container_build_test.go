package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestCustomCommandContainerBuildPassesWithBlockToDocker reproduces
// https://github.com/cloudposse/atmos/issues/2876 end to end: a
// `.atmos.d/commands.yaml`-style custom command with a `type: container,
// action: build` step and a `with:` block must invoke docker with the full
// configured Buildx engine, builder, cache, tags, dockerfile, and context --
// not silently fall back to `docker build -f Dockerfile .`.
//
// This writes real config to disk, loads it through cfg.InitCliConfig (the
// same production config-loading path `atmos` itself uses), registers it via
// processCustomCommands, and invokes the resulting custom command
// through RootCmd.Execute() exactly as a user would from the shell. A fake,
// logging `docker` executable on PATH (testhelpers.InstallFakeContainerRuntime)
// captures the real argv Atmos emits, so this exercises the real command
// executor rather than manually constructing schema.Task/WorkflowStep/
// ContainerBuildStep values in Go.
func TestCustomCommandContainerBuildPassesWithBlockToDocker(t *testing.T) {
	_ = NewTestKit(t)

	tempDir := t.TempDir()
	appDir := filepath.Join(tempDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "Dockerfile"), []byte("FROM scratch\n"), 0o644))

	atmosYAML := `
base_path: "."
commands:
  - name: test-container-build-with-block
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

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
	t.Setenv("ATMOS_BASE_PATH", tempDir)
	t.Chdir(tempDir)

	argsPath := filepath.Join(t.TempDir(), "docker-args.log")
	t.Setenv("ATMOS_FAKE_RUNTIME_ARGS_FILE", argsPath)
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: "docker",
		Mode: testhelpers.FakeContainerRuntimeStep,
	})

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd))

	RootCmd.SetArgs([]string{"test-container-build-with-block"})
	require.NoError(t, RootCmd.Execute())

	content, err := os.ReadFile(argsPath)
	require.NoError(t, err, "the fake docker executable must have been invoked at least once")
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")

	var buildLine string
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) > 1 && fields[0] == "buildx" && fields[1] == "build" {
			buildLine = line
			break
		}
	}
	require.NotEmpty(t, buildLine,
		"expected a `docker buildx build ...` invocation; got invocations: %v", lines)

	fields := strings.Split(buildLine, "\t")
	assert.Contains(t, fields, "--builder", "configured Buildx driver must be applied")
	assert.Contains(t, fields, "atmos-native-ci")
	assert.Contains(t, fields, "--cache-from", "configured registry cache-from must be applied")
	assert.Contains(t, fields, "--cache-to", "configured registry cache-to must be applied")
	assert.Contains(t, fields, "-t", "configured tag must be applied")
	assert.Contains(t, fields, "example.invalid/demo:sha-test")
	assert.Contains(t, fields, "-f", "configured Dockerfile must be applied")
	// context/dockerfile are relative paths in the with: block, resolved
	// against the step's working directory (#2880) into absolute paths --
	// docker still receives the same file, just no longer as a bare relative
	// string a differing subprocess cwd could silently misresolve.
	assert.Contains(t, fields, filepath.Join(appDir, "Dockerfile"), "configured Dockerfile must be applied")
	assert.Contains(t, fields, appDir, "configured context must be applied")

	// The exact bug report's symptom: Atmos must not fall back to a bare,
	// unconfigured `docker build -f Dockerfile .`.
	for _, line := range lines {
		assert.NotEqual(t, "build\t-f\tDockerfile\t.", line,
			"must not silently fall back to a bare, unconfigured docker build")
	}
}
