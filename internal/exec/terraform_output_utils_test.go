package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewAuthContextWrapper_NilContext(t *testing.T) {
	result := newAuthContextWrapper(nil)
	assert.Nil(t, result)
}

func TestNewAuthContextWrapper_WithContext(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-profile",
			CredentialsFile: "/path/to/credentials",
			ConfigFile:      "/path/to/config",
			Region:          "us-east-1",
		},
	}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)
	require.NotNil(t, wrapper.stackInfo)
	assert.Equal(t, authContext, wrapper.stackInfo.AuthContext)
}

func TestAuthContextWrapper_GetStackInfo(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-profile",
		},
	}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)

	stackInfo := wrapper.GetStackInfo()
	require.NotNil(t, stackInfo)
	assert.Equal(t, authContext, stackInfo.AuthContext)
}

func TestAuthContextWrapper_GetStackInfo_EmptyAuthContext(t *testing.T) {
	authContext := &schema.AuthContext{}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)

	stackInfo := wrapper.GetStackInfo()
	require.NotNil(t, stackInfo)
	assert.Equal(t, authContext, stackInfo.AuthContext)
}

func TestAuthContextWrapper_AuthenticateProvider(t *testing.T) {
	wrapper := &authContextWrapper{
		stackInfo: &schema.ConfigAndStacksInfo{},
	}

	ctx := context.Background()
	_, err := wrapper.AuthenticateProvider(ctx, "test-provider")

	// Should return an error, not panic.
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
}

func TestEnvironToMapFiltered(t *testing.T) {
	// Test environToMap (which calls environToMapFiltered internally).
	// We can't easily control the environment, but we can verify the function works.
	result := environToMap()
	assert.NotNil(t, result)
	// The result should not contain prohibited env vars.
	for _, prohibited := range prohibitedEnvVars {
		_, exists := result[prohibited]
		assert.False(t, exists, "Should not contain prohibited env var: %s", prohibited)
	}
}
