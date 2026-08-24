package cache

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	metadataSuffix = ".metadata.json"
	lockSuffix     = ".lock"
	tempPrefix     = ".tmp-"

	// GroupProvider/GroupModule classify a cache object by its top-level directory.
	GroupProvider = "provider"
	GroupModule   = "module"
	groupOther    = "other"
)

// Entry describes a single cached object on disk.
type Entry struct {
	// Key is the canonical cache key (the object's path under the root, forward-slashed).
	Key string `json:"key"`
	// Group classifies the object: provider, module, or other.
	Group string `json:"group"`
	// Kind is the artifact kind from the sidecar (metadata or artifact), if known.
	Kind string `json:"kind,omitempty"`
	// Size is the object size in bytes.
	Size int64 `json:"size"`
	// ModTime is the filesystem modification time.
	ModTime time.Time `json:"mod_time"`
	// FetchedAt is when the object was cached (from the sidecar), if recorded.
	FetchedAt time.Time `json:"fetched_at,omitempty"`
}

// Summary aggregates filesystem facts about the cache. It deliberately contains NO
// hit rate: there is no persistent hit/miss store (the filesystem is the index).
// Hit statistics are per-run only, surfaced by the savings report.
type Summary struct {
	Root        string `json:"root"`
	ObjectCount int    `json:"object_count"`
	TotalSize   int64  `json:"total_size"`
	Providers   int    `json:"providers"`
	Modules     int    `json:"modules"`
	Largest     *Entry `json:"largest,omitempty"`
	Oldest      *Entry `json:"oldest,omitempty"`
}

// sidecarMeta mirrors the proxy's on-disk sidecar fields needed for inspection.
type sidecarMeta struct {
	Custom struct {
		Kind      string `json:"kind"`
		FetchedAt string `json:"fetched_at"`
	} `json:"custom"`
}

// ResolveRoot resolves the cache root for the given configuration (the configured
// location, else the XDG cache directory). It does not create anything.
func ResolveRoot(atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "tfcache.ResolveRoot")()

	cfg := atmosConfig.Components.Terraform.Cache
	if cfg == nil {
		cfg = &schema.TerraformCacheConfig{}
	}
	return resolveRoot(cfg)
}

// List walks the cache root and returns every cached artifact: provider and module
// objects only. It excludes metadata sidecars, lock files, in-flight temp files, and
// proxy infrastructure that is co-located in the cache root but is not a cached
// artifact — notably the self-signed TLS certificate under tls/ (see tls.go) and the
// vestigial layout directories. Because Summarize/Delete/Prune all build on List, the
// TLS material is never counted in stats and never pruned or deleted.
func List(root string) ([]Entry, error) {
	defer perf.Track(nil, "tfcache.List")()

	var entries []Entry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !isObjectFile(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		// Only provider and module artifacts are cached objects. Anything else under
		// the root (e.g. the proxy's tls/ certificate) is infrastructure, not a
		// cached artifact, and must not inflate counts or be eligible for pruning.
		if classifyGroup(key) == groupOther {
			return nil
		}
		entries = append(entries, newEntry(key, path, info))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: walking cache root: %w", errUtils.ErrInvalidConfig, err)
	}
	return entries, nil
}

// Summarize computes filesystem facts about the cache (size, counts, breakdown,
// largest, oldest). No hit rate — see Summary.
func Summarize(root string) (Summary, error) {
	defer perf.Track(nil, "tfcache.Summarize")()

	entries, err := List(root)
	if err != nil {
		return Summary{}, err
	}

	s := Summary{Root: root, ObjectCount: len(entries)}
	for i := range entries {
		e := entries[i]
		s.TotalSize += e.Size
		switch e.Group {
		case GroupProvider:
			s.Providers++
		case GroupModule:
			s.Modules++
		}
		if s.Largest == nil || e.Size > s.Largest.Size {
			cp := e
			s.Largest = &cp
		}
		if s.Oldest == nil || e.ModTime.Before(s.Oldest.ModTime) {
			cp := e
			s.Oldest = &cp
		}
	}
	return s, nil
}

// Delete removes a single cached object (and its sidecar), cleaning up emptied
// parent directories up to the root. Idempotent.
func Delete(root, key string) error {
	defer perf.Track(nil, "tfcache.Delete")()

	// Enforce the same artifact-only contract as List/Prune: only provider and module
	// objects are deletable. Reject path traversal and non-artifact keys (e.g. the
	// proxy's tls/ certificate) so user-supplied keys can never remove cache
	// infrastructure or escape the root.
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(key)))
	if filepath.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return fmt.Errorf("%w: invalid cache key %q", errUtils.ErrInvalidConfig, key)
	}
	if classifyGroup(normalized) == groupOther {
		return fmt.Errorf("%w: unsupported cache key %q", errUtils.ErrInvalidConfig, key)
	}

	objPath := filepath.Join(root, filepath.FromSlash(normalized))
	if err := os.Remove(objPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: deleting %q: %w", errUtils.ErrInvalidConfig, normalized, err)
	}
	_ = os.Remove(objPath + metadataSuffix)
	_ = os.Remove(objPath + lockSuffix)
	cleanupEmptyDirs(root, filepath.Dir(objPath))
	return nil
}

// Prune removes cached metadata older than olderThan. Immutable artifacts (provider
// zips, module archives) are retained unless includeArtifacts is set. When dryRun is
// true nothing is deleted; the entries that WOULD be removed are returned.
func Prune(root string, olderThan time.Duration, includeArtifacts, dryRun bool) ([]Entry, error) {
	defer perf.Track(nil, "tfcache.Prune")()

	entries, err := List(root)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-olderThan)
	var pruned []Entry
	for i := range entries {
		if !shouldPrune(&entries[i], cutoff, includeArtifacts) {
			continue
		}
		pruned = append(pruned, entries[i])
		if dryRun {
			continue
		}
		if delErr := Delete(root, entries[i].Key); delErr != nil {
			return pruned, delErr
		}
	}
	return pruned, nil
}

// shouldPrune decides whether an entry is eligible for pruning.
func shouldPrune(e *Entry, cutoff time.Time, includeArtifacts bool) bool {
	if e.Kind == "artifact" && !includeArtifacts {
		return false
	}
	ts := e.FetchedAt
	if ts.IsZero() {
		ts = e.ModTime
	}
	return ts.Before(cutoff)
}

// newEntry builds an Entry, enriching it from the sidecar when present.
func newEntry(key, path string, info fs.FileInfo) Entry {
	e := Entry{
		Key:     key,
		Group:   classifyGroup(key),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if sc, ok := readSidecar(path + metadataSuffix); ok {
		e.Kind = sc.Custom.Kind
		if t, err := time.Parse(time.RFC3339, sc.Custom.FetchedAt); err == nil {
			e.FetchedAt = t
		}
	}
	return e
}

func classifyGroup(key string) string {
	switch {
	case strings.HasPrefix(key, "providers/"):
		return GroupProvider
	case strings.HasPrefix(key, "modules/"):
		return GroupModule
	default:
		return groupOther
	}
}

// isObjectFile reports whether name is a cached object (not a sidecar, lock, or temp).
func isObjectFile(name string) bool {
	if strings.HasSuffix(name, metadataSuffix) || strings.HasSuffix(name, lockSuffix) {
		return false
	}
	return !strings.HasPrefix(name, tempPrefix)
}

func readSidecar(path string) (sidecarMeta, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return sidecarMeta{}, false
	}
	var sc sidecarMeta
	if err := json.Unmarshal(b, &sc); err != nil {
		return sidecarMeta{}, false
	}
	return sc, true
}

// cleanupEmptyDirs removes empty directories from dir up to (but not including) root.
func cleanupEmptyDirs(root, dir string) {
	for dir != root && strings.HasPrefix(dir, root) {
		if err := os.Remove(dir); err != nil {
			return // Non-empty or error: stop.
		}
		dir = filepath.Dir(dir)
	}
}
