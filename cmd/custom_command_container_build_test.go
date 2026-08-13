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
	assert.Contains(t, fields, "-t", "configured tag must be applied")
	assert.Contains(t, fields, "example.invalid/demo:sha-test")
	assert.Contains(t, fields, "-f", "configured Dockerfile must be applied")
	// context/dockerfile are relative paths in the with: block, resolved
	// against the step's working directory (#2880) into absolute paths --
	// docker still receives the same file, just no longer as a bare relative
	// string a differing subprocess cwd could silently misresolve.
	assert.Contains(t, fields, filepath.Join(appDir, "Dockerfile"), "configured Dockerfile must be applied")
	assert.Contains(t, fields, appDir, "configured context must be applied")

	// The driver: block must provision a real Buildx builder before the build
	// runs (pkg/container/docker.go's ensureBuilder calls `docker buildx
	// create` as a separate invocation), and the cache entries must reach
	// docker as the exact configured reference/mode, not merely as a bare
	// `--cache-from`/`--cache-to` flag with an unchecked value. The fake
	// runtime already records every invocation unconditionally, so both are
	// present in the same recorded args log used above.
	var createLine string
	for _, line := range lines {
		createFields := strings.Split(line, "\t")
		if len(createFields) > 1 && createFields[0] == "buildx" && createFields[1] == "create" {
			createLine = line
			break
		}
	}
	require.NotEmpty(t, createLine,
		"expected a `docker buildx create ...` invocation provisioning the configured driver; got invocations: %v", lines)
	createFields := strings.Split(createLine, "\t")

	flagValueCases := []struct {
		name   string
		fields []string
		flag   string
		want   string
	}{
		{"builder uses configured driver provider", createFields, "--driver", "docker-container"},
		{"builder uses configured driver image opt", createFields, "--driver-opt", "image=mirror.gcr.io/moby/buildkit:buildx-stable-1"},
		{"cache-from carries the configured ref", fields, "--cache-from", "ref=example.invalid/demo:buildcache,type=registry"},
		{"cache-to carries the configured ref and mode=max", fields, "--cache-to", "mode=max,ref=example.invalid/demo:buildcache,type=registry"},
	}
	for _, tc := range flagValueCases {
		t.Run(tc.name, func(t *testing.T) {
			assertFlagValue(t, tc.fields, tc.flag, tc.want)
		})
	}
	assert.Contains(t, createFields, "atmos-native-ci", "builder create must use the configured driver name")

	// The exact bug report's symptom: Atmos must not fall back to a bare,
	// unconfigured `docker build -f Dockerfile .`.
	for _, line := range lines {
		assert.NotEqual(t, "build\t-f\tDockerfile\t.", line,
			"must not silently fall back to a bare, unconfigured docker build")
	}
}

// assertFlagValue asserts fields contains flag immediately followed by want,
// so a flag's actual configured value is checked rather than merely its
// presence somewhere in the argv.
func assertFlagValue(t *testing.T, fields []string, flag, want string) {
	t.Helper()

	for i, field := range fields {
		if field == flag {
			if i+1 >= len(fields) {
				t.Errorf("flag %q has no following value in %v", flag, fields)
				return
			}
			assert.Equal(t, want, fields[i+1], "%s value", flag)
			return
		}
	}
	t.Errorf("expected flag %q not found in %v", flag, fields)
}
