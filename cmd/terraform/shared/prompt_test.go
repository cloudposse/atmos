package shared

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
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
