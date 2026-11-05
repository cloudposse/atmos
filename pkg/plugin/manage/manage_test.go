package manage_test

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInstallFrozenRestoreAndTamperDetection(t *testing.T) {
	manager, source, packageDir := setup(t)
	ctx := t.Context()
	require.NoError(t, manager.Add(ctx, manage.AddOptions{Alias: "fixture", Source: source, Constraint: "^1.0.0"}))
	raw, err := os.ReadFile(filepath.Join(manager.Project, manager.ConfigFile))
	require.NoError(t, err)
	require.Contains(t, string(raw), "# keep this comment")
	require.Contains(t, string(raw), "unrelated: preserved")
	require.NoError(t, yaml.Unmarshal(raw, manager.Config))
	lockedBytes, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	locked, err := pluginlock.Load(manager.Project)
	require.NoError(t, err)
	entry := locked.Spec.Packages["fixture"]
	// Mutate the tag's bytes. Frozen restore must keep using the original digest.
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "bin", "plugin"), []byte("changed executable"), 0o755))
	changed, err := pluginoci.BuildIndex(packageDir, manager.Installer.Descriptors)
	require.NoError(t, err)
	repository, err := name.NewRepository(strings.TrimPrefix(source, "oci://"))
	require.NoError(t, err)
	require.NoError(t, manager.Installer.Registry.WriteIndex(ctx, repository.Tag("1.0.0"), changed))
	manager.Installer.Cache, err = install.NewCache(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, manager.Restore(ctx, manage.RestoreOptions{Frozen: true}))
	after, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	require.Equal(t, lockedBytes, after)
	directory, err := manager.Installer.Cache.Installed(&entry, install.CurrentPlatform())
	require.NoError(t, err)
	binary := filepath.Join(directory, "bin", "plugin")
	actual, err := os.ReadFile(binary)
	require.NoError(t, err)
	require.Equal(t, "precompiled fixture", string(actual))
	require.NoError(t, os.WriteFile(binary, []byte("tampered binary"), 0o755))
	_, err = manager.Installer.Cache.Installed(&entry, install.CurrentPlatform())
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
}

func TestOfflineRestoreAndRequiredSignatureFailClosed(t *testing.T) {
	manager, source, packageDir := setup(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(packageDir, layout, manager.Installer.Descriptors))
	manager.Config.Plugins.Policy.Offline = true
	_, err := manager.Installer.Prepare(t.Context(), &install.Request{Source: source, Constraint: "1.0.0"})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	_, err = manager.Installer.Prepare(t.Context(), &install.Request{Source: source, Constraint: "1.0.0", Layout: layout})
	require.NoError(t, err)
	manager.Config.Plugins.Policy.RequireSignature = true
	manager.Config.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}
	_, err = manager.Installer.Prepare(t.Context(), &install.Request{Source: source, Constraint: "1.0.0", Layout: layout})
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
}

func setup(t *testing.T) (*manage.Manager, string, string) {
	t.Helper()
	project, packageDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("# keep this comment\nunrelated: preserved\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(packageDir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "bin", "plugin"), []byte("precompiled fixture"), 0o755))
	definition := strings.ReplaceAll(fixtureManifest, "PLATFORM", install.CurrentPlatform())
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "plugin.yaml"), []byte(definition), 0o644))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	index, err := pluginoci.BuildIndex(packageDir, descriptors)
	require.NoError(t, err)
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	source := strings.TrimPrefix(server.URL, "http://") + "/acme/plugin"
	repository, err := name.NewRepository(source)
	require.NoError(t, err)
	client := atmosoci.NewClient(nil)
	require.NoError(t, client.WriteIndex(context.Background(), repository.Tag("1.0.0"), index))
	cache, err := install.NewCache(t.TempDir())
	require.NoError(t, err)
	config := &schema.AtmosConfiguration{}
	service := &install.Service{Config: config, Cache: cache, Registry: client, Descriptors: descriptors, Platform: install.CurrentPlatform()}
	return &manage.Manager{Project: project, ConfigFile: "atmos.yaml", Config: config, Installer: service}, "oci://" + source, packageDir
}

const fixtureManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: example/fixture
  version: 1.0.0
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    PLATFORM: {executable: bin/plugin}
  capabilities:
    fixtures:
      example/fixture: {}
`
