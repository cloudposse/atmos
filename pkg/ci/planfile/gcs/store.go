package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/gcp"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "gcs"

	// Metadata is stored as a JSON file alongside the planfile.
	metadataSuffix = ".metadata.json"
)

// Store implements the planfile.Store interface using Google Cloud Storage.
type Store struct {
	// client is the GCS storage client.
	client *storage.Client
	// bucket is the GCS bucket name.
	bucket string
	// prefix is the optional key prefix for all objects.
	prefix string
}

// NewStore creates a new GCS store.
func NewStore(opts planfile.StoreOptions) (planfile.Store, error) {
	defer perf.Track(opts.AtmosConfig, "gcs.NewStore")()

	bucket, ok := opts.Options["bucket"].(string)
	if !ok || bucket == "" {
		return nil, fmt.Errorf("%w: bucket is required for GCS store", errUtils.ErrPlanfileStoreInvalidArgs)
	}

	prefix, _ := opts.Options["prefix"].(string)
	credentials, _ := opts.Options["credentials"].(string)

	// Use unified GCP authentication.
	clientOpts := gcp.GetClientOptions(gcp.AuthOptions{
		Credentials: credentials,
	})

	client, err := storage.NewClient(context.Background(), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrGCSConfigLoadFailed, err)
	}

	return &Store{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

// Name returns the store type name.
func (s *Store) Name() string {
	defer perf.Track(nil, "gcs.Store.Name")()

	return storeName
}

// fullKey returns the full GCS object key with prefix.
func (s *Store) fullKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return strings.TrimRight(s.prefix, "/") + "/" + key
}

// Upload uploads a planfile to GCS.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, metadata *planfile.Metadata) error {
	defer perf.Track(nil, "gcs.Upload")()

	fullKey := s.fullKey(key)

	// Stream directly into the GCS writer to avoid buffering the entire planfile in memory.
	obj := s.client.Bucket(s.bucket).Object(fullKey)
	writer := obj.NewWriter(ctx)

	if _, err := io.Copy(writer, data); err != nil {
		_ = writer.Close()
		return fmt.Errorf("%w: failed to write planfile to GCS: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("%w: failed to upload planfile to GCS: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Upload metadata if provided.
	if metadata != nil {
		if err := s.uploadMetadata(ctx, fullKey, metadata); err != nil {
			// Best-effort rollback: delete the planfile that was already uploaded.
			deleteErr := s.client.Bucket(s.bucket).Object(fullKey).Delete(ctx)
			return errors.Join(
				fmt.Errorf("%w: failed to upload metadata to GCS: %w", errUtils.ErrPlanfileUploadFailed, err),
				deleteErr,
			)
		}
	}

	return nil
}

// uploadMetadata uploads the metadata sidecar JSON file to GCS.
func (s *Store) uploadMetadata(ctx context.Context, planfileKey string, metadata *planfile.Metadata) error {
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metaObj := s.client.Bucket(s.bucket).Object(planfileKey + metadataSuffix)
	metaWriter := metaObj.NewWriter(ctx)
	metaWriter.ContentType = "application/json"

	if _, err := metaWriter.Write(metadataJSON); err != nil {
		_ = metaWriter.Close()
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	if err := metaWriter.Close(); err != nil {
		return fmt.Errorf("failed to close metadata writer: %w", err)
	}

	return nil
}

// Download downloads a planfile from GCS.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *planfile.Metadata, error) {
	defer perf.Track(nil, "gcs.Download")()

	fullKey := s.fullKey(key)

	// Download the planfile.
	obj := s.client.Bucket(s.bucket).Object(fullKey)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
		}
		return nil, nil, fmt.Errorf("%w: failed to download planfile from GCS: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	// Try to load metadata.
	metadata, _ := s.loadMetadata(ctx, fullKey)

	return reader, metadata, nil
}

// Delete deletes a planfile from GCS.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "gcs.Delete")()

	fullKey := s.fullKey(key)

	// Delete the planfile.
	obj := s.client.Bucket(s.bucket).Object(fullKey)
	if err := obj.Delete(ctx); err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("%w: failed to delete planfile from GCS: %w", errUtils.ErrPlanfileDeleteFailed, err)
	}

	// Try to delete metadata (ignore errors).
	metaObj := s.client.Bucket(s.bucket).Object(fullKey + metadataSuffix)
	_ = metaObj.Delete(ctx)

	return nil
}

// List lists planfiles matching the given prefix.
func (s *Store) List(ctx context.Context, prefix string) ([]planfile.PlanfileInfo, error) {
	defer perf.Track(nil, "gcs.List")()

	fullPrefix := s.fullKey(prefix)

	var files []planfile.PlanfileInfo
	it := s.client.Bucket(s.bucket).Objects(ctx, &storage.Query{
		Prefix: fullPrefix,
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: failed to list planfiles in GCS: %w", errUtils.ErrPlanfileListFailed, err)
		}

		key := attrs.Name

		// Skip metadata files.
		if len(key) > len(metadataSuffix) && key[len(key)-len(metadataSuffix):] == metadataSuffix {
			continue
		}

		// Remove prefix to get relative key.
		relKey := key
		if s.prefix != "" && len(key) > len(s.prefix)+1 {
			relKey = key[len(s.prefix)+1:]
		}

		// Try to load metadata.
		metadata, _ := s.loadMetadata(ctx, key)

		files = append(files, planfile.PlanfileInfo{
			Key:          relKey,
			Size:         attrs.Size,
			LastModified: attrs.Updated,
			Metadata:     metadata,
		})
	}

	return files, nil
}

// Exists checks if a planfile exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "gcs.Exists")()

	fullKey := s.fullKey(key)

	obj := s.client.Bucket(s.bucket).Object(fullKey)
	_, err := obj.Attrs(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: failed to check if %s exists in GCS: %w", errUtils.ErrPlanfileStatFailed, key, err)
	}

	return true, nil
}

// GetMetadata retrieves metadata for a planfile without downloading the content.
func (s *Store) GetMetadata(ctx context.Context, key string) (*planfile.Metadata, error) {
	defer perf.Track(nil, "gcs.GetMetadata")()

	fullKey := s.fullKey(key)

	// Check if the planfile exists.
	obj := s.client.Bucket(s.bucket).Object(fullKey)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
		}
		return nil, fmt.Errorf("%w: failed to get metadata for %s from GCS: %w", errUtils.ErrPlanfileStatFailed, key, err)
	}

	// Try to load metadata from separate file.
	metadata, err := s.loadMetadata(ctx, fullKey)
	if err != nil || metadata == nil {
		// Return minimal metadata from GCS object attributes.
		metadata = &planfile.Metadata{
			CreatedAt: attrs.Updated,
		}
	}

	return metadata, nil
}

// loadMetadata loads metadata from the metadata sidecar file in GCS.
func (s *Store) loadMetadata(ctx context.Context, planfileKey string) (*planfile.Metadata, error) {
	metadataKey := planfileKey + metadataSuffix

	obj := s.client.Bucket(s.bucket).Object(metadataKey)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get metadata from GCS: %w", errUtils.ErrPlanfileMetadataFailed, err)
	}
	defer reader.Close()

	var metadata planfile.Metadata
	if err := json.NewDecoder(reader).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("%w: failed to decode metadata JSON: %w", errUtils.ErrPlanfileMetadataFailed, err)
	}

	return &metadata, nil
}

// isNotFoundError checks if the error is or wraps a GCS "object not exist" error.
func isNotFoundError(err error) bool {
	return errors.Is(err, storage.ErrObjectNotExist)
}

// Ensure Store implements planfile.Store at compile time.
var _ planfile.Store = (*Store)(nil)

func init() {
	planfile.Register(storeName, NewStore)
}
