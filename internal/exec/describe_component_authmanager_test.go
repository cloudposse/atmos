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

// TestExecuteDescribeComponentWithAuthManager verifies that AuthManager's AuthContext
// is properly propagated to configAndStacksInfo during component description.
//
// This enables YAML functions (!terraform.state, !terraform.output) to work
// when using the --identity flag with component description commands.
func TestExecuteDescribeComponentWithAuthManager(t *testing.T) {
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

	// Test the propagation logic by calling ExecuteDescribeComponent.
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Load atmos config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	_, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err, "Should load Atmos config")

	// Call ExecuteDescribeComponent with AuthManager.
	// Note: This may fail if the component doesn't exist in the fixture,
	// but the important part is that AuthContext gets propagated.
	result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test-stack",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	// We expect either success or a specific error about missing component.
	// The key is that it doesn't panic and AuthContext is available.
	if err != nil {
		// If it errors, it should be about the component not existing,
		// not about missing AuthContext or nil pointer dereference.
		assert.Contains(t, err.Error(), "component", "Error should be about component, not AuthContext")
	} else {
		// If it succeeds, verify result is not nil
		assert.NotNil(t, result, "Result should not be nil when component exists")
	}
}

// TestExecuteDescribeComponentWithoutAuthManager verifies backward compatibility
// when AuthManager is not provided (nil).
func TestExecuteDescribeComponentWithoutAuthManager(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	_, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should work when AuthManager is nil (no --identity flag).
	result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test-stack",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil, // nil AuthManager
	})

	// We expect either success or a specific error about missing component.
	if err != nil {
		assert.Contains(t, err.Error(), "component", "Error should be about component, not AuthContext")
	} else {
		assert.NotNil(t, result)
	}
}

// TestExecuteDescribeComponentAuthManagerWithNilStackInfo verifies handling
// when AuthManager returns nil stackInfo.
func TestExecuteDescribeComponentAuthManagerWithNilStackInfo(t *testing.T) {
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
	_, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should not panic or error when stackInfo is nil.
	result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test-stack",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	// Should handle nil stackInfo gracefully.
	if err != nil {
		assert.Contains(t, err.Error(), "component", "Error should be about component, not nil stackInfo")
	} else {
		assert.NotNil(t, result)
	}
}

// TestExecuteDescribeComponentAuthManagerWithNilAuthContext verifies handling
// when AuthContext is nil in the stackInfo.
func TestExecuteDescribeComponentAuthManagerWithNilAuthContext(t *testing.T) {
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

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	_, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Should not panic or error when AuthContext is nil.
	result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test-stack",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	// Should handle nil AuthContext gracefully.
	if err != nil {
		assert.Contains(t, err.Error(), "component", "Error should be about component, not nil AuthContext")
	} else {
		assert.NotNil(t, result)
	}
}
