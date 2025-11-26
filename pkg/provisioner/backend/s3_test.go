package backend

import (
	"context"
	"errors"
	"testing"

	//nolint:depguard
	"github.com/aws/aws-sdk-go-v2/service/s3"
	//nolint:depguard
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

//nolint:dupl // Mock struct intentionally mirrors S3ClientAPI interface for testing.
type mockS3Client struct {
	headBucketFunc           func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	createBucketFunc         func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	putBucketVersioningFunc  func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error)
	putBucketEncryptionFunc  func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error)
	putPublicAccessBlockFunc func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)
	putBucketTaggingFunc     func(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error)
	listObjectVersionsFunc   func(ctx context.Context, params *s3.ListObjectVersionsInput, optFns ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
	deleteObjectsFunc        func(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	deleteBucketFunc         func(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
}

func (m *mockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if m.headBucketFunc != nil {
		return m.headBucketFunc(ctx, params, optFns...)
	}
	return &s3.HeadBucketOutput{}, nil
}

func (m *mockS3Client) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	if m.createBucketFunc != nil {
		return m.createBucketFunc(ctx, params, optFns...)
	}
	return &s3.CreateBucketOutput{}, nil
}

func (m *mockS3Client) PutBucketVersioning(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
	if m.putBucketVersioningFunc != nil {
		return m.putBucketVersioningFunc(ctx, params, optFns...)
	}
	return &s3.PutBucketVersioningOutput{}, nil
}

func (m *mockS3Client) PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	if m.putBucketEncryptionFunc != nil {
		return m.putBucketEncryptionFunc(ctx, params, optFns...)
	}
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (m *mockS3Client) PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	if m.putPublicAccessBlockFunc != nil {
		return m.putPublicAccessBlockFunc(ctx, params, optFns...)
	}
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (m *mockS3Client) PutBucketTagging(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
	if m.putBucketTaggingFunc != nil {
		return m.putBucketTaggingFunc(ctx, params, optFns...)
	}
	return &s3.PutBucketTaggingOutput{}, nil
}

func (m *mockS3Client) ListObjectVersions(ctx context.Context, params *s3.ListObjectVersionsInput, optFns ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error) {
	if m.listObjectVersionsFunc != nil {
		return m.listObjectVersionsFunc(ctx, params, optFns...)
	}
	return &s3.ListObjectVersionsOutput{}, nil
}

func (m *mockS3Client) DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	if m.deleteObjectsFunc != nil {
		return m.deleteObjectsFunc(ctx, params, optFns...)
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func (m *mockS3Client) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	if m.deleteBucketFunc != nil {
		return m.deleteBucketFunc(ctx, params, optFns...)
	}
	return &s3.DeleteBucketOutput{}, nil
}

func TestExtractS3Config(t *testing.T) {
	tests := []struct {
		name          string
		backendConfig map[string]any
		want          *s3Config
		wantErr       error
	}{
		{
			name: "valid config with all fields",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "us-west-2",
				"assume_role": map[string]any{
					"role_arn": "arn:aws:iam::123456789012:role/TerraformRole",
				},
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-west-2",
				roleArn: "arn:aws:iam::123456789012:role/TerraformRole",
			},
			wantErr: nil,
		},
		{
			name: "valid config without role ARN",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "us-east-1",
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-east-1",
				roleArn: "",
			},
			wantErr: nil,
		},
		{
			name: "missing bucket",
			backendConfig: map[string]any{
				"region": "us-west-2",
			},
			want:    nil,
			wantErr: errUtils.ErrBucketRequired,
		},
		{
			name: "empty bucket",
			backendConfig: map[string]any{
				"bucket": "",
				"region": "us-west-2",
			},
			want:    nil,
			wantErr: errUtils.ErrBucketRequired,
		},
		{
			name: "missing region",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
			},
			want:    nil,
			wantErr: errUtils.ErrRegionRequired,
		},
		{
			name: "empty region",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "",
			},
			want:    nil,
			wantErr: errUtils.ErrRegionRequired,
		},
		{
			name: "invalid bucket type",
			backendConfig: map[string]any{
				"bucket": 12345,
				"region": "us-west-2",
			},
			want:    nil,
			wantErr: errUtils.ErrBucketRequired,
		},
		{
			name: "invalid region type",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": 12345,
			},
			want:    nil,
			wantErr: errUtils.ErrRegionRequired,
		},
		{
			name: "assume_role with empty role_arn",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "us-west-2",
				"assume_role": map[string]any{
					"role_arn": "",
				},
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-west-2",
				roleArn: "",
			},
			wantErr: nil,
		},
		{
			name: "assume_role with invalid type",
			backendConfig: map[string]any{
				"bucket":      "my-terraform-state",
				"region":      "us-west-2",
				"assume_role": "not-a-map",
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-west-2",
				roleArn: "",
			},
			wantErr: nil,
		},
		{
			name: "complex role ARN",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "eu-west-1",
				"assume_role": map[string]any{
					"role_arn":     "arn:aws:iam::987654321098:role/CrossAccountRole",
					"session_name": "terraform-session", // Extra field (ignored).
				},
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "eu-west-1",
				roleArn: "arn:aws:iam::987654321098:role/CrossAccountRole",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractS3Config(tt.backendConfig)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestS3ProvisionerRegistration(t *testing.T) {
	// Test that S3 provisioner is registered in init().
	provisioner := GetBackendCreate("s3")
	assert.NotNil(t, provisioner, "S3 provisioner should be registered")
}

func TestS3Config_FieldValues(t *testing.T) {
	// Test s3Config structure holds correct values.
	config := &s3Config{
		bucket:  "test-bucket",
		region:  "us-west-2",
		roleArn: "arn:aws:iam::123456789012:role/TestRole",
	}

	assert.Equal(t, "test-bucket", config.bucket)
	assert.Equal(t, "us-west-2", config.region)
	assert.Equal(t, "arn:aws:iam::123456789012:role/TestRole", config.roleArn)
}

func TestExtractS3Config_AllRegions(t *testing.T) {
	// Test various AWS regions.
	regions := []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"eu-west-1",
		"eu-central-1",
		"ap-southeast-1",
		"ap-northeast-1",
	}

	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			backendConfig := map[string]any{
				"bucket": "test-bucket",
				"region": region,
			}

			got, err := extractS3Config(backendConfig)
			require.NoError(t, err)
			assert.Equal(t, region, got.region)
		})
	}
}

func TestExtractS3Config_BucketNameValidation(t *testing.T) {
	// Test various bucket name scenarios.
	tests := []struct {
		name       string
		bucketName any
		shouldPass bool
	}{
		{
			name:       "valid bucket name",
			bucketName: "my-terraform-state-bucket",
			shouldPass: true,
		},
		{
			name:       "bucket with dots",
			bucketName: "my.terraform.state.bucket",
			shouldPass: true,
		},
		{
			name:       "bucket with numbers",
			bucketName: "terraform-state-123456",
			shouldPass: true,
		},
		{
			name:       "nil bucket",
			bucketName: nil,
			shouldPass: false,
		},
		{
			name:       "empty string bucket",
			bucketName: "",
			shouldPass: false,
		},
		{
			name:       "int bucket",
			bucketName: 123,
			shouldPass: false,
		},
		{
			name:       "bool bucket",
			bucketName: true,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backendConfig := map[string]any{
				"bucket": tt.bucketName,
				"region": "us-west-2",
			}

			_, err := extractS3Config(backendConfig)
			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrBucketRequired)
			}
		})
	}
}

func TestErrFormatConstant(t *testing.T) {
	// Verify error format constant.
	assert.Equal(t, "%w: %w", errFormat)
}

// Tests for S3 operations using mock client.

func TestBucketExists_BucketExists(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			assert.Equal(t, "test-bucket", *params.Bucket)
			return &s3.HeadBucketOutput{}, nil
		},
	}

	exists, err := bucketExists(ctx, mockClient, "test-bucket")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestBucketExists_BucketNotFound(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return nil, &types.NotFound{}
		},
	}

	exists, err := bucketExists(ctx, mockClient, "nonexistent-bucket")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestBucketExists_NoSuchBucket(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return nil, &types.NoSuchBucket{}
		},
	}

	exists, err := bucketExists(ctx, mockClient, "nonexistent-bucket")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestBucketExists_NetworkError(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return nil, errors.New("network timeout")
		},
	}

	exists, err := bucketExists(ctx, mockClient, "test-bucket")
	require.Error(t, err)
	assert.False(t, exists)
	// Error wraps errUtils.ErrCheckBucketExist.
	assert.Contains(t, err.Error(), "failed to check bucket existence")
}

func TestCreateBucket_Success(t *testing.T) {
	ctx := context.Background()
	var capturedInput *s3.CreateBucketInput
	mockClient := &mockS3Client{
		createBucketFunc: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
			capturedInput = params
			return &s3.CreateBucketOutput{}, nil
		},
	}

	err := createBucket(ctx, mockClient, "test-bucket", "us-west-2")
	require.NoError(t, err)
	assert.Equal(t, "test-bucket", *capturedInput.Bucket)
	assert.Equal(t, types.BucketLocationConstraint("us-west-2"), capturedInput.CreateBucketConfiguration.LocationConstraint)
}

func TestCreateBucket_UsEast1NoLocationConstraint(t *testing.T) {
	ctx := context.Background()
	var capturedInput *s3.CreateBucketInput
	mockClient := &mockS3Client{
		createBucketFunc: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
			capturedInput = params
			return &s3.CreateBucketOutput{}, nil
		},
	}

	err := createBucket(ctx, mockClient, "test-bucket", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "test-bucket", *capturedInput.Bucket)
	// us-east-1 should not have LocationConstraint.
	assert.Nil(t, capturedInput.CreateBucketConfiguration)
}

func TestCreateBucket_Failure(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		createBucketFunc: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
			return nil, errors.New("bucket already exists in another region")
		},
	}

	err := createBucket(ctx, mockClient, "test-bucket", "us-west-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket already exists")
}

func TestEnsureBucket_BucketAlreadyExists(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return &s3.HeadBucketOutput{}, nil
		},
	}

	alreadyExisted, err := ensureBucket(ctx, mockClient, "existing-bucket", "us-west-2")
	require.NoError(t, err)
	assert.True(t, alreadyExisted)
}

func TestEnsureBucket_CreateNewBucket(t *testing.T) {
	ctx := context.Background()
	createCalled := false
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return nil, &types.NotFound{}
		},
		createBucketFunc: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
			createCalled = true
			return &s3.CreateBucketOutput{}, nil
		},
	}

	alreadyExisted, err := ensureBucket(ctx, mockClient, "new-bucket", "us-west-2")
	require.NoError(t, err)
	assert.False(t, alreadyExisted)
	assert.True(t, createCalled, "CreateBucket should have been called")
}

func TestEnsureBucket_HeadBucketError(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return nil, errors.New("network error")
		},
	}

	_, err := ensureBucket(ctx, mockClient, "test-bucket", "us-west-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCheckBucketExist)
}

func TestEnsureBucket_CreateBucketError(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return nil, &types.NotFound{}
		},
		createBucketFunc: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
			return nil, errors.New("permission denied")
		},
	}

	_, err := ensureBucket(ctx, mockClient, "new-bucket", "us-west-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCreateBucket)
}

func TestEnableVersioning_Success(t *testing.T) {
	ctx := context.Background()
	var capturedInput *s3.PutBucketVersioningInput
	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			capturedInput = params
			return &s3.PutBucketVersioningOutput{}, nil
		},
	}

	err := enableVersioning(ctx, mockClient, "test-bucket")
	require.NoError(t, err)
	assert.Equal(t, "test-bucket", *capturedInput.Bucket)
	assert.Equal(t, types.BucketVersioningStatusEnabled, capturedInput.VersioningConfiguration.Status)
}

func TestEnableVersioning_Failure(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			return nil, errors.New("permission denied")
		},
	}

	err := enableVersioning(ctx, mockClient, "test-bucket")
	require.Error(t, err)
}

func TestEnableEncryption_Success(t *testing.T) {
	ctx := context.Background()
	var capturedInput *s3.PutBucketEncryptionInput
	mockClient := &mockS3Client{
		putBucketEncryptionFunc: func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
			capturedInput = params
			return &s3.PutBucketEncryptionOutput{}, nil
		},
	}

	err := enableEncryption(ctx, mockClient, "test-bucket")
	require.NoError(t, err)
	assert.Equal(t, "test-bucket", *capturedInput.Bucket)
	require.Len(t, capturedInput.ServerSideEncryptionConfiguration.Rules, 1)
	assert.Equal(t, types.ServerSideEncryptionAes256, capturedInput.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault.SSEAlgorithm)
	// BucketKeyEnabled should not be set for AES-256 (only applies to SSE-KMS).
	assert.Nil(t, capturedInput.ServerSideEncryptionConfiguration.Rules[0].BucketKeyEnabled)
}

func TestEnableEncryption_Failure(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putBucketEncryptionFunc: func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
			return nil, errors.New("permission denied")
		},
	}

	err := enableEncryption(ctx, mockClient, "test-bucket")
	require.Error(t, err)
}

func TestBlockPublicAccess_Success(t *testing.T) {
	ctx := context.Background()
	var capturedInput *s3.PutPublicAccessBlockInput
	mockClient := &mockS3Client{
		putPublicAccessBlockFunc: func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
			capturedInput = params
			return &s3.PutPublicAccessBlockOutput{}, nil
		},
	}

	err := blockPublicAccess(ctx, mockClient, "test-bucket")
	require.NoError(t, err)
	assert.Equal(t, "test-bucket", *capturedInput.Bucket)
	assert.True(t, *capturedInput.PublicAccessBlockConfiguration.BlockPublicAcls)
	assert.True(t, *capturedInput.PublicAccessBlockConfiguration.BlockPublicPolicy)
	assert.True(t, *capturedInput.PublicAccessBlockConfiguration.IgnorePublicAcls)
	assert.True(t, *capturedInput.PublicAccessBlockConfiguration.RestrictPublicBuckets)
}

func TestBlockPublicAccess_Failure(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putPublicAccessBlockFunc: func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
			return nil, errors.New("permission denied")
		},
	}

	err := blockPublicAccess(ctx, mockClient, "test-bucket")
	require.Error(t, err)
}

func TestApplyTags_Success(t *testing.T) {
	ctx := context.Background()
	var capturedInput *s3.PutBucketTaggingInput
	mockClient := &mockS3Client{
		putBucketTaggingFunc: func(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
			capturedInput = params
			return &s3.PutBucketTaggingOutput{}, nil
		},
	}

	err := applyTags(ctx, mockClient, "test-bucket")
	require.NoError(t, err)
	assert.Equal(t, "test-bucket", *capturedInput.Bucket)
	require.Len(t, capturedInput.Tagging.TagSet, 2)

	// Find Name and ManagedBy tags.
	var nameTag, managedByTag *types.Tag
	for i := range capturedInput.Tagging.TagSet {
		tag := &capturedInput.Tagging.TagSet[i]
		if *tag.Key == "Name" {
			nameTag = tag
		}
		if *tag.Key == "ManagedBy" {
			managedByTag = tag
		}
	}

	require.NotNil(t, nameTag)
	assert.Equal(t, "test-bucket", *nameTag.Value)
	require.NotNil(t, managedByTag)
	assert.Equal(t, "Atmos", *managedByTag.Value)
}

func TestApplyTags_Failure(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putBucketTaggingFunc: func(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
			return nil, errors.New("permission denied")
		},
	}

	err := applyTags(ctx, mockClient, "test-bucket")
	require.Error(t, err)
}

func TestApplyS3BucketDefaults_NewBucket(t *testing.T) {
	ctx := context.Background()
	versioningCalled := false
	encryptionCalled := false
	publicAccessCalled := false
	taggingCalled := false

	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			versioningCalled = true
			return &s3.PutBucketVersioningOutput{}, nil
		},
		putBucketEncryptionFunc: func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
			encryptionCalled = true
			return &s3.PutBucketEncryptionOutput{}, nil
		},
		putPublicAccessBlockFunc: func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
			publicAccessCalled = true
			return &s3.PutPublicAccessBlockOutput{}, nil
		},
		putBucketTaggingFunc: func(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
			taggingCalled = true
			return &s3.PutBucketTaggingOutput{}, nil
		},
	}

	err := applyS3BucketDefaults(ctx, mockClient, "new-bucket", false)
	require.NoError(t, err)
	assert.True(t, versioningCalled, "Versioning should be enabled")
	assert.True(t, encryptionCalled, "Encryption should be enabled")
	assert.True(t, publicAccessCalled, "Public access should be blocked")
	assert.True(t, taggingCalled, "Tags should be applied")
}

func TestApplyS3BucketDefaults_ExistingBucket(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			callCount++
			return &s3.PutBucketVersioningOutput{}, nil
		},
		putBucketEncryptionFunc: func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
			callCount++
			return &s3.PutBucketEncryptionOutput{}, nil
		},
		putPublicAccessBlockFunc: func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
			callCount++
			return &s3.PutPublicAccessBlockOutput{}, nil
		},
		putBucketTaggingFunc: func(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
			callCount++
			return &s3.PutBucketTaggingOutput{}, nil
		},
	}

	// With alreadyExisted=true, all operations should still be called.
	err := applyS3BucketDefaults(ctx, mockClient, "existing-bucket", true)
	require.NoError(t, err)
	assert.Equal(t, 4, callCount, "All 4 operations should be called")
}

func TestApplyS3BucketDefaults_VersioningFails(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			return nil, errors.New("versioning failed")
		},
	}

	err := applyS3BucketDefaults(ctx, mockClient, "test-bucket", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEnableVersioning)
}

func TestApplyS3BucketDefaults_EncryptionFails(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			return &s3.PutBucketVersioningOutput{}, nil
		},
		putBucketEncryptionFunc: func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
			return nil, errors.New("encryption failed")
		},
	}

	err := applyS3BucketDefaults(ctx, mockClient, "test-bucket", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEnableEncryption)
}

func TestApplyS3BucketDefaults_PublicAccessFails(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			return &s3.PutBucketVersioningOutput{}, nil
		},
		putBucketEncryptionFunc: func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
			return &s3.PutBucketEncryptionOutput{}, nil
		},
		putPublicAccessBlockFunc: func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
			return nil, errors.New("public access block failed")
		},
	}

	err := applyS3BucketDefaults(ctx, mockClient, "test-bucket", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrBlockPublicAccess)
}

func TestApplyS3BucketDefaults_TaggingFails(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{
		putBucketVersioningFunc: func(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
			return &s3.PutBucketVersioningOutput{}, nil
		},
		putBucketEncryptionFunc: func(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
			return &s3.PutBucketEncryptionOutput{}, nil
		},
		putPublicAccessBlockFunc: func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
			return &s3.PutPublicAccessBlockOutput{}, nil
		},
		putBucketTaggingFunc: func(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
			return nil, errors.New("tagging failed")
		},
	}

	err := applyS3BucketDefaults(ctx, mockClient, "test-bucket", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrApplyTags)
}

// Verify mock implements interface.
var _ S3ClientAPI = (*mockS3Client)(nil)

// Additional tests for interface compliance.

func TestS3ClientAPI_InterfaceCompliance(t *testing.T) {
	// This test verifies that our mock properly implements the interface.
	var client S3ClientAPI = &mockS3Client{}
	assert.NotNil(t, client)
}

func TestCreateBucket_AllRegions(t *testing.T) {
	// Test bucket creation with various regions.
	regions := map[string]bool{
		"us-east-1":      false, // No location constraint.
		"us-west-2":      true,  // Has location constraint.
		"eu-west-1":      true,
		"ap-northeast-1": true,
	}

	for region, shouldHaveConstraint := range regions {
		t.Run(region, func(t *testing.T) {
			ctx := context.Background()
			var capturedInput *s3.CreateBucketInput
			mockClient := &mockS3Client{
				createBucketFunc: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
					capturedInput = params
					return &s3.CreateBucketOutput{}, nil
				},
			}

			err := createBucket(ctx, mockClient, "test-bucket", region)
			require.NoError(t, err)

			if shouldHaveConstraint {
				require.NotNil(t, capturedInput.CreateBucketConfiguration)
				assert.Equal(t, types.BucketLocationConstraint(region), capturedInput.CreateBucketConfiguration.LocationConstraint)
			} else {
				assert.Nil(t, capturedInput.CreateBucketConfiguration)
			}
		})
	}
}

// Note: Integration tests for S3 bucket operations (bucketExists, createBucket, etc.)
// with real AWS credentials would be placed in tests/ directory.
// The tests above provide comprehensive unit test coverage using mocked S3 client.
