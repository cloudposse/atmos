package cloudformation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubOutputsSeams overrides loadAWSConfig and newCloudFormationClient for a
// single test, restoring the originals on cleanup.
func stubOutputsSeams(t *testing.T, cfgErr error, client cloudFormationAPI) {
	t.Helper()

	origLoadConfig := loadAWSConfig
	origNewClient := newCloudFormationClient

	loadAWSConfig = func(_ context.Context, _, _ string, _ time.Duration, _ *schema.AWSAuthContext) (aws.Config, error) {
		if cfgErr != nil {
			return aws.Config{}, cfgErr
		}
		return aws.Config{}, nil
	}
	newCloudFormationClient = func(_ aws.Config, _ string) cloudFormationAPI { return client }

	t.Cleanup(func() {
		loadAWSConfig = origLoadConfig
		newCloudFormationClient = origNewClient
	})
}

// GetOutputs must extract every Output's key/value pair from a successful
// DescribeStacks response.
func TestGetOutputs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockcloudFormationAPI(ctrl)

	mockClient.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			Outputs: []cfntypes.Output{
				{OutputKey: aws.String("VpcId"), OutputValue: aws.String("vpc-123")},
				{OutputKey: aws.String("SubnetId"), OutputValue: aws.String("subnet-456")},
			},
		}},
	}, nil)

	stubOutputsSeams(t, nil, mockClient)

	outputs, err := GetOutputs(context.Background(), "us-east-1", "vpc", nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"VpcId": "vpc-123", "SubnetId": "subnet-456"}, outputs)
}

// GetOutputs must skip an Output with a nil OutputKey (unnamed/malformed
// entry) rather than panicking, and must represent a nil OutputValue as a nil
// map value rather than dropping the key.
func TestGetOutputs_NilKeyAndValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockcloudFormationAPI(ctrl)

	mockClient.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			Outputs: []cfntypes.Output{
				{OutputKey: nil, OutputValue: aws.String("ignored")},
				{OutputKey: aws.String("EmptyValue"), OutputValue: nil},
			},
		}},
	}, nil)

	stubOutputsSeams(t, nil, mockClient)

	outputs, err := GetOutputs(context.Background(), "us-east-1", "vpc", nil)
	require.NoError(t, err)
	assert.Len(t, outputs, 1, "the nil-OutputKey entry must be skipped")
	value, ok := outputs["EmptyValue"]
	require.True(t, ok, "a nil OutputValue must still produce the key")
	assert.Nil(t, value)
}

// GetOutputs must error when the stack has no matching Stacks entry
// (CloudFormation's shape for "stack not found" via a successful, empty
// response rather than an API error).
func TestGetOutputs_StackNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockcloudFormationAPI(ctrl)

	mockClient.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{},
	}, nil)

	stubOutputsSeams(t, nil, mockClient)

	_, err := GetOutputs(context.Background(), "us-east-1", "missing-stack", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackNotFound)
	assert.Contains(t, err.Error(), "missing-stack")
}

// GetOutputs must wrap a DescribeStacks API error.
func TestGetOutputs_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockcloudFormationAPI(ctrl)

	sentinel := errors.New("access denied")
	mockClient.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, sentinel)

	stubOutputsSeams(t, nil, mockClient)

	_, err := GetOutputs(context.Background(), "us-east-1", "vpc", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	assert.ErrorIs(t, err, sentinel)
}

// GetOutputs must propagate a config-loading failure without ever
// constructing a client or calling DescribeStacks.
func TestGetOutputs_LoadConfigError(t *testing.T) {
	sentinel := errors.New("no credentials")
	ctrl := gomock.NewController(t)
	mockClient := NewMockcloudFormationAPI(ctrl) // no expectations: must never be called.

	stubOutputsSeams(t, sentinel, mockClient)

	_, err := GetOutputs(context.Background(), "us-east-1", "vpc", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// GetOutputs must forward the AuthContext through to loadAWSConfig unchanged.
func TestGetOutputs_PassesAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockcloudFormationAPI(ctrl)
	mockClient.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{}},
	}, nil)

	authContext := &schema.AWSAuthContext{Profile: "dev", EndpointURL: "http://localhost:4566"}

	origLoadConfig := loadAWSConfig
	origNewClient := newCloudFormationClient
	var capturedAuth *schema.AWSAuthContext
	var capturedEndpoint string
	loadAWSConfig = func(_ context.Context, _, _ string, _ time.Duration, authCtx *schema.AWSAuthContext) (aws.Config, error) {
		capturedAuth = authCtx
		return aws.Config{}, nil
	}
	newCloudFormationClient = func(_ aws.Config, endpointURL string) cloudFormationAPI {
		capturedEndpoint = endpointURL
		return mockClient
	}
	t.Cleanup(func() {
		loadAWSConfig = origLoadConfig
		newCloudFormationClient = origNewClient
	})

	_, err := GetOutputs(context.Background(), "us-east-1", "vpc", authContext)
	require.NoError(t, err)
	assert.Same(t, authContext, capturedAuth)
	assert.Equal(t, "http://localhost:4566", capturedEndpoint, "the auth context's emulator endpoint override must reach client construction")
}

// defaultNewCloudFormationClient must target the custom endpoint when one is
// configured. Mirrors pkg/component/aws/cloudformation/client_test.go's
// TestNewClient_SetsCustomEndpoint: we assert the real SDK client's resolved
// Options().BaseEndpoint rather than just checking it doesn't panic.
func TestDefaultNewCloudFormationClient_SetsCustomEndpoint(t *testing.T) {
	client := defaultNewCloudFormationClient(aws.Config{Region: "us-east-1"}, "http://localhost:4566")

	real, ok := client.(*cloudformation.Client)
	require.True(t, ok, "defaultNewCloudFormationClient must return the real SDK client")

	opts := real.Options()
	require.NotNil(t, opts.BaseEndpoint, "BaseEndpoint must be set when an endpoint override is configured")
	assert.Equal(t, "http://localhost:4566", *opts.BaseEndpoint)
}

// Without an endpoint override, BaseEndpoint must be left nil so the client
// targets real AWS via the SDK's normal region-based endpoint resolution.
func TestDefaultNewCloudFormationClient_NoEndpointOverride(t *testing.T) {
	client := defaultNewCloudFormationClient(aws.Config{Region: "us-east-1"}, "")

	real, ok := client.(*cloudformation.Client)
	require.True(t, ok)

	opts := real.Options()
	assert.Nil(t, opts.BaseEndpoint, "BaseEndpoint must stay nil (real AWS) when no override is configured")
}
