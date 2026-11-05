package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	blobLocal "github.com/cloudposse/atmos/pkg/artifact/blob/local"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Format that wraps a failure to open the artifact directory with the operation's sentinel.
	openDirFormat = "%w: failed to open artifact directory for %s: %w"

	storeName        = "local/dir"
	metadataSuffix   = ".metadata.json"
	defaultDirPerms  = 0o755
	defaultFilePerms = 0o644
)

// Store implements the artifact.Backend interface using the local filesystem.
// It stores each artifact as a single file with a metadata JSON sidecar. Object I/O is delegated
// to the shared transport in pkg/artifact/blob/local; this type only owns the legacy CI
// concerns: artifact naming, the metadata sidecar format, and query matching.
type Store struct {
	basePath string
}

// NewStore creates a new local filesystem artifact backend.
func NewStore(opts artifact.StoreOptions) (artifact.Backend, error) {
	defer perf.Track(opts.AtmosConfig, "artifact.local.NewStore")()

	path, ok := opts.Options["path"].(string)
	if !ok || path == "" {
		path = ".atmos/artifacts"
	}

	// Expand path if it starts with ~.
	if len(path) > 0 && path[0] == '~' {
		home, err := homedir.Dir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Create the base directory if it doesn't exist.
	if err := os.MkdirAll(path, defaultDirPerms); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory %s: %w", path, err)
	}

	return &Store{basePath: path}, nil
}

// Name returns the store type name.
func (s *Store) Name() string {
	return storeName
}

// transport returns the shared filesystem blob store rooted at the base path. The store is
// stateless apart from a per-root lock registry, so it is built on demand.
func (s *Store) transport() (*blobLocal.Store, error) {
	root := s.basePath
	if root == "" {
		root = "."
	}
	return blobLocal.New(root)
}

// key maps a validated artifact name to the transport key. Names are cleaned the same way the
// filesystem would clean them, so "a/./b" and "a//b" address the same object as before.
func (s *Store) key(name string) string {
	rel, err := filepath.Rel(s.basePath, filepath.Join(s.basePath, name))
	if err != nil {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(rel)
}

// Upload uploads a single data stream to the local filesystem.
// Writes the data as a single file and the metadata as a JSON sidecar.
func (s *Store) Upload(ctx context.Context, name string, data io.Reader, size int64, metadata *Metadata) error {
	defer perf.Track(nil, "artifact.local.Upload")()

	if err := s.validateName(name); err != nil {
		return err
	}
	transport, err := s.transport()
	if err != nil {
		return fmt.Errorf(openDirFormat, errUtils.ErrArtifactUploadFailed, name, err)
	}
	key := s.key(name)

	// The declared size is advisory for this backend, as it always was.
	if _, err := transport.Put(ctx, key, data, blob.PutOptions{Size: blob.UnknownSize}); err != nil {
		return fmt.Errorf("%w: failed to write file %s: %w", errUtils.ErrArtifactUploadFailed, name, err)
	}

	// Write metadata sidecar.
	if metadata == nil {
		metadata = &Metadata{
			CreatedAt: time.Now(),
		}
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: failed to marshal metadata for %s: %w", errUtils.ErrArtifactUploadFailed, name, err)
	}
	_, err = transport.Put(ctx, key+metadataSuffix, bytes.NewReader(metadataJSON), blob.PutOptions{Size: int64(len(metadataJSON))})
	if err != nil {
		return fmt.Errorf("%w: failed to write metadata for %s: %w", errUtils.ErrArtifactUploadFailed, name, err)
	}

	return nil
}

// Metadata is an alias for artifact.Metadata used in the local backend.
type Metadata = artifact.Metadata

// Download downloads a single data stream from the local filesystem.
// Returns an io.ReadCloser for the file and the metadata sidecar.
// Callers must close the returned reader when done.
func (s *Store) Download(ctx context.Context, name string) (io.ReadCloser, *artifact.Metadata, error) {
	defer perf.Track(nil, "artifact.local.Download")()

	if err := s.validateName(name); err != nil {
		return nil, nil, err
	}
	transport, err := s.transport()
	if err != nil {
		return nil, nil, fmt.Errorf(openDirFormat, errUtils.ErrArtifactDownloadFailed, name, err)
	}
	key := s.key(name)

	object, err := transport.Get(ctx, key, blob.GetOptions{})
	if err != nil {
		if blob.IsNotFound(err) {
			if s.isDirectory(name) {
				return nil, nil, fmt.Errorf("%w: %s is a directory, not a file", errUtils.ErrArtifactDownloadFailed, name)
			}
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, name)
		}
		return nil, nil, fmt.Errorf("%w: failed to open artifact %s: %w", errUtils.ErrArtifactDownloadFailed, name, err)
	}

	// Load metadata.
	metadata, _ := s.loadMetadata(ctx, transport, key+metadataSuffix)

	return object.Body, metadata, nil
}

// isDirectory reports whether name addresses a directory, which is not an artifact.
func (s *Store) isDirectory(name string) bool {
	info, err := os.Stat(filepath.Join(s.basePath, name))
	return err == nil && info.IsDir()
}

// Delete deletes an artifact from the local filesystem.
// Removes both the artifact file and its metadata sidecar.
// Idempotent — returns nil if the artifact does not exist.
func (s *Store) Delete(ctx context.Context, name string) error {
	defer perf.Track(nil, "artifact.local.Delete")()

	if err := s.validateName(name); err != nil {
		return err
	}
	transport, err := s.transport()
	if err != nil {
		return fmt.Errorf(openDirFormat, errUtils.ErrArtifactDeleteFailed, name, err)
	}
	key := s.key(name)

	// Delete the artifact file; the transport also removes emptied parent directories.
	if err := transport.Delete(ctx, key, blob.DeleteOptions{}); err != nil {
		return fmt.Errorf("%w: failed to delete artifact %s: %w", errUtils.ErrArtifactDeleteFailed, name, err)
	}

	// Delete metadata sidecar (ignore errors if it doesn't exist).
	_ = transport.Delete(ctx, key+metadataSuffix, blob.DeleteOptions{})

	return nil
}

// List lists artifacts matching the given query.
// Finds metadata sidecar files with a corresponding artifact file (not dir),
// applies Query filters, and sorts newest first.
func (s *Store) List(ctx context.Context, query artifact.Query) ([]artifact.ArtifactInfo, error) {
	defer perf.Track(nil, "artifact.local.List")()

	transport, err := s.transport()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open artifact directory: %w", errUtils.ErrArtifactListFailed, err)
	}
	objects, _, err := blob.ListAll(ctx, transport, blob.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list artifacts: %w", errUtils.ErrArtifactListFailed, err)
	}

	byKey := make(map[string]blob.ObjectInfo, len(objects))
	for _, object := range objects {
		byKey[object.Key] = object
	}

	var artifacts []artifact.ArtifactInfo
	for _, object := range objects {
		name, ok := strings.CutSuffix(object.Key, metadataSuffix)
		if !ok {
			continue
		}

		// Only metadata sidecars with a corresponding artifact file are artifacts.
		file, ok := byKey[name]
		if !ok {
			continue
		}

		metadata, err := s.loadMetadata(ctx, transport, object.Key)
		if err != nil {
			continue
		}

		// Apply query filters.
		if !query.All && !s.matchesQuery(metadata, query) {
			continue
		}

		artifacts = append(artifacts, artifact.ArtifactInfo{
			Name:         filepath.FromSlash(name),
			Size:         file.Size,
			LastModified: file.LastModified,
			Metadata:     metadata,
		})
	}

	// Sort by last modified (newest first).
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].LastModified.After(artifacts[j].LastModified)
	})

	return artifacts, nil
}

// Exists checks if an artifact exists as a file (not directory).
func (s *Store) Exists(ctx context.Context, name string) (bool, error) {
	defer perf.Track(nil, "artifact.local.Exists")()

	if err := s.validateName(name); err != nil {
		return false, err
	}
	transport, err := s.transport()
	if err != nil {
		return false, fmt.Errorf("failed to check if artifact %s exists: %w", name, err)
	}

	if _, err := transport.Stat(ctx, s.key(name), blob.StatOptions{}); err != nil {
		if blob.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if artifact %s exists: %w", name, err)
	}
	return true, nil
}

// GetMetadata retrieves metadata for an artifact without downloading the content.
func (s *Store) GetMetadata(ctx context.Context, name string) (*artifact.Metadata, error) {
	defer perf.Track(nil, "artifact.local.GetMetadata")()

	if err := s.validateName(name); err != nil {
		return nil, err
	}
	transport, err := s.transport()
	if err != nil {
		return nil, fmt.Errorf(openDirFormat, errUtils.ErrArtifactMetadataFailed, name, err)
	}
	key := s.key(name)

	// Check if the artifact file exists.
	info, err := transport.Stat(ctx, key, blob.StatOptions{})
	if err != nil {
		if blob.IsNotFound(err) {
			if s.isDirectory(name) {
				return nil, fmt.Errorf("%w: %s is a directory, not a file", errUtils.ErrArtifactNotFound, name)
			}
			return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, name)
		}
		return nil, fmt.Errorf("%w: failed to stat artifact %s: %w", errUtils.ErrArtifactMetadataFailed, name, err)
	}

	metadata, err := s.loadMetadata(ctx, transport, key+metadataSuffix)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load metadata for %s: %w", errUtils.ErrArtifactMetadataFailed, name, err)
	}
	if metadata == nil {
		// Return minimal metadata from file info.
		metadata = &artifact.Metadata{
			CreatedAt: info.LastModified,
		}
	}

	return metadata, nil
}

// validateName ensures the name doesn't escape the base path via path traversal.
func (s *Store) validateName(name string) error {
	fullPath := filepath.Join(s.basePath, name)
	cleanPath := filepath.Clean(fullPath)
	cleanBase := filepath.Clean(s.basePath)

	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
		return fmt.Errorf("%w: name contains path traversal: %s", errUtils.ErrArtifactStoreInvalidArgs, name)
	}
	return nil
}

// loadMetadata loads metadata from a JSON sidecar object. A missing sidecar yields nil metadata.
func (s *Store) loadMetadata(ctx context.Context, transport blob.Store, metadataKey string) (*artifact.Metadata, error) {
	object, err := transport.Get(ctx, metadataKey, blob.GetOptions{})
	if err != nil {
		if blob.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	defer object.Body.Close()

	data, err := io.ReadAll(object.Body)
	if err != nil {
		return nil, err
	}

	var metadata artifact.Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// matchesQuery checks if an artifact's metadata matches the query filters.
func (s *Store) matchesQuery(metadata *artifact.Metadata, query artifact.Query) bool {
	if metadata == nil {
		return false
	}

	// If no filters are set, match everything.
	if len(query.Components) == 0 && len(query.Stacks) == 0 && len(query.SHAs) == 0 {
		return true
	}

	// Check component filter.
	if len(query.Components) > 0 && !slices.Contains(query.Components, metadata.Component) {
		return false
	}

	// Check stack filter.
	if len(query.Stacks) > 0 && !slices.Contains(query.Stacks, metadata.Stack) {
		return false
	}

	// Check SHA filter.
	if len(query.SHAs) > 0 && !slices.Contains(query.SHAs, metadata.SHA) {
		return false
	}

	return true
}

func init() {
	artifact.Register(storeName, NewStore)
}

// Ensure Store implements artifact.Backend.
var _ artifact.Backend = (*Store)(nil)
