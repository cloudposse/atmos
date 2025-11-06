package internal

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
)

// TestCommandProviderInterface verifies that CommandProvider interface
// includes the new methods for flag/positional args/compatibility aliases.
func TestCommandProviderInterface(t *testing.T) {
	// Use the mockCommandProvider from registry_test.go
	mock := &mockCommandProvider{
		name:  "test",
		group: "Test Commands",
		cmd:   &cobra.Command{Use: "test"},
	}

	// Verify interface methods are implemented
	assert.Equal(t, "test", mock.GetName())
	assert.Equal(t, "Test Commands", mock.GetGroup())
	assert.NotNil(t, mock.GetCommand())

	// Verify new methods return nil (default for simple commands)
	assert.Nil(t, mock.GetFlagsBuilder())
	assert.Nil(t, mock.GetPositionalArgsBuilder())
	assert.Nil(t, mock.GetCompatibilityAliases())
}

// TestCommandProviderWithFlags verifies CommandProvider with flags builder.
func TestCommandProviderWithFlags(t *testing.T) {
	// Create a mock with flags
	parser := flags.NewStandardParser(
		flags.WithBoolFlag("test", "t", false, "Test flag"),
	)

	// Create custom mock that returns non-nil flags builder
	mock := &mockProviderWithFlags{
		mockCommandProvider: mockCommandProvider{
			name:  "test-with-flags",
			group: "Test Commands",
			cmd:   &cobra.Command{Use: "test-with-flags"},
		},
		flagsBuilder: parser,
	}

	// Verify flags builder is returned
	builder := mock.GetFlagsBuilder()
	assert.NotNil(t, builder)
	assert.Equal(t, parser, builder)
}

// TestCommandProviderWithPositionalArgs verifies CommandProvider with positional args.
func TestCommandProviderWithPositionalArgs(t *testing.T) {
	// Create a mock with positional args
	posBuilder := flags.NewPositionalArgsBuilder()
	posBuilder.AddArg(&flags.PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    true,
		TargetField: "Component",
	})

	mock := &mockProviderWithPositionalArgs{
		mockCommandProvider: mockCommandProvider{
			name:  "test-with-args",
			group: "Test Commands",
			cmd:   &cobra.Command{Use: "test-with-args"},
		},
		positionalBuilder: posBuilder,
	}

	// Verify positional args builder is returned
	builder := mock.GetPositionalArgsBuilder()
	assert.NotNil(t, builder)
	assert.Equal(t, posBuilder, builder)
}

// TestCommandProviderWithCompatibilityAliases verifies CommandProvider with compatibility aliases.
func TestCommandProviderWithCompatibilityAliases(t *testing.T) {
	// Create a mock with compatibility aliases
	aliases := map[string]flags.CompatibilityAlias{
		"-var": {Behavior: flags.MapToAtmosFlag, Target: "--var"},
	}

	mock := &mockProviderWithAliases{
		mockCommandProvider: mockCommandProvider{
			name:  "test-with-aliases",
			group: "Test Commands",
			cmd:   &cobra.Command{Use: "test-with-aliases"},
		},
		compatibilityAliases: aliases,
	}

	// Verify compatibility aliases are returned
	returnedAliases := mock.GetCompatibilityAliases()
	assert.NotNil(t, returnedAliases)
	assert.Equal(t, aliases, returnedAliases)
	assert.Contains(t, returnedAliases, "-var")
	assert.Equal(t, flags.MapToAtmosFlag, returnedAliases["-var"].Behavior)
	assert.Equal(t, "--var", returnedAliases["-var"].Target)
}

// mockProviderWithFlags extends mockCommandProvider with non-nil flags builder.
type mockProviderWithFlags struct {
	mockCommandProvider
	flagsBuilder flags.Builder
}

func (m *mockProviderWithFlags) GetFlagsBuilder() flags.Builder {
	return m.flagsBuilder
}

// mockProviderWithPositionalArgs extends mockCommandProvider with non-nil positional args builder.
type mockProviderWithPositionalArgs struct {
	mockCommandProvider
	positionalBuilder *flags.PositionalArgsBuilder
}

func (m *mockProviderWithPositionalArgs) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return m.positionalBuilder
}

// mockProviderWithAliases extends mockCommandProvider with non-nil compatibility aliases.
type mockProviderWithAliases struct {
	mockCommandProvider
	compatibilityAliases map[string]flags.CompatibilityAlias
}

func (m *mockProviderWithAliases) GetCompatibilityAliases() map[string]flags.CompatibilityAlias {
	return m.compatibilityAliases
}
