package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
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

// Compile-time sentinel: the tests below rely on this smithy error type's Code field.
var _ = smithy.GenericAPIError{Code: "NoSuchBucket"}

// useFakeS3 installs a tracking S3 client for the duration of the test.
func useFakeS3(t *testing.T, headErr error) *createTrackingS3Client {
	t.Helper()
	t.Cleanup(backend.ResetS3ClientFactory)
	client := &createTrackingS3Client{fakeS3Client: fakeS3Client{headBucketErr: headErr}}
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return client })
	useRealBucketExistenceCheck(t)
	return client
}

func missingBucketHints(err error) string {
	return strings.Join(cockroachErrors.GetAllHints(err), "\n")
}

// Without provision.backend.enabled, a missing bucket must produce the hinted sentinel on every
// read-only packaging verb, never the raw S3 error, and never create or upload anything.
func TestNotEnabledMissingBucketReadOnlyVerbsReturnHintedError(t *testing.T) {
	for _, operation := range []Operation{OperationDiff, OperationValidate, OperationChangesetCreate} {
		t.Run(string(operation), func(t *testing.T) {
			s3Client := useFakeS3(t, &types.NotFound{})
			ctrl := gomock.NewController(t)
			// A strict mock with no expectations fails the test on any upload or lookup.
			stubNewS3BackendExact(t, artifact.NewMockBackend(ctrl), nil)
			client := NewMockCloudFormationClient(ctrl)

			spec := oversizedSpec()
			_, err := runPreview(previewContext(), client, operation, spec)

			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendMissing)
			assert.False(t, s3Client.createBucketCalled)
			assert.Empty(t, spec.TemplateURL)
			assert.Contains(t, cockroachErrors.GetAllDetails(err)[0], `The packaging bucket "templates" does not exist.`)
			hints := missingBucketHints(err)
			assert.Contains(t, hints, "atmos aws cloudformation backend create vpc -s dev")
			assert.Contains(t, hints, "provision.backend.enabled: true")
			assert.NotContains(t, err.Error(), "PutObject")
		})
	}
}

// Apply with provision.backend.enabled unset must not create the bucket either: it fails with the
// same hinted sentinel.
func TestNotEnabledMissingBucketApplyReturnsHintedError(t *testing.T) {
	s3Client := useFakeS3(t, &types.NotFound{})
	ctrl := gomock.NewController(t)
	stubNewS3BackendExact(t, artifact.NewMockBackend(ctrl), nil)

	octx := previewContext()
	provision := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	selected, err := target.SelectTargetWithDefault(provision, "", "default", cfg.CloudFormationComponentType)
	require.NoError(t, err)

	spec := oversizedSpec()
	err = packageIfNeeded(octx, provision, selected, spec, map[string]any{})

	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendMissing)
	assert.False(t, s3Client.createBucketCalled, "apply must not create the bucket unless provision.backend.enabled is true")
	assert.Empty(t, spec.TemplateURL)
	assert.Contains(t, missingBucketHints(err), "atmos aws cloudformation backend create vpc -s dev")
}

// Negative path: an existence-check failure (not "false") is deferred to the upload's own error.
func TestNotEnabledExistenceCheckFailureFallsThroughToUpload(t *testing.T) {
	useFakeS3(t, errors.New("access denied probing bucket"))
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockBackend(ctrl)
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	stubNewS3Backend(t, mockBackend, nil)
	client := NewMockCloudFormationClient(ctrl)

	spec := oversizedSpec()
	expectPreviewRequest(t, client, OperationValidate, spec)
	_, err := runPreview(previewContext(), client, OperationValidate, spec)

	require.NoError(t, err)
	assert.NotEmpty(t, spec.TemplateURL)
}

// Even when the pre-check is skipped or races, an S3 NoSuchBucket from the upload maps to the
// hinted sentinel instead of surfacing the raw SDK error.
func TestUploadNoSuchBucketMapsToHintedError(t *testing.T) {
	useFakeS3(t, nil) // The bucket looks present to the probe.
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockBackend(ctrl)
	raw := fmt.Errorf("%w: failed to upload artifact to S3: %w", errUtils.ErrArtifactUploadFailed,
		&smithy.GenericAPIError{Code: "NoSuchBucket", Message: "The specified bucket does not exist."})
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(raw)
	stubNewS3Backend(t, mockBackend, nil)
	client := NewMockCloudFormationClient(ctrl)

	spec := oversizedSpec()
	_, err := runPreview(previewContext(), client, OperationDiff, spec)

	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendMissing)
	assert.NotContains(t, err.Error(), "PutObject")
	assert.Contains(t, missingBucketHints(err), "atmos aws cloudformation backend create vpc -s dev")
}

// Negative path: other upload failures (here AccessDenied) are not remapped.
func TestUploadOtherErrorIsNotMappedToBackendMissing(t *testing.T) {
	useFakeS3(t, nil)
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockBackend(ctrl)
	raw := fmt.Errorf("failed to upload artifact to S3: %w", &smithy.GenericAPIError{Code: "AccessDenied", Message: "denied"})
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(raw)
	stubNewS3Backend(t, mockBackend, nil)
	client := NewMockCloudFormationClient(ctrl)

	_, err := runPreview(previewContext(), client, OperationDiff, oversizedSpec())

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendMissing)
	assert.ErrorContains(t, err, "AccessDenied")
}

func TestIsNoSuchBucket(t *testing.T) {
	assert.True(t, isNoSuchBucket(fmt.Errorf("wrapped: %w", &smithy.GenericAPIError{Code: "NoSuchBucket"})))
	assert.False(t, isNoSuchBucket(&smithy.GenericAPIError{Code: "NoSuchKey"}))
	assert.False(t, isNoSuchBucket(errors.New("NoSuchBucket")))
	assert.False(t, isNoSuchBucket(nil))
}

// ensurePackagingBucket with the opt-in enabled and a read-only verb keeps the original
// enabled-specific wording that points at apply.
func TestEnsurePackagingBucketEnabledReadOnlyKeepsApplyHint(t *testing.T) {
	useFakeS3(t, &types.NotFound{})
	err := ensurePackagingBucket(context.Background(), autoProvisionArgs{
		AtmosConfig:     previewContext().AtmosConfig,
		S3Target:        &targetS3Config{Bucket: "templates", Region: "us-east-1"},
		ComponentConfig: enabledProvisionConfig(),
		Component:       "vpc",
		Stack:           "dev",
	}, false)

	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendMissing)
	assert.Contains(t, missingBucketHints(err), "atmos aws cloudformation apply vpc -s dev")
}
