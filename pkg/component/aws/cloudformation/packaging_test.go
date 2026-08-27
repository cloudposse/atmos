package cloudformation

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNeedsPackaging(t *testing.T) {
	assert.False(t, needsPackaging("small template"))
	assert.False(t, needsPackaging(strings.Repeat("a", templateInlineSizeLimit)))
	assert.True(t, needsPackaging(strings.Repeat("a", templateInlineSizeLimit+1)))
}

func TestPackageObjectName(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}

	name := packageObjectName("", info, "abcdef1234567890")
	assert.Equal(t, "dev/vpc/template-abcdef123456.yaml", name)

	prefixed := packageObjectName("assets/", info, "abcdef1234567890")
	assert.Equal(t, "assets/dev/vpc/template-abcdef123456.yaml", prefixed)
}

func TestPackageURL(t *testing.T) {
	url := packageURL(&targetS3Config{Bucket: "my-bucket"}, "dev/vpc/template-abc.yaml")
	assert.Equal(t, "s3://my-bucket/dev/vpc/template-abc.yaml", url)
}

// newS3Backend must construct a real backend for the default (no-identity)
// credential chain — this exercises the artifact.StoreOptions plumbing
// without making any network call (constructing the AWS config is local; see
// environment_test.go's TestBuildAWSConfig_PropagatesRegion for the same
// property on the sibling AWS SDK client path).
func TestNewS3Backend_DefaultIdentity(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}
	s3Target := &targetS3Config{Bucket: "my-bucket", Prefix: "assets/", Region: "us-east-1"}

	backend, err := newS3Backend(atmosConfig, info, s3Target)
	require.NoError(t, err)
	require.NotNil(t, backend)
	assert.Equal(t, "aws/s3", backend.Name())
}

// newS3Backend must route through the identity-aware path (Identity +
// Resolver set) when info.Identity is set and info.AuthManager implements
// types.AuthManager — this skips the default credential chain entirely
// (verified indirectly: construction succeeds without hitting AWS, since the
// identity-aware Store defers auth to the first real operation).
func TestNewS3Backend_WithIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockManager := authtypes.NewMockAuthManager(ctrl)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{Identity: "deploy-role", AuthManager: mockManager}
	s3Target := &targetS3Config{Bucket: "my-bucket"}

	backend, err := newS3Backend(atmosConfig, info, s3Target)
	require.NoError(t, err)
	require.NotNil(t, backend)
}

// newS3Backend must fail loudly, not silently fall back to the default (ambient)
// credential chain, when info.Identity names a specific identity but
// info.AuthManager doesn't implement types.AuthManager — e.g. left nil by a bug
// elsewhere, or a test double of the wrong shape. Falling back silently would
// upload the packaged template using whatever ambient AWS credentials happen to
// be available (the CI runner's own role, a developer's default profile)
// instead of the identity the component explicitly requested.
func TestNewS3Backend_IdentitySetButAuthManagerWrongType_Errors(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{Identity: "deploy-role", AuthManager: nil}
	s3Target := &targetS3Config{Bucket: "my-bucket"}

	_, err := newS3Backend(atmosConfig, info, s3Target)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationIdentityResolutionFailed)
}

// newS3Backend must propagate the underlying store's validation error (e.g. a
// missing bucket) rather than swallowing it.
func TestNewS3Backend_MissingBucket(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	_, err := newS3Backend(atmosConfig, info, &targetS3Config{})
	require.Error(t, err)
}

// stubNewS3Backend overrides the newS3BackendFunc seam for a single test,
// injecting a gomock-generated artifact.Backend so uploadPackage's own logic
// (digest computation, object naming, error wrapping) can be exercised
// without a real S3 call. Auto-restores on cleanup.
func stubNewS3Backend(t *testing.T, backend artifact.Backend, err error) {
	t.Helper()
	original := newS3BackendFunc
	newS3BackendFunc = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _ *targetS3Config) (artifact.Backend, error) {
		return backend, err
	}
	t.Cleanup(func() { newS3BackendFunc = original })
}

// uploadPackage must call Backend.Upload with the correct content-addressed
// name/size and return a package URL + digest built from the same target.
func TestUploadPackage_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockBackend(ctrl)

	templateBody := "AWSTemplateFormatVersion: '2010-09-09'"
	var gotName string
	var gotSize int64
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, name string, data io.Reader, size int64, _ *artifact.Metadata) error {
			gotName = name
			gotSize = size
			body, err := io.ReadAll(data)
			require.NoError(t, err)
			assert.Equal(t, templateBody, string(body))
			return nil
		},
	)
	stubNewS3Backend(t, mockBackend, nil)

	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}
	s3Target := &targetS3Config{Bucket: "my-bucket"}

	pkg, err := uploadPackage(context.Background(), &schema.AtmosConfiguration{}, info, s3Target, templateBody)
	require.NoError(t, err)
	require.NotNil(t, pkg)
	assert.Equal(t, "s3://my-bucket/"+gotName, pkg.URL)
	assert.Len(t, pkg.SHA256, 64, "SHA256 must be a full hex-encoded digest")
	assert.Equal(t, int64(len(templateBody)), gotSize)
	assert.Contains(t, gotName, "dev/vpc/template-")
}

// uploadPackage must wrap the backend's construction error, naming the
// bucket/key it was trying to reach, rather than returning a bare error.
func TestUploadPackage_BackendConstructionError(t *testing.T) {
	stubNewS3Backend(t, nil, errors.New("bucket not accessible"))

	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}
	s3Target := &targetS3Config{Bucket: "my-bucket"}

	_, err := uploadPackage(context.Background(), &schema.AtmosConfiguration{}, info, s3Target, "template body")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket not accessible")
}

// uploadPackage must wrap a real upload failure with the s3:// destination it
// was uploading to, so the error is actionable.
func TestUploadPackage_UploadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockBackend(ctrl)
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("access denied"))
	stubNewS3Backend(t, mockBackend, nil)

	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}
	s3Target := &targetS3Config{Bucket: "my-bucket"}

	_, err := uploadPackage(context.Background(), &schema.AtmosConfiguration{}, info, s3Target, "template body")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "packaging template to s3://my-bucket/")
	assert.Contains(t, err.Error(), "access denied")
}
