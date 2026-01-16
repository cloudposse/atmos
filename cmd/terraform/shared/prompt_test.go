package shared

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHandlePromptError(t *testing.T) {
	// Save original OsExit and restore it after tests.
	originalOsExit := errUtils.OsExit
	defer func() {
		errUtils.OsExit = originalOsExit
	}()

	tests := []struct {
		name             string
		err              error
		promptName       string
		expectExit       bool
		expectedExitCode int
		expectedReturn   error
	}{
		{
			name:           "nil error returns nil",
			err:            nil,
			promptName:     "component",
			expectExit:     false,
			expectedReturn: nil,
		},
		{
			name:           "ErrInteractiveModeNotAvailable returns nil",
			err:            errUtils.ErrInteractiveModeNotAvailable,
			promptName:     "stack",
			expectExit:     false,
			expectedReturn: nil,
		},
		{
			name:           "generic error returns the error",
			err:            errors.New("some error"),
			promptName:     "component",
			expectExit:     false,
			expectedReturn: errors.New("some error"),
		},
		{
			name:             "ErrUserAborted triggers exit with SIGINT code",
			err:              errUtils.ErrUserAborted,
			promptName:       "component",
			expectExit:       true,
			expectedExitCode: errUtils.ExitCodeSIGINT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var exitCalled bool
			var exitCode int
			errUtils.OsExit = func(code int) {
				exitCalled = true
				exitCode = code
			}

			result := HandlePromptError(tt.err, tt.promptName)

			if tt.expectExit {
				assert.True(t, exitCalled, "OsExit should be called")
				assert.Equal(t, tt.expectedExitCode, exitCode, "Exit code should match")
			} else {
				assert.False(t, exitCalled, "OsExit should not be called")
				if tt.expectedReturn == nil {
					assert.NoError(t, result)
				} else {
					assert.Error(t, result)
					assert.Equal(t, tt.expectedReturn.Error(), result.Error())
				}
			}
		})
	}
}

func TestHandlePromptError_WrappedErrors(t *testing.T) {
	// Save original OsExit and restore it after tests.
	originalOsExit := errUtils.OsExit
	defer func() {
		errUtils.OsExit = originalOsExit
	}()

	t.Run("wrapped ErrUserAborted triggers exit", func(t *testing.T) {
		var exitCalled bool
		var exitCode int
		errUtils.OsExit = func(code int) {
			exitCalled = true
			exitCode = code
		}

		// Wrap the error.
		wrappedErr := errUtils.Build(errUtils.ErrUserAborted).WithExplanation("user cancelled").Err()

		HandlePromptError(wrappedErr, "test")

		assert.True(t, exitCalled, "OsExit should be called for wrapped ErrUserAborted")
		assert.Equal(t, errUtils.ExitCodeSIGINT, exitCode)
	})

	t.Run("wrapped ErrInteractiveModeNotAvailable returns nil", func(t *testing.T) {
		var exitCalled bool
		errUtils.OsExit = func(code int) {
			exitCalled = true
		}

		// Wrap the error.
		wrappedErr := errUtils.Build(errUtils.ErrInteractiveModeNotAvailable).WithExplanation("no TTY").Err()

		result := HandlePromptError(wrappedErr, "test")

		assert.False(t, exitCalled, "OsExit should not be called")
		assert.NoError(t, result, "Should return nil for wrapped ErrInteractiveModeNotAvailable")
	})
}

func TestStackContainsComponent(t *testing.T) {
	tests := []struct {
		name      string
		stackData any
		component string
		expected  bool
	}{
		{
			name:      "nil stack data",
			stackData: nil,
			component: "vpc",
			expected:  false,
		},
		{
			name:      "invalid stack data type (string)",
			stackData: "invalid",
			component: "vpc",
			expected:  false,
		},
		{
			name:      "invalid stack data type (int)",
			stackData: 123,
			component: "vpc",
			expected:  false,
		},
		{
			name:      "stack without components key",
			stackData: map[string]any{"other": "value"},
			component: "vpc",
			expected:  false,
		},
		{
			name: "invalid components type",
			stackData: map[string]any{
				"components": "invalid",
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "components without terraform key",
			stackData: map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "invalid terraform type",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": "invalid",
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component not found",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"eks":    map[string]any{},
						"rds":    map[string]any{},
						"aurora": map[string]any{},
					},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component found",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{"var1": "value1"},
						"eks": map[string]any{"var2": "value2"},
					},
				},
			},
			component: "vpc",
			expected:  true,
		},
		{
			name: "component found among many",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"component1": map[string]any{},
						"component2": map[string]any{},
						"target":     map[string]any{},
						"component3": map[string]any{},
					},
				},
			},
			component: "target",
			expected:  true,
		},
		{
			name: "empty terraform map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component with empty value",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": nil,
					},
				},
			},
			component: "vpc",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stackContainsComponent(tt.stackData, tt.component)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComponentsArgCompletion(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		toComplete        string
		expectedDirective cobra.ShellCompDirective
	}{
		{
			name:              "with args returns no completions",
			args:              []string{"existing-component"},
			toComplete:        "",
			expectedDirective: cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name:              "with multiple args returns no completions",
			args:              []string{"comp1", "comp2"},
			toComplete:        "c",
			expectedDirective: cobra.ShellCompDirectiveNoFileComp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			completions, directive := ComponentsArgCompletion(cmd, tt.args, tt.toComplete)

			// When args are provided, should return nil completions.
			assert.Nil(t, completions)
			assert.Equal(t, tt.expectedDirective, directive)
		})
	}
}

func TestStackFlagCompletion_ArgsHandling(t *testing.T) {
	// This tests the branching logic based on args, not the actual completion values
	// since those require config initialization.

	tests := []struct {
		name              string
		args              []string
		expectedDirective cobra.ShellCompDirective
	}{
		{
			name:              "no args - lists all stacks",
			args:              []string{},
			expectedDirective: cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name:              "empty string arg - lists all stacks",
			args:              []string{""},
			expectedDirective: cobra.ShellCompDirectiveNoFileComp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			_, directive := StackFlagCompletion(cmd, tt.args, "")

			// Always returns NoFileComp directive.
			assert.Equal(t, tt.expectedDirective, directive)
		})
	}
}

// TestIsComponentDeployable tests the helper that checks if a component can be deployed.
func TestIsComponentDeployable(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig any
		expected        bool
	}{
		{
			name:            "nil config is deployable",
			componentConfig: nil,
			expected:        true,
		},
		{
			name:            "non-map config is deployable",
			componentConfig: "string",
			expected:        true,
		},
		{
			name:            "empty map is deployable",
			componentConfig: map[string]any{},
			expected:        true,
		},
		{
			name: "component without metadata is deployable",
			componentConfig: map[string]any{
				"vars": map[string]any{"foo": "bar"},
			},
			expected: true,
		},
		{
			name: "component with invalid metadata type is deployable",
			componentConfig: map[string]any{
				"metadata": "invalid",
			},
			expected: true,
		},
		{
			name: "real component is deployable",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"type": "real",
				},
			},
			expected: true,
		},
		{
			name: "abstract component is not deployable",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"type": "abstract",
				},
			},
			expected: false,
		},
		{
			name: "enabled component is deployable",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name: "disabled component is not deployable",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"enabled": false,
				},
			},
			expected: false,
		},
		{
			name: "abstract and disabled component is not deployable",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"type":    "abstract",
					"enabled": false,
				},
			},
			expected: false,
		},
		{
			name: "real and enabled component is deployable",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"type":    "real",
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name: "real but disabled component is not deployable",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"type":    "real",
					"enabled": false,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isComponentDeployable(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFilterDeployableComponents tests filtering a map of components to only deployable ones.
func TestFilterDeployableComponents(t *testing.T) {
	tests := []struct {
		name       string
		components map[string]any
		expected   []string
	}{
		{
			name:       "nil map returns empty slice",
			components: nil,
			expected:   []string{},
		},
		{
			name:       "empty map returns empty slice",
			components: map[string]any{},
			expected:   []string{},
		},
		{
			name: "all real components are returned",
			components: map[string]any{
				"vpc": map[string]any{
					"metadata": map[string]any{"type": "real"},
				},
				"eks": map[string]any{
					"metadata": map[string]any{"type": "real"},
				},
			},
			expected: []string{"eks", "vpc"},
		},
		{
			name: "abstract components are filtered out",
			components: map[string]any{
				"vpc": map[string]any{
					"metadata": map[string]any{"type": "real"},
				},
				"base-vpc": map[string]any{
					"metadata": map[string]any{"type": "abstract"},
				},
			},
			expected: []string{"vpc"},
		},
		{
			name: "disabled components are filtered out",
			components: map[string]any{
				"vpc": map[string]any{
					"metadata": map[string]any{"enabled": true},
				},
				"old-vpc": map[string]any{
					"metadata": map[string]any{"enabled": false},
				},
			},
			expected: []string{"vpc"},
		},
		{
			name: "mixed filtering",
			components: map[string]any{
				"vpc": map[string]any{
					"metadata": map[string]any{"type": "real", "enabled": true},
				},
				"base-vpc": map[string]any{
					"metadata": map[string]any{"type": "abstract"},
				},
				"disabled-vpc": map[string]any{
					"metadata": map[string]any{"enabled": false},
				},
				"eks": map[string]any{}, // No metadata - should be deployable.
			},
			expected: []string{"eks", "vpc"},
		},
		{
			name: "components without metadata are deployable",
			components: map[string]any{
				"simple-component": map[string]any{
					"vars": map[string]any{"foo": "bar"},
				},
			},
			expected: []string{"simple-component"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterDeployableComponents(tt.components)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockSetup holds the common mock configuration for tests.
type mockSetup struct {
	configError error
	stacksError error
	stacksMap   map[string]any
}

// setupMocksWithCleanup sets up the mocks with the given configuration and returns a cleanup function.
func setupMocksWithCleanup(t *testing.T) (func(ms mockSetup), func()) {
	t.Helper()
	originalInitCliConfig := initCliConfig
	originalExecuteDescribeStacks := executeDescribeStacks

	setMocks := func(ms mockSetup) {
		initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			if ms.configError != nil {
				return schema.AtmosConfiguration{}, ms.configError
			}
			return schema.AtmosConfiguration{}, nil
		}

		executeDescribeStacks = func(_ *schema.AtmosConfiguration, _ string, _ []string, _ []string, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
			if ms.stacksError != nil {
				return nil, ms.stacksError
			}
			return ms.stacksMap, nil
		}
	}

	cleanup := func() {
		initCliConfig = originalInitCliConfig
		executeDescribeStacks = originalExecuteDescribeStacks
	}

	return setMocks, cleanup
}

// TestListTerraformComponents tests the listTerraformComponents function.
func TestListTerraformComponents(t *testing.T) {
	setMocks, cleanup := setupMocksWithCleanup(t)
	defer cleanup()

	tests := []struct {
		name               string
		mockConfigError    error
		mockStacksError    error
		mockStacksMap      map[string]any
		expectedComponents []string
		expectedError      bool
	}{
		{
			name:            "success with multiple components across stacks",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
							"eks": map[string]any{},
						},
					},
				},
				"prod-us-west-2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc":    map[string]any{},
							"aurora": map[string]any{},
						},
					},
				},
			},
			expectedComponents: []string{"aurora", "eks", "vpc"},
			expectedError:      false,
		},
		{
			name:               "returns error when config init fails",
			mockConfigError:    errors.New("config init failed"),
			mockStacksError:    nil,
			mockStacksMap:      nil,
			expectedComponents: nil,
			expectedError:      true,
		},
		{
			name:               "returns error when describe stacks fails",
			mockConfigError:    nil,
			mockStacksError:    errors.New("describe stacks failed"),
			mockStacksMap:      nil,
			expectedComponents: nil,
			expectedError:      true,
		},
		{
			name:               "returns empty slice for empty stacks",
			mockConfigError:    nil,
			mockStacksError:    nil,
			mockStacksMap:      map[string]any{},
			expectedComponents: []string{},
			expectedError:      false,
		},
		{
			name:            "filters out abstract components",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{"type": "real"},
							},
							"base-component": map[string]any{
								"metadata": map[string]any{"type": "abstract"},
							},
						},
					},
				},
			},
			expectedComponents: []string{"vpc"},
			expectedError:      false,
		},
		{
			name:            "deduplicates components across stacks",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"stack1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
				"stack2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
				"stack3": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
			},
			expectedComponents: []string{"vpc"},
			expectedError:      false,
		},
		{
			name:            "handles stacks without terraform components",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{
							"chart1": map[string]any{},
						},
					},
				},
			},
			expectedComponents: []string{},
			expectedError:      false,
		},
		{
			name:            "handles invalid stack data type",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": "invalid-stack-data",
			},
			expectedComponents: []string{},
			expectedError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMocks(mockSetup{
				configError: tt.mockConfigError,
				stacksError: tt.mockStacksError,
				stacksMap:   tt.mockStacksMap,
			})

			result, err := listTerraformComponents()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedComponents, result)
			}
		})
	}
}

// TestListTerraformComponentsForStack tests the listTerraformComponentsForStack function.
func TestListTerraformComponentsForStack(t *testing.T) {
	setMocks, cleanup := setupMocksWithCleanup(t)
	defer cleanup()

	tests := []struct {
		name               string
		stack              string
		mockConfigError    error
		mockStacksError    error
		mockStacksMap      map[string]any
		expectedComponents []string
		expectedError      bool
	}{
		{
			name:            "success with specific stack",
			stack:           "dev-us-east-1",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
							"eks": map[string]any{},
						},
					},
				},
			},
			expectedComponents: []string{"eks", "vpc"},
			expectedError:      false,
		},
		{
			name:            "returns empty when stack not found",
			stack:           "nonexistent",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
			},
			expectedComponents: []string{},
			expectedError:      false,
		},
		{
			name:               "returns error when config init fails",
			stack:              "dev",
			mockConfigError:    errors.New("config init failed"),
			mockStacksError:    nil,
			mockStacksMap:      nil,
			expectedComponents: nil,
			expectedError:      true,
		},
		{
			name:               "returns error when describe stacks fails",
			stack:              "dev",
			mockConfigError:    nil,
			mockStacksError:    errors.New("describe stacks failed"),
			mockStacksMap:      nil,
			expectedComponents: nil,
			expectedError:      true,
		},
		{
			name:            "handles invalid stack data type",
			stack:           "dev",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": "invalid-data",
			},
			expectedComponents: []string{},
			expectedError:      false,
		},
		{
			name:            "handles stack without components key",
			stack:           "dev",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"settings": map[string]any{},
				},
			},
			expectedComponents: []string{},
			expectedError:      false,
		},
		{
			name:            "handles stack without terraform components",
			stack:           "dev",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{},
					},
				},
			},
			expectedComponents: []string{},
			expectedError:      false,
		},
		{
			name:            "filters out abstract components",
			stack:           "dev",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
							"base-vpc": map[string]any{
								"metadata": map[string]any{"type": "abstract"},
							},
						},
					},
				},
			},
			expectedComponents: []string{"vpc"},
			expectedError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMocks(mockSetup{
				configError: tt.mockConfigError,
				stacksError: tt.mockStacksError,
				stacksMap:   tt.mockStacksMap,
			})

			result, err := listTerraformComponentsForStack(tt.stack)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedComponents, result)
			}
		})
	}
}

// TestListTerraformComponentsForStack_EmptyStackDelegation tests that empty stack delegates to listTerraformComponents.
func TestListTerraformComponentsForStack_EmptyStackDelegation(t *testing.T) {
	// This test needs custom mock setup to track calls, so we keep it separate.
	originalInitCliConfig := initCliConfig
	originalExecuteDescribeStacks := executeDescribeStacks
	defer func() {
		initCliConfig = originalInitCliConfig
		executeDescribeStacks = originalExecuteDescribeStacks
	}()

	describeStacksCalled := false
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	executeDescribeStacks = func(_ *schema.AtmosConfiguration, filterStack string, _ []string, _ []string, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		describeStacksCalled = true
		// When stack is empty, it should call listTerraformComponents which passes empty string.
		assert.Equal(t, "", filterStack)
		return map[string]any{
			"stack1": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{},
					},
				},
			},
		}, nil
	}

	result, err := listTerraformComponentsForStack("")

	assert.NoError(t, err)
	assert.True(t, describeStacksCalled)
	assert.Equal(t, []string{"vpc"}, result)
}

// TestListStacksForComponent tests the listStacksForComponent function.
func TestListStacksForComponent(t *testing.T) {
	setMocks, cleanup := setupMocksWithCleanup(t)
	defer cleanup()

	tests := []struct {
		name            string
		component       string
		mockConfigError error
		mockStacksError error
		mockStacksMap   map[string]any
		expectedStacks  []string
		expectedError   bool
	}{
		{
			name:            "success with matching stacks",
			component:       "vpc",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
							"eks": map[string]any{},
						},
					},
				},
				"prod-us-west-2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
				"staging": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"rds": map[string]any{},
						},
					},
				},
			},
			expectedStacks: []string{"dev-us-east-1", "prod-us-west-2"},
			expectedError:  false,
		},
		{
			name:            "no matching stacks",
			component:       "nonexistent",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
			},
			expectedStacks: nil,
			expectedError:  false,
		},
		{
			name:            "returns error when config init fails",
			component:       "vpc",
			mockConfigError: errors.New("config init failed"),
			mockStacksError: nil,
			mockStacksMap:   nil,
			expectedStacks:  nil,
			expectedError:   true,
		},
		{
			name:            "returns error when describe stacks fails",
			component:       "vpc",
			mockConfigError: nil,
			mockStacksError: errors.New("describe stacks failed"),
			mockStacksMap:   nil,
			expectedStacks:  nil,
			expectedError:   true,
		},
		{
			name:            "returns empty for empty stacks map",
			component:       "vpc",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap:   map[string]any{},
			expectedStacks:  nil,
			expectedError:   false,
		},
		{
			name:            "results are sorted alphabetically",
			component:       "vpc",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"z-stack": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{"vpc": map[string]any{}},
					},
				},
				"a-stack": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{"vpc": map[string]any{}},
					},
				},
				"m-stack": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{"vpc": map[string]any{}},
					},
				},
			},
			expectedStacks: []string{"a-stack", "m-stack", "z-stack"},
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMocks(mockSetup{
				configError: tt.mockConfigError,
				stacksError: tt.mockStacksError,
				stacksMap:   tt.mockStacksMap,
			})

			result, err := listStacksForComponent(tt.component)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStacks, result)
			}
		})
	}
}

// TestListAllStacks tests the listAllStacks function.
func TestListAllStacks(t *testing.T) {
	setMocks, cleanup := setupMocksWithCleanup(t)
	defer cleanup()

	tests := []struct {
		name            string
		mockConfigError error
		mockStacksError error
		mockStacksMap   map[string]any
		expectedStacks  []string
		expectedError   bool
	}{
		{
			name:            "success with multiple stacks",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev-us-east-1":  map[string]any{},
				"prod-us-west-2": map[string]any{},
				"staging":        map[string]any{},
			},
			expectedStacks: []string{"dev-us-east-1", "prod-us-west-2", "staging"},
			expectedError:  false,
		},
		{
			name:            "returns error when config init fails",
			mockConfigError: errors.New("config init failed"),
			mockStacksError: nil,
			mockStacksMap:   nil,
			expectedStacks:  nil,
			expectedError:   true,
		},
		{
			name:            "returns error when describe stacks fails",
			mockConfigError: nil,
			mockStacksError: errors.New("describe stacks failed"),
			mockStacksMap:   nil,
			expectedStacks:  nil,
			expectedError:   true,
		},
		{
			name:            "returns empty for empty stacks map",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap:   map[string]any{},
			expectedStacks:  []string{},
			expectedError:   false,
		},
		{
			name:            "results are sorted alphabetically",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"z-stack": map[string]any{},
				"a-stack": map[string]any{},
				"m-stack": map[string]any{},
			},
			expectedStacks: []string{"a-stack", "m-stack", "z-stack"},
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMocks(mockSetup{
				configError: tt.mockConfigError,
				stacksError: tt.mockStacksError,
				stacksMap:   tt.mockStacksMap,
			})

			result, err := listAllStacks()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStacks, result)
			}
		})
	}
}

// TestComponentsArgCompletionWithStack tests the componentsArgCompletionWithStack function.
func TestComponentsArgCompletionWithStack(t *testing.T) {
	setMocks, cleanup := setupMocksWithCleanup(t)
	defer cleanup()

	tests := []struct {
		name               string
		args               []string
		stack              string
		mockConfigError    error
		mockStacksError    error
		mockStacksMap      map[string]any
		expectedComponents []string
		expectedDirective  cobra.ShellCompDirective
	}{
		{
			name:            "with stack filter returns filtered components",
			args:            []string{},
			stack:           "dev",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
							"eks": map[string]any{},
						},
					},
				},
			},
			expectedComponents: []string{"eks", "vpc"},
			expectedDirective:  cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name:            "without stack filter returns all components",
			args:            []string{},
			stack:           "",
			mockConfigError: nil,
			mockStacksError: nil,
			mockStacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"rds": map[string]any{},
						},
					},
				},
			},
			expectedComponents: []string{"rds", "vpc"},
			expectedDirective:  cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name:               "with args returns nil",
			args:               []string{"existing-component"},
			stack:              "",
			mockConfigError:    nil,
			mockStacksError:    nil,
			mockStacksMap:      nil,
			expectedComponents: nil,
			expectedDirective:  cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name:               "returns NoFileComp on error",
			args:               []string{},
			stack:              "",
			mockConfigError:    errors.New("config error"),
			mockStacksError:    nil,
			mockStacksMap:      nil,
			expectedComponents: nil,
			expectedDirective:  cobra.ShellCompDirectiveNoFileComp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMocks(mockSetup{
				configError: tt.mockConfigError,
				stacksError: tt.mockStacksError,
				stacksMap:   tt.mockStacksMap,
			})

			cmd := &cobra.Command{Use: "test"}
			result, directive := componentsArgCompletionWithStack(cmd, tt.args, "", tt.stack)

			assert.Equal(t, tt.expectedComponents, result)
			assert.Equal(t, tt.expectedDirective, directive)
		})
	}
}

// Ensure cfg import is used.
var _ = cfg.InitCliConfig
