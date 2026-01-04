package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "s3"

	// Metadata is stored as a JSON file alongside the planfile.
	metadataSuffix = ".metadata.json"
)

// Store implements the planfile.Store interface using AWS S3.
type Store struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewStore creates a new S3 store.
func NewStore(opts planfile.StoreOptions) (planfile.Store, error) {
	defer perf.Track(opts.AtmosConfig, "s3.NewStore")()

	bucket, ok := opts.Options["bucket"].(string)
	if !ok || bucket == "" {
		return nil, fmt.Errorf("%w: bucket is required for S3 store", errUtils.ErrPlanfileStoreNotFound)
	}

	prefix, _ := opts.Options["prefix"].(string)
	region, _ := opts.Options["region"].(string)

	// Load AWS config.
	var awsOpts []func(*config.LoadOptions) error
	if region != "" {
		awsOpts = append(awsOpts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), awsOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAWSConfigLoadFailed, err)
	}

	client := s3.NewFromConfig(cfg)

	return &Store{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
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

// Upload uploads a planfile to S3.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, metadata *planfile.Metadata) error {
	defer perf.Track(nil, "s3.Upload")()

	fullKey := s.fullKey(key)

	// Read data into buffer (S3 requires knowing content length or using multipart).
	buf, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("%w: failed to read planfile data: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Upload the planfile.
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(fullKey),
		Body:          bytes.NewReader(buf),
		ContentLength: aws.Int64(int64(len(buf))),
	})
	if err != nil {
		return fmt.Errorf("%w: failed to upload planfile to S3: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Upload metadata if provided.
	if metadata != nil {
		metadataKey := fullKey + metadataSuffix
		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: failed to marshal metadata: %w", errUtils.ErrPlanfileUploadFailed, err)
		}

		_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(s.bucket),
			Key:           aws.String(metadataKey),
			Body:          bytes.NewReader(metadataJSON),
			ContentLength: aws.Int64(int64(len(metadataJSON))),
			ContentType:   aws.String("application/json"),
		})
		if err != nil {
			return fmt.Errorf("%w: failed to upload metadata to S3: %w", errUtils.ErrPlanfileUploadFailed, err)
		}
	}

	return nil
}

// Download downloads a planfile from S3.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *planfile.Metadata, error) {
	defer perf.Track(nil, "s3.Download")()

	fullKey := s.fullKey(key)

	// Download the planfile.
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		if isNoSuchKeyError(err) {
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
		}
		return nil, nil, fmt.Errorf("%w: failed to download planfile from S3: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	// Try to load metadata.
	metadata, _ := s.loadMetadata(ctx, fullKey)

	return result.Body, metadata, nil
}

// Delete deletes a planfile from S3.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "s3.Delete")()

	fullKey := s.fullKey(key)

	// Delete the planfile.
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("%w: failed to delete planfile from S3: %w", errUtils.ErrPlanfileDeleteFailed, err)
	}

	// Try to delete metadata (ignore errors).
	_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey + metadataSuffix),
	})

	return nil
}

// List lists planfiles matching the given prefix.
func (s *Store) List(ctx context.Context, prefix string) ([]planfile.PlanfileInfo, error) {
	defer perf.Track(nil, "s3.List")()

	fullPrefix := s.fullKey(prefix)

	var files []planfile.PlanfileInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to list planfiles in S3: %w", errUtils.ErrPlanfileListFailed, err)
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

			files = append(files, planfile.PlanfileInfo{
				Key:          relKey,
				Size:         aws.ToInt64(obj.Size),
				LastModified: lastModified,
				Metadata:     metadata,
			})
		}
	}

	return files, nil
}

// Exists checks if a planfile exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "s3.Exists")()

	fullKey := s.fullKey(key)

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: failed to check if %s exists in S3: %w", errUtils.ErrPlanfileStatFailed, key, err)
	}

	return true, nil
}

// GetMetadata retrieves metadata for a planfile without downloading the content.
func (s *Store) GetMetadata(ctx context.Context, key string) (*planfile.Metadata, error) {
	defer perf.Track(nil, "s3.GetMetadata")()

	fullKey := s.fullKey(key)

	// Check if the planfile exists.
	headResult, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
		}
		return nil, fmt.Errorf("%w: failed to get metadata for %s from S3: %w", errUtils.ErrPlanfileStatFailed, key, err)
	}

	// Try to load metadata from separate file.
	metadata, err := s.loadMetadata(ctx, fullKey)
	if err != nil || metadata == nil {
		// Return minimal metadata from S3 object.
		metadata = &planfile.Metadata{}
		if headResult.LastModified != nil {
			metadata.CreatedAt = *headResult.LastModified
		}
	}

	return metadata, nil
}

// loadMetadata loads metadata from the metadata file in S3.
func (s *Store) loadMetadata(ctx context.Context, planfileKey string) (*planfile.Metadata, error) {
	metadataKey := planfileKey + metadataSuffix

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(metadataKey),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get metadata from S3: %w", errUtils.ErrPlanfileMetadataFailed, err)
	}
	defer result.Body.Close()

	var metadata planfile.Metadata
	if err := json.NewDecoder(result.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("%w: failed to decode metadata JSON: %w", errUtils.ErrPlanfileMetadataFailed, err)
	}

	return &metadata, nil
}

// isNoSuchKeyError checks if the error is a NoSuchKey error.
func isNoSuchKeyError(err error) bool {
	if err == nil {
		return false
	}
	var noSuchKey *types.NoSuchKey
	return errorAs(err, &noSuchKey)
}

// isNotFoundError checks if the error is a NotFound error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var notFound *types.NotFound
	return errorAs(err, &notFound)
}

// errorAs is a helper that wraps errors.As for AWS SDK error types.
func errorAs[T any](err error, target *T) bool {
	for err != nil {
		if t, ok := err.(T); ok {
			*target = t
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

func init() {
	planfile.Register(storeName, NewStore)
}

// Ensure Store implements planfile.Store.
var _ planfile.Store = (*Store)(nil)
