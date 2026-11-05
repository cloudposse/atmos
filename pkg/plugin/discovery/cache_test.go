package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

func TestCorruptCacheRecoversOnlineAndFailsOffline(t *testing.T) {
	f := newFixture(t)
	release := f.publish(t, "1.0.0", "Original")
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(f.service.Config.Plugins.CacheDir, "discovery", "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	for _, corrupt := range []string{`{`, `{"version":2}`, `{"version":1,"source":"oci://other.example.com/repo","kind":"repository"}`} {
		require.NoError(t, os.WriteFile(files[0], []byte(corrupt), 0o600))
		f.service.Config.Plugins.Policy.Offline = true
		_, err = f.service.Search(t.Context(), Request{})
		require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
		f.service.Config.Plugins.Policy.Offline = false
		results, err := f.service.Search(t.Context(), Request{})
		require.NoError(t, err)
		require.Equal(t, release.Digest, results[0].Versions[0].Digest)
	}
}

func TestAddingSourceKeepsExistingCacheAndXDGDefault(t *testing.T) {
	f := newFixture(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ATMOS_XDG_CACHE_HOME", "")
	f.service.Config.Plugins.CacheDir = ""
	f.publish(t, "1.0.0", "Original")
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(xdg.LookupXDGCacheDir("plugins/discovery"), "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	// Adding another discovery source must not invalidate an unchanged source.
	f.service.Config.Plugins.Registries["other"] = schema.PluginRegistry{Repositories: []string{"oci://other.example.com/plugins/other"}}
	f.service.Config.Plugins.Policy.Offline = true
	results, err := f.service.Search(t.Context(), Request{Registry: "local"})
	require.NoError(t, err)
	require.Equal(t, "acme/fixture", results[0].Name)
	_, err = f.service.Search(t.Context(), Request{Registry: "other"})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
}

func TestTagVersionMustMatchInspectedMetadata(t *testing.T) {
	f := newFixture(t)
	release := f.publish(t, "1.0.0", "Original")
	require.NoError(t, f.service.Registry.WriteIndex(t.Context(), f.repository.Tag("2.0.0"), release.Index))
	results, err := f.service.Search(t.Context(), Request{})
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	require.Empty(t, results)
	files, err := filepath.Glob(filepath.Join(f.service.Config.Plugins.CacheDir, "discovery", "*.json"))
	require.NoError(t, err)
	require.Empty(t, files, "a failed refresh must not persist a partial source index")
}

func TestDiscoveryFiltersNonReleaseTags(t *testing.T) {
	f := newFixture(t)
	release := f.publish(t, "1.0.0", "Original")
	require.NoError(t, f.service.Registry.WriteIndex(t.Context(), f.repository.Tag("latest"), release.Index))
	require.NoError(t, f.service.Registry.WriteIndex(t.Context(), f.repository.Tag("v1.0.0"), release.Index))
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Versions, 1)
	require.Equal(t, "1.0.0", results[0].Versions[0].Version)
	require.Equal(t, release.Digest, results[0].Versions[0].Digest)
}

func TestCatalogCurationSurvivesMergedRepositoryIndex(t *testing.T) {
	entry := catalog.Entry{Name: "acme/plugin", Source: "oci://registry.example.com/acme/plugin", Versions: []catalog.Version{
		{Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)},
	}}
	curated := entry
	curated.Versions = []catalog.Version{{Version: "1.0.0", Digest: entry.Versions[0].Digest, Yanked: &catalog.Yanked{Reason: "withdrawn"}}}
	merged, err := mergeEntry([]catalog.Entry{entry}, &curated)
	require.NoError(t, err)
	require.Equal(t, "withdrawn", merged[0].Versions[0].Yanked.Reason)
	merged, err = mergeEntry(merged, &entry)
	require.NoError(t, err)
	require.Equal(t, "withdrawn", merged[0].Versions[0].Yanked.Reason)
	require.Nil(t, entry.Versions[0].Yanked, "merging must not mutate the input")
	conflict := entry
	conflict.Source = "oci://other.example.com/plugin"
	_, err = mergeEntry(merged, &conflict)
	require.ErrorIs(t, err, errUtils.ErrPluginCatalogConflict)
	conflict = entry
	conflict.Versions = []catalog.Version{{Version: "1.0.0", Digest: "sha256:" + strings.Repeat("b", 64)}}
	_, err = mergeEntry(merged, &conflict)
	require.ErrorIs(t, err, errUtils.ErrPluginImmutableRelease)
}

func TestMetadataWorkersRespectConcurrencyLimit(t *testing.T) {
	f := newFixture(t)
	f.service.Config.Plugins.MaxConcurrency = 2
	f.service.Config.Plugins.Registries = map[string]schema.PluginRegistry{
		"local": {Repositories: []string{"oci://registry.example.com/one", "oci://registry.example.com/two", "oci://registry.example.com/three"}},
	}
	registry := NewMockRegistry(gomock.NewController(t))
	f.service.Registry = registry
	var active, peak atomic.Int64
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	registry.EXPECT().Tags(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ name.Repository) ([]string, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old; old = peak.Load() {
			if peak.CompareAndSwap(old, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
			return []string{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}).Times(3)
	done := make(chan error, 1)
	go func() { _, err := f.service.Search(t.Context(), Request{}); done <- err }()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	require.Equal(t, int64(2), active.Load())
	close(release)
	require.NoError(t, <-done)
	require.Equal(t, int64(2), peak.Load())
}

func TestInvalidCacheMetadataCannotBeUsedOffline(t *testing.T) {
	f := newFixture(t)
	f.publish(t, "1.0.0", "Original")
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(f.service.Config.Plugins.CacheDir, "discovery", "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	original, err := os.ReadFile(files[0])
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		mutate func(*snapshot)
	}{
		{"invalid entry", func(value *snapshot) { value.Entries[0].Name = "" }},
		{"release missing version", func(value *snapshot) {
			for digest, entry := range value.Releases {
				entry.Versions = nil
				value.Releases[digest] = entry
			}
		}},
		{"release digest mismatch", func(value *snapshot) {
			for digest, entry := range value.Releases {
				entry.Versions[0].Digest = "sha256:" + strings.Repeat("b", 64)
				value.Releases[digest] = entry
			}
		}},
		{"invalid release metadata", func(value *snapshot) {
			for digest, entry := range value.Releases {
				entry.Name = ""
				value.Releases[digest] = entry
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value snapshot
			require.NoError(t, json.Unmarshal(original, &value))
			require.NotEmpty(t, value.Releases)
			tc.mutate(&value)
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(files[0], raw, 0o600))
			f.service.Config.Plugins.Policy.Offline = true
			results, err := f.service.Search(t.Context(), Request{})
			require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
			require.Empty(t, results)
		})
	}
	t.Run("oversized cache", func(t *testing.T) {
		require.NoError(t, os.WriteFile(files[0], []byte(strings.Repeat(" ", maxCacheSize+1)), 0o600))
		results, err := f.service.Search(t.Context(), Request{})
		require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
		require.Empty(t, results)
	})
}

func TestDiscoveryConfigurationAndFilesystemFailuresDoNotContactRegistry(t *testing.T) {
	for _, ttl := range []string{"not-a-duration", "-1s"} {
		t.Run(ttl, func(t *testing.T) {
			f := newFixture(t)
			f.service.Config.Plugins.Discovery.CacheTTL = ttl
			results, err := f.service.Search(t.Context(), Request{})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Empty(t, results)
			require.Zero(t, f.requests.Load())
		})
	}
	t.Run("cache path is a file", func(t *testing.T) {
		f := newFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(f.service.Config.Plugins.CacheDir, "discovery"), []byte("occupied"), 0o600))
		results, err := f.service.Search(t.Context(), Request{})
		require.Error(t, err)
		require.Empty(t, results)
		require.Zero(t, f.requests.Load())
	})
}

func TestDiscoveryFailedMetadataRefreshNeverCachesPartialResults(t *testing.T) {
	failure := errors.New("registry metadata unavailable")
	digest := v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}
	for _, stage := range []string{"tags", "head", "get"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			registry := NewMockRegistry(gomock.NewController(t))
			f.service.Registry = registry
			if stage == "tags" {
				registry.EXPECT().Tags(gomock.Any(), gomock.Any()).Return(nil, failure)
			} else {
				registry.EXPECT().Tags(gomock.Any(), gomock.Any()).Return([]string{"1.0.0"}, nil)
				if stage == "head" {
					registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(nil, failure)
				} else {
					registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(&v1.Descriptor{Digest: digest}, nil)
					registry.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, failure)
				}
			}
			results, err := f.service.Search(t.Context(), Request{})
			require.ErrorIs(t, err, failure)
			require.Empty(t, results)
			files, err := filepath.Glob(filepath.Join(f.service.Config.Plugins.CacheDir, "discovery", "*.json"))
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
}

func TestCatalogRefreshErrorsNeverProduceResults(t *testing.T) {
	failure := errors.New("catalog unavailable")
	for _, stage := range []string{"head", "get"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			f.service.Config.Plugins.Registries = map[string]schema.PluginRegistry{"local": {Catalog: "oci://" + f.repository.Tag("catalog").Name()}}
			registry := NewMockRegistry(gomock.NewController(t))
			f.service.Registry = registry
			if stage == "head" {
				registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(nil, failure)
			} else {
				registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(&v1.Descriptor{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}}, nil)
				registry.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, failure)
			}
			results, err := f.service.Search(t.Context(), Request{})
			require.ErrorIs(t, err, failure)
			require.Empty(t, results)
		})
	}
}

func TestDiscoveryRejectsWrongReleaseArtifact(t *testing.T) {
	for _, mediaType := range []types.MediaType{types.OCIManifestSchema1, types.OCIImageIndex} {
		t.Run(string(mediaType), func(t *testing.T) {
			f := newFixture(t)
			raw, err := empty.Index.RawManifest()
			require.NoError(t, err)
			descriptor := &remote.Descriptor{Descriptor: v1.Descriptor{MediaType: mediaType, Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}}, Manifest: raw}
			registry := NewMockRegistry(gomock.NewController(t))
			f.service.Registry = registry
			registry.EXPECT().Tags(gomock.Any(), gomock.Any()).Return([]string{"1.0.0"}, nil)
			registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(&descriptor.Descriptor, nil)
			registry.EXPECT().Get(gomock.Any(), gomock.Any()).Return(descriptor, nil)
			results, err := f.service.Search(t.Context(), Request{})
			require.Error(t, err)
			require.Empty(t, results)
		})
	}
}

func TestCancelledDiscoveryRefreshDoesNotPublishCache(t *testing.T) {
	for _, tags := range [][]string{{}, {"1.0.0"}} {
		t.Run(strings.Join(tags, ","), func(t *testing.T) {
			f := newFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			registry := NewMockRegistry(gomock.NewController(t))
			f.service.Registry = registry
			registry.EXPECT().Tags(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, name.Repository) ([]string, error) { cancel(); return tags, nil })
			results, err := f.service.Search(ctx, Request{})
			require.ErrorIs(t, err, context.Canceled)
			require.Empty(t, results)
			files, err := filepath.Glob(filepath.Join(f.service.Config.Plugins.CacheDir, "discovery", "*.json"))
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
}

func TestSnapshotWriteFailurePreservesPreviousContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	original := []byte("previous verified snapshot")
	require.NoError(t, os.WriteFile(path, original, 0o600))
	for _, value := range []*snapshot{
		{FetchedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Source: strings.Repeat("x", maxCacheSize+1)},
	} {
		require.Error(t, writeSnapshot(path, value))
		current, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, original, current)
	}
}
