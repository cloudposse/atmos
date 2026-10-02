package cloudformation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

func templateDigest(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// A content-addressed object whose sidecar records the same digest is byte-identical, so
// uploadPackage must skip the upload (a strict mock fails the test if Upload is called) and
// still report where the template lives.
func TestUploadPackage_ReusesExistingObject(t *testing.T) {
	body := "AWSTemplateFormatVersion: '2010-09-09'"
	digest := templateDigest(body)
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockBackend(ctrl)
	mockBackend.EXPECT().GetMetadata(gomock.Any(), gomock.Any()).
		Return(&artifact.Metadata{SHA256: digest}, nil)
	stubNewS3BackendExact(t, mockBackend, nil)

	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}
	pkg, err := uploadPackage(context.Background(), &schema.AtmosConfiguration{}, info,
		&targetS3Config{Bucket: "my-bucket", Region: "us-east-1", Prefix: "templates"}, body)
	require.NoError(t, err)
	assert.True(t, pkg.Reused)
	key := "templates/dev/vpc/template-" + digest[:12] + ".yaml"
	assert.Equal(t, "s3://my-bucket/"+key, pkg.S3URI)
	assert.Equal(t, "https://my-bucket.s3.us-east-1.amazonaws.com/"+key, pkg.URL)
	assert.Equal(t, digest, pkg.SHA256)
}

// Every lookup outcome other than "present with a matching digest" must upload, which also
// rewrites the metadata sidecar so the object and its sidecar stay consistent.
func TestUploadPackage_UploadsUnlessAlreadyPublished(t *testing.T) {
	body := "AWSTemplateFormatVersion: '2010-09-09'"
	tests := []struct {
		name     string
		metadata *artifact.Metadata
		err      error
	}{
		{name: "object missing", err: errUtils.ErrArtifactNotFound},
		{name: "sidecar missing leaves an empty digest", metadata: &artifact.Metadata{}},
		{name: "digest mismatch", metadata: &artifact.Metadata{SHA256: "deadbeef"}},
		{name: "lookup denied", err: errors.New("access denied")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockBackend := artifact.NewMockBackend(ctrl)
			mockBackend.EXPECT().GetMetadata(gomock.Any(), gomock.Any()).Return(tt.metadata, tt.err)
			uploaded := false
			mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, r io.Reader, _ int64, md *artifact.Metadata) error {
					uploaded = true
					content, err := io.ReadAll(r)
					require.NoError(t, err)
					assert.Equal(t, body, string(content))
					assert.Equal(t, templateDigest(body), md.SHA256)
					return nil
				})
			stubNewS3BackendExact(t, mockBackend, nil)

			pkg, err := uploadPackage(context.Background(), &schema.AtmosConfiguration{},
				&schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}, &targetS3Config{Bucket: "my-bucket", Region: "us-east-1"}, body)
			require.NoError(t, err)
			assert.True(t, uploaded)
			assert.False(t, pkg.Reused)
		})
	}
}

// A configured prefix with leading or trailing slashes must never produce an S3 key that starts
// with "/" or contains an empty segment, in the key, the S3 URI, or the https URL.
func TestUploadPackage_PrefixSlashesAreTrimmed(t *testing.T) {
	for _, prefix := range []string{"/lead", "lead/", "/lead/", "lead"} {
		t.Run(prefix, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockBackend := artifact.NewMockBackend(ctrl)
			stubNewS3Backend(t, mockBackend, nil)
			mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			block := map[string]any{"bucket": "my-bucket", "region": "us-east-1", "prefix": prefix}
			target, err := s3ConfigFromTarget("artifacts", block)
			require.NoError(t, err)
			assert.Equal(t, "lead", target.Prefix)

			// A target constructed directly (bypassing config normalization) is trimmed too.
			direct := &targetS3Config{Bucket: "my-bucket", Region: "us-east-1", Prefix: prefix}
			pkg, err := uploadPackage(context.Background(), &schema.AtmosConfiguration{},
				&schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}, direct, "Resources: {}")
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(pkg.S3URI, "s3://my-bucket/lead/dev/vpc/"), pkg.S3URI)
			assert.True(t, strings.HasPrefix(pkg.URL, "https://my-bucket.s3.us-east-1.amazonaws.com/lead/dev/vpc/"), pkg.URL)
		})
	}
}

// Publish-only apply reports where the template went through the summary keys the CLI prints.
func TestPackageTemplate_RecordsDestinationInSummary(t *testing.T) {
	tests := []struct {
		name       string
		reused     bool
		wantReused bool
	}{
		{name: "uploaded", reused: false, wantReused: false},
		{name: "already present", reused: true, wantReused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockBackend := artifact.NewMockBackend(ctrl)
			body := strings.Repeat("a", templateInlineSizeLimit+1)
			if tt.reused {
				mockBackend.EXPECT().GetMetadata(gomock.Any(), gomock.Any()).Return(&artifact.Metadata{SHA256: templateDigest(body)}, nil)
			} else {
				mockBackend.EXPECT().GetMetadata(gomock.Any(), gomock.Any()).Return(nil, errUtils.ErrArtifactNotFound)
				mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			}
			stubNewS3BackendExact(t, mockBackend, nil)

			spec := &stackSpec{StackName: "vpc", TemplateBody: body}
			summary := map[string]any{}
			err := prepareTemplateForAPI(previewContext(), spec, summary)
			require.NoError(t, err)

			uri, _ := summary["package_s3_uri"].(string)
			assert.True(t, strings.HasPrefix(uri, "s3://templates/dev/vpc/template-"), uri)
			assert.Equal(t, spec.TemplateURL, summary["package_url"])
			assert.Equal(t, templateDigest(body), summary["package_sha256"])
			assert.Equal(t, tt.wantReused, summary["package_reused"])
		})
	}
}
