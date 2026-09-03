package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsBackendProvisionEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{name: "nil config", config: nil, want: false},
		{name: "no provision section", config: map[string]any{}, want: false},
		{name: "provision section not a map", config: map[string]any{"provision": "nope"}, want: false},
		{name: "no backend section", config: map[string]any{"provision": map[string]any{}}, want: false},
		{
			name:   "enabled false",
			config: map[string]any{"provision": map[string]any{"backend": map[string]any{"enabled": false}}},
			want:   false,
		},
		{
			name:   "enabled not a bool",
			config: map[string]any{"provision": map[string]any{"backend": map[string]any{"enabled": "true"}}},
			want:   false,
		},
		{
			name:   "enabled true",
			config: map[string]any{"provision": map[string]any{"backend": map[string]any{"enabled": true}}},
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isBackendProvisionEnabled(tt.config))
		})
	}
}

// createTrackingS3Client wraps fakeS3Client to record whether CreateBucket
// was actually invoked, proving auto-provision either did or didn't attempt
// creation rather than just returning without error either way.
type createTrackingS3Client struct {
	fakeS3Client
	createBucketCalled bool
	createBucketErr    error
}

func (c *createTrackingS3Client) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	c.createBucketCalled = true
	if c.createBucketErr != nil {
		return nil, c.createBucketErr
	}
	return c.fakeS3Client.CreateBucket(ctx, params, optFns...)
}

func enabledProvisionConfig() map[string]any {
	return map[string]any{"provision": map[string]any{"backend": map[string]any{"enabled": true}}}
}

// autoProvisionBackendIfEnabled must not touch AWS at all when
// provision.backend.enabled isn't set — unchanged behavior for every
// existing component that hasn't opted in.
func TestAutoProvisionBackendIfEnabled_Disabled_NoOp(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	client := &createTrackingS3Client{}
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return client })

	s3cfg := &targetS3Config{Bucket: "my-bucket", Region: "us-east-1"}
	err := autoProvisionBackendIfEnabled(context.Background(), autoProvisionArgs{
		AtmosConfig:     &schema.AtmosConfiguration{},
		S3Target:        s3cfg,
		ComponentConfig: map[string]any{},
		Component:       "vpc",
		Stack:           "dev",
	})
	require.NoError(t, err)
	assert.False(t, client.createBucketCalled, "CreateBucket must not be called when provision.backend.enabled is unset")
}

// autoProvisionBackendIfEnabled must not create a bucket that already
// exists — only the existence check runs, no reconciliation, unlike the
// explicit `backend create`/`update` commands.
func TestAutoProvisionBackendIfEnabled_AlreadyExists_NoOp(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	client := &createTrackingS3Client{} // HeadBucket succeeds by default => exists.
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return client })

	s3cfg := &targetS3Config{Bucket: "my-bucket", Region: "us-east-1"}
	err := autoProvisionBackendIfEnabled(context.Background(), autoProvisionArgs{
		AtmosConfig:     &schema.AtmosConfiguration{},
		S3Target:        s3cfg,
		ComponentConfig: enabledProvisionConfig(),
		Component:       "vpc",
		Stack:           "dev",
	})
	require.NoError(t, err)
	assert.False(t, client.createBucketCalled, "CreateBucket must not be called when the bucket already exists")
}

// autoProvisionBackendIfEnabled must create the bucket when enabled and
// missing — the actual auto-provisioning behavior from-rain.mdx documents.
func TestAutoProvisionBackendIfEnabled_Missing_Creates(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	client := &createTrackingS3Client{fakeS3Client: fakeS3Client{headBucketErr: &types.NotFound{}}}
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return client })

	s3cfg := &targetS3Config{Bucket: "my-bucket", Region: "us-east-1"}
	err := autoProvisionBackendIfEnabled(context.Background(), autoProvisionArgs{
		AtmosConfig:     &schema.AtmosConfiguration{},
		S3Target:        s3cfg,
		ComponentConfig: enabledProvisionConfig(),
		Component:       "vpc",
		Stack:           "dev",
	})
	require.NoError(t, err)
	assert.True(t, client.createBucketCalled, "CreateBucket must be called when enabled and the bucket is missing")
}

// autoProvisionBackendIfEnabled must return a hard error when creation
// fails — apply must not silently continue into uploadPackage against a
// bucket that may not exist.
func TestAutoProvisionBackendIfEnabled_CreateFails_ReturnsError(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	client := &createTrackingS3Client{
		fakeS3Client:    fakeS3Client{headBucketErr: &types.NotFound{}},
		createBucketErr: errors.New("access denied"),
	}
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return client })

	s3cfg := &targetS3Config{Bucket: "my-bucket", Region: "us-east-1"}
	err := autoProvisionBackendIfEnabled(context.Background(), autoProvisionArgs{
		AtmosConfig:     &schema.AtmosConfiguration{},
		S3Target:        s3cfg,
		ComponentConfig: enabledProvisionConfig(),
		Component:       "vpc",
		Stack:           "dev",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	assert.True(t, client.createBucketCalled)
}

// autoProvisionBackendIfEnabled must defer to uploadPackage's own error
// (silent no-op here) when the existence check itself fails, matching
// Terraform's own auto-provision hook's leniency for this specific failure.
func TestAutoProvisionBackendIfEnabled_ExistenceCheckFails_DefersSilently(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	client := &createTrackingS3Client{fakeS3Client: fakeS3Client{headBucketErr: errors.New("throttled")}}
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return client })

	s3cfg := &targetS3Config{Bucket: "my-bucket", Region: "us-east-1"}
	err := autoProvisionBackendIfEnabled(context.Background(), autoProvisionArgs{
		AtmosConfig:     &schema.AtmosConfiguration{},
		S3Target:        s3cfg,
		ComponentConfig: enabledProvisionConfig(),
		Component:       "vpc",
		Stack:           "dev",
	})
	require.NoError(t, err)
	assert.False(t, client.createBucketCalled)
}
