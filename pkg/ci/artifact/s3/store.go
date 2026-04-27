package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "aws/s3"

	// Metadata is stored as a JSON file alongside the artifact.
	metadataSuffix = ".metadata.json"
)

// Store implements the artifact.Backend interface using AWS S3.
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
func (s *Store) SetAuthContext(resolver artifact.AuthContextResolver, identityName string) {
	s.authResolver = resolver
	if identityName != "" {
		s.identityName = identityName
	}
}

// initDefaultClient initializes the AWS client using the default credential chain.
func (s *Store) initDefaultClient(ctx context.Context) error {
	var awsOpts []func(*config.LoadOptions) error
	if s.region != "" {
		awsOpts = append(awsOpts, config.WithRegion(s.region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAWSConfigLoadFailed, err)
	}

	s.client = s3.NewFromConfig(cfg)
	return nil
}

// initIdentityClient initializes the AWS client using identity-based credentials
// resolved through the injected AuthContextResolver.
func (s *Store) initIdentityClient(ctx context.Context) error {
	if s.authResolver == nil {
		// No resolver was injected; fall back to the default chain.
		return s.initDefaultClient(ctx)
	}

	authContext, err := s.authResolver.ResolveAWSAuthContext(ctx, s.identityName)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve AWS auth context for identity %q: %w",
			errUtils.ErrAWSConfigLoadFailed, s.identityName, err)
	}

	cfgOpts := s.buildAuthConfigOpts(authContext)
	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAWSConfigLoadFailed, err)
	}

	s.client = s3.NewFromConfig(cfg)
	return nil
}

// buildAuthConfigOpts constructs AWS SDK config options from the auth context.
func (s *Store) buildAuthConfigOpts(authContext *artifact.AWSAuthConfig) []func(*config.LoadOptions) error {
	var cfgOpts []func(*config.LoadOptions) error
	if authContext == nil {
		return cfgOpts
	}
	if authContext.CredentialsFile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedCredentialsFiles([]string{authContext.CredentialsFile}))
	}
	if authContext.ConfigFile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigFiles([]string{authContext.ConfigFile}))
	}
	if authContext.Profile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(authContext.Profile))
	}

	// Use region from auth context if the store doesn't specify one.
	region := s.region
	if region == "" && authContext.Region != "" {
		region = authContext.Region
	}
	if region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	return cfgOpts
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

// Upload uploads a single data stream to S3 with a metadata sidecar.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, size int64, metadata *artifact.Metadata) error {
	defer perf.Track(nil, "s3.Upload")()

	if err := s.ensureClient(ctx); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	fullKey := s.fullKey(key)

	// Read all data into memory for S3 PutObject (needs content length).
	dataBytes, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("%w: failed to read data: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	// Upload the data.
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(fullKey),
		Body:          bytes.NewReader(dataBytes),
		ContentLength: aws.Int64(int64(len(dataBytes))),
	})
	if err != nil {
		return fmt.Errorf("%w: failed to upload artifact to S3: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	// Upload metadata if provided.
	if metadata != nil {
		metadataKey := fullKey + metadataSuffix
		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: failed to marshal metadata: %w", errUtils.ErrArtifactUploadFailed, err)
		}

		_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(s.bucket),
			Key:           aws.String(metadataKey),
			Body:          bytes.NewReader(metadataJSON),
			ContentLength: aws.Int64(int64(len(metadataJSON))),
			ContentType:   aws.String("application/json"),
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

	if err := s.ensureClient(ctx); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	fullKey := s.fullKey(key)

	// Download the data.
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		if isNoSuchKeyError(err) {
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
		}
		return nil, nil, fmt.Errorf("%w: failed to download artifact from S3: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	// Try to load metadata.
	metadata, _ := s.loadMetadata(ctx, fullKey)

	return result.Body, metadata, nil
}

// Delete deletes an artifact from S3.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "s3.Delete")()

	if err := s.ensureClient(ctx); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrArtifactDeleteFailed, err)
	}

	fullKey := s.fullKey(key)

	// Delete the artifact.
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("%w: failed to delete artifact from S3: %w", errUtils.ErrArtifactDeleteFailed, err)
	}

	// Try to delete metadata (ignore errors).
	_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey + metadataSuffix),
	})

	return nil
}

// List lists artifacts matching the given query.
func (s *Store) List(ctx context.Context, query artifact.Query) ([]artifact.ArtifactInfo, error) {
	defer perf.Track(nil, "s3.List")()

	if err := s.ensureClient(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactListFailed, err)
	}

	// Convert query to prefix-based S3 listing.
	prefix := s.queryToPrefix(query)
	fullPrefix := s.fullKey(prefix)

	var files []artifact.ArtifactInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to list artifacts in S3: %w", errUtils.ErrArtifactListFailed, err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)

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

			var lastModified time.Time
			if obj.LastModified != nil {
				lastModified = *obj.LastModified
			}

			files = append(files, artifact.ArtifactInfo{
				Name:         relKey,
				Size:         aws.ToInt64(obj.Size),
				LastModified: lastModified,
				Metadata:     metadata,
			})
		}
	}

	return files, nil
}

// Exists checks if an artifact exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "s3.Exists")()

	if err := s.ensureClient(ctx); err != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrArtifactListFailed, err)
	}

	fullKey := s.fullKey(key)

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: failed to check if %s exists in S3: %w", errUtils.ErrArtifactListFailed, key, err)
	}

	return true, nil
}

// GetMetadata retrieves metadata for an artifact without downloading the content.
func (s *Store) GetMetadata(ctx context.Context, key string) (*artifact.Metadata, error) {
	defer perf.Track(nil, "s3.GetMetadata")()

	if err := s.ensureClient(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactMetadataFailed, err)
	}

	fullKey := s.fullKey(key)

	// Check if the artifact exists.
	headResult, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
		}
		return nil, fmt.Errorf("%w: failed to get metadata for %s from S3: %w", errUtils.ErrArtifactMetadataFailed, key, err)
	}

	// Try to load metadata from separate file.
	metadata, err := s.loadMetadata(ctx, fullKey)
	if err != nil || metadata == nil {
		// Return minimal metadata from S3 object.
		metadata = &artifact.Metadata{}
		if headResult.LastModified != nil {
			metadata.CreatedAt = *headResult.LastModified
		}
	}

	return metadata, nil
}

// loadMetadata loads metadata from the metadata file in S3.
func (s *Store) loadMetadata(ctx context.Context, artifactKey string) (*artifact.Metadata, error) {
	metadataKey := artifactKey + metadataSuffix

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(metadataKey),
	})
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
