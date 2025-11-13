package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
