package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// mockAWSGetter is a mock implementation of AWSGetter for testing.
type mockAWSGetter struct {
	identity *AWSCallerIdentity
	err      error
}

func (m *mockAWSGetter) GetCallerIdentity(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*AWSCallerIdentity, error) {
	return m.identity, m.err
}

// runAWSYamlFuncTest is a helper that reduces duplication in AWS YAML function tests.
func runAWSYamlFuncTest(
	input string,
	mockIdentity *AWSCallerIdentity,
	mockErr error,
	testFunc func(*schema.AtmosConfiguration, string, *schema.ConfigAndStacksInfo) any,
) any {
	// Clear cache before each test.
	ClearAWSIdentityCache()

	// Set up mock.
	restore := SetAWSGetter(&mockAWSGetter{
		identity: mockIdentity,
		err:      mockErr,
	})
	defer restore()

	atmosConfig := &schema.AtmosConfiguration{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	return testFunc(atmosConfig, input, stackInfo)
}

func TestProcessTagAwsAccountID(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		mockIdentity    *AWSCallerIdentity
		mockErr         error
		expectedResult  string
		shouldReturnNil bool
	}{
		{
			name:  "valid account ID",
			input: u.AtmosYamlFuncAwsAccountID,
			mockIdentity: &AWSCallerIdentity{
				Account: "123456789012",
				Arn:     "arn:aws:iam::123456789012:user/testuser",
				UserID:  "AIDAEXAMPLE",
			},
			mockErr:        nil,
			expectedResult: "123456789012",
		},
		{
			name:  "different account ID",
			input: u.AtmosYamlFuncAwsAccountID,
			mockIdentity: &AWSCallerIdentity{
				Account: "987654321098",
				Arn:     "arn:aws:sts::987654321098:assumed-role/TestRole/session",
				UserID:  "AROAEXAMPLE:session",
			},
			mockErr:        nil,
			expectedResult: "987654321098",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runAWSYamlFuncTest(tt.input, tt.mockIdentity, tt.mockErr, processTagAwsAccountID)

			if tt.shouldReturnNil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestProcessTagAwsCallerIdentityArn(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		mockIdentity   *AWSCallerIdentity
		mockErr        error
		expectedResult string
	}{
		{
			name:  "valid IAM user ARN",
			input: u.AtmosYamlFuncAwsCallerIdentityArn,
			mockIdentity: &AWSCallerIdentity{
				Account: "123456789012",
				Arn:     "arn:aws:iam::123456789012:user/testuser",
				UserID:  "AIDAEXAMPLE",
			},
			mockErr:        nil,
			expectedResult: "arn:aws:iam::123456789012:user/testuser",
		},
		{
			name:  "valid assumed role ARN",
			input: u.AtmosYamlFuncAwsCallerIdentityArn,
			mockIdentity: &AWSCallerIdentity{
				Account: "987654321098",
				Arn:     "arn:aws:sts::987654321098:assumed-role/AdminRole/session-name",
				UserID:  "AROAEXAMPLE:session-name",
			},
			mockErr:        nil,
			expectedResult: "arn:aws:sts::987654321098:assumed-role/AdminRole/session-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runAWSYamlFuncTest(tt.input, tt.mockIdentity, tt.mockErr, processTagAwsCallerIdentityArn)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestProcessTagAwsCallerIdentityUserID(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		mockIdentity   *AWSCallerIdentity
		mockErr        error
		expectedResult string
	}{
		{
			name:  "valid IAM user ID",
			input: u.AtmosYamlFuncAwsCallerIdentityUserID,
			mockIdentity: &AWSCallerIdentity{
				Account: "123456789012",
				Arn:     "arn:aws:iam::123456789012:user/testuser",
				UserID:  "AIDAEXAMPLE123456789",
			},
			mockErr:        nil,
			expectedResult: "AIDAEXAMPLE123456789",
		},
		{
			name:  "valid assumed role user ID",
			input: u.AtmosYamlFuncAwsCallerIdentityUserID,
			mockIdentity: &AWSCallerIdentity{
				Account: "987654321098",
				Arn:     "arn:aws:sts::987654321098:assumed-role/AdminRole/session-name",
				UserID:  "AROAEXAMPLE:session-name",
			},
			mockErr:        nil,
			expectedResult: "AROAEXAMPLE:session-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runAWSYamlFuncTest(tt.input, tt.mockIdentity, tt.mockErr, processTagAwsCallerIdentityUserID)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestProcessTagAwsRegion(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		mockIdentity   *AWSCallerIdentity
		mockErr        error
		expectedResult string
	}{
		{
			name:  "us-east-1 region",
			input: u.AtmosYamlFuncAwsRegion,
			mockIdentity: &AWSCallerIdentity{
				Account: "123456789012",
				Arn:     "arn:aws:iam::123456789012:user/testuser",
				UserID:  "AIDAEXAMPLE",
				Region:  "us-east-1",
			},
			mockErr:        nil,
			expectedResult: "us-east-1",
		},
		{
			name:  "eu-west-1 region",
			input: u.AtmosYamlFuncAwsRegion,
			mockIdentity: &AWSCallerIdentity{
				Account: "987654321098",
				Arn:     "arn:aws:sts::987654321098:assumed-role/AdminRole/session",
				UserID:  "AROAEXAMPLE:session",
				Region:  "eu-west-1",
			},
			mockErr:        nil,
			expectedResult: "eu-west-1",
		},
		{
			name:  "ap-northeast-1 region",
			input: u.AtmosYamlFuncAwsRegion,
			mockIdentity: &AWSCallerIdentity{
				Account: "111111111111",
				Arn:     "arn:aws:iam::111111111111:root",
				UserID:  "111111111111",
				Region:  "ap-northeast-1",
			},
			mockErr:        nil,
			expectedResult: "ap-northeast-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runAWSYamlFuncTest(tt.input, tt.mockIdentity, tt.mockErr, processTagAwsRegion)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestAWSIdentityCache(t *testing.T) {
	// Clear cache before test.
	ClearAWSIdentityCache()

	callCount := 0
	mockGetter := &mockAWSGetter{
		identity: &AWSCallerIdentity{
			Account: "111111111111",
			Arn:     "arn:aws:iam::111111111111:user/cachetest",
			UserID:  "AIDACACHETEST",
		},
		err: nil,
	}

	// Wrap to count calls.
	countingGetter := &countingAWSGetter{
		wrapped:   mockGetter,
		callCount: &callCount,
	}

	restore := SetAWSGetter(countingGetter)
	defer restore()

	atmosConfig := &schema.AtmosConfiguration{}
	ctx := context.Background()

	// First call should hit the mock.
	identity1, err := getAWSCallerIdentityCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "111111111111", identity1.Account)
	assert.Equal(t, 1, callCount, "First call should invoke the getter")

	// Second call with same auth context should use cache.
	identity2, err := getAWSCallerIdentityCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "111111111111", identity2.Account)
	assert.Equal(t, 1, callCount, "Second call should use cache, not invoke getter")

	// Call with different auth context should hit mock again.
	differentAuth := &schema.AWSAuthContext{
		Profile:         "different-profile",
		CredentialsFile: "/different/path",
	}
	identity3, err := getAWSCallerIdentityCached(ctx, atmosConfig, differentAuth)
	require.NoError(t, err)
	assert.Equal(t, "111111111111", identity3.Account)
	assert.Equal(t, 2, callCount, "Different auth context should invoke getter")

	// Clear cache and verify next call hits mock.
	ClearAWSIdentityCache()
	identity4, err := getAWSCallerIdentityCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "111111111111", identity4.Account)
	assert.Equal(t, 3, callCount, "After cache clear, should invoke getter")
}

// countingAWSGetter wraps another getter and counts calls.
type countingAWSGetter struct {
	wrapped   AWSGetter
	callCount *int
}

func (c *countingAWSGetter) GetCallerIdentity(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*AWSCallerIdentity, error) {
	*c.callCount++
	return c.wrapped.GetCallerIdentity(ctx, atmosConfig, authContext)
}

func TestAWSCacheWithErrors(t *testing.T) {
	// Clear cache before test.
	ClearAWSIdentityCache()

	callCount := 0
	expectedErr := errors.New("mock AWS error")
	mockGetter := &mockAWSGetter{
		identity: nil,
		err:      expectedErr,
	}

	countingGetter := &countingAWSGetter{
		wrapped:   mockGetter,
		callCount: &callCount,
	}

	restore := SetAWSGetter(countingGetter)
	defer restore()

	atmosConfig := &schema.AtmosConfiguration{}
	ctx := context.Background()

	// First call should return error and cache it.
	_, err := getAWSCallerIdentityCached(ctx, atmosConfig, nil)
	require.Error(t, err)
	assert.Equal(t, 1, callCount)

	// Second call should return cached error.
	_, err = getAWSCallerIdentityCached(ctx, atmosConfig, nil)
	require.Error(t, err)
	assert.Equal(t, 1, callCount, "Errors should be cached too")
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name        string
		authContext *schema.AWSAuthContext
		expected    string
	}{
		{
			name:        "nil auth context",
			authContext: nil,
			expected:    "default",
		},
		{
			name: "with profile and credentials file",
			authContext: &schema.AWSAuthContext{
				Profile:         "my-profile",
				CredentialsFile: "/home/user/.aws/credentials",
			},
			expected: "my-profile:/home/user/.aws/credentials",
		},
		{
			name: "empty profile",
			authContext: &schema.AWSAuthContext{
				Profile:         "",
				CredentialsFile: "/some/path",
			},
			expected: ":/some/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCacheKey(tt.authContext)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAWSGetterInterface(t *testing.T) {
	// Ensure defaultAWSGetter implements AWSGetter.
	var _ AWSGetter = &defaultAWSGetter{}
}

func TestProcessTagAwsWithAuthContext(t *testing.T) {
	// Clear cache before test.
	ClearAWSIdentityCache()

	// Set up mock with specific identity.
	restore := SetAWSGetter(&mockAWSGetter{
		identity: &AWSCallerIdentity{
			Account: "222222222222",
			Arn:     "arn:aws:sts::222222222222:assumed-role/MyRole/session",
			UserID:  "AROAEXAMPLE:session",
		},
		err: nil,
	})
	defer restore()

	atmosConfig := &schema.AtmosConfiguration{}

	// Test with auth context in stackInfo.
	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				Profile:         "test-profile",
				CredentialsFile: "/test/credentials",
			},
		},
	}

	result := processTagAwsAccountID(atmosConfig, u.AtmosYamlFuncAwsAccountID, stackInfo)
	assert.Equal(t, "222222222222", result)

	// Clear cache for next test.
	ClearAWSIdentityCache()

	result = processTagAwsCallerIdentityArn(atmosConfig, u.AtmosYamlFuncAwsCallerIdentityArn, stackInfo)
	assert.Equal(t, "arn:aws:sts::222222222222:assumed-role/MyRole/session", result)
}

func TestProcessSimpleTagsWithAWSFunctions(t *testing.T) {
	// Clear cache before test.
	ClearAWSIdentityCache()

	// Set up mock.
	restore := SetAWSGetter(&mockAWSGetter{
		identity: &AWSCallerIdentity{
			Account: "333333333333",
			Arn:     "arn:aws:iam::333333333333:user/integration-test",
			UserID:  "AIDAINTEGRATION",
			Region:  "us-west-2",
		},
		err: nil,
	})
	defer restore()

	atmosConfig := &schema.AtmosConfiguration{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	// Test !aws.account_id through processSimpleTags.
	result, handled := processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsAccountID, "", nil, stackInfo)
	assert.True(t, handled)
	assert.Equal(t, "333333333333", result)

	// Clear cache for next test.
	ClearAWSIdentityCache()

	// Test !aws.caller_identity_arn through processSimpleTags.
	result, handled = processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsCallerIdentityArn, "", nil, stackInfo)
	assert.True(t, handled)
	assert.Equal(t, "arn:aws:iam::333333333333:user/integration-test", result)

	// Clear cache for next test.
	ClearAWSIdentityCache()

	// Test !aws.caller_identity_user_id through processSimpleTags.
	result, handled = processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsCallerIdentityUserID, "", nil, stackInfo)
	assert.True(t, handled)
	assert.Equal(t, "AIDAINTEGRATION", result)

	// Clear cache for next test.
	ClearAWSIdentityCache()

	// Test !aws.region through processSimpleTags.
	result, handled = processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsRegion, "", nil, stackInfo)
	assert.True(t, handled)
	assert.Equal(t, "us-west-2", result)
}

func TestProcessSimpleTagsSkipsAWSFunctions(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	// Test that skipping works for aws.account_id.
	skip := []string{"aws.account_id"}
	result, handled := processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsAccountID, "", skip, stackInfo)
	assert.False(t, handled)
	assert.Nil(t, result)

	// Test that skipping works for aws.caller_identity_arn.
	skip = []string{"aws.caller_identity_arn"}
	result, handled = processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsCallerIdentityArn, "", skip, stackInfo)
	assert.False(t, handled)
	assert.Nil(t, result)

	// Test that skipping works for aws.caller_identity_user_id.
	skip = []string{"aws.caller_identity_user_id"}
	result, handled = processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsCallerIdentityUserID, "", skip, stackInfo)
	assert.False(t, handled)
	assert.Nil(t, result)

	// Test that skipping works for aws.region.
	skip = []string{"aws.region"}
	result, handled = processSimpleTags(atmosConfig, u.AtmosYamlFuncAwsRegion, "", skip, stackInfo)
	assert.False(t, handled)
	assert.Nil(t, result)
}

// TestAWSYamlFunctionConstants verifies the constants are defined correctly.
func TestAWSYamlFunctionConstants(t *testing.T) {
	assert.Equal(t, "!aws.account_id", u.AtmosYamlFuncAwsAccountID)
	assert.Equal(t, "!aws.caller_identity_arn", u.AtmosYamlFuncAwsCallerIdentityArn)
	assert.Equal(t, "!aws.caller_identity_user_id", u.AtmosYamlFuncAwsCallerIdentityUserID)
	assert.Equal(t, "!aws.region", u.AtmosYamlFuncAwsRegion)
}

// TestErrorWrapping verifies that AWS errors are properly wrapped.
func TestErrorWrapping(t *testing.T) {
	// Clear cache before test.
	ClearAWSIdentityCache()

	underlyingErr := errors.New("network timeout")
	restore := SetAWSGetter(&mockAWSGetter{
		identity: nil,
		err:      underlyingErr,
	})
	defer restore()

	atmosConfig := &schema.AtmosConfiguration{}
	ctx := context.Background()

	_, err := getAWSCallerIdentityCached(ctx, atmosConfig, nil)
	require.Error(t, err)

	// The error should be wrapped with the underlying error accessible.
	assert.Contains(t, err.Error(), "network timeout")
}

// TestDefaultAWSGetterExists verifies the default getter exists.
func TestDefaultAWSGetterExists(t *testing.T) {
	// The awsGetter variable should be initialized.
	assert.NotNil(t, awsGetter)

	// It should be a *defaultAWSGetter.
	_, ok := awsGetter.(*defaultAWSGetter)
	assert.True(t, ok, "Default awsGetter should be *defaultAWSGetter")
}

// TestSetAWSGetterRestore verifies the restore function works.
func TestSetAWSGetterRestore(t *testing.T) {
	originalGetter := awsGetter

	mockGetter := &mockAWSGetter{
		identity: &AWSCallerIdentity{Account: "444444444444"},
	}

	restore := SetAWSGetter(mockGetter)

	// Verify getter was replaced.
	assert.Equal(t, mockGetter, awsGetter)

	// Restore original.
	restore()

	// Verify original was restored.
	assert.Equal(t, originalGetter, awsGetter)
}

// TestErrAwsGetCallerIdentity verifies the error constant exists.
func TestErrAwsGetCallerIdentity(t *testing.T) {
	assert.NotNil(t, errUtils.ErrAwsGetCallerIdentity)
	assert.Equal(t, "failed to get AWS caller identity", errUtils.ErrAwsGetCallerIdentity.Error())
}
