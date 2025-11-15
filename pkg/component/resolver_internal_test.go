package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test extractComponentsSection function.
func TestExtractComponentsSection(t *testing.T) {
	tests := []struct {
		name          string
		stackConfig   map[string]any
		componentType string
		stack         string
		wantErr       bool
		errContains   string
		expectedLen   int // Number of components expected if successful
	}{
		{
			name: "valid terraform components",
			stackConfig: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc":    map[string]any{"vars": map[string]any{}},
						"lambda": map[string]any{"vars": map[string]any{}},
					},
				},
			},
			componentType: "terraform",
			stack:         "test-stack",
			wantErr:       false,
			expectedLen:   2,
		},
		{
			name: "valid helmfile components",
			stackConfig: map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{
						"nginx": map[string]any{"vars": map[string]any{}},
					},
				},
			},
			componentType: "helmfile",
			stack:         "test-stack",
			wantErr:       false,
			expectedLen:   1,
		},
		{
			name:          "missing components section",
			stackConfig:   map[string]any{},
			componentType: "terraform",
			stack:         "test-stack",
			wantErr:       true,
			errContains:   "has no components section",
		},
		{
			name: "invalid components section type",
			stackConfig: map[string]any{
				"components": "not a map",
			},
			componentType: "terraform",
			stack:         "test-stack",
			wantErr:       true,
			errContains:   "invalid components section",
		},
		{
			name: "missing component type",
			stackConfig: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{},
				},
			},
			componentType: "helmfile",
			stack:         "test-stack",
			wantErr:       true,
			errContains:   "has no helmfile components",
		},
		{
			name: "invalid component type section",
			stackConfig: map[string]any{
				"components": map[string]any{
					"terraform": "not a map",
				},
			},
			componentType: "terraform",
			stack:         "test-stack",
			wantErr:       true,
			errContains:   "invalid terraform components section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractComponentsSection(tt.stackConfig, tt.componentType, tt.stack)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Len(t, result, tt.expectedLen)
			}
		})
	}
}

// Test findComponentMatches function.
func TestFindComponentMatches(t *testing.T) {
	tests := []struct {
		name              string
		typeComponentsMap map[string]any
		componentName     string
		expectedMatches   []string
	}{
		{
			name: "direct match",
			typeComponentsMap: map[string]any{
				"vpc":    map[string]any{"vars": map[string]any{}},
				"lambda": map[string]any{"vars": map[string]any{}},
			},
			componentName:   "vpc",
			expectedMatches: []string{"vpc"},
		},
		{
			name: "alias via component field",
			typeComponentsMap: map[string]any{
				"vpc-prod": map[string]any{
					"component": "vpc",
					"vars":      map[string]any{},
				},
				"vpc-dev": map[string]any{
					"component": "vpc",
					"vars":      map[string]any{},
				},
			},
			componentName:   "vpc",
			expectedMatches: []string{"vpc-prod", "vpc-dev"},
		},
		{
			name: "alias via metadata.component field",
			typeComponentsMap: map[string]any{
				"custom-vpc": map[string]any{
					"metadata": map[string]any{
						"component": "vpc",
					},
					"vars": map[string]any{},
				},
			},
			componentName:   "vpc",
			expectedMatches: []string{"custom-vpc"},
		},
		{
			name: "mixed: direct match takes precedence",
			typeComponentsMap: map[string]any{
				"vpc": map[string]any{
					"vars": map[string]any{},
				},
				"vpc-prod": map[string]any{
					"component": "vpc",
					"vars":      map[string]any{},
				},
			},
			componentName:   "vpc",
			expectedMatches: []string{"vpc"}, // Direct match returns immediately
		},
		{
			name: "no matches",
			typeComponentsMap: map[string]any{
				"lambda": map[string]any{"vars": map[string]any{}},
				"rds":    map[string]any{"vars": map[string]any{}},
			},
			componentName:   "vpc",
			expectedMatches: []string{},
		},
		{
			name: "both component and metadata.component aliases",
			typeComponentsMap: map[string]any{
				"vpc-prod": map[string]any{
					"component": "vpc",
					"vars":      map[string]any{},
				},
				"vpc-staging": map[string]any{
					"metadata": map[string]any{
						"component": "vpc",
					},
					"vars": map[string]any{},
				},
			},
			componentName:   "vpc",
			expectedMatches: []string{"vpc-prod", "vpc-staging"},
		},
		{
			name: "invalid component config types are skipped",
			typeComponentsMap: map[string]any{
				"invalid": "not a map",
				"vpc": map[string]any{
					"vars": map[string]any{},
				},
			},
			componentName:   "vpc",
			expectedMatches: []string{"vpc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := findComponentMatches(tt.typeComponentsMap, tt.componentName)

			if len(tt.expectedMatches) == 0 {
				assert.Empty(t, matches)
			} else {
				assert.ElementsMatch(t, tt.expectedMatches, matches)
			}
		})
	}
}

// Test handleComponentMatches function.
func TestHandleComponentMatches(t *testing.T) {
	tests := []struct {
		name          string
		matches       []string
		componentName string
		stack         string
		componentType string
		wantErr       bool
		errContains   string
		expectedKey   string
	}{
		{
			name:          "no matches - error",
			matches:       []string{},
			componentName: "vpc",
			stack:         "prod-stack",
			componentType: "terraform",
			wantErr:       true,
			errContains:   "not found in stack",
		},
		{
			name:          "single match - success",
			matches:       []string{"vpc"},
			componentName: "vpc",
			stack:         "prod-stack",
			componentType: "terraform",
			wantErr:       false,
			expectedKey:   "vpc",
		},
		{
			name:          "single alias match - success",
			matches:       []string{"vpc-prod"},
			componentName: "vpc",
			stack:         "prod-stack",
			componentType: "terraform",
			wantErr:       false,
			expectedKey:   "vpc-prod",
		},
		{
			name:          "multiple matches - ambiguous error",
			matches:       []string{"vpc-prod", "vpc-staging"},
			componentName: "vpc",
			stack:         "prod-stack",
			componentType: "terraform",
			wantErr:       true,
			errContains:   "ambiguous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleComponentMatches(tt.matches, tt.componentName, tt.stack, tt.componentType)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedKey, result)
			}
		})
	}
}
