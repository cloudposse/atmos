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
