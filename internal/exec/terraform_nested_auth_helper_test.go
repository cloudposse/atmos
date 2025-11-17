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
