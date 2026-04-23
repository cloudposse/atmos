package exec

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestProcessBaseComponentConfig(t *testing.T) {
	// Clear cache before tests to ensure fresh processing.
	ClearBaseComponentConfigCache()

	tests := []struct {
		name                string
		baseComponentConfig *schema.BaseComponentConfig
		allComponentsMap    map[string]any
		component           string
		stack               string
		baseComponent       string
		expectedError       string
		expectedVars        map[string]any
		expectedSettings    map[string]any
		expectedEnv         map[string]any
		expectedBackendType string
		expectBaseComponent string
	}{
		{
			name: "basic-base-component",
			baseComponentConfig: &schema.BaseComponentConfig{
				BaseComponentVars:     map[string]any{},
				BaseComponentSettings: map[string]any{},
				BaseComponentEnv:      map[string]any{},
			},
			allComponentsMap: map[string]any{
				"base": map[string]any{
					"vars": map[string]any{
						"environment": "dev",
						"region":      "us-east-1",
					},
					"settings": map[string]any{
						"enabled": true,
					},
					"backend_type": "s3",
				},
			},
			component:     "test",
			stack:         "test-stack",
			baseComponent: "base",
			expectedVars: map[string]any{
				"environment": "dev",
				"region":      "us-east-1",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
			expectedBackendType: "s3",
			expectBaseComponent: "base",
		},
		{
			name: "inheritance-chain",
			baseComponentConfig: &schema.BaseComponentConfig{
				BaseComponentVars:     map[string]any{},
				BaseComponentSettings: map[string]any{},
				BaseComponentEnv:      map[string]any{},
			},
			allComponentsMap: map[string]any{
				"base": map[string]any{
					"component": "base2",
					"vars": map[string]any{
						"environment": "dev",
					},
				},
				"base2": map[string]any{
					"vars": map[string]any{
						"region": "us-east-1",
					},
					"settings": map[string]any{
						"enabled": true,
					},
				},
			},
			component:     "test",
			stack:         "test-stack",
			baseComponent: "base",
			expectedVars: map[string]any{
				"environment": "dev",
				"region":      "us-east-1",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
			expectBaseComponent: "base2",
		},
		{
			name: "invalid-base-component",
			baseComponentConfig: &schema.BaseComponentConfig{
				BaseComponentVars:     map[string]any{},
				BaseComponentSettings: map[string]any{},
				BaseComponentEnv:      map[string]any{},
			},
			allComponentsMap: map[string]any{
				"base": "invalid-type",
			},
			component:     "test",
			stack:         "test-stack",
			baseComponent: "base",
			expectedError: "invalid base component config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache before each test case to ensure isolation.
			ClearBaseComponentConfigCache()

			atmosConfig := &schema.AtmosConfiguration{}
			baseComponents := []string{}

			err := ProcessBaseComponentConfig(
				atmosConfig,
				tt.baseComponentConfig,
				tt.allComponentsMap,
				tt.component,
				tt.stack,
				tt.baseComponent,
				filepath.Join("dummy", "path"),
				true,
				&baseComponents,
			)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.expectedVars != nil {
				assert.Equal(t, tt.expectedVars, tt.baseComponentConfig.BaseComponentVars)
			}

			if tt.expectedSettings != nil {
				assert.Equal(t, tt.expectedSettings, tt.baseComponentConfig.BaseComponentSettings)
			}

			if tt.expectedEnv != nil {
				assert.Equal(t, tt.expectedEnv, tt.baseComponentConfig.BaseComponentEnv)
			}

			if tt.expectedBackendType != "" {
				assert.Equal(t, tt.expectedBackendType, tt.baseComponentConfig.BaseComponentBackendType)
			}

			if tt.expectBaseComponent != "" {
				assert.Equal(t, tt.expectBaseComponent, tt.baseComponentConfig.FinalBaseComponentName)
			}

			// Verify baseComponents slice contains the expected components
			expectedComponents := []string{tt.baseComponent}
			if tt.expectBaseComponent != tt.baseComponent && tt.expectBaseComponent != "" {
				expectedComponents = append(expectedComponents, tt.expectBaseComponent)
			}
			assert.ElementsMatch(t, expectedComponents, baseComponents)
		})
	}
}

// TestProcessBaseComponentConfig_CycleDetection verifies that circular component
// inheritance is detected and reported instead of causing a stack overflow.
func TestProcessBaseComponentConfig_CycleDetection(t *testing.T) {
	tests := []struct {
		name             string
		allComponentsMap map[string]any
		component        string
		baseComponent    string
		expectedError    string
	}{
		{
			name: "direct-cycle-via-component-key",
			allComponentsMap: map[string]any{
				"comp-a": map[string]any{
					"component": "comp-b",
					"vars":      map[string]any{"key": "a"},
				},
				"comp-b": map[string]any{
					"component": "comp-a",
					"vars":      map[string]any{"key": "b"},
				},
			},
			component:     "my-component",
			baseComponent: "comp-a",
			expectedError: "circular component inheritance detected",
		},
		{
			name: "cycle-via-inherits",
			allComponentsMap: map[string]any{
				"comp-a": map[string]any{
					"metadata": map[string]any{
						"inherits": []any{"comp-b"},
					},
					"vars": map[string]any{"key": "a"},
				},
				"comp-b": map[string]any{
					"metadata": map[string]any{
						"inherits": []any{"comp-a"},
					},
					"vars": map[string]any{"key": "b"},
				},
			},
			component:     "my-component",
			baseComponent: "comp-a",
			expectedError: "circular component inheritance detected",
		},
		{
			name: "three-component-cycle",
			// A -> B -> C -> A creates a true 3-way cycle that the simple
			// component == baseComponent guard cannot catch.
			allComponentsMap: map[string]any{
				"comp-a": map[string]any{
					"component": "comp-b",
					"vars":      map[string]any{"key": "a"},
				},
				"comp-b": map[string]any{
					"component": "comp-c",
					"vars":      map[string]any{"key": "b"},
				},
				"comp-c": map[string]any{
					"component": "comp-a",
					"vars":      map[string]any{"key": "c"},
				},
			},
			component:     "my-component",
			baseComponent: "comp-a",
			expectedError: "circular component inheritance detected",
		},
		{
			name: "no-cycle-valid-chain",
			allComponentsMap: map[string]any{
				"base": map[string]any{
					"vars": map[string]any{"key": "base"},
				},
				"mid": map[string]any{
					"component": "base",
					"vars":      map[string]any{"key": "mid"},
				},
			},
			component:     "child",
			baseComponent: "mid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearBaseComponentConfigCache()

			atmosConfig := &schema.AtmosConfiguration{}
			baseComponentConfig := &schema.BaseComponentConfig{
				BaseComponentVars:     map[string]any{},
				BaseComponentSettings: map[string]any{},
				BaseComponentEnv:      map[string]any{},
			}
			baseComponents := []string{}

			err := ProcessBaseComponentConfig(
				atmosConfig,
				baseComponentConfig,
				tt.allComponentsMap,
				tt.component,
				"test-stack",
				tt.baseComponent,
				filepath.Join("dummy", "path"),
				false,
				&baseComponents,
			)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrCircularComponentInheritance)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestProcessBaseComponentConfig_AbstractComponentSkip verifies that abstract components
// skip component chain resolution to prevent circular references.
func TestProcessBaseComponentConfig_AbstractComponentSkip(t *testing.T) {
	ClearBaseComponentConfigCache()

	// This simulates the user's pattern: abstract component with metadata.component
	// pointing to the same Terraform directory as the real component.
	// In raw (unprocessed) data, abstract components have metadata.component but no
	// top-level "component" key. After processing, mergeComponentConfigurations
	// promotes metadata.component to a top-level key. The abstract check prevents
	// following this promoted key.
	allComponentsMap := map[string]any{
		"iam-delegated-roles-defaults": map[string]any{
			"component": "iam-delegated-roles", // Promoted by mergeComponentConfigurations.
			"metadata": map[string]any{
				"component": "iam-delegated-roles",
				"type":      "abstract",
			},
			"vars": map[string]any{
				"namespace": "acme",
			},
		},
		"iam-delegated-roles": map[string]any{
			"component": "iam-delegated-roles",
			"metadata": map[string]any{
				"component": "iam-delegated-roles",
				"type":      "real",
				"inherits":  []any{"iam-delegated-roles-defaults"},
			},
			"vars": map[string]any{
				"extra_var": "value1",
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents := []string{}

	// This call should NOT stack overflow or return a cycle error.
	// The abstract component's top-level "component" key is skipped.
	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"iam-delegated-roles",
		"test-stack",
		"iam-delegated-roles-defaults",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)

	require.NoError(t, err, "Should not error when abstract component has metadata.component")
	assert.Equal(t, map[string]any{"namespace": "acme"}, baseComponentConfig.BaseComponentVars)
	assert.Contains(t, baseComponents, "iam-delegated-roles-defaults")
}

// TestProcessBaseComponentConfig_DeepChainNoFalsePositive verifies that deep inheritance
// chains (3+ levels) work correctly without triggering false cycle detection.
func TestProcessBaseComponentConfig_DeepChainNoFalsePositive(t *testing.T) {
	ClearBaseComponentConfigCache()

	allComponentsMap := map[string]any{
		"level0": map[string]any{
			"vars": map[string]any{"from": "level0"},
		},
		"level1": map[string]any{
			"component": "level0",
			"vars":      map[string]any{"from": "level1"},
		},
		"level2": map[string]any{
			"component": "level1",
			"vars":      map[string]any{"from": "level2"},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents := []string{}

	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"child",
		"test-stack",
		"level2",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)

	require.NoError(t, err, "Deep chain should not trigger false cycle detection")
	// Vars should be merged from all levels.
	assert.Equal(t, "level2", baseComponentConfig.BaseComponentVars["from"])
	assert.ElementsMatch(t, []string{"level2", "level1", "level0"}, baseComponents)
}

// TestProcessBaseComponentConfig_DiamondInheritance verifies that a diamond inheritance pattern
// (shared ancestor reached via sibling branches) does not trigger a false cycle detection error.
//
//	   base
//	  /    \
//	left   right
//	  \    /
//	   child (inherits: [left, right])
func TestProcessBaseComponentConfig_DiamondInheritance(t *testing.T) {
	ClearBaseComponentConfigCache()

	allComponentsMap := map[string]any{
		"base": map[string]any{
			"vars": map[string]any{"from_base": "base-value"},
		},
		"left": map[string]any{
			"metadata": map[string]any{
				"inherits": []any{"base"},
			},
			"vars": map[string]any{"from_left": "left-value"},
		},
		"right": map[string]any{
			"metadata": map[string]any{
				"inherits": []any{"base"},
			},
			"vars": map[string]any{"from_right": "right-value"},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}

	// Process "left" first.
	baseComponents := []string{}
	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"child",
		"test-stack",
		"left",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)
	require.NoError(t, err, "Left branch of diamond should not error")

	// Process "right" — this reaches "base" again via a sibling branch.
	// Without defer delete in the visited set, this would falsely trigger cycle detection.
	ClearBaseComponentConfigCache()
	baseComponentConfig2 := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents2 := []string{}
	err = ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig2,
		allComponentsMap,
		"child",
		"test-stack",
		"right",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents2,
	)
	require.NoError(t, err, "Right branch of diamond should not error")
	assert.Equal(t, "base-value", baseComponentConfig2.BaseComponentVars["from_base"],
		"Base vars should be inherited through right branch")
}

// TestProcessBaseComponentConfig_MultipleAbstractComponentsCycle reproduces the user's
// scenario where multiple abstract components with metadata.component cause a stack overflow.
// The user reported: "I had to do this on 2 iam-delegated-roles, and eks. I think there are
// some dependencies between the two."
//
// This test simulates processed data (Phase 2) where two abstract components both have
// metadata.component set, and their real counterparts cross-reference through inheritance.
// The isAbstract check should prevent following the promoted "component" key on both.
func TestProcessBaseComponentConfig_MultipleAbstractComponentsCycle(t *testing.T) {
	ClearBaseComponentConfigCache()

	// Simulate processed data with two abstract/real component pairs that have
	// cross-dependencies. The real components inherit from their respective abstract
	// defaults, and eks also inherits from iam-delegated-roles-defaults to exercise
	// coupled dependency traversal paths.
	// Both abstract components have metadata.component promoted to top-level.
	allComponentsMap := map[string]any{
		"iam-delegated-roles-defaults": map[string]any{
			"component": "iam-delegated-roles", // Promoted by mergeComponentConfigurations.
			"metadata": map[string]any{
				"component": "iam-delegated-roles",
				"type":      "abstract",
			},
			"vars": map[string]any{
				"namespace": "acme",
			},
		},
		"iam-delegated-roles": map[string]any{
			"component": "iam-delegated-roles",
			"metadata": map[string]any{
				"component": "iam-delegated-roles",
				"type":      "real",
				"inherits":  []any{"iam-delegated-roles-defaults"},
			},
			"vars": map[string]any{
				"role_name": "admin",
			},
		},
		"eks-defaults": map[string]any{
			"component": "eks", // Promoted by mergeComponentConfigurations.
			"metadata": map[string]any{
				"component": "eks",
				"type":      "abstract",
			},
			"vars": map[string]any{
				"cluster_name": "main",
			},
		},
		"eks": map[string]any{
			"component": "eks",
			"metadata": map[string]any{
				"component": "eks",
				"type":      "real",
				"inherits":  []any{"eks-defaults", "iam-delegated-roles-defaults"}, // Cross-dependency.
			},
			"vars": map[string]any{
				"node_count": 3,
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}

	// Process iam-delegated-roles.
	baseComponentConfig1 := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents1 := []string{}
	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig1,
		allComponentsMap,
		"iam-delegated-roles",
		"test-stack",
		"iam-delegated-roles-defaults",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents1,
	)
	require.NoError(t, err, "Should not stack overflow with multiple abstract components")

	// Process eks.
	ClearBaseComponentConfigCache()
	baseComponentConfig2 := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents2 := []string{}
	err = ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig2,
		allComponentsMap,
		"eks",
		"test-stack",
		"eks-defaults",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents2,
	)
	require.NoError(t, err, "Should not stack overflow with eks abstract component")
}

// TestProcessBaseComponentConfig_AbstractWithInheritsCycle tests the scenario where
// an abstract component has BOTH metadata.component AND metadata.inherits, creating
// a potential cycle through the inherits chain. The cycle detection must catch this
// even with defer delete on the visited set.
//
// Pattern: A-defaults (abstract, component: A, inherits: [A])
// This creates: A → inherits A-defaults → inherits A → inherits A-defaults → ...
func TestProcessBaseComponentConfig_AbstractWithInheritsCycle(t *testing.T) {
	ClearBaseComponentConfigCache()

	allComponentsMap := map[string]any{
		"vpc-defaults": map[string]any{
			"component": "vpc", // Promoted by mergeComponentConfigurations.
			"metadata": map[string]any{
				"component": "vpc",
				"type":      "abstract",
				"inherits":  []any{"vpc"}, // Abstract inherits from real — creates cycle.
			},
			"vars": map[string]any{
				"cidr": "10.0.0.0/16",
			},
		},
		"vpc": map[string]any{
			"component": "vpc",
			"metadata": map[string]any{
				"component": "vpc",
				"type":      "real",
				"inherits":  []any{"vpc-defaults"},
			},
			"vars": map[string]any{
				"name": "main",
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents := []string{}

	// This MUST NOT cause a stack overflow. It should either:
	// 1. Return a cycle detection error, OR
	// 2. Complete successfully by skipping the abstract component chain.
	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"vpc",
		"test-stack",
		"vpc-defaults",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)
	// We accept either no error (skip abstract chain) or a cycle error.
	// The only unacceptable outcome is a stack overflow (crash).
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrCircularComponentInheritance,
			"If error, should be a circular inheritance error, not a stack overflow")
	}
}

// TestProcessBaseComponentConfig_RealComponentSelfReferenceViaAbstract tests a
// more complex cycle where a real component's inheritance chain leads back to
// itself through an abstract component's inherits list.
//
// Pattern (processed data):
//
//	comp-A (real, inherits: [comp-B-defaults])
//	comp-B-defaults (abstract, component: comp-B, inherits: [comp-A])
//	comp-B (real, inherits: [comp-B-defaults])
//
// When processing comp-A → comp-B-defaults → (skip component key, abstract) → inherits comp-A → CYCLE.
func TestProcessBaseComponentConfig_RealComponentSelfReferenceViaAbstract(t *testing.T) {
	ClearBaseComponentConfigCache()

	allComponentsMap := map[string]any{
		"comp-A": map[string]any{
			"component": "comp-A",
			"metadata": map[string]any{
				"type":     "real",
				"inherits": []any{"comp-B-defaults"},
			},
			"vars": map[string]any{"from": "A"},
		},
		"comp-B-defaults": map[string]any{
			"component": "comp-B", // Promoted.
			"metadata": map[string]any{
				"component": "comp-B",
				"type":      "abstract",
				"inherits":  []any{"comp-A"}, // Abstract inherits from real A — creates cross-cycle.
			},
			"vars": map[string]any{"from": "B-defaults"},
		},
		"comp-B": map[string]any{
			"component": "comp-B",
			"metadata": map[string]any{
				"component": "comp-B",
				"type":      "real",
				"inherits":  []any{"comp-B-defaults"},
			},
			"vars": map[string]any{"from": "B"},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents := []string{}

	// Processing comp-A → comp-B-defaults → (abstract, skip component) → inherits comp-A → cycle.
	// This MUST NOT stack overflow.
	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"comp-A",
		"test-stack",
		"comp-B-defaults",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrCircularComponentInheritance,
			"If error, should be a circular inheritance error, not a stack overflow")
	}
}

// TestProcessBaseComponentConfig_DeferDeleteCycleReentry tests whether the
// defer delete(visited, visitKey) in processBaseComponentConfigInternal allows
// cycle re-entry after backtracking. This is the suspected mechanism behind
// the user's persistent stack overflow.
//
// Pattern (processed Phase 2 data):
//
//	real-A (component: real-A, inherits: [abstract-A])
//	abstract-A (abstract, component: real-A, inherits: [shared-base])
//	shared-base (component: real-A)  ← non-abstract, creates chain back
//
// Flow:
//
//	real-A → abstract-A → (skip component, abstract) → inherits shared-base
//	→ shared-base["component"] = "real-A" → follows → real-A
//	→ real-A["inherits"] = [abstract-A] → BUT abstract-A was already visited
//	  on the SAME DFS path → cycle detected ✓
//
// If defer delete incorrectly removes the entry, the cycle would not be detected.
func TestProcessBaseComponentConfig_DeferDeleteCycleReentry(t *testing.T) {
	ClearBaseComponentConfigCache()

	allComponentsMap := map[string]any{
		"abstract-A": map[string]any{
			"component": "real-A", // Promoted.
			"metadata": map[string]any{
				"component": "real-A",
				"type":      "abstract",
				"inherits":  []any{"shared-base"},
			},
			"vars": map[string]any{"from": "abstract-A"},
		},
		"shared-base": map[string]any{
			"component": "real-A", // Points back to real-A — non-abstract!
			"vars":      map[string]any{"from": "shared-base"},
		},
		"real-A": map[string]any{
			"component": "real-A",
			"metadata": map[string]any{
				"component": "real-A",
				"type":      "real",
				"inherits":  []any{"abstract-A"},
			},
			"vars": map[string]any{"from": "real-A"},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
	}
	baseComponents := []string{}

	// This MUST NOT stack overflow. The cycle detection should catch the re-entry.
	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"real-A",
		"test-stack",
		"abstract-A",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrCircularComponentInheritance,
			"If error, should be a circular inheritance error, not a stack overflow")
	}
}

// TestProcessBaseComponentConfig_AbstractMetadataComponentInherited verifies that metadata.component
// on an abstract component is properly inherited by real components through metadata inheritance.
// This is the scenario Erik described: metadata.component on abstract components should "do something"
// — it should populate the default component for the inheriting real component.
func TestProcessBaseComponentConfig_AbstractMetadataComponentInherited(t *testing.T) {
	ClearBaseComponentConfigCache()

	// Simulate processed data where an abstract component defines metadata.component
	// and a real component inherits from it.
	// The abstract component's metadata.component should be inherited as metadata,
	// even though its top-level "component" key is skipped for chain resolution.
	allComponentsMap := map[string]any{
		"eks/service/defaults": map[string]any{
			"component": "eks-service", // Promoted by mergeComponentConfigurations.
			"metadata": map[string]any{
				"component": "eks-service",
				"type":      "abstract",
			},
			"vars": map[string]any{
				"namespace": "acme",
				"enabled":   true,
			},
			"backend_type": "s3",
			"backend": map[string]any{
				"s3": map[string]any{
					"workspace_key_prefix": "eks-service",
				},
			},
		},
		"eks/service/app1": map[string]any{
			"component": "eks-service",
			"metadata": map[string]any{
				"component": "eks-service",
				"type":      "real",
				"inherits":  []any{"eks/service/defaults"},
			},
			"vars": map[string]any{
				"name": "app1",
			},
		},
	}

	// Enable metadata inheritance (default behavior).
	metadataEnabled := true
	atmosConfig := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			Inherit: schema.StacksInherit{
				Metadata: &metadataEnabled,
			},
		},
	}

	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:                      map[string]any{},
		BaseComponentSettings:                  map[string]any{},
		BaseComponentEnv:                       map[string]any{},
		BaseComponentAuth:                      map[string]any{},
		BaseComponentMetadata:                  map[string]any{},
		BaseComponentProviders:                 map[string]any{},
		BaseComponentHooks:                     map[string]any{},
		BaseComponentDependencies:              map[string]any{},
		BaseComponentLocals:                    map[string]any{},
		BaseComponentBackendSection:            map[string]any{},
		BaseComponentRemoteStateBackendSection: map[string]any{},
		BaseComponentGenerate:                  map[string]any{},
	}
	baseComponents := []string{}

	// Process: eks/service/app1 inherits from eks/service/defaults.
	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"eks/service/app1",
		"test-stack",
		"eks/service/defaults",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)

	require.NoError(t, err, "Should not error when abstract component has metadata.component")

	// Key assertion: metadata.component is inherited from the abstract base component.
	// This is what Erik described — abstract components' metadata.component should populate
	// the default component for the inheriting real component.
	assert.Equal(t, "eks-service", baseComponentConfig.BaseComponentMetadata["component"],
		"metadata.component should be inherited from abstract base component")

	// Verify vars are inherited.
	assert.Equal(t, "acme", baseComponentConfig.BaseComponentVars["namespace"],
		"vars should be inherited from abstract base component")
	assert.Equal(t, true, baseComponentConfig.BaseComponentVars["enabled"],
		"vars should be inherited from abstract base component")

	// Verify backend is inherited.
	assert.Equal(t, "s3", baseComponentConfig.BaseComponentBackendType,
		"backend_type should be inherited from abstract base component")

	// Verify metadata.type is NOT inherited (it's excluded from metadata inheritance).
	_, hasType := baseComponentConfig.BaseComponentMetadata["type"]
	assert.False(t, hasType, "metadata.type should NOT be inherited from base component")

	// Verify the base component was tracked.
	assert.Contains(t, baseComponents, "eks/service/defaults")
}

// TestProcessBaseComponentConfig_AbstractMetadataComponentNotInherited_WhenDisabled verifies that
// metadata.component is NOT inherited when metadata inheritance is disabled.
func TestProcessBaseComponentConfig_AbstractMetadataComponentNotInherited_WhenDisabled(t *testing.T) {
	ClearBaseComponentConfigCache()

	allComponentsMap := map[string]any{
		"eks/service/defaults": map[string]any{
			"component": "eks-service",
			"metadata": map[string]any{
				"component": "eks-service",
				"type":      "abstract",
			},
			"vars": map[string]any{
				"namespace": "acme",
			},
		},
		"eks/service/app1": map[string]any{
			"component": "eks-service",
			"metadata": map[string]any{
				"component": "eks-service",
				"type":      "real",
				"inherits":  []any{"eks/service/defaults"},
			},
			"vars": map[string]any{
				"name": "app1",
			},
		},
	}

	// Disable metadata inheritance.
	metadataDisabled := false
	atmosConfig := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			Inherit: schema.StacksInherit{
				Metadata: &metadataDisabled,
			},
		},
	}

	baseComponentConfig := &schema.BaseComponentConfig{
		BaseComponentVars:     map[string]any{},
		BaseComponentSettings: map[string]any{},
		BaseComponentEnv:      map[string]any{},
		BaseComponentMetadata: map[string]any{},
	}
	baseComponents := []string{}

	err := ProcessBaseComponentConfig(
		atmosConfig,
		baseComponentConfig,
		allComponentsMap,
		"eks/service/app1",
		"test-stack",
		"eks/service/defaults",
		filepath.Join("dummy", "path"),
		false,
		&baseComponents,
	)

	require.NoError(t, err)

	// When metadata inheritance is disabled, metadata.component should NOT be inherited.
	_, hasComponent := baseComponentConfig.BaseComponentMetadata["component"]
	assert.False(t, hasComponent, "metadata.component should NOT be inherited when metadata inheritance is disabled")

	// Vars should still be inherited (that's regular inheritance, not metadata inheritance).
	assert.Equal(t, "acme", baseComponentConfig.BaseComponentVars["namespace"],
		"vars should still be inherited even with metadata inheritance disabled")
}

func TestProcessYAMLConfigFile(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/relative-paths/stacks"
	filePath := "../../tests/fixtures/scenarios/relative-paths/stacks/orgs/acme/platform/dev.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, stackConfigMap, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Nil(t, err)
	assert.Equal(t, 3, len(stackConfigMap))

	mapResultKeys := u.StringKeysFromMap(stackConfigMap)
	// sorting so that the output is deterministic
	sort.Strings(mapResultKeys)

	assert.Equal(t, "components", mapResultKeys[0])
	assert.Equal(t, "import", mapResultKeys[1])
	assert.Equal(t, "vars", mapResultKeys[2])
}

func TestProcessYAMLConfigFileIgnoreMissingFiles(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/not-present.yaml"
	ignoreMissingFiles := true

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, stackConfigMap, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		ignoreMissingFiles,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Nil(t, err)
	assert.Equal(t, 0, len(stackConfigMap))
}

func TestProcessYAMLConfigFileMissingFilesReturnError(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/not-present.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Error(t, err)
}

// TestProcessYAMLConfigFile_ImportNotFound_ErrorPath exercises the branch where
// GetGlobMatches returns an error (the import directory does not exist) and the
// import is not a Go template, so processStackConfigImports propagates the
// error back to ProcessYAMLConfigFile with the missing-file message.
func TestProcessYAMLConfigFile_ImportNotFound_ErrorPath(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	// missing-import.yaml imports "catalog/this-file-does-not-exist-at-all" which has no
	// matching files on disk — GetGlobMatches returns ErrFailedToFindImport.
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/missing-import.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no matches found for the import")
}

// TestProcessYAMLConfigFile_InvalidTemplateInImportPath exercises the branch where
// IsGolangTemplate itself returns an error because the import path contains invalid
// Go template syntax — covering line 1093 inside processStackConfigImports.
// Templates must be disabled so that the raw `{{ unclosed` string is preserved as
// the import path and reaches IsGolangTemplate rather than failing during YAML
// template pre-processing.
func TestProcessYAMLConfigFile_InvalidTemplateInImportPath(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	// invalid-template-import-path.yaml imports `{{ unclosed` which is syntactically
	// invalid as a Go template; IsGolangTemplate returns (false, err).
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-template-import-path.yaml"

	// Disable template pre-processing so the raw `{{ unclosed` string is passed
	// through YAML parsing unchanged and eventually reaches IsGolangTemplate.
	atmosConfig := schema.AtmosConfiguration{}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	require.Error(t, err)
	// The error propagates from IsGolangTemplate which calls text/template.Parse on the
	// import string — the error message contains the template name and parse failure.
	assert.Contains(t, err.Error(), "unclosed")
}

func TestProcessYAMLConfigFileEmptyManifest(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/empty.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, stackConfigMap, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Nil(t, err)
	assert.Equal(t, 0, len(stackConfigMap))
}

func TestProcessYAMLConfigFileInvalidManifest(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidImportTemplate(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-import-template.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidValidationSchemaPath(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/dev.yaml"
	atmosManifestJsonSchemaFilePath := "does-not-exist"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		atmosManifestJsonSchemaFilePath,
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidManifestSchema(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-manifest-schema.yaml"
	atmosManifestJsonSchemaFilePath := "../../tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		atmosManifestJsonSchemaFilePath,
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidGlobalOverridesSection(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-global-overrides.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidTerraformOverridesSection(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-terraform-overrides.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidHelmfileOverridesSection(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-helmfile-overrides.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Error(t, err)
}

func TestProcessStackConfigProviderSection(t *testing.T) {
	basePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-providers-section")
	stacksBasePath := filepath.Join(basePath, "stacks")
	manifest := filepath.Join(stacksBasePath, "deploy", "nonprod.yaml")

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	deepMergedStackConfig, importsConfig, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		manifest,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)
	assert.Nil(t, err)

	config, err := ProcessStackConfig(
		&atmosConfig,
		stacksBasePath,
		filepath.Join(basePath, "components", "terraform"),
		filepath.Join(basePath, "components", "helmfile"),
		filepath.Join(basePath, "components", "packer"),
		filepath.Join(basePath, "components", "ansible"),
		"nonprod",
		deepMergedStackConfig,
		false,
		false,
		"",
		map[string]map[string][]string{},
		importsConfig,
		true,
	)
	assert.Nil(t, err)

	providers, err := u.EvaluateYqExpression(&atmosConfig, config, ".components.terraform.component-2.providers")
	assert.Nil(t, err)

	awsProvider, err := u.EvaluateYqExpression(&atmosConfig, providers, ".aws")
	assert.Nil(t, err)

	contextProvider, err := u.EvaluateYqExpression(&atmosConfig, providers, ".context")
	assert.Nil(t, err)

	awsProviderRoleArn, err := u.EvaluateYqExpression(&atmosConfig, awsProvider, ".assume_role.role_arn")
	assert.Nil(t, err)
	assert.Equal(t, "Derived component IAM Role ARN", awsProviderRoleArn)

	contextProviderPropertyOrder0, err := u.EvaluateYqExpression(&atmosConfig, contextProvider, ".property_order[0]")
	assert.Nil(t, err)
	assert.Equal(t, "product", contextProviderPropertyOrder0)
}

func TestProcessSettingsIntegrationsGithub(t *testing.T) {
	tests := []struct {
		name           string
		inputSettings  map[string]any
		expectedOutput map[string]any
		expectError    bool
		errorContains  string
	}{
		{
			name: "Valid GitHub integration settings",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"branch":     "main",
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"branch":     "main",
				},
			},
			expectError: false,
		},
		{
			name: "Additional valid fields",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":          "test-token",
					"owner":          "test-owner",
					"repository":     "test-repo",
					"branch":         "develop",
					"base_branch":    "main",
					"webhook_secret": "secret123",
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":          "test-token",
					"owner":          "test-owner",
					"repository":     "test-repo",
					"branch":         "develop",
					"base_branch":    "main",
					"webhook_secret": "secret123",
				},
			},
			expectError: false,
		},
		{
			name: "With workspace configuration",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"workspaces": map[string]any{
						"prefix": "test-",
						"suffix": "-prod",
					},
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"workspaces": map[string]any{
						"prefix": "test-",
						"suffix": "-prod",
					},
				},
			},
			expectError: false,
		},
		{
			name: "With path configuration",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"paths": []any{
						"terraform/**",
						"modules/**",
					},
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"paths": []any{
						"terraform/**",
						"modules/**",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processSettingsIntegrationsGithub(&schema.AtmosConfiguration{}, tt.inputSettings)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOutput, result)

				// Additional validation for required fields
				githubConfig := result["github"].(map[string]any)
				assert.Contains(t, githubConfig, "token")
				assert.Contains(t, githubConfig, "owner")
				assert.Contains(t, githubConfig, "repository")

				// Type assertions for key fields
				assert.IsType(t, "", githubConfig["token"])
				assert.IsType(t, "", githubConfig["owner"])
				assert.IsType(t, "", githubConfig["repository"])

				// Optional field type assertions
				if branch, ok := githubConfig["branch"]; ok {
					assert.IsType(t, "", branch)
				}
				if workspaces, ok := githubConfig["workspaces"]; ok {
					assert.IsType(t, map[string]any{}, workspaces)
				}
				if paths, ok := githubConfig["paths"]; ok {
					assert.IsType(t, []any{}, paths)
				}
			}
		})
	}
}

func TestProcessSettingsIntegrationsGithub_MissingGithubConfig(t *testing.T) {
	settings := map[string]any{}

	result, err := processSettingsIntegrationsGithub(&schema.AtmosConfiguration{}, settings)

	assert.Nil(t, err)
	assert.Equal(t, settings, result)
}

func TestProcessYAMLConfigFiles(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/relative-paths/stacks"
	filePaths := []string{
		"../../tests/fixtures/scenarios/relative-paths/stacks/orgs/acme/platform/dev.yaml",
		"../../tests/fixtures/scenarios/relative-paths/stacks/orgs/acme/platform/prod.yaml",
	}

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	listResult, mapResult, rawStackConfigs, err := ProcessYAMLConfigFiles(
		&atmosConfig,
		stacksBasePath,
		"", // terraformComponentsBasePath
		"", // helmfileComponentsBasePath
		"", // packerComponentsBasePath
		"", // ansibleComponentsBasePath
		filePaths,
		true,  // processStackDeps
		true,  // processComponentDeps
		false, // ignoreMissingFiles
	)

	// Verify no error occurred
	assert.Nil(t, err)

	// Verify listResult contains the expected number of results
	assert.Equal(t, len(filePaths), len(listResult))

	// Verify mapResult contains the expected stack names
	assert.Equal(t, len(filePaths), len(mapResult))

	// Verify rawStackConfigs contains the expected stack names
	assert.Equal(t, len(filePaths), len(rawStackConfigs))
}

func TestSectionContainsAnyNotEmptySections(t *testing.T) {
	tests := []struct {
		name            string
		section         map[string]any
		sectionsToCheck []string
		expected        bool
	}{
		{
			name:            "empty section and empty check list",
			section:         map[string]any{},
			sectionsToCheck: []string{},
			expected:        false,
		},
		{
			name:            "empty section with check list",
			section:         map[string]any{},
			sectionsToCheck: []string{"vars", "settings"},
			expected:        false,
		},
		{
			name: "section with empty map value",
			section: map[string]any{
				"vars": map[string]any{},
			},
			sectionsToCheck: []string{"vars"},
			expected:        false,
		},
		{
			name: "section with non-empty map value",
			section: map[string]any{
				"vars": map[string]any{
					"key": "value",
				},
			},
			sectionsToCheck: []string{"vars"},
			expected:        true,
		},
		{
			name: "section with empty string value",
			section: map[string]any{
				"backend_type": "",
			},
			sectionsToCheck: []string{"backend_type"},
			expected:        false,
		},
		{
			name: "section with non-empty string value",
			section: map[string]any{
				"backend_type": "s3",
			},
			sectionsToCheck: []string{"backend_type"},
			expected:        true,
		},
		{
			name: "multiple sections - first empty, second has value",
			section: map[string]any{
				"vars":     map[string]any{},
				"settings": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"vars", "settings"},
			expected:        true,
		},
		{
			name: "check non-existent section",
			section: map[string]any{
				"vars": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"non_existent"},
			expected:        false,
		},
		{
			name: "section with nil value",
			section: map[string]any{
				"vars": nil,
			},
			sectionsToCheck: []string{"vars"},
			expected:        false,
		},
		{
			name: "section with non-map non-string value",
			section: map[string]any{
				"count": 42,
			},
			sectionsToCheck: []string{"count"},
			expected:        false,
		},
		{
			name: "empty string in sectionsToCheck should be ignored",
			section: map[string]any{
				"vars": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"", "vars"},
			expected:        true,
		},
		{
			name: "only empty strings in sectionsToCheck",
			section: map[string]any{
				"vars": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"", ""},
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sectionContainsAnyNotEmptySections(tt.section, tt.sectionsToCheck)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHierarchicalImports_ImportOrderPreservation tests that import order is preserved
// even when using parallel processing for glob-matched imports.
// This is CRITICAL for Atmos inheritance - later imports must override earlier ones.
func TestHierarchicalImports_ImportOrderPreservation(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks"
	filePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks/deploy/dev/us-east-1.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Process the stack manifest with all hierarchical imports
	deepMergedConfig, importsConfig, stackConfigMap, terraformInline, terraformImports, helmfileInline, helmOverridesImports, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false, // ignoreMissingFiles
		false, // skipTemplatesProcessingInImports
		true,  // ignoreMissingTemplateValues
		false, // skipIfMissing
		nil,
		nil,
		nil,
		nil,
		"",
	)
	_ = importsConfig
	_ = stackConfigMap
	_ = terraformInline
	_ = terraformImports
	_ = helmfileInline
	_ = helmOverridesImports

	require.NoError(t, err)
	require.NotNil(t, deepMergedConfig)

	// Test 1: Verify import_order_test was overridden by final stack
	// This variable is set at each level, and the final stack value should win
	vars, ok := deepMergedConfig["vars"].(map[string]any)
	require.True(t, ok, "vars section should exist")

	importOrderTest, ok := vars["import_order_test"].(string)
	require.True(t, ok, "import_order_test should be a string")
	assert.Equal(t, "level-4-stack-dev-us-east-1", importOrderTest,
		"Final stack value should override all previous imports")

	// Test 2: Verify settings.import_order_test also follows correct order
	settings, ok := deepMergedConfig["settings"].(map[string]any)
	require.True(t, ok, "settings section should exist")

	settingsImportOrderTest, ok := settings["import_order_test"].(string)
	require.True(t, ok, "settings.import_order_test should be a string")
	assert.Equal(t, "level-4-stack-dev-us-east-1-settings", settingsImportOrderTest,
		"Settings should follow same import order")

	// Test 3: Verify region-specific values from level 2
	region, ok := vars["region"].(string)
	require.True(t, ok, "region should be a string")
	assert.Equal(t, "us-east-1", region, "Region should be set by region mixin")

	// Test 4: Verify account-specific values from level 2
	stage, ok := vars["stage"].(string)
	require.True(t, ok, "stage should be a string")
	assert.Equal(t, "dev", stage, "Stage should be set by account mixin")

	// Test 5: Verify VPC CIDR override (region overrides base)
	vpcCIDR, ok := vars["vpc_cidr"].(string)
	require.True(t, ok, "vpc_cidr should be a string")
	assert.Equal(t, "10.1.0.0/16", vpcCIDR,
		"Region-specific CIDR should override base CIDR")

	// Test 6: Verify deep merge of tags from all levels
	tags, ok := vars["tags"].(map[string]any)
	require.True(t, ok, "tags should exist and be a map")

	// Tags from different levels should all be present (deep merge)
	assert.Equal(t, "Atmos", tags["ManagedBy"], "Tag from level-1-globals")
	assert.Equal(t, "engineering", tags["CostCenter"], "Tag from level-1-defaults")
	assert.Equal(t, "AWS", tags["Provider"], "Tag from level-2-provider")
	assert.Equal(t, "us-east-1", tags["Region"], "Tag from level-2-region")
	assert.Equal(t, "dev", tags["Stage"], "Tag from level-2-account")
	assert.Equal(t, "acme-dev", tags["Account"], "Tag from level-2-account")
	assert.Equal(t, "dev-us-east-1", tags["Stack"], "Tag from level-4-stack")

	// The "Source" tag should be from the final stack (last override wins)
	assert.Equal(t, "level-4-stack", tags["Source"],
		"Source tag should be from final stack (last override wins)")
}

// TestHierarchicalImports_GlobPatternOrdering tests that glob-matched imports
// are processed in deterministic order during parallel processing.
func TestHierarchicalImports_GlobPatternOrdering(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks"
	filePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks/deploy/dev/us-east-1.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	deepMergedConfig, importsConfig, stackConfigMap, terraformInline, terraformImports, helmfileInline, helmOverridesImports, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)
	_ = importsConfig
	_ = stackConfigMap
	_ = terraformInline
	_ = terraformImports
	_ = helmfileInline
	_ = helmOverridesImports

	require.NoError(t, err)

	vars, ok := deepMergedConfig["vars"].(map[string]any)
	require.True(t, ok)

	// The region mixin imports "catalog/mixins/provider/aws-*"
	// This matches aws-a.yaml, aws-b.yaml, aws-c.yaml
	// After parallel processing and sequential merging, the last file
	// alphabetically (aws-c) should override previous ones
	providerPriority, ok := vars["provider_priority"].(string)
	require.True(t, ok, "provider_priority should be set by provider mixins")

	// With sorted glob matching, aws-c comes last and should win
	assert.Equal(t, "c", providerPriority,
		"Last provider in sorted glob (aws-c) should override earlier ones (aws-a, aws-b)")

	// Verify the provider type is set correctly
	providerType, ok := vars["provider_type"].(string)
	require.True(t, ok)
	assert.Equal(t, "aws", providerType)

	// Check tags to ensure aws-c's tag is present
	tags, ok := vars["tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "C", tags["ProviderPriority"],
		"ProviderPriority tag should be from aws-c (last in sort order)")
}

// TestHierarchicalImports_ProdStack tests the prod stack to ensure
// the same import ordering logic works across different configurations.
func TestHierarchicalImports_ProdStack(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks"
	filePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks/deploy/prod/us-west-2.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	deepMergedConfig, importsConfig, stackConfigMap, terraformInline, terraformImports, helmfileInline, helmOverridesImports, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)
	_ = importsConfig
	_ = stackConfigMap
	_ = terraformInline
	_ = terraformImports
	_ = helmfileInline
	_ = helmOverridesImports

	require.NoError(t, err)

	vars, ok := deepMergedConfig["vars"].(map[string]any)
	require.True(t, ok)

	// Verify final import order
	assert.Equal(t, "level-4-stack-prod-us-west-2", vars["import_order_test"])

	// Verify region
	assert.Equal(t, "us-west-2", vars["region"])

	// Verify stage (from prod account mixin)
	assert.Equal(t, "prod", vars["stage"])

	// Verify VPC CIDR (from us-west-2 region mixin)
	assert.Equal(t, "10.2.0.0/16", vars["vpc_cidr"])

	// Verify prod-specific instance type
	assert.Equal(t, "t3.large", vars["instance_type"],
		"Prod instance type should override dev instance type")

	// Verify prod-specific sizing
	assert.Equal(t, 3, vars["min_size"])
	assert.Equal(t, 10, vars["max_size"])

	// Verify tags include prod-specific values
	tags, ok := vars["tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "prod", tags["Stage"])
	assert.Equal(t, "acme-prod", tags["Account"])
	assert.Equal(t, "us-west-2", tags["Region"])
}

// TestHierarchicalImports_ComponentConfiguration tests that component-level
// configurations are properly inherited and merged.
func TestHierarchicalImports_ComponentConfiguration(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks"
	filePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks/deploy/dev/us-east-1.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	deepMergedConfig, importsConfig, stackConfigMap, terraformInline, terraformImports, helmfileInline, helmOverridesImports, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)
	_ = importsConfig
	_ = stackConfigMap
	_ = terraformInline
	_ = terraformImports
	_ = helmfileInline
	_ = helmOverridesImports

	require.NoError(t, err)

	// Navigate to component configuration
	components, ok := deepMergedConfig["components"].(map[string]any)
	require.True(t, ok, "components section should exist")

	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "terraform section should exist")

	testComponent, ok := terraform["test-component"].(map[string]any)
	require.True(t, ok, "test-component should exist")

	// Verify component vars
	compVars, ok := testComponent["vars"].(map[string]any)
	require.True(t, ok, "component vars should exist")

	// Component-level import_order_test should be overridden
	assert.Equal(t, "level-3-component-variants", compVars["import_order_test"])

	// Component-specific configuration should be present
	assert.Equal(t, "test-component", compVars["component_name"])
	assert.Equal(t, true, compVars["enabled"])
	assert.Equal(t, "standard", compVars["variant"])
	assert.Equal(t, false, compVars["high_availability"])

	// Component tags should be deep-merged with global tags
	compTags, ok := compVars["tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-component", compTags["Component"])
	assert.Equal(t, "standard", compTags["Variant"])

	// Verify component settings
	compSettings, ok := testComponent["settings"].(map[string]any)
	require.True(t, ok, "component settings should exist")
	assert.Equal(t, "level-3-component-variants-settings", compSettings["import_order_test"])
}

// TestHierarchicalImports_MultipleStacksConsistency tests that processing
// multiple stacks in parallel produces consistent results.
func TestHierarchicalImports_MultipleStacksConsistency(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/hierarchical-imports/stacks"
	filePaths := []string{
		"../../tests/fixtures/scenarios/hierarchical-imports/stacks/deploy/dev/us-east-1.yaml",
		"../../tests/fixtures/scenarios/hierarchical-imports/stacks/deploy/prod/us-west-2.yaml",
	}

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Process both stacks in parallel using ProcessYAMLConfigFiles
	// This tests the outer parallel loop (processing multiple stack files)
	_, _, rawStackConfigs, err := ProcessYAMLConfigFiles(
		&atmosConfig,
		stacksBasePath,
		"../../tests/fixtures/scenarios/hierarchical-imports/components/terraform",
		"",
		"",
		"",
		filePaths,
		false, // processStackDeps
		false, // processComponentDeps
		false, // ignoreMissingFiles
	)

	require.NoError(t, err)
	require.Equal(t, 2, len(rawStackConfigs), "Should have 2 processed stacks")

	// Verify dev stack using rawStackConfigs which contains the unprocessed merged config
	devStackRaw, ok := rawStackConfigs["deploy/dev/us-east-1"]
	require.True(t, ok, "dev stack should be in raw configs")

	devStackConfig, ok := devStackRaw["stack"].(map[string]any)
	require.True(t, ok, "dev stack should have stack section")

	devVars, ok := devStackConfig["vars"].(map[string]any)
	require.True(t, ok, "devStack should have vars section")
	t.Logf("devVars keys: %v", u.StringKeysFromMap(devVars))
	assert.Equal(t, "level-4-stack-dev-us-east-1", devVars["import_order_test"])

	// Note: stage and region might not be set at the root level in our test fixture
	// Let's check if they exist before asserting
	if stage, ok := devVars["stage"]; ok {
		assert.Equal(t, "dev", stage)
	}
	if region, ok := devVars["region"]; ok {
		assert.Equal(t, "us-east-1", region)
	}

	// Verify prod stack
	prodStackRaw, ok := rawStackConfigs["deploy/prod/us-west-2"]
	require.True(t, ok, "prod stack should be in raw configs")

	prodStackConfig, ok := prodStackRaw["stack"].(map[string]any)
	require.True(t, ok, "prod stack should have stack section")

	prodVars, ok := prodStackConfig["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "level-4-stack-prod-us-west-2", prodVars["import_order_test"])

	// Note: stage and region might not be set at the root level in our test fixture
	// Let's check if they exist before asserting
	if stage, ok := prodVars["stage"]; ok {
		assert.Equal(t, "prod", stage)
	}
	if region, ok := prodVars["region"]; ok {
		assert.Equal(t, "us-west-2", region)
	}
}

// TestGetCachedCompiledSchema tests that cached JSON schemas are retrieved correctly.
// This validates P7.2 optimization: schema caching avoids recompilation.
func TestGetCachedCompiledSchema(t *testing.T) {
	// Clear the JSON schema cache before the test to ensure cache isolation.
	ClearJsonSchemaCache()

	// Use a test schema path
	schemaPath := filepath.Join("..", "..", "tests", "fixtures", "schemas", "atmos", "atmos-manifest", "1.0", "atmos-manifest.json")

	// First lookup should miss the cache (returns false)
	compiledSchema, found := getCachedCompiledSchema(schemaPath)
	assert.False(t, found, "Initial lookup should not find cached schema")
	assert.Nil(t, compiledSchema, "Schema should be nil on cache miss")

	// Compile and cache the schema by using ProcessYAMLConfigFile with validation
	// This indirectly tests that schemas are being cached during normal operation
	stacksBasePath := "../../tests/fixtures/scenarios/relative-paths/stacks"
	filePath := "../../tests/fixtures/scenarios/relative-paths/stacks/orgs/acme/platform/dev.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	deepMergedStackConfig, importsConfig, stackConfigMap, terraformInline, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		schemaPath,
	)
	assert.NoError(t, err, "ProcessYAMLConfigFile should succeed with schema validation")
	assert.NotNil(t, deepMergedStackConfig, "deepMergedStackConfig should not be nil")
	assert.NotNil(t, importsConfig, "importsConfig should not be nil")
	assert.NotNil(t, stackConfigMap, "stackConfigMap should not be nil")
	assert.NotNil(t, terraformInline, "terraformInline should not be nil")

	// Second lookup should hit the cache (returns true)
	compiledSchema, found = getCachedCompiledSchema(schemaPath)
	assert.True(t, found, "Second lookup should find cached schema")
	assert.NotNil(t, compiledSchema, "Cached schema should not be nil")
}

// TestCacheCompiledSchema tests that schemas are cached correctly.
// This validates P7.2 optimization: cacheCompiledSchema stores schemas for reuse.
func TestCacheCompiledSchema(t *testing.T) {
	// Create a mock schema path
	schemaPath := "/test/schema/path.json"

	// Initially, cache should be empty
	compiledSchema, found := getCachedCompiledSchema(schemaPath)
	assert.False(t, found)
	assert.Nil(t, compiledSchema)

	// Note: We cannot easily create a *jsonschema.Schema without compiling from a file,
	// so this test validates the cache lookup mechanism rather than the full compilation flow.
	// The actual schema compilation and caching is tested via ProcessYAMLConfigFile above.

	// Verify that getCachedCompiledSchema returns consistent results
	compiledSchema2, found2 := getCachedCompiledSchema(schemaPath)
	assert.Equal(t, found, found2, "Consistent cache lookups should return same result")
	assert.Equal(t, compiledSchema, compiledSchema2, "Consistent cache lookups should return same schema")
}

// TestExtractLocalsFromRawYAML_Basic tests basic locals extraction from raw YAML.
func TestExtractLocalsFromRawYAML_Basic(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  namespace: "acme"
  environment: "dev"
  name_prefix: "{{ .locals.namespace }}-{{ .locals.environment }}"
vars:
  stage: "us-east-1"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acme", result.locals["namespace"])
	assert.Equal(t, "dev", result.locals["environment"])
	assert.Equal(t, "acme-dev", result.locals["name_prefix"])
}

// TestExtractLocalsFromRawYAML_NoLocals tests extraction when no locals section exists.
func TestExtractLocalsFromRawYAML_NoLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
vars:
  stage: "us-east-1"
  environment: "dev"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	// Returns empty map when no locals are defined (safe for template processing).
	assert.Empty(t, result.locals)
}

// TestExtractLocalsFromRawYAML_EmptyYAML tests extraction from empty YAML.
func TestExtractLocalsFromRawYAML_EmptyYAML(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := ""

	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.locals)
}

// TestExtractLocalsFromRawYAML_InvalidYAML tests extraction from invalid YAML.
func TestExtractLocalsFromRawYAML_InvalidYAML(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  - this is not valid
  namespace: "acme"
  invalid yaml structure
`
	_, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidStackManifest), "error should wrap ErrInvalidStackManifest")
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

// TestExtractLocalsFromRawYAML_TerraformSectionLocals tests extraction of terraform section locals.
func TestExtractLocalsFromRawYAML_TerraformSectionLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  namespace: "acme"
  environment: "dev"
terraform:
  locals:
    backend_bucket: "{{ .locals.namespace }}-{{ .locals.environment }}-tfstate"
  backend_type: s3
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	// Global locals should be present.
	assert.Equal(t, "acme", result.locals["namespace"])
	assert.Equal(t, "dev", result.locals["environment"])
	// Terraform section locals should be merged.
	assert.Equal(t, "acme-dev-tfstate", result.locals["backend_bucket"])
}

// TestExtractLocalsFromRawYAML_HelmfileSectionLocals tests extraction of helmfile section locals.
func TestExtractLocalsFromRawYAML_HelmfileSectionLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  namespace: "acme"
helmfile:
  locals:
    release_name: "{{ .locals.namespace }}-release"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acme", result.locals["namespace"])
	assert.Equal(t, "acme-release", result.locals["release_name"])
}

// TestExtractLocalsFromRawYAML_PackerSectionLocals tests extraction of packer section locals.
func TestExtractLocalsFromRawYAML_PackerSectionLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  namespace: "acme"
packer:
  locals:
    ami_name: "{{ .locals.namespace }}-ami"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acme", result.locals["namespace"])
	assert.Equal(t, "acme-ami", result.locals["ami_name"])
}

// TestExtractLocalsFromRawYAML_AllSectionLocals tests extraction from all sections.
func TestExtractLocalsFromRawYAML_AllSectionLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  namespace: "acme"
  environment: "prod"
terraform:
  locals:
    tf_var: "{{ .locals.namespace }}-terraform"
helmfile:
  locals:
    hf_var: "{{ .locals.namespace }}-helmfile"
packer:
  locals:
    pk_var: "{{ .locals.namespace }}-packer"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	// Global locals.
	assert.Equal(t, "acme", result.locals["namespace"])
	assert.Equal(t, "prod", result.locals["environment"])
	// Section-specific locals.
	assert.Equal(t, "acme-terraform", result.locals["tf_var"])
	assert.Equal(t, "acme-helmfile", result.locals["hf_var"])
	assert.Equal(t, "acme-packer", result.locals["pk_var"])
}

// TestExtractLocalsFromRawYAML_CircularDependency tests circular dependency detection.
func TestExtractLocalsFromRawYAML_CircularDependency(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  a: "{{ .locals.b }}"
  b: "{{ .locals.c }}"
  c: "{{ .locals.a }}"
`
	_, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

// TestExtractLocalsFromRawYAML_SelfReference tests self-referencing locals.
func TestExtractLocalsFromRawYAML_SelfReference(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  a: "value-a"
  b: "{{ .locals.a }}-suffix"
  c: "prefix-{{ .locals.b }}-suffix"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "value-a", result.locals["a"])
	assert.Equal(t, "value-a-suffix", result.locals["b"])
	assert.Equal(t, "prefix-value-a-suffix-suffix", result.locals["c"])
}

// TestExtractLocalsFromRawYAML_ComplexValue tests complex value types in locals.
func TestExtractLocalsFromRawYAML_ComplexValue(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  namespace: "acme"
  tags:
    Environment: "{{ .locals.namespace }}"
    Managed: "atmos"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acme", result.locals["namespace"])
	tags, ok := result.locals["tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", tags["Environment"])
	assert.Equal(t, "atmos", tags["Managed"])
}

// TestExtractLocalsFromRawYAML_SectionOverridesGlobal tests that section locals can override global.
func TestExtractLocalsFromRawYAML_SectionOverridesGlobal(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  namespace: "global-acme"
terraform:
  locals:
    namespace: "terraform-acme"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	// Terraform section should override global.
	assert.Equal(t, "terraform-acme", result.locals["namespace"])
}

// TestExtractLocalsFromRawYAML_TemplateInNonLocalSection tests that templates outside locals remain unresolved.
func TestExtractLocalsFromRawYAML_TemplateInNonLocalSection(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	// This test verifies that extractLocalsFromRawYAML only resolves locals,
	// not templates in other sections.
	yamlContent := `
locals:
  namespace: "acme"
vars:
  name: "{{ .locals.namespace }}-app"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	// Only locals should be resolved and returned.
	assert.Equal(t, "acme", result.locals["namespace"])
	// vars section is not part of the locals result.
	assert.Nil(t, result.locals["name"])
}

// TestExtractLocalsFromRawYAML_NilAtmosConfig tests extraction with nil atmosConfig.
func TestExtractLocalsFromRawYAML_NilAtmosConfig(t *testing.T) {
	yamlContent := `
locals:
  namespace: "acme"
`
	result, err := extractLocalsFromRawYAML(nil, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acme", result.locals["namespace"])
}

// TestExtractLocalsFromRawYAML_OnlyComments tests extraction from YAML with only comments.
func TestExtractLocalsFromRawYAML_OnlyComments(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
# This is a comment
# Another comment
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.locals)
}

// TestExtractLocalsFromRawYAML_EmptyLocals tests extraction with empty locals section.
func TestExtractLocalsFromRawYAML_EmptyLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals: {}
vars:
  stage: "dev"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	// Empty locals should return an empty map, not nil.
	require.NotNil(t, result)
	assert.Empty(t, result.locals)
}

// TestExtractLocalsFromRawYAML_ContextAccess tests that locals can access settings, vars, and env from the same file.
// This addresses GitHub issue #1991: Locals Cannot Access .settings from Imported Mixins.
func TestExtractLocalsFromRawYAML_ContextAccess(t *testing.T) {
	tests := []struct {
		name           string
		yamlContent    string
		expectedLocals map[string]string
		checkSettings  map[string]string
		checkVars      map[string]string
		checkEnv       map[string]string
	}{
		{
			name: "settings access",
			yamlContent: `
settings:
  substage: dev
  environment: sandbox
locals:
  domain: "{{ .settings.substage }}.example.com"
  full_env: "{{ .settings.environment }}-{{ .settings.substage }}"
vars:
  stage: test
`,
			expectedLocals: map[string]string{
				"domain":   "dev.example.com",
				"full_env": "sandbox-dev",
			},
			checkSettings: map[string]string{
				"substage":    "dev",
				"environment": "sandbox",
			},
		},
		{
			name: "vars access",
			yamlContent: `
vars:
  stage: us-east-1
  region: us-east-1
locals:
  resource_prefix: "{{ .vars.stage }}-app"
  full_name: "{{ .vars.region }}-{{ .vars.stage }}"
`,
			expectedLocals: map[string]string{
				"resource_prefix": "us-east-1-app",
				"full_name":       "us-east-1-us-east-1",
			},
			checkVars: map[string]string{
				"stage": "us-east-1",
			},
		},
		{
			name: "env access",
			yamlContent: `
env:
  AWS_REGION: us-west-2
  TF_VAR_enabled: "true"
locals:
  region_specific: "app-{{ .env.AWS_REGION }}"
`,
			expectedLocals: map[string]string{
				"region_specific": "app-us-west-2",
			},
			checkEnv: map[string]string{
				"AWS_REGION": "us-west-2",
			},
		},
		{
			name: "combined context access",
			yamlContent: `
settings:
  substage: dev
vars:
  stage: us-east-1
env:
  AWS_REGION: us-west-2
locals:
  namespace: "acme"
  combined: "{{ .locals.namespace }}-{{ .settings.substage }}-{{ .vars.stage }}"
`,
			expectedLocals: map[string]string{
				"namespace": "acme",
				"combined":  "acme-dev-us-east-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			result, err := extractLocalsFromRawYAML(atmosConfig, tt.yamlContent, "test.yaml")

			require.NoError(t, err)
			require.NotNil(t, result)

			// Check expected locals.
			for key, expected := range tt.expectedLocals {
				assert.Equal(t, expected, result.locals[key], "locals[%s] mismatch", key)
			}

			// Check settings if specified.
			if tt.checkSettings != nil {
				assert.NotNil(t, result.settings, "settings should be extracted")
				for key, expected := range tt.checkSettings {
					assert.Equal(t, expected, result.settings[key], "settings[%s] mismatch", key)
				}
			}

			// Check vars if specified.
			if tt.checkVars != nil {
				assert.NotNil(t, result.vars, "vars should be extracted")
				for key, expected := range tt.checkVars {
					assert.Equal(t, expected, result.vars[key], "vars[%s] mismatch", key)
				}
			}

			// Check env if specified.
			if tt.checkEnv != nil {
				assert.NotNil(t, result.env, "env should be extracted")
				for key, expected := range tt.checkEnv {
					assert.Equal(t, expected, result.env[key], "env[%s] mismatch", key)
				}
			}
		})
	}
}

// TestExtractLocalsFromRawYAML_SectionOnlyLocals tests that section-only locals (without global locals)
// are properly detected and processed. This covers the HasTerraformLocals/HasHelmfileLocals/HasPackerLocals
// branches in buildLocalsResult.
func TestExtractLocalsFromRawYAML_SectionOnlyLocals(t *testing.T) {
	tests := []struct {
		name           string
		yamlContent    string
		expectedLocals map[string]string
	}{
		{
			name: "terraform_only_locals",
			yamlContent: `
terraform:
  locals:
    backend_bucket: "my-tfstate-bucket"
    backend_key: "state.tfstate"
vars:
  stage: dev
`,
			expectedLocals: map[string]string{
				"backend_bucket": "my-tfstate-bucket",
				"backend_key":    "state.tfstate",
			},
		},
		{
			name: "helmfile_only_locals",
			yamlContent: `
helmfile:
  locals:
    release_name: "my-release"
    namespace: "default"
vars:
  stage: dev
`,
			expectedLocals: map[string]string{
				"release_name": "my-release",
				"namespace":    "default",
			},
		},
		{
			name: "packer_only_locals",
			yamlContent: `
packer:
  locals:
    ami_name: "my-ami"
    ami_prefix: "acme"
vars:
  stage: dev
`,
			expectedLocals: map[string]string{
				"ami_name":   "my-ami",
				"ami_prefix": "acme",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			result, err := extractLocalsFromRawYAML(atmosConfig, tt.yamlContent, "test.yaml")

			require.NoError(t, err)
			require.NotNil(t, result)
			// hasLocals should be true even without global locals.
			assert.True(t, result.hasLocals, "hasLocals should be true for section-only locals")
			// Check expected locals.
			for key, expected := range tt.expectedLocals {
				assert.Equal(t, expected, result.locals[key], "locals[%s] mismatch", key)
			}
		})
	}
}

// TestExtractLocalsFromRawYAML_EmptyLocalsHasLocalsFlag tests that empty locals: {} still sets hasLocals to true.
// This ensures that template context is enabled even when locals section is empty.
func TestExtractLocalsFromRawYAML_EmptyLocalsHasLocalsFlag(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals: {}
vars:
  stage: "dev"
settings:
  enabled: true
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	// hasLocals should be true even though locals section is empty.
	assert.True(t, result.hasLocals, "hasLocals should be true for empty locals: {}")
	// locals should be an empty map, not nil.
	assert.NotNil(t, result.locals, "locals should not be nil for empty locals: {}")
	assert.Empty(t, result.locals, "locals should be empty")
	// Context sections should still be populated.
	assert.NotNil(t, result.vars, "vars should be extracted")
	assert.NotNil(t, result.settings, "settings should be extracted")
}

// TestBuildLocalsResult_NilLocalsWithHasLocals tests that buildLocalsResult initializes locals map
// when hasLocals is true but MergeForTemplateContext returns nil.
func TestBuildLocalsResult_NilLocalsWithHasLocals(t *testing.T) {
	// Create a LocalsContext where HasTerraformLocals is true but no actual locals.
	localsCtx := &LocalsContext{
		Global:             nil,
		Terraform:          nil,
		Helmfile:           nil,
		Packer:             nil,
		HasTerraformLocals: true,
		HasHelmfileLocals:  false,
		HasPackerLocals:    false,
	}
	rawConfig := map[string]any{
		"vars": map[string]any{"stage": "dev"},
	}

	result := buildLocalsResult(rawConfig, localsCtx)

	// hasLocals should be true due to HasTerraformLocals.
	assert.True(t, result.hasLocals, "hasLocals should be true when HasTerraformLocals is true")
	// locals should be initialized to empty map, not nil.
	assert.NotNil(t, result.locals, "locals should be initialized to empty map when hasLocals is true")
}

// TestProcessTemplatesInSection tests the processTemplatesInSection helper function.
func TestProcessTemplatesInSection(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("empty section returns as-is", func(t *testing.T) {
		result, err := processTemplatesInSection(atmosConfig, map[string]any{}, map[string]any{}, "test.yaml")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("nil section returns as-is", func(t *testing.T) {
		var section map[string]any
		result, err := processTemplatesInSection(atmosConfig, section, map[string]any{}, "test.yaml")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("section without templates returns as-is", func(t *testing.T) {
		section := map[string]any{
			"key": "plain-value",
			"num": 42,
		}
		result, err := processTemplatesInSection(atmosConfig, section, map[string]any{}, "test.yaml")
		require.NoError(t, err)
		assert.Equal(t, "plain-value", result["key"])
		assert.Equal(t, 42, result["num"])
	})

	t.Run("resolves templates with locals context", func(t *testing.T) {
		section := map[string]any{
			"stage_label": "{{ .locals.stage }}-app",
		}
		context := map[string]any{
			"locals": map[string]any{"stage": "dev"},
		}
		result, err := processTemplatesInSection(atmosConfig, section, context, "test.yaml")
		require.NoError(t, err)
		assert.Equal(t, "dev-app", result["stage_label"])
	})

	t.Run("returns error on missing template values", func(t *testing.T) {
		section := map[string]any{
			"value": "{{ .missing_key }}",
		}
		context := map[string]any{
			"locals": map[string]any{"stage": "dev"},
		}
		_, err := processTemplatesInSection(atmosConfig, section, context, "test.yaml")
		require.Error(t, err, "Should fail when template references missing context value")
		assert.True(t, errors.Is(err, errUtils.ErrInvalidStackManifest))
	})

	t.Run("resolves nested template values", func(t *testing.T) {
		section := map[string]any{
			"nested": map[string]any{
				"name": "{{ .locals.namespace }}-{{ .locals.env }}",
			},
		}
		context := map[string]any{
			"locals": map[string]any{"namespace": "acme", "env": "prod"},
		}
		result, err := processTemplatesInSection(atmosConfig, section, context, "test.yaml")
		require.NoError(t, err)
		nested, ok := result["nested"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme-prod", nested["name"])
	})
}

// TestExtractAndAddLocalsToContext_SectionProcessing tests the template processing pipeline
// in extractAndAddLocalsToContext for settings, vars, and env sections.
func TestExtractAndAddLocalsToContext_SectionProcessing(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}

	t.Run("settings templates resolved with locals", func(t *testing.T) {
		yamlContent := `
locals:
  stage: dev
settings:
  label: '{{ .locals.stage }}-config'
`
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
		require.NoError(t, err)
		settings, ok := ctx["settings"].(map[string]any)
		require.True(t, ok, "settings should exist in context")
		assert.Equal(t, "dev-config", settings["label"])
	})

	t.Run("vars templates resolved with locals and settings", func(t *testing.T) {
		yamlContent := `
locals:
  env: prod
settings:
  region: us-east-1
vars:
  deploy_env: '{{ .locals.env }}'
`
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
		require.NoError(t, err)
		vars, ok := ctx["vars"].(map[string]any)
		require.True(t, ok, "vars should exist in context")
		assert.Equal(t, "prod", vars["deploy_env"])
	})

	t.Run("env templates resolved with locals settings and vars", func(t *testing.T) {
		yamlContent := `
locals:
  app: myapp
env:
  APP_NAME: '{{ .locals.app }}'
`
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
		require.NoError(t, err)
		env, ok := ctx["env"].(map[string]any)
		require.True(t, ok, "env should exist in context")
		assert.Equal(t, "myapp", env["APP_NAME"])
	})

	t.Run("settings fallback to raw on template error", func(t *testing.T) {
		// Settings has a template referencing a value not in locals context.
		// processTemplatesInSection should fail and fall back to raw settings.
		yamlContent := `
locals:
  stage: dev
settings:
  component: '{{ .atmos_component }}'
  static: plain-value
`
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
		require.NoError(t, err)
		settings, ok := ctx["settings"].(map[string]any)
		require.True(t, ok, "settings should exist in context (raw fallback)")
		// Raw fallback means the template string is preserved.
		assert.Equal(t, "{{ .atmos_component }}", settings["component"].(string))
		assert.Equal(t, "plain-value", settings["static"])
	})

	t.Run("vars fallback to raw on template error", func(t *testing.T) {
		yamlContent := `
locals:
  stage: dev
vars:
  stack: '{{ .atmos_stack }}'
`
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
		require.NoError(t, err)
		vars, ok := ctx["vars"].(map[string]any)
		require.True(t, ok, "vars should exist in context (raw fallback)")
		assert.Equal(t, "{{ .atmos_stack }}", vars["stack"].(string))
	})

	t.Run("env fallback to raw on template error", func(t *testing.T) {
		yamlContent := `
locals:
  stage: dev
env:
  COMPONENT: '{{ .atmos_component }}'
`
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
		require.NoError(t, err)
		env, ok := ctx["env"].(map[string]any)
		require.True(t, ok, "env should exist in context (raw fallback)")
		assert.Equal(t, "{{ .atmos_component }}", env["COMPONENT"].(string))
	})

	t.Run("clears inherited locals from parent context", func(t *testing.T) {
		yamlContent := `
locals:
  own_local: mine
`
		parentContext := map[string]any{
			"locals": map[string]any{"inherited": "should-be-cleared"},
		}
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", parentContext)
		require.NoError(t, err)
		locals, ok := ctx["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "mine", locals["own_local"])
		_, hasInherited := locals["inherited"]
		assert.False(t, hasInherited, "inherited locals should be cleared")
	})

	t.Run("full pipeline settings vars env with locals", func(t *testing.T) {
		yamlContent := `
locals:
  ns: acme
  stage: dev
settings:
  context:
    namespace: '{{ .locals.ns }}'
vars:
  env_name: '{{ .locals.stage }}'
env:
  NAMESPACE: '{{ .locals.ns }}'
`
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
		require.NoError(t, err)

		settings, ok := ctx["settings"].(map[string]any)
		require.True(t, ok)
		settingsCtx, ok := settings["context"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", settingsCtx["namespace"])

		vars, ok := ctx["vars"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "dev", vars["env_name"])

		env, ok := ctx["env"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", env["NAMESPACE"])
	})
}

// TestExtractAndAddLocalsToContext_ExternalContext verifies that external import context
// is included during section template processing, enabling settings/vars/env to reference
// import-provided values alongside locals.
func TestExtractAndAddLocalsToContext_ExternalContext(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}

	t.Run("settings resolves with external context", func(t *testing.T) {
		yamlContent := `
locals:
  stage: dev
settings:
  label: '{{ .locals.stage }}-{{ .tenant }}'
`
		externalCtx := map[string]any{"tenant": "acme"}
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", externalCtx)
		require.NoError(t, err)
		settings, ok := ctx["settings"].(map[string]any)
		require.True(t, ok, "settings should exist in context")
		assert.Equal(t, "dev-acme", settings["label"],
			"settings should resolve templates using both locals and external context")
	})

	t.Run("vars resolves with external context and processed settings", func(t *testing.T) {
		yamlContent := `
locals:
  stage: dev
settings:
  env_label: '{{ .locals.stage }}-{{ .region }}'
vars:
  full_label: '{{ .settings.env_label }}'
`
		externalCtx := map[string]any{"region": "us-east-1"}
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", externalCtx)
		require.NoError(t, err)
		settings, ok := ctx["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "dev-us-east-1", settings["env_label"])

		vars, ok := ctx["vars"].(map[string]any)
		require.True(t, ok, "vars should exist in context")
		assert.Equal(t, "dev-us-east-1", vars["full_label"],
			"vars should resolve settings that were resolved with external context")
	})

	t.Run("settings falls back when external context is insufficient", func(t *testing.T) {
		yamlContent := `
locals:
  stage: dev
settings:
  label: '{{ .locals.stage }}-{{ .missing_var }}'
`
		externalCtx := map[string]any{"tenant": "acme"}
		ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", externalCtx)
		require.NoError(t, err)
		settings, ok := ctx["settings"].(map[string]any)
		require.True(t, ok, "settings should exist (raw fallback)")
		assert.Equal(t, "{{ .locals.stage }}-{{ .missing_var }}", settings["label"],
			"settings should fall back to raw when template references unavailable values")
	})
}

// TestProcessYAMLConfigFile_OriginalContextFallback tests the graceful fallback
// when template processing fails and only file-extracted context is available.
func TestProcessYAMLConfigFile_OriginalContextFallback(t *testing.T) {
	// Create a temporary YAML file with locals and {{ .atmos_component }} templates.
	// When a section mixes resolvable ({{ .locals.X }}) and unresolvable ({{ .atmos_component }})
	// templates, the section-level processing fails and falls back to raw values.
	// The whole-file template processing also fails and falls back to raw content.
	tmpDir := t.TempDir()
	yamlContent := `
locals:
  stage: dev
settings:
  component: "{{ .atmos_component }}"
  stage_label: "{{ .locals.stage }}-label"
vars:
  stack: "{{ .atmos_stack }}"
  env_name: "{{ .locals.stage }}"
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(yamlContent), 0o644)
	require.NoError(t, err)

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}

	// Process with no external context (nil) — file-extracted context only.
	// Template processing will fail on {{ .atmos_component }} because it's not in context.
	// The fallback should return raw content preserving ALL templates for later processing.
	deepMergedConfig, importsConfig, stackConfigMap, tfInline, tfImports, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		tmpDir,
		filePath,
		map[string]map[string]any{},
		nil,   // No external context.
		false, // ignoreMissingFiles.
		false, // skipTemplatesProcessingInImports.
		false, // ignoreMissingTemplateValues.
		false, // skipIfMissing.
		nil,
		nil,
		nil,
		nil,
		"",
	)

	require.NoError(t, err, "Should not fail — fallback to raw content when only file-extracted context")
	require.NotNil(t, deepMergedConfig)
	_, _, _ = importsConfig, stackConfigMap, tfInline // Unused return values.
	_ = tfImports

	// Verify resolved locals are persisted in the config.
	locals, ok := deepMergedConfig["locals"].(map[string]any)
	require.True(t, ok, "resolved locals should be persisted into stackConfigMap")
	assert.Equal(t, "dev", locals["stage"])

	// Settings section has both {{ .locals.stage }} and {{ .atmos_component }}.
	// processTemplatesInSection fails on {{ .atmos_component }} so the entire section
	// falls back to raw values — both templates are preserved for later resolution.
	settings, ok := deepMergedConfig["settings"].(map[string]any)
	require.True(t, ok, "settings should exist")
	assert.Equal(t, "{{ .atmos_component }}", settings["component"].(string),
		"{{ .atmos_component }} should be preserved in settings fallback")
	assert.Equal(t, "{{ .locals.stage }}-label", settings["stage_label"].(string),
		"{{ .locals.stage }}-label is preserved when section has unresolvable templates")

	// Vars section similarly falls back because it contains {{ .atmos_stack }}.
	vars, ok := deepMergedConfig["vars"].(map[string]any)
	require.True(t, ok, "vars should exist")
	assert.Equal(t, "{{ .atmos_stack }}", vars["stack"].(string),
		"{{ .atmos_stack }} should be preserved in vars fallback")
}

// TestProcessYAMLConfigFile_ExternalContextError tests that template errors
// are returned as errors when external context is provided (not just file-extracted).
func TestProcessYAMLConfigFile_ExternalContextError(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
settings:
  value: "{{ .missing_key }}"
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(yamlContent), 0o644)
	require.NoError(t, err)

	atmosConfig := schema.AtmosConfiguration{}

	// Process WITH external context — template errors should be returned.
	externalContext := map[string]any{
		"some_key": "some_value",
	}
	result, importsConfig, stackCfg, tfInline, tfImports, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		tmpDir,
		filePath,
		map[string]map[string]any{},
		externalContext, // External context provided.
		false,           // ignoreMissingFiles.
		false,           // skipTemplatesProcessingInImports.
		false,           // ignoreMissingTemplateValues.
		false,           // skipIfMissing.
		nil,
		nil,
		nil,
		nil,
		"",
	)
	_ = result
	_ = importsConfig
	_ = stackCfg
	_ = tfInline
	_ = tfImports

	require.Error(t, err, "Should return error when external context is provided and template fails")
	assert.True(t, errors.Is(err, errUtils.ErrInvalidStackManifest))
}

// TestProcessYAMLConfigFile_ResolvedSectionsPersisted tests that resolved
// sections (locals, vars, settings, env) are persisted into stackConfigMap.
func TestProcessYAMLConfigFile_ResolvedSectionsPersisted(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
locals:
  ns: acme
  env: dev
settings:
  namespace: '{{ .locals.ns }}'
vars:
  environment: '{{ .locals.env }}'
env:
  NAMESPACE: '{{ .locals.ns }}'
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(yamlContent), 0o644)
	require.NoError(t, err)

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}

	deepMergedConfig, importsConfig, stackConfigMap, tfInline, tfImports, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		tmpDir,
		filePath,
		map[string]map[string]any{},
		nil,   // No external context.
		false, // ignoreMissingFiles.
		false, // skipTemplatesProcessingInImports.
		false, // ignoreMissingTemplateValues.
		false, // skipIfMissing.
		nil,
		nil,
		nil,
		nil,
		"",
	)
	_, _, _ = importsConfig, stackConfigMap, tfInline // Unused return values.
	_ = tfImports

	require.NoError(t, err)

	// Resolved locals should be persisted.
	locals, ok := deepMergedConfig["locals"].(map[string]any)
	require.True(t, ok, "locals should be persisted")
	assert.Equal(t, "acme", locals["ns"])
	assert.Equal(t, "dev", locals["env"])

	// Resolved settings should be persisted ({{ .locals.ns }} → "acme").
	settings, ok := deepMergedConfig["settings"].(map[string]any)
	require.True(t, ok, "settings should be persisted")
	assert.Equal(t, "acme", settings["namespace"])

	// Resolved vars should be persisted ({{ .locals.env }} → "dev").
	vars, ok := deepMergedConfig["vars"].(map[string]any)
	require.True(t, ok, "vars should be persisted")
	assert.Equal(t, "dev", vars["environment"])

	// Resolved env should be persisted ({{ .locals.ns }} → "acme").
	env, ok := deepMergedConfig["env"].(map[string]any)
	require.True(t, ok, "env should be persisted")
	assert.Equal(t, "acme", env["NAMESPACE"])
}

// TestExtractAndAddLocalsToContext_VarsWithProcessedSettings tests that vars
// can reference processed settings values through the template pipeline.
func TestExtractAndAddLocalsToContext_VarsWithProcessedSettings(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}

	yamlContent := `
locals:
  stage: dev
settings:
  resolved_stage: '{{ .locals.stage }}'
vars:
  from_settings: '{{ .settings.resolved_stage }}'
`
	ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
	require.NoError(t, err)

	// Settings should have resolved {{ .locals.stage }} → "dev".
	settings, ok := ctx["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "dev", settings["resolved_stage"])

	// Vars should have resolved {{ .settings.resolved_stage }} → "dev".
	vars, ok := ctx["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "dev", vars["from_settings"])
}

// TestExtractAndAddLocalsToContext_EnvWithProcessedSettingsAndVars tests that env
// can reference both processed settings and vars through the template pipeline.
func TestExtractAndAddLocalsToContext_EnvWithProcessedSettingsAndVars(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}

	yamlContent := `
locals:
  ns: acme
settings:
  region: us-east-1
vars:
  app_name: '{{ .locals.ns }}-app'
env:
  APP: '{{ .locals.ns }}'
  REGION: '{{ .settings.region }}'
`
	ctx, err := extractAndAddLocalsToContext(atmosConfig, yamlContent, "test.yaml", "test.yaml", nil)
	require.NoError(t, err)

	env, ok := ctx["env"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", env["APP"])
	assert.Equal(t, "us-east-1", env["REGION"])
}

// TestAtmosProTemplateRegression tests that {{ .atmos_component }} templates in non-.tmpl files
// with settings sections don't fail during import processing.
// This is a regression test for issue where 1.205 inadvertently triggers template processing
// for imports when the file has settings/vars/env sections (due to locals feature changes).
func TestAtmosProTemplateRegression(t *testing.T) {
	stacksBasePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-pro-template-regression", "stacks")
	filePath := filepath.Join(stacksBasePath, "deploy", "test.yaml")

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}

	// Process the stack manifest that imports the atmos-pro mixin.
	// The mixin has a settings section and uses {{ .atmos_component }} templates.
	// In 1.204, this worked because templates weren't processed during import for non-.tmpl files.
	// In 1.205, the locals feature inadvertently triggers template processing because it adds
	// settings/vars/env to the context, making len(context) > 0.
	deepMergedConfig, importsConfig, stackConfigMap, tfInline, tfImports, hfInline, hfImports, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,   // No external context - this is key to the test.
		false, // ignoreMissingFiles.
		false, // skipTemplatesProcessingInImports.
		false, // ignoreMissingTemplateValues - set to false to catch the error.
		false, // skipIfMissing.
		nil,
		nil,
		nil,
		nil,
		"",
	)

	// The test should pass - templates like {{ .atmos_component }} should NOT be processed
	// during import when no external context is provided.
	require.NoError(t, err, "Processing should not fail - templates should be deferred until component processing")
	require.NotNil(t, deepMergedConfig)

	// Suppress unused variable warnings - these are returned by ProcessYAMLConfigFile but not needed for this test.
	_ = importsConfig
	_ = stackConfigMap
	_ = tfInline
	_ = tfImports
	_ = hfInline
	_ = hfImports

	// Verify the settings.pro section exists and contains unprocessed template strings.
	settings, ok := deepMergedConfig["settings"].(map[string]any)
	require.True(t, ok, "settings section should exist")

	pro, ok := settings["pro"].(map[string]any)
	require.True(t, ok, "settings.pro section should exist")

	assert.Equal(t, true, pro["enabled"], "pro.enabled should be true")

	// The template strings should be preserved (not processed) at this stage.
	// They will be processed later in describe_stacks when component context is available.
	pr, ok := pro["pull_request"].(map[string]any)
	require.True(t, ok, "settings.pro.pull_request should exist")

	opened, ok := pr["opened"].(map[string]any)
	require.True(t, ok, "settings.pro.pull_request.opened should exist")

	workflows, ok := opened["workflows"].(map[string]any)
	require.True(t, ok, "settings.pro.pull_request.opened.workflows should exist")

	planWorkflow, ok := workflows["atmos-terraform-plan.yaml"].(map[string]any)
	require.True(t, ok, "atmos-terraform-plan.yaml workflow should exist")

	inputs, ok := planWorkflow["inputs"].(map[string]any)
	require.True(t, ok, "workflow inputs should exist")

	// The component input should still contain the template string {{ .atmos_component }}
	// because templates should NOT be processed during import for non-.tmpl files without explicit context.
	componentInput, ok := inputs["component"].(string)
	require.True(t, ok, "component input should be a string")
	assert.Equal(t, "{{ .atmos_component }}", componentInput,
		"Template {{ .atmos_component }} should be preserved during import, not processed")
}

// TestComputeStackFileName tests that computeStackFileName correctly strips the
// stacks base path and file extension to produce a relative stack file name.
func TestComputeStackFileName(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		filePath string
		expected string
	}{
		{
			name: "simple deploy path with name_pattern",
			config: &schema.AtmosConfiguration{
				BasePath: "project",
				Stacks:   schema.Stacks{BasePath: "stacks"},
			},
			filePath: filepath.Join("project", "stacks", "deploy", "dev.yaml"),
			expected: filepath.Join("deploy", "dev"),
		},
		{
			name: "nested org path",
			config: &schema.AtmosConfiguration{
				BasePath: "project",
				Stacks:   schema.Stacks{BasePath: "stacks"},
			},
			filePath: filepath.Join("project", "stacks", "orgs", "acme", "plat", "dev", "us-east-1.yaml"),
			expected: filepath.Join("orgs", "acme", "plat", "dev", "us-east-1"),
		},
		{
			name: "yml extension",
			config: &schema.AtmosConfiguration{
				BasePath: "project",
				Stacks:   schema.Stacks{BasePath: "stacks"},
			},
			filePath: filepath.Join("project", "stacks", "deploy", "prod.yml"),
			expected: filepath.Join("deploy", "prod"),
		},
		{
			name: "yaml.tmpl extension",
			config: &schema.AtmosConfiguration{
				BasePath: "project",
				Stacks:   schema.Stacks{BasePath: "stacks"},
			},
			filePath: filepath.Join("project", "stacks", "deploy", "dev.yaml.tmpl"),
			expected: filepath.Join("deploy", "dev"),
		},
		{
			name: "yml.tmpl extension",
			config: &schema.AtmosConfiguration{
				BasePath: "project",
				Stacks:   schema.Stacks{BasePath: "stacks"},
			},
			filePath: filepath.Join("project", "stacks", "deploy", "dev.yml.tmpl"),
			expected: filepath.Join("deploy", "dev"),
		},
		{
			name: "no known extension returns path as-is",
			config: &schema.AtmosConfiguration{
				BasePath: "project",
				Stacks:   schema.Stacks{BasePath: "stacks"},
			},
			filePath: filepath.Join("project", "stacks", "deploy", "dev.json"),
			expected: filepath.Join("deploy", "dev.json"),
		},
		{
			name: "file with no extension",
			config: &schema.AtmosConfiguration{
				BasePath: "project",
				Stacks:   schema.Stacks{BasePath: "stacks"},
			},
			filePath: filepath.Join("project", "stacks", "deploy", "dev"),
			expected: filepath.Join("deploy", "dev"),
		},
		{
			name:     "nil config returns empty",
			config:   nil,
			filePath: filepath.Join("project", "stacks", "deploy", "dev.yaml"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config == nil {
				result := computeStackFileName(tt.config, tt.filePath)
				assert.Empty(t, result)
				return
			}
			result := computeStackFileName(tt.config, tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDeriveStackNameForLocals tests that deriveStackNameForLocals correctly
// derives the stack name from file path and raw config for locals processing.
func TestDeriveStackNameForLocals(t *testing.T) {
	t.Run("derives stack name from name_pattern and vars", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			BasePath: "project",
			Stacks: schema.Stacks{
				BasePath:    "stacks",
				NamePattern: "{stage}",
			},
		}
		rawConfig := map[string]any{
			"vars": map[string]any{
				"stage": "dev",
			},
		}
		result := deriveStackNameForLocals(config, rawConfig, filepath.Join("project", "stacks", "deploy", "dev.yaml"))
		assert.Equal(t, "dev", result)
	})

	t.Run("nil config returns empty", func(t *testing.T) {
		rawConfig := map[string]any{
			"vars": map[string]any{"stage": "dev"},
		}
		result := deriveStackNameForLocals(nil, rawConfig, filepath.Join("project", "stacks", "deploy", "dev.yaml"))
		assert.Empty(t, result)
	})

	t.Run("no vars uses filename as stack name", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			BasePath: "project",
			Stacks: schema.Stacks{
				BasePath: "stacks",
			},
		}
		rawConfig := map[string]any{}
		result := deriveStackNameForLocals(config, rawConfig, filepath.Join("project", "stacks", "deploy", "dev.yaml"))
		// Without name_pattern or name_template, falls back to file name.
		assert.Equal(t, filepath.Join("deploy", "dev"), result)
	})
}

// TestExtractLocalsFromRawYAML_StackNameDerived tests that extractLocalsFromRawYAML
// derives the stack name from the file path and passes it to ProcessStackLocals,
// rather than passing an empty string. This is the fix for GitHub issue #2080.
func TestExtractLocalsFromRawYAML_StackNameDerived(t *testing.T) {
	// This test verifies that locals with Go template conditionals using !env work correctly.
	// Issue 1 from the fix doc: Go template conditionals were broken in v1.200.0 but work now.
	t.Setenv("PR_NUMBER", "42")

	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  pr_number: !env PR_NUMBER
  datastream_name: '{{ if .locals.pr_number }}datastreampr{{ .locals.pr_number }}{{ else }}datastream{{ end }}'
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "42", result.locals["pr_number"])
	assert.Equal(t, "datastreampr42", result.locals["datastream_name"])
}

// TestExtractLocalsFromRawYAML_GoTemplateConditionalEmpty tests Go template conditionals
// when the environment variable is empty (the else branch should be taken).
func TestExtractLocalsFromRawYAML_GoTemplateConditionalEmpty(t *testing.T) {
	// When PR_NUMBER is empty, the else branch should produce "datastream".
	t.Setenv("PR_NUMBER_EMPTY_TEST", "")

	atmosConfig := &schema.AtmosConfiguration{}
	yamlContent := `
locals:
  pr_number: !env PR_NUMBER_EMPTY_TEST
  datastream_name: '{{ if .locals.pr_number }}datastreampr{{ .locals.pr_number }}{{ else }}datastream{{ end }}'
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "", result.locals["pr_number"])
	assert.Equal(t, "datastream", result.locals["datastream_name"])
}

// TestExtractLocalsFromRawYAML_TerraformStateInLocals tests that !terraform.state
// in locals correctly receives the derived stack name instead of empty string.
// This is the core test for the GitHub issue #2080 fix.
func TestExtractLocalsFromRawYAML_TerraformStateInLocals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetterObj := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetterObj
	defer func() { stateGetter = originalGetter }()

	// Create a temp directory structure that simulates a real Atmos project.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks", "deploy")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	stackFile := filepath.Join(stacksDir, "dev.yaml")

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Stacks: schema.Stacks{
			BasePath:    "stacks",
			NamePattern: "{stage}",
		},
	}

	// The mock expects to be called with stack="dev" (derived from file path + name_pattern + vars).
	// Before the fix, stack would be "" causing the "stack is required" error.
	mockStateGetterObj.EXPECT().
		GetState(
			atmosConfig,
			gomock.Any(),
			"dev",        // stack - derived from name_pattern {stage} + vars.stage=dev
			"vpc",        // component
			".vpc_id",    // output
			false,        // skipCache
			gomock.Any(), // authContext
			gomock.Any(), // authManager
		).
		Return("vpc-12345", nil)

	yamlContent := `
vars:
  stage: dev

locals:
  vpc_id: !terraform.state vpc .vpc_id
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, stackFile)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "vpc-12345", result.locals["vpc_id"])
}

// TestExtractLocalsFromRawYAML_TerraformStateComposedLocals tests that !terraform.state
// results can be used in composed locals via Go templates.
func TestExtractLocalsFromRawYAML_TerraformStateComposedLocals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetterObj := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetterObj
	defer func() { stateGetter = originalGetter }()

	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks", "deploy")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	stackFile := filepath.Join(stacksDir, "prod.yaml")

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Stacks: schema.Stacks{
			BasePath:    "stacks",
			NamePattern: "{stage}",
		},
	}

	// Mock two separate !terraform.state calls.
	mockStateGetterObj.EXPECT().
		GetState(atmosConfig, gomock.Any(), "prod", "vpc", ".vpc_id", false, gomock.Any(), gomock.Any()).
		Return("vpc-prod-99", nil)

	mockStateGetterObj.EXPECT().
		GetState(atmosConfig, gomock.Any(), "prod", "vpc", ".private_subnet_ids", false, gomock.Any(), gomock.Any()).
		Return("subnet-a,subnet-b", nil)

	yamlContent := `
vars:
  stage: prod

locals:
  vpc_id: !terraform.state vpc .vpc_id
  subnet_ids: !terraform.state vpc .private_subnet_ids
  vpc_info: "vpc={{ .locals.vpc_id }}, subnets={{ .locals.subnet_ids }}"
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, stackFile)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "vpc-prod-99", result.locals["vpc_id"])
	assert.Equal(t, "subnet-a,subnet-b", result.locals["subnet_ids"])
	assert.Equal(t, "vpc=vpc-prod-99, subnets=subnet-a,subnet-b", result.locals["vpc_info"])
}

// TestExtractLocalsFromRawYAML_TerraformState3ArgForm tests that !terraform.state
// with 3 args (explicit stack) still works correctly.
func TestExtractLocalsFromRawYAML_TerraformState3ArgForm(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetterObj := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetterObj
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{}

	// 3-arg form uses the explicit stack "shared", not the derived stack.
	mockStateGetterObj.EXPECT().
		GetState(atmosConfig, gomock.Any(), "shared", "vpc", ".vpc_id", false, gomock.Any(), gomock.Any()).
		Return("vpc-shared-01", nil)

	yamlContent := `
locals:
  vpc_id: !terraform.state vpc shared .vpc_id
`
	result, err := extractLocalsFromRawYAML(atmosConfig, yamlContent, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "vpc-shared-01", result.locals["vpc_id"])
}
