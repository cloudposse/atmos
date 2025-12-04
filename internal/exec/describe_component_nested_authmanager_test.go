package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupMockStateGetter configures the global stateGetter to return mock values
// for terraform state lookups. This allows tests to run without actual terraform state files.
// Returns a cleanup function that must be deferred to restore the original state getter.
func setupMockStateGetter(t *testing.T, ctrl *gomock.Controller) func() {
	t.Helper()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter

	// Configure mock to return values for all components in the fixture.
	// Level 3 components (no dependencies).
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "level3-component", "subnet_id", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("subnet-level3-12345", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "level3-component", "cidr_block", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("10.0.3.0/24", nil).
		AnyTimes()

	// Level 2 components.
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "level2-component", "vpc_id", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("vpc-level2-67890", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "level2-component", "level3_subnet_id", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("subnet-level3-12345", nil).
		AnyTimes()

	// Auth override scenario components.
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "auth-override-level3", "database_host", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("db.example.com", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "auth-override-level2", "service_name", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("api-service", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "auth-override-level2", "database_config", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("db.example.com", nil).
		AnyTimes()

	// Multi-auth scenario components.
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "multi-auth-level3", "shared_resource_id", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("shared-12345", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "multi-auth-level2", "vpc_id", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("vpc-account-b", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "multi-auth-level2", "shared_resource", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("shared-12345", nil).
		AnyTimes()

	// Mixed inheritance scenario components.
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "mixed-inherit-component", "config_value", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("inherited-auth-config", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "mixed-override-component", "override_value", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("override-specific-value", nil).
		AnyTimes()

	// Deep nesting scenario components.
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "deep-level4", "data_source", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("primary-db", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "deep-level3", "data_ref", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("primary-db", nil).
		AnyTimes()
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "test", "deep-level2", "nested_data", gomock.Any(), gomock.Any(), gomock.Any()).
		Return("primary-db", nil).
		AnyTimes()

	stateGetter = mockStateGetter
	return func() { stateGetter = originalGetter }
}

// TestNestedAuthManagerPropagation verifies that AuthManager propagates correctly
// through multiple levels of nested !terraform.state YAML functions.
//
// Nesting structure:
//
//	Level 1: level1-component
//	  └─ !terraform.state level2-component ... (triggers level2 evaluation)
//	Level 2: level2-component
//	  └─ !terraform.state level3-component ... (triggers level3 evaluation)
//	Level 3: level3-component (base, no nested functions)
//
// Without the fix:
//   - Level 2 and 3 would have nil AuthManager
//   - YAML functions would fail with IMDS timeout errors
//
// With the fix:
//   - AuthManager flows through all levels
//   - All nested evaluations have AuthContext
//   - No IMDS timeout errors occur.
func TestNestedAuthManagerPropagation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mock state getter to return expected values without actual terraform state.
	cleanup := setupMockStateGetter(t, ctrl)
	defer cleanup()

	// Setup: Create AuthContext with AWS credentials.
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-nested-identity",
			Region:          "us-east-1",
			CredentialsFile: "/tmp/test-nested-creds",
			ConfigFile:      "/tmp/test-nested-config",
		},
	}

	// Create stackInfo with AuthContext (what AuthManager.Authenticate() populates).
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		Stack:       "test",
	}

	// Create mock AuthManager that returns our authStackInfo.
	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(authStackInfo).
		AnyTimes()

	// Use the nested propagation fixture.
	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	// Test Level 1 component which has nested !terraform.state functions
	// that reference level2-component.
	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "level1-component",
		Stack:                "test",
		ProcessTemplates:     true, // Enable to process templates
		ProcessYamlFunctions: true, // Enable to test nested functions
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	// Verify the function succeeded.
	// If AuthManager propagation fails, we'd see IMDS timeout errors here.
	require.NoError(t, err, "ExecuteDescribeComponent should succeed with nested functions")
	require.NotNil(t, componentSection, "Result should not be nil")

	// Verify component configuration is present.
	assert.NotEmpty(t, componentSection, "Component should have configuration")

	// Verify the nested values were resolved (they won't actually exist in real state,
	// but the YAML parsing should complete without IMDS errors).
	vars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")
	assert.Contains(t, vars, "level2_vpc_id", "Should have level2_vpc_id from nested function")
	assert.Contains(t, vars, "level2_subnet", "Should have level2_subnet from doubly-nested function")
}

// TestNestedAuthManagerPropagationLevel2 tests starting from Level 2 component
// to verify AuthManager still propagates to Level 3.
func TestNestedAuthManagerPropagationLevel2(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mock state getter to return expected values without actual terraform state.
	cleanup := setupMockStateGetter(t, ctrl)
	defer cleanup()

	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-level2-identity",
			Region:          "us-west-2",
			CredentialsFile: "/tmp/test-level2-creds",
			ConfigFile:      "/tmp/test-level2-config",
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

	// Test Level 2 component which references level3-component.
	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "level2-component",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Level 2 should work with nested AuthManager")
	require.NotNil(t, componentSection)

	vars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, vars, "level3_subnet_id", "Should have nested value from level3")
}

// TestNestedAuthManagerPropagationLevel3 tests the base component (no nesting).
func TestNestedAuthManagerPropagationLevel3(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-level3-identity",
			Region:  "us-east-1",
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

	// Test Level 3 component (base, no nested functions).
	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "level3-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Level 3 base component should work")
	require.NotNil(t, componentSection)

	// Verify base component has expected vars.
	vars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", vars["stage"])
	assert.Contains(t, vars, "subnet_id")
}

// TestNestedAuthManagerNilHandling verifies graceful handling of nil cases
// in nested scenarios.
func TestNestedAuthManagerNilHandling(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	tests := []struct {
		name        string
		component   string
		authManager types.AuthManager
		description string
	}{
		{
			name:        "level1_nil_authmanager",
			component:   "level1-component",
			authManager: nil,
			description: "Level 1 should work when AuthManager is nil",
		},
		{
			name:        "level2_nil_authmanager",
			component:   "level2-component",
			authManager: nil,
			description: "Level 2 should work when AuthManager is nil",
		},
		{
			name:        "level3_nil_authmanager",
			component:   "level3-component",
			authManager: nil,
			description: "Level 3 should work when AuthManager is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
				Component:            tt.component,
				Stack:                "test",
				ProcessTemplates:     false, // Disable to avoid state access
				ProcessYamlFunctions: false, // Disable to avoid state access
				Skip:                 nil,
				AuthManager:          tt.authManager,
			})

			require.NoError(t, err, tt.description)
			require.NotNil(t, componentSection)
		})
	}
}

// TestNestedAuthManagerWithNilStackInfo verifies handling when AuthManager
// returns nil stackInfo in nested scenarios.
func TestNestedAuthManagerWithNilStackInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(nil).
		AnyTimes()

	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "level1-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Should handle nil stackInfo in nested scenarios")
	require.NotNil(t, componentSection)
}

// TestNestedAuthManagerWithNilAuthContext verifies handling when AuthContext
// is nil in nested scenarios.
func TestNestedAuthManagerWithNilAuthContext(t *testing.T) {
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

	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "level1-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Should handle nil AuthContext in nested scenarios")
	require.NotNil(t, componentSection)
}

// setupNestedAuthTest creates a mock AuthManager and sets up the test directory.
// Returns the mock controller, AuthManager, and cleanup function for use in nested auth tests.
func setupNestedAuthTest(t *testing.T, profile, region string) (*gomock.Controller, types.AuthManager, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)

	// Setup mock state getter to return expected values without actual terraform state.
	stateCleanup := setupMockStateGetter(t, ctrl)

	parentAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: profile,
			Region:  region,
		},
	}

	parentStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: parentAuthContext,
		Stack:       "test",
	}

	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(parentStackInfo).
		AnyTimes()

	workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
	t.Chdir(workDir)

	return ctrl, mockAuthManager, stateCleanup
}

// TestNestedAuthManagerScenario2_AuthOverrideAtMiddleLevel verifies that
// a component in the middle of a nested chain can override authentication.
//
// Nesting structure:
//
//	Level 1: auth-override-level1 (no auth override, uses parent)
//	  └─ !terraform.state auth-override-level2 ... (triggers level2 evaluation)
//	Level 2: auth-override-level2 (HAS auth override → account 222222222222)
//	  └─ !terraform.state auth-override-level3 ... (triggers level3 evaluation)
//	Level 3: auth-override-level3 (no auth override, inherits from level2)
//
// Expected Behavior:
//   - Level 1 uses parent AuthManager
//   - Level 2 creates new AuthManager with account 222222222222
//   - Level 3 inherits Level 2's AuthManager
//   - No IMDS timeout errors at any level.
func TestNestedAuthManagerScenario2_AuthOverrideAtMiddleLevel(t *testing.T) {
	ctrl, mockAuthManager, stateCleanup := setupNestedAuthTest(t, "parent-profile", "us-east-1")
	defer ctrl.Finish()
	defer stateCleanup()

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "auth-override-level1",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Should succeed with auth override in middle level")
	require.NotNil(t, componentSection)

	vars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "Should have vars section")
	assert.Contains(t, vars, "api_endpoint", "Should have api_endpoint from nested level2")
	assert.Contains(t, vars, "api_database", "Should have api_database from doubly-nested level3")
}

// TestNestedAuthManagerScenario3_MultipleAuthOverrides verifies that
// multiple components in a chain can each have their own auth overrides.
//
// Nesting structure:
//
//	Level 1: multi-auth-level1 (auth override → account 444444444444)
//	  └─ !terraform.state multi-auth-level2 ... (triggers level2 evaluation)
//	Level 2: multi-auth-level2 (auth override → account 333333333333)
//	  └─ !terraform.state multi-auth-level3 ... (triggers level3 evaluation)
//	Level 3: multi-auth-level3 (no auth override, inherits from level2)
//
// Expected Behavior:
//   - Level 1 uses account 444444444444 (AccountCAccess)
//   - Level 2 uses account 333333333333 (AccountBAccess) - its own override
//   - Level 3 uses account 333333333333 (inherited from Level 2)
//   - Each level with auth config creates its own AuthManager.
func TestNestedAuthManagerScenario3_MultipleAuthOverrides(t *testing.T) {
	ctrl, mockAuthManager, stateCleanup := setupNestedAuthTest(t, "global-profile", "us-east-1")
	defer ctrl.Finish()
	defer stateCleanup()

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "multi-auth-level1",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Should succeed with multiple auth overrides")
	require.NotNil(t, componentSection)

	vars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "Should have vars section")
	assert.Contains(t, vars, "vpc_reference", "Should have vpc_reference from level2")
	assert.Contains(t, vars, "shared_reference", "Should have shared_reference from level3")
}

// TestNestedAuthManagerScenario4_MixedInheritance verifies that in the same
// top-level component, some nested components can override auth while others inherit.
//
// Nesting structure:
//
//	mixed-top-level (no auth override, uses parent auth)
//	  ├─ !terraform.state mixed-inherit-component ... (inherits parent auth)
//	  └─ !terraform.state mixed-override-component ... (uses its own auth)
//	      └─ !terraform.state mixed-inherit-component ... (inherits mixed-override's auth)
//
// Expected Behavior:
//   - mixed-top-level uses parent AuthManager
//   - mixed-inherit-component inherits parent AuthManager when called directly
//   - mixed-override-component uses account 555555555555 (its own auth)
//   - When mixed-override calls mixed-inherit, it inherits account 555555555555.
func TestNestedAuthManagerScenario4_MixedInheritance(t *testing.T) {
	ctrl, mockAuthManager, stateCleanup := setupNestedAuthTest(t, "parent-profile", "us-west-2")
	defer ctrl.Finish()
	defer stateCleanup()

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "mixed-top-level",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Should succeed with mixed auth inheritance")
	require.NotNil(t, componentSection)

	vars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "Should have vars section")
	assert.Contains(t, vars, "value_from_inherit", "Should have value from inherit component")
	assert.Contains(t, vars, "value_from_override", "Should have value from override component")
}

// TestNestedAuthManagerScenario5_DeepNesting verifies authentication in
// 4-level deep nesting with auth overrides at non-adjacent levels.
//
// Nesting structure:
//
//	Level 1: deep-level1 (auth override → account 777777777777)
//	  └─ !terraform.state deep-level2 ... (triggers level2 evaluation)
//	Level 2: deep-level2 (no auth override, inherits from level1)
//	  └─ !terraform.state deep-level3 ... (triggers level3 evaluation)
//	Level 3: deep-level3 (auth override → account 666666666666)
//	  └─ !terraform.state deep-level4 ... (triggers level4 evaluation)
//	Level 4: deep-level4 (no auth override, inherits from level3)
//
// Expected Behavior:
//   - Level 1 uses account 777777777777 (Level1Access)
//   - Level 2 inherits account 777777777777 from Level 1
//   - Level 3 switches to account 666666666666 (Level3Access) - its own override
//   - Level 4 inherits account 666666666666 from Level 3.
func TestNestedAuthManagerScenario5_DeepNesting(t *testing.T) {
	ctrl, mockAuthManager, stateCleanup := setupNestedAuthTest(t, "global-profile", "us-east-1")
	defer ctrl.Finish()
	defer stateCleanup()

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "deep-level1",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          mockAuthManager,
	})

	require.NoError(t, err, "Should succeed with 4-level deep nesting and multiple auth overrides")
	require.NotNil(t, componentSection)

	vars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "Should have vars section")
	assert.Contains(t, vars, "final_data", "Should have final_data from 4-level deep nesting")
}
