package discovery

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_registry_test.go -package=discovery github.com/cloudposse/atmos/pkg/oci Registry

var (
	_ = schema.PluginRegistry{Host: "", Repositories: nil}
	_ = schema.PluginDiscoveryConfig{CacheTTL: ""}
)

type fixture struct {
	service      *Service
	server       *httptest.Server
	repository   name.Repository
	requests     atomic.Int64
	gets         atomic.Int64
	tags         atomic.Int64
	fail         atomic.Bool
	payloads     sync.Map
	payloadReads atomic.Int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{}
	handler := containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0)))
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.observe(w, r) {
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(f.server.Close)
	var err error
	f.repository, err = name.NewRepository(f.server.Listener.Addr().String() + "/acme/fixture")
	require.NoError(t, err)
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	f.service = &Service{
		Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{CacheDir: t.TempDir(), Registries: map[string]schema.PluginRegistry{
			"local": {Repositories: []string{"oci://" + f.repository.Name()}},
		}}},
		Registry: atmosoci.NewClient(nil), Descriptors: descriptors,
	}
	return f
}

func (f *fixture) observe(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	f.requests.Add(1)
	if r.Method == http.MethodGet && r.URL.Path != "/v2/" {
		f.gets.Add(1)
	}
	if strings.HasSuffix(r.URL.Path, "/tags/list") {
		f.tags.Add(1)
	}
	if _, exists := f.payloads.Load(r.URL.Path); exists {
		f.payloadReads.Add(1)
		http.Error(w, "executable download prohibited", http.StatusForbidden)
		return true
	}
	if f.fail.Load() {
		http.Error(w, "registry unavailable", http.StatusServiceUnavailable)
		return true
	}
	return false
}

func (f *fixture) publish(t *testing.T, version, description string) *pluginoci.Release {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("executable:"+version+description), 0o755))
	text := fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: acme/fixture, version: %s, description: %s}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    linux/amd64: {executable: bin/plugin}
  capabilities:
    fixtures:
      acme/fixture: {}
`, version, description)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(text), 0o600))
	index, err := pluginoci.BuildIndex(directory, f.service.Descriptors)
	require.NoError(t, err)
	require.NoError(t, f.service.Registry.WriteIndex(t.Context(), f.repository.Tag(version), index))
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	for platform := range release.Platforms {
		descriptor := release.Platforms[platform]
		image, err := index.Image(descriptor.Digest)
		require.NoError(t, err)
		manifest, err := image.Manifest()
		require.NoError(t, err)
		f.payloads.Store("/v2/"+f.repository.RepositoryStr()+"/blobs/"+manifest.Layers[0].Digest.String(), true)
	}
	return release
}

func TestRepositoryIncrementalCacheAndFailedRefresh(t *testing.T) {
	f := newFixture(t)
	first := f.publish(t, "1.0.0", "Original")
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "local", results[0].Registry)
	require.Equal(t, "acme/fixture", results[0].Name)
	require.Equal(t, first.Digest, results[0].Versions[0].Digest)
	calls := f.requests.Load()
	cached, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Equal(t, results, cached)
	require.Equal(t, calls, f.requests.Load())

	beforeGets := f.gets.Load()
	_, err = f.service.Search(t.Context(), Request{Refresh: true})
	require.NoError(t, err)
	// Only the tag listing is fetched; unchanged metadata uses the digest cache.
	require.Equal(t, int64(1), f.gets.Load()-beforeGets)
	second := f.publish(t, "1.1.0", "New")
	results, err = f.service.Search(t.Context(), Request{Refresh: true})
	require.NoError(t, err)
	require.Equal(t, "New", results[0].Description)
	require.Equal(t, []catalog.Version{
		{Version: "1.0.0", Digest: first.Digest, Platforms: []string{"linux/amd64"}, Compatibility: first.Definition.Spec.Compatibility},
		{Version: "1.1.0", Digest: second.Digest, Platforms: []string{"linux/amd64"}, Compatibility: second.Definition.Spec.Compatibility},
	}, results[0].Versions)
	f.fail.Store(true)
	_, err = f.service.Search(t.Context(), Request{Refresh: true})
	require.Error(t, err)
	cached, err = f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Equal(t, results, cached)
	require.Zero(t, f.payloadReads.Load())
}

func TestChangedTagsAndRemovedVersionsRefresh(t *testing.T) {
	f := newFixture(t)
	original := f.publish(t, "1.0.0", "Original")
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	changed := f.publish(t, "1.0.0", "Changed")
	require.NotEqual(t, original.Digest, changed.Digest)
	results, err := f.service.Search(t.Context(), Request{Refresh: true})
	require.NoError(t, err)
	require.Equal(t, changed.Digest, results[0].Versions[0].Digest)
	require.Equal(t, "Changed", results[0].Description)
	registry := NewMockRegistry(gomock.NewController(t))
	registry.EXPECT().Tags(gomock.Any(), f.repository).Return([]string{}, nil)
	f.service.Registry = registry
	results, err = f.service.Search(t.Context(), Request{Refresh: true})
	require.NoError(t, err)
	require.Empty(t, results)
	require.Zero(t, f.payloadReads.Load())
}

func TestOfflineTTLRefreshAndScopeIsolation(t *testing.T) {
	f := newFixture(t)
	f.publish(t, "1.0.0", "Original")
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	f.service.Config.Plugins.Discovery.CacheTTL = "0"
	f.service.Config.Plugins.Policy.Offline = true
	calls := f.requests.Load()
	cached, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Equal(t, results, cached)
	_, err = f.service.Search(t.Context(), Request{Refresh: true})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	require.Equal(t, calls, f.requests.Load())
	f.service.Config.Auth.Realm = "different"
	_, err = f.service.Search(t.Context(), Request{})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	f.service.Config.Auth.Realm = ""
	f.service.Config.Plugins.Policy.Offline = false
	_, err = f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Greater(t, f.requests.Load(), calls)
}

func TestConcurrentColdSearchSharesSnapshot(t *testing.T) {
	f := newFixture(t)
	f.publish(t, "1.0.0", "Original")
	before := f.tags.Load()
	const workers = 12
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			<-start
			results, err := f.service.Search(t.Context(), Request{})
			if err == nil && (len(results) != 1 || results[0].Name != "acme/fixture") {
				err = errUtils.ErrPluginInvalidConfig
			}
			errs <- err
		})
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(1), f.tags.Load()-before)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := f.service.Search(ctx, Request{Refresh: true})
	require.ErrorIs(t, err, context.Canceled)
}

func TestMultipleCatalogsAndRepositorySources(t *testing.T) {
	f := newFixture(t)
	release := f.publish(t, "1.0.0", "Original")
	reference := "oci://" + f.server.Listener.Addr().String() + "/product/catalog:stable"
	file := catalog.New("product")
	require.NoError(t, catalog.Add(file, "oci://"+f.repository.Name(), release, f.service.Descriptors))
	require.NoError(t, catalog.Publish(t.Context(), f.service.Registry, reference, file, ""))
	f.service.Config.Plugins.Registries["product"] = schema.PluginRegistry{Catalog: reference}
	f.service.Config.Plugins.Registries["auth-only"] = schema.PluginRegistry{Host: f.repository.RegistryStr()}
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "local", results[0].Registry)
	require.Equal(t, "product", results[1].Registry)
	require.Equal(t, results[0].Entry, results[1].Entry)
	before := f.gets.Load()
	results, err = f.service.Search(t.Context(), Request{Registry: "product", Refresh: true, Filter: catalog.Filter{Query: "original", Capability: "fixture", Runtime: "native"}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "product", results[0].Registry)
	require.Equal(t, before, f.gets.Load())
	_, err = f.service.Search(t.Context(), Request{Registry: "missing"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	results, err = f.service.Search(t.Context(), Request{Registry: "auth-only"})
	require.NoError(t, err)
	require.Empty(t, results)
}
