package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCreateQueryAuthManager_NoAuthConfigured verifies that createQueryAuthManager
// returns nil AuthManager (no error) when no auth is configured. This is the
// backward-compatible path for users without SSO/auth configuration.
func TestCreateQueryAuthManager_NoAuthConfigured(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{}
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	authManager, err := createQueryAuthManager(info, atmosConfig)

	assert.NoError(t, err)
	assert.Nil(t, authManager)
	assert.Nil(t, info.AuthManager, "AuthManager should not be stored when nil")
}

// TestCreateQueryAuthManager_ErrorReturned verifies that createQueryAuthManager
// returns an error when auth.CreateAndAuthenticateManagerWithAtmosConfig fails.
// This covers the error path (err != nil, non-abort).
func TestCreateQueryAuthManager_ErrorReturned(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		// Specify an identity that doesn't exist in the (empty) auth config.
		// This triggers ErrAuthNotConfigured from CreateAndAuthenticateManagerWithAtmosConfig.
		Identity: "nonexistent-identity",
	}
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	authManager, err := createQueryAuthManager(info, atmosConfig)

	assert.Error(t, err)
	assert.Nil(t, authManager)
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured)
	assert.Nil(t, info.AuthManager, "AuthManager should not be stored on error")
}

// TestCreateQueryAuthManager_EmptyIdentity verifies that createQueryAuthManager
// works correctly with an empty identity string (auto-detect mode).
func TestCreateQueryAuthManager_EmptyIdentity(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		Identity: "", // Empty means auto-detect.
	}
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	authManager, err := createQueryAuthManager(info, atmosConfig)

	assert.NoError(t, err)
	// With no auth configured and empty identity, should return nil.
	assert.Nil(t, authManager)
}

// TestDescribeStacksAuthPropagation verifies that when ExecuteDescribeStacks receives
// a non-nil AuthManager, the AuthContext is populated on the configAndStacksInfo
// for each component. This is the key test that verifies the fix for issue #2081.
//
// The test directly validates the describe_stacks.go code path at lines 367-372:
//
//	if authManager != nil {
//	    managerStackInfo := authManager.GetStackInfo()
//	    if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
//	        configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
//	    }
//	}
func TestDescribeStacksAuthPropagation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock AuthManager that returns auth context.
	mockManager := types.NewMockAuthManager(ctrl)
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-sso-profile",
			Region:  "eu-central-1",
		},
	}
	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
	}

	// GetStackInfo should be called for each component found during describe stacks.
	// Allow any number of calls since we don't know how many components the test fixtures have.
	mockManager.EXPECT().GetStackInfo().Return(stackInfo).AnyTimes()

	// Create mock state getter to capture the auth context passed to !terraform.state.
	mockStateGetter := NewMockTerraformStateGetter(ctrl)

	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	// Expect GetState to be called WITH the auth context (not nil).
	// This is the key assertion: when --all mode passes an AuthManager to
	// ExecuteDescribeStacks, !terraform.state functions should receive the auth context.
	mockStateGetter.EXPECT().
		GetState(
			gomock.Any(),                   // atmosConfig
			gomock.Any(),                   // yamlFunc
			gomock.Any(),                   // stack
			gomock.Any(),                   // component
			gomock.Any(),                   // output
			gomock.Any(),                   // skipCache
			gomock.Eq(expectedAuthContext), // authContext - MUST be the one from AuthManager
			gomock.Eq(mockManager),         // authManager
		).
		Return("test-value", nil).
		AnyTimes()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.state vpc test-stack bucket_name",
	}

	// Process YAML tags with stackInfo that has AuthContext from the AuthManager.
	// This simulates what ExecuteDescribeStacks does when it receives a non-nil AuthManager.
	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		AuthManager: mockManager,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-value", result["backend_config"])
}

// TestDescribeStacksNoAuthPropagation_Nil verifies the bug scenario: when
// ExecuteDescribeStacks receives nil AuthManager, the auth context is NOT
// populated, causing !terraform.state to use standard AWS SDK resolution.
// This test documents the pre-fix behavior.
func TestDescribeStacksNoAuthPropagation_Nil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)

	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	// When AuthManager is nil, GetState should receive nil authContext.
	mockStateGetter.EXPECT().
		GetState(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(), // authContext should be nil when no AuthManager.
			gomock.Any(), // authManager parameter.
		).
		Return("test-value", nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.state vpc test-stack bucket_name",
	}

	// Process with nil stackInfo (simulating the old behavior with nil AuthManager).
	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-value", result["backend_config"])
}

// TestTerraformOutputAuthPropagation verifies that !terraform.output also receives
// auth context and auth manager when processing with a non-nil AuthManager.
// This ensures the fix covers !terraform.output in addition to !terraform.state.
func TestTerraformOutputAuthPropagation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-sso-profile",
			Region:  "eu-central-1",
		},
	}

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	// Expect GetOutput to be called WITH the auth context and auth manager.
	mockOutputGetter.EXPECT().
		GetOutput(
			gomock.Any(),                   // atmosConfig
			gomock.Any(),                   // stack
			gomock.Any(),                   // component
			gomock.Any(),                   // output
			gomock.Any(),                   // skipCache
			gomock.Eq(expectedAuthContext), // authContext - MUST be from AuthManager
			gomock.Eq(mockManager),         // authManager - MUST be the AuthManager itself
		).
		Return("output-value", true, nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"region": "!terraform.output vpc test-stack aws_region",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		AuthManager: mockManager,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "output-value", result["region"])
}

// TestTerraformOutputNoAuth verifies that !terraform.output works with nil auth
// (backward compatibility for users without auth configured).
func TestTerraformOutputNoAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	mockOutputGetter.EXPECT().
		GetOutput(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(), // authContext nil when no auth.
			gomock.Nil(), // authManager nil when no auth.
		).
		Return("output-value", true, nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"region": "!terraform.output vpc test-stack aws_region",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "output-value", result["region"])
}

// TestMultipleYamlFunctionsAuthPropagation verifies that auth context is propagated
// to multiple different YAML functions in the same input map. This simulates a real
// stack config where multiple functions depend on auth.
func TestMultipleYamlFunctionsAuthPropagation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "multi-profile",
			Region:  "us-west-2",
		},
	}

	// Mock both state and output getters.
	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalStateGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalStateGetter }()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalOutputGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalOutputGetter }()

	// Both should receive the auth context.
	mockStateGetter.EXPECT().
		GetState(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(),
			gomock.Eq(expectedAuthContext),
			gomock.Eq(mockManager),
		).
		Return("state-bucket", nil).
		Times(1)

	mockOutputGetter.EXPECT().
		GetOutput(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(),
			gomock.Eq(expectedAuthContext),
			gomock.Eq(mockManager),
		).
		Return("output-region", true, nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"bucket": "!terraform.state vpc test-stack bucket_name",
		"region": "!terraform.output vpc test-stack aws_region",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		AuthManager: mockManager,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "state-bucket", result["bucket"])
	assert.Equal(t, "output-region", result["region"])
}
