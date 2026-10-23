package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	maxCacheSize         = 32 << 20
	privateDirectoryMode = 0o700
	privateFileMode      = 0o600
)

type indexCache struct {
	directory, scope string
	ttl              time.Duration
}

type snapshot struct {
	Version   int                      `json:"version"`
	Source    string                   `json:"source"`
	Kind      string                   `json:"kind"`
	FetchedAt time.Time                `json:"fetched_at"`
	Digest    string                   `json:"digest,omitempty"`
	Entries   []catalog.Entry          `json:"entries"`
	Releases  map[string]catalog.Entry `json:"releases,omitempty"`
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
	raw, err := json.Marshal([]any{s.Config.Auth, info.Value, bindings})
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
	return &indexCache{directory: directory, scope: hash(raw), ttl: ttl}, nil
}

func hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func (s *Service) loadSource(ctx context.Context, cache *indexCache, item source, refresh bool) ([]catalog.Entry, error) {
	key := hash([]byte(cache.scope + "\x00" + item.kind + "\x00" + item.reference))
	if err := os.MkdirAll(cache.directory, privateDirectoryMode); err != nil {
		return nil, err
	}
	path := filepath.Join(cache.directory, key+".json")
	var result []catalog.Entry
	var cached bool
	err := filelock.New(path+".lock").WithExclusive(ctx, func() error {
		var err error
		result, cached, err = s.updateSnapshot(ctx, cache, item, refresh, path)
		return err
	})
	if err != nil {
		return nil, err
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

func (s *Service) updateSnapshot(ctx context.Context, cache *indexCache, item source, refresh bool, path string) ([]catalog.Entry, bool, error) {
	old := readSnapshot(path, item)
	if old != nil && (s.Config.Plugins.Policy.Offline || (!refresh && fresh(old.FetchedAt, cache.ttl))) {
		if err := pluginpolicy.Source(ctx, "search", item.reference, pluginpolicy.WithCache()); err != nil {
			return nil, true, err
		}
		return old.Entries, true, nil
	}
	if s.Config.Plugins.Policy.Offline {
		return nil, false, errUtils.ErrPluginOfflineArtifact
	}
	if err := pluginpolicy.Source(ctx, "search", item.reference); err != nil {
		return nil, false, err
	}
	return s.refreshSnapshot(ctx, item, old, path)
}

func (s *Service) refreshSnapshot(ctx context.Context, item source, old *snapshot, path string) ([]catalog.Entry, bool, error) {
	current, err := s.refresh(ctx, item, old)
	if err != nil {
		return nil, false, err
	}
	current.Version, current.Source, current.Kind, current.FetchedAt = 1, item.reference, item.kind, time.Now().UTC()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := writeSnapshot(path, current); err != nil {
		return nil, false, err
	}
	return current.Entries, false, nil
}

func fresh(fetched time.Time, ttl time.Duration) bool {
	age := time.Since(fetched)
	return age >= 0 && age < ttl
}

func readSnapshot(path string, item source) *snapshot {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxCacheSize+1))
	if err != nil || len(raw) > maxCacheSize {
		return nil
	}
	var value snapshot
	if json.Unmarshal(raw, &value) != nil || value.Version != 1 || value.Source != item.reference || value.Kind != item.kind {
		return nil
	}
	if !validSnapshot(&value) {
		return nil
	}
	return &value
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

func writeSnapshot(path string, value *snapshot) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > maxCacheSize {
		return errUtils.ErrPluginPayloadTooLarge
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(path, raw, privateFileMode)
}
