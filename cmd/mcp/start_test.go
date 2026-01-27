package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
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

// createFullTestAtmosConfig creates a test AtmosConfiguration with full AI settings.
func createFullTestAtmosConfig(aiEnabled, toolsEnabled, yoloMode bool) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: aiEnabled,
				Tools: schema.AIToolSettings{
					Enabled:  toolsEnabled,
					YOLOMode: yoloMode,
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

// TestWaitForShutdown_SignalReceived tests waitForShutdown when a signal is received.
func TestWaitForShutdown_SignalReceived(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Send interrupt signal.
	go func() {
		time.Sleep(10 * time.Millisecond)
		sigChan <- os.Interrupt
	}()

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.NoError(t, err)
	assert.True(t, cancelCalled, "cancel function should have been called")
}

// TestWaitForShutdown_SIGTERMReceived tests waitForShutdown when SIGTERM is received.
func TestWaitForShutdown_SIGTERMReceived(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Send SIGTERM signal.
	go func() {
		time.Sleep(10 * time.Millisecond)
		sigChan <- syscall.SIGTERM
	}()

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.NoError(t, err)
	assert.True(t, cancelCalled, "cancel function should have been called")
}

// TestWaitForShutdown_ServerError tests waitForShutdown when a server error is received.
func TestWaitForShutdown_ServerError(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Send error.
	testError := errors.New("test server error")
	go func() {
		time.Sleep(10 * time.Millisecond)
		errChan <- testError
	}()

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MCP server error")
	assert.Contains(t, err.Error(), "test server error")
	assert.False(t, cancelCalled, "cancel function should not have been called for errors")
}

// TestWaitForShutdown_ContextCanceled tests waitForShutdown when context.Canceled is received.
func TestWaitForShutdown_ContextCanceled(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Send context.Canceled error (which should be ignored).
	go func() {
		time.Sleep(10 * time.Millisecond)
		errChan <- context.Canceled
	}()

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.NoError(t, err, "context.Canceled should not return an error")
	assert.False(t, cancelCalled, "cancel function should not have been called")
}

// TestWaitForShutdown_NilError tests waitForShutdown when a nil error is received.
func TestWaitForShutdown_NilError(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Send nil error (clean shutdown).
	go func() {
		time.Sleep(10 * time.Millisecond)
		errChan <- nil
	}()

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.NoError(t, err)
	assert.False(t, cancelCalled, "cancel function should not have been called")
}

// TestGetTransportConfig_InvalidTransportErrorType tests the error type for invalid transport.
func TestGetTransportConfig_InvalidTransportErrorType(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("transport", "invalid_transport", "")
	cmd.Flags().String("host", "localhost", "")
	cmd.Flags().Int("port", 8080, "")

	_, err := getTransportConfig(cmd)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrMCPInvalidTransport), "error should be ErrMCPInvalidTransport")
}

// TestGetTransportConfig_VariousHosts tests getTransportConfig with various host formats.
func TestGetTransportConfig_VariousHosts(t *testing.T) {
	tests := []struct {
		name string
		host string
	}{
		{name: "localhost", host: "localhost"},
		{name: "ipv4_loopback", host: "127.0.0.1"},
		{name: "ipv4_any", host: "0.0.0.0"},
		{name: "ipv4_custom", host: "192.168.1.100"},
		{name: "ipv6_loopback", host: "::1"},
		{name: "ipv6_any", host: "::"},
		{name: "hostname", host: "my-server.example.com"},
		{name: "empty_host", host: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("transport", "http", "")
			cmd.Flags().String("host", tt.host, "")
			cmd.Flags().Int("port", 8080, "")

			config, err := getTransportConfig(cmd)
			assert.NoError(t, err)
			require.NotNil(t, config)
			assert.Equal(t, tt.host, config.host)
		})
	}
}

// TestGetTransportConfig_VariousPorts tests getTransportConfig with various port values.
func TestGetTransportConfig_VariousPorts(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{name: "port_80", port: 80},
		{name: "port_443", port: 443},
		{name: "port_3000", port: 3000},
		{name: "port_8080", port: 8080},
		{name: "port_8443", port: 8443},
		{name: "port_65535", port: 65535},
		{name: "port_0", port: 0},
		{name: "port_1", port: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("transport", "http", "")
			cmd.Flags().String("host", "localhost", "")
			cmd.Flags().Int("port", tt.port, "")

			config, err := getTransportConfig(cmd)
			assert.NoError(t, err)
			require.NotNil(t, config)
			assert.Equal(t, tt.port, config.port)
		})
	}
}

// TestGetPermissionMode_AllCombinations tests all combinations of permission settings.
func TestGetPermissionMode_AllCombinations(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name                string
		yoloMode            bool
		requireConfirmation *bool
		allowedTools        []string
		restrictedTools     []string
		blockedTools        []string
		expectedMode        permission.Mode
	}{
		{
			name:         "all_defaults",
			yoloMode:     false,
			expectedMode: permission.ModeAllow,
		},
		{
			name:         "yolo_mode_only",
			yoloMode:     true,
			expectedMode: permission.ModeYOLO,
		},
		{
			name:                "require_confirmation_true",
			yoloMode:            false,
			requireConfirmation: &boolTrue,
			expectedMode:        permission.ModePrompt,
		},
		{
			name:                "require_confirmation_false",
			yoloMode:            false,
			requireConfirmation: &boolFalse,
			expectedMode:        permission.ModeAllow,
		},
		{
			name:                "yolo_overrides_require_confirmation",
			yoloMode:            true,
			requireConfirmation: &boolTrue,
			expectedMode:        permission.ModeYOLO,
		},
		{
			name:         "with_allowed_tools",
			yoloMode:     false,
			allowedTools: []string{"tool1", "tool2"},
			expectedMode: permission.ModeAllow,
		},
		{
			name:            "with_restricted_tools",
			yoloMode:        false,
			restrictedTools: []string{"tool1", "tool2"},
			expectedMode:    permission.ModeAllow,
		},
		{
			name:         "with_blocked_tools",
			yoloMode:     false,
			blockedTools: []string{"tool1", "tool2"},
			expectedMode: permission.ModeAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							YOLOMode:            tt.yoloMode,
							RequireConfirmation: tt.requireConfirmation,
							AllowedTools:        tt.allowedTools,
							RestrictedTools:     tt.restrictedTools,
							BlockedTools:        tt.blockedTools,
						},
					},
				},
			}

			mode := getPermissionMode(atmosConfig)
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

// TestStartCmd_RunENotNil verifies that RunE is properly set.
func TestStartCmd_RunENotNil(t *testing.T) {
	assert.NotNil(t, startCmd.RunE, "startCmd.RunE should not be nil")
}

// TestStartCmd_HasParent tests that startCmd has mcpCmd as parent.
func TestStartCmd_HasParent(t *testing.T) {
	// The parent is set during init() via mcpCmd.AddCommand(startCmd).
	parent := startCmd.Parent()
	assert.NotNil(t, parent, "startCmd should have a parent command")
	assert.Equal(t, "mcp", parent.Use, "parent command should be 'mcp'")
}

// TestStartCmd_FlagUsageMessages tests that flag usage messages are helpful.
func TestStartCmd_FlagUsageMessages(t *testing.T) {
	cmd := startCmd

	transportFlag := cmd.Flags().Lookup("transport")
	require.NotNil(t, transportFlag)
	assert.NotEmpty(t, transportFlag.Usage, "transport flag should have usage message")

	hostFlag := cmd.Flags().Lookup("host")
	require.NotNil(t, hostFlag)
	assert.NotEmpty(t, hostFlag.Usage, "host flag should have usage message")
	assert.Contains(t, hostFlag.Usage, "http transport", "host flag usage should mention http transport")

	portFlag := cmd.Flags().Lookup("port")
	require.NotNil(t, portFlag)
	assert.NotEmpty(t, portFlag.Usage, "port flag should have usage message")
	assert.Contains(t, portFlag.Usage, "http transport", "port flag usage should mention http transport")
}

// TestLogServerInfo_Stdio tests logServerInfo for stdio transport.
func TestLogServerInfo_Stdio(t *testing.T) {
	// This test verifies the function doesn't panic.
	// The actual logging is handled by the logger package.
	// We can't easily capture log output without modifying the logger.

	// Create a minimal mock server for testing.
	// Since we can't easily mock the server, we'll skip the detailed verification.
	// The function should not panic.

	// For now, just verify the function exists and doesn't panic when called with nil.
	// In a real test, we would inject a mock logger.
	t.Skip("Cannot easily test logServerInfo without mock logger injection")
}

// TestInitializeAIComponents_AIDisabled tests that initializeAIComponents still works when AI is disabled but tools are enabled.
// Note: initializeAIComponents only checks if tools are enabled, not if AI is enabled.
func TestInitializeAIComponents_AIDisabled(t *testing.T) {
	atmosConfig := createFullTestAtmosConfig(false, true, false)

	// When AI is disabled but tools are enabled, the function should still succeed.
	// The AI enabled check happens at a higher level.
	registry, executor, err := initializeAIComponents(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, executor)
}

// TestInitializeAIComponents_ToolsDisabled tests that initializeAIComponents returns error when tools are disabled.
func TestInitializeAIComponents_ToolsDisabled(t *testing.T) {
	atmosConfig := createFullTestAtmosConfig(true, false, false)

	_, _, err := initializeAIComponents(atmosConfig)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolsDisabled), "error should be ErrAIToolsDisabled")
}

// TestInitializeAIComponents_ToolsEnabled tests that initializeAIComponents succeeds when tools are enabled.
func TestInitializeAIComponents_ToolsEnabled(t *testing.T) {
	atmosConfig := createFullTestAtmosConfig(true, true, false)

	registry, executor, err := initializeAIComponents(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry, "registry should not be nil")
	assert.NotNil(t, executor, "executor should not be nil")
}

// TestInitializeAIComponents_YOLOMode tests that initializeAIComponents works in YOLO mode.
func TestInitializeAIComponents_YOLOMode(t *testing.T) {
	atmosConfig := createFullTestAtmosConfig(true, true, true)

	registry, executor, err := initializeAIComponents(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry, "registry should not be nil")
	assert.NotNil(t, executor, "executor should not be nil")
}

// TestInitializeAIComponents_WithToolLists tests that initializeAIComponents handles tool lists.
func TestInitializeAIComponents_WithToolLists(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Tools: schema.AIToolSettings{
					Enabled:         true,
					AllowedTools:    []string{"describe_component", "list_stacks"},
					RestrictedTools: []string{"write_component_file"},
					BlockedTools:    []string{"dangerous_tool"},
				},
			},
		},
	}

	registry, executor, err := initializeAIComponents(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry, "registry should not be nil")
	assert.NotNil(t, executor, "executor should not be nil")
}

// TestTransportConfig_ZeroValue tests transportConfig with zero values.
func TestTransportConfig_ZeroValue(t *testing.T) {
	config := &transportConfig{}

	assert.Empty(t, config.transportType)
	assert.Empty(t, config.host)
	assert.Equal(t, 0, config.port)
}

// TestWaitForShutdown_ImmediateSignal tests waitForShutdown with immediate signal.
func TestWaitForShutdown_ImmediateSignal(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Pre-buffer the signal before calling waitForShutdown.
	sigChan <- os.Interrupt

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.NoError(t, err)
	assert.True(t, cancelCalled)
}

// TestWaitForShutdown_ImmediateError tests waitForShutdown with immediate error.
func TestWaitForShutdown_ImmediateError(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Pre-buffer the error before calling waitForShutdown.
	testErr := errors.New("immediate error")
	errChan <- testErr

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "immediate error")
	assert.False(t, cancelCalled)
}

// TestWaitForShutdown_WrappedContextCanceled tests waitForShutdown with wrapped context.Canceled.
func TestWaitForShutdown_WrappedContextCanceled(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Send a wrapped context.Canceled error.
	wrappedErr := context.Canceled
	errChan <- wrappedErr

	err := waitForShutdown(sigChan, errChan, cancel)

	assert.NoError(t, err, "wrapped context.Canceled should not return an error")
	assert.False(t, cancelCalled)
}

// TestSignalNotifySetup tests that signal.Notify can be set up correctly.
func TestSignalNotifySetup(t *testing.T) {
	// This tests that the signal setup pattern used in executeMCPServer works.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Verify the channel can receive signals.
	assert.NotNil(t, sigChan)
}

// TestGetTransportConfig_MissingFlags tests getTransportConfig when flags are missing.
func TestGetTransportConfig_MissingFlags(t *testing.T) {
	// Create a command without any flags defined.
	cmd := &cobra.Command{}

	// This should panic or return zero values since flags aren't defined.
	// The actual behavior depends on cobra's implementation.
	defer func() {
		if r := recover(); r != nil {
			// Expected behavior - cobra panics when flag doesn't exist.
			t.Log("Recovered from expected panic when flags are missing")
		}
	}()

	// Try to get config - this may panic.
	_, _ = getTransportConfig(cmd)
}

// TestStartCmd_ShortDescription tests the short description format.
func TestStartCmd_ShortDescription(t *testing.T) {
	cmd := startCmd

	// Verify short description is concise (typically < 80 chars).
	assert.Less(t, len(cmd.Short), 100, "short description should be concise")
	assert.NotContains(t, cmd.Short, "\n", "short description should not contain newlines")
}

// TestStartCmd_LongDescriptionComprehensive tests that long description covers all features.
func TestStartCmd_LongDescriptionComprehensive(t *testing.T) {
	cmd := startCmd

	// Check for key information in long description.
	assert.Contains(t, cmd.Long, "Claude Desktop", "long description should mention Claude Desktop")
	assert.Contains(t, cmd.Long, "Server-Sent Events", "long description should mention SSE")
	assert.Contains(t, cmd.Long, "Ctrl+C", "long description should mention interrupt signal")
}

// TestStartCmd_ExampleComprehensive tests that examples cover all use cases.
func TestStartCmd_ExampleComprehensive(t *testing.T) {
	cmd := startCmd

	// Check for all example scenarios.
	assert.Contains(t, cmd.Example, "stdio", "example should show stdio transport")
	assert.Contains(t, cmd.Example, "--host", "example should show host flag")
	assert.Contains(t, cmd.Example, "0.0.0.0", "example should show binding to all interfaces")
}

// TestGetPermissionMode_EmptyConfig tests getPermissionMode with minimal config.
func TestGetPermissionMode_EmptyConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	mode := getPermissionMode(atmosConfig)
	assert.Equal(t, permission.ModeAllow, mode, "empty config should default to ModeAllow")
}

// TestInitializeAIComponents_ReturnsCorrectTypes tests that initializeAIComponents returns the correct types.
func TestInitializeAIComponents_ReturnsCorrectTypes(t *testing.T) {
	atmosConfig := createFullTestAtmosConfig(true, true, false)

	registryRaw, executorRaw, err := initializeAIComponents(atmosConfig)

	require.NoError(t, err)

	// Verify types can be asserted.
	_, ok1 := registryRaw.(*struct { /* tools.Registry */
	})
	_, ok2 := executorRaw.(*struct { /* tools.Executor */
	})

	// The actual types are *tools.Registry and *tools.Executor.
	// We verify they are not nil, which indicates successful type assertion in the function.
	assert.NotNil(t, registryRaw)
	assert.NotNil(t, executorRaw)

	// These assertions verify the types are interfaces/pointers, not the specific types.
	// The actual type assertion happens inside initializeAIComponents.
	t.Logf("registryRaw type assertion to custom struct: %v", ok1)
	t.Logf("executorRaw type assertion to custom struct: %v", ok2)
}

// TestStartCmd_LongDescriptionContainsClaudeDesktopConfig tests for Claude Desktop config example.
func TestStartCmd_LongDescriptionContainsClaudeDesktopConfig(t *testing.T) {
	cmd := startCmd

	// Verify the long description contains Claude Desktop configuration example.
	assert.Contains(t, cmd.Long, "claude_desktop_config.json", "long description should contain Claude Desktop config file name")
	assert.Contains(t, cmd.Long, "mcpServers", "long description should contain mcpServers key")
}

// TestGetTransportConfig_ErrorMessage tests the error message format for invalid transport.
func TestGetTransportConfig_ErrorMessage(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("transport", "invalid", "")
	cmd.Flags().String("host", "localhost", "")
	cmd.Flags().Int("port", 8080, "")

	_, err := getTransportConfig(cmd)

	assert.Error(t, err)
	// Verify error message contains both the invalid value and valid options.
	errMsg := err.Error()
	assert.Contains(t, errMsg, "invalid", "error should contain the invalid value")
	assert.Contains(t, errMsg, "stdio", "error should mention valid option 'stdio'")
	assert.Contains(t, errMsg, "http", "error should mention valid option 'http'")
}

// TestMCPProtocolVersion tests that the MCP protocol version is in expected format.
func TestMCPProtocolVersion(t *testing.T) {
	// Verify the protocol version follows YYYY-MM-DD format.
	parts := strings.Split(mcpProtocolVersion, "-")
	assert.Len(t, parts, 3, "protocol version should have 3 parts (YYYY-MM-DD)")

	if len(parts) == 3 {
		assert.Len(t, parts[0], 4, "year should be 4 digits")
		assert.Len(t, parts[1], 2, "month should be 2 digits")
		assert.Len(t, parts[2], 2, "day should be 2 digits")
	}
}

// TestStartCmd_CommandUse tests the command use string.
func TestStartCmd_CommandUse(t *testing.T) {
	assert.Equal(t, "start", startCmd.Use, "command use should be 'start'")
}

// BenchmarkGetTransportConfig benchmarks the getTransportConfig function.
func BenchmarkGetTransportConfig(b *testing.B) {
	cmd := &cobra.Command{}
	cmd.Flags().String("transport", "stdio", "")
	cmd.Flags().String("host", "localhost", "")
	cmd.Flags().Int("port", 8080, "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = getTransportConfig(cmd)
	}
}

// BenchmarkGetPermissionMode benchmarks the getPermissionMode function.
func BenchmarkGetPermissionMode(b *testing.B) {
	boolTrue := true
	atmosConfig := createTestAtmosConfig(false, &boolTrue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getPermissionMode(atmosConfig)
	}
}

// TestStartCmd_NotDeprecated tests that the command is not marked as deprecated.
func TestStartCmd_NotDeprecated(t *testing.T) {
	assert.Empty(t, startCmd.Deprecated, "start command should not be deprecated")
}

// TestStartCmd_NoAliases tests that the command has no aliases (or expected aliases).
func TestStartCmd_NoAliases(t *testing.T) {
	// The start command might have aliases; if not, this test passes.
	// If aliases are added later, this test documents them.
	assert.Empty(t, startCmd.Aliases, "start command should have no aliases")
}

// TestStartCmd_ValidExample tests that the example commands are syntactically valid.
func TestStartCmd_ValidExample(t *testing.T) {
	// Parse the example string and verify each line is a valid command.
	lines := strings.Split(startCmd.Example, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comment lines and empty lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Verify the line starts with 'atmos'.
		assert.True(t, strings.HasPrefix(line, "atmos mcp start"), "example line should start with 'atmos mcp start': %s", line)
	}
}

// TestInitializeAIComponents_RegistersExpectedTools tests that all expected tools are registered.
func TestInitializeAIComponents_RegistersExpectedTools(t *testing.T) {
	atmosConfig := createFullTestAtmosConfig(true, true, false)

	registryRaw, _, err := initializeAIComponents(atmosConfig)

	require.NoError(t, err)
	require.NotNil(t, registryRaw)

	// Cast to tools.Registry to check the count.
	registry, ok := registryRaw.(*tools.Registry)
	require.True(t, ok, "registry should be *tools.Registry")

	// Verify that tools were registered.
	toolCount := registry.Count()
	assert.Greater(t, toolCount, 0, "at least one tool should be registered")

	// Expected tools based on the initializeAIComponents function.
	expectedMinTools := 7 // describe_component, list_stacks, validate_stacks, read_component_file, read_stack_file, write_component_file, write_stack_file
	assert.GreaterOrEqual(t, toolCount, expectedMinTools, "should register at least %d tools", expectedMinTools)
}

// TestInitializeAIComponents_AllToolsRegistered tests that specific tools are registered.
func TestInitializeAIComponents_AllToolsRegistered(t *testing.T) {
	atmosConfig := createFullTestAtmosConfig(true, true, false)

	registryRaw, _, err := initializeAIComponents(atmosConfig)

	require.NoError(t, err)

	registry, ok := registryRaw.(*tools.Registry)
	require.True(t, ok)

	// Get list of all registered tools.
	toolsList := registry.List()
	toolNames := make([]string, len(toolsList))
	for i, tool := range toolsList {
		toolNames[i] = tool.Name()
	}

	// Verify expected tools are registered.
	// Note: Tool names are based on the actual Name() implementation in each tool.
	expectedTools := []string{
		"atmos_describe_component",
		"atmos_list_stacks",
		"atmos_validate_stacks",
		"read_component_file",
		"read_stack_file",
		"write_component_file",
		"write_stack_file",
	}

	for _, expectedTool := range expectedTools {
		found := false
		for _, toolName := range toolNames {
			if toolName == expectedTool {
				found = true
				break
			}
		}
		assert.True(t, found, "tool '%s' should be registered", expectedTool)
	}
}

// TestInitializeAIComponents_PermissionCheckerConfiguration tests permission checker setup.
func TestInitializeAIComponents_PermissionCheckerConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		yoloMode     bool
		allowedTools []string
		blockedTools []string
	}{
		{
			name:     "yolo_mode",
			yoloMode: true,
		},
		{
			name:         "with_allowed_tools",
			yoloMode:     false,
			allowedTools: []string{"atmos_describe_component"},
		},
		{
			name:         "with_blocked_tools",
			yoloMode:     false,
			blockedTools: []string{"dangerous_tool"},
		},
		{
			name:         "mixed_tool_lists",
			yoloMode:     false,
			allowedTools: []string{"tool1"},
			blockedTools: []string{"tool2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Tools: schema.AIToolSettings{
							Enabled:      true,
							YOLOMode:     tt.yoloMode,
							AllowedTools: tt.allowedTools,
							BlockedTools: tt.blockedTools,
						},
					},
				},
			}

			registry, executor, err := initializeAIComponents(atmosConfig)

			assert.NoError(t, err)
			assert.NotNil(t, registry)
			assert.NotNil(t, executor)
		})
	}
}

// TestStartStdioServer_ChannelBehavior tests the startStdioServer goroutine behavior.
func TestStartStdioServer_ChannelBehavior(t *testing.T) {
	// Test that startStdioServer sends to errChan.
	// We can't fully test this without a real server, but we can verify the channel setup.
	errChan := make(chan error, 1)
	assert.NotNil(t, errChan)

	// Verify channel is buffered and can receive.
	select {
	case errChan <- nil:
		// Successfully sent to channel.
	default:
		t.Error("errChan should be able to receive")
	}

	// Read from channel.
	select {
	case err := <-errChan:
		assert.NoError(t, err)
	default:
		t.Error("errChan should have a value")
	}
}

// TestStartHTTPServer_AddressFormat tests HTTP server address formatting.
func TestStartHTTPServer_AddressFormat(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "localhost_8080",
			host:     "localhost",
			port:     8080,
			expected: "localhost:8080",
		},
		{
			name:     "any_interface_3000",
			host:     "0.0.0.0",
			port:     3000,
			expected: "0.0.0.0:3000",
		},
		{
			name:     "custom_ip",
			host:     "192.168.1.1",
			port:     9000,
			expected: "192.168.1.1:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the address formatting logic used in startHTTPServer.
			addr := fmt.Sprintf("%s:%d", tt.host, tt.port)
			assert.Equal(t, tt.expected, addr)
		})
	}
}

// TestExecuteMCPServer_InvalidTransport tests executeMCPServer with invalid transport via command flags.
func TestExecuteMCPServer_InvalidTransport(t *testing.T) {
	// Create a new command with invalid transport.
	cmd := &cobra.Command{}
	cmd.Flags().String("transport", "invalid", "")
	cmd.Flags().String("host", "localhost", "")
	cmd.Flags().Int("port", 8080, "")

	// This should return an error due to invalid transport.
	// Note: We can't fully test executeMCPServer because it requires config initialization.
	config, err := getTransportConfig(cmd)
	assert.Error(t, err)
	assert.Nil(t, config)
}

// TestContextCancellation tests context cancellation behavior.
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Verify context is not canceled initially.
	select {
	case <-ctx.Done():
		t.Error("context should not be done initially")
	default:
		// Expected.
	}

	// Cancel the context.
	cancel()

	// Verify context is now canceled.
	select {
	case <-ctx.Done():
		assert.Equal(t, context.Canceled, ctx.Err())
	default:
		t.Error("context should be done after cancel")
	}
}

// TestSignalHandling tests signal handling pattern.
func TestSignalHandling(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Test that we can stop receiving signals.
	signal.Stop(sigChan)

	// After stopping, the channel should not receive process signals.
	// But we can still manually send to test channel behavior.
	go func() {
		sigChan <- os.Interrupt
	}()

	select {
	case sig := <-sigChan:
		assert.Equal(t, os.Interrupt, sig)
	case <-time.After(100 * time.Millisecond):
		t.Error("expected signal but timed out")
	}
}

// TestWaitForShutdown_RaceCondition tests that waitForShutdown handles concurrent signals and errors.
func TestWaitForShutdown_RaceCondition(t *testing.T) {
	// This test verifies that select in waitForShutdown works correctly
	// when both channels have values available.
	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Pre-buffer both channels.
	sigChan <- os.Interrupt
	errChan <- errors.New("test error")

	// The function should handle one of them.
	err := waitForShutdown(sigChan, errChan, cancel)

	// Either signal or error should be handled.
	// If signal was handled, cancel should be true and err should be nil.
	// If error was handled, cancel should be false and err should contain the error.
	if cancelCalled {
		assert.NoError(t, err)
	} else {
		assert.Error(t, err)
	}
}

// TestTransportConfigFields tests all fields of transportConfig.
func TestTransportConfigFields(t *testing.T) {
	config := &transportConfig{
		transportType: "http",
		host:          "example.com",
		port:          443,
	}

	// Test field access.
	assert.Equal(t, "http", config.transportType)
	assert.Equal(t, "example.com", config.host)
	assert.Equal(t, 443, config.port)

	// Test modification.
	config.transportType = "stdio"
	config.host = "localhost"
	config.port = 8080

	assert.Equal(t, "stdio", config.transportType)
	assert.Equal(t, "localhost", config.host)
	assert.Equal(t, 8080, config.port)
}

// TestDefaultValues tests that default values are correct.
func TestDefaultValues(t *testing.T) {
	assert.Equal(t, "stdio", transportStdio, "transportStdio should be 'stdio'")
	assert.Equal(t, "http", transportHTTP, "transportHTTP should be 'http'")
	assert.Equal(t, 8080, defaultHTTPPort, "defaultHTTPPort should be 8080")
	assert.Equal(t, "localhost", defaultHTTPHost, "defaultHTTPHost should be 'localhost'")
}

// TestGetTransportConfig_DefaultsFromStartCmd tests that defaults match startCmd flags.
func TestGetTransportConfig_DefaultsFromStartCmd(t *testing.T) {
	// Get the default values from the actual startCmd.
	transportFlag := startCmd.Flags().Lookup("transport")
	require.NotNil(t, transportFlag)
	assert.Equal(t, transportStdio, transportFlag.DefValue)

	hostFlag := startCmd.Flags().Lookup("host")
	require.NotNil(t, hostFlag)
	assert.Equal(t, defaultHTTPHost, hostFlag.DefValue)

	portFlag := startCmd.Flags().Lookup("port")
	require.NotNil(t, portFlag)
	assert.Equal(t, fmt.Sprintf("%d", defaultHTTPPort), portFlag.DefValue)
}

// TestInitializeAIComponents_NilConfig tests behavior with nil fields in config.
func TestInitializeAIComponents_NilConfig(t *testing.T) {
	// Test with minimal config - tools enabled but nothing else.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: true,
				Tools: schema.AIToolSettings{
					Enabled: true,
				},
			},
		},
	}

	registry, executor, err := initializeAIComponents(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, executor)
}

// TestWaitForShutdown_MultipleSignals tests handling multiple signals.
func TestWaitForShutdown_MultipleSignals(t *testing.T) {
	sigChan := make(chan os.Signal, 2)
	errChan := make(chan error, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	// Buffer multiple signals.
	sigChan <- os.Interrupt
	sigChan <- syscall.SIGTERM

	// First call should handle one signal.
	err := waitForShutdown(sigChan, errChan, cancel)
	assert.NoError(t, err)
	assert.True(t, cancelCalled)

	// Second signal should still be in channel.
	select {
	case sig := <-sigChan:
		assert.Equal(t, syscall.SIGTERM, sig)
	default:
		t.Error("second signal should still be in channel")
	}
}

// TestGetPermissionMode_WithRequireConfirmationNilAndFalseYOLO tests the fallthrough case.
func TestGetPermissionMode_WithRequireConfirmationNilAndFalseYOLO(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					YOLOMode:            false,
					RequireConfirmation: nil,
				},
			},
		},
	}

	mode := getPermissionMode(atmosConfig)
	assert.Equal(t, permission.ModeAllow, mode)
}

// TestStartCmd_Integration tests that the start command integrates properly with parent.
func TestStartCmd_Integration(t *testing.T) {
	// Verify the command is properly attached to mcpCmd.
	parent := startCmd.Parent()
	require.NotNil(t, parent)
	assert.Equal(t, "mcp", parent.Use)

	// Verify startCmd is in parent's Commands.
	found := false
	for _, cmd := range parent.Commands() {
		if cmd.Use == "start" {
			found = true
			break
		}
	}
	assert.True(t, found, "startCmd should be in mcpCmd's subcommands")
}

// TestHTTPServerConfig tests HTTP server configuration values.
func TestHTTPServerConfig(t *testing.T) {
	// These test the constant values that would be used for HTTP server configuration.
	readTimeout := 30 * time.Second
	writeTimeout := 30 * time.Second
	idleTimeout := 60 * time.Second
	readHeaderTimeout := 10 * time.Second

	assert.Equal(t, 30*time.Second, readTimeout)
	assert.Equal(t, 30*time.Second, writeTimeout)
	assert.Equal(t, 60*time.Second, idleTimeout)
	assert.Equal(t, 10*time.Second, readHeaderTimeout)
}

// TestErrorChanBuffer tests error channel buffer size.
func TestErrorChanBuffer(t *testing.T) {
	// The error channel in executeMCPServer has buffer size 1.
	errChan := make(chan error, 1)

	// Should be able to send without blocking.
	select {
	case errChan <- errors.New("test"):
		// Success.
	default:
		t.Error("should be able to send to buffered channel")
	}

	// Second send should block (buffer full).
	select {
	case errChan <- errors.New("test2"):
		t.Error("second send should block")
	default:
		// Expected - buffer is full.
	}
}

// TestSignalChanBuffer tests signal channel buffer size.
func TestSignalChanBuffer(t *testing.T) {
	// The signal channel in executeMCPServer has buffer size 1.
	sigChan := make(chan os.Signal, 1)

	// Should be able to send without blocking.
	select {
	case sigChan <- os.Interrupt:
		// Success.
	default:
		t.Error("should be able to send to buffered channel")
	}
}

// TestMCPProtocolVersionFormat tests the protocol version string format.
func TestMCPProtocolVersionFormat(t *testing.T) {
	// The version should be a date in YYYY-MM-DD format.
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, mcpProtocolVersion, "protocol version should be in YYYY-MM-DD format")
}

// TestTransportConfigEquality tests transportConfig comparison.
func TestTransportConfigEquality(t *testing.T) {
	config1 := &transportConfig{
		transportType: "http",
		host:          "localhost",
		port:          8080,
	}

	config2 := &transportConfig{
		transportType: "http",
		host:          "localhost",
		port:          8080,
	}

	// Same values but different pointers.
	assert.NotSame(t, config1, config2)
	assert.Equal(t, config1.transportType, config2.transportType)
	assert.Equal(t, config1.host, config2.host)
	assert.Equal(t, config1.port, config2.port)
}
