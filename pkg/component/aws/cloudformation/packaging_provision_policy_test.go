package cloudformation

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
)

// backendEnabledContext is previewContext with provision.backend.enabled: true.
func backendEnabledContext() *opContext {
	octx := previewContext()
	provision := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	provision["backend"] = map[string]any{"enabled": true}
	return octx
}

func oversizedSpec() *stackSpec {
	return &stackSpec{StackName: "vpc", TemplateBody: strings.Repeat("a", templateInlineSizeLimit+1)}
}

// Only apply/deploy may create the packaging bucket. The validate, diff and changeset create
// verbs must fail with a hint when the bucket is missing, never create it, and never upload.
func TestPreviewVerbsDoNotProvisionMissingBucket(t *testing.T) {
	for _, operation := range []Operation{OperationDiff, OperationValidate, OperationChangesetCreate} {
		t.Run(string(operation), func(t *testing.T) {
			t.Cleanup(backend.ResetS3ClientFactory)
			s3Client := &createTrackingS3Client{fakeS3Client: fakeS3Client{headBucketErr: &types.NotFound{}}}
			backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return s3Client })

			ctrl := gomock.NewController(t)
			// A strict mock with no expectations fails the test on any upload or lookup.
			stubNewS3BackendExact(t, artifact.NewMockBackend(ctrl), nil)
			client := NewMockCloudFormationClient(ctrl)

			spec := oversizedSpec()
			_, err := runPreview(backendEnabledContext(), client, operation, spec)

			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendMissing)
			assert.False(t, s3Client.createBucketCalled, "%s must not create the bucket", operation)
			assert.Empty(t, spec.TemplateURL)
			hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
			assert.Contains(t, hints, "atmos aws cloudformation backend create vpc -s dev")
			assert.Contains(t, hints, "atmos aws cloudformation apply vpc -s dev")
		})
	}
}

// Negative path: when the bucket already exists the preview verbs package and proceed normally,
// still without creating anything. Without provision.backend.enabled the bucket is not even probed.
func TestPreviewVerbsPackageWhenBucketExistsOrBackendNotEnabled(t *testing.T) {
	tests := []struct {
		name         string
		octx         func() *opContext
		wantHeadCall bool
	}{
		{name: "backend enabled and bucket exists", octx: backendEnabledContext, wantHeadCall: true},
		{name: "backend not enabled", octx: previewContext, wantHeadCall: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(backend.ResetS3ClientFactory)
			factoryCalls := 0
			s3Client := &createTrackingS3Client{}
			backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI {
				factoryCalls++
				return s3Client
			})

			ctrl := gomock.NewController(t)
			mockBackend := artifact.NewMockBackend(ctrl)
			mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			stubNewS3Backend(t, mockBackend, nil)
			client := NewMockCloudFormationClient(ctrl)

			spec := oversizedSpec()
			expectPreviewRequest(t, client, OperationValidate, spec)
			_, err := runPreview(tt.octx(), client, OperationValidate, spec)

			require.NoError(t, err)
			assert.NotEmpty(t, spec.TemplateURL)
			assert.False(t, s3Client.createBucketCalled)
			assert.Equal(t, tt.wantHeadCall, factoryCalls > 0)
		})
	}
}

// Apply keeps provisioning the bucket (the documented behavior), via packageIfNeeded.
func TestPackageIfNeededStillProvisionsMissingBucket(t *testing.T) {
	t.Cleanup(backend.ResetS3ClientFactory)
	s3Client := &createTrackingS3Client{fakeS3Client: fakeS3Client{headBucketErr: &types.NotFound{}}}
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return s3Client })

	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockBackend(ctrl)
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	stubNewS3Backend(t, mockBackend, nil)

	octx := backendEnabledContext()
	octx.Ctx = context.Background()
	provision := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	selected, err := target.SelectTargetWithDefault(provision, "", "default", cfg.CloudFormationComponentType)
	require.NoError(t, err)

	spec := oversizedSpec()
	require.NoError(t, packageIfNeeded(octx, provision, selected, spec, map[string]any{}))
	assert.True(t, s3Client.createBucketCalled, "apply must create the missing bucket")
	assert.NotEmpty(t, spec.TemplateURL)
}

// The packaging-ambiguity error must explain how to declare the deploy-style target.
func TestResolvePackagingTargetAmbiguityExplainsDeployTarget(t *testing.T) {
	provision := map[string]any{"targets": map[string]any{
		"a": map[string]any{"kind": kindAwsS3, "bucket": "a", "region": "us-east-1"},
		"b": map[string]any{"kind": kindAwsS3, "bucket": "b", "region": "us-east-1"},
	}}
	implicit := &target.SelectedTarget{Name: "default", Kind: cfg.CloudFormationComponentType}

	_, err := resolvePackagingTarget(provision, implicit)
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
	assert.Contains(t, hints, "kind: aws/cloudformation")
	assert.Contains(t, hints, "packaging:")
	assert.Contains(t, hints, "default:")
	assert.Contains(t, cockroachErrors.GetAllDetails(err)[0], "a, b")

	// An explicit deploy target that names one of them resolves.
	explicit := &target.SelectedTarget{Name: "deploy", Kind: cfg.CloudFormationComponentType, Config: map[string]any{"packaging": "b"}}
	got, err := resolvePackagingTarget(provision, explicit)
	require.NoError(t, err)
	assert.Equal(t, "b", got.Name)

	// A packaging name that matches nothing lists the valid names.
	explicit.Config["packaging"] = "missing"
	_, err = resolvePackagingTarget(provision, explicit)
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "a, b")
}
