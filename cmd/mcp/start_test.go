package mcp

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// createTestAtmosConfig creates a test AtmosConfiguration with the given AI tool settings.
func createTestAtmosConfig(yoloMode bool, requireConfirmation *bool) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					YOLOMode:            yoloMode,
					RequireConfirmation: requireConfirmation,
				},
			},
		},
	}
}

// TestStartCmd_BasicProperties tests the basic properties of the start command.
func TestStartCmd_BasicProperties(t *testing.T) {
	cmd := startCmd

	assert.Equal(t, "start", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)
}

// TestStartCmd_Flags tests that all expected flags are properly defined.
func TestStartCmd_Flags(t *testing.T) {
	cmd := startCmd

	// Test transport flag.
	transportFlag := cmd.Flags().Lookup("transport")
	assert.NotNil(t, transportFlag, "transport flag should be defined")
	assert.Equal(t, "stdio", transportFlag.DefValue, "default transport should be stdio")
	assert.Contains(t, transportFlag.Usage, "stdio or http")

	// Test host flag.
	hostFlag := cmd.Flags().Lookup("host")
	assert.NotNil(t, hostFlag, "host flag should be defined")
	assert.Equal(t, "localhost", hostFlag.DefValue, "default host should be localhost")

	// Test port flag.
	portFlag := cmd.Flags().Lookup("port")
	assert.NotNil(t, portFlag, "port flag should be defined")
	assert.Equal(t, "8080", portFlag.DefValue, "default port should be 8080")
}

// TestGetTransportConfig tests the getTransportConfig function.
func TestGetTransportConfig(t *testing.T) {
	tests := []struct {
		name          string
		transport     string
		host          string
		port          int
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid stdio transport",
			transport:   "stdio",
			host:        "localhost",
			port:        8080,
			expectError: false,
		},
		{
			name:        "valid http transport",
			transport:   "http",
			host:        "0.0.0.0",
			port:        3000,
			expectError: false,
		},
		{
			name:          "invalid transport",
			transport:     "invalid",
			host:          "localhost",
			port:          8080,
			expectError:   true,
			errorContains: "invalid transport",
		},
		{
			name:          "empty transport",
			transport:     "",
			host:          "localhost",
			port:          8080,
			expectError:   true,
			errorContains: "invalid transport",
		},
		{
			name:          "websocket transport not supported",
			transport:     "websocket",
			host:          "localhost",
			port:          8080,
			expectError:   true,
			errorContains: "invalid transport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock command with the flags set.
			cmd := &cobra.Command{}
			cmd.Flags().String("transport", tt.transport, "")
			cmd.Flags().String("host", tt.host, "")
			cmd.Flags().Int("port", tt.port, "")

			config, err := getTransportConfig(cmd)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, config)
				assert.Equal(t, tt.transport, config.transportType)
				assert.Equal(t, tt.host, config.host)
				assert.Equal(t, tt.port, config.port)
			}
		})
	}
}

// TestGetPermissionMode tests the getPermissionMode function.
func TestGetPermissionMode(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		expectedMode permission.Mode
	}{
		{
			name:         "yolo mode enabled",
			atmosConfig:  createTestAtmosConfig(true, nil),
			expectedMode: permission.ModeYOLO,
		},
		{
			name:         "require confirmation enabled",
			atmosConfig:  createTestAtmosConfig(false, &boolTrue),
			expectedMode: permission.ModePrompt,
		},
		{
			name:         "require confirmation explicitly disabled",
			atmosConfig:  createTestAtmosConfig(false, &boolFalse),
			expectedMode: permission.ModeAllow,
		},
		{
			name:         "default mode (no settings)",
			atmosConfig:  createTestAtmosConfig(false, nil),
			expectedMode: permission.ModeAllow,
		},
		{
			name:         "yolo takes precedence over require confirmation",
			atmosConfig:  createTestAtmosConfig(true, &boolTrue),
			expectedMode: permission.ModeYOLO,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := getPermissionMode(tt.atmosConfig)
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

// TestTransportConfig_Struct tests the transportConfig struct.
func TestTransportConfig_Struct(t *testing.T) {
	config := &transportConfig{
		transportType: "http",
		host:          "0.0.0.0",
		port:          3000,
	}

	assert.Equal(t, "http", config.transportType)
	assert.Equal(t, "0.0.0.0", config.host)
	assert.Equal(t, 3000, config.port)
}

// TestStartCmd_Constants tests that the constants are properly defined.
func TestStartCmd_Constants(t *testing.T) {
	assert.Equal(t, "stdio", transportStdio)
	assert.Equal(t, "http", transportHTTP)
	assert.Equal(t, 8080, defaultHTTPPort)
	assert.Equal(t, "localhost", defaultHTTPHost)
	assert.Equal(t, "2025-03-26", mcpProtocolVersion)
}

// TestStartCmd_ExampleContainsUsageInfo tests that the command example contains useful information.
func TestStartCmd_ExampleContainsUsageInfo(t *testing.T) {
	cmd := startCmd

	// Check that example contains stdio usage.
	assert.Contains(t, cmd.Example, "atmos mcp start")

	// Check that example contains http transport usage.
	assert.Contains(t, cmd.Example, "--transport http")

	// Check that example contains port flag usage.
	assert.Contains(t, cmd.Example, "--port")
}

// TestStartCmd_LongDescriptionContainsTransportModes tests the long description.
func TestStartCmd_LongDescriptionContainsTransportModes(t *testing.T) {
	cmd := startCmd

	// Check that long description mentions stdio transport.
	assert.Contains(t, cmd.Long, "stdio")

	// Check that long description mentions http transport.
	assert.Contains(t, cmd.Long, "http")

	// Check that long description mentions MCP.
	assert.Contains(t, cmd.Long, "MCP")
}

// TestGetTransportConfig_FlagRetrieval tests flag retrieval edge cases.
func TestGetTransportConfig_FlagRetrieval(t *testing.T) {
	// Test with default values.
	cmd := &cobra.Command{}
	cmd.Flags().String("transport", transportStdio, "")
	cmd.Flags().String("host", defaultHTTPHost, "")
	cmd.Flags().Int("port", defaultHTTPPort, "")

	config, err := getTransportConfig(cmd)

	assert.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, transportStdio, config.transportType)
	assert.Equal(t, defaultHTTPHost, config.host)
	assert.Equal(t, defaultHTTPPort, config.port)
}

// TestGetTransportConfig_HTTPWithCustomHost tests http transport with custom host.
func TestGetTransportConfig_HTTPWithCustomHost(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("transport", "http", "")
	cmd.Flags().String("host", "192.168.1.1", "")
	cmd.Flags().Int("port", 9000, "")

	config, err := getTransportConfig(cmd)

	assert.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "http", config.transportType)
	assert.Equal(t, "192.168.1.1", config.host)
	assert.Equal(t, 9000, config.port)
}

// TestTransportTypeValidation tests transport type validation exhaustively.
func TestTransportTypeValidation(t *testing.T) {
	validTransports := []string{"stdio", "http"}
	invalidTransports := []string{"", "ws", "websocket", "grpc", "tcp", "udp", "STDIO", "HTTP", "Stdio", "Http"}

	for _, transport := range validTransports {
		t.Run("valid_"+transport, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("transport", transport, "")
			cmd.Flags().String("host", "localhost", "")
			cmd.Flags().Int("port", 8080, "")

			config, err := getTransportConfig(cmd)
			assert.NoError(t, err)
			require.NotNil(t, config)
			assert.Equal(t, transport, config.transportType)
		})
	}

	for _, transport := range invalidTransports {
		t.Run("invalid_"+transport, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("transport", transport, "")
			cmd.Flags().String("host", "localhost", "")
			cmd.Flags().Int("port", 8080, "")

			config, err := getTransportConfig(cmd)
			assert.Error(t, err)
			assert.Nil(t, config)
		})
	}
}

// TestStartCmd_FlagTypes tests that flag types are correct.
func TestStartCmd_FlagTypes(t *testing.T) {
	cmd := startCmd

	// Transport should be a string flag.
	transportFlag := cmd.Flags().Lookup("transport")
	require.NotNil(t, transportFlag)
	assert.Equal(t, "string", transportFlag.Value.Type())

	// Host should be a string flag.
	hostFlag := cmd.Flags().Lookup("host")
	require.NotNil(t, hostFlag)
	assert.Equal(t, "string", hostFlag.Value.Type())

	// Port should be an int flag.
	portFlag := cmd.Flags().Lookup("port")
	require.NotNil(t, portFlag)
	assert.Equal(t, "int", portFlag.Value.Type())
}

// TestGetPermissionMode_NilPointers tests getPermissionMode with nil pointers.
func TestGetPermissionMode_NilPointers(t *testing.T) {
	atmosConfig := createTestAtmosConfig(false, nil)

	mode := getPermissionMode(atmosConfig)
	assert.Equal(t, permission.ModeAllow, mode)
}
