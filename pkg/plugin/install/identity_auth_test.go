package install

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIdentityBoundRegistryPublishInstallCacheLogout(t *testing.T) {
	fixture, release, directory := installFixture(t)
	t.Setenv("ATMOS_FORCE_TTY", "false")
	t.Setenv("ATMOS_KEYRING_PASSWORD", "registry-test-keyring-password")
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv("PLUGIN_TEST_REGISTRY_PASSWORD", "registry-secret")
	var authenticated atomic.Int32
	registry := containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "corporate-bot" || password != "registry-secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="plugins"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authenticated.Add(1)
		registry.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	project := t.TempDir()
	config := &schema.AtmosConfiguration{CliConfigPath: filepath.Join(project, "atmos.yaml"), Auth: schema.AuthConfig{
		Realm: "registry-identity-test", Keyring: schema.KeyringConfig{Type: "file", Spec: map[string]any{"path": filepath.Join(project, "keyring")}},
		Identities: map[string]schema.Identity{
			"registry-bot": {Kind: "oci/user", Spec: map[string]any{"registry": host, "username": "corporate-bot", "password_env": "PLUGIN_TEST_REGISTRY_PASSWORD"}},
			// This unavailable auth plugin must not block the built-in bootstrap identity.
			"future-auth": {Kind: "not-installed/provider"},
		},
	}, Plugins: schema.PluginsConfig{CacheDir: filepath.Join(project, "cache"), Registries: map[string]schema.PluginRegistry{"corporate": {Catalog: "oci://" + host + "/catalog:stable", Identity: "registry-bot"}}}}
	service, err := NewService(config, fixture.Descriptors)
	require.NoError(t, err)
	layout := filepath.Join(project, "release")
	require.NoError(t, pluginoci.Pack(directory, layout, fixture.Descriptors))
	repository := "oci://" + host + "/plugins/fixture"
	published, err := pluginoci.Publish(t.Context(), service.Registry, layout, repository, fixture.Descriptors)
	require.NoError(t, err)
	require.Equal(t, release.Digest, published.Digest)
	require.Positive(t, authenticated.Load())
	ref, err := name.NewRepository(host + "/plugins/fixture")
	require.NoError(t, err)
	tags, err := service.Registry.Tags(t.Context(), ref)
	require.NoError(t, err)
	require.Equal(t, []string{"1.0.0"}, tags)
	// Another process/client can reuse the Atmos auth session after the initial
	// environment secret is gone; no second registry credential store is involved.
	t.Setenv("PLUGIN_TEST_REGISTRY_PASSWORD", "")
	fresh, err := NewService(config, fixture.Descriptors)
	require.NoError(t, err)
	installed, err := fresh.Prepare(t.Context(), &Request{Source: repository, Constraint: "1.0.0"})
	require.NoError(t, err, "%s", errUtils.Format(err, errUtils.DefaultFormatterConfig()))
	require.Equal(t, release.Digest, installed.IndexDigest)
	require.Equal(t, "acme/fixture", installed.Plugin)
	// An authenticated catalog's identity never follows a package to another host.
	var forwarded atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			forwarded.Store(true)
		}
		registry.ServeHTTP(w, r)
	}))
	t.Cleanup(other.Close)
	otherRef, err := name.NewRepository(strings.TrimPrefix(other.URL, "http://") + "/plugins/fixture")
	require.NoError(t, err)
	_, err = fresh.Registry.Tags(t.Context(), otherRef)
	require.NoError(t, err)
	require.False(t, forwarded.Load(), "bound credentials must never be forwarded to a different registry host")
	require.NoError(t, registryauth.Logout(t.Context(), config, "registry-bot"))
	_, err = fresh.Registry.Tags(t.Context(), ref)
	require.Error(t, err, "already-created clients must stop using credentials after logout")
	// Ambient Docker credentials cannot override a failed explicit identity.
	dockerConfig, err := docker.NewConfigManager()
	require.NoError(t, err)
	require.NoError(t, dockerConfig.WriteAuth(host, "corporate-bot", "registry-secret"))
	afterLogout, err := NewService(config, fixture.Descriptors)
	require.NoError(t, err)
	before := authenticated.Load()
	_, err = afterLogout.Registry.Tags(t.Context(), ref)
	require.Error(t, err)
	require.Equal(t, before, authenticated.Load(), "identity failure must stop before an authenticated registry request")
}
