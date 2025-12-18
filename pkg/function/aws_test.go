package function

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsIdentity "github.com/cloudposse/atmos/pkg/aws/identity"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockAWSGetter is a test implementation of identity.Getter.
type mockAWSGetter struct {
	identity *awsIdentity.CallerIdentity
	err      error
	calls    int
}

func (m *mockAWSGetter) GetCallerIdentity(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*awsIdentity.CallerIdentity, error) {
	m.calls++
	return m.identity, m.err
}

func TestNewAwsAccountIDFunction(t *testing.T) {
	fn := NewAwsAccountIDFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagAwsAccountID, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestNewAwsCallerIdentityArnFunction(t *testing.T) {
	fn := NewAwsCallerIdentityArnFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagAwsCallerIdentityArn, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestNewAwsCallerIdentityUserIDFunction(t *testing.T) {
	fn := NewAwsCallerIdentityUserIDFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagAwsCallerIdentityUserID, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestNewAwsRegionFunction(t *testing.T) {
	fn := NewAwsRegionFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagAwsRegion, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestAwsAccountIDFunction_Execute(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	// Set up mock.
	mock := &mockAWSGetter{
		identity: &awsIdentity.CallerIdentity{
			Account: "123456789012",
			Arn:     "arn:aws:iam::123456789012:user/test",
			UserID:  "AIDAEXAMPLE",
			Region:  "us-west-2",
		},
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsAccountIDFunction()
	result, err := fn.Execute(context.Background(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, "123456789012", result)
}

func TestAwsAccountIDFunction_Execute_Error(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	// Set up mock that returns an error.
	expectedErr := errors.New("AWS credentials not configured")
	mock := &mockAWSGetter{
		err: expectedErr,
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsAccountIDFunction()
	_, err := fn.Execute(context.Background(), "", nil)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestAwsCallerIdentityArnFunction_Execute(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	// Set up mock.
	mock := &mockAWSGetter{
		identity: &awsIdentity.CallerIdentity{
			Account: "123456789012",
			Arn:     "arn:aws:iam::123456789012:user/test",
			UserID:  "AIDAEXAMPLE",
			Region:  "us-west-2",
		},
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsCallerIdentityArnFunction()
	result, err := fn.Execute(context.Background(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::123456789012:user/test", result)
}

func TestAwsCallerIdentityArnFunction_Execute_Error(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	expectedErr := errors.New("STS error")
	mock := &mockAWSGetter{err: expectedErr}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsCallerIdentityArnFunction()
	_, err := fn.Execute(context.Background(), "", nil)

	require.Error(t, err)
}

func TestAwsCallerIdentityUserIDFunction_Execute(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	mock := &mockAWSGetter{
		identity: &awsIdentity.CallerIdentity{
			Account: "123456789012",
			Arn:     "arn:aws:iam::123456789012:user/test",
			UserID:  "AIDAEXAMPLE",
			Region:  "us-west-2",
		},
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsCallerIdentityUserIDFunction()
	result, err := fn.Execute(context.Background(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, "AIDAEXAMPLE", result)
}

func TestAwsCallerIdentityUserIDFunction_Execute_Error(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	expectedErr := errors.New("STS error")
	mock := &mockAWSGetter{err: expectedErr}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsCallerIdentityUserIDFunction()
	_, err := fn.Execute(context.Background(), "", nil)

	require.Error(t, err)
}

func TestAwsRegionFunction_Execute(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	mock := &mockAWSGetter{
		identity: &awsIdentity.CallerIdentity{
			Account: "123456789012",
			Arn:     "arn:aws:iam::123456789012:user/test",
			UserID:  "AIDAEXAMPLE",
			Region:  "us-west-2",
		},
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsRegionFunction()
	result, err := fn.Execute(context.Background(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, "us-west-2", result)
}

func TestAwsRegionFunction_Execute_Error(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	expectedErr := errors.New("Region error")
	mock := &mockAWSGetter{err: expectedErr}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsRegionFunction()
	_, err := fn.Execute(context.Background(), "", nil)

	require.Error(t, err)
}

func TestGetAWSIdentity_WithStackInfo(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	mock := &mockAWSGetter{
		identity: &awsIdentity.CallerIdentity{
			Account: "987654321098",
			Arn:     "arn:aws:iam::987654321098:role/admin",
			UserID:  "AROA12345",
			Region:  "eu-west-1",
		},
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	// Create execution context with stack info and auth context.
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		StackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "custom-profile",
					Region:  "eu-west-1",
				},
			},
		},
	}

	fn := NewAwsAccountIDFunction()
	result, err := fn.Execute(context.Background(), "", execCtx)

	require.NoError(t, err)
	assert.Equal(t, "987654321098", result)
}

func TestGetAWSIdentity_NilExecutionContext(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	mock := &mockAWSGetter{
		identity: &awsIdentity.CallerIdentity{
			Account: "123456789012",
			Arn:     "arn:aws:iam::123456789012:user/default",
			UserID:  "DEFAULT",
			Region:  "us-east-1",
		},
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	fn := NewAwsAccountIDFunction()
	result, err := fn.Execute(context.Background(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, "123456789012", result)
}

func TestGetAWSIdentity_PartialStackInfo(t *testing.T) {
	// Clear identity cache before test.
	awsIdentity.ClearIdentityCache()

	mock := &mockAWSGetter{
		identity: &awsIdentity.CallerIdentity{
			Account: "111222333444",
			Arn:     "arn:aws:iam::111222333444:user/test",
			UserID:  "TEST",
			Region:  "ap-southeast-1",
		},
	}

	restore := awsIdentity.SetGetter(mock)
	defer restore()

	// Execution context with StackInfo but nil AuthContext.
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		StackInfo:   &schema.ConfigAndStacksInfo{},
	}

	fn := NewAwsRegionFunction()
	result, err := fn.Execute(context.Background(), "", execCtx)

	require.NoError(t, err)
	assert.Equal(t, "ap-southeast-1", result)
}
