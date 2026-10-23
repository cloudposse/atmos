package cmd

import (
	"bytes"
	"debug/buildinfo"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPluginBuildStreamsDiagnosticsAndPreservesPreviousBinary(t *testing.T) {
	_ = NewTestKit(t)
	directory := t.TempDir()
	environment := pluginTestEnvironment(t, directory)
	executable, err := os.Executable()
	require.NoError(t, err)
	for name, content := range map[string]string{
		"plugin.yaml": `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: example/build-output, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  capabilities:
    commands:
      about: {description: Build fixture}
`,
		"go.mod":  "module example.com/build-output\n",
		"main.go": "package main\nfunc main() {}\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600))
	}
	run := func() (string, string, error) {
		t.Helper()
		command := exec.CommandContext(t.Context(), executable, "plugin", "build")
		command.Dir, command.Env = directory, environment
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err
	}
	stdout, stderr, err := run()
	require.NoError(t, err, stderr)
	require.Empty(t, stdout, "build progress belongs on the UI channel")
	require.Contains(t, stderr, "Plugin built successfully")
	destination := filepath.Join(directory, "bin", "plugin")
	previous, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\nfunc main() { missingCompilerSymbol() }\n"), 0o600))
	stdout, stderr, err = run()
	require.Error(t, err)
	require.Empty(t, stdout, "compiler failures must leave stdout clean")
	require.Contains(t, stderr, "undefined: missingCompilerSymbol")
	require.Contains(t, stderr, "Compiling plugin failed")
	require.NotContains(t, stderr, "Plugin built successfully")
	current, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, previous, current, "the previous successful binary remains usable")
}

func TestPluginBuildRejectsConflictingPlatformOptions(t *testing.T) {
	for _, tc := range []struct {
		name, platform, all string
		arguments           []string
	}{
		{name: "both flags", arguments: []string{"--platform=linux/amd64", "--all-platforms"}},
		{name: "platform environment and all flag", platform: "linux/arm64", arguments: []string{"--all-platforms"}},
		{name: "all environment and platform flag", all: "true", arguments: []string{"--platform=linux/amd64"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			t.Setenv("ATMOS_PLUGIN_BUILD_PLATFORM", tc.platform)
			t.Setenv("ATMOS_PLUGIN_BUILD_ALL_PLATFORMS", tc.all)
			arguments := append([]string{"build", t.TempDir()}, tc.arguments...)
			stdout, err := invokePluginCoverage(t, nil, arguments...)
			require.ErrorIs(t, err, errUtils.ErrPluginFlagConflict, "reject conflicts before opening a manifest or starting Go")
			require.Empty(t, stdout)
			require.Contains(t, err.Error(), "--platform and --all-platforms")
		})
	}
}

func TestPluginBuildPlatformSelection(t *testing.T) {
	for _, tc := range []struct {
		name, platform, all string
		arguments           []string
		built               []string
	}{
		{name: "variant environment", platform: "linux/arm/v7", built: []string{"linux-arm-v7"}},
		{name: "flag overrides environment", platform: "linux/arm64", all: "true", arguments: []string{"--platform=linux/amd64", "--all-platforms=false"}, built: []string{"linux-amd64"}},
		{name: "all platforms flag", arguments: []string{"--all-platforms"}, built: []string{"linux-amd64", "linux-arm64", "linux-arm-v7"}},
		{name: "all platforms environment", all: "true", built: []string{"linux-amd64", "linux-arm64", "linux-arm-v7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			t.Setenv("ATMOS_PLUGIN_BUILD_PLATFORM", tc.platform)
			t.Setenv("ATMOS_PLUGIN_BUILD_ALL_PLATFORMS", tc.all)
			// Ambient compiler targets must never redirect the declared build output.
			t.Setenv("GOOS", "windows")
			t.Setenv("GOARCH", "arm64")
			t.Setenv("GOWORK", "off")
			directory := pluginBuildPlatformFixture(t)
			output := capturePluginUI(t)
			arguments := append([]string{"build", directory}, tc.arguments...)
			stdout, err := invokePluginCoverage(t, nil, arguments...)
			require.NoError(t, err)
			require.Empty(t, stdout, "each success is reported on the UI channel")
			for file, architecture := range map[string]string{"linux-amd64": "amd64", "linux-arm64": "arm64", "linux-arm-v7": "arm"} {
				path := filepath.Join(directory, "bin", file)
				if !slices.Contains(tc.built, file) {
					require.NoFileExists(t, path)
					continue
				}
				metadata, err := buildinfo.ReadFile(path)
				require.NoError(t, err)
				settings := make(map[string]string)
				for _, setting := range metadata.Settings {
					settings[setting.Key] = setting.Value
				}
				require.Equal(t, "linux", settings["GOOS"])
				require.Equal(t, architecture, settings["GOARCH"])
				if architecture == "arm" {
					require.Equal(t, "7,softfloat", settings["GOARM"])
				}
				require.Contains(t, output.String(), file, "show every successful executable path")
			}
		})
	}
}

func pluginBuildPlatformFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for name, content := range map[string]string{
		"plugin.yaml": `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: example/build-platforms, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    linux/amd64: {executable: bin/linux-amd64}
    linux/arm64: {executable: bin/linux-arm64}
    linux/arm/v7: {executable: bin/linux-arm-v7}
  capabilities:
    commands:
      about: {description: Build fixture}
`,
		"go.mod":  "module example.com/build-platforms\n",
		"main.go": "package main\nfunc main() { panic(\"building must not execute plugin code\") }\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600))
	}
	return directory
}

func TestPluginBuildReportsCompletedPlatformsOnLaterFailure(t *testing.T) {
	_ = NewTestKit(t)
	t.Setenv("ATMOS_PLUGIN_BUILD_PLATFORM", "")
	t.Setenv("ATMOS_PLUGIN_BUILD_ALL_PLATFORMS", "false")
	t.Setenv("GOWORK", "off")
	directory := pluginBuildPlatformFixture(t)
	// Go selects this file only for the last target in lexical platform order.
	require.NoError(t, os.WriteFile(filepath.Join(directory, "invalid_arm64.go"), []byte("package main\nvar _ = missingCompilerSymbol\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o755))
	previous := []byte("previous ARM64 build")
	failedOutput := filepath.Join(directory, "bin", "linux-arm64")
	require.NoError(t, os.WriteFile(failedOutput, previous, 0o600))
	output := capturePluginUI(t)
	stdout, err := invokePluginCoverage(t, nil, "build", directory, "--all-platforms")
	require.ErrorIs(t, err, errUtils.ErrPluginBuild)
	require.Empty(t, stdout)
	for _, completed := range []string{"linux-amd64", "linux-arm-v7"} {
		require.FileExists(t, filepath.Join(directory, "bin", completed))
		require.Contains(t, output.String(), completed)
	}
	require.Equal(t, 2, strings.Count(output.String(), "Plugin built successfully"))
	current, err := os.ReadFile(failedOutput)
	require.NoError(t, err)
	require.Equal(t, previous, current)
}
