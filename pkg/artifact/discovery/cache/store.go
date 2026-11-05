package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultTTL is how long a snapshot stays fresh.
	DefaultTTL = 15 * time.Minute

	// DefaultMaxSize bounds the bytes read from, and written to, one snapshot file.
	DefaultMaxSize = 32 << 20

	// DirectoryMode is the permission of the cache directory.
	DirectoryMode = 0o700

	// FileMode is the permission of a snapshot file.
	FileMode = 0o600

	// The separator splits the hashed parts of a key; no part may contain it.
	separator = "\x00"
)

// Header is the metadata every snapshot starts with. Embed it first in a snapshot struct so the
// serialized keys keep their order: version, source, kind, fetched_at.
type Header struct {
	// Version is the snapshot schema version.
	Version int `json:"version"`

	// Source is the reference the snapshot describes.
	Source string `json:"source"`

	// Kind is the source kind, for example "repository" or "catalog".
	Kind string `json:"kind"`

	// FetchedAt is when the snapshot was fetched, in UTC.
	FetchedAt time.Time `json:"fetched_at"`
}

// Snapshot is implemented by pointers to snapshot structs. A struct that embeds [Header] satisfies
// it through the embedded methods; a struct that keeps the four fields itself implements both
// methods.
type Snapshot interface {
	// SnapshotHeader returns the header of the snapshot.
	SnapshotHeader() Header

	// SetSnapshotHeader replaces the header of the snapshot.
	SetSnapshotHeader(Header)
}

// SnapshotHeader returns the header itself.
//
//nolint:lintroller // Trivial accessor.
func (h *Header) SnapshotHeader() Header { return *h }

// SetSnapshotHeader replaces the header.
//
//nolint:lintroller // Trivial accessor.
func (h *Header) SetSnapshotHeader(value Header) { *h = value }

// Store is one private snapshot directory plus the scope that isolates its files.
type Store struct {
	directory string
	scope     string
	ttl       time.Duration
	maxSize   int64
	now       func() time.Time
}

// Option customizes a [Store].
type Option func(*Store)

// WithTTL sets the freshness window. Zero makes every online load a refresh.
func WithTTL(ttl time.Duration) Option {
	defer perf.Track(nil, "cache.WithTTL")()

	return func(s *Store) { s.ttl = ttl }
}

// WithMaxSize sets the bound on a snapshot file, in bytes.
func WithMaxSize(size int64) Option {
	defer perf.Track(nil, "cache.WithMaxSize")()

	return func(s *Store) { s.maxSize = size }
}

// WithClock injects the clock; tests use a deterministic one.
func WithClock(now func() time.Time) Option {
	defer perf.Track(nil, "cache.WithClock")()

	return func(s *Store) { s.now = now }
}

// New returns a store rooted at directory. The scope is the hash from [Scope]; it separates the
// snapshots of different effective configurations that share the directory.
func New(directory, scope string, opts ...Option) *Store {
	defer perf.Track(nil, "cache.New")()

	store := &Store{directory: directory, scope: scope, ttl: DefaultTTL, maxSize: DefaultMaxSize, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	return store
}

// Directory returns the snapshot directory.
func (s *Store) Directory() string {
	defer perf.Track(nil, "cache.Store.Directory")()

	return s.directory
}

// TTL returns the freshness window.
func (s *Store) TTL() time.Duration {
	defer perf.Track(nil, "cache.Store.TTL")()

	return s.ttl
}

// Path returns the snapshot file of a source.
func (s *Store) Path(kind, reference string) string {
	defer perf.Track(nil, "cache.Store.Path")()

	return filepath.Join(s.directory, Key(s.scope, kind, reference)+".json")
}

// Scope hashes the parts that define a cache scope: the effective configuration, the
// authentication realm, and host bindings. Callers pass configuration, never credential values.
// Equal parts always produce the same scope.
func Scope(parts ...any) (string, error) {
	defer perf.Track(nil, "cache.Scope")()

	raw, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	return Hash(raw), nil
}

// Hash returns the lowercase hexadecimal SHA-256 of raw.
func Hash(raw []byte) string {
	defer perf.Track(nil, "cache.Hash")()

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Key derives the file stem of a source within a scope.
func Key(scope, kind, reference string) string {
	defer perf.Track(nil, "cache.Key")()

	return Hash([]byte(scope + separator + kind + separator + reference))
}

// Fresh reports whether a snapshot fetched at the given time is still within the TTL.
func (s *Store) Fresh(fetched time.Time) bool {
	defer perf.Track(nil, "cache.Store.Fresh")()

	age := s.now().Sub(fetched)
	return age >= 0 && age < s.ttl
}

// read returns the raw bytes of a snapshot file within the size bound, or nil when it is absent,
// unreadable, or oversized.
func (s *Store) read(path string) []byte {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, s.maxSize+1))
	if err != nil || int64(len(raw)) > s.maxSize {
		return nil
	}
	return raw
}

// WriteSnapshot serializes value and atomically replaces the file at path; a reader sees the old
// or the new file, never a mix. A value larger than the size bound fails with errTooLarge and
// leaves the previous file untouched.
func (s *Store) WriteSnapshot(path string, value any, errTooLarge error) error {
	defer perf.Track(nil, "cache.Store.WriteSnapshot")()

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if int64(len(raw)) > s.maxSize {
		return errTooLarge
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(path, raw, FileMode)
}
