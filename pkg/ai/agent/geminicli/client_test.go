package geminicli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/types"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
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
					Binary: "/usr/local/bin/gemini",
					Model:  "gemini-2.5-flash",
				},
			},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/gemini", client.binaryPath)
	assert.Equal(t, "gemini-2.5-flash", client.model)
}

func TestNewClient_MCPServers_NotCaptured_WhenEmpty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {Binary: "/usr/local/bin/gemini"}},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Nil(t, client.mcpServers)
	assert.Nil(t, client.mcpServers)
}

func TestNewClient_MCPServers_Captured_WhenConfigured(t *testing.T) {
	// Change to temp dir so writeMCPSettingsInCwd doesn't pollute the package dir.
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {Binary: "/usr/local/bin/gemini"}},
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

	// Verify settings.json was created in cwd.
	settingsFile := filepath.Join(tmpDir, ".gemini", "settings.json")
	data, err := os.ReadFile(settingsFile)
	require.NoError(t, err)

	var settings geminiSettings
	require.NoError(t, json.Unmarshal(data, &settings))
	assert.Contains(t, settings.MCPServers, "aws-docs")
}

func TestParseResponse_ValidJSON(t *testing.T) {
	input := `{"session_id": "abc123", "response": "The VPC is configured correctly."}`
	result, err := parseResponse([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "The VPC is configured correctly.", result)
}

func TestParseResponse_PlainText(t *testing.T) {
	result, err := parseResponse([]byte("Plain text from gemini"))
	require.NoError(t, err)
	assert.Equal(t, "Plain text from gemini", result)
}

func TestParseResponse_Empty(t *testing.T) {
	_, err := parseResponse([]byte(""))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderParseResponse)
}

func TestSendMessageWithTools_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "gemini"}
	_, err := client.SendMessageWithTools(context.Background(), "test", nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

func TestGetModel(t *testing.T) {
	client := &Client{model: "gemini-2.5-flash"}
	assert.Equal(t, "gemini-2.5-flash", client.GetModel())
}

func TestGetMaxTokens(t *testing.T) {
	client := &Client{}
	assert.Equal(t, 0, client.GetMaxTokens())
}

func TestProviderName(t *testing.T) {
	assert.Equal(t, "gemini-cli", ProviderName)
}

func TestWriteMCPSettingsInCwd(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	servers := map[string]schema.MCPServerConfig{
		"test-server": {Command: "echo", Args: []string{"hello"}},
		"auth-server": {Command: "uvx", Args: []string{"pkg@latest"}, Identity: "admin"},
	}

	settingsFile, err := writeMCPSettingsInCwd(servers, "")
	require.NoError(t, err)

	data, err := os.ReadFile(settingsFile)
	require.NoError(t, err)

	var settings geminiSettings
	require.NoError(t, json.Unmarshal(data, &settings))
	assert.Len(t, settings.MCPServers, 2)

	// auth-server should be wrapped with atmos auth exec.
	authEntry := settings.MCPServers["auth-server"]
	assert.Equal(t, "atmos", authEntry.Command)
	assert.Contains(t, authEntry.Args, "auth")
	assert.Contains(t, authEntry.Args, "-i")
	assert.Contains(t, authEntry.Args, "admin")
}

func TestWriteMCPSettingsInCwd_WithToolchainPATH(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	servers := map[string]schema.MCPServerConfig{
		"test": {Command: "uvx", Args: []string{"pkg@latest"}, Env: map[string]string{"KEY": "val"}},
	}

	settingsFile, err := writeMCPSettingsInCwd(servers, "/toolchain/bin")
	require.NoError(t, err)

	data, err := os.ReadFile(settingsFile)
	require.NoError(t, err)

	var settings struct {
		MCPServers map[string]mcpclient.MCPJSONServer `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &settings))
	assert.Contains(t, settings.MCPServers["test"].Env["PATH"], "/toolchain/bin")
}

func TestSendMessageWithToolsAndHistory_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "gemini"}
	_, err := client.SendMessageWithToolsAndHistory(context.Background(), nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

func TestFormatMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What stacks?"},
		{Role: types.RoleAssistant, Content: "You have 4."},
	}
	result := formatMessages(messages)
	assert.Contains(t, result, "What stacks?")
	assert.Contains(t, result, "Assistant: You have 4.")
}

func TestFormatMessages_Empty(t *testing.T) {
	result := formatMessages(nil)
	assert.Empty(t, result)
}

func TestResolveToolchainPATH_NoDeps(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	result := resolveToolchainPATH(atmosConfig)
	assert.Empty(t, result)
}
