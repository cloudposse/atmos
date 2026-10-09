package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/smithy-go"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestIsStackNotFoundError(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil"},
		{name: "bracketed name", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack [vpc] does not exist"}, want: true},
		{name: "stack id", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"}, want: true},
		{name: "stack ARN", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id arn:aws:cloudformation:us-east-1:123456789012:stack/vpc/id does not exist or has been deleted"}, want: true},
		{name: "wrapped API error", err: fmt.Errorf("describe stack: %w", &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack [vpc] does not exist"}), want: true},
		{name: "plain message", err: errors.New("Stack [vpc] does not exist")},
		{name: "wrong API code", err: &smithy.GenericAPIError{Code: "AccessDenied", Message: "Stack [vpc] does not exist"}},
		{name: "missing role", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "Role arn:aws:iam::123456789012:role/deploy does not exist or cannot be assumed"}},
		{name: "missing S3 object", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "S3 error: The specified key does not exist"}},
		{name: "missing stack resource", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack [vpc] resource Bucket does not exist"}},
		{name: "misleading wrapper", err: fmt.Errorf("Stack [vpc] does not exist: %w", &smithy.GenericAPIError{Code: "ValidationError", Message: "invalid template"})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isStackNotFoundError(tt.err))
		})
	}
}

func TestMissingDependencyDoesNotBecomeMissingStack(t *testing.T) {
	t.Parallel()

	apiErr := &smithy.GenericAPIError{Code: "ValidationError", Message: "S3 error: The specified key does not exist"}
	wrapped := &smithy.OperationError{ServiceID: "CloudFormation", OperationName: "DescribeStacks", Err: apiErr}

	t.Run("changeset preserves cause", func(t *testing.T) {
		t.Parallel()
		client := NewMockCloudFormationClient(gomock.NewController(t))
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, wrapped)
		// No CreateChangeSet call is permitted after the failed existence check.
		_, err := createChangeSet(context.Background(), client, &stackSpec{StackName: "vpc"})
		require.ErrorIs(t, err, wrapped)
		require.ErrorIs(t, err, apiErr)
	})

	t.Run("API wrap has no missing-stack hint", func(t *testing.T) {
		t.Parallel()
		err := wrapAPICallError("vpc", wrapped)
		require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
		require.ErrorIs(t, err, apiErr)
		assert.Empty(t, cockroachErrors.GetAllHints(err))
		var got smithy.APIError
		require.ErrorAs(t, err, &got)
		assert.Same(t, apiErr, got)
	})

	for _, source := range []string{"events", "stack"} {
		t.Run("delete poll "+source, func(t *testing.T) {
			t.Parallel()
			client := NewMockCloudFormationClient(gomock.NewController(t))
			if source == "events" {
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, wrapped)
			} else {
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
				client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, wrapped)
			}
			_, poll, err := pollStackEvents(context.Background(), client, "vpc", map[string]bool{}, OperationDelete)
			require.ErrorIs(t, err, apiErr)
			assert.False(t, poll.Gone)
		})
	}
}
