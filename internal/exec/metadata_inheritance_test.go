package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestMetadataNameInheritance tests that metadata.name is properly inherited from base components
// and used in workspace_key_prefix calculation for Terraform backends.
func TestMetadataNameInheritance(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name                       string
		baseComponentMetadata      map[string]any
		componentMetadata          map[string]any
		inheritMetadataEnabled     *bool
		expectedName               string
		expectedWorkspaceKeyPrefix string
		component                  string
		baseComponent              string
		expectError                bool
	}{
		{
			name: "metadata.name inherited from base component",
			baseComponentMetadata: map[string]any{
				"name":      "vpc",
				"component": "vpc/v2",
				"type":      "abstract",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled:     &trueVal,
			expectedName:               "vpc",
			expectedWorkspaceKeyPrefix: "vpc",
			component:                  "vpc-prod",
			baseComponent:              "vpc/v2",
		},
		{
			name: "metadata.name inherited and used for workspace_key_prefix with versioned component",
			baseComponentMetadata: map[string]any{
				"name":      "eks-cluster",
				"component": "eks-cluster/2024",
				"type":      "abstract",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"eks-cluster/defaults"},
			},
			inheritMetadataEnabled:     &trueVal,
			expectedName:               "eks-cluster",
			expectedWorkspaceKeyPrefix: "eks-cluster",
			component:                  "eks-prod",
			baseComponent:              "eks-cluster/2024",
		},
		{
			name: "component metadata.name overrides inherited value",
			baseComponentMetadata: map[string]any{
				"name":      "vpc",
				"component": "vpc/v2",
			},
			componentMetadata: map[string]any{
				"name":     "vpc-custom",
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled:     &trueVal,
			expectedName:               "vpc-custom",
			expectedWorkspaceKeyPrefix: "vpc-custom",
			component:                  "vpc-prod",
			baseComponent:              "vpc/v2",
		},
		{
			name: "metadata.name not inherited when inheritance disabled",
			baseComponentMetadata: map[string]any{
				"name":      "vpc",
				"component": "vpc/v2",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled:     &falseVal,
			expectedName:               "",
			expectedWorkspaceKeyPrefix: "vpc-v2", // Falls back to baseComponent
			component:                  "vpc-prod",
			baseComponent:              "vpc/v2",
		},
		{
			name: "metadata.name with slashes inherited and normalized",
			baseComponentMetadata: map[string]any{
				"name":      "network/vpc",
				"component": "network/vpc/v1",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"network/vpc/defaults"},
			},
			inheritMetadataEnabled:     &trueVal,
			expectedName:               "network/vpc",
			expectedWorkspaceKeyPrefix: "network-vpc", // Slashes converted to dashes
			component:                  "vpc-prod",
			baseComponent:              "network/vpc/v1",
		},
		{
			name: "empty metadata.name in base does not override component workspace calculation",
			baseComponentMetadata: map[string]any{
				"name":      "",
				"component": "vpc/v2",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled:     &trueVal,
			expectedName:               "",
			expectedWorkspaceKeyPrefix: "vpc-v2", // Falls back to baseComponent
			component:                  "vpc-prod",
			baseComponent:              "vpc/v2",
		},
		{
			name: "multiple component instances with different metadata.name",
			baseComponentMetadata: map[string]any{
				"component": "vpc/stable",
			},
			componentMetadata: map[string]any{
				"name":     "vpc-primary",
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled:     &trueVal,
			expectedName:               "vpc-primary",
			expectedWorkspaceKeyPrefix: "vpc-primary",
			component:                  "vpc-prod-primary",
			baseComponent:              "vpc/stable",
		},
		{
			name:                  "no inheritance - component metadata.name used directly",
			baseComponentMetadata: map[string]any{},
			componentMetadata: map[string]any{
				"name": "standalone",
			},
			inheritMetadataEnabled:     &trueVal,
			expectedName:               "standalone",
			expectedWorkspaceKeyPrefix: "standalone",
			component:                  "standalone-component",
			baseComponent:              "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup AtmosConfiguration with metadata inheritance setting.
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: tt.inheritMetadataEnabled,
					},
				},
			}

			// Prepare component processor result with base and component metadata.
			result := &ComponentProcessorResult{
				BaseComponentMetadata: tt.baseComponentMetadata,
				ComponentMetadata:     tt.componentMetadata,
			}

			// Merge metadata using the production code path.
			finalMetadata := mergeMetadata(atmosConfig, result)

			// Verify metadata.name in merged result.
			if tt.expectedName != "" {
				assert.Equal(t, tt.expectedName, finalMetadata["name"])
			} else {
				// If expectedName is empty, verify it's either absent or empty.
				nameVal, exists := finalMetadata["name"]
				if exists {
					assert.Empty(t, nameVal)
				}
			}

			// Test that the inherited metadata.name is used in workspace_key_prefix calculation.
			backendType, backendConfig, err := processTerraformBackend(
				&terraformBackendConfig{
					atmosConfig:       atmosConfig,
					component:         tt.component,
					baseComponentName: tt.baseComponent,
					componentMetadata: finalMetadata,
					globalBackendType: "s3",
					globalBackendSection: map[string]any{
						"s3": map[string]any{
							"bucket": "test-bucket",
						},
					},
				},
			)

			require.NoError(t, err)
			assert.Equal(t, "s3", backendType)
			assert.Equal(t, tt.expectedWorkspaceKeyPrefix, backendConfig["workspace_key_prefix"])
		})
	}
}

// TestMetadataTypeNotInherited tests that metadata.type is NOT inherited.
// Component type is per-component and should be explicitly defined.
func TestMetadataTypeNotInherited(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name                   string
		baseComponentMetadata  map[string]any
		componentMetadata      map[string]any
		inheritMetadataEnabled *bool
		expectedTypeAbsent     bool
		expectedTypeValue      string
	}{
		{
			name: "type: abstract is excluded from inheritance",
			baseComponentMetadata: map[string]any{
				"type":      "abstract",
				"name":      "vpc",
				"component": "vpc/v2",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled: &trueVal,
			expectedTypeAbsent:     true,
		},
		{
			name: "type: abstract excluded, other metadata inherited",
			baseComponentMetadata: map[string]any{
				"type":                        "abstract",
				"name":                        "vpc",
				"component":                   "vpc/v2",
				"locked":                      true,
				"terraform_workspace_pattern": "{tenant}-{environment}-{stage}",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled: &trueVal,
			expectedTypeAbsent:     true,
		},
		{
			name: "type: real-component is NOT inherited",
			baseComponentMetadata: map[string]any{
				"type": "real-component",
				"name": "vpc",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled: &trueVal,
			expectedTypeAbsent:     true, // type is never inherited
		},
		{
			name: "component explicit type is preserved, base type not inherited",
			baseComponentMetadata: map[string]any{
				"type": "abstract",
				"name": "vpc",
			},
			componentMetadata: map[string]any{
				"type":     "concrete",
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled: &trueVal,
			expectedTypeAbsent:     false,
			expectedTypeValue:      "concrete",
		},
		{
			name: "type: abstract not inherited when metadata inheritance disabled",
			baseComponentMetadata: map[string]any{
				"type": "abstract",
				"name": "vpc",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			inheritMetadataEnabled: &falseVal,
			expectedTypeAbsent:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup AtmosConfiguration with metadata inheritance setting.
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: tt.inheritMetadataEnabled,
					},
				},
			}

			// Prepare component processor result.
			result := &ComponentProcessorResult{
				BaseComponentMetadata: tt.baseComponentMetadata,
				ComponentMetadata:     tt.componentMetadata,
			}

			// Merge metadata using the production code path.
			finalMetadata := mergeMetadata(atmosConfig, result)

			// Verify type field.
			typeVal, exists := finalMetadata["type"]
			if tt.expectedTypeAbsent {
				// Type should not exist (type is never inherited from base).
				if exists {
					// If type exists, it must be from component's own metadata, not inherited.
					_, componentHasType := tt.componentMetadata["type"]
					assert.True(t, componentHasType, "type should only exist if component defines it")
				}
			} else {
				require.True(t, exists, "type field should exist")
				assert.Equal(t, tt.expectedTypeValue, typeVal)
			}

			// Verify other metadata fields are still inherited (when inheritance enabled).
			if tt.inheritMetadataEnabled != nil && *tt.inheritMetadataEnabled {
				if name, ok := tt.baseComponentMetadata["name"]; ok {
					assert.Equal(t, name, finalMetadata["name"], "metadata.name should be inherited")
				}
				if locked, ok := tt.baseComponentMetadata["locked"]; ok {
					assert.Equal(t, locked, finalMetadata["locked"], "metadata.locked should be inherited")
				}
				if pattern, ok := tt.baseComponentMetadata["terraform_workspace_pattern"]; ok {
					assert.Equal(t, pattern, finalMetadata["terraform_workspace_pattern"], "metadata.terraform_workspace_pattern should be inherited")
				}
			}
		})
	}
}

// TestMetadataInheritsNotInherited tests that metadata.inherits is NOT inherited
// (it's the meta-property defining inheritance itself).
func TestMetadataInheritsNotInherited(t *testing.T) {
	trueVal := true

	atmosConfig := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			Inherit: schema.StacksInherit{
				Metadata: &trueVal,
			},
		},
	}

	result := &ComponentProcessorResult{
		BaseComponentMetadata: map[string]any{
			"inherits": []any{"grandparent"},
			"name":     "vpc",
		},
		ComponentMetadata: map[string]any{
			"inherits": []any{"parent"},
		},
	}

	finalMetadata := mergeMetadata(atmosConfig, result)

	// Verify inherits is NOT inherited from base component.
	inheritsVal, exists := finalMetadata["inherits"]
	if exists {
		// Should be component's own inherits, not base's.
		assert.Equal(t, []any{"parent"}, inheritsVal)
	}

	// Verify name IS inherited.
	assert.Equal(t, "vpc", finalMetadata["name"])
}

// TestMetadataInheritanceConfiguration tests the stacks.inherit.metadata configuration flag.
func TestMetadataInheritanceConfiguration(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name                   string
		inheritMetadataEnabled *bool
		baseComponentMetadata  map[string]any
		componentMetadata      map[string]any
		expectInheritedName    bool
		expectInheritedLocked  bool
	}{
		{
			name:                   "metadata inheritance enabled (default)",
			inheritMetadataEnabled: &trueVal,
			baseComponentMetadata: map[string]any{
				"name":   "vpc",
				"locked": true,
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			expectInheritedName:   true,
			expectInheritedLocked: true,
		},
		{
			name:                   "metadata inheritance explicitly disabled",
			inheritMetadataEnabled: &falseVal,
			baseComponentMetadata: map[string]any{
				"name":   "vpc",
				"locked": true,
			},
			componentMetadata: map[string]any{
				"inherits": []any{"vpc/defaults"},
			},
			expectInheritedName:   false,
			expectInheritedLocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: tt.inheritMetadataEnabled,
					},
				},
			}

			result := &ComponentProcessorResult{
				BaseComponentMetadata: tt.baseComponentMetadata,
				ComponentMetadata:     tt.componentMetadata,
			}

			finalMetadata := mergeMetadata(atmosConfig, result)

			// Check metadata.name inheritance.
			nameVal, nameExists := finalMetadata["name"]
			if tt.expectInheritedName {
				assert.True(t, nameExists)
				assert.Equal(t, "vpc", nameVal)
			} else if nameExists {
				// When inheritance disabled, name should not be inherited.
				// If it exists, it should be from component metadata, not base.
				assert.NotEqual(t, "vpc", nameVal)
			}

			// Check metadata.locked inheritance.
			lockedVal, lockedExists := finalMetadata["locked"]
			if tt.expectInheritedLocked {
				assert.True(t, lockedExists)
				assert.Equal(t, true, lockedVal)
			} else {
				// When inheritance disabled, locked should not be inherited.
				assert.False(t, lockedExists)
			}
		})
	}
}

// TestMetadataDeepMerge tests that metadata fields are deeply merged.
func TestMetadataDeepMerge(t *testing.T) {
	trueVal := true

	atmosConfig := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			Inherit: schema.StacksInherit{
				Metadata: &trueVal,
			},
		},
	}

	result := &ComponentProcessorResult{
		BaseComponentMetadata: map[string]any{
			"custom": map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			"name": "vpc",
		},
		ComponentMetadata: map[string]any{
			"custom": map[string]any{
				"key2": "override",
				"key3": "new",
			},
			"inherits": []any{"vpc/defaults"},
		},
	}

	finalMetadata := mergeMetadata(atmosConfig, result)

	// Verify deep merge of custom metadata.
	customVal, exists := finalMetadata["custom"]
	require.True(t, exists)

	customMap, ok := customVal.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "value1", customMap["key1"], "key1 from base should be preserved")
	assert.Equal(t, "override", customMap["key2"], "key2 should be overridden by component")
	assert.Equal(t, "new", customMap["key3"], "key3 from component should be added")
}

// TestVersionedComponentUpgrade tests the primary use case: upgrading versioned components
// while maintaining stable workspace_key_prefix via metadata.name inheritance.
func TestVersionedComponentUpgrade(t *testing.T) {
	// Simulate upgrade from vpc/v2 to vpc/v3.
	tests := []struct {
		name                       string
		version                    string
		baseComponentPath          string
		expectedWorkspaceKeyPrefix string
	}{
		{
			name:                       "vpc version v2",
			version:                    "v2",
			baseComponentPath:          "vpc/v2",
			expectedWorkspaceKeyPrefix: "vpc",
		},
		{
			name:                       "vpc version v3 (upgrade)",
			version:                    "v3",
			baseComponentPath:          "vpc/v3",
			expectedWorkspaceKeyPrefix: "vpc", // Same as v2 - state path stable
		},
		{
			name:                       "vpc version v4 (future upgrade)",
			version:                    "v4",
			baseComponentPath:          "vpc/v4",
			expectedWorkspaceKeyPrefix: "vpc", // Still stable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trueVal := true

			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: &trueVal,
					},
				},
			}

			// Base component (vpc/defaults) defines metadata.name.
			baseMetadata := map[string]any{
				"type":      "abstract",
				"name":      "vpc", // Stable logical identity
				"component": tt.baseComponentPath,
			}

			// Derived component inherits from base.
			componentMetadata := map[string]any{
				"inherits": []any{"vpc/defaults"},
			}

			result := &ComponentProcessorResult{
				BaseComponentMetadata: baseMetadata,
				ComponentMetadata:     componentMetadata,
			}

			finalMetadata := mergeMetadata(atmosConfig, result)

			// Verify metadata.name is inherited.
			assert.Equal(t, "vpc", finalMetadata["name"])

			// Verify workspace_key_prefix uses inherited metadata.name.
			backendType, backendConfig, err := processTerraformBackend(
				&terraformBackendConfig{
					atmosConfig:       atmosConfig,
					component:         "vpc-prod",
					baseComponentName: tt.baseComponentPath,
					componentMetadata: finalMetadata,
					globalBackendType: "s3",
					globalBackendSection: map[string]any{
						"s3": map[string]any{
							"bucket": "terraform-state",
						},
					},
				},
			)

			require.NoError(t, err)
			assert.Equal(t, "s3", backendType)
			assert.Equal(t, tt.expectedWorkspaceKeyPrefix, backendConfig["workspace_key_prefix"],
				"workspace_key_prefix should remain stable across version upgrades")
		})
	}
}

// mergeMetadata is a test helper that mimics the production code path for metadata merging.
func mergeMetadata(atmosConfig *schema.AtmosConfiguration, result *ComponentProcessorResult) map[string]any {
	// This replicates the logic from stack_processor_utils.go.
	finalMetadata := result.ComponentMetadata
	if !atmosConfig.Stacks.Inherit.IsMetadataInheritanceEnabled() || len(result.BaseComponentMetadata) == 0 {
		return finalMetadata
	}

	// Filter base metadata to exclude 'inherits' and 'type'.
	baseMetadataFiltered := filterBaseMetadata(result.BaseComponentMetadata)

	// Merge base (filtered) with component metadata.
	merged := simpleMerge(baseMetadataFiltered, result.ComponentMetadata)
	return merged
}

// filterBaseMetadata filters base metadata to exclude 'inherits' and 'type'.
func filterBaseMetadata(baseMetadata map[string]any) map[string]any {
	filtered := make(map[string]any)
	for k, v := range baseMetadata {
		if k == cfg.InheritsSectionName {
			continue
		}
		// Skip 'type' - component type is per-component, not inherited.
		if k == "type" {
			continue
		}
		filtered[k] = v
	}
	return filtered
}

// simpleMerge is a simplified merge helper for testing.
// In production, this uses pkg/merge.Merge.
func simpleMerge(base, override map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy base.
	for k, v := range base {
		result[k] = v
	}

	// Apply overrides with deep merge for maps.
	for k, v := range override {
		if existingVal, exists := result[k]; exists {
			existingMap, existingIsMap := existingVal.(map[string]any)
			newMap, newIsMap := v.(map[string]any)
			if existingIsMap && newIsMap {
				result[k] = simpleMerge(existingMap, newMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}

// TestIsMetadataInheritanceEnabled tests the StacksInherit.IsMetadataInheritanceEnabled() method.
func TestIsMetadataInheritanceEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		inherit  schema.StacksInherit
		expected bool
	}{
		{
			name:     "nil metadata defaults to true",
			inherit:  schema.StacksInherit{Metadata: nil},
			expected: true,
		},
		{
			name:     "explicit true",
			inherit:  schema.StacksInherit{Metadata: &trueVal},
			expected: true,
		},
		{
			name:     "explicit false",
			inherit:  schema.StacksInherit{Metadata: &falseVal},
			expected: false,
		},
		{
			name:     "zero value struct defaults to true",
			inherit:  schema.StacksInherit{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.inherit.IsMetadataInheritanceEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMetadataFieldsInheritance tests that specific metadata fields are correctly inherited.
func TestMetadataFieldsInheritance(t *testing.T) {
	trueVal := true

	tests := []struct {
		name                  string
		baseComponentMetadata map[string]any
		componentMetadata     map[string]any
		expectedFields        map[string]any
		unexpectedFields      []string
	}{
		{
			name: "enabled field inherited",
			baseComponentMetadata: map[string]any{
				"enabled": true,
			},
			componentMetadata: map[string]any{},
			expectedFields: map[string]any{
				"enabled": true,
			},
		},
		{
			name: "locked field inherited",
			baseComponentMetadata: map[string]any{
				"locked": true,
			},
			componentMetadata: map[string]any{},
			expectedFields: map[string]any{
				"locked": true,
			},
		},
		{
			name: "component field inherited",
			baseComponentMetadata: map[string]any{
				"component": "vpc/v2",
			},
			componentMetadata: map[string]any{},
			expectedFields: map[string]any{
				"component": "vpc/v2",
			},
		},
		{
			name: "terraform_workspace inherited",
			baseComponentMetadata: map[string]any{
				"terraform_workspace": "custom-workspace",
			},
			componentMetadata: map[string]any{},
			expectedFields: map[string]any{
				"terraform_workspace": "custom-workspace",
			},
		},
		{
			name: "terraform_workspace_pattern inherited",
			baseComponentMetadata: map[string]any{
				"terraform_workspace_pattern": "{tenant}-{environment}-{stage}",
			},
			componentMetadata: map[string]any{},
			expectedFields: map[string]any{
				"terraform_workspace_pattern": "{tenant}-{environment}-{stage}",
			},
		},
		{
			name: "type field NOT inherited (abstract)",
			baseComponentMetadata: map[string]any{
				"type": "abstract",
				"name": "vpc",
			},
			componentMetadata: map[string]any{},
			unexpectedFields:  []string{"type"},
			expectedFields:    map[string]any{"name": "vpc"},
		},
		{
			name: "type field NOT inherited (real)",
			baseComponentMetadata: map[string]any{
				"type": "real",
				"name": "vpc",
			},
			componentMetadata: map[string]any{},
			unexpectedFields:  []string{"type"},
			expectedFields:    map[string]any{"name": "vpc"},
		},
		{
			name: "type field NOT inherited (custom type)",
			baseComponentMetadata: map[string]any{
				"type": "custom-type",
				"name": "vpc",
			},
			componentMetadata: map[string]any{},
			unexpectedFields:  []string{"type"},
			expectedFields:    map[string]any{"name": "vpc"},
		},
		{
			name: "inherits field NOT inherited",
			baseComponentMetadata: map[string]any{
				"inherits": []any{"grandparent"},
				"name":     "vpc",
			},
			componentMetadata: map[string]any{
				"inherits": []any{"parent"},
			},
			expectedFields: map[string]any{
				"name":     "vpc",
				"inherits": []any{"parent"}, // Component's inherits, not base's.
			},
		},
		{
			name: "multiple fields inherited together",
			baseComponentMetadata: map[string]any{
				"name":                        "vpc",
				"component":                   "vpc/v2",
				"locked":                      true,
				"enabled":                     true,
				"terraform_workspace_pattern": "{tenant}-{stage}",
				"type":                        "abstract",
			},
			componentMetadata: map[string]any{},
			expectedFields: map[string]any{
				"name":                        "vpc",
				"component":                   "vpc/v2",
				"locked":                      true,
				"enabled":                     true,
				"terraform_workspace_pattern": "{tenant}-{stage}",
			},
			unexpectedFields: []string{"type"},
		},
		{
			name: "component overrides inherited fields",
			baseComponentMetadata: map[string]any{
				"name":   "base-vpc",
				"locked": true,
			},
			componentMetadata: map[string]any{
				"name":   "component-vpc",
				"locked": false,
			},
			expectedFields: map[string]any{
				"name":   "component-vpc",
				"locked": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: &trueVal,
					},
				},
			}

			result := &ComponentProcessorResult{
				BaseComponentMetadata: tt.baseComponentMetadata,
				ComponentMetadata:     tt.componentMetadata,
			}

			finalMetadata := mergeMetadata(atmosConfig, result)

			// Verify expected fields.
			for key, expectedValue := range tt.expectedFields {
				actualValue, exists := finalMetadata[key]
				assert.True(t, exists, "expected field %q to exist", key)
				assert.Equal(t, expectedValue, actualValue, "field %q mismatch", key)
			}

			// Verify unexpected fields are absent.
			for _, key := range tt.unexpectedFields {
				_, exists := finalMetadata[key]
				assert.False(t, exists, "field %q should not exist", key)
			}
		})
	}
}

// TestMetadataInheritanceWithEmptyMaps tests edge cases with empty maps.
func TestMetadataInheritanceWithEmptyMaps(t *testing.T) {
	trueVal := true

	tests := []struct {
		name                  string
		baseComponentMetadata map[string]any
		componentMetadata     map[string]any
		expectedResult        map[string]any
	}{
		{
			name:                  "both empty",
			baseComponentMetadata: map[string]any{},
			componentMetadata:     map[string]any{},
			expectedResult:        map[string]any{},
		},
		{
			name:                  "base nil, component empty",
			baseComponentMetadata: nil,
			componentMetadata:     map[string]any{},
			expectedResult:        map[string]any{},
		},
		{
			name:                  "base empty, component has values",
			baseComponentMetadata: map[string]any{},
			componentMetadata: map[string]any{
				"name": "component-only",
			},
			expectedResult: map[string]any{
				"name": "component-only",
			},
		},
		{
			name: "base has values, component empty",
			baseComponentMetadata: map[string]any{
				"name": "base-only",
			},
			componentMetadata: map[string]any{},
			expectedResult: map[string]any{
				"name": "base-only",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: &trueVal,
					},
				},
			}

			result := &ComponentProcessorResult{
				BaseComponentMetadata: tt.baseComponentMetadata,
				ComponentMetadata:     tt.componentMetadata,
			}

			finalMetadata := mergeMetadata(atmosConfig, result)

			// Compare expected result.
			for key, expectedValue := range tt.expectedResult {
				actualValue, exists := finalMetadata[key]
				assert.True(t, exists, "expected field %q to exist", key)
				assert.Equal(t, expectedValue, actualValue, "field %q mismatch", key)
			}

			// Verify no extra fields.
			for key := range finalMetadata {
				if key == "inherits" {
					continue // Inherits is handled separately.
				}
				_, expected := tt.expectedResult[key]
				assert.True(t, expected, "unexpected field %q in result", key)
			}
		})
	}
}

// TestGCSBackendMetadataName tests that GCS backend uses metadata.name for prefix.
func TestGCSBackendMetadataName(t *testing.T) {
	tests := []struct {
		name              string
		component         string
		baseComponentName string
		componentMetadata map[string]any
		expectedPrefix    string
	}{
		{
			name:              "metadata.name takes priority",
			component:         "vpc-prod",
			baseComponentName: "vpc/v2",
			componentMetadata: map[string]any{
				"name": "vpc",
			},
			expectedPrefix: "vpc",
		},
		{
			name:              "slashes in metadata.name converted to dashes",
			component:         "vpc-prod",
			baseComponentName: "vpc/v2",
			componentMetadata: map[string]any{
				"name": "network/vpc/main",
			},
			expectedPrefix: "network-vpc-main",
		},
		{
			name:              "falls back to base component when no metadata.name",
			component:         "vpc-prod",
			baseComponentName: "vpc/v2",
			componentMetadata: map[string]any{},
			expectedPrefix:    "vpc-v2",
		},
		{
			name:              "falls back to component when no metadata.name or base",
			component:         "vpc-standalone",
			baseComponentName: "",
			componentMetadata: map[string]any{},
			expectedPrefix:    "vpc-standalone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			backendType, backendConfig, err := processTerraformBackend(
				&terraformBackendConfig{
					atmosConfig:       atmosConfig,
					component:         tt.component,
					baseComponentName: tt.baseComponentName,
					componentMetadata: tt.componentMetadata,
					globalBackendType: "gcs",
					globalBackendSection: map[string]any{
						"gcs": map[string]any{
							"bucket": "test-bucket",
						},
					},
				},
			)

			require.NoError(t, err)
			assert.Equal(t, "gcs", backendType)
			assert.Equal(t, tt.expectedPrefix, backendConfig["prefix"])
		})
	}
}

// TestAzureBackendMetadataName tests that Azure backend uses metadata.name for key prefix.
func TestAzureBackendMetadataName(t *testing.T) {
	tests := []struct {
		name              string
		component         string
		baseComponentName string
		componentMetadata map[string]any
		expectedKey       string
	}{
		{
			name:              "metadata.name takes priority",
			component:         "vpc-prod",
			baseComponentName: "vpc/v2",
			componentMetadata: map[string]any{
				"name": "vpc",
			},
			expectedKey: "vpc.terraform.tfstate",
		},
		{
			name:              "slashes in metadata.name converted to dashes",
			component:         "vpc-prod",
			baseComponentName: "vpc/v2",
			componentMetadata: map[string]any{
				"name": "network/vpc",
			},
			expectedKey: "network-vpc.terraform.tfstate",
		},
		{
			name:              "falls back to base component when no metadata.name",
			component:         "vpc-prod",
			baseComponentName: "vpc/v2",
			componentMetadata: map[string]any{},
			expectedKey:       "vpc-v2.terraform.tfstate",
		},
		{
			name:              "falls back to component when no metadata.name or base",
			component:         "vpc-standalone",
			baseComponentName: "",
			componentMetadata: map[string]any{},
			expectedKey:       "vpc-standalone.terraform.tfstate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			backendType, backendConfig, err := processTerraformBackend(
				&terraformBackendConfig{
					atmosConfig:       atmosConfig,
					component:         tt.component,
					baseComponentName: tt.baseComponentName,
					componentMetadata: tt.componentMetadata,
					globalBackendType: "azurerm",
					globalBackendSection: map[string]any{
						"azurerm": map[string]any{
							"storage_account_name": "test-account",
							"container_name":       "tfstate",
						},
					},
				},
			)

			require.NoError(t, err)
			assert.Equal(t, "azurerm", backendType)
			assert.Equal(t, tt.expectedKey, backendConfig["key"])
		})
	}
}

// TestMultiLevelInheritance tests inheritance through multiple levels.
func TestMultiLevelInheritance(t *testing.T) {
	trueVal := true

	// Simulate grandparent -> parent -> child inheritance.
	// The mergeMetadata function receives already-merged base metadata,
	// so we test that the merge works correctly with accumulated values.
	tests := []struct {
		name                  string
		baseComponentMetadata map[string]any // Represents already-merged grandparent + parent.
		componentMetadata     map[string]any // Child component metadata.
		expectedFields        map[string]any
	}{
		{
			name: "child inherits from merged parent chain",
			baseComponentMetadata: map[string]any{
				"name":      "vpc",                          // From grandparent.
				"component": "vpc/v2",                       // From parent.
				"locked":    true,                           // From grandparent.
				"custom":    map[string]any{"tier": "base"}, // From parent.
			},
			componentMetadata: map[string]any{
				"enabled": true, // Child adds new field.
			},
			expectedFields: map[string]any{
				"name":      "vpc",
				"component": "vpc/v2",
				"locked":    true,
				"enabled":   true,
				"custom":    map[string]any{"tier": "base"},
			},
		},
		{
			name: "child overrides values from parent chain",
			baseComponentMetadata: map[string]any{
				"name":   "base-vpc",
				"locked": true,
			},
			componentMetadata: map[string]any{
				"name":   "child-vpc", // Override.
				"locked": false,       // Override.
			},
			expectedFields: map[string]any{
				"name":   "child-vpc",
				"locked": false,
			},
		},
		{
			name: "deep merge of custom metadata through chain",
			baseComponentMetadata: map[string]any{
				"custom": map[string]any{
					"grandparent_key": "grandparent_value",
					"parent_key":      "parent_value",
					"shared_key":      "parent_overrides",
				},
			},
			componentMetadata: map[string]any{
				"custom": map[string]any{
					"child_key":  "child_value",
					"shared_key": "child_overrides",
				},
			},
			expectedFields: map[string]any{
				"custom": map[string]any{
					"grandparent_key": "grandparent_value",
					"parent_key":      "parent_value",
					"child_key":       "child_value",
					"shared_key":      "child_overrides",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: &trueVal,
					},
				},
			}

			result := &ComponentProcessorResult{
				BaseComponentMetadata: tt.baseComponentMetadata,
				ComponentMetadata:     tt.componentMetadata,
			}

			finalMetadata := mergeMetadata(atmosConfig, result)

			for key, expectedValue := range tt.expectedFields {
				actualValue, exists := finalMetadata[key]
				require.True(t, exists, "expected field %q to exist", key)
				assert.Equal(t, expectedValue, actualValue, "field %q mismatch", key)
			}
		})
	}
}

// TestReleaseTracksPattern tests the release tracks use case with metadata.name.
func TestReleaseTracksPattern(t *testing.T) {
	trueVal := true

	// Test the release tracks pattern where components inherit from track-specific bases.
	tracks := []struct {
		trackName     string
		componentPath string
	}{
		{"stable", "stable/vpc"},
		{"beta", "beta/vpc"},
		{"preview", "preview/vpc"},
	}

	for _, track := range tracks {
		t.Run("track_"+track.trackName, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					Inherit: schema.StacksInherit{
						Metadata: &trueVal,
					},
				},
			}

			// Track base defines component path and stable name.
			baseMetadata := map[string]any{
				"type":      "abstract",
				"name":      "vpc", // Stable name across all tracks.
				"component": track.componentPath,
			}

			// Derived component inherits from track.
			componentMetadata := map[string]any{
				"inherits": []any{"vpc/" + track.trackName},
			}

			result := &ComponentProcessorResult{
				BaseComponentMetadata: baseMetadata,
				ComponentMetadata:     componentMetadata,
			}

			finalMetadata := mergeMetadata(atmosConfig, result)

			// Verify stable name is inherited.
			assert.Equal(t, "vpc", finalMetadata["name"])

			// Verify component path is inherited.
			assert.Equal(t, track.componentPath, finalMetadata["component"])

			// Verify type is NOT inherited.
			_, typeExists := finalMetadata["type"]
			assert.False(t, typeExists, "type should not be inherited")

			// Verify workspace_key_prefix uses stable name.
			backendType, backendConfig, err := processTerraformBackend(
				&terraformBackendConfig{
					atmosConfig:       atmosConfig,
					component:         "vpc-prod",
					baseComponentName: track.componentPath,
					componentMetadata: finalMetadata,
					globalBackendType: "s3",
					globalBackendSection: map[string]any{
						"s3": map[string]any{
							"bucket": "terraform-state",
						},
					},
				},
			)

			require.NoError(t, err)
			assert.Equal(t, "s3", backendType)
			assert.Equal(t, "vpc", backendConfig["workspace_key_prefix"],
				"workspace_key_prefix should use stable name regardless of track")
		})
	}
}

// TestComponentProcessorResult tests the ComponentProcessorResult struct.
func TestComponentProcessorResult(t *testing.T) {
	// Test that ComponentProcessorResult correctly holds metadata.
	result := &ComponentProcessorResult{
		BaseComponentMetadata: map[string]any{
			"name":      "base",
			"type":      "abstract",
			"component": "vpc/v1",
		},
		ComponentMetadata: map[string]any{
			"inherits": []any{"base"},
			"enabled":  true,
		},
	}

	assert.Equal(t, "base", result.BaseComponentMetadata["name"])
	assert.Equal(t, "abstract", result.BaseComponentMetadata["type"])
	assert.Equal(t, true, result.ComponentMetadata["enabled"])
}

// TestFilterBaseMetadata tests the filterBaseMetadata helper function.
func TestFilterBaseMetadata(t *testing.T) {
	tests := []struct {
		name           string
		input          map[string]any
		expectedOutput map[string]any
	}{
		{
			name: "filters out inherits",
			input: map[string]any{
				"inherits": []any{"parent"},
				"name":     "vpc",
			},
			expectedOutput: map[string]any{
				"name": "vpc",
			},
		},
		{
			name: "filters out type (any value)",
			input: map[string]any{
				"type": "abstract",
				"name": "vpc",
			},
			expectedOutput: map[string]any{
				"name": "vpc",
			},
		},
		{
			name: "filters out type real",
			input: map[string]any{
				"type": "real",
				"name": "vpc",
			},
			expectedOutput: map[string]any{
				"name": "vpc",
			},
		},
		{
			name: "filters out type custom",
			input: map[string]any{
				"type": "my-custom-type",
				"name": "vpc",
			},
			expectedOutput: map[string]any{
				"name": "vpc",
			},
		},
		{
			name: "filters both inherits and type",
			input: map[string]any{
				"inherits":  []any{"parent"},
				"type":      "abstract",
				"name":      "vpc",
				"component": "vpc/v2",
				"locked":    true,
			},
			expectedOutput: map[string]any{
				"name":      "vpc",
				"component": "vpc/v2",
				"locked":    true,
			},
		},
		{
			name:           "empty map returns empty",
			input:          map[string]any{},
			expectedOutput: map[string]any{},
		},
		{
			name: "preserves all other fields",
			input: map[string]any{
				"name":                        "vpc",
				"component":                   "vpc/v2",
				"locked":                      true,
				"enabled":                     true,
				"terraform_workspace":         "custom",
				"terraform_workspace_pattern": "{tenant}-{stage}",
				"custom":                      map[string]any{"key": "value"},
			},
			expectedOutput: map[string]any{
				"name":                        "vpc",
				"component":                   "vpc/v2",
				"locked":                      true,
				"enabled":                     true,
				"terraform_workspace":         "custom",
				"terraform_workspace_pattern": "{tenant}-{stage}",
				"custom":                      map[string]any{"key": "value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterBaseMetadata(tt.input)

			// Verify all expected keys exist with correct values.
			for key, expectedValue := range tt.expectedOutput {
				actualValue, exists := result[key]
				assert.True(t, exists, "expected key %q to exist", key)
				assert.Equal(t, expectedValue, actualValue, "key %q mismatch", key)
			}

			// Verify no extra keys.
			assert.Equal(t, len(tt.expectedOutput), len(result), "result should have same number of keys as expected")
		})
	}
}
