package list

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	f "github.com/cloudposse/atmos/pkg/list/format"
)

// TestNewCommonListParser tests the newCommonListParser helper function.
func TestNewCommonListParser(t *testing.T) {
	testCases := []struct {
		name              string
		additionalOptions []flags.Option
	}{
		{
			name:              "parser with no additional options",
			additionalOptions: nil,
		},
		{
			name: "parser with one additional option",
			additionalOptions: []flags.Option{
				flags.WithBoolFlag("test-flag", "", false, "Test flag"),
			},
		},
		{
			name: "parser with multiple additional options",
			additionalOptions: []flags.Option{
				flags.WithBoolFlag("abstract", "", false, "Include abstract components"),
				flags.WithBoolFlag("vars", "", false, "Show only vars"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify function doesn't panic and returns a valid parser
			assert.NotPanics(t, func() {
				parser := newCommonListParser(tc.additionalOptions...)
				assert.NotNil(t, parser, "Parser should not be nil")

				// Register flags on a test command to verify parser works
				cmd := &cobra.Command{Use: "test"}
				assert.NotPanics(t, func() {
					parser.RegisterFlags(cmd)
				}, "RegisterFlags should not panic")
			})
		})
	}
}

// TestNewCommonListParser_CreatesValidParser tests that the parser is usable.
func TestNewCommonListParser_CreatesValidParser(t *testing.T) {
	parser := newCommonListParser()
	assert.NotNil(t, parser, "Parser should not be nil")

	cmd := &cobra.Command{Use: "test"}
	assert.NotPanics(t, func() {
		parser.RegisterFlags(cmd)
	}, "RegisterFlags should work on a fresh command")
}

// TestAddStackCompletion tests the addStackCompletion function.
func TestAddStackCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// Test that adding stack completion doesn't panic
	assert.NotPanics(t, func() {
		addStackCompletion(cmd)
	})

	// Verify stack flag was added
	stackFlag := cmd.PersistentFlags().Lookup("stack")
	assert.NotNil(t, stackFlag, "Should add stack flag if not present")
	assert.Equal(t, "s", stackFlag.Shorthand)
	assert.Equal(t, "", stackFlag.DefValue)
}

// TestAddStackCompletion_ExistingFlag tests that adding stack completion to a command with existing stack flag doesn't break.
func TestAddStackCompletion_ExistingFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// Pre-add stack flag
	cmd.PersistentFlags().StringP("stack", "s", "", "Stack filter")

	// Adding stack completion should not panic even if flag exists
	assert.NotPanics(t, func() {
		addStackCompletion(cmd)
	})

	// Verify flag still exists
	stackFlag := cmd.PersistentFlags().Lookup("stack")
	assert.NotNil(t, stackFlag)
}

// TestGetIdentityFromCommand tests the getIdentityFromCommand function.
func TestGetIdentityFromCommand(t *testing.T) {
	testCases := []struct {
		name           string
		setupCmd       func() *cobra.Command
		setupViper     func()
		expectedResult string
	}{
		{
			name: "returns empty when no identity set",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
			},
			expectedResult: "",
		},
		{
			name: "returns flag value when flag is changed",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity")
				_ = cmd.Flags().Set("identity", "my-identity")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
			},
			expectedResult: "my-identity",
		},
		{
			name: "returns viper value when flag not changed",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
				viper.Set("identity", "env-identity")
			},
			expectedResult: "env-identity",
		},
		{
			name: "flag takes precedence over viper",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity")
				_ = cmd.Flags().Set("identity", "flag-identity")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
				viper.Set("identity", "env-identity")
			},
			expectedResult: "flag-identity",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupViper()
			cmd := tc.setupCmd()
			result := getIdentityFromCommand(cmd)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

// TestSetDefaultCSVDelimiter tests the setDefaultCSVDelimiter function.
func TestSetDefaultCSVDelimiter(t *testing.T) {
	testCases := []struct {
		name              string
		initialDelimiter  string
		format            string
		expectedDelimiter string
	}{
		{
			name:              "CSV format with default TSV delimiter changes to comma",
			initialDelimiter:  f.DefaultTSVDelimiter,
			format:            string(f.FormatCSV),
			expectedDelimiter: f.DefaultCSVDelimiter,
		},
		{
			name:              "CSV format with custom delimiter stays unchanged",
			initialDelimiter:  "|",
			format:            string(f.FormatCSV),
			expectedDelimiter: "|",
		},
		{
			name:              "TSV format with default TSV delimiter stays unchanged",
			initialDelimiter:  f.DefaultTSVDelimiter,
			format:            string(f.FormatTSV),
			expectedDelimiter: f.DefaultTSVDelimiter,
		},
		{
			name:              "JSON format with default TSV delimiter stays unchanged",
			initialDelimiter:  f.DefaultTSVDelimiter,
			format:            string(f.FormatJSON),
			expectedDelimiter: f.DefaultTSVDelimiter,
		},
		{
			name:              "empty format with default TSV delimiter stays unchanged",
			initialDelimiter:  f.DefaultTSVDelimiter,
			format:            "",
			expectedDelimiter: f.DefaultTSVDelimiter,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			delimiter := tc.initialDelimiter
			setDefaultCSVDelimiter(&delimiter, tc.format)
			assert.Equal(t, tc.expectedDelimiter, delimiter)
		})
	}
}

// TestGetComponentFilter tests the getComponentFilter function.
func TestGetComponentFilter(t *testing.T) {
	testCases := []struct {
		name           string
		args           []string
		expectedResult string
	}{
		{
			name:           "empty args returns empty string",
			args:           []string{},
			expectedResult: "",
		},
		{
			name:           "nil args returns empty string",
			args:           nil,
			expectedResult: "",
		},
		{
			name:           "single arg returns first arg",
			args:           []string{"component1"},
			expectedResult: "component1",
		},
		{
			name:           "multiple args returns first arg",
			args:           []string{"component1", "component2", "component3"},
			expectedResult: "component1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getComponentFilter(tc.args)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

// TestValidateComponentFilter tests the validateComponentFilter function.
func TestValidateComponentFilter(t *testing.T) {
	testCases := []struct {
		name            string
		componentFilter string
		expectError     bool
	}{
		{
			name:            "empty filter returns no error",
			componentFilter: "",
			expectError:     false,
		},
		// Note: Testing with actual component requires a valid atmosConfig setup
		// which would be an integration test. Unit tests focus on the empty case.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// For the empty filter case, we don't need a real config
			if tc.componentFilter == "" {
				err := validateComponentFilter(nil, tc.componentFilter)
				if tc.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}

// TestStackFlagCompletion tests the stackFlagCompletion function behavior.
func TestStackFlagCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// Test with empty args - this will try to list all stacks
	// In unit test context without valid config, it should return ShellCompDirectiveNoFileComp
	_, directive := stackFlagCompletion(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	// Test with component arg - this will try to list stacks for component
	// In unit test context without valid config, it should return ShellCompDirectiveNoFileComp
	_, directive = stackFlagCompletion(cmd, []string{"mycomponent"}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestNewCommonListParser_FlagsRegistered tests that expected flags are registered.
func TestNewCommonListParser_FlagsRegistered(t *testing.T) {
	parser := newCommonListParser()
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify expected flags exist
	expectedFlags := []string{"format", "max-columns", "delimiter", "stack", "query"}
	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "Expected flag %s to be registered", flagName)
	}
}

// TestNewCommonListParser_WithAdditionalFlags tests parser with additional flags.
func TestNewCommonListParser_WithAdditionalFlags(t *testing.T) {
	parser := newCommonListParser(
		flags.WithBoolFlag("upload", "", false, "Upload flag"),
		flags.WithStringFlag("custom", "c", "default", "Custom flag"),
	)
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify additional flags exist
	uploadFlag := cmd.Flags().Lookup("upload")
	assert.NotNil(t, uploadFlag, "Expected upload flag to be registered")
	assert.Equal(t, "false", uploadFlag.DefValue)

	customFlag := cmd.Flags().Lookup("custom")
	assert.NotNil(t, customFlag, "Expected custom flag to be registered")
	assert.Equal(t, "default", customFlag.DefValue)
	assert.Equal(t, "c", customFlag.Shorthand)
}

// TestHandleNoValuesError tests the handleNoValuesError function.
func TestHandleNoValuesError(t *testing.T) {
	testCases := []struct {
		name                string
		err                 error
		componentFilter     string
		expectedOutput      string
		expectedError       error
		expectLogFuncCalled bool
	}{
		{
			name:                "NoValuesFoundError calls logFunc and returns empty",
			err:                 &listerrors.NoValuesFoundError{},
			componentFilter:     "test-component",
			expectedOutput:      "",
			expectedError:       nil,
			expectLogFuncCalled: true,
		},
		{
			name:                "other error returns the error unchanged",
			err:                 errors.New("some other error"),
			componentFilter:     "test-component",
			expectedOutput:      "",
			expectedError:       errors.New("some other error"),
			expectLogFuncCalled: false,
		},
		{
			name:                "wrapped NoValuesFoundError calls logFunc",
			err:                 errors.Join(errors.New("wrapper"), &listerrors.NoValuesFoundError{}),
			componentFilter:     "wrapped-component",
			expectedOutput:      "",
			expectedError:       nil,
			expectLogFuncCalled: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logFuncCalled := false
			logFuncComponent := ""
			logFunc := func(component string) {
				logFuncCalled = true
				logFuncComponent = component
			}

			output, err := handleNoValuesError(tc.err, tc.componentFilter, logFunc)

			assert.Equal(t, tc.expectedOutput, output)
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectLogFuncCalled, logFuncCalled)
			if tc.expectLogFuncCalled {
				assert.Equal(t, tc.componentFilter, logFuncComponent)
			}
		})
	}
}
