package backend

import (
	"context"
	"os"
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

// captureStdout mirrors pkg/component/aws/cloudformation/executor_test.go's
// helper of the same name, used to assert on data.Write*'s stdout output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestDefaultConfigInitializer_InitConfigAndAuth_FailsFastWithoutRealConfig(t *testing.T) {
	// Run from an empty directory with no Atmos config and a component/stack that can't
	// exist, so this stays a fast, isolated unit test that still exercises the real
	// InitCliConfig -> SetupComponentAuthForCLI wiring.
	t.Chdir(t.TempDir())

	ci := &defaultConfigInitializer{}
	atmosConfig, info, err := ci.InitConfigAndAuth("nonexistent-component", "nonexistent-stack", "")

	assert.Error(t, err)
	assert.Nil(t, atmosConfig)
	assert.Nil(t, info)
}

func TestDefaultConfigInitializer_DescribeComponent_FailsFastWithoutRealConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	ci := &defaultConfigInitializer{}
	_, err := ci.DescribeComponent(&schema.AtmosConfiguration{}, nil, "nonexistent-component", "nonexistent-stack")

	assert.Error(t, err)
}

func TestSetConfigInitializer_NilResetsToDefault(t *testing.T) {
	t.Cleanup(ResetDependencies)

	SetConfigInitializer(NewMockConfigInitializer(nil))
	assert.IsType(t, &MockConfigInitializer{}, configInit)

	SetConfigInitializer(nil)

	assert.IsType(t, &defaultConfigInitializer{}, configInit)
}

func TestSetProvisioner_NilResetsToDefault(t *testing.T) {
	t.Cleanup(ResetDependencies)

	SetProvisioner(NewMockProvisioner(nil))
	assert.IsType(t, &MockProvisioner{}, prov)

	SetProvisioner(nil)

	assert.IsType(t, &defaultProvisioner{}, prov)
}

func singleS3TargetComponentConfig() map[string]any {
	return map[string]any{
		"provision": map[string]any{
			"targets": map[string]any{
				"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket", "region": "us-east-1"},
			},
		},
	}
}

func TestDefaultProvisioner_CreateBackend_NoTarget(t *testing.T) {
	p := &defaultProvisioner{}
	err := p.CreateBackend(context.Background(), &CreateBackendParams{
		AtmosConfig:     &schema.AtmosConfiguration{},
		Component:       "vpc",
		Stack:           "dev",
		ComponentConfig: map[string]any{},
	})
	require.Error(t, err)
}

func TestDefaultProvisioner_CreateBackend_Success(t *testing.T) {
	t.Cleanup(backend.ResetRegistryForTesting)

	called := false
	backend.RegisterBackendCreate("s3", func(_ context.Context, _ *schema.AtmosConfiguration, backendConfig map[string]any, _ *schema.AuthContext) (*backend.ProvisionResult, error) {
		called = true
		assert.Equal(t, "my-bucket", backendConfig["bucket"])
		assert.Equal(t, "us-east-1", backendConfig["region"])
		return &backend.ProvisionResult{}, nil
	})

	p := &defaultProvisioner{}
	err := p.CreateBackend(context.Background(), &CreateBackendParams{
		AtmosConfig:     &schema.AtmosConfiguration{},
		Component:       "vpc",
		Stack:           "dev",
		ComponentConfig: singleS3TargetComponentConfig(),
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestDefaultProvisioner_DeleteBackend_NoTarget(t *testing.T) {
	p := &defaultProvisioner{}
	err := p.DeleteBackend(context.Background(), &DeleteBackendParams{
		CreateBackendParams: CreateBackendParams{
			AtmosConfig:     &schema.AtmosConfiguration{},
			Component:       "vpc",
			Stack:           "dev",
			ComponentConfig: map[string]any{},
		},
		Force: true,
	})
	require.Error(t, err)
}

func TestDefaultProvisioner_DeleteBackend_Success(t *testing.T) {
	t.Cleanup(backend.ResetRegistryForTesting)

	called := false
	backend.RegisterBackendDelete("s3", func(_ context.Context, _ *schema.AtmosConfiguration, backendConfig map[string]any, _ *schema.AuthContext, force bool) error {
		called = true
		assert.True(t, force)
		assert.Equal(t, "my-bucket", backendConfig["bucket"])
		return nil
	})

	p := &defaultProvisioner{}
	err := p.DeleteBackend(context.Background(), &DeleteBackendParams{
		CreateBackendParams: CreateBackendParams{
			AtmosConfig:     &schema.AtmosConfiguration{},
			Component:       "vpc",
			Stack:           "dev",
			ComponentConfig: singleS3TargetComponentConfig(),
		},
		Force: true,
	})
	require.NoError(t, err)
	assert.True(t, called)
}

// fakeS3Client is a minimal S3ClientAPI stub for describe/list tests.
type fakeS3Client struct{ exists bool }

func (f *fakeS3Client) HeadBucket(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if !f.exists {
		return nil, &types.NotFound{}
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

func TestDefaultProvisioner_DescribeBackend(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI {
		return &fakeS3Client{exists: true}
	})

	p := &defaultProvisioner{}

	tests := []struct{ format string }{{"table"}, {"yaml"}, {"json"}, {""}}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			out := captureStdout(t, func() {
				err := p.DescribeBackend(context.Background(), &DescribeBackendParams{
					CreateBackendParams: CreateBackendParams{
						AtmosConfig:     &schema.AtmosConfiguration{},
						Component:       "vpc",
						ComponentConfig: singleS3TargetComponentConfig(),
					},
					Format: tt.format,
				})
				require.NoError(t, err)
			})
			assert.NotEmpty(t, out)
		})
	}
}

func TestDefaultProvisioner_DescribeBackend_NoTarget(t *testing.T) {
	p := &defaultProvisioner{}
	err := p.DescribeBackend(context.Background(), &DescribeBackendParams{
		CreateBackendParams: CreateBackendParams{
			AtmosConfig:     &schema.AtmosConfiguration{},
			ComponentConfig: map[string]any{},
		},
	})
	require.Error(t, err)
}

func TestDefaultProvisioner_ListBackends(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI {
		return &fakeS3Client{exists: false}
	})

	p := &defaultProvisioner{}
	out := captureStdout(t, func() {
		err := p.ListBackends(context.Background(), &ListBackendsParams{
			AtmosConfig:     &schema.AtmosConfiguration{},
			Component:       "vpc",
			ComponentConfig: singleS3TargetComponentConfig(),
			Format:          "table",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "my-bucket")
	assert.Contains(t, out, "does not exist")
}

func TestDefaultProvisioner_ListBackends_Empty(t *testing.T) {
	p := &defaultProvisioner{}
	out := captureStdout(t, func() {
		err := p.ListBackends(context.Background(), &ListBackendsParams{
			AtmosConfig:     &schema.AtmosConfiguration{},
			ComponentConfig: map[string]any{},
			Format:          "table",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "No `kind: aws/s3` provision targets declared.")
}

// renderBackendStatuses must reject an unrecognized --format value with an
// error, rather than silently falling through to the table renderer (the
// same "bad --format is silently swallowed" bug class fixed elsewhere in
// this feature for `output`'s renderOutputsSummary).
func TestRenderBackendStatuses_UnknownFormat(t *testing.T) {
	err := renderBackendStatuses("bogus", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestRenderBackendStatuses_KnownFormats(t *testing.T) {
	for _, format := range []string{"json", "yaml", "table", ""} {
		t.Run(format, func(t *testing.T) {
			err := renderBackendStatuses(format, nil)
			require.NoError(t, err)
		})
	}
}
