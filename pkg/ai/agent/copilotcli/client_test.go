package copilotcli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewClient_Disabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{Enabled: false},
	}
	_, err := NewClient(atmosConfig)
	assert.ErrorIs(t, err, errUtils.ErrAIDisabledInConfiguration)
}

func TestNewClient_BinaryNotOnPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {}},
		},
	}
	t.Setenv("PATH", t.TempDir())

	_, err := NewClient(atmosConfig)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderBinaryNotFound)
}

func TestNewClient_CustomBinary(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {
					Binary:   "/usr/local/bin/copilot",
					Model:    "claude-sonnet-4.6",
					FullAuto: true,
				},
			},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/copilot", client.binaryPath)
	assert.Equal(t, "claude-sonnet-4.6", client.model)
	assert.True(t, client.fullAuto)
}

func TestNewClient_MCPServers_NotCaptured_WhenEmpty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {Binary: "/usr/local/bin/copilot"}},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Nil(t, client.mcpServers)
	assert.False(t, client.hasMCPServers)
}

func TestNewClient_MCPServers_Captured_WhenConfigured(t *testing.T) {
	// Use temp COPILOT_HOME to avoid touching the real ~/.copilot directory.
	t.Setenv(CopilotHomeEnvVar, t.TempDir())

	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {Binary: "/usr/local/bin/copilot"}},
		},
		MCP: schema.MCPSettings{
			Servers: map[string]schema.MCPServerConfig{
				"aws-docs": {Command: "uvx", Args: []string{"docs@latest"}},
			},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, client.mcpServers, 1)
	assert.True(t, client.hasMCPServers)

	// Verify config was written and restore it.
	defer client.restoreMCPConfig()
	configPath := copilotMCPConfigPath()
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"aws-docs"`)
	assert.Contains(t, string(data), `"command": "uvx"`)
	assert.Contains(t, string(data), `"type": "local"`)
}

func TestExtractResult_PlainText(t *testing.T) {
	result, err := ExtractResult([]byte("Plain text output\n"))
	require.NoError(t, err)
	assert.Equal(t, "Plain text output", result)
}

func TestExtractResult_Empty(t *testing.T) {
	_, err := ExtractResult([]byte(""))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderParseResponse)
}

func TestExtractResult_WhitespaceOnly(t *testing.T) {
	_, err := ExtractResult([]byte("   \n\t  \n"))
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderParseResponse)
}

func TestSendMessageWithTools_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "copilot"}
	_, err := client.SendMessageWithTools(context.Background(), "test", nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

func TestSendMessageWithToolsAndHistory_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "copilot"}
	_, err := client.SendMessageWithToolsAndHistory(context.Background(), nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

func TestGetModel(t *testing.T) {
	client := &Client{model: "gpt-5.2"}
	assert.Equal(t, "gpt-5.2", client.GetModel())
}

func TestGetMaxTokens(t *testing.T) {
	client := &Client{}
	assert.Equal(t, 0, client.GetMaxTokens())
}

func TestProviderName(t *testing.T) {
	assert.Equal(t, "copilot-cli", ProviderName)
}

func TestBuildArgs_Basic(t *testing.T) {
	client := &Client{}
	args := client.buildArgs("hello world")
	assert.Equal(t, "-p", args[0])
	assert.Equal(t, "hello world", args[1])
	assert.Contains(t, args, "-s")
	assert.Contains(t, args, "--no-ask-user")
	assert.NotContains(t, args, "--allow-all-tools")
	assert.NotContains(t, args, "--model")
}

func TestBuildArgs_WithModel(t *testing.T) {
	client := &Client{model: "claude-sonnet-4.6"}
	args := client.buildArgs("hi")
	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "claude-sonnet-4.6")
}

func TestBuildArgs_ModelSameAsProvider(t *testing.T) {
	client := &Client{model: ProviderName}
	args := client.buildArgs("hi")
	assert.NotContains(t, args, "--model")
}

func TestBuildArgs_FullAuto(t *testing.T) {
	client := &Client{fullAuto: true}
	args := client.buildArgs("hi")
	assert.Contains(t, args, "--allow-all-tools")
}

func TestBuildArgs_MCPImpliesAllowAllTools(t *testing.T) {
	client := &Client{hasMCPServers: true}
	args := client.buildArgs("hi")
	assert.Contains(t, args, "--allow-all-tools")
}

func TestCopilotConfigDir_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CopilotHomeEnvVar, dir)
	assert.Equal(t, dir, copilotConfigDir())
	assert.Equal(t, filepath.Join(dir, "mcp-config.json"), copilotMCPConfigPath())
}

func TestWriteAndRestoreMCPConfig_NoOriginal(t *testing.T) {
	t.Setenv(CopilotHomeEnvVar, t.TempDir())

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"test-srv": {Command: "echo", Args: []string{"hello"}},
		},
	}

	require.NoError(t, client.writeMCPConfig())

	configPath := copilotMCPConfigPath()
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var parsed struct {
		MCPServers map[string]copilotMCPServer `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Contains(t, parsed.MCPServers, "test-srv")
	assert.Equal(t, "echo", parsed.MCPServers["test-srv"].Command)
	assert.Equal(t, []string{"hello"}, parsed.MCPServers["test-srv"].Args)
	assert.Equal(t, "local", parsed.MCPServers["test-srv"].Type)
	assert.Equal(t, []string{"*"}, parsed.MCPServers["test-srv"].Tools)

	// Restore — no original existed, file should be removed.
	client.restoreMCPConfig()
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}

func TestWriteAndRestoreMCPConfig_PreservesExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv(CopilotHomeEnvVar, home)

	// Pre-existing user config with one server.
	original := `{"mcpServers":{"user-srv":{"command":"node","args":["srv.js"],"type":"local","tools":["*"]}}}`
	configPath := copilotMCPConfigPath()
	require.NoError(t, os.WriteFile(configPath, []byte(original), 0o600))

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"atmos-srv": {Command: "echo", Args: []string{"hello"}},
		},
	}
	require.NoError(t, client.writeMCPConfig())
	assert.True(t, client.configBackedUp)

	// Both the user's server and the Atmos-managed one are present.
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var parsed struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Contains(t, parsed.MCPServers, "user-srv")
	assert.Contains(t, parsed.MCPServers, "atmos-srv")

	// Restore — the original file content comes back byte-for-byte.
	client.restoreMCPConfig()
	restored, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, original, string(restored))
}

func TestWriteMCPConfig_AuthServerWrapped(t *testing.T) {
	t.Setenv(CopilotHomeEnvVar, t.TempDir())

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"auth-server": {Command: "uvx", Args: []string{"pkg@latest"}, Identity: "admin"},
		},
	}
	require.NoError(t, client.writeMCPConfig())
	defer client.restoreMCPConfig()

	data, err := os.ReadFile(copilotMCPConfigPath())
	require.NoError(t, err)
	// Auth-requiring servers are wrapped with `atmos auth exec`.
	assert.Contains(t, string(data), `"command": "atmos"`)
	assert.Contains(t, string(data), `"auth"`)
}

func TestRestoreMCPConfig_NoBackupRemovesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv(CopilotHomeEnvVar, home)

	configPath := copilotMCPConfigPath()
	require.NoError(t, os.WriteFile(configPath, []byte("{}"), 0o600))

	client := &Client{configBackedUp: false}
	client.restoreMCPConfig()
	_, err := os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}

// formatMessages tests are in pkg/ai/agent/base/messages_tools_test.go (FormatMessagesAsPrompt).
