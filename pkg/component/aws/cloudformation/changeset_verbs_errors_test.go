package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDescribeNamedChangeSet_ErrorClassification(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		apiErr   error
		notFound bool
	}{
		{
			name:     "typed changeset not found without message text",
			apiErr:   &cfntypes.ChangeSetNotFoundException{Message: awsString("not found")},
			notFound: true,
		},
		{
			name:     "generic modeled code",
			apiErr:   &smithy.GenericAPIError{Code: "ChangeSetNotFound", Message: "unknown change set"},
			notFound: true,
		},
		{
			name:     "containing stack absent",
			apiErr:   &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"},
			notFound: true,
		},
		{
			name:   "missing role",
			apiErr: &smithy.GenericAPIError{Code: "ValidationError", Message: "Role deploy does not exist"},
		},
		{
			name:   "missing template object",
			apiErr: &smithy.GenericAPIError{Code: "ValidationError", Message: "S3 error: The specified key does not exist"},
		},
		{
			name:   "wrong code with changeset text",
			apiErr: &smithy.GenericAPIError{Code: "AccessDenied", Message: "ChangeSet [cs-1] does not exist"},
		},
		{
			name:   "untyped message",
			apiErr: errors.New("ChangeSet [cs-1] does not exist"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := NewMockCloudFormationClient(gomock.NewController(t))
			wrapped := fmt.Errorf("DescribeChangeSet failed: %w", tt.apiErr)
			client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(nil, wrapped)

			result, err := describeNamedChangeSet(context.Background(), client, "vpc", "cs-1")
			require.Nil(t, result)
			require.ErrorIs(t, err, wrapped)
			require.ErrorIs(t, err, tt.apiErr)
			if tt.notFound {
				assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetNotFound)
				assert.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
				assert.Contains(t, err.Error(), "cs-1")
				assert.Contains(t, err.Error(), "vpc")
			} else {
				assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
				assert.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetNotFound)
			}
			var expected smithy.APIError
			if errors.As(tt.apiErr, &expected) {
				var actual smithy.APIError
				require.ErrorAs(t, err, &actual)
				assert.Same(t, expected, actual)
			}
		})
	}
}
