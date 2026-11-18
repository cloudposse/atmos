package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/cache"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// The maximum size of one snapshot file is the shared cache bound.
	maxCacheSize = cache.DefaultMaxSize

	// The snapshot version is the plugin snapshot schema version.
	snapshotVersion = 1
)

// indexCache is the plugin discovery cache: the shared store scoped to this configuration.
type indexCache struct {
	store *cache.Store
}

// snapshot is the plugin schema stored in a source's snapshot file. It keeps the four shared header
// fields itself so the serialized keys stay version, source, kind, fetched_at, digest, entries,
// releases.
type snapshot struct {
	Version   int                      `json:"version"`
	Source    string                   `json:"source"`
	Kind      string                   `json:"kind"`
	FetchedAt time.Time                `json:"fetched_at"`
	Digest    string                   `json:"digest,omitempty"`
	Entries   []catalog.Entry          `json:"entries"`
	Releases  map[string]catalog.Entry `json:"releases,omitempty"`
}

var _ cache.Snapshot = (*snapshot)(nil)

// SnapshotHeader implements [cache.Snapshot].
//
//nolint:lintroller // Trivial accessor.
func (s *snapshot) SnapshotHeader() cache.Header {
	return cache.Header{Version: s.Version, Source: s.Source, Kind: s.Kind, FetchedAt: s.FetchedAt}
}

// SetSnapshotHeader implements [cache.Snapshot].
//
//nolint:lintroller // Trivial accessor.
func (s *snapshot) SetSnapshotHeader(header cache.Header) {
	s.Version, s.Source, s.Kind, s.FetchedAt = header.Version, header.Source, header.Kind, header.FetchedAt
}

func (s *Service) newCache() (*indexCache, error) {
	directory := filepath.Join(s.Config.Plugins.CacheDir, "discovery")
	if s.Config.Plugins.CacheDir == "" {
		directory = xdg.LookupXDGCacheDir("plugins/discovery")
	}
	info, err := realm.GetRealm(s.Config.Auth.Realm, s.Config.CliConfigPath)
	if err != nil {
		return nil, err
	}
	// Store only a hash of the auth configuration, never credentials or tokens.
	bindings, err := registryauth.PluginBindings(s.Config)
	if err != nil {
		return nil, err
	}
	scope, err := cache.Scope(s.Config.Auth, info.Value, bindings)
	if err != nil {
		return nil, err
	}
	ttl := defaultTTL
	if value := s.Config.Plugins.Discovery.CacheTTL; value != "" {
		ttl, err = time.ParseDuration(value)
		if err != nil || ttl < 0 {
			return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
		}
	}
	return &indexCache{store: cache.New(directory, scope, cache.WithTTL(ttl), cache.WithMaxSize(maxCacheSize))}, nil
}

func (s *Service) loadSource(ctx context.Context, store *indexCache, item source, refresh bool) ([]catalog.Entry, error) {
	spec := &cache.Source[snapshot, *snapshot]{
		Kind:      item.kind,
		Reference: item.reference,
		Version:   snapshotVersion,
		Decode:    decodeSnapshot,
		Fetch: func(ctx context.Context, previous *snapshot) (*snapshot, error) {
			return s.refresh(ctx, item, previous)
		},
		Authorize: func(ctx context.Context, cached bool) error {
			var options []pluginpolicy.Option
			if cached {
				options = append(options, pluginpolicy.WithCache())
			}
			return pluginpolicy.Source(ctx, "search", item.reference, options...)
		},
		ErrOffline:  errUtils.ErrPluginOfflineArtifact,
		ErrTooLarge: errUtils.ErrPluginPayloadTooLarge,
	}
	value, cached, err := cache.Load(ctx, store.store, spec, cache.Mode{Refresh: refresh, Offline: s.Config.Plugins.Policy.Offline})
	if err != nil {
		return nil, err
	}
	var result []catalog.Entry
	if value != nil {
		result = value.Entries
	}
	if err := checkEntrySources(ctx, result, cached); err != nil {
		return nil, err
	}
	return result, nil
}

func checkEntrySources(ctx context.Context, entries []catalog.Entry, cached bool) error {
	var options []pluginpolicy.Option
	if cached {
		options = append(options, pluginpolicy.WithCache())
	}
	for i := range entries {
		if err := pluginpolicy.Source(ctx, "search", entries[i].Source, options...); err != nil {
			return err
		}
	}
	return nil
}

// decodeSnapshot parses a plugin snapshot and rejects one whose entries or releases are invalid.
func decodeSnapshot(raw []byte) (*snapshot, error) {
	var value snapshot
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if !validSnapshot(&value) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	return &value, nil
}

func validSnapshot(value *snapshot) bool {
	if !validEntries(value.Entries) {
		return false
	}
	for digest := range value.Releases {
		entry := value.Releases[digest]
		if len(entry.Versions) != 1 || entry.Versions[0].Digest != digest || !validEntries([]catalog.Entry{entry}) {
			return false
		}
	}
	return true
}

func validEntries(entries []catalog.Entry) bool {
	file := catalog.New("discovery")
	file.Spec.Entries = entries
	raw, err := json.Marshal(file)
	if err != nil {
		return false
	}
	_, err = catalog.Parse(raw)
	return err == nil
}

// writeSnapshot atomically replaces the snapshot file at path, leaving the previous contents in
// place when the new value cannot be serialized or exceeds the size bound.
func writeSnapshot(path string, value *snapshot) error {
	return cache.New("", "", cache.WithMaxSize(maxCacheSize)).WriteSnapshot(path, value, errUtils.ErrPluginPayloadTooLarge)
}
