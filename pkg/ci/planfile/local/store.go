package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName        = "local"
	metadataSuffix   = ".metadata.json"
	defaultDirPerms  = 0o755
	defaultFilePerms = 0o644
)

// Store implements the planfile.Store interface using the local filesystem.
type Store struct {
	basePath string
}

// NewStore creates a new local filesystem store.
func NewStore(opts planfile.StoreOptions) (planfile.Store, error) {
	defer perf.Track(opts.AtmosConfig, "local.NewStore")()

	path, ok := opts.Options["path"].(string)
	if !ok || path == "" {
		path = ".atmos/planfiles"
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
		return nil, fmt.Errorf("failed to create planfile directory %s: %w", path, err)
	}

	return &Store{basePath: path}, nil
}

// Name returns the store type name.
func (s *Store) Name() string {
	defer perf.Track(nil, "local.Store.Name")()

	return storeName
}

// Upload uploads a planfile to the local filesystem.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, metadata *planfile.Metadata) error {
	defer perf.Track(nil, "local.Upload")()

	fullPath := filepath.Join(s.basePath, key)

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(fullPath), defaultDirPerms); err != nil {
		return fmt.Errorf("%w: failed to create directory for %s: %w", errUtils.ErrPlanfileUploadFailed, key, err)
	}

	// Write the planfile.
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("%w: failed to create file %s: %w", errUtils.ErrPlanfileUploadFailed, key, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, data); err != nil {
		return fmt.Errorf("%w: failed to write file %s: %w", errUtils.ErrPlanfileUploadFailed, key, err)
	}

	// Write metadata if provided.
	if metadata != nil {
		metadataPath := fullPath + metadataSuffix
		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: failed to marshal metadata for %s: %w", errUtils.ErrPlanfileUploadFailed, key, err)
		}
		if err := os.WriteFile(metadataPath, metadataJSON, defaultFilePerms); err != nil {
			return fmt.Errorf("%w: failed to write metadata for %s: %w", errUtils.ErrPlanfileUploadFailed, key, err)
		}
	}

	return nil
}

// Download downloads a planfile from the local filesystem.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *planfile.Metadata, error) {
	defer perf.Track(nil, "local.Download")()

	fullPath := filepath.Join(s.basePath, key)

	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
		}
		return nil, nil, fmt.Errorf("%w: failed to open file %s: %w", errUtils.ErrPlanfileDownloadFailed, key, err)
	}

	// Try to load metadata.
	metadata, _ := s.loadMetadata(fullPath)

	return f, metadata, nil
}

// Delete deletes a planfile from the local filesystem.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "local.Delete")()

	fullPath := filepath.Join(s.basePath, key)

	// Delete the planfile.
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted.
		}
		return fmt.Errorf("%w: failed to delete file %s: %w", errUtils.ErrPlanfileDeleteFailed, key, err)
	}

	// Try to delete metadata (ignore errors if it doesn't exist).
	_ = os.Remove(fullPath + metadataSuffix)

	// Try to clean up empty parent directories.
	s.cleanupEmptyDirs(filepath.Dir(fullPath))

	return nil
}

// List lists planfiles matching the given prefix.
func (s *Store) List(_ context.Context, prefix string) ([]planfile.PlanfileInfo, error) {
	defer perf.Track(nil, "local.List")()

	searchPath := filepath.Join(s.basePath, prefix)
	var files []planfile.PlanfileInfo

	// Walk the directory tree using WalkDir for cleaner error handling.
	err := filepath.WalkDir(s.basePath, func(path string, d os.DirEntry, walkErr error) error {
		// On error, skip the entry and continue walking.
		if walkErr != nil {
			return filepath.SkipDir
		}

		// Skip directories and metadata files.
		if d.IsDir() || strings.HasSuffix(path, metadataSuffix) {
			return nil
		}

		// Try to add the file to the list, ignoring any errors.
		s.addFileToList(&files, path, d, prefix, searchPath)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list planfiles: %w", errUtils.ErrPlanfileListFailed, err)
	}

	// Sort by last modified (newest first).
	sort.Slice(files, func(i, j int) bool {
		return files[i].LastModified.After(files[j].LastModified)
	})

	return files, nil
}

// Exists checks if a planfile exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "local.Exists")()

	fullPath := filepath.Join(s.basePath, key)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: failed to check if %s exists: %w", errUtils.ErrPlanfileStatFailed, key, err)
	}
	return true, nil
}

// addFileToList adds a file to the planfile list if it matches the criteria.
// Errors are silently ignored to allow walking to continue.
func (s *Store) addFileToList(files *[]planfile.PlanfileInfo, path string, d os.DirEntry, prefix, searchPath string) {
	// Check if the path matches the prefix.
	relPath, err := filepath.Rel(s.basePath, path)
	if err != nil {
		return
	}

	if prefix != "" && !hasPrefix(relPath, prefix) && !hasPrefix(path, searchPath) {
		return
	}

	// Get file info for size and modification time.
	info, err := d.Info()
	if err != nil {
		return
	}

	// Load metadata if available.
	metadata, _ := s.loadMetadata(path)

	*files = append(*files, planfile.PlanfileInfo{
		Key:          relPath,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		Metadata:     metadata,
	})
}

// GetMetadata retrieves metadata for a planfile.
func (s *Store) GetMetadata(ctx context.Context, key string) (*planfile.Metadata, error) {
	defer perf.Track(nil, "local.GetMetadata")()

	fullPath := filepath.Join(s.basePath, key)

	// Check if the planfile exists.
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
		}
		return nil, fmt.Errorf("%w: failed to stat planfile %s: %w", errUtils.ErrPlanfileStatFailed, key, err)
	}

	metadata, err := s.loadMetadata(fullPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load metadata for %s: %w", errUtils.ErrPlanfileMetadataFailed, key, err)
	}
	if metadata == nil {
		// Return minimal metadata from file info.
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get file info for %s: %w", errUtils.ErrPlanfileStatFailed, key, err)
		}
		metadata = &planfile.Metadata{
			CreatedAt: info.ModTime(),
		}
	}

	return metadata, nil
}

// loadMetadata loads metadata from the metadata file.
func (s *Store) loadMetadata(planfilePath string) (*planfile.Metadata, error) {
	metadataPath := planfilePath + metadataSuffix
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metadata planfile.Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
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

// hasPrefix checks if s has the given prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func init() {
	planfile.Register(storeName, NewStore)
}

// Ensure Store implements planfile.Store.
var _ planfile.Store = (*Store)(nil)
