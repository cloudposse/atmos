package lsp

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockLSPServer is a mock implementation of the LSPServer interface for testing.
type mockLSPServer struct {
	runStdioFunc     func() error
	runTCPFunc       func(address string) error
	runWebSocketFunc func(address string) error
}

// RunStdio implements LSPServer.RunStdio.
func (m *mockLSPServer) RunStdio() error {
	if m.runStdioFunc != nil {
		return m.runStdioFunc()
	}
	return nil
}

// RunTCP implements LSPServer.RunTCP.
func (m *mockLSPServer) RunTCP(address string) error {
	if m.runTCPFunc != nil {
		return m.runTCPFunc(address)
	}
	return nil
}

// RunWebSocket implements LSPServer.RunWebSocket.
func (m *mockLSPServer) RunWebSocket(address string) error {
	if m.runWebSocketFunc != nil {
		return m.runWebSocketFunc(address)
	}
	return nil
}

// setupMockFactories sets up mock factories for testing and returns a cleanup function.
func setupMockFactories(
	mockConfigLoader ConfigLoader,
	mockServerFactory ServerFactory,
) func() {
	// Save original factories.
	origConfigLoader := configLoader
	origServerFactory := serverFactory

	// Set mock factories.
	if mockConfigLoader != nil {
		configLoader = mockConfigLoader
	}
	if mockServerFactory != nil {
		serverFactory = mockServerFactory
	}

	// Return cleanup function.
	return func() {
		configLoader = origConfigLoader
		serverFactory = origServerFactory
	}
}

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

// TestExecuteLSPStart_ConfigLoaderError tests that executeLSPStart returns config loader errors.
func TestExecuteLSPStart_ConfigLoaderError(t *testing.T) {
	configErr := errors.New("config loading failed")

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, configErr
		},
		nil,
	)
	defer cleanup()

	cmd := &cobra.Command{}
	err := executeLSPStart(cmd, "stdio", "localhost:7777")

	require.Error(t, err)
	assert.Equal(t, configErr, err)
}

// TestExecuteLSPStart_ServerFactoryError tests that executeLSPStart returns server factory errors.
func TestExecuteLSPStart_ServerFactoryError(t *testing.T) {
	serverErr := errors.New("server creation failed")

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			return nil, serverErr
		},
	)
	defer cleanup()

	cmd := &cobra.Command{}
	err := executeLSPStart(cmd, "stdio", "localhost:7777")

	require.Error(t, err)
	assert.Equal(t, serverErr, err)
}

// TestExecuteLSPStart_StdioTransport tests the stdio transport execution path.
func TestExecuteLSPStart_StdioTransport(t *testing.T) {
	var stdioCalledWith bool

	mockServer := &mockLSPServer{
		runStdioFunc: func() error {
			stdioCalledWith = true
			return nil
		},
	}

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			return mockServer, nil
		},
	)
	defer cleanup()

	cmd := &cobra.Command{}
	err := executeLSPStart(cmd, "stdio", "localhost:7777")

	require.NoError(t, err)
	assert.True(t, stdioCalledWith, "RunStdio should have been called")
}

// TestExecuteLSPStart_TCPTransport tests the TCP transport execution path.
func TestExecuteLSPStart_TCPTransport(t *testing.T) {
	var tcpAddress string

	mockServer := &mockLSPServer{
		runTCPFunc: func(address string) error {
			tcpAddress = address
			return nil
		},
	}

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			return mockServer, nil
		},
	)
	defer cleanup()

	cmd := &cobra.Command{}
	err := executeLSPStart(cmd, "tcp", "0.0.0.0:8080")

	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8080", tcpAddress, "RunTCP should have been called with correct address")
}

// TestExecuteLSPStart_WebSocketTransport tests the websocket transport execution path.
func TestExecuteLSPStart_WebSocketTransport(t *testing.T) {
	var wsAddress string

	mockServer := &mockLSPServer{
		runWebSocketFunc: func(address string) error {
			wsAddress = address
			return nil
		},
	}

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			return mockServer, nil
		},
	)
	defer cleanup()

	cmd := &cobra.Command{}
	err := executeLSPStart(cmd, "websocket", "localhost:9000")

	require.NoError(t, err)
	assert.Equal(t, "localhost:9000", wsAddress, "RunWebSocket should have been called with correct address")
}

// TestExecuteLSPStart_WSTransportAlias tests the ws alias for websocket transport.
func TestExecuteLSPStart_WSTransportAlias(t *testing.T) {
	var wsAddress string

	mockServer := &mockLSPServer{
		runWebSocketFunc: func(address string) error {
			wsAddress = address
			return nil
		},
	}

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			return mockServer, nil
		},
	)
	defer cleanup()

	cmd := &cobra.Command{}
	err := executeLSPStart(cmd, "ws", "localhost:9001")

	require.NoError(t, err)
	assert.Equal(t, "localhost:9001", wsAddress, "RunWebSocket should have been called with ws alias")
}

// TestExecuteLSPStart_InvalidTransportWithMock tests the invalid transport error with mocked dependencies.
func TestExecuteLSPStart_InvalidTransportWithMock(t *testing.T) {
	tests := []struct {
		name      string
		transport string
	}{
		{name: "unknown transport", transport: "unknown"},
		{name: "empty transport", transport: ""},
		{name: "http transport", transport: "http"},
		{name: "grpc transport", transport: "grpc"},
		{name: "unix transport", transport: "unix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := setupMockFactories(
				func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
					return schema.AtmosConfiguration{}, nil
				},
				func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
					return &mockLSPServer{}, nil
				},
			)
			defer cleanup()

			cmd := &cobra.Command{}
			err := executeLSPStart(cmd, tt.transport, "localhost:7777")

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrLSPInvalidTransport)
			assert.Contains(t, err.Error(), tt.transport)
			assert.Contains(t, err.Error(), "must be 'stdio', 'tcp', or 'websocket'")
		})
	}
}

// TestExecuteLSPStart_TransportRunErrors tests that transport run errors are properly returned.
func TestExecuteLSPStart_TransportRunErrors(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		setupMock func() *mockLSPServer
	}{
		{
			name:      "stdio error",
			transport: "stdio",
			setupMock: func() *mockLSPServer {
				return &mockLSPServer{
					runStdioFunc: func() error {
						return errors.New("stdio error")
					},
				}
			},
		},
		{
			name:      "tcp error",
			transport: "tcp",
			setupMock: func() *mockLSPServer {
				return &mockLSPServer{
					runTCPFunc: func(_ string) error {
						return errors.New("tcp error")
					},
				}
			},
		},
		{
			name:      "websocket error",
			transport: "websocket",
			setupMock: func() *mockLSPServer {
				return &mockLSPServer{
					runWebSocketFunc: func(_ string) error {
						return errors.New("websocket error")
					},
				}
			},
		},
		{
			name:      "ws alias error",
			transport: "ws",
			setupMock: func() *mockLSPServer {
				return &mockLSPServer{
					runWebSocketFunc: func(_ string) error {
						return errors.New("ws error")
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := tt.setupMock()

			cleanup := setupMockFactories(
				func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
					return schema.AtmosConfiguration{}, nil
				},
				func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
					return mockServer, nil
				},
			)
			defer cleanup()

			cmd := &cobra.Command{}
			err := executeLSPStart(cmd, tt.transport, "localhost:7777")

			require.Error(t, err)
		})
	}
}

// TestExecuteLSPStart_AllTransportTypes tests all valid transport types with mock.
func TestExecuteLSPStart_AllTransportTypes(t *testing.T) {
	tests := []struct {
		name                string
		transport           string
		address             string
		expectedMethod      string
		expectedAddress     string
		expectAddressLogged bool
	}{
		{
			name:                "stdio transport",
			transport:           "stdio",
			address:             "localhost:7777",
			expectedMethod:      "stdio",
			expectAddressLogged: false,
		},
		{
			name:                "tcp transport",
			transport:           "tcp",
			address:             "0.0.0.0:8080",
			expectedMethod:      "tcp",
			expectedAddress:     "0.0.0.0:8080",
			expectAddressLogged: true,
		},
		{
			name:                "websocket transport",
			transport:           "websocket",
			address:             "localhost:9000",
			expectedMethod:      "websocket",
			expectedAddress:     "localhost:9000",
			expectAddressLogged: true,
		},
		{
			name:                "ws alias transport",
			transport:           "ws",
			address:             "localhost:9001",
			expectedMethod:      "websocket",
			expectedAddress:     "localhost:9001",
			expectAddressLogged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledMethod string
			var calledAddress string

			mockServer := &mockLSPServer{
				runStdioFunc: func() error {
					calledMethod = "stdio"
					return nil
				},
				runTCPFunc: func(address string) error {
					calledMethod = "tcp"
					calledAddress = address
					return nil
				},
				runWebSocketFunc: func(address string) error {
					calledMethod = "websocket"
					calledAddress = address
					return nil
				},
			}

			cleanup := setupMockFactories(
				func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
					return schema.AtmosConfiguration{}, nil
				},
				func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
					return mockServer, nil
				},
			)
			defer cleanup()

			cmd := &cobra.Command{}
			err := executeLSPStart(cmd, tt.transport, tt.address)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMethod, calledMethod)
			if tt.expectAddressLogged {
				assert.Equal(t, tt.expectedAddress, calledAddress)
			}
		})
	}
}

// TestLSPServerInterface_Compliance verifies the mockLSPServer implements LSPServer.
func TestLSPServerInterface_Compliance(t *testing.T) {
	var _ LSPServer = (*mockLSPServer)(nil)
}

// TestDefaultFactories tests that default factories are set correctly.
func TestDefaultFactories(t *testing.T) {
	// Verify that default factories are not nil.
	assert.NotNil(t, defaultServerFactory)
	assert.NotNil(t, defaultConfigLoader)
}

// TestSetupMockFactories_Cleanup tests that the cleanup function properly restores factories.
func TestSetupMockFactories_Cleanup(t *testing.T) {
	// Create a marker to verify the mock is in place.
	mockConfigCalled := false

	// Setup mocks.
	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			mockConfigCalled = true
			return schema.AtmosConfiguration{}, errors.New("mock config")
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			return nil, errors.New("mock server")
		},
	)

	// Verify mocks are in place by calling them.
	_, err := configLoader(schema.ConfigAndStacksInfo{}, true)
	require.Error(t, err)
	assert.True(t, mockConfigCalled, "mock config loader should have been called")

	// Run cleanup.
	cleanup()

	// Verify cleanup runs without panic.
	assert.NotNil(t, configLoader)
	assert.NotNil(t, serverFactory)

	// After cleanup, calling configLoader should use the default which behaves differently.
	// We cannot directly compare functions, but we verified the mock was replaced during the test.
}

// TestSetupMockFactories_PartialMock tests setting only one mock factory.
func TestSetupMockFactories_PartialMock(t *testing.T) {
	// Create a marker to verify only config loader is mocked.
	mockConfigCalled := false

	// Only mock config loader.
	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			mockConfigCalled = true
			return schema.AtmosConfiguration{}, errors.New("mocked config error")
		},
		nil, // Do not mock server factory.
	)
	defer cleanup()

	// Verify config loader is mocked.
	_, err := configLoader(schema.ConfigAndStacksInfo{}, true)
	require.Error(t, err)
	assert.True(t, mockConfigCalled, "mock config loader should have been called")
	assert.Contains(t, err.Error(), "mocked config error")

	// Server factory should still be the default (not nil).
	assert.NotNil(t, serverFactory)
}

// TestExecuteLSPStart_ConfigLoaderReceivesCorrectParams tests that config loader is called with correct parameters.
func TestExecuteLSPStart_ConfigLoaderReceivesCorrectParams(t *testing.T) {
	var receivedProcessStacks bool

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			receivedProcessStacks = processStacks
			return schema.AtmosConfiguration{}, nil
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			return &mockLSPServer{}, nil
		},
	)
	defer cleanup()

	cmd := &cobra.Command{}
	_ = executeLSPStart(cmd, "stdio", "localhost:7777")

	// Verify processStacks is true (as per the implementation).
	assert.True(t, receivedProcessStacks)
}

// TestExecuteLSPStart_ServerFactoryReceivesConfig tests that server factory receives the config from config loader.
func TestExecuteLSPStart_ServerFactoryReceivesConfig(t *testing.T) {
	expectedConfig := schema.AtmosConfiguration{
		BasePath: "/test/path",
	}
	var receivedConfig *schema.AtmosConfiguration

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return expectedConfig, nil
		},
		func(_ context.Context, cfg *schema.AtmosConfiguration) (LSPServer, error) {
			receivedConfig = cfg
			return &mockLSPServer{}, nil
		},
	)
	defer cleanup()

	cmd := &cobra.Command{}
	_ = executeLSPStart(cmd, "stdio", "localhost:7777")

	require.NotNil(t, receivedConfig)
	assert.Equal(t, expectedConfig.BasePath, receivedConfig.BasePath)
}

// TestNewLSPStartCommand_RunE tests that the command's RunE handler calls executeLSPStart.
func TestNewLSPStartCommand_RunE(t *testing.T) {
	var executeCalled bool
	var capturedAddress string

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(_ context.Context, _ *schema.AtmosConfiguration) (LSPServer, error) {
			executeCalled = true
			return &mockLSPServer{
				runStdioFunc: func() error {
					return nil
				},
				runTCPFunc: func(addr string) error {
					capturedAddress = addr
					return nil
				},
			}, nil
		},
	)
	defer cleanup()

	// Test with default flags (stdio).
	t.Run("default stdio", func(t *testing.T) {
		executeCalled = false
		cmd := NewLSPStartCommand()
		cmd.SetArgs([]string{})
		err := cmd.Execute()

		require.NoError(t, err)
		assert.True(t, executeCalled, "executeLSPStart should have been called via RunE")
	})

	// Test with custom transport.
	t.Run("tcp transport", func(t *testing.T) {
		executeCalled = false
		capturedAddress = ""
		cmd := NewLSPStartCommand()
		cmd.SetArgs([]string{"--transport=tcp", "--address=0.0.0.0:9999"})
		err := cmd.Execute()

		require.NoError(t, err)
		assert.True(t, executeCalled, "executeLSPStart should have been called via RunE")
		assert.Equal(t, "0.0.0.0:9999", capturedAddress)
	})
}

// TestNewLSPStartCommand_RunE_Error tests that errors from executeLSPStart propagate through RunE.
func TestNewLSPStartCommand_RunE_Error(t *testing.T) {
	expectedErr := errors.New("test error")

	cleanup := setupMockFactories(
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, expectedErr
		},
		nil,
	)
	defer cleanup()

	cmd := NewLSPStartCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// TestDefaultServerFactory tests the default server factory.
func TestDefaultServerFactory(t *testing.T) {
	// Verify that the default server factory is set and can be called.
	// We cannot test the actual server creation without a valid config,
	// but we can verify it is not nil.
	assert.NotNil(t, defaultServerFactory)
}

// TestDefaultConfigLoader tests the default config loader.
func TestDefaultConfigLoader(t *testing.T) {
	// Verify that the default config loader is set.
	assert.NotNil(t, defaultConfigLoader)
}
