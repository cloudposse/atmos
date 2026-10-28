package bootstrap

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAutomaticSelectionInstallsAndRestoresLockedRelease(t *testing.T) {
	t.Parallel()
	atmos, requests := automaticFixture(t)
	declaration := atmos.Plugins.Packages["fixture"]
	before, err := os.ReadFile(filepath.Join(atmos.BasePathConfigDir, "atmos.yaml"))
	require.NoError(t, err)
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.NotEmpty(t, state.Packages["fixture"].Path)
	require.Positive(t, requests.Load())
	locked, err := os.ReadFile(pluginlock.Path(atmos.BasePathConfigDir))
	require.NoError(t, err)
	after, err := os.ReadFile(filepath.Join(atmos.BasePathConfigDir, "atmos.yaml"))
	require.NoError(t, err)
	require.Equal(t, before, after, "automatic installation must not rewrite declarations")
	require.Equal(t, declaration, atmos.Plugins.Packages["fixture"])

	requests.Store(0)
	atmos.Plugins.Policy.Offline = true
	_, err = selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.Zero(t, requests.Load(), "verified cached startup must not access a registry")

	atmos.Plugins.CacheDir = t.TempDir()
	_, err = selectedPackages(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	require.Zero(t, requests.Load())
	atmos.Plugins.Policy.Offline = false
	_, err = selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.Positive(t, requests.Load())
	restored, err := os.ReadFile(pluginlock.Path(atmos.BasePathConfigDir))
	require.NoError(t, err)
	require.Equal(t, locked, restored, "restoration must not rewrite the existing lock")
}

func TestAutomaticSelectionRejectsDriftAndTampering(t *testing.T) {
	t.Parallel()
	atmos, requests := automaticFixture(t)
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	requests.Store(0)
	declaration := atmos.Plugins.Packages["fixture"]
	declaration.Version = "^2.0.0"
	atmos.Plugins.Packages["fixture"] = declaration
	_, err = selectedPackages(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginFrozenDrift)
	require.Zero(t, requests.Load(), "ordinary use must not resolve an upgrade")
	declaration.Version = "^1.0.0"
	atmos.Plugins.Packages["fixture"] = declaration
	require.NoError(t, os.WriteFile(filepath.Join(state.Packages["fixture"].Path, "bin", "plugin"), []byte("tampered"), 0o700))
	// A corrupt cache is treated like a missing one: an online command restores it by its locked digest.
	repaired, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.Positive(t, requests.Load(), "restoration contacts the registry")
	payload, err := os.ReadFile(filepath.Join(repaired.Packages["fixture"].Path, "bin", "plugin"))
	require.NoError(t, err)
	require.Equal(t, "static fixture; never executed", string(payload))
	_, err = os.Stat(pluginlock.Path(atmos.BasePathConfigDir))
	require.NoError(t, err)
}

func TestCommandSelectionRestoresOnlyRequestedAlias(t *testing.T) {
	t.Parallel()
	atmos, _ := automaticFixture(t)
	atmos.Plugins.Packages["unrelated"] = schema.PluginPackage{Source: "oci://127.0.0.1:1/absent/plugin", Version: "^1.0.0"}
	activation, err := commandSelection(t.Context(), atmos, "fixture")
	require.NoError(t, err)
	require.Equal(t, "about", activation.Name)
	locked, err := pluginlock.Load(atmos.BasePathConfigDir)
	require.NoError(t, err)
	require.Contains(t, locked.Spec.Packages, "fixture")
	require.NotContains(t, locked.Spec.Packages, "unrelated")
	require.Len(t, atmos.Plugins.Packages, 2, "selection must not mutate the caller's configuration")
}

func TestAutomaticSelectionSkipsDisabledAndPathPackages(t *testing.T) {
	t.Parallel()
	atmos, requests := automaticFixture(t)
	disabled := false
	declaration := atmos.Plugins.Packages["fixture"]
	declaration.Enabled = &disabled
	atmos.Plugins.Packages["fixture"] = declaration
	atmos.Plugins.Policy.RequireSignature = true
	atmos.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.Empty(t, state.Packages)
	atmos.Plugins.Policy.RequireSignature = false
	atmos.Plugins.Policy.TrustedPublishers = nil
	declaration.Enabled = nil
	atmos.Plugins.Packages["fixture"] = declaration
	local := localPackageFixture(t)
	declaration.Path = local
	atmos.Plugins.Packages["fixture"] = declaration
	state, err = selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.Equal(t, local, state.Packages["fixture"].Path)
	require.True(t, state.Packages["fixture"].Local)
	require.Zero(t, requests.Load())
	_, err = os.Stat(pluginlock.Path(atmos.BasePathConfigDir))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestAutomaticSelectionConcurrentFirstUseAndCancellation(t *testing.T) {
	t.Parallel()
	atmos, requests := automaticFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := selectedPackages(ctx, atmos)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, requests.Load())
	_, err = os.Stat(pluginlock.Path(atmos.BasePathConfigDir))
	require.ErrorIs(t, err, os.ErrNotExist)

	results := make(chan error, 4)
	for range cap(results) {
		go func() {
			_, err := selectedPackages(t.Context(), atmos)
			results <- err
		}()
	}
	for range cap(results) {
		require.NoError(t, <-results)
	}
	locked, err := pluginlock.Load(atmos.BasePathConfigDir)
	require.NoError(t, err)
	require.Len(t, locked.Spec.Packages, 1)
	require.Equal(t, "1.0.0", locked.Spec.Packages["fixture"].Version)
}

func TestAutomaticSelectionIgnoresMovedTag(t *testing.T) {
	t.Parallel()
	atmos, _ := automaticFixture(t)
	_, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("replacement release"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	descriptors, err := Descriptors()
	require.NoError(t, err)
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	repository, err := name.NewRepository(strings.TrimPrefix(atmos.Plugins.Packages["fixture"].Source, "oci://"))
	require.NoError(t, err)
	require.NoError(t, atmosoci.NewClient(nil).WriteIndex(t.Context(), repository.Tag("1.0.0"), index))
	atmos.Plugins.CacheDir = t.TempDir()
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	payload, err := os.ReadFile(filepath.Join(state.Packages["fixture"].Path, "bin", "plugin"))
	require.NoError(t, err)
	require.Equal(t, "static fixture; never executed", string(payload))
}

// localPackageFixture writes a plugin directory that a path package can select and returns it.
func localPackageFixture(t *testing.T) string {
	executable := "bin/plugin"
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, filepath.Dir(executable)), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, executable), []byte("static fixture; never executed"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	return directory
}

// declareLocal makes the fixture package run from directory, as a profile that sets path would.
func declareLocal(atmos *schema.AtmosConfiguration, alias, directory string) {
	declaration := atmos.Plugins.Packages[alias]
	declaration.Path = directory
	atmos.Plugins.Packages[alias] = declaration
}

func automaticFixture(t *testing.T) (*schema.AtmosConfiguration, *atomic.Int64) {
	t.Helper()
	project, directory := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("# preserve project configuration\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("static fixture; never executed"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	descriptors, err := Descriptors()
	require.NoError(t, err)
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	var requests atomic.Int64
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	source := strings.TrimPrefix(server.URL, "http://") + "/example/plugin"
	repository, err := name.NewRepository(source)
	require.NoError(t, err)
	require.NoError(t, atmosoci.NewClient(nil).WriteIndex(t.Context(), repository.Tag("1.0.0"), index))
	requests.Store(0)
	atmos := &schema.AtmosConfiguration{BasePathConfigDir: project}
	atmos.Plugins.CacheDir = t.TempDir()
	atmos.Plugins.Packages = map[string]schema.PluginPackage{"fixture": {Source: "oci://" + source, Version: "^1.0.0"}}
	return atmos, &requests
}

const automaticManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: example/automatic
  version: 1.0.0
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    PLATFORM: {executable: bin/plugin}
  capabilities:
    commands:
      about: {description: Static automatic-install fixture}
`
