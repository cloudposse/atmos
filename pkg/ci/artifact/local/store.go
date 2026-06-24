package local

import (
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
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName        = "local/dir"
	metadataSuffix   = ".metadata.json"
	defaultDirPerms  = 0o755
	defaultFilePerms = 0o644
)

// Store implements the artifact.Backend interface using the local filesystem.
// It stores each artifact as a single file with a metadata JSON sidecar.
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

// Upload uploads a single data stream to the local filesystem.
// Writes the data as a single file and the metadata as a JSON sidecar.
func (s *Store) Upload(ctx context.Context, name string, data io.Reader, size int64, metadata *Metadata) error {
	defer perf.Track(nil, "artifact.local.Upload")()

	if err := s.validateName(name); err != nil {
		return err
	}

	filePath := filepath.Join(s.basePath, name)

	// Create parent directories for the artifact file.
	if err := os.MkdirAll(filepath.Dir(filePath), defaultDirPerms); err != nil {
		return fmt.Errorf("%w: failed to create directory for %s: %w", errUtils.ErrArtifactUploadFailed, name, err)
	}

	// Write the data stream to a single file.
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("%w: failed to create file %s: %w", errUtils.ErrArtifactUploadFailed, name, err)
	}

	if _, err := io.Copy(f, data); err != nil {
		f.Close()
		return fmt.Errorf("%w: failed to write file %s: %w", errUtils.ErrArtifactUploadFailed, name, err)
	}
	f.Close()

	// Write metadata sidecar.
	if metadata == nil {
		metadata = &Metadata{
			CreatedAt: time.Now(),
		}
	}

	metadataPath := filepath.Join(s.basePath, name+metadataSuffix)

	// Create parent directories for metadata file.
	if err := os.MkdirAll(filepath.Dir(metadataPath), defaultDirPerms); err != nil {
		return fmt.Errorf("%w: failed to create directory for metadata: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: failed to marshal metadata for %s: %w", errUtils.ErrArtifactUploadFailed, name, err)
	}
	if err := os.WriteFile(metadataPath, metadataJSON, defaultFilePerms); err != nil {
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

	filePath := filepath.Join(s.basePath, name)

	// Check if artifact file exists and is not a directory.
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, name)
		}
		return nil, nil, fmt.Errorf("%w: failed to stat artifact %s: %w", errUtils.ErrArtifactDownloadFailed, name, err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("%w: %s is a directory, not a file", errUtils.ErrArtifactDownloadFailed, name)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to open artifact %s: %w", errUtils.ErrArtifactDownloadFailed, name, err)
	}

	// Load metadata.
	metadata, _ := s.loadMetadata(filepath.Join(s.basePath, name+metadataSuffix))

	return f, metadata, nil
}

// Delete deletes an artifact from the local filesystem.
// Removes both the artifact file and its metadata sidecar.
// Idempotent — returns nil if the artifact does not exist.
func (s *Store) Delete(ctx context.Context, name string) error {
	defer perf.Track(nil, "artifact.local.Delete")()

	if err := s.validateName(name); err != nil {
		return err
	}

	filePath := filepath.Join(s.basePath, name)

	// Delete the artifact file.
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: failed to delete artifact %s: %w", errUtils.ErrArtifactDeleteFailed, name, err)
	}

	// Delete metadata sidecar (ignore errors if it doesn't exist).
	_ = os.Remove(filepath.Join(s.basePath, name+metadataSuffix))

	// Try to clean up empty parent directories.
	s.cleanupEmptyDirs(filepath.Dir(filePath))

	return nil
}

// List lists artifacts matching the given query.
// Walks basePath looking for metadata sidecar files with a corresponding artifact file (not dir),
// applies Query filters, and sorts newest first.
func (s *Store) List(ctx context.Context, query artifact.Query) ([]artifact.ArtifactInfo, error) {
	defer perf.Track(nil, "artifact.local.List")()

	var artifacts []artifact.ArtifactInfo

	// Walk basePath looking for metadata sidecar files.
	err := filepath.WalkDir(s.basePath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Skip entries with errors.
		}

		// We only care about .metadata.json files.
		if d.IsDir() || !strings.HasSuffix(path, metadataSuffix) {
			return nil
		}

		// Derive artifact name from metadata path.
		relPath, err := filepath.Rel(s.basePath, path)
		if err != nil {
			return nil
		}
		artifactName := strings.TrimSuffix(relPath, metadataSuffix)

		// Verify the artifact file exists and is not a directory.
		artifactFile := filepath.Join(s.basePath, artifactName)
		fileInfo, err := os.Stat(artifactFile)
		if err != nil || fileInfo.IsDir() {
			return nil
		}

		// Load metadata.
		metadata, err := s.loadMetadata(path)
		if err != nil {
			return nil
		}

		// Apply query filters.
		if !query.All && !s.matchesQuery(metadata, query) {
			return nil
		}

		artifacts = append(artifacts, artifact.ArtifactInfo{
			Name:         artifactName,
			Size:         fileInfo.Size(),
			LastModified: fileInfo.ModTime(),
			Metadata:     metadata,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list artifacts: %w", errUtils.ErrArtifactListFailed, err)
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

	filePath := filepath.Join(s.basePath, name)
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if artifact %s exists: %w", name, err)
	}
	return !info.IsDir(), nil
}

// GetMetadata retrieves metadata for an artifact without downloading the content.
func (s *Store) GetMetadata(ctx context.Context, name string) (*artifact.Metadata, error) {
	defer perf.Track(nil, "artifact.local.GetMetadata")()

	if err := s.validateName(name); err != nil {
		return nil, err
	}

	// Check if the artifact file exists.
	filePath := filepath.Join(s.basePath, name)
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, name)
		}
		return nil, fmt.Errorf("%w: failed to stat artifact %s: %w", errUtils.ErrArtifactMetadataFailed, name, err)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory, not a file", errUtils.ErrArtifactNotFound, name)
	}

	metadataPath := filepath.Join(s.basePath, name+metadataSuffix)
	metadata, err := s.loadMetadata(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load metadata for %s: %w", errUtils.ErrArtifactMetadataFailed, name, err)
	}
	if metadata == nil {
		// Return minimal metadata from file info.
		metadata = &artifact.Metadata{
			CreatedAt: fileInfo.ModTime(),
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

// loadMetadata loads metadata from a JSON sidecar file.
func (s *Store) loadMetadata(metadataPath string) (*artifact.Metadata, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
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

// cleanupEmptyDirs removes empty parent directories up to the base path.
func (s *Store) cleanupEmptyDirs(dir string) {
	for dir != s.basePath && dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		_ = os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

func init() {
	artifact.Register(storeName, NewStore)
}

// Ensure Store implements artifact.Backend.
var _ artifact.Backend = (*Store)(nil)
