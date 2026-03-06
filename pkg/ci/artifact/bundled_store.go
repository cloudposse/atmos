package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// BundledStore implements the Store interface by wrapping a Backend.
// It handles bundling []FileEntry into a tar archive before delegating
// to the backend for single-stream storage, and unbundling on download.
type BundledStore struct {
	backend Backend
}

// NewBundledStore creates a new BundledStore wrapping the given Backend.
func NewBundledStore(backend Backend) *BundledStore {
	return &BundledStore{backend: backend}
}

// Name returns the backend type name.
func (s *BundledStore) Name() string {
	return s.backend.Name()
}

// Upload bundles files into a tar archive, computes SHA256, and delegates to the backend.
func (s *BundledStore) Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error {
	defer perf.Track(nil, "artifact.BundledStore.Upload")()

	// Create tar archive from files.
	tarData, err := CreateTarArchive(files)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	// Compute SHA256 of the tar archive.
	h := sha256.Sum256(tarData)
	sha256Hex := hex.EncodeToString(h[:])

	// Set metadata SHA256.
	if metadata == nil {
		metadata = &Metadata{
			CreatedAt: time.Now(),
		}
	}
	metadata.SHA256 = sha256Hex

	// Delegate to backend.
	return s.backend.Upload(ctx, name, bytes.NewReader(tarData), int64(len(tarData)), metadata)
}

// Download downloads from the backend and extracts files from the tar archive.
func (s *BundledStore) Download(ctx context.Context, name string) ([]FileResult, *Metadata, error) {
	defer perf.Track(nil, "artifact.BundledStore.Download")()

	reader, metadata, err := s.backend.Download(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()

	// Extract files from tar archive.
	files, err := ExtractTarArchive(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	return files, metadata, nil
}

// Delete delegates directly to the backend.
func (s *BundledStore) Delete(ctx context.Context, name string) error {
	defer perf.Track(nil, "artifact.BundledStore.Delete")()

	return s.backend.Delete(ctx, name)
}

// List delegates directly to the backend.
func (s *BundledStore) List(ctx context.Context, query Query) ([]ArtifactInfo, error) {
	defer perf.Track(nil, "artifact.BundledStore.List")()

	return s.backend.List(ctx, query)
}

// Exists delegates directly to the backend.
func (s *BundledStore) Exists(ctx context.Context, name string) (bool, error) {
	defer perf.Track(nil, "artifact.BundledStore.Exists")()

	return s.backend.Exists(ctx, name)
}

// GetMetadata delegates directly to the backend.
func (s *BundledStore) GetMetadata(ctx context.Context, name string) (*Metadata, error) {
	defer perf.Track(nil, "artifact.BundledStore.GetMetadata")()

	return s.backend.GetMetadata(ctx, name)
}

// Ensure BundledStore implements Store.
var _ Store = (*BundledStore)(nil)
