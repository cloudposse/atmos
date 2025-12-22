package backend

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ErrR2UploadFailed is returned when uploading to R2 fails.
var ErrR2UploadFailed = fmt.Errorf("failed to upload to R2")

// R2Backend implements the Backend interface for Cloudflare R2.
type R2Backend struct {
	config *R2Config
	client *s3.Client
}

// NewR2Backend creates a new R2 backend with the given configuration.
func NewR2Backend(config *R2Config) (*R2Backend, error) {
	// Create S3-compatible client for R2.
	client := s3.New(s3.Options{
		Region: "auto", // R2 uses auto region
		Credentials: credentials.NewStaticCredentialsProvider(
			config.AccessKeyID,
			config.SecretKey,
			"",
		),
		BaseEndpoint: aws.String(config.GetEndpoint()),
	})

	return &R2Backend{
		config: config,
		client: client,
	}, nil
}

// Validate checks that the backend credentials and configuration are valid by attempting
// to list buckets or head the configured bucket.
func (r *R2Backend) Validate(ctx context.Context) error {
	// Try to head the bucket to verify credentials and bucket access.
	_, err := r.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(r.config.BucketName),
	})
	if err != nil {
		return fmt.Errorf("%w: %v\n\nVerify your credentials and bucket name are correct", ErrR2Validation, err)
	}

	return nil
}

// Upload uploads a file from localPath to the backend at remotePath.
// The title parameter is accepted for interface compatibility but not used by R2.
func (r *R2Backend) Upload(ctx context.Context, localPath, remotePath, _ string) error {
	// Read the file.
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("%w: failed to read file %s: %v", ErrR2UploadFailed, localPath, err)
	}

	// Build the full key with prefix.
	key := r.buildKey(remotePath)

	// Determine content type from file extension.
	contentType := r.getContentType(localPath)

	// Upload to R2.
	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.config.BucketName),
		Key:         aws.String(key),
		Body:        strings.NewReader(string(data)),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("%w: failed to upload %s to R2: %v", ErrR2UploadFailed, localPath, err)
	}

	return nil
}

// GetPublicURL returns the public URL for a file at remotePath.
func (r *R2Backend) GetPublicURL(remotePath string) string {
	key := r.buildKey(remotePath)

	// If base URL is configured, use it.
	if r.config.BaseURL != "" {
		baseURL := strings.TrimSuffix(r.config.BaseURL, "/")
		return fmt.Sprintf("%s/%s", baseURL, key)
	}

	// Otherwise, use the default R2 public URL format.
	// Note: This requires the bucket to be configured with public access.
	return fmt.Sprintf("https://%s/%s", r.config.BucketName, key)
}

// buildKey constructs the full key path including the prefix.
func (r *R2Backend) buildKey(remotePath string) string {
	if r.config.Prefix == "" {
		return remotePath
	}

	prefix := strings.TrimSuffix(r.config.Prefix, "/")
	return fmt.Sprintf("%s/%s", prefix, remotePath)
}

// SupportsFormat checks if R2 backend supports a given file format.
// R2 supports all file formats (acts as fallback for non-video files).
func (r *R2Backend) SupportsFormat(ext string) bool {
	// R2 is object storage and supports all formats.
	return true
}

// getContentType determines the MIME type from file extension.
func (r *R2Backend) getContentType(filePath string) string {
	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)

	if contentType == "" {
		// Default to octet-stream if unknown.
		return "application/octet-stream"
	}

	return contentType
}
