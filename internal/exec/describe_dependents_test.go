package exec

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Tests for getComponentDependencies helper function.

func TestGetComponentDependencies(t *testing.T) {
	t.Run("prefers dependencies.components over settings.depends_on", func(t *testing.T) {
		componentMap := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"component": "vpc"},
					map[string]any{"component": "rds", "stack": "tenant1-ue1-prod"},
				},
			},
			"settings": map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{"component": "old-vpc"},
				},
			},
		}

		deps, settingsSection, source := getComponentDependencies(componentMap)

		assert.Len(t, deps, 2)
		assert.Equal(t, "vpc", deps[0].Component)
		assert.Equal(t, "rds", deps[1].Component)
		assert.Equal(t, "tenant1-ue1-prod", deps[1].Stack)
		assert.NotNil(t, settingsSection)
		assert.Equal(t, dependencySourceDependenciesComponents, source)
	})

	t.Run("falls back to settings.depends_on when dependencies.components is empty", func(t *testing.T) {
		componentMap := map[string]any{
			"dependencies": map[string]any{
				"tools": map[string]any{
					"terraform": "1.9.8",
				},
			},
			"settings": map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{"component": "vpc"},
					2: map[string]any{"component": "subnet", "environment": "ue2"},
				},
			},
		}

		deps, settingsSection, source := getComponentDependencies(componentMap)

		assert.Len(t, deps, 2)
		// Note: map iteration order is not guaranteed, so we check for presence.
		componentNames := make(map[string]bool)
		for _, dep := range deps {
			componentNames[dep.Component] = true
		}
		assert.True(t, componentNames["vpc"])
		assert.True(t, componentNames["subnet"])
		assert.NotNil(t, settingsSection)
		assert.Equal(t, dependencySourceSettingsDependsOn, source)
	})

	t.Run("returns nil when no dependencies defined", func(t *testing.T) {
		componentMap := map[string]any{
			"vars": map[string]any{
				"name": "my-component",
			},
			"settings": map[string]any{
				"spacelift": map[string]any{
					"workspace_enabled": true,
				},
			},
		}

		deps, settingsSection, source := getComponentDependencies(componentMap)

		assert.Nil(t, deps)
		assert.NotNil(t, settingsSection)
		assert.Equal(t, dependencySourceNone, source)
	})

	t.Run("returns nil deps but settings when settings has no depends_on", func(t *testing.T) {
		componentMap := map[string]any{
			"settings": map[string]any{
				"atlantis": map[string]any{
					"enabled": true,
				},
			},
		}

		deps, settingsSection, source := getComponentDependencies(componentMap)

		assert.Nil(t, deps)
		assert.NotNil(t, settingsSection)
		assert.Equal(t, dependencySourceNone, source)
	})

	t.Run("handles empty component map", func(t *testing.T) {
		componentMap := map[string]any{}

		deps, settingsSection, source := getComponentDependencies(componentMap)

		assert.Nil(t, deps)
		assert.Nil(t, settingsSection)
		assert.Equal(t, dependencySourceNone, source)
	})

	t.Run("handles dependencies.components with stack field", func(t *testing.T) {
		componentMap := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{
						"component": "shared-vpc",
						"stack":     "acme-ue1-network",
					},
				},
			},
		}

		deps, _, source := getComponentDependencies(componentMap)

		assert.Len(t, deps, 1)
		assert.Equal(t, "shared-vpc", deps[0].Component)
		assert.Equal(t, "acme-ue1-network", deps[0].Stack)
		assert.Equal(t, dependencySourceDependenciesComponents, source)
	})

	t.Run("handles file and folder dependencies in dependencies.components", func(t *testing.T) {
		componentMap := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"component": "vpc"},
					map[string]any{"kind": "file", "path": "configs/settings.json"},
					map[string]any{"kind": "folder", "path": "src/lambda"},
				},
			},
		}

		deps, _, source := getComponentDependencies(componentMap)
		assert.Equal(t, dependencySourceDependenciesComponents, source)

		assert.Len(t, deps, 3)
		assert.Equal(t, "vpc", deps[0].Component)
		assert.Equal(t, "file", deps[1].Kind)
		assert.Equal(t, "configs/settings.json", deps[1].Path)
		assert.Equal(t, "folder", deps[2].Kind)
		assert.Equal(t, "src/lambda", deps[2].Path)
	})

	t.Run("handles cross-type kind in dependencies.components", func(t *testing.T) {
		componentMap := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"component": "vpc", "kind": "terraform"},
					map[string]any{"component": "chart", "kind": "helmfile"},
				},
			},
		}

		deps, _, source := getComponentDependencies(componentMap)
		assert.Equal(t, dependencySourceDependenciesComponents, source)

		assert.Len(t, deps, 2)
		assert.Equal(t, "vpc", deps[0].Component)
		assert.Equal(t, "terraform", deps[0].Kind)
		assert.Equal(t, "chart", deps[1].Component)
		assert.Equal(t, "helmfile", deps[1].Kind)
	})
}

func TestGetComponentDependenciesListMergeBehavior(t *testing.T) {
	// This test documents the expected merge behavior.
	// The actual merging happens in the stack processor, but we test that
	// our function correctly extracts the merged list.

	t.Run("dependencies.components is a flat list after merge", func(t *testing.T) {
		// After stack inheritance merging, the final component section should
		// have a flat list of all dependencies from parent and child stacks.
		// This test verifies that getComponentDependencies correctly extracts
		// this merged list.
		componentMap := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					// These would have been merged from parent stack.
					map[string]any{"component": "account-settings"},
					map[string]any{"component": "iam-baseline"},
					// These would have been added by child stack.
					map[string]any{"component": "vpc"},
					map[string]any{"component": "rds", "stack": "tenant1-ue1-prod"},
				},
			},
		}

		deps, _, source := getComponentDependencies(componentMap)

		assert.Len(t, deps, 4)
		// Verify all components are present in order.
		assert.Equal(t, "account-settings", deps[0].Component)
		assert.Equal(t, "iam-baseline", deps[1].Component)
		assert.Equal(t, "vpc", deps[2].Component)
		assert.Equal(t, "rds", deps[3].Component)
		assert.Equal(t, dependencySourceDependenciesComponents, source)
	})
}

func TestDependenciesSchemaComponents(t *testing.T) {
	// Test that the Dependencies struct correctly holds component dependencies.
	t.Run("Dependencies struct holds component list", func(t *testing.T) {
		deps := schema.Dependencies{
			Tools: map[string]string{
				"terraform": "1.9.8",
			},
			Components: []schema.ComponentDependency{
				{Component: "vpc"},
				{Component: "rds", Stack: "tenant1-ue1-prod"},
				{Component: "tgw/hub", Stack: "acme-ue1-network"},
				{Kind: "file", Path: "configs/settings.json"},
				{Kind: "folder", Path: "src/lambda"},
			},
		}

		assert.Len(t, deps.Components, 5)
		assert.Equal(t, "vpc", deps.Components[0].Component)
		assert.Equal(t, "rds", deps.Components[1].Component)
		assert.Equal(t, "tenant1-ue1-prod", deps.Components[1].Stack)
		assert.Equal(t, "tgw/hub", deps.Components[2].Component)
		assert.Equal(t, "acme-ue1-network", deps.Components[2].Stack)
		assert.True(t, deps.Components[3].IsFileDependency())
		assert.Equal(t, "configs/settings.json", deps.Components[3].Path)
		assert.True(t, deps.Components[4].IsFolderDependency())
		assert.Equal(t, "src/lambda", deps.Components[4].Path)
	})
}

func TestNewDescribeDependentsExec(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	exec := NewDescribeDependentsExec(atmosConfig)

	assert.NotNil(t, exec)
	assert.Equal(t, atmosConfig, exec.(*describeDependentsExec).atmosConfig)
	assert.NotNil(t, exec.(*describeDependentsExec).executeDescribeDependents)
	assert.NotNil(t, exec.(*describeDependentsExec).newPageCreator)
	assert.NotNil(t, exec.(*describeDependentsExec).isTTYSupportForStdout)
}

func TestDescribeDependentsExec_Execute_Success_NoQuery(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	dependents := []schema.Dependent{
		{Component: "comp1", Stack: "stack1"},
		{Component: "comp2", Stack: "stack2"},
	}

	// Mock functions
	mockExecuteDescribeDependents := func(config *schema.AtmosConfiguration, args *DescribeDependentsArgs) ([]schema.Dependent, error) {
		assert.Equal(t, "test-component", args.Component)
		assert.Equal(t, "test-stack", args.Stack)
		assert.False(t, args.IncludeSettings)
		return dependents, nil
	}

	mockIsTTYSupportForStdout := func() bool {
		return true
	}

	ctrl := gomock.NewController(t)
	mockPageCreator := pager.NewMockPageCreator(ctrl)
	exec := &describeDependentsExec{
		executeDescribeDependents: mockExecuteDescribeDependents,
		evaluateYqExpression: func(config *schema.AtmosConfiguration, data any, query string) (any, error) {
			assert.Equal(t, atmosConfig, config)
			assert.Equal(t, dependents, data)
			assert.Equal(t, "", query) // No query provided
			return dependents, nil
		},
		atmosConfig:           atmosConfig,
		newPageCreator:        mockPageCreator,
		isTTYSupportForStdout: mockIsTTYSupportForStdout,
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "yaml",
		File:      "output.yaml",
		Query:     "", // No query
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.NoError(t, err)
}

func TestDescribeDependentsExec_Execute_Success_WithQuery(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	newMockPageCreator := pager.NewMockPageCreator(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}
	dependents := []schema.Dependent{
		{Component: "comp1", Stack: "stack1"},
		{Component: "comp2", Stack: "stack2"},
	}
	queryResult := map[string]string{"filtered": "result"}

	mockEvaluateYqExpression := func(config *schema.AtmosConfiguration, data any, query string) (any, error) {
		assert.Equal(t, atmosConfig, config)
		assert.Equal(t, dependents, data)
		assert.Equal(t, ".name", query)
		return queryResult, nil
	}

	mockIsTTYSupportForStdout := func() bool {
		return false
	}

	exec := &describeDependentsExec{
		atmosConfig: atmosConfig,
		executeDescribeDependents: func(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs) ([]schema.Dependent, error) {
			return dependents, nil
		},
		newPageCreator:        newMockPageCreator,
		isTTYSupportForStdout: mockIsTTYSupportForStdout,
		evaluateYqExpression:  mockEvaluateYqExpression,
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "json",
		File:      "",
		Query:     ".name",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.NoError(t, err)
}

func TestDescribeDependentsExec_Execute_ExecuteDescribeDependentsError(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	expectedError := errors.New("execute describe dependents failed")
	pagerMock := pager.NewMockPageCreator(gomock.NewController(t))
	exec := &describeDependentsExec{
		atmosConfig: atmosConfig,
		executeDescribeDependents: func(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs) ([]schema.Dependent, error) {
			return nil, expectedError
		},
		newPageCreator:        pagerMock,
		isTTYSupportForStdout: func() bool { return false },
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "yaml",
		File:      "",
		Query:     "",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestDescribeDependentsExec_Execute_YqExpressionError(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	ctrl := gomock.NewController(t)
	mockPageCreator := pager.NewMockPageCreator(ctrl)
	dependents := []schema.Dependent{
		{Component: "comp1", Stack: "stack1"},
	}
	expectedError := errors.New("yq expression evaluation failed")

	mockEvaluateYqExpression := func(config *schema.AtmosConfiguration, data any, query string) (any, error) {
		return nil, expectedError
	}

	exec := &describeDependentsExec{
		atmosConfig: atmosConfig,
		executeDescribeDependents: func(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs) ([]schema.Dependent, error) {
			return dependents, nil
		},
		newPageCreator: mockPageCreator,
		isTTYSupportForStdout: func() bool {
			return false
		},
		evaluateYqExpression: mockEvaluateYqExpression,
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "yaml",
		File:      "",
		Query:     ".invalid",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestDescribeDependentsExec_Execute_EmptyDependents(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	dependents := []schema.Dependent{}

	mockExecuteDescribeDependents := func(config *schema.AtmosConfiguration, args *DescribeDependentsArgs) ([]schema.Dependent, error) {
		return dependents, nil
	}

	mockIsTTYSupportForStdout := func() bool {
		return false
	}

	pagerMock := pager.NewMockPageCreator(gomock.NewController(t))
	exec := &describeDependentsExec{
		atmosConfig:               atmosConfig,
		executeDescribeDependents: mockExecuteDescribeDependents,
		newPageCreator:            pagerMock,
		isTTYSupportForStdout:     mockIsTTYSupportForStdout,
	}

	props := &DescribeDependentsExecProps{
		Component: "nonexistent-component",
		Stack:     "nonexistent-stack",
		Format:    "json",
		File:      "",
		Query:     "",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.NoError(t, err)
}

func TestDescribeDependentsExec_Execute_DifferentFormatsAndFiles(t *testing.T) {
	testCases := []struct {
		name     string
		format   string
		file     string
		expected string
	}{
		{
			name:     "JSON format with file",
			format:   "json",
			file:     "output.json",
			expected: "Dependents of 'comp' in stack 'stack'",
		},
		{
			name:     "YAML format without file",
			format:   "yaml",
			file:     "",
			expected: "Dependents of 'comp' in stack 'stack'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			dependents := []schema.Dependent{{Component: "comp1", Stack: "stack1"}}
			pagerMock := pager.NewMockPageCreator(gomock.NewController(t))
			// Mock functions
			mockExecuteDescribeDependents := func(config *schema.AtmosConfiguration, args *DescribeDependentsArgs) ([]schema.Dependent, error) {
				return dependents, nil
			}

			mockIsTTYSupportForStdout := func() bool { return false }

			exec := &describeDependentsExec{
				atmosConfig:               atmosConfig,
				executeDescribeDependents: mockExecuteDescribeDependents,
				newPageCreator:            pagerMock,
				isTTYSupportForStdout:     mockIsTTYSupportForStdout,
			}

			props := &DescribeDependentsExecProps{
				Component: "comp",
				Stack:     "stack",
				Format:    tc.format,
				File:      tc.file,
				Query:     "",
			}

			err := exec.Execute(props)
			assert.NoError(t, err)
		})
	}
}

func TestDescribeDependents_WithStacksNameTemplate(t *testing.T) {
	// Environment isolation
	// Working directory isolation.
	workDir := "../../tests/fixtures/scenarios/depends-on-with-stacks-name-template"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Init Atmos config.
	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, true)
	require.NoError(t, err, "InitCliConfig failed")

	// Build an OS-specific expected path once
	componentPath := filepath.Join("..", "..", "components", "terraform", "mock")

	// Matrix-driven cases
	cases := []struct {
		name      string
		component string
		stack     string
		expected  []schema.Dependent
	}{
		{
			name:      "ue1-network-vpc",
			component: "vpc",
			stack:     "ue1-network",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-network",
					StackSlug:     "ue1-network-tgw-attachment",
				},
				{
					Component:     "tgw/hub",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-network",
					StackSlug:     "ue1-network-tgw-hub",
				},
			},
		},
		{
			name:      "uw2-network-vpc",
			component: "vpc",
			stack:     "uw2-network",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-network",
					StackSlug:     "uw2-network-tgw-attachment",
				},
			},
		},
		{
			name:      "ue1-prod-vpc",
			component: "vpc",
			stack:     "ue1-prod",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-prod",
					StackSlug:     "ue1-prod-tgw-attachment",
				},
			},
		},
		{
			name:      "uw2-prod-vpc",
			component: "vpc",
			stack:     "uw2-prod",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-prod",
					StackSlug:     "uw2-prod-tgw-attachment",
				},
			},
		},
		{
			name:      "ue1-network-tgw-hub",
			component: "tgw/hub",
			stack:     "ue1-network",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-network",
					StackSlug:     "ue1-network-tgw-attachment",
				},
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-network",
					StackSlug:     "uw2-network-tgw-attachment",
				},
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-prod",
					StackSlug:     "ue1-prod-tgw-attachment",
				},
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-prod",
					StackSlug:     "uw2-prod-tgw-attachment",
				},
				{
					Component:     "tgw/cross-region-hub-connector",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-network",
					StackSlug:     "uw2-network-tgw-cross-region-hub-connector",
				},
			},
		},
		{
			name:      "uw2-network-tgw-cross-region-hub-connector",
			component: "tgw/cross-region-hub-connector",
			stack:     "uw2-network",
			expected:  []schema.Dependent{},
		},
		{
			name:      "ue1-network-tgw-attachment",
			component: "tgw/attachment",
			stack:     "ue1-network",
			expected:  []schema.Dependent{},
		},
		{
			name:      "uw2-network-tgw-attachment",
			component: "tgw/attachment",
			stack:     "uw2-network",
			expected:  []schema.Dependent{},
		},
		{
			name:      "ue1-prod-tgw-attachment",
			component: "tgw/attachment",
			stack:     "ue1-prod",
			expected:  []schema.Dependent{},
		},
		{
			name:      "uw2-prod-tgw-attachment",
			component: "tgw/attachment",
			stack:     "uw2-prod",
			expected:  []schema.Dependent{},
		},
	}

	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExecuteDescribeDependents(&atmosConfig, &DescribeDependentsArgs{
				Component:            tc.component,
				Stack:                tc.stack,
				IncludeSettings:      false,
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
				Skip:                 nil,
				OnlyInStack:          "",
			})
			require.NoError(t, err)

			// Order-agnostic equality on struct slices
			assert.ElementsMatch(t, tc.expected, res)
		})
	}
}

func TestDescribeDependents_WithStacksNamePattern(t *testing.T) {
	// Working directory isolation.
	workDir := "../../tests/fixtures/scenarios/depends-on-with-stacks-name-pattern"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Init Atmos config
	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, true)
	require.NoError(t, err, "InitCliConfig failed")

	// Build an OS-specific expected path once
	componentPath := filepath.Join("..", "..", "components", "terraform", "mock")

	// Matrix-driven cases
	cases := []struct {
		name      string
		component string
		stack     string
		expected  []schema.Dependent
	}{
		{
			name:      "ue1-network-vpc",
			component: "vpc",
			stack:     "ue1-network",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-network",
					StackSlug:     "ue1-network-tgw-attachment",
					Environment:   "ue1",
					Stage:         "network",
				},
				{
					Component:     "tgw/hub",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-network",
					StackSlug:     "ue1-network-tgw-hub",
					Environment:   "ue1",
					Stage:         "network",
				},
			},
		},
		{
			name:      "uw2-network-vpc",
			component: "vpc",
			stack:     "uw2-network",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-network",
					StackSlug:     "uw2-network-tgw-attachment",
					Environment:   "uw2",
					Stage:         "network",
				},
			},
		},
		{
			name:      "ue1-prod-vpc",
			component: "vpc",
			stack:     "ue1-prod",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-prod",
					StackSlug:     "ue1-prod-tgw-attachment",
					Environment:   "ue1",
					Stage:         "prod",
				},
			},
		},
		{
			name:      "uw2-prod-vpc",
			component: "vpc",
			stack:     "uw2-prod",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-prod",
					StackSlug:     "uw2-prod-tgw-attachment",
					Environment:   "uw2",
					Stage:         "prod",
				},
			},
		},
		{
			name:      "ue1-network-tgw-hub",
			component: "tgw/hub",
			stack:     "ue1-network",
			expected: []schema.Dependent{
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-network",
					StackSlug:     "ue1-network-tgw-attachment",
					Environment:   "ue1",
					Stage:         "network",
				},
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-network",
					StackSlug:     "uw2-network-tgw-attachment",
					Environment:   "uw2",
					Stage:         "network",
				},
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "ue1-prod",
					StackSlug:     "ue1-prod-tgw-attachment",
					Environment:   "ue1",
					Stage:         "prod",
				},
				{
					Component:     "tgw/attachment",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-prod",
					StackSlug:     "uw2-prod-tgw-attachment",
					Environment:   "uw2",
					Stage:         "prod",
				},
				{
					Component:     "tgw/cross-region-hub-connector",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "uw2-network",
					StackSlug:     "uw2-network-tgw-cross-region-hub-connector",
					Environment:   "uw2",
					Stage:         "network",
				},
			},
		},
		{
			name:      "uw2-network-tgw-cross-region-hub-connector",
			component: "tgw/cross-region-hub-connector",
			stack:     "uw2-network",
			expected:  []schema.Dependent{},
		},
		{
			name:      "ue1-network-tgw-attachment",
			component: "tgw/attachment",
			stack:     "ue1-network",
			expected:  []schema.Dependent{},
		},
		{
			name:      "uw2-network-tgw-attachment",
			component: "tgw/attachment",
			stack:     "uw2-network",
			expected:  []schema.Dependent{},
		},
		{
			name:      "ue1-prod-tgw-attachment",
			component: "tgw/attachment",
			stack:     "ue1-prod",
			expected:  []schema.Dependent{},
		},
		{
			name:      "uw2-prod-tgw-attachment",
			component: "tgw/attachment",
			stack:     "uw2-prod",
			expected:  []schema.Dependent{},
		},
	}

	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExecuteDescribeDependents(&atmosConfig, &DescribeDependentsArgs{
				Component:            tc.component,
				Stack:                tc.stack,
				IncludeSettings:      false,
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
				Skip:                 nil,
				OnlyInStack:          "",
			})
			require.NoError(t, err)

			// Order-agnostic equality on struct slices
			assert.ElementsMatch(t, tc.expected, res)
		})
	}
}

// mk is a helper that builds a Dependent and derives StackSlug = Stack + "-" + Component.
func mk(stack, component, path string) schema.Dependent {
	return schema.Dependent{
		Stack:         stack,
		Component:     component,
		StackSlug:     stack + "-" + strings.ReplaceAll(component, "/", "-"),
		ComponentPath: path,
	}
}

// withChildren attaches dependent children to dependents.
func withChildren(d schema.Dependent, kids []schema.Dependent) schema.Dependent {
	d.Dependents = kids
	return d
}

func TestSortDependentsByStackSlug_BasicOrder(t *testing.T) {
	deps := []schema.Dependent{
		mk("uw2-network", "vpc", "p4"),
		mk("ue1-network", "tgw/hub", "p2"),
		mk("ue1-network", "tgw/attachment", "p1"),
		mk("uw2-network", "tgw/attachment", "p3"),
	}

	// Expected order by StackSlug
	expected := []schema.Dependent{
		mk("ue1-network", "tgw/attachment", "p1"),
		mk("ue1-network", "tgw/hub", "p2"),
		mk("uw2-network", "tgw/attachment", "p3"),
		mk("uw2-network", "vpc", "p4"),
	}

	sortDependentsByStackSlug(deps)
	require.Equal(t, expected, deps)
}

func TestSortDependentsByStackSlugRecursive_EmptyAndNil(t *testing.T) {
	// nil slice
	var nilDeps []schema.Dependent
	sortDependentsByStackSlugRecursive(nilDeps) // should not panic
	require.Nil(t, nilDeps)

	// empty slice
	empty := []schema.Dependent{}
	sortDependentsByStackSlugRecursive(empty) // should not panic
	require.Empty(t, empty)
}

func TestSortDependentsByStackSlugRecursive_BasicAndNested(t *testing.T) {
	// Unsorted tree
	deps := []schema.Dependent{
		withChildren(mk("uw2-network", "vpc", "p3"), []schema.Dependent{
			mk("uw2-network", "tgw/hub", "c2"),
			mk("uw2-network", "tgw/attachment", "c1"),
		}),
		withChildren(mk("ue1-network", "vpc", "p1"), []schema.Dependent{
			mk("ue1-network", "tgw/attachment", "a1"),
		}),
		withChildren(mk("ue1-network", "tgw/hub", "p2"), []schema.Dependent{
			mk("ue1-network", "z-comp", "g2"),
			mk("ue1-network", "a-comp", "g1"),
		}),
	}

	// Expected (sorted recursively)
	expected := []schema.Dependent{
		withChildren(mk("ue1-network", "tgw/hub", "p2"), []schema.Dependent{
			mk("ue1-network", "a-comp", "g1"),
			mk("ue1-network", "z-comp", "g2"),
		}),
		withChildren(mk("ue1-network", "vpc", "p1"), []schema.Dependent{
			mk("ue1-network", "tgw/attachment", "a1"),
		}),
		withChildren(mk("uw2-network", "vpc", "p3"), []schema.Dependent{
			mk("uw2-network", "tgw/attachment", "c1"),
			mk("uw2-network", "tgw/hub", "c2"),
		}),
	}

	sortDependentsByStackSlugRecursive(deps)
	require.Equal(t, expected, deps)
}

func TestSortDependentsByStackSlugRecursive_TieStabilityAtNestedLevel(t *testing.T) {
	// Children have identical (Stack, Component) → identical StackSlug; order should be stable.
	parent := withChildren(mk("ue1-network", "vpc", "parent"), []schema.Dependent{
		mk("ue1-network", "tgw/attachment", "first"),
		mk("ue1-network", "tgw/attachment", "second"),
	})

	deps := []schema.Dependent{parent}
	sortDependentsByStackSlugRecursive(deps)

	require.Len(t, deps, 1)
	require.Len(t, deps[0].Dependents, 2)
	require.Equal(t, "first", deps[0].Dependents[0].ComponentPath)
	require.Equal(t, "second", deps[0].Dependents[1].ComponentPath)
}

// TestDescribeDependents_DependenciesComponentsFormat tests that the new
// dependencies.components format works correctly.
// It verifies that:
// 1. Direct dependencies in dependencies.components are correctly read.
// 2. The describe dependents command correctly identifies dependents.
// 3. Component dependencies from child stacks are processed (currently replace, not append).
func TestDescribeDependents_DependenciesComponentsFormat(t *testing.T) {
	// Working directory isolation.
	workDir := "../../tests/fixtures/scenarios/dependencies-components-inheritance"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Init Atmos config.
	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, true)
	require.NoError(t, err, "InitCliConfig failed")

	// Build OS-specific expected paths.
	terraformComponentPath := filepath.Join("..", "..", "components", "terraform", "mock")
	helmfileComponentPath := filepath.Join("..", "..", "components", "helmfile", "mock")

	// Test cases for describe dependents with dependencies.components format.
	cases := []struct {
		name      string
		component string
		stack     string
		expected  []schema.Dependent
	}{
		{
			name:      "vpc has dependents including cross-type helmfile and templated stack",
			component: "vpc",
			stack:     "dev",
			expected: []schema.Dependent{
				{
					Component:     "eks",
					ComponentType: "terraform",
					ComponentPath: terraformComponentPath,
					Stack:         "dev",
					StackSlug:     "dev-eks",
					Stage:         "dev",
				},
				{
					Component:     "nginx",
					ComponentType: "helmfile",
					ComponentPath: helmfileComponentPath,
					Stack:         "dev",
					StackSlug:     "dev-nginx",
					Stage:         "dev",
				},
				{
					Component:     "rds",
					ComponentType: "terraform",
					ComponentPath: terraformComponentPath,
					Stack:         "dev",
					StackSlug:     "dev-rds",
					Stage:         "dev",
				},
				{
					Component:     "subnet",
					ComponentType: "terraform",
					ComponentPath: terraformComponentPath,
					Stack:         "dev",
					StackSlug:     "dev-subnet",
					Stage:         "dev",
				},
			},
		},
		{
			name:      "subnet has dependent rds",
			component: "subnet",
			stack:     "dev",
			expected: []schema.Dependent{
				{
					Component:     "rds",
					ComponentType: "terraform",
					ComponentPath: terraformComponentPath,
					Stack:         "dev",
					StackSlug:     "dev-rds",
					Stage:         "dev",
				},
			},
		},
		{
			name:      "rds has no dependents",
			component: "rds",
			stack:     "dev",
			expected:  []schema.Dependent{},
		},
		// NOTE: The following test cases document the CURRENT behavior (replace merge).
		// When list_merge_strategy: append is implemented for dependencies.components,
		// VPC would also depend on account-settings and iam-baseline from base.yaml.
		// Currently, child dependencies REPLACE parent dependencies for lists.
		{
			name:      "account-settings has no dependents (child replaced parent deps)",
			component: "account-settings",
			stack:     "dev",
			// With replace merge: VPC's deps from base.yaml are replaced by dev.yaml deps.
			// account-settings is not in dev.yaml's VPC dependencies, so no dependents.
			expected: []schema.Dependent{},
		},
		{
			name:      "iam-baseline has no dependents (child replaced parent deps)",
			component: "iam-baseline",
			stack:     "dev",
			// Same as above: iam-baseline is not in dev.yaml's VPC dependencies.
			expected: []schema.Dependent{},
		},
	}

	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExecuteDescribeDependents(&atmosConfig, &DescribeDependentsArgs{
				Component:            tc.component,
				Stack:                tc.stack,
				IncludeSettings:      false,
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
				Skip:                 nil,
				OnlyInStack:          "",
			})
			require.NoError(t, err)

			// Order-agnostic equality on struct slices.
			assert.ElementsMatch(t, tc.expected, res)
		})
	}
}

// TestDescribeDependents_DependenciesComponentsInheritance_WithAppendMerge tests that
// when list_merge_strategy: append is configured in atmos.yaml, dependencies.components
// uses append merge during stack processing.
// This is the EXPECTED behavior documented in the plan.
func TestDescribeDependents_DependenciesComponentsInheritance_WithAppendMerge(t *testing.T) {
	// Working directory isolation.
	workDir := "../../tests/fixtures/scenarios/dependencies-components-inheritance-append"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Init Atmos config.
	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, true)
	require.NoError(t, err, "InitCliConfig failed")

	// Verify that the list merge strategy is set to append in the config.
	require.Equal(t, "append", atmosConfig.Settings.ListMergeStrategy,
		"This test requires list_merge_strategy: append in atmos.yaml")

	// Build an OS-specific expected path once.
	componentPath := filepath.Join("..", "..", "components", "terraform", "mock")

	// With append merge, VPC should have dependencies from both base.yaml and dev.yaml.
	// This means account-settings and iam-baseline should have VPC as a dependent.
	cases := []struct {
		name      string
		component string
		stack     string
		expected  []schema.Dependent
	}{
		{
			name:      "account-settings has dependent vpc (inherited with append merge)",
			component: "account-settings",
			stack:     "dev",
			expected: []schema.Dependent{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "dev",
					StackSlug:     "dev-vpc",
					Stage:         "dev",
				},
			},
		},
		{
			name:      "iam-baseline has dependent vpc (inherited with append merge)",
			component: "iam-baseline",
			stack:     "dev",
			expected: []schema.Dependent{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					ComponentPath: componentPath,
					Stack:         "dev",
					StackSlug:     "dev-vpc",
					Stage:         "dev",
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExecuteDescribeDependents(&atmosConfig, &DescribeDependentsArgs{
				Component:            tc.component,
				Stack:                tc.stack,
				IncludeSettings:      false,
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
				Skip:                 nil,
				OnlyInStack:          "",
			})
			require.NoError(t, err)

			// Order-agnostic equality on struct slices.
			assert.ElementsMatch(t, tc.expected, res)
		})
	}
}
