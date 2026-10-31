package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	blobs3 "github.com/cloudposse/atmos/pkg/artifact/blob/s3"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "aws/s3"

	// Metadata is stored as a JSON file alongside the artifact.
	metadataSuffix = ".metadata.json"
)

// Store implements the artifact.Backend interface using AWS S3.
//
// It is a CI adapter over the shared transport in pkg/artifact/blob/s3: this type owns the legacy
// key pattern (prefix joined with path.Join), the ".metadata.json" sidecar, and the legacy error
// sentinels, while client construction and object I/O live in the transport. Unlike the generic
// transport, it keeps the historical fallback to the default credential chain when an identity is
// configured but no resolver was injected.
type Store struct {
	client *s3.Client
	bucket string
	prefix string
	region string

	// Identity-based authentication fields. When identityName is non-empty,
	// initialization is deferred until ensureClient is called.
	identityName string
	authResolver artifact.AuthContextResolver
	initOnce     sync.Once
	initErr      error

	// Shared transport over client, built on first use.
	transportOnce  sync.Once
	transportStore *blobs3.Store
	transportErr   error
}

// NewStore creates a new S3 backend.
// If opts.Identity is empty, the AWS client is initialized eagerly using the
// default credential chain. If non-empty, initialization is deferred until
// first use so the artifact registry can inject an AuthContextResolver.
func NewStore(opts artifact.StoreOptions) (artifact.Backend, error) {
	defer perf.Track(opts.AtmosConfig, "s3.NewStore")()

	bucket, ok := opts.Options["bucket"].(string)
	if !ok || bucket == "" {
		return nil, fmt.Errorf("%w: bucket is required for S3 store", errUtils.ErrArtifactStoreNotFound)
	}

	prefix, _ := opts.Options["prefix"].(string)
	region, _ := opts.Options["region"].(string)

	store := &Store{
		bucket:       bucket,
		prefix:       prefix,
		region:       region,
		identityName: opts.Identity,
	}

	if opts.Identity == "" {
		if err := store.initDefaultClient(context.Background()); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// SetAuthContext implements artifact.IdentityAwareBackend.
// A non-empty identityName overrides the identity supplied at construction.
//
// Must be called before the first Store operation. SetAuthContext writes
// authResolver and identityName without synchronization; ensureClient reads
// them inside initOnce.Do, so calling SetAuthContext concurrently with (or
// after) operations that may trigger ensureClient is unsafe. The artifact
// registry honors this contract by calling SetAuthContext synchronously
// before returning the backend.
func (s *Store) SetAuthContext(resolver artifact.AuthContextResolver, identityName string) {
	s.authResolver = resolver
	if identityName != "" {
		s.identityName = identityName
	}
}

// initDefaultClient initializes the AWS client using the default credential chain.
func (s *Store) initDefaultClient(ctx context.Context) error {
	client, err := blobs3.NewClient(ctx, &blobs3.ClientConfig{Region: s.region, Legacy: true})
	if err != nil {
		return err
	}
	s.client = client
	return nil
}

// initIdentityClient initializes the AWS client using identity-based credentials
// resolved through the injected AuthContextResolver.
func (s *Store) initIdentityClient(ctx context.Context) error {
	// A missing resolver falls back to the default chain: legacy CI behavior that the generic
	// transport deliberately does not share.
	var resolver blobs3.AuthResolver
	if s.authResolver != nil {
		resolver = s.authResolver
	}
	client, err := blobs3.NewClient(ctx, &blobs3.ClientConfig{
		Region:   s.region,
		Identity: s.identityName,
		Resolver: resolver,
		Legacy:   true,
	})
	if err != nil {
		return err
	}
	s.client = client
	return nil
}

// buildClientOptFns constructs S3 client options from the auth context,
// mirroring cloudformation.newClient's EndpointURL override handling.
// Without this, an identity scoped to an emulator (e.g. Floci) still routes
// S3 calls to the real AWS endpoint.
func buildClientOptFns(authContext *artifact.AWSAuthConfig) []func(*s3.Options) {
	return blobs3.BuildClientOptFns(authContext)
}

// buildAuthConfigOpts constructs AWS SDK config options from the auth context.
func (s *Store) buildAuthConfigOpts(authContext *artifact.AWSAuthConfig) []func(*config.LoadOptions) error {
	return blobs3.BuildAuthConfigOpts(s.region, authContext)
}

// ensureClient lazily initializes the AWS client on first operation.
// The client-nil check sits inside initOnce.Do to avoid a data race between
// the unsynchronized read and a write that happens inside Do on another goroutine.
func (s *Store) ensureClient(ctx context.Context) error {
	s.initOnce.Do(func() {
		if s.client != nil {
			return // Already initialized (eager init path).
		}
		if s.identityName == "" {
			s.initErr = s.initDefaultClient(ctx)
		} else {
			s.initErr = s.initIdentityClient(ctx)
		}
	})

	return s.initErr
}

// Name returns the store type name.
func (s *Store) Name() string {
	defer perf.Track(nil, "s3.Store.Name")()

	return storeName
}

// fullKey returns the full S3 key with prefix.
func (s *Store) fullKey(key string) string {
	if s.prefix == "" {
		return key
	}
	//nolint:forbidigo // S3 keys use forward slashes regardless of OS, path.Join is correct here.
	return path.Join(s.prefix, key)
}

// transport returns the shared S3 transport over the lazily initialized client. Keys are passed
// through untouched (fullKey already applied the legacy prefix), so the transport is created
// without a prefix and with opaque-key validation to keep historical key patterns valid.
func (s *Store) transport(ctx context.Context) (*blobs3.Store, error) {
	if err := s.ensureClient(ctx); err != nil {
		return nil, err
	}
	s.transportOnce.Do(func() {
		s.transportStore, s.transportErr = blobs3.NewFromClient(s.client, s.bucket, blobs3.WithOpaqueKeys())
	})
	return s.transportStore, s.transportErr
}

// Upload uploads a single data stream to S3 with a metadata sidecar.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, size int64, metadata *artifact.Metadata) error {
	defer perf.Track(nil, "s3.Upload")()

	transport, err := s.transport(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	fullKey := s.fullKey(key)

	// Upload the data. The declared size is advisory, as it always was; the transport streams the
	// body and uses a multipart upload only for bodies larger than one part.
	if _, err := transport.Put(ctx, fullKey, data, blob.PutOptions{Size: blob.UnknownSize}); err != nil {
		return fmt.Errorf("%w: failed to upload artifact to S3: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	// Upload metadata if provided.
	if metadata != nil {
		metadataKey := fullKey + metadataSuffix
		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: failed to marshal metadata: %w", errUtils.ErrArtifactUploadFailed, err)
		}

		_, err = transport.Put(ctx, metadataKey, bytes.NewReader(metadataJSON), blob.PutOptions{
			Size:        int64(len(metadataJSON)),
			ContentType: "application/json",
		})
		if err != nil {
			return fmt.Errorf("%w: failed to upload metadata to S3: %w", errUtils.ErrArtifactUploadFailed, err)
		}
	}

	return nil
}

// Download downloads a single data stream from S3.
// Returns an io.ReadCloser for the data and the metadata sidecar.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *artifact.Metadata, error) {
	defer perf.Track(nil, "s3.Download")()

	transport, err := s.transport(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	fullKey := s.fullKey(key)

	// Download the data.
	object, err := transport.Get(ctx, fullKey, blob.GetOptions{})
	if err != nil {
		if isMissing(err) {
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
		}
		return nil, nil, fmt.Errorf("%w: failed to download artifact from S3: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	// Try to load metadata.
	metadata, _ := s.loadMetadata(ctx, transport, fullKey)

	return object.Body, metadata, nil
}

// Delete deletes an artifact from S3.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "s3.Delete")()

	transport, err := s.transport(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrArtifactDeleteFailed, err)
	}

	fullKey := s.fullKey(key)

	// Delete the artifact.
	if err := transport.Delete(ctx, fullKey, blob.DeleteOptions{}); err != nil {
		return fmt.Errorf("%w: failed to delete artifact from S3: %w", errUtils.ErrArtifactDeleteFailed, err)
	}

	// Try to delete metadata (ignore errors).
	_ = transport.Delete(ctx, fullKey+metadataSuffix, blob.DeleteOptions{})

	return nil
}

// List lists artifacts matching the given query.
func (s *Store) List(ctx context.Context, query artifact.Query) ([]artifact.ArtifactInfo, error) {
	defer perf.Track(nil, "s3.List")()

	transport, err := s.transport(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactListFailed, err)
	}

	// Convert query to prefix-based S3 listing.
	prefix := s.queryToPrefix(query)
	fullPrefix := s.fullKey(prefix)

	objects, _, err := blob.ListAll(ctx, transport, blob.ListOptions{Prefix: fullPrefix})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list artifacts in S3: %w", errUtils.ErrArtifactListFailed, err)
	}

	var files []artifact.ArtifactInfo
	for _, obj := range objects {
		key := obj.Key

		// Skip metadata files.
		if strings.HasSuffix(key, metadataSuffix) && len(key) > len(metadataSuffix) {
			continue
		}

		// Remove prefix to get relative key.
		relKey := key
		if s.prefix != "" && len(key) > len(s.prefix)+1 {
			relKey = key[len(s.prefix)+1:]
		}

		// Try to load metadata.
		metadata, _ := s.loadMetadata(ctx, transport, key)

		files = append(files, artifact.ArtifactInfo{
			Name:         relKey,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			Metadata:     metadata,
		})
	}

	return files, nil
}

// Exists checks if an artifact exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "s3.Exists")()

	transport, err := s.transport(ctx)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrArtifactListFailed, err)
	}

	fullKey := s.fullKey(key)

	if _, err := transport.Stat(ctx, fullKey, blob.StatOptions{}); err != nil {
		if isMissing(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: failed to check if %s exists in S3: %w", errUtils.ErrArtifactListFailed, key, err)
	}

	return true, nil
}

// GetMetadata retrieves metadata for an artifact without downloading the content.
func (s *Store) GetMetadata(ctx context.Context, key string) (*artifact.Metadata, error) {
	defer perf.Track(nil, "s3.GetMetadata")()

	transport, err := s.transport(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactMetadataFailed, err)
	}

	fullKey := s.fullKey(key)

	// Check if the artifact exists.
	info, err := transport.Stat(ctx, fullKey, blob.StatOptions{})
	if err != nil {
		if isMissing(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
		}
		return nil, fmt.Errorf("%w: failed to get metadata for %s from S3: %w", errUtils.ErrArtifactMetadataFailed, key, err)
	}

	// Try to load metadata from separate file.
	metadata, err := s.loadMetadata(ctx, transport, fullKey)
	if err != nil || metadata == nil {
		// Return minimal metadata from S3 object.
		metadata = &artifact.Metadata{CreatedAt: info.LastModified}
	}

	return metadata, nil
}

// loadMetadata loads metadata from the metadata file in S3.
func (s *Store) loadMetadata(ctx context.Context, transport blob.Store, artifactKey string) (*artifact.Metadata, error) {
	metadataKey := artifactKey + metadataSuffix

	result, err := transport.Get(ctx, metadataKey, blob.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get metadata from S3: %w", errUtils.ErrArtifactMetadataFailed, err)
	}
	defer result.Body.Close()

	var metadata artifact.Metadata
	if err := json.NewDecoder(result.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("%w: failed to decode metadata JSON: %w", errUtils.ErrArtifactMetadataFailed, err)
	}

	return &metadata, nil
}

// queryToPrefix converts an artifact.Query to an S3 prefix string.
func (s *Store) queryToPrefix(query artifact.Query) string {
	if query.All {
		return ""
	}

	var prefix string
	if len(query.Stacks) > 0 {
		prefix = query.Stacks[0]
	}
	if len(query.Components) > 0 && prefix != "" {
		prefix += "/" + query.Components[0]
	}

	return prefix
}

// isMissing reports whether err means the object does not exist, whether it was classified by the
// shared transport or is a raw SDK error.
func isMissing(err error) bool {
	return blob.IsNotFound(err) || isNoSuchKeyError(err) || isNotFoundError(err)
}

// isNoSuchKeyError checks if the error is a NoSuchKey error.
func isNoSuchKeyError(err error) bool {
	var noSuchKey *types.NoSuchKey
	return errors.As(err, &noSuchKey)
}

// isNotFoundError checks if the error is a NotFound error.
func isNotFoundError(err error) bool {
	var notFound *types.NotFound
	return errors.As(err, &notFound)
}

func init() {
	artifact.Register(storeName, NewStore)
}

// Ensure Store implements artifact.Backend and artifact.IdentityAwareBackend.
var (
	_ artifact.Backend              = (*Store)(nil)
	_ artifact.IdentityAwareBackend = (*Store)(nil)
)
