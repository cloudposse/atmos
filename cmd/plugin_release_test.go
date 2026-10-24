package cmd

import (
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
)

func TestPluginReleaseBuildsTestsAndPublishesSource(t *testing.T) {
	_ = NewTestKit(t)
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "plugin")
	_, err = invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-path", filepath.Join(repository, "sdk"))
	require.NoError(t, err)
	server := httptest.NewServer(containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	destination := strings.TrimPrefix(server.URL, "http://") + "/plugins/lifecycle"
	progress := capturePluginUI(t)
	output, err := invokePluginCoverage(t, config, "release", directory, "--to", destination, "--format=json")
	require.NoError(t, err)
	var published map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &published), "stdout remains machine-readable despite compiler and conformance output")
	require.Contains(t, output, "sha256:")
	require.Contains(t, progress.String(), "current host")
	require.FileExists(t, filepath.Join(directory, pluginManifestExecutable(t, directory)))
	require.NoDirExists(t, filepath.Join(directory, ".atmos", "release"), "the CLI owns disposable staging")
	// Test failures must stop the lifecycle before any version is published.
	writePluginFixture(t, directory, wrongStdoutFixture)
	output, err = invokePluginCoverage(t, config, "release", directory, "--to", destination, "--format=json")
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	require.Empty(t, output)
}

func TestPluginReleaseLocalPreflight(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	for _, args := range [][]string{
		{"--to=example.com/plugin:1.0.0"},
		{"--to=example.com/plugin", "--yes"},
		{"--to=example.com/plugin", "--format=unknown"},
	} {
		output, err := invokePluginCoverage(t, config, append([]string{"release", "absent"}, args...)...)
		require.Error(t, err)
		require.Empty(t, output)
	}
	_, err := invokePluginCoverage(t, nil, "release", "--to=example.com/plugin")
	require.Error(t, err)
	_, err = invokePluginCoverage(t, config, "release", filepath.Join(t.TempDir(), "absent"), "--to=example.com/plugin")
	require.Error(t, err)
	config, directory := pluginNativeProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", ".private.json"), []byte("{}"), 0o600))
	_, err = invokePluginCoverage(t, config, "release", directory, "--to=example.com/plugin")
	require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
}

func TestPluginPublishRequiresPublicVisibilityBeforeRegistryWrites(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	// A layout is not opened and the destination is never contacted when the
	// requested visibility preflight cannot establish public GHCR visibility.
	output, err := invokePluginCoverage(t, config, "publish", "missing-layout", "--to=registry.example.com/plugin", "--verify-public")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Contains(t, err.Error(), "visibility preflight requires a GHCR")
	require.Empty(t, output)
}

func TestPluginTestBuildCoverageAndHostGuard(t *testing.T) {
	_ = NewTestKit(t)
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "plugin")
	_, err = invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-path", filepath.Join(repository, "sdk"))
	require.NoError(t, err)
	executable := filepath.Join(directory, pluginManifestExecutable(t, directory))
	output, err := invokePluginCoverage(t, config, "test", directory, "--build", "--platform=unsupported/amd64", "--coverprofile=coverage.out")
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
	require.Empty(t, output)
	require.NoFileExists(t, executable, "foreign platform rejected before invoking compiler or Go tests")
	diagnostics, err := os.CreateTemp(t.TempDir(), "diagnostics")
	require.NoError(t, err)
	previousStderr := os.Stderr
	os.Stderr = diagnostics
	t.Cleanup(func() { os.Stderr = previousStderr; _ = diagnostics.Close() })
	output, err = invokePluginCoverage(t, config, "test", directory, "--build", "--platform="+pluginplatform.Current(), "--coverprofile=coverage.out", "--format=json")
	require.NoError(t, err)
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &report), "compiler/unit coverage output must stay off stdout")
	require.Contains(t, report, "results")
	require.FileExists(t, executable)
	profile, err := os.ReadFile(filepath.Join(directory, "coverage.out"))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(profile), "mode: atomic\n"))
	_, err = diagnostics.Seek(0, 0)
	require.NoError(t, err)
	progress, err := io.ReadAll(diagnostics)
	require.NoError(t, err)
	require.Contains(t, string(progress), "total:")
	require.Contains(t, string(progress), "(statements)")
}
