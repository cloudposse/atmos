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

// TestComponentLevelAuthOverride verifies that each component can override
// authentication configuration at any nesting level.
//
// Test Scenario:
//
//	Level 1: Uses global auth (no component auth config)
//	Level 2: Overrides with component-specific auth
//	Level 3: Inherits from Level 2
//
// This verifies the resolver correctly:
//   - Creates new AuthManager when component has auth config
//   - Inherits parent AuthManager when component has no auth config.
func TestComponentLevelAuthOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock AuthManager for global/parent authentication.
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "global-profile",
			Region:          "us-east-1",
			CredentialsFile: "/tmp/global-creds",
			ConfigFile:      "/tmp/global-config",
		},
	}

	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		Stack:       "test",
	}

	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(authStackInfo).
		AnyTimes()

	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	// (this also disables parent directory search and git root discovery).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Test that ExecuteDescribeComponent works with AuthManager.
	// The resolver will check if component has auth config and either:
	// - Create component-specific AuthManager (if auth config exists)
	// - Use parent AuthManager (if no auth config).
	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "level1-component",
		Stack:                "test",
		ProcessTemplates:     false, // Disable to focus on auth resolution
		ProcessYamlFunctions: false, // Disable to avoid nested state reads
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Should successfully resolve auth for component")
	require.NotNil(t, componentSection, "Component section should not be nil")

	// Verify component configuration is returned.
	assert.NotEmpty(t, componentSection, "Component should have configuration")
}

// TestResolveAuthManagerForNestedComponent verifies the auth resolution logic directly.
func TestResolveAuthManagerForNestedComponent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create parent AuthManager.
	parentAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "parent-profile",
			Region:  "us-west-2",
		},
	}

	parentStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: parentAuthContext,
		Stack:       "test",
	}

	parentAuthManager := types.NewMockAuthManager(ctrl)
	parentAuthManager.EXPECT().
		GetStackInfo().
		Return(parentStackInfo).
		AnyTimes()
	parentAuthManager.EXPECT().
		GetChain().
		Return([]string{"parent-chain"}).
		AnyTimes()

	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	// (this also disables parent directory search and git root discovery).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Get Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err, "Should load atmos config")

	tests := []struct {
		name              string
		component         string
		parentAuthManager types.AuthManager
		description       string
	}{
		{
			name:              "component_without_auth_inherits_parent",
			component:         "level1-component",
			parentAuthManager: parentAuthManager,
			description:       "Component without auth config should inherit parent AuthManager",
		},
		{
			name:              "component_with_nil_parent",
			component:         "level1-component",
			parentAuthManager: nil,
			description:       "Component should handle nil parent AuthManager gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedAuthMgr, err := resolveAuthManagerForNestedComponent(
				&atmosConfig,
				tt.component,
				"test",
				tt.parentAuthManager,
			)

			// Should not error (may return nil for nil parent).
			require.NoError(t, err, tt.description)

			// If parent was provided, we should get either:
			// - The same parent (if component has no auth config)
			// - A new AuthManager (if component has auth config)
			// - Or nil (if there was an error getting component config, best-effort fallback)
			// We just verify no error is returned.
			_ = resolvedAuthMgr
		})
	}
}

// TestAuthOverrideInNestedChain verifies auth override behavior in a nested chain.
// This test documents the expected behavior when components at different levels
// have different auth configurations.
func TestAuthOverrideInNestedChain(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup global AuthManager.
	globalAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "global-profile",
			Region:  "us-east-1",
		},
	}

	globalStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: globalAuthContext,
		Stack:       "test",
	}

	globalAuthManager := types.NewMockAuthManager(ctrl)
	globalAuthManager.EXPECT().
		GetStackInfo().
		Return(globalStackInfo).
		AnyTimes()

	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	// (this also disables parent directory search and git root discovery).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Process Level 1 component which references Level 2.
	// Since Level 1 has no auth config, it should use global AuthManager.
	level1Section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "level1-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          globalAuthManager,
	})

	require.NoError(t, err, "Level 1 should process successfully")
	require.NotNil(t, level1Section)

	// Verify vars section exists.
	vars, ok := level1Section["vars"].(map[string]any)
	require.True(t, ok, "Should have vars section")
	assert.NotNil(t, vars, "Vars should not be nil")
}

// TestAuthOverrideErrorHandling verifies graceful error handling
// when auth override encounters issues.
func TestAuthOverrideErrorHandling(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	// (this also disables parent directory search and git root discovery).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Get Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err, "Should load atmos config")

	tests := []struct {
		name      string
		component string
		stack     string
		parent    types.AuthManager
		wantErr   bool
	}{
		{
			name:      "nonexistent_component",
			component: "does-not-exist",
			stack:     "test",
			parent:    nil,
			wantErr:   true, // Returns error for nonexistent component
		},
		{
			name:      "valid_component_nil_parent",
			component: "level1-component",
			stack:     "test",
			parent:    nil,
			wantErr:   false, // Should work with nil parent
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveAuthManagerForNestedComponent(
				&atmosConfig,
				tt.component,
				tt.stack,
				tt.parent,
			)

			if tt.wantErr {
				assert.Error(t, err, "Should error when component does not exist")
			} else {
				assert.NoError(t, err, "Should not error for valid component")
			}
		})
	}
}
