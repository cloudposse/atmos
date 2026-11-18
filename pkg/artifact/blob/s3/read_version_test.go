package s3

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

const versionedBucket = "versioned-bucket"

// expectHead makes the mock report a fixed object with the given HeadObject version ID.
func expectHead(api *MockAPI, versionID *string) {
	api.EXPECT().
		HeadObject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{ContentLength: aws.Int64(1), ETag: aws.String(`"etag"`), VersionId: versionID}, nil
		}).
		Times(1)
}

// newVersionedStore returns a store over the mocked API.
func newVersionedStore(t *testing.T, api API) *Store {
	t.Helper()
	s, err := NewFromClient(api, versionedBucket, WithPartSize(smallPart))
	require.NoError(t, err)
	return s
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
			api := NewMockAPI(gomock.NewController(t))
			expectHead(api, tc.versionID)
			var tagging []*s3.GetObjectTaggingInput
			api.EXPECT().
				GetObjectTagging(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, in *s3.GetObjectTaggingInput, _ ...func(*s3.Options)) (*s3.GetObjectTaggingOutput, error) {
					tagging = append(tagging, in)
					return &s3.GetObjectTaggingOutput{TagSet: []types.Tag{{Key: aws.String("a"), Value: aws.String("b")}}}, nil
				}).
				Times(1)
			s := newVersionedStore(t, api)

			info, err := s.Stat(context.Background(), "k", blob.StatOptions{IncludeTags: true})
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"a": "b"}, info.Tags)
			require.Len(t, tagging, 1)
			assert.Equal(t, aws.ToString(tc.want), aws.ToString(tagging[0].VersionId))
			assert.Equal(t, tc.want == nil, tagging[0].VersionId == nil)
			assert.Equal(t, versionedBucket, aws.ToString(tagging[0].Bucket))
		})
	}
}

func TestStatDoesNotFallBackWhenVersionedTagReadFails(t *testing.T) {
	api := NewMockAPI(gomock.NewController(t))
	expectHead(api, aws.String("v1"))
	// A denied version-specific read must not retry against the current version, so exactly one
	// tagging call is allowed and it must carry the headed version.
	api.EXPECT().
		GetObjectTagging(gomock.Any(), gomock.Cond(func(x any) bool {
			in, ok := x.(*s3.GetObjectTaggingInput)
			return ok && aws.ToString(in.VersionId) == "v1"
		})).
		Return(nil, errUtils.ErrBlobTaggingDenied).
		Times(1)
	s := newVersionedStore(t, api)

	info, err := s.Stat(context.Background(), "k", blob.StatOptions{IncludeTags: true})
	require.ErrorIs(t, err, errUtils.ErrBlobTaggingDenied)
	assert.Nil(t, info)
}
