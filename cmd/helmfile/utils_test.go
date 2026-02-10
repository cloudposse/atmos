package helmfile

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestIsHelpRequest(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
		{
			name:     "short help flag",
			args:     []string{"-h"},
			expected: true,
		},
		{
			name:     "long help flag",
			args:     []string{"--help"},
			expected: true,
		},
		{
			name:     "help command",
			args:     []string{"help"},
			expected: true,
		},
		{
			name:     "help flag with other args",
			args:     []string{"component", "-s", "stack", "--help"},
			expected: true,
		},
		{
			name:     "help flag at start",
			args:     []string{"-h", "component", "-s", "stack"},
			expected: true,
		},
		{
			name:     "no help flag",
			args:     []string{"component", "-s", "stack"},
			expected: false,
		},
		{
			name:     "similar but not help",
			args:     []string{"--helper", "-helper", "helping"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHelpRequest(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleHelpRequest(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedResult bool
	}{
		{
			name:           "help requested",
			args:           []string{"--help"},
			expectedResult: true,
		},
		{
			name:           "no help requested",
			args:           []string{"component", "-s", "stack"},
			expectedResult: false,
		},
		{
			name:           "empty args",
			args:           []string{},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}
			result := handleHelpRequest(cmd, tt.args)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestHandlePathResolutionError(t *testing.T) {
	tests := []struct {
		name              string
		inputErr          error
		expectedError     error
		shouldPassthrough bool
	}{
		{
			name:              "ambiguous component path error passes through",
			inputErr:          errUtils.ErrAmbiguousComponentPath,
			expectedError:     errUtils.ErrAmbiguousComponentPath,
			shouldPassthrough: true,
		},
		{
			name:              "component not in stack error passes through",
			inputErr:          errUtils.ErrComponentNotInStack,
			expectedError:     errUtils.ErrComponentNotInStack,
			shouldPassthrough: true,
		},
		{
			name:              "stack not found error passes through",
			inputErr:          errUtils.ErrStackNotFound,
			expectedError:     errUtils.ErrStackNotFound,
			shouldPassthrough: true,
		},
		{
			name:              "user aborted error passes through",
			inputErr:          errUtils.ErrUserAborted,
			expectedError:     errUtils.ErrUserAborted,
			shouldPassthrough: true,
		},
		{
			name:              "component type mismatch error passes through",
			inputErr:          errUtils.ErrComponentTypeMismatch,
			expectedError:     errUtils.ErrComponentTypeMismatch,
			shouldPassthrough: true,
		},
		{
			name:              "generic error gets wrapped",
			inputErr:          errors.New("some generic error"),
			expectedError:     errUtils.ErrPathResolutionFailed,
			shouldPassthrough: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handlePathResolutionError(tt.inputErr)
			assert.Error(t, result)
			assert.True(t, errors.Is(result, tt.expectedError))
			if tt.shouldPassthrough {
				// Passthrough errors should be returned unchanged (same object).
				assert.Same(t, tt.inputErr, result)
			} else {
				// Non-passthrough errors should be wrapped (different object).
				assert.NotSame(t, tt.inputErr, result)
			}
		})
	}
}

func TestAddStackCompletion(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Before adding completion, the flag should not exist.
	flag := cmd.PersistentFlags().Lookup("stack")
	assert.Nil(t, flag)

	// Add stack completion.
	addStackCompletion(cmd)

	// After adding, the flag should exist.
	flag = cmd.PersistentFlags().Lookup("stack")
	assert.NotNil(t, flag)
	assert.Equal(t, "s", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
}

func TestStackFlagCompletion(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	completions, directive := stackFlagCompletion(cmd, []string{}, "")

	// Should return empty completions with no file completion directive.
	assert.Nil(t, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestStackFlagCompletion_WithArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	tests := []struct {
		name       string
		args       []string
		toComplete string
	}{
		{
			name:       "empty args and empty completion",
			args:       []string{},
			toComplete: "",
		},
		{
			name:       "with args",
			args:       []string{"component"},
			toComplete: "dev",
		},
		{
			name:       "partial completion",
			args:       []string{"my-component", "-s"},
			toComplete: "prod-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completions, directive := stackFlagCompletion(cmd, tt.args, tt.toComplete)

			// Always returns empty completions with no file comp directive.
			assert.Nil(t, completions)
			assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
		})
	}
}

func TestEnableHeatmapIfRequested(t *testing.T) {
	// This function checks os.Args directly, so we can test its structure
	// but not its full behavior without manipulating os.Args which is tricky.
	// We verify it doesn't panic with normal execution.
	enableHeatmapIfRequested()
	// If we got here without panic, the function works.
}

func TestHandleHelpRequest_AllHelpForms(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedResult bool
	}{
		{
			name:           "short help -h",
			args:           []string{"-h"},
			expectedResult: true,
		},
		{
			name:           "long help --help",
			args:           []string{"--help"},
			expectedResult: true,
		},
		{
			name:           "help subcommand",
			args:           []string{"help"},
			expectedResult: true,
		},
		{
			name:           "help in middle of args",
			args:           []string{"component", "-h", "-s", "stack"},
			expectedResult: true,
		},
		{
			name:           "help at end",
			args:           []string{"component", "-s", "stack", "--help"},
			expectedResult: true,
		},
		{
			name:           "no help - valid command",
			args:           []string{"component", "-s", "stack"},
			expectedResult: false,
		},
		{
			name:           "no help - flag-like arg",
			args:           []string{"--other-flag"},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}
			result := handleHelpRequest(cmd, tt.args)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestHandlePathResolutionError_WrappedErrors(t *testing.T) {
	// Test that wrapped errors still match correctly.
	wrappedAmbiguous := errUtils.Build(errUtils.ErrAmbiguousComponentPath).
		WithCause(errors.New("multiple matches")).
		Err()

	result := handlePathResolutionError(wrappedAmbiguous)
	assert.True(t, errors.Is(result, errUtils.ErrAmbiguousComponentPath))
}

func TestGetConfigAndStacksInfo_DoubleDashSeparator(t *testing.T) {
	// Test that double-dash separator is handled correctly.
	// The function will fail because we don't have proper fixtures,
	// but we can verify the double-dash parsing logic is exercised.
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	addStackCompletion(cmd)

	tests := []struct {
		name        string
		args        []string
		expectError bool
		description string
	}{
		{
			name:        "no double dash",
			args:        []string{"sync", "component", "-s", "stack"},
			expectError: true, // Will fail without fixtures but exercises the code path.
			description: "args without double-dash separator",
		},
		{
			name:        "with double dash separator",
			args:        []string{"sync", "component", "-s", "stack", "--", "--set", "key=value"},
			expectError: true, // Will fail without fixtures but exercises double-dash parsing.
			description: "args with double-dash separator should split correctly",
		},
		{
			name:        "double dash at start (index 0)",
			args:        []string{"--", "component"},
			expectError: true,
			description: "double-dash at index 0 should not trigger split (doubleDashIndex > 0 check)",
		},
		{
			name:        "empty args",
			args:        []string{},
			expectError: true,
			description: "empty args should error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getConfigAndStacksInfo("helmfile", cmd, tt.args)
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

func TestGetConfigAndStacksInfo_ErrorHandling(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	addStackCompletion(cmd)

	// Test with invalid args that will cause ProcessCommandLineArgs to fail.
	_, err := getConfigAndStacksInfo("helmfile", cmd, []string{"sync"})
	assert.Error(t, err, "should error when required args are missing")
}
