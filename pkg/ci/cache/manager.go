package cache

import (
	"context"
	"errors"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Manager orchestrates cache operations against a Backend using a resolved
// Config. It is the single source of truth shared by the CLI subcommands and
// the automatic lifecycle hooks, so "automatic" behavior is exactly the same
// idempotent operations invoked at startup/exit. A per-root state marker keeps
// manual and automatic invocations from double-executing.
type Manager struct {
	backend Backend
	cfg     *Config
}

// NewManager creates a Manager from a backend and resolved config.
func NewManager(backend Backend, cfg *Config) *Manager {
	defer perf.Track(nil, "cache.NewManager")()

	return &Manager{backend: backend, cfg: cfg}
}

// Config returns the resolved configuration.
func (m *Manager) Config() *Config {
	defer perf.Track(nil, "cache.Manager.Config")()

	return m.cfg
}

// RestoreResult describes the outcome of a restore.
type RestoreResult struct {
	// Hit is true when an entry was restored (exact or prefix match).
	Hit bool
	// MatchedKey is the key that was actually restored.
	MatchedKey string
	// Exact is true when the exact key matched (vs a restore-key prefix).
	Exact bool
	// Skipped is true when restore was a no-op because this key was already
	// restored in this lifecycle.
	Skipped bool
}

// SaveResult describes the outcome of a save.
type SaveResult struct {
	// Saved is true when an entry was uploaded (or already present remotely).
	Saved bool
	// Skipped is true when save was a no-op.
	Skipped bool
	// Reason explains a skip (e.g. "exact cache hit", "already saved").
	Reason string
}

// Restore restores the configured cache into the cache root. It is idempotent:
// once a key has been restored in this lifecycle, a subsequent Restore is a
// no-op.
func (m *Manager) Restore(ctx context.Context) (*RestoreResult, error) {
	defer perf.Track(nil, "cache.Manager.Restore")()

	if err := m.cfg.validate(); err != nil {
		return nil, err
	}

	if e := lookupEntry(m.cfg.Root, m.cfg.Key); e != nil && (e.RestoredFrom == restoredExact || e.RestoredFrom == restoredPrefix) {
		log.Debug("CI cache already restored this lifecycle", fieldKey, m.cfg.Key, "matched", e.MatchedKey)
		return &RestoreResult{Hit: true, MatchedKey: e.MatchedKey, Exact: e.RestoredFrom == restoredExact, Skipped: true}, nil
	}

	matchedKey, rc, err := m.backend.Restore(ctx, m.cfg.Key, m.cfg.RestoreKeys)
	if err != nil {
		if errors.Is(err, errUtils.ErrCacheNotFound) {
			recordRestore(m.cfg.Root, m.cfg.Key, restoredMiss, "")
			log.Debug("CI cache miss", fieldKey, m.cfg.Key)
			return &RestoreResult{Hit: false}, nil
		}
		return nil, wrapErr(errUtils.ErrCacheRestoreFailed, err)
	}
	defer rc.Close()

	if err := extractToRoot(rc, m.cfg.Root); err != nil {
		return nil, err
	}

	exact := matchedKey == m.cfg.Key
	from := restoredPrefix
	if exact {
		from = restoredExact
	}
	recordRestore(m.cfg.Root, m.cfg.Key, from, matchedKey)
	log.Debug("CI cache restored", fieldKey, m.cfg.Key, "matched", matchedKey, "exact", exact)
	return &RestoreResult{Hit: true, MatchedKey: matchedKey, Exact: exact}, nil
}

// Save archives the cache root and uploads it under the configured key. It is
// idempotent and respects write-once semantics: when the exact key was a hit at
// restore time (unchanged content) or has already been saved this lifecycle,
// Save is a no-op.
func (m *Manager) Save(ctx context.Context) (*SaveResult, error) {
	defer perf.Track(nil, "cache.Manager.Save")()

	if err := m.cfg.validate(); err != nil {
		return nil, err
	}

	if e := lookupEntry(m.cfg.Root, m.cfg.Key); e != nil {
		if e.Saved {
			return &SaveResult{Skipped: true, Reason: "already saved this lifecycle"}, nil
		}
		if e.RestoredFrom == restoredExact {
			return &SaveResult{Skipped: true, Reason: "exact cache hit; content unchanged"}, nil
		}
	}

	archivePath, size, err := m.buildArchive()
	if err != nil {
		return nil, err
	}
	defer os.Remove(archivePath)

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, wrapErr(errUtils.ErrCacheSaveFailed, err)
	}
	defer f.Close()

	if err := m.backend.Save(ctx, m.cfg.Key, f, size); err != nil {
		if errors.Is(err, errUtils.ErrCacheAlreadyExists) {
			recordSaved(m.cfg.Root, m.cfg.Key)
			return &SaveResult{Saved: true, Skipped: true, Reason: "already exists remotely"}, nil
		}
		return nil, wrapErr(errUtils.ErrCacheSaveFailed, err)
	}

	recordSaved(m.cfg.Root, m.cfg.Key)
	log.Debug("CI cache saved", fieldKey, m.cfg.Key, "size", size)
	return &SaveResult{Saved: true}, nil
}

// buildArchive writes a tar.gz of the cache root to a temp file and returns its
// path and size. The caller is responsible for removing the file.
func (m *Manager) buildArchive() (string, int64, error) {
	tmp, err := os.CreateTemp("", "atmos-cache-*.tar.gz")
	if err != nil {
		return "", 0, wrapErr(errUtils.ErrCacheArchiveFailed, err)
	}
	defer tmp.Close()

	if err := archiveRoot(tmp, m.cfg.Root, m.cfg.Includes); err != nil {
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703: path is from os.CreateTemp, not user input.
		return "", 0, err
	}

	info, err := tmp.Stat()
	if err != nil {
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703: path is from os.CreateTemp, not user input.
		return "", 0, wrapErr(errUtils.ErrCacheArchiveFailed, err)
	}
	return tmp.Name(), info.Size(), nil
}

// List returns cache entries matching the prefix.
func (m *Manager) List(ctx context.Context, prefix string) ([]Entry, error) {
	defer perf.Track(nil, "cache.Manager.List")()

	entries, err := m.backend.List(ctx, ListOptions{KeyPrefix: prefix})
	if err != nil {
		return nil, wrapErr(errUtils.ErrCacheListFailed, err)
	}
	return entries, nil
}

// Delete removes a cache entry by exact key.
func (m *Manager) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "cache.Manager.Delete")()

	if key == "" {
		return errUtils.ErrCacheKeyRequired
	}
	if err := m.backend.Delete(ctx, key); err != nil {
		return wrapErr(errUtils.ErrCacheDeleteFailed, err)
	}
	return nil
}
