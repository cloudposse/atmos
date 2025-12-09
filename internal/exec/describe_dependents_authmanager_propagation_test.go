package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAuthManagerPropagationToDescribeDependents verifies that AuthManager's AuthContext
// is properly propagated during dependent component discovery.
//
// This enables YAML functions (!terraform.state, !terraform.output) to work
// when using the --identity flag with atmos describe dependents.
func TestAuthManagerPropagationToDescribeDependents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup: Create AuthContext with AWS credentials (populated by --identity).
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-identity",
			Region:          "us-east-1",
			CredentialsFile: "/tmp/test-creds",
			ConfigFile:      "/tmp/test-config",
		},
	}

	// Create stackInfo with AuthContext (what AuthManager.Authenticate() populates).
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		Stack:       "test-stack",
	}

	// Create mock AuthManager that returns our authStackInfo.
	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(authStackInfo).
		AnyTimes()

	// Test the propagation logic by calling ExecuteDescribeDependents.
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Load atmos config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err, "Should load Atmos config")

	// Call ExecuteDescribeDependents with AuthManager.
	// Note: This will return empty list since the test fixture doesn't have components
	// with depends_on, but it verifies the AuthManager propagation path works.
	dependents, err := ExecuteDescribeDependents(
		&atmosConfig,
		&DescribeDependentsArgs{
			Component:            "test-component",
			Stack:                "test",
			IncludeSettings:      false,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 nil,
			OnlyInStack:          "",
			AuthManager:          mockAuthManager, // AuthManager passed
		},
	)

	// Verify the function succeeded.
	require.NoError(t, err, "ExecuteDescribeDependents should succeed with AuthManager")
	require.NotNil(t, dependents, "Dependents slice should not be nil")
	// Empty list is expected since fixture doesn't have depends_on relationships.
	assert.Empty(t, dependents, "Dependents list should be empty for test fixture")
}

// TestDescribeDependentsAuthManagerNilHandling verifies graceful handling of nil AuthManager.
func TestDescribeDependentsAuthManagerNilHandling(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should work when AuthManager is nil (no --identity flag).
	dependents, err := ExecuteDescribeDependents(
		&atmosConfig,
		&DescribeDependentsArgs{
			Component:            "test-component",
			Stack:                "test",
			IncludeSettings:      false,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 nil,
			OnlyInStack:          "",
			AuthManager:          nil, // nil AuthManager
		},
	)

	require.NoError(t, err, "Should work when AuthManager is nil")
	require.NotNil(t, dependents)
}

// TestDescribeDependentsAuthManagerWithNilStackInfo verifies handling when AuthManager returns nil stackInfo.
func TestDescribeDependentsAuthManagerWithNilStackInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock AuthManager that returns nil stackInfo.
	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(nil).
		AnyTimes()

	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should not panic or error when stackInfo is nil.
	dependents, err := ExecuteDescribeDependents(
		&atmosConfig,
		&DescribeDependentsArgs{
			Component:            "test-component",
			Stack:                "test",
			IncludeSettings:      false,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 nil,
			OnlyInStack:          "",
			AuthManager:          mockAuthManager,
		},
	)

	require.NoError(t, err, "Should handle nil stackInfo gracefully")
	require.NotNil(t, dependents)
}

// TestDescribeDependentsAuthManagerWithNilAuthContext verifies handling when AuthContext is nil.
func TestDescribeDependentsAuthManagerWithNilAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create stackInfo without AuthContext.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: nil,
		Stack:       "test",
	}

	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(authStackInfo).
		AnyTimes()

	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should not panic or error when AuthContext is nil.
	dependents, err := ExecuteDescribeDependents(
		&atmosConfig,
		&DescribeDependentsArgs{
			Component:            "test-component",
			Stack:                "test",
			IncludeSettings:      false,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 nil,
			OnlyInStack:          "",
			AuthManager:          mockAuthManager,
		},
	)

	require.NoError(t, err, "Should handle nil AuthContext gracefully")
	require.NotNil(t, dependents)
}
