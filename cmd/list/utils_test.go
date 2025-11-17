package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
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
