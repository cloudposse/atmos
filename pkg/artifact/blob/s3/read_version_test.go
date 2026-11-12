package s3

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

// errTaggingDenied stands in for a version-specific tagging denial.
var errTaggingDenied = errors.New("tagging denied")

// versionedAPI wraps a real API, reporting a fixed HeadObject version ID and recording the
// GetObjectTagging inputs. A non-nil taggingErr fails every tagging request.
type versionedAPI struct {
	API
	versionID  *string
	taggingErr error
	tagging    []*s3.GetObjectTaggingInput
}

func (a *versionedAPI) HeadObject(ctx context.Context, in *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	out, err := a.API.HeadObject(ctx, in, optFns...)
	if err != nil {
		return nil, err
	}
	out.VersionId = a.versionID
	return out, nil
}

func (a *versionedAPI) GetObjectTagging(ctx context.Context, in *s3.GetObjectTaggingInput, optFns ...func(*s3.Options)) (*s3.GetObjectTaggingOutput, error) {
	a.tagging = append(a.tagging, in)
	if a.taggingErr != nil {
		return nil, a.taggingErr
	}
	// The fake endpoint is unversioned, so serve the current tags for any requested version.
	plain := *in
	plain.VersionId = nil
	return a.API.GetObjectTagging(ctx, &plain, optFns...)
}

func TestStatPinsTagReadToTheHeadedVersion(t *testing.T) {
	tests := []struct {
		name      string
		versionID *string
		want      *string
	}{
		{name: "versioned bucket pins the read", versionID: aws.String("3HL4kqtJlcpXroDTDmJ"), want: aws.String("3HL4kqtJlcpXroDTDmJ")},
		{name: "unversioned bucket reads current tags", versionID: nil, want: nil},
		{name: "empty version reads current tags", versionID: aws.String(""), want: nil},
		{name: "replaceable null version reads current tags", versionID: aws.String(nullVersionID), want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFake(t)
			api := &versionedAPI{API: fake.Client(), versionID: tc.versionID}
			s := newVersionedStore(t, fake, api)
			putBytes(t, s, "k", []byte("x"), blob.PutOptions{Tags: map[string]string{"a": "b"}})

			info, err := s.Stat(context.Background(), "k", blob.StatOptions{IncludeTags: true})
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"a": "b"}, info.Tags)
			require.Len(t, api.tagging, 1)
			assert.Equal(t, aws.ToString(tc.want), aws.ToString(api.tagging[0].VersionId))
			assert.Equal(t, tc.want == nil, api.tagging[0].VersionId == nil)
		})
	}
}

func TestStatDoesNotFallBackWhenVersionedTagReadFails(t *testing.T) {
	fake := newFake(t)
	api := &versionedAPI{API: fake.Client(), versionID: aws.String("v1")}
	s := newVersionedStore(t, fake, api)
	putBytes(t, s, "k", []byte("x"), blob.PutOptions{Tags: map[string]string{"a": "b"}})
	api.taggingErr = errTaggingDenied

	info, err := s.Stat(context.Background(), "k", blob.StatOptions{IncludeTags: true})
	require.ErrorIs(t, err, errTaggingDenied)
	assert.Nil(t, info)
	require.Len(t, api.tagging, 1, "a denied version-specific read must not retry against the current version")
	assert.Equal(t, "v1", aws.ToString(api.tagging[0].VersionId))
}

// newVersionedStore returns a store whose requests go through the wrapping API.
func newVersionedStore(t *testing.T, fake *s3test.Server, api *versionedAPI) *Store {
	t.Helper()
	s, err := NewFromClient(api, fake.Bucket(), WithPartSize(smallPart))
	require.NoError(t, err)
	return s
}
