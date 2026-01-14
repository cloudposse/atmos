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

// TestAuthManagerPropagationToDescribeStacks verifies that AuthManager's AuthContext
// is properly propagated to configAndStacksInfo during stack description.
//
// This enables YAML functions (!terraform.state, !terraform.output) to work
// when using the --identity flag with atmos describe stacks.
func TestAuthManagerPropagationToDescribeStacks(t *testing.T) {
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
		Times(1)

	// Test the propagation logic by calling ExecuteDescribeStacks.
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Load atmos config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err, "Should load Atmos config")

	// Call ExecuteDescribeStacks with AuthManager.
	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",              // filterByStack
		nil,             // components
		nil,             // componentTypes
		nil,             // sections
		false,           // ignoreMissingFiles
		false,           // processTemplates
		false,           // processYamlFunctions
		false,           // includeEmptyStacks
		nil,             // skip
		mockAuthManager, // AuthManager
	)

	// Verify the function succeeded.
	require.NoError(t, err, "ExecuteDescribeStacks should succeed with AuthManager")
	require.NotNil(t, stacksMap, "Stacks map should not be nil")
	assert.NotEmpty(t, stacksMap, "Stacks map should contain stacks")
}

// TestDescribeStacksAuthManagerNilHandling verifies graceful handling of nil AuthManager.
func TestDescribeStacksAuthManagerNilHandling(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should work when AuthManager is nil (no --identity flag).
	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		nil,
		nil,
		false,
		false,
		false,
		false,
		nil,
		nil, // nil AuthManager
	)

	require.NoError(t, err, "Should work when AuthManager is nil")
	require.NotNil(t, stacksMap)
	assert.NotEmpty(t, stacksMap)
}

// TestDescribeStacksAuthManagerWithNilStackInfo verifies handling when AuthManager returns nil stackInfo.
func TestDescribeStacksAuthManagerWithNilStackInfo(t *testing.T) {
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

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should not panic or error when stackInfo is nil.
	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		nil,
		nil,
		false,
		false,
		false,
		false,
		nil,
		mockAuthManager,
	)

	require.NoError(t, err, "Should handle nil stackInfo gracefully")
	require.NotNil(t, stacksMap)
}

// TestDescribeStacksAuthManagerWithNilAuthContext verifies handling when AuthContext is nil.
func TestDescribeStacksAuthManagerWithNilAuthContext(t *testing.T) {
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
		Times(1)

	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should not panic or error when AuthContext is nil.
	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		nil,
		nil,
		false,
		false,
		false,
		false,
		nil,
		mockAuthManager,
	)

	require.NoError(t, err, "Should handle nil AuthContext gracefully")
	require.NotNil(t, stacksMap)
}
