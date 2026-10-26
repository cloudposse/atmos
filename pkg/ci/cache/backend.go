// Package cache provides a CI-provider-scoped remote build cache, modeled on
// the artifact subsystem (pkg/ci/artifact). It lets Atmos restore a well-known
// cache directory (the toolchain install path and anything else under the XDG
// cache root) at startup and save it at exit, using the same store that
// actions/cache uses when running inside a CI provider.
//
// The Backend interface is key/restore-key oriented (write-once entries with
// prefix fallback), unlike the artifact Backend which is name-oriented. A
// concrete Backend is provided by the active CI provider (see
// pkg/ci/providers/github), so all operations are no-ops with
// errUtils.ErrCacheUnavailable when no cache-capable provider is detected.
package cache

import (
	"context"
	"io"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate mockgen -package cache -destination mock_backend.go github.com/cloudposse/atmos/pkg/ci/cache Backend

// Backend defines the interface for a remote CI cache store.
//
// Cache entries are immutable (write-once): saving a key that already exists
// returns errUtils.ErrCacheAlreadyExists. Restore performs an exact-key lookup
// first, then falls back to the supplied restore-keys (prefix matches, newest
// first), mirroring the semantics of actions/cache.
type Backend interface {
	// Name returns the backend type name (e.g., "github/actions").
	Name() string

	// Save uploads a single data stream (a tar.gz archive) under the exact key.
	// Implementations return errUtils.ErrCacheAlreadyExists when the key exists.
	Save(ctx context.Context, key string, data io.Reader, size int64) error

	// Restore downloads the entry for the exact key, or the first restore-key
	// prefix match when the exact key is absent. It returns the key that was
	// actually matched and a reader the caller must close. When nothing
	// matches it returns errUtils.ErrCacheNotFound.
	Restore(ctx context.Context, key string, restoreKeys []string) (matchedKey string, rc io.ReadCloser, err error)

	// List returns cache entries, optionally filtered by key prefix.
	List(ctx context.Context, opts ListOptions) ([]Entry, error)

	// Delete removes a cache entry by exact key. Deleting a missing key is a
	// no-op (returns nil) so callers can treat delete as idempotent.
	Delete(ctx context.Context, key string) error
}

// ListOptions filters the entries returned by Backend.List.
type ListOptions struct {
	// KeyPrefix restricts results to entries whose key starts with this value.
	// Empty matches all entries.
	KeyPrefix string
}

// Entry describes a single cache entry as reported by Backend.List.
type Entry struct {
	// Key is the cache key.
	Key string

	// Size is the entry size in bytes (0 if unknown).
	Size int64

	// CreatedAt is when the entry was created (zero if unknown).
	CreatedAt time.Time

	// ID is the provider-specific identifier for the entry (may be empty).
	ID string
}

// Options contains the inputs a BackendFactory uses to construct a Backend.
type Options struct {
	// AtmosConfig is the active Atmos configuration (may be nil).
	AtmosConfig *schema.AtmosConfiguration

	// Options carries backend-specific configuration (owner, repo, etc.).
	Options map[string]any
}

// BackendFactory creates a Backend from Options.
type BackendFactory func(opts Options) (Backend, error)
