package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// metadataSuffix matches the local artifact backend's sidecar suffix so the cache
// management commands (which use the artifact local backend) can List/Delete
// objects the proxy writes.
const metadataSuffix = ".metadata.json"

const (
	storeDirPerm  = 0o755
	storeFilePerm = 0o644
)

// Meta is the proxy's view of a cached object's metadata.
type Meta struct {
	Size        int64
	SHA256      string
	FetchedAt   time.Time
	Kind        ArtifactKind
	ContentType string
}

// Store is the proxy's cache backend: a content store keyed by canonical cache
// keys, with per-key locking. The filesystem implementation (FileStore) writes the
// object at <root>/<key> and an artifact-compatible <key>.metadata.json sidecar so
// the object's path IS the key (the source of truth — no separate index).
type Store interface {
	// Lock returns the per-key file lock (one writer, many readers).
	Lock(key string) cache.FileLock
	// Stat returns metadata for key and whether it exists.
	Stat(key string) (Meta, bool, error)
	// Open returns a reader for the cached object plus its metadata.
	Open(key string) (io.ReadCloser, Meta, error)
	// Commit streams the request data to a temp file (hashing as it goes), runs
	// Verify against the computed SHA-256, then atomically renames it into place and
	// writes the sidecar. A non-nil Verify error rejects the object (nothing is committed).
	Commit(ctx context.Context, req CommitRequest) (Meta, error)
	// Root returns the cache root directory.
	Root() string
}

// CommitRequest describes an object to commit to the store.
type CommitRequest struct {
	// Key is the canonical cache key (and object path under the root).
	Key string
	// Data is the object content to store.
	Data io.Reader
	// Kind classifies the object for freshness policy.
	Kind ArtifactKind
	// ContentType is recorded in the sidecar and used when serving.
	ContentType string
	// Verify, when non-nil, validates the computed SHA-256 before the object is
	// committed; a non-nil error rejects it.
	Verify func(sha256Hex string) error
}

// sidecar is the JSON persisted next to each object. The Custom-style fields mirror
// what the artifact backend stores so the two interoperate.
type sidecar struct {
	SHA256    string    `json:"sha256,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Custom    struct {
		Kind        string `json:"kind"`
		ContentType string `json:"content_type,omitempty"`
		FetchedAt   string `json:"fetched_at"`
	} `json:"custom"`
}

// FileStore is the filesystem-backed Store.
type FileStore struct {
	root string
}

// NewFileStore returns a Store rooted at root.
func NewFileStore(root string) *FileStore {
	defer perf.Track(nil, "proxy.NewFileStore")()

	return &FileStore{root: root}
}

// Root returns the cache root.
func (s *FileStore) Root() string {
	defer perf.Track(nil, "proxy.FileStore.Root")()

	return s.root
}

func (s *FileStore) objectPath(key string) string {
	return filepath.Join(s.root, filepath.FromSlash(key))
}

func (s *FileStore) sidecarPath(key string) string {
	return s.objectPath(key) + metadataSuffix
}

// Lock returns a file lock on the object path (FileLock appends a .lock sidecar).
// The parent directory is created first so flock can open the lock file even on a
// cold key whose directory does not yet exist.
func (s *FileStore) Lock(key string) cache.FileLock {
	defer perf.Track(nil, "proxy.FileStore.Lock")()

	_ = os.MkdirAll(filepath.Dir(s.objectPath(key)), storeDirPerm)
	return cache.NewFileLock(s.objectPath(key))
}

// Stat returns the object's metadata if it exists.
func (s *FileStore) Stat(key string) (Meta, bool, error) {
	defer perf.Track(nil, "proxy.FileStore.Stat")()

	info, err := os.Stat(s.objectPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, false, nil
		}
		return Meta{}, false, err
	}
	if info.IsDir() {
		return Meta{}, false, nil
	}

	meta := Meta{Size: info.Size()}
	if sc, ok := s.readSidecar(key); ok {
		meta.SHA256 = sc.SHA256
		meta.ContentType = sc.Custom.ContentType
		meta.Kind = kindFromString(sc.Custom.Kind)
		if t, perr := time.Parse(time.RFC3339, sc.Custom.FetchedAt); perr == nil {
			meta.FetchedAt = t
		} else {
			meta.FetchedAt = info.ModTime()
		}
	} else {
		meta.FetchedAt = info.ModTime()
	}
	return meta, true, nil
}

// Open returns a reader for the cached object plus its metadata.
func (s *FileStore) Open(key string) (io.ReadCloser, Meta, error) {
	defer perf.Track(nil, "proxy.FileStore.Open")()

	meta, ok, err := s.Stat(key)
	if err != nil {
		return nil, Meta{}, err
	}
	if !ok {
		return nil, Meta{}, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
	}
	f, err := os.Open(s.objectPath(key))
	if err != nil {
		return nil, Meta{}, err
	}
	return f, meta, nil
}

// Commit streams data to a temp file while hashing, verifies, then atomically
// renames into place and writes the sidecar. All paths are derived from the
// request key joined under the cache root.
//
// A tempObject holds a staged temp file (path, digest, size) awaiting commit.
type tempObject struct {
	name   string
	sha256 string
	size   int64
}

func (s *FileStore) Commit(ctx context.Context, req CommitRequest) (Meta, error) {
	defer perf.Track(nil, "proxy.FileStore.Commit")()

	finalPath := s.objectPath(req.Key)
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, storeDirPerm); err != nil {
		return Meta{}, fmt.Errorf("%w: creating cache dir: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	staged, err := writeTempObject(dir, req)
	if err != nil {
		return Meta{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(staged.name)
		}
	}()

	if req.Verify != nil {
		if verr := req.Verify(staged.sha256); verr != nil {
			return Meta{}, verr
		}
	}

	if err := os.Chmod(staged.name, storeFilePerm); err != nil {
		return Meta{}, fmt.Errorf("%w: chmod object %s: %w", errUtils.ErrArtifactUploadFailed, req.Key, err)
	}
	if err := os.Rename(staged.name, finalPath); err != nil {
		return Meta{}, fmt.Errorf("%w: committing object %s: %w", errUtils.ErrArtifactUploadFailed, req.Key, err)
	}
	committed = true

	meta := Meta{Size: staged.size, SHA256: staged.sha256, FetchedAt: time.Now().UTC(), Kind: req.Kind, ContentType: req.ContentType}
	if err := s.writeSidecar(req.Key, meta); err != nil {
		// The object is committed; a missing sidecar degrades to mtime-based freshness.
		log.Debug("Registry cache: failed to write sidecar", "key", req.Key, "error", err)
	}
	return meta, nil
}

// writeTempObject streams req.Data to a temp file in dir while computing its
// SHA-256, returning the staged temp path, hex digest, and byte size.
func writeTempObject(dir string, req CommitRequest) (tempObject, error) {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return tempObject{}, fmt.Errorf("%w: creating temp object: %w", errUtils.ErrArtifactUploadFailed, err)
	}
	tmpName := tmp.Name()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), req.Data)
	if err != nil {
		tmp.Close()
		_ = os.Remove(tmpName) //nolint:gosec // tmpName is a CreateTemp path inside the cache root.
		return tempObject{}, fmt.Errorf("%w: writing object %s: %w", errUtils.ErrArtifactUploadFailed, req.Key, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:gosec // tmpName is a CreateTemp path inside the cache root.
		return tempObject{}, fmt.Errorf("%w: closing object %s: %w", errUtils.ErrArtifactUploadFailed, req.Key, err)
	}

	return tempObject{name: tmpName, sha256: hex.EncodeToString(hasher.Sum(nil)), size: size}, nil
}

// writeSidecar persists the sidecar atomically: it writes a temp file in the same
// directory then renames it over the final path. This mirrors the object commit so
// a concurrent, lock-free reader (the hit fast path) never observes a torn sidecar.
func (s *FileStore) writeSidecar(key string, meta Meta) error {
	var sc sidecar
	sc.SHA256 = meta.SHA256
	sc.CreatedAt = meta.FetchedAt
	sc.Custom.Kind = meta.Kind.String()
	sc.Custom.ContentType = meta.ContentType
	sc.Custom.FetchedAt = meta.FetchedAt.Format(time.RFC3339)

	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}

	finalPath := s.sidecarPath(key)
	tmp, err := os.CreateTemp(filepath.Dir(finalPath), ".tmp-sidecar-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, storeFilePerm); err != nil { //nolint:gosec // tmpName is a store-internal temp path, not user input.
		return err
	}
	if err := os.Rename(tmpName, finalPath); err != nil { //nolint:gosec // paths are store-internal, derived from the validated cache key.
		return err
	}
	committed = true
	return nil
}

func (s *FileStore) readSidecar(key string) (sidecar, bool) {
	b, err := os.ReadFile(s.sidecarPath(key))
	if err != nil {
		return sidecar{}, false
	}
	var sc sidecar
	if err := json.Unmarshal(b, &sc); err != nil {
		return sidecar{}, false
	}
	return sc, true
}

func kindFromString(s string) ArtifactKind {
	switch s {
	case "metadata":
		return KindMetadata
	case "artifact":
		return KindArtifact
	case "passthrough":
		return KindPassthrough
	default:
		return KindMetadata
	}
}
