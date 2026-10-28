package cmd

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"
	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
)

func TestPluginCatalogMaintenanceUXAndLocalValidation(t *testing.T) {
	config, directory := builtPluginProject(t)
	var requests atomic.Int64
	handler := containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	repositoryRef := "oci://" + host + "/plugins/oidc"
	catalogRef := "oci://" + host + "/catalog:latest"
	layout := filepath.Join(config.BasePath, "layout")
	output := capturePluginUI(t)
	run := func(args ...string) string {
		t.Helper()
		_, err := invokePluginCoverage(t, config, args...)
		require.NoError(t, err, "%v", args)
		text := ansi.Strip(output.String())
		output.Reset()
		return text
	}
	run("pack", directory, "--output", layout)
	run("registry", "add", "local", catalogRef)
	reloadPluginCoverageConfig(t, config)
	run("publish", layout, "--to", strings.TrimPrefix(repositoryRef, "oci://"))

	t.Run("add and publish report what happened", func(t *testing.T) {
		text := run("catalog", "add", "local", repositoryRef+":0.1.0")
		assert.Contains(t, text, "✓")
		assert.Contains(t, text, "example/oidc")
		assert.Contains(t, text, "0.1.0")
		catalogPath := filepath.Join(config.BasePath, "catalog.yaml")
		text = run("catalog", "build", layout, "--source", repositoryRef, "--output", catalogPath, "--name=corporate")
		assert.Contains(t, text, "Built catalog corporate")
		assert.Contains(t, text, catalogPath)
		text = run("catalog", "publish", catalogPath, "--to="+catalogRef+"-built")
		assert.Contains(t, text, "Published catalog corporate")
	})

	t.Run("untagged release references fail locally with a hint", func(t *testing.T) {
		before := requests.Load()
		_, err := invokePluginCoverage(t, config, "catalog", "add", "local", repositoryRef)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		assert.Contains(t, formatPluginError(err), "tag or digest")
		assert.NotContains(t, formatPluginError(err), "latest")
		assert.Equal(t, before, requests.Load(), "no registry request may be made for a reference that is invalid locally")
	})

	t.Run("a catalog argument without oci:// lists the configured aliases", func(t *testing.T) {
		before := requests.Load()
		_, err := invokePluginCoverage(t, config, "catalog", "add", host+"/catalog:latest", repositoryRef+":0.1.0")
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		rendered := formatPluginError(err)
		assert.Contains(t, rendered, "oci://")
		assert.Contains(t, rendered, "local")
		assert.Equal(t, before, requests.Load())
		_, err = invokePluginCoverage(t, config, "catalog", "publish", filepath.Join(config.BasePath, "catalog.yaml"), "--to=nonsense")
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		assert.Contains(t, formatPluginError(err), "local")
	})

	t.Run("versions match with or without a leading v and misses list what exists", func(t *testing.T) {
		_, err := invokePluginCoverage(t, config, "catalog", "remove", "local", "example/oidc", "--version=v9.9.9")
		require.ErrorIs(t, err, errUtils.ErrPluginCatalogVersionNotFound)
		assert.Contains(t, err.Error(), "available: 0.1.0")
		_, err = invokePluginCoverage(t, config, "catalog", "remove", "local", "example/absent")
		require.ErrorIs(t, err, errUtils.ErrPluginCatalogEntryNotFound)
		assert.Contains(t, formatPluginError(err), "example/oidc")
		text := run("catalog", "remove", "local", "example/oidc", "--version=v0.1.0")
		assert.Contains(t, text, "Unlisted example/oidc version 0.1.0")
	})
}

func TestPluginCatalogBuildPreservesOutputOnFailure(t *testing.T) {
	config := pluginCoverageProject(t)
	layout, _ := pluginSigningLayout(t, config)
	emptyLayout := filepath.Join(config.BasePath, "empty-layout")
	require.NoError(t, atmosoci.SaveIndex(emptyLayout, mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: empty.Image})))
	for _, tc := range []struct {
		name, layout, source, catalogName string
	}{
		{"not a plugin release", emptyLayout, "oci://registry.example.com/plugins/example", "plugins"},
		{"tagged source", layout, "oci://registry.example.com/plugins/example:latest", "plugins"},
		{"empty catalog name", layout, "oci://registry.example.com/plugins/example", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "catalog.yaml")
			require.NoError(t, os.WriteFile(output, []byte("existing catalog"), 0o600))
			_, err := invokePluginCoverage(t, config, "catalog", "build", tc.layout, "--source="+tc.source, "--name="+tc.catalogName, "--output="+output)
			require.Error(t, err)
			contents, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Equal(t, "existing catalog", string(contents))
		})
	}
	t.Run("unwritable destination", func(t *testing.T) {
		output := t.TempDir()
		_, err := invokePluginCoverage(t, config, "catalog", "build", layout, "--source=oci://registry.example.com/plugins/example", "--output="+output)
		require.Error(t, err)
		require.DirExists(t, output)
	})
}
