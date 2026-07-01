package backend

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractS3Config_CustomEndpoint verifies the provisioner reads the standard
// Terraform S3 backend custom-endpoint settings so it can target any
// S3-compatible endpoint (S3-compatible object stores, local emulators).
func TestExtractS3Config_CustomEndpoint(t *testing.T) {
	cfg, err := extractS3Config(map[string]any{
		"bucket":         "my-state",
		"region":         "us-east-1",
		"access_key":     "test",
		"secret_key":     "test",
		"use_path_style": true,
		"endpoints": map[string]any{
			"s3": "http://localhost:4566",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "my-state", cfg.bucket)
	assert.Equal(t, "http://localhost:4566", cfg.endpoint)
	assert.True(t, cfg.usePathStyle)
	assert.Equal(t, "test", cfg.accessKey)
	assert.Equal(t, "test", cfg.secretKey)
}

func TestExtractS3Endpoint(t *testing.T) {
	// Modern endpoints { s3 = ... } block.
	assert.Equal(t, "http://modern", extractS3Endpoint(map[string]any{
		"endpoints": map[string]any{"s3": "http://modern"},
	}))
	// Legacy top-level endpoint key.
	assert.Equal(t, "http://legacy", extractS3Endpoint(map[string]any{
		"endpoint": "http://legacy",
	}))
	// None.
	assert.Empty(t, extractS3Endpoint(map[string]any{}))
}

func TestExtractS3PathStyle(t *testing.T) {
	assert.True(t, extractS3PathStyle(map[string]any{"use_path_style": true}))
	assert.True(t, extractS3PathStyle(map[string]any{"force_path_style": true}))
	assert.True(t, extractS3PathStyle(map[string]any{"s3_use_path_style": true}))
	assert.False(t, extractS3PathStyle(map[string]any{"use_path_style": false}))
	assert.False(t, extractS3PathStyle(map[string]any{}))
}

// TestS3ClientOptions verifies the custom-endpoint fields translate into the
// correct AWS SDK S3 client options.
func TestS3ClientOptions(t *testing.T) {
	opts := s3ClientOptions(&s3Config{endpoint: "http://localhost:4566", usePathStyle: true})
	require.Len(t, opts, 2)

	o := &s3.Options{}
	for _, fn := range opts {
		fn(o)
	}
	require.NotNil(t, o.BaseEndpoint)
	assert.Equal(t, "http://localhost:4566", *o.BaseEndpoint)
	assert.True(t, o.UsePathStyle)

	// No custom endpoint/path-style → no options (default AWS resolution).
	assert.Empty(t, s3ClientOptions(&s3Config{}))
}
