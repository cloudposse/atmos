package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func cachedBootstrapAuth(t *testing.T, atmos *schema.AtmosConfiguration) {
	t.Helper()
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("must never execute while registering"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "schemas"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "provider.json"), []byte(`{"type":"object"}`), 0o600))
	source := strings.ReplaceAll(automaticManifest, "example/automatic", "example/bootstrap-auth")
	source = strings.ReplaceAll(source, "PLATFORM", install.CurrentPlatform())
	source = strings.Replace(source, "    commands:\n      about: {description: Static automatic-install fixture}", "    auth:\n      providers:\n        test/bootstrap-provider: {schema: schemas/provider.json, credentials: [oidc/v1]}", 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(source), 0o600))
	registry, err := Descriptors()
	require.NoError(t, err)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, registry))
	service, err := install.NewService(atmos, registry)
	require.NoError(t, err)
	entry, err := service.Prepare(t.Context(), &install.Request{Source: "oci://registry.example.com/bootstrap-auth", Constraint: "1.0.0", Layout: layout})
	require.NoError(t, err)
	file := pluginlock.New()
	file.Spec.Packages["bootstrap-auth"] = *entry
	require.NoError(t, pluginlock.Save(ProjectDirectory(atmos), file))
	atmos.Plugins.Packages["bootstrap-auth"] = schema.PluginPackage{Source: entry.Source, Version: entry.Constraint}
	atmos.Auth.Providers = map[string]schema.Provider{"corporate": {Kind: "test/bootstrap-provider"}}
	atmos.Auth.Identities = map[string]schema.Identity{"reader": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "corporate"}}}
	atmos.Plugins.Registries = map[string]schema.PluginRegistry{"corporate": {Catalog: "oci://registry.example.com/catalog:stable", Identity: "reader"}}
}

func TestAvailableAuthRejectsBrokenLocalStateWithoutFetching(t *testing.T) {
	for _, failure := range []string{"pending transaction", "invalid lock", "corrupt cache", "unavailable keyring", "session directory obstruction"} {
		t.Run(failure, func(t *testing.T) {
			atmos, requests := automaticFixture(t)
			cachedBootstrapAuth(t, atmos)
			var expected error
			switch failure {
			case "pending transaction":
				directory := filepath.Join(ProjectDirectory(atmos), ".atmos", "plugins")
				require.NoError(t, os.MkdirAll(directory, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(directory, "transaction.json"), []byte("[]"), 0o600))
				expected = errUtils.ErrPluginTransactionPending
			case "invalid lock":
				require.NoError(t, os.WriteFile(pluginlock.Path(ProjectDirectory(atmos)), []byte("invalid: ["), 0o600))
			case "corrupt cache":
				file, err := pluginlock.Load(ProjectDirectory(atmos))
				require.NoError(t, err)
				cache, err := install.NewCache(atmos.Plugins.CacheDir)
				require.NoError(t, err)
				entry := file.Spec.Packages["bootstrap-auth"]
				directory, err := cache.Installed(&entry, install.CurrentPlatform())
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("tampered"), 0o700))
				expected = errUtils.ErrPluginCacheCorrupt
			case "unavailable keyring":
				t.Setenv("ATMOS_KEYRING_TYPE", "unsupported")
				expected = credentials.ErrCredentialStore
			case "session directory obstruction":
				obstruction := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(obstruction, []byte("preserve"), 0o600))
				t.Setenv("ATMOS_XDG_CACHE_HOME", obstruction)
			}
			cleanup, err := Prepare(t.Context(), atmos)
			require.Error(t, err)
			if expected != nil {
				require.ErrorIs(t, err, expected)
			}
			require.Nil(t, cleanup)
			require.Zero(t, requests.Load())
			require.False(t, authregistry.HasProvider("test/bootstrap-provider"))
		})
	}
}

func TestAvailableAuthSkipsUncachedReleaseAndUnrelatedLocalCommands(t *testing.T) {
	atmos, requests := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	atmos.Plugins.CacheDir = t.TempDir()
	local := developmentCommandFixture(t)
	atmos.Plugins.Packages["local-command"] = local.Plugins.Packages["fixture"]
	cleanup, err := PrepareAvailable(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.False(t, authregistry.HasProvider("test/bootstrap-provider"))
	require.Zero(t, requests.Load(), "bootstrap must not fetch an uncached auth package")
}

func TestRegistryAuthKindsFollowIdentityChainsAndTerminateCycles(t *testing.T) {
	atmos := &schema.AtmosConfiguration{}
	atmos.Plugins.Registries = map[string]schema.PluginRegistry{"private": {Identity: "first"}, "missing": {Identity: "missing"}}
	atmos.Auth.Providers = map[string]schema.Provider{"provider": {Kind: "test/provider"}}
	atmos.Auth.Identities = map[string]schema.Identity{
		"first":  {Kind: "test/first", Via: &schema.IdentityVia{Identity: "second"}},
		"second": {Kind: "test/second", Provider: "provider", Via: &schema.IdentityVia{Identity: "first"}},
	}
	require.Equal(t, map[string]bool{"test/first": true, "test/second": true, "test/provider": true}, registryAuthKinds(atmos))
}

func TestAvailableAuthRegistersLocalProvidersWithEmbedderInteraction(t *testing.T) {
	atmos, requests := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	require.NoError(t, err)
	cache, err := install.NewCache(atmos.Plugins.CacheDir)
	require.NoError(t, err)
	entry := file.Spec.Packages["bootstrap-auth"]
	directory, err := cache.Installed(&entry, install.CurrentPlatform())
	require.NoError(t, err)
	atmos.Plugins.Packages["bootstrap-auth"] = schema.PluginPackage{Path: directory}
	browser := &api.LocalBrowser{Interactive: true}
	configured := false
	cleanup, err := PrepareAvailable(t.Context(), atmos, WithInteraction(browser, func() bool { return true }), func(services *adapter.Services) {
		configured = true
		require.Same(t, browser, services.Browser)
		require.True(t, services.Interactive())
	})
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.True(t, configured)
	require.True(t, authregistry.HasProvider("test/bootstrap-provider"))
	require.Zero(t, requests.Load())
}

func TestAvailableAuthRejectsInvalidLocalPackage(t *testing.T) {
	for _, source := range []string{"invalid: [", strings.ReplaceAll(automaticManifest, "commands:", "unsupported:")} {
		t.Run(source, func(t *testing.T) {
			atmos := developmentCommandFixture(t)
			atmos.Auth.Identities = map[string]schema.Identity{"reader": {Kind: "test/missing-identity"}}
			atmos.Plugins.Registries = map[string]schema.PluginRegistry{"private": {Identity: "reader"}}
			require.NoError(t, os.WriteFile(filepath.Join(atmos.Plugins.Packages["fixture"].Path, "plugin.yaml"), []byte(source), 0o600))
			cleanup, err := PrepareAvailable(t.Context(), atmos)
			require.Error(t, err)
			require.Nil(t, cleanup)
		})
	}
}

func TestAvailableAuthEnforcesCurrentPolicyWithoutFetching(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "cached release", true: "local build"}[local], func(t *testing.T) {
			atmos, requests := automaticFixture(t)
			cachedBootstrapAuth(t, atmos)
			if local {
				file, err := pluginlock.Load(ProjectDirectory(atmos))
				require.NoError(t, err)
				cache, err := install.NewCache(atmos.Plugins.CacheDir)
				require.NoError(t, err)
				entry := file.Spec.Packages["bootstrap-auth"]
				directory, err := cache.Installed(&entry, install.CurrentPlatform())
				require.NoError(t, err)
				atmos.Plugins.Packages["bootstrap-auth"] = schema.PluginPackage{Path: directory}
			}
			bootstrapAdmissionPolicy(t, "cel", "true")
			cleanup, err := PrepareAvailable(t.Context(), atmos)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Nil(t, cleanup)
			require.False(t, authregistry.HasProvider("test/bootstrap-provider"))
			require.Zero(t, requests.Load())
		})
	}
}

func TestAvailableAuthRequiresOfflineSignatureEvidence(t *testing.T) {
	atmos, requests := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	atmos.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "example", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}
	before, err := os.ReadFile(pluginlock.Path(ProjectDirectory(atmos)))
	require.NoError(t, err)
	cleanup, err := PrepareAvailable(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Nil(t, cleanup)
	require.False(t, authregistry.HasProvider("test/bootstrap-provider"))
	require.Zero(t, requests.Load())
	after, err := os.ReadFile(pluginlock.Path(ProjectDirectory(atmos)))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestAvailableAuthRegistrationDoesNotRestoreMissingPackages(t *testing.T) {
	atmos, requests := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	cleanup, err := PrepareAvailable(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Zero(t, requests.Load(), "preparing auth must never download an unavailable plugin")
	provider, err := authregistry.NewProvider("corporate", &schema.Provider{Kind: "test/bootstrap-provider"})
	require.NoError(t, err)
	require.NotNil(t, provider)
	cleanup()
	_, err = authregistry.NewProvider("corporate", &schema.Provider{Kind: "test/bootstrap-provider"})
	require.ErrorIs(t, err, errUtils.ErrInvalidProviderKind)
}

func TestPrepareRegistersCachedAuthBeforeRestoringOtherPlugins(t *testing.T) {
	atmos, _ := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	// A kind that no locked export provides makes the unlocked package a needed fallback.
	atmos.Auth.Providers["other"] = schema.Provider{Kind: "test/unsatisfied-kind"}
	declaration := atmos.Plugins.Packages["fixture"]
	target, err := url.Parse(strings.Replace(declaration.Source, "oci://", "http://", 1))
	require.NoError(t, err)
	target.Path = ""
	proxy := httputil.NewSingleHostReverseProxy(target)
	var observed, missing atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Store(true)
		if _, err := authregistry.NewProvider("corporate", &schema.Provider{Kind: "test/bootstrap-provider"}); err != nil {
			missing.Store(true)
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	original, _ := url.Parse(strings.Replace(declaration.Source, "oci://", "http://", 1))
	declaration.Source = "oci://" + strings.TrimPrefix(server.URL, "http://") + original.Path
	atmos.Plugins.Packages["fixture"] = declaration
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.True(t, observed.Load())
	require.False(t, missing.Load(), "verified local auth constructors must be registered before registry requests")
	provider, err := authregistry.NewProvider("corporate", &schema.Provider{Kind: "test/bootstrap-provider"})
	require.NoError(t, err)
	require.NotNil(t, provider, "the final batch must replace, not collide with or lose, the temporary auth batch")
	cleanup()
	_, err = authregistry.NewProvider("corporate", &schema.Provider{Kind: "test/bootstrap-provider"})
	require.ErrorIs(t, err, errUtils.ErrInvalidProviderKind)
}

func TestAvailableAuthIgnoresDisabledAndUnrelatedPathPackages(t *testing.T) {
	atmos, _ := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	disabled := false
	atmos.Plugins.Packages["disabled"] = schema.PluginPackage{Enabled: &disabled, Path: filepath.Join(t.TempDir(), "missing")}
	first, err := PrepareAvailable(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(first)
	second, err := PrepareAvailable(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(second)
	first()
	require.True(t, authregistry.HasProvider("test/bootstrap-provider"), "overlapping consumers must retain the shared registration")
	second()
	require.False(t, authregistry.HasProvider("test/bootstrap-provider"))
	// Built-in-only registry auth needs no plugin state, even if an unrelated
	// path package points at a build that does not exist yet.
	require.True(t, authregistry.HasIdentity("aws/user"), "the fixture must use a registered built-in kind")
	atmos.Auth.Identities = map[string]schema.Identity{"reader": {Kind: "aws/user"}}
	atmos.Plugins.Packages["unrelated"] = schema.PluginPackage{Path: filepath.Join(t.TempDir(), "missing")}
	cleanup, err := PrepareAvailable(t.Context(), atmos)
	require.NoError(t, err)
	cleanup()
}
