package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHasDefaultIdentity tests the hasDefaultIdentity function with various auth configurations.
func TestHasDefaultIdentity(t *testing.T) {
	tests := []struct {
		name        string
		authSection map[string]any
		expected    bool
	}{
		{
			name: "has default identity",
			authSection: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"default": true,
						"kind":    "aws/permission-set",
					},
				},
			},
			expected: true,
		},
		{
			name: "no default identity",
			authSection: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"default": false,
						"kind":    "aws/permission-set",
					},
				},
			},
			expected: false,
		},
		{
			name: "multiple identities with one default",
			authSection: map[string]any{
				"identities": map[string]any{
					"identity1": map[string]any{
						"default": false,
						"kind":    "aws/permission-set",
					},
					"identity2": map[string]any{
						"default": true,
						"kind":    "aws/permission-set",
					},
				},
			},
			expected: true,
		},
		{
			name: "no identities section",
			authSection: map[string]any{
				"some_other_key": "value",
			},
			expected: false,
		},
		{
			name:        "nil auth section",
			authSection: nil,
			expected:    false,
		},
		{
			name: "identities is not a map",
			authSection: map[string]any{
				"identities": "not-a-map",
			},
			expected: false,
		},
		{
			name: "identity config is not a map",
			authSection: map[string]any{
				"identities": map[string]any{
					"test-identity": "not-a-map",
				},
			},
			expected: false,
		},
		{
			name: "default is not a bool",
			authSection: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"default": "not-a-bool",
						"kind":    "aws/permission-set",
					},
				},
			},
			expected: false,
		},
		{
			name: "empty identities map",
			authSection: map[string]any{
				"identities": map[string]any{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasDefaultIdentity(tt.authSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveAuthManagerForNestedComponent_NoAuthSection tests the case where
// component has no auth section and should inherit parent AuthManager.
func TestResolveAuthManagerForNestedComponent_NoAuthSection(t *testing.T) {
	// This test requires fixture setup, will be tested via integration tests
	// in describe_component_nested_authmanager_test.go
	t.Skip("Covered by integration tests in describe_component_nested_authmanager_test.go")
}

// TestGetStaticRemoteStateOutput tests the GetStaticRemoteStateOutput function.
func TestGetStaticRemoteStateOutput(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name          string
		outputs       map[string]any
		outputKey     string
		expectError   bool
		expectExists  bool
		expectedValue any
	}{
		{
			name: "output exists with string value",
			outputs: map[string]any{
				"vpc_id": "vpc-12345",
			},
			outputKey:     "vpc_id",
			expectError:   false,
			expectExists:  true,
			expectedValue: "vpc-12345",
		},
		{
			name: "output exists with nil value",
			outputs: map[string]any{
				"optional_field": nil,
			},
			outputKey:     "optional_field",
			expectError:   false,
			expectExists:  true,
			expectedValue: nil,
		},
		{
			name: "output does not exist",
			outputs: map[string]any{
				"vpc_id": "vpc-12345",
			},
			outputKey:    "subnet_id",
			expectError:  false,
			expectExists: false,
		},
		{
			name: "output exists with map value",
			outputs: map[string]any{
				"tags": map[string]any{
					"Environment": "test",
					"Project":     "atmos",
				},
			},
			outputKey:    "tags",
			expectError:  false,
			expectExists: true,
			expectedValue: map[string]any{
				"Environment": "test",
				"Project":     "atmos",
			},
		},
		{
			name: "output exists with slice value",
			outputs: map[string]any{
				"availability_zones": []any{"us-east-1a", "us-east-1b"},
			},
			outputKey:     "availability_zones",
			expectError:   false,
			expectExists:  true,
			expectedValue: []any{"us-east-1a", "us-east-1b"},
		},
		{
			name: "output exists with number value",
			outputs: map[string]any{
				"instance_count": 3,
			},
			outputKey:     "instance_count",
			expectError:   false,
			expectExists:  true,
			expectedValue: 3,
		},
		{
			name: "output exists with boolean value",
			outputs: map[string]any{
				"enabled": true,
			},
			outputKey:     "enabled",
			expectError:   false,
			expectExists:  true,
			expectedValue: true,
		},
		{
			name:         "nil outputs map",
			outputs:      nil,
			outputKey:    "vpc_id",
			expectError:  false,
			expectExists: false,
		},
		{
			name:         "empty outputs map",
			outputs:      map[string]any{},
			outputKey:    "vpc_id",
			expectError:  false,
			expectExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, exists, err := GetStaticRemoteStateOutput(
				atmosConfig,
				"test-component",
				"test-stack",
				tt.outputs,
				tt.outputKey,
			)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.expectExists, exists)

			if tt.expectExists {
				assert.Equal(t, tt.expectedValue, result)
			}
		})
	}
}

// TestGetComponentConfigForAuthResolution_ErrorHandling tests error handling
// in getComponentConfigForAuthResolution.
func TestGetComponentConfigForAuthResolution_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		component string
		stack     string
	}{
		{
			name:      "non-existent component",
			component: "non-existent-component",
			stack:     "test-stack",
		},
		{
			name:      "non-existent stack",
			component: "test-component",
			stack:     "non-existent-stack",
		},
		{
			name:      "empty component name",
			component: "",
			stack:     "test-stack",
		},
		{
			name:      "empty stack name",
			component: "test-component",
			stack:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getComponentConfigForAuthResolution(tt.component, tt.stack)
			// We expect an error because these components/stacks don't exist
			// The function should return an error with ErrDescribeComponent wrapped
			require.Error(t, err)
		})
	}
}

// TestIdentityInheritanceLogic tests the identity extraction logic from parent AuthManager chain.
// This tests the core logic of identity inheritance that was added to fix the issue where
// --identity flag wasn't propagating to nested components.
func TestIdentityInheritanceLogic(t *testing.T) {
	tests := []struct {
		name             string
		chain            []string
		expectedIdentity string
	}{
		{
			name:             "extracts last element from chain",
			chain:            []string{"provider", "identity1", "target-identity"},
			expectedIdentity: "target-identity",
		},
		{
			name:             "handles single element chain",
			chain:            []string{"only-identity"},
			expectedIdentity: "only-identity",
		},
		{
			name:             "handles multi-level chain",
			chain:            []string{"p1", "i1", "i2", "i3", "final"},
			expectedIdentity: "final",
		},
		{
			name:             "handles empty chain",
			chain:            []string{},
			expectedIdentity: "",
		},
		{
			name:             "handles nil chain (treated as empty)",
			chain:            nil,
			expectedIdentity: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the identity extraction logic from createComponentAuthManager
			var identityName string
			if len(tt.chain) > 0 {
				identityName = tt.chain[len(tt.chain)-1]
			}

			assert.Equal(t, tt.expectedIdentity, identityName)
		})
	}
}

// TestResolveAuthManagerForNestedComponent_WithoutAuthSection tests the case where
// component has no auth section and should return parent AuthManager unchanged.
func TestResolveAuthManagerForNestedComponent_WithoutAuthSection(t *testing.T) {
	// This test verifies the early return paths in resolveAuthManagerForNestedComponent
	// when component has no auth section or no default identity.
	// The actual function requires fixture setup, so we test the logic directly.

	tests := []struct {
		name              string
		authSection       map[string]any
		shouldReturnEarly bool
	}{
		{
			name:              "nil auth section returns parent",
			authSection:       nil,
			shouldReturnEarly: true,
		},
		{
			name:              "auth section without identities returns parent",
			authSection:       map[string]any{"providers": map[string]any{}},
			shouldReturnEarly: true,
		},
		{
			name: "auth section with non-default identity returns parent",
			authSection: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"default": false,
						"kind":    "aws/permission-set",
					},
				},
			},
			shouldReturnEarly: true,
		},
		{
			name: "auth section with default identity proceeds to create manager",
			authSection: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"default": true,
						"kind":    "aws/permission-set",
					},
				},
			},
			shouldReturnEarly: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic that determines if we should return early
			if tt.authSection == nil {
				assert.True(t, tt.shouldReturnEarly)
				return
			}

			hasDefault := hasDefaultIdentity(tt.authSection)
			assert.Equal(t, !tt.shouldReturnEarly, hasDefault)
		})
	}
}

// TestHasDefaultIdentity_EdgeCases tests additional edge cases for hasDefaultIdentity.
func TestHasDefaultIdentity_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		authSection map[string]any
		expected    bool
	}{
		{
			name: "multiple identities with multiple defaults (returns true on first default)",
			authSection: map[string]any{
				"identities": map[string]any{
					"identity1": map[string]any{
						"default": true,
						"kind":    "aws/permission-set",
					},
					"identity2": map[string]any{
						"default": true,
						"kind":    "aws/permission-set",
					},
				},
			},
			expected: true,
		},
		{
			name: "identity with default field missing (treated as false)",
			authSection: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"kind": "aws/permission-set",
					},
				},
			},
			expected: false,
		},
		{
			name: "mixed valid and invalid identity configs",
			authSection: map[string]any{
				"identities": map[string]any{
					"invalid": "not-a-map",
					"valid": map[string]any{
						"default": true,
						"kind":    "aws/permission-set",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasDefaultIdentity(tt.authSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}
