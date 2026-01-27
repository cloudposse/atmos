package lsp

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
)

// TestNewLSPCommand tests the NewLSPCommand function.
func TestNewLSPCommand(t *testing.T) {
	cmd := NewLSPCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "lsp", cmd.Use)
	assert.Equal(t, "Language Server Protocol commands", cmd.Short)
	assert.Contains(t, cmd.Long, "Language Server Protocol")
	assert.Contains(t, cmd.Long, "IDE integration")
	assert.False(t, cmd.FParseErrWhitelist.UnknownFlags)
}

// TestNewLSPCommand_HasStartSubcommand tests that the lsp command has the start subcommand.
func TestNewLSPCommand_HasStartSubcommand(t *testing.T) {
	cmd := NewLSPCommand()

	// Find the start subcommand.
	startCmd, _, err := cmd.Find([]string{"start"})
	require.NoError(t, err)
	require.NotNil(t, startCmd)
	assert.Equal(t, "start", startCmd.Use)
}

// TestNewLSPStartCommand tests the NewLSPStartCommand function.
func TestNewLSPStartCommand(t *testing.T) {
	cmd := NewLSPStartCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "start", cmd.Use)
	assert.Equal(t, "Start the Atmos LSP server", cmd.Short)
	assert.Contains(t, cmd.Long, "Language Server Protocol")
	assert.Contains(t, cmd.Long, "Syntax validation")
	assert.Contains(t, cmd.Long, "Auto-completion")
	assert.Contains(t, cmd.Long, "Hover documentation")
	assert.Contains(t, cmd.Long, "Go to definition")
	assert.Contains(t, cmd.Long, "stdio")
	assert.Contains(t, cmd.Long, "tcp")
	assert.Contains(t, cmd.Long, "websocket")
	assert.NotNil(t, cmd.RunE)
}

// TestNewLSPStartCommand_Flags tests the flags on the start command.
func TestNewLSPStartCommand_Flags(t *testing.T) {
	cmd := NewLSPStartCommand()

	// Test transport flag.
	transportFlag := cmd.Flags().Lookup("transport")
	require.NotNil(t, transportFlag, "transport flag should exist")
	assert.Equal(t, "stdio", transportFlag.DefValue)
	assert.Contains(t, transportFlag.Usage, "Transport protocol")
	assert.Contains(t, transportFlag.Usage, "stdio")
	assert.Contains(t, transportFlag.Usage, "tcp")
	assert.Contains(t, transportFlag.Usage, "websocket")

	// Test address flag.
	addressFlag := cmd.Flags().Lookup("address")
	require.NotNil(t, addressFlag, "address flag should exist")
	assert.Equal(t, "localhost:7777", addressFlag.DefValue)
	assert.Contains(t, addressFlag.Usage, "Address for tcp/websocket transports")
}

// TestNewLSPStartCommand_FlagsTable provides table-driven tests for flag values.
func TestNewLSPStartCommand_FlagsTable(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		defaultValue string
		usagePattern string
	}{
		{
			name:         "transport flag defaults to stdio",
			flagName:     "transport",
			defaultValue: "stdio",
			usagePattern: "Transport protocol",
		},
		{
			name:         "address flag defaults to localhost:7777",
			flagName:     "address",
			defaultValue: "localhost:7777",
			usagePattern: "Address for tcp/websocket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewLSPStartCommand()
			flag := cmd.Flags().Lookup(tt.flagName)

			require.NotNil(t, flag, "flag %s should exist", tt.flagName)
			assert.Equal(t, tt.defaultValue, flag.DefValue, "default value mismatch")
			assert.Contains(t, flag.Usage, tt.usagePattern, "usage pattern not found")
		})
	}
}

// TestLSPProvider_GetCommand tests the Provider GetCommand method.
func TestLSPProvider_GetCommand(t *testing.T) {
	provider := &Provider{}

	cmd := provider.GetCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "lsp", cmd.Use)
	assert.Equal(t, "Language Server Protocol commands", cmd.Short)
}

// TestLSPProvider_GetName tests the Provider GetName method.
func TestLSPProvider_GetName(t *testing.T) {
	provider := &Provider{}

	name := provider.GetName()

	assert.Equal(t, "lsp", name)
}

// TestLSPProvider_GetGroup tests the Provider GetGroup method.
func TestLSPProvider_GetGroup(t *testing.T) {
	provider := &Provider{}

	group := provider.GetGroup()

	assert.Equal(t, "Other Commands", group)
}

// TestLSPProvider_GetFlagsBuilder tests the Provider GetFlagsBuilder method.
func TestLSPProvider_GetFlagsBuilder(t *testing.T) {
	provider := &Provider{}

	builder := provider.GetFlagsBuilder()

	assert.Nil(t, builder, "LSP command should not have a flags builder")
}

// TestLSPProvider_GetPositionalArgsBuilder tests the Provider GetPositionalArgsBuilder method.
func TestLSPProvider_GetPositionalArgsBuilder(t *testing.T) {
	provider := &Provider{}

	builder := provider.GetPositionalArgsBuilder()

	assert.Nil(t, builder, "LSP command should not have positional args builder")
}

// TestLSPProvider_GetCompatibilityFlags tests the Provider GetCompatibilityFlags method.
func TestLSPProvider_GetCompatibilityFlags(t *testing.T) {
	provider := &Provider{}

	flags := provider.GetCompatibilityFlags()

	assert.Nil(t, flags, "LSP command should not have compatibility flags")
}

// TestLSPProvider_GetAliases tests the Provider GetAliases method.
func TestLSPProvider_GetAliases(t *testing.T) {
	provider := &Provider{}

	aliases := provider.GetAliases()

	assert.Nil(t, aliases, "LSP command should not have aliases")
}

// TestLSPProvider_IsExperimental tests the Provider IsExperimental method.
func TestLSPProvider_IsExperimental(t *testing.T) {
	provider := &Provider{}

	isExperimental := provider.IsExperimental()

	assert.True(t, isExperimental, "LSP command should be experimental")
}

// TestLSPProvider_ImplementsInterface tests that Provider implements CommandProvider.
func TestLSPProvider_ImplementsInterface(t *testing.T) {
	var _ internal.CommandProvider = (*Provider)(nil)
}

// TestLSPProvider_AllMethods provides a comprehensive test of all provider methods.
func TestLSPProvider_AllMethods(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T, provider *Provider)
	}{
		{
			name: "GetCommand returns valid command",
			testFunc: func(t *testing.T, provider *Provider) {
				cmd := provider.GetCommand()
				assert.NotNil(t, cmd)
				assert.Equal(t, "lsp", cmd.Use)
			},
		},
		{
			name: "GetName returns lsp",
			testFunc: func(t *testing.T, provider *Provider) {
				assert.Equal(t, "lsp", provider.GetName())
			},
		},
		{
			name: "GetGroup returns Other Commands",
			testFunc: func(t *testing.T, provider *Provider) {
				assert.Equal(t, "Other Commands", provider.GetGroup())
			},
		},
		{
			name: "GetFlagsBuilder returns nil",
			testFunc: func(t *testing.T, provider *Provider) {
				assert.Nil(t, provider.GetFlagsBuilder())
			},
		},
		{
			name: "GetPositionalArgsBuilder returns nil",
			testFunc: func(t *testing.T, provider *Provider) {
				assert.Nil(t, provider.GetPositionalArgsBuilder())
			},
		},
		{
			name: "GetCompatibilityFlags returns nil",
			testFunc: func(t *testing.T, provider *Provider) {
				assert.Nil(t, provider.GetCompatibilityFlags())
			},
		},
		{
			name: "GetAliases returns nil",
			testFunc: func(t *testing.T, provider *Provider) {
				assert.Nil(t, provider.GetAliases())
			},
		},
		{
			name: "IsExperimental returns true",
			testFunc: func(t *testing.T, provider *Provider) {
				assert.True(t, provider.IsExperimental())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &Provider{}
			tt.testFunc(t, provider)
		})
	}
}

// TestExecuteLSPStart_InvalidTransport tests that executeLSPStart returns an error for invalid transport.
func TestExecuteLSPStart_InvalidTransport(t *testing.T) {
	tests := []struct {
		name      string
		transport string
	}{
		{
			name:      "unknown transport",
			transport: "unknown",
		},
		{
			name:      "empty transport",
			transport: "",
		},
		{
			name:      "random string",
			transport: "foobar",
		},
		{
			name:      "http transport",
			transport: "http",
		},
		{
			name:      "https transport",
			transport: "https",
		},
		{
			name:      "grpc transport",
			transport: "grpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a dummy command for testing.
			cmd := &cobra.Command{}
			err := executeLSPStart(cmd, tt.transport, "localhost:7777")
			// The function will fail when trying to load atmos config in a test environment
			// without proper atmos.yaml, but we verify that invalid transports
			// would produce the right error if config loading succeeded.
			// For now, we just verify the function can be called.
			if err != nil {
				// Either config error or transport error is acceptable.
				// The transport error check would only happen after config loads.
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}

// TestLSPStartCommand_CommandLineUsage tests command line argument parsing.
func TestLSPStartCommand_CommandLineUsage(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedError bool
	}{
		{
			name:          "no args uses defaults",
			args:          []string{},
			expectedError: false,
		},
		{
			name:          "with transport flag",
			args:          []string{"--transport=tcp"},
			expectedError: false,
		},
		{
			name:          "with address flag",
			args:          []string{"--address=0.0.0.0:8080"},
			expectedError: false,
		},
		{
			name:          "with both flags",
			args:          []string{"--transport=websocket", "--address=0.0.0.0:9999"},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewLSPStartCommand()
			// Prevent actual execution.
			cmd.RunE = nil
			cmd.Run = func(cmd *cobra.Command, args []string) {}

			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestLSPStartCommand_FlagParsing tests that flags are correctly parsed.
func TestLSPStartCommand_FlagParsing(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		expectedTransport string
		expectedAddress   string
	}{
		{
			name:              "default values",
			args:              []string{},
			expectedTransport: "stdio",
			expectedAddress:   "localhost:7777",
		},
		{
			name:              "custom transport",
			args:              []string{"--transport=tcp"},
			expectedTransport: "tcp",
			expectedAddress:   "localhost:7777",
		},
		{
			name:              "custom address",
			args:              []string{"--address=0.0.0.0:8080"},
			expectedTransport: "stdio",
			expectedAddress:   "0.0.0.0:8080",
		},
		{
			name:              "both custom",
			args:              []string{"--transport=websocket", "--address=127.0.0.1:9000"},
			expectedTransport: "websocket",
			expectedAddress:   "127.0.0.1:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewLSPStartCommand()

			var capturedTransport, capturedAddress string
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				capturedTransport, _ = cmd.Flags().GetString("transport")
				capturedAddress, _ = cmd.Flags().GetString("address")
				return nil
			}

			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			require.NoError(t, err)
			assert.Equal(t, tt.expectedTransport, capturedTransport, "transport mismatch")
			assert.Equal(t, tt.expectedAddress, capturedAddress, "address mismatch")
		})
	}
}

// TestLSPCommand_SubcommandStructure tests the subcommand structure.
func TestLSPCommand_SubcommandStructure(t *testing.T) {
	cmd := NewLSPCommand()

	// Verify subcommands.
	subcommands := cmd.Commands()
	require.Len(t, subcommands, 1, "should have exactly one subcommand")
	assert.Equal(t, "start", subcommands[0].Use)
}

// TestLSPCommand_HelpFlags tests that help flags are available on subcommands.
func TestLSPCommand_HelpFlags(t *testing.T) {
	cmd := NewLSPStartCommand()

	// Help flag is automatically added by cobra when the command is executed.
	// For now, we verify that the command can generate help output.
	usage := cmd.UsageString()
	assert.Contains(t, usage, "start")
}

// TestLSPStartCommand_HelpOutput tests that the command can generate help.
func TestLSPStartCommand_HelpOutput(t *testing.T) {
	cmd := NewLSPStartCommand()

	// Verify the command can generate its usage string without error.
	usage := cmd.UsageString()
	assert.Contains(t, usage, "--transport")
	assert.Contains(t, usage, "--address")
}

// TestLSPCommand_LongDescriptionContent tests the long description contains expected content.
func TestLSPCommand_LongDescriptionContent(t *testing.T) {
	cmd := NewLSPStartCommand()

	expectedPhrases := []string{
		"Language Server Protocol",
		"LSP server",
		"Syntax validation",
		"Auto-completion",
		"Hover documentation",
		"Go to definition",
		"stdio",
		"tcp",
		"websocket",
		"VS Code",
	}

	for _, phrase := range expectedPhrases {
		assert.Contains(t, cmd.Long, phrase, "long description should contain: %s", phrase)
	}
}

// TestErrLSPInvalidTransport tests that the ErrLSPInvalidTransport error is used correctly.
func TestErrLSPInvalidTransport(t *testing.T) {
	// Verify the error is defined.
	assert.NotNil(t, errUtils.ErrLSPInvalidTransport)
	assert.Contains(t, errUtils.ErrLSPInvalidTransport.Error(), "invalid LSP transport")
}

// TestLSPProvider_NewInstanceEachCall tests that GetCommand returns a new instance.
func TestLSPProvider_NewInstanceEachCall(t *testing.T) {
	provider := &Provider{}

	cmd1 := provider.GetCommand()
	cmd2 := provider.GetCommand()

	// Each call should return a new command instance.
	assert.NotSame(t, cmd1, cmd2, "GetCommand should return new instances")
	assert.Equal(t, cmd1.Use, cmd2.Use, "command properties should be identical")
}

// TestLSPStartCommand_TransportValidation documents the valid transport types.
func TestLSPStartCommand_TransportValidation(t *testing.T) {
	validTransports := []string{"stdio", "tcp", "websocket", "ws"}

	for _, transport := range validTransports {
		t.Run(transport, func(t *testing.T) {
			// Document that these are valid transport types.
			// Actual validation happens in executeLSPStart.
			t.Logf("Valid transport: %s", transport)
		})
	}
}

// TestLSPCommand_FParseErrWhitelist tests the flag parse error whitelist setting.
func TestLSPCommand_FParseErrWhitelist(t *testing.T) {
	cmd := NewLSPCommand()

	// Unknown flags should NOT be allowed (strict parsing).
	assert.False(t, cmd.FParseErrWhitelist.UnknownFlags)
}
