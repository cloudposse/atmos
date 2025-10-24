package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCircularDependencyDetection(t *testing.T) {
	t.Skip("Integration test requires real Terraform state backends - skipping for now")

	tests := []struct {
		name          string
		component     string
		stack         string
		expectError   bool
		errorContains string
		description   string
	}{
		{
			name:          "direct_circular_dependency",
			component:     "vpc",
			stack:         "core",
			expectError:   true,
			errorContains: "circular dependency",
			description:   "Core vpc depends on staging vpc, staging vpc depends on core vpc",
		},
		{
			name:          "direct_circular_dependency_reverse",
			component:     "vpc",
			stack:         "staging",
			expectError:   true,
			errorContains: "circular dependency",
			description:   "Same cycle but starting from staging",
		},
		{
			name:          "indirect_circular_dependency",
			component:     "component-a",
			stack:         "indirect-a",
			expectError:   true,
			errorContains: "circular dependency",
			description:   "A → B → C → A cycle",
		},
		{
			name:        "valid_dependency_chain",
			component:   "component-root",
			stack:       "valid-chain",
			expectError: false,
			description: "Valid linear dependency: root → middle → leaf",
		},
		{
			name:        "no_dependencies",
			component:   "component-leaf",
			stack:       "valid-chain",
			expectError: false,
			description: "Component with no dependencies should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
				ComponentFromArg: tt.component,
				Stack:            tt.stack,
			}, true)

			require.NoError(t, err, "Failed to initialize Atmos config")

			// Clear any existing resolution context before test.
			ClearResolutionContext()
			defer ClearResolutionContext()

			// Note: GetTerraformState will be called during YAML function processing
			// and should detect the cycle.
			_, err = ExecuteDescribeComponent(tt.component, tt.stack, false, true, nil)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains,
						"Error message should indicate circular dependency")
				}
			} else {
				assert.NoError(t, err, tt.description)
			}

			_ = atmosConfig
		})
	}
}

func TestCircularDependencyErrorMessage(t *testing.T) {
	t.Skip("Integration test requires real Terraform state backends - skipping for now")

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "core",
	}, true)

	require.NoError(t, err, "Failed to initialize Atmos config")

	// Clear any existing resolution context before test.
	ClearResolutionContext()
	defer ClearResolutionContext()

	_, err = ExecuteDescribeComponent("vpc", "core", false, true, nil)
	if err != nil {
		// Verify error message contains helpful information.
		errMsg := err.Error()

		// Should mention circular dependency.
		assert.Contains(t, errMsg, "circular", "Error should mention circular dependency")

		// Should show the components involved.
		assert.Contains(t, errMsg, "vpc", "Error should mention the component")

		// Should show the stacks involved.
		assert.Contains(t, errMsg, "core", "Error should mention core stack")
		assert.Contains(t, errMsg, "staging", "Error should mention staging stack")

		t.Logf("Error message:\n%s", errMsg)
	}
}

func TestSelfReferencingDependency(t *testing.T) {
	// Test case where a component tries to reference itself.
	// This should be detected as an immediate cycle.

	t.Skip("Self-referencing test requires additional fixture - implement after basic detection works")
}

func TestMixedFunctionTypes(t *testing.T) {
	// Test cycles involving both !terraform.state and atmos.Component().
	// Component A uses !terraform.state to reference B.
	// Component B uses atmos.Component() to reference A.

	t.Skip("Mixed function type test requires additional fixtures - implement after basic detection works")
}
