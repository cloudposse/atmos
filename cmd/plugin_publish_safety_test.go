package cmd

import (
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// pluginNativeProject scaffolds a plugin with a native header matching the host
// platform declared by the scaffold, so pack accepts it without compiling anything.
func pluginNativeProject(t *testing.T) (*schema.AtmosConfiguration, string) {
	t.Helper()
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "plugin")
	_, err := invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-version=v0.0.0")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, pluginManifestExecutable(t, directory)), ocitest.Binary(runtime.GOOS, runtime.GOARCH), 0o700))
	return config, directory
}

func pluginManifestExecutable(t *testing.T, directory string) string {
	t.Helper()
	definition, err := manifest.Load(directory)
	require.NoError(t, err)
	return definition.Spec.Runtime.Executable
}

func TestPluginPackRejectsNonNativeExecutableAndListsFiles(t *testing.T) {
	config, directory := pluginNativeProject(t)
	output, err := invokePluginCoverage(t, config, "pack", directory, "--output", filepath.Join(config.BasePath, "layout"), "--format=json")
	require.NoError(t, err)
	var packed struct {
		Files []pluginoci.BundleFile `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &packed))
	paths := make([]string, 0, len(packed.Files))
	for _, file := range packed.Files {
		require.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, file.Platform)
		paths = append(paths, file.Path)
	}
	require.Equal(t, pluginManifestExecutable(t, directory), paths[0])
	require.Equal(t, "plugin.yaml", paths[1])
	require.NotContains(t, strings.Join(paths, ","), ".git")

	require.NoError(t, os.WriteFile(filepath.Join(directory, pluginManifestExecutable(t, directory)), []byte("#!/bin/sh\n"), 0o700))
	rejected := filepath.Join(config.BasePath, "rejected")
	_, err = invokePluginCoverage(t, config, "pack", directory, "--output", rejected)
	require.ErrorIs(t, err, errUtils.ErrPluginBundleExecutableInvalid)
	require.NoDirExists(t, rejected)
}

func TestPluginValidateAppliesPackagingGuards(t *testing.T) {
	config, directory := pluginNativeProject(t)
	_, err := invokePluginCoverage(t, config, "validate", directory)
	require.NoError(t, err)
	executable := pluginManifestExecutable(t, directory)
	require.NoError(t, os.Remove(filepath.Join(directory, executable)))
	_, err = invokePluginCoverage(t, config, "validate", directory)
	require.NoError(t, err, "a missing build output is not a validation failure")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", ".credentials.json"), []byte("{}"), 0o600))
	_, err = invokePluginCoverage(t, config, "validate", directory)
	require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
	require.NoError(t, os.Remove(filepath.Join(directory, "schemas", ".credentials.json")))
	manifestPath := filepath.Join(directory, "plugin.yaml")
	raw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, []byte(strings.ReplaceAll(string(raw), "executable: "+executable, "executable: .git/config")), 0o600))
	_, err = invokePluginCoverage(t, config, "validate", directory)
	require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
}

func TestPluginPublishPreflightRunsBeforeAnyWork(t *testing.T) {
	config := pluginCoverageProject(t)
	// The layout does not exist: every rejection below must come from local pre-flight checks.
	tests := []struct {
		name      string
		arguments []string
		want      error
		contains  string
	}{
		{"destination with tag", []string{"--to=oci://example.com/acme/plugin:1.0.0"}, errUtils.ErrPluginInvalidConfig, "includes a tag or digest"},
		{"destination with digest", []string{"--to=example.com/acme/plugin@sha256:" + strings.Repeat("a", 64)}, errUtils.ErrPluginInvalidConfig, "includes a tag or digest"},
		{"cosign version without sign", []string{"--to=oci://example.com/acme/plugin", "--cosign-version=3.0.0"}, errUtils.ErrPluginFlagConflict, "--cosign-version"},
		{"yes without sign", []string{"--to=oci://example.com/acme/plugin", "--yes"}, errUtils.ErrPluginFlagConflict, "--yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := invokePluginCoverage(t, config, append([]string{"publish", "missing-layout"}, tt.arguments...)...)
			require.ErrorIs(t, err, tt.want)
			require.Contains(t, err.Error(), tt.contains)
			require.NotEmpty(t, errUtils.AllHints(err))
		})
	}
}

func TestPluginDeleteRequiresConfirmationAndReportsSuccess(t *testing.T) {
	config, directory := pluginNativeProject(t)
	layout := filepath.Join(config.BasePath, "layout")
	_, err := invokePluginCoverage(t, config, "pack", directory, "--output", layout)
	require.NoError(t, err)
	index, err := atmosoci.LoadIndex(layout, "")
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	server := httptest.NewServer(containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	repository := strings.TrimPrefix(server.URL, "http://") + "/plugins/oidc"
	_, err = invokePluginCoverage(t, config, "publish", layout, "--to", repository)
	require.NoError(t, err)
	reference, err := name.NewDigest(repository + "@" + release.Digest)
	require.NoError(t, err)
	client := atmosoci.NewClient(config)
	_, err = client.Head(t.Context(), reference)
	require.NoError(t, err)

	_, err = invokePluginCoverage(t, config, "delete", reference.Name())
	require.ErrorIs(t, err, errUtils.ErrPluginConfirmationRequired)
	require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "--yes")
	_, err = client.Head(t.Context(), reference)
	require.NoError(t, err, "a refused delete must leave the release in place")

	_, err = invokePluginCoverage(t, config, "delete", "oci://"+reference.Name(), "--yes")
	require.NoError(t, err)
	_, err = client.Head(t.Context(), reference)
	require.True(t, atmosoci.IsNotFound(err), "release should be gone: %v", err)

	for _, invalid := range []string{repository + ":1.0.0", repository, repository + "@sha256:short"} {
		_, err = invokePluginCoverage(t, config, "delete", invalid, "--yes")
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, invalid)
		require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "@sha256:")
	}
}
