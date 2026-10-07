package backend

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel so a rename of the SDK field fails the build.
var _ = s3.CreateBucketInput{BucketNamespace: types.BucketNamespace("")}

// captureCreateBucket returns a mock client that records the CreateBucketInput it receives.
func captureCreateBucket(captured **s3.CreateBucketInput) *mockS3Client {
	return &mockS3Client{
		createBucketFunc: func(_ context.Context, params *s3.CreateBucketInput, _ ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
			*captured = params
			return &s3.CreateBucketOutput{}, nil
		},
	}
}

func TestApplyCreateOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []CreateOption
		want string
	}{
		{name: "no options", opts: nil, want: ""},
		{name: "nil option is skipped", opts: []CreateOption{nil}, want: ""},
		{name: "namespace set", opts: []CreateOption{WithBucketNamespace("ns-a")}, want: "ns-a"},
		{name: "last option wins", opts: []CreateOption{WithBucketNamespace("ns-a"), WithBucketNamespace("ns-b")}, want: "ns-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, applyCreateOptions(tt.opts).bucketNamespace)
		})
	}
}

func TestCreateBucket_BucketNamespace(t *testing.T) {
	tests := []struct {
		name           string
		region         string
		opts           []CreateOption
		wantNamespace  types.BucketNamespace
		wantConstraint bool
	}{
		{name: "unset in us-east-1", region: "us-east-1", wantNamespace: "", wantConstraint: false},
		{name: "unset in other region", region: "us-west-2", wantNamespace: "", wantConstraint: true},
		{name: "empty option leaves field unset", region: "us-east-1", opts: []CreateOption{WithBucketNamespace("")}, wantNamespace: "", wantConstraint: false},
		{name: "set in us-east-1 has no constraint", region: "us-east-1", opts: []CreateOption{WithBucketNamespace("opaque-value")}, wantNamespace: "opaque-value", wantConstraint: false},
		{name: "set in other region keeps constraint", region: "eu-west-1", opts: []CreateOption{WithBucketNamespace("opaque-value")}, wantNamespace: "opaque-value", wantConstraint: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *s3.CreateBucketInput
			client := captureCreateBucket(&got)

			require.NoError(t, createBucket(context.Background(), client, "my-bucket", tt.region, tt.opts...))
			require.NotNil(t, got)

			assert.Equal(t, "my-bucket", aws.ToString(got.Bucket))
			assert.Equal(t, tt.wantNamespace, got.BucketNamespace)
			if tt.wantConstraint {
				require.NotNil(t, got.CreateBucketConfiguration)
				assert.Equal(t, types.BucketLocationConstraint(tt.region), got.CreateBucketConfiguration.LocationConstraint)
			} else {
				assert.Nil(t, got.CreateBucketConfiguration)
			}
		})
	}
}

func TestEnsureBucket_ForwardsBucketNamespace(t *testing.T) {
	var got *s3.CreateBucketInput
	client := captureCreateBucket(&got)
	client.headBucketFunc = func(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
		return nil, &types.NotFound{}
	}

	existed, err := ensureBucket(context.Background(), client, "my-bucket", "us-west-2", WithBucketNamespace("opaque-value"))
	require.NoError(t, err)
	assert.False(t, existed)
	require.NotNil(t, got)
	assert.Equal(t, types.BucketNamespace("opaque-value"), got.BucketNamespace)
}

func TestCreateS3Backend_BucketNamespaceWithAssumeRole(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	tests := []struct {
		name          string
		opts          []CreateOption
		wantNamespace types.BucketNamespace
	}{
		{
			name:          "namespace reaches CreateBucket",
			opts:          []CreateOption{WithBucketNamespace(string(types.BucketNamespaceAccountRegional))},
			wantNamespace: types.BucketNamespaceAccountRegional,
		},
		{name: "nothing set when option absent", opts: nil, wantNamespace: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *s3.CreateBucketInput
			client := captureCreateBucket(&got)
			client.headBucketFunc = func(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
				return nil, &types.NotFound{}
			}

			SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) S3ClientAPI { return client })
			t.Cleanup(ResetS3ClientFactory)

			backendConfig := map[string]any{
				"bucket": "templated-name-passes-through",
				"region": "us-west-2",
				"assume_role": map[string]any{
					"role_arn": "arn:aws:iam::123456789012:role/StateAdmin",
				},
			}

			_, err := CreateS3Backend(context.Background(), nil, backendConfig, nil, tt.opts...)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "templated-name-passes-through", aws.ToString(got.Bucket))
			assert.Equal(t, tt.wantNamespace, got.BucketNamespace)
		})
	}
}

func TestValidateBucketNamespace(t *testing.T) {
	// Every value the SDK defines must be accepted, so the check cannot drift from the SDK enum.
	sdkValues := types.BucketNamespace("").Values()
	require.NotEmpty(t, sdkValues)
	for _, v := range sdkValues {
		t.Run("accepts SDK value "+string(v), func(t *testing.T) {
			require.NoError(t, validateBucketNamespace(string(v)))
		})
	}

	t.Run("empty is valid", func(t *testing.T) {
		require.NoError(t, validateBucketNamespace(""))
	})

	for _, bad := range []string{"opaque-value", "Global", " global", "account_regional"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			err := validateBucketNamespace(bad)
			require.ErrorIs(t, err, errUtils.ErrUnsupportedBucketNamespace)
		})
	}
}

func TestCreateS3Backend_RejectsUnsupportedBucketNamespaceBeforeAWSCalls(t *testing.T) {
	calls := 0
	factory := func(aws.Config, ...func(*s3.Options)) S3ClientAPI {
		calls++
		return &mockS3Client{}
	}
	SetS3ClientFactory(factory)
	t.Cleanup(ResetS3ClientFactory)

	backendConfig := map[string]any{"bucket": "my-bucket", "region": "us-west-2"}

	_, err := CreateS3Backend(context.Background(), nil, backendConfig, nil, WithBucketNamespace("opaque-value"))
	require.ErrorIs(t, err, errUtils.ErrUnsupportedBucketNamespace)
	assert.Zero(t, calls, "no S3 client should be built for an unsupported namespace")
}

func TestProvisionBackend_BucketNamespace(t *testing.T) {
	tests := []struct {
		name           string
		provisionBlock map[string]any
		wantCalled     bool
		wantNamespace  string
		wantErr        error
	}{
		{
			name:           "absent forwards empty namespace",
			provisionBlock: map[string]any{"enabled": true},
			wantCalled:     true,
			wantNamespace:  "",
		},
		{
			name:           "nil value forwards empty namespace",
			provisionBlock: map[string]any{"enabled": true, "bucket_namespace": nil},
			wantCalled:     true,
			wantNamespace:  "",
		},
		{
			name:           "string forwarded verbatim",
			provisionBlock: map[string]any{"enabled": true, "bucket_namespace": "opaque-value"},
			wantCalled:     true,
			wantNamespace:  "opaque-value",
		},
		{
			name:           "non-string returns sentinel and skips create",
			provisionBlock: map[string]any{"enabled": true, "bucket_namespace": 42},
			wantCalled:     false,
			wantErr:        errUtils.ErrInvalidBucketNamespace,
		},
		{
			name:           "boolean returns sentinel and skips create",
			provisionBlock: map[string]any{"enabled": true, "bucket_namespace": true},
			wantCalled:     false,
			wantErr:        errUtils.ErrInvalidBucketNamespace,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetBackendRegistry()

			called := false
			var gotNamespace string
			var gotBackendConfig map[string]any
			RegisterBackendCreate("s3", func(_ context.Context, _ *schema.AtmosConfiguration, backendConfig map[string]any, _ *schema.AuthContext, opts ...CreateOption) (*ProvisionResult, error) {
				called = true
				gotBackendConfig = backendConfig
				gotNamespace = applyCreateOptions(opts).bucketNamespace
				return &ProvisionResult{}, nil
			})

			componentConfig := map[string]any{
				"backend_type": "s3",
				"backend": map[string]any{
					"bucket": "test-bucket",
					"region": "us-west-2",
				},
				"provision": map[string]any{"backend": tt.provisionBlock},
			}

			_, err := ProvisionBackend(context.Background(), &schema.AtmosConfiguration{}, componentConfig, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantCalled, called)
			if tt.wantCalled {
				assert.Equal(t, tt.wantNamespace, gotNamespace)
				// The option is read from provision.backend and never leaks into the backend config.
				assert.NotContains(t, gotBackendConfig, "bucket_namespace")
				assert.Equal(t, map[string]any{"bucket": "test-bucket", "region": "us-west-2"}, gotBackendConfig)
			}
		})
	}
}

func TestProvisionBackend_BucketNamespaceIgnoredByNonS3Backend(t *testing.T) {
	resetBackendRegistry()

	called := false
	RegisterBackendCreate("azurerm", func(_ context.Context, _ *schema.AtmosConfiguration, _ map[string]any, _ *schema.AuthContext, _ ...CreateOption) (*ProvisionResult, error) {
		called = true
		return &ProvisionResult{}, nil
	})

	componentConfig := map[string]any{
		"backend_type": "azurerm",
		"backend":      map[string]any{"storage_account_name": "acct"},
		"provision": map[string]any{
			"backend": map[string]any{"enabled": true, "bucket_namespace": "opaque-value"},
		},
	}

	_, err := ProvisionBackend(context.Background(), &schema.AtmosConfiguration{}, componentConfig, nil)
	require.NoError(t, err)
	assert.True(t, called)
}
