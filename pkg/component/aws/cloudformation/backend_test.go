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

func TestResolveS3BackendTarget(t *testing.T) {
	singleTarget := map[string]any{
		"targets": map[string]any{
			"artifacts": map[string]any{"kind": kindAwsS3, "bucket": "my-bucket", "region": "us-east-1"},
		},
	}
	multiTarget := map[string]any{
		"targets": map[string]any{
			"east":   map[string]any{"kind": kindAwsS3, "bucket": "bucket-east", "region": "us-east-1"},
			"west":   map[string]any{"kind": kindAwsS3, "bucket": "bucket-west", "region": "us-west-2"},
			"deploy": map[string]any{"kind": "git"},
		},
	}
	noTarget := map[string]any{"targets": map[string]any{}}

	tests := []struct {
		name        string
		provision   map[string]any
		flagTarget  string
		wantBucket  string
		wantErr     bool
		errContains string
	}{
		{name: "implicit single target", provision: singleTarget, wantBucket: "my-bucket"},
		{name: "no target declared", provision: noTarget, wantErr: true},
		{name: "nil provision section", provision: nil, wantErr: true},
		{name: "ambiguous without flag", provision: multiTarget, wantErr: true},
		{name: "flag selects among multiple", provision: multiTarget, flagTarget: "west", wantBucket: "bucket-west"},
		{name: "flag names non-existent target", provision: multiTarget, flagTarget: "missing", wantErr: true},
		{name: "flag names non-s3 target", provision: multiTarget, flagTarget: "deploy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveS3BackendTarget(tt.provision, tt.flagTarget)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrInvalidAwsCloudFormationSettings))
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantBucket, got.Bucket)
		})
	}
}

func TestFindS3BackendTargets(t *testing.T) {
	provision := map[string]any{
		"targets": map[string]any{
			"good":          map[string]any{"kind": kindAwsS3, "bucket": "good-bucket", "prefix": "templates"},
			"missingBucket": map[string]any{"kind": kindAwsS3},
			"other":         map[string]any{"kind": "git"},
		},
	}

	got := FindS3BackendTargets(provision)
	require.Len(t, got, 1)
	require.Contains(t, got, "good")
	assert.Equal(t, "good-bucket", got["good"].Bucket)
	assert.Equal(t, "templates", got["good"].Prefix)
}

func TestFindS3BackendTargets_Empty(t *testing.T) {
	got := FindS3BackendTargets(nil)
	assert.Empty(t, got)
}

func TestBuildSyntheticBackendConfig(t *testing.T) {
	tests := []struct {
		name            string
		s3cfg           *targetS3Config
		componentConfig map[string]any
		authContext     *schema.AuthContext
		wantRegion      string
	}{
		{
			name:       "region from target",
			s3cfg:      &targetS3Config{Bucket: "b1", Region: "eu-west-1"},
			wantRegion: "eu-west-1",
		},
		{
			name:  "region falls back to component settings",
			s3cfg: &targetS3Config{Bucket: "b1"},
			componentConfig: map[string]any{
				"settings": map[string]any{
					"aws_cloudformation": map[string]any{"region": "ap-southeast-2"},
				},
			},
			wantRegion: "ap-southeast-2",
		},
		{
			name:        "region falls back to auth context",
			s3cfg:       &targetS3Config{Bucket: "b1"},
			authContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Region: "us-west-1"}},
			wantRegion:  "us-west-1",
		},
		{
			name:       "region empty when nothing set",
			s3cfg:      &targetS3Config{Bucket: "b1"},
			wantRegion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSyntheticBackendConfig(tt.s3cfg, tt.componentConfig, tt.authContext)
			assert.Equal(t, backendTypeS3, got["backend_type"])

			backendBlock, ok := got["backend"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.s3cfg.Bucket, backendBlock["bucket"])
			assert.Equal(t, tt.wantRegion, backendBlock["region"])
			_, hasPrefix := backendBlock["prefix"]
			assert.False(t, hasPrefix, "prefix must not leak into the synthetic backend block")

			provisionBlock, ok := got["provision"].(map[string]any)
			require.True(t, ok)
			backendEnabled, ok := provisionBlock["backend"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, true, backendEnabled["enabled"])

			// A CFN artifact bucket holds packaged templates, not Terraform state —
			// these overrides suppress pkg/provisioner/backend's default ".tfstate"/
			// "Terraform state file(s)" deletion-warning wording for this backend.
			assert.Equal(t, "", got["state_file_suffix"])
			assert.Equal(t, "", got["state_file_label"])
		})
	}
}

// fakeS3Client is a minimal S3ClientAPI stub for DescribeS3BackendTarget tests.
type fakeS3Client struct {
	headBucketErr error
}

func (f *fakeS3Client) HeadBucket(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if f.headBucketErr != nil {
		return nil, f.headBucketErr
	}
	return &s3.HeadBucketOutput{}, nil
}

func (f *fakeS3Client) CreateBucket(_ context.Context, _ *s3.CreateBucketInput, _ ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	return &s3.CreateBucketOutput{}, nil
}

func (f *fakeS3Client) PutBucketVersioning(_ context.Context, _ *s3.PutBucketVersioningInput, _ ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
	return &s3.PutBucketVersioningOutput{}, nil
}

func (f *fakeS3Client) PutBucketEncryption(_ context.Context, _ *s3.PutBucketEncryptionInput, _ ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (f *fakeS3Client) PutPublicAccessBlock(_ context.Context, _ *s3.PutPublicAccessBlockInput, _ ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (f *fakeS3Client) PutBucketTagging(_ context.Context, _ *s3.PutBucketTaggingInput, _ ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
	return &s3.PutBucketTaggingOutput{}, nil
}

func (f *fakeS3Client) ListObjectVersions(_ context.Context, _ *s3.ListObjectVersionsInput, _ ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error) {
	return &s3.ListObjectVersionsOutput{}, nil
}

func (f *fakeS3Client) DeleteObjects(_ context.Context, _ *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	return &s3.DeleteObjectsOutput{}, nil
}

func (f *fakeS3Client) DeleteBucket(_ context.Context, _ *s3.DeleteBucketInput, _ ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	return &s3.DeleteBucketOutput{}, nil
}

func TestDescribeS3BackendTarget(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)

	tests := []struct {
		name       string
		client     *fakeS3Client
		wantExists bool
	}{
		{name: "bucket exists", client: &fakeS3Client{}, wantExists: true},
		{name: "bucket does not exist", client: &fakeS3Client{headBucketErr: &types.NotFound{}}, wantExists: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI {
				return tt.client
			})

			s3cfg := &targetS3Config{Bucket: "my-bucket", Region: "us-east-1"}
			status, err := DescribeS3BackendTarget(context.Background(), &schema.AtmosConfiguration{}, s3cfg, nil, nil)
			require.NoError(t, err)
			require.NotNil(t, status)
			assert.Equal(t, tt.wantExists, status.Exists)
			assert.Equal(t, "us-east-1", status.Region)
			assert.Same(t, s3cfg, status.Target)
		})
	}
}

func TestDescribeS3BackendTarget_MissingRegion(t *testing.T) {
	// No target/component/auth-context region: extractS3Config inside
	// backend.S3BackendExists fails fast with ErrRegionRequired.
	s3cfg := &targetS3Config{Bucket: "my-bucket"}
	_, err := DescribeS3BackendTarget(context.Background(), &schema.AtmosConfiguration{}, s3cfg, nil, nil)
	require.Error(t, err)
}
