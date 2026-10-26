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
	defer client.restoreSettings()
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

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"test-server": {Command: "echo", Args: []string{"hello"}},
			"auth-server": {Command: "uvx", Args: []string{"pkg@latest"}, Identity: "admin"},
		},
	}

	settingsFile, err := client.writeMCPSettingsInCwd()
	require.NoError(t, err)
	defer client.restoreSettings()

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

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"test": {Command: "uvx", Args: []string{"pkg@latest"}, Env: map[string]string{"KEY": "val"}},
		},
		toolchainPATH: "/toolchain/bin",
	}

	settingsFile, err := client.writeMCPSettingsInCwd()
	require.NoError(t, err)
	defer client.restoreSettings()

	data, err := os.ReadFile(settingsFile)
	require.NoError(t, err)

	var settings struct {
		MCPServers map[string]mcpclient.MCPJSONServer `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &settings))
	assert.Contains(t, settings.MCPServers["test"].Env["PATH"], "/toolchain/bin")
}

func TestWriteMCPSettingsInCwd_BackupRestore(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create existing settings file.
	geminiDir := filepath.Join(tmpDir, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0o700))
	originalContent := []byte(`{"existing": true}`)
	require.NoError(t, os.WriteFile(filepath.Join(geminiDir, "settings.json"), originalContent, 0o600))

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"test": {Command: "echo", Args: []string{"hello"}},
		},
	}

	_, err := client.writeMCPSettingsInCwd()
	require.NoError(t, err)
	assert.True(t, client.settingsBackedUp)

	// Restore and verify original content is back.
	client.restoreSettings()
	restored, err := os.ReadFile(filepath.Join(geminiDir, "settings.json"))
	require.NoError(t, err)
	assert.Equal(t, originalContent, restored)
}

func TestRestoreSettings_NoBackup(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"test": {Command: "echo", Args: []string{"hello"}},
		},
	}

	_, err := client.writeMCPSettingsInCwd()
	require.NoError(t, err)
	assert.False(t, client.settingsBackedUp)

	// Restore should remove the file since no backup existed.
	client.restoreSettings()
	_, err = os.Stat(filepath.Join(".gemini", "settings.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestSendMessageWithToolsAndHistory_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "gemini"}
	_, err := client.SendMessageWithToolsAndHistory(context.Background(), nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

// formatMessages tests are in pkg/ai/agent/base/messages_tools_test.go (FormatMessagesAsPrompt).

// resolveToolchainPATH tests are in pkg/ai/agent/base/config_test.go (ResolveToolchainPATH).

func TestBuildArgs_Basic(t *testing.T) {
	client := &Client{}
	args := client.buildArgs("test prompt")
	assert.Contains(t, args, "--prompt")
	assert.Contains(t, args, "test prompt")
	assert.Contains(t, args, "--output-format")
	assert.Contains(t, args, "json")
	assert.Contains(t, args, "--approval-mode")
	assert.Contains(t, args, "auto_edit")
	assert.NotContains(t, args, "--allowed-mcp-server-names")
}

func TestBuildArgs_WithModel(t *testing.T) {
	client := &Client{model: "gemini-2.5-flash"}
	args := client.buildArgs("test")
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "gemini-2.5-flash")
}

func TestBuildArgs_ModelSameAsProvider(t *testing.T) {
	client := &Client{model: ProviderName}
	args := client.buildArgs("test")
	assert.NotContains(t, args, "-m")
}

func TestBuildArgs_WithMCPServers(t *testing.T) {
	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"aws-billing": {Command: "uvx"},
			"aws-docs":    {Command: "uvx"},
		},
	}
	args := client.buildArgs("test")
	assert.Contains(t, args, "--allowed-mcp-server-names")
	// Server names should be sorted.
	for i, a := range args {
		if a == "--allowed-mcp-server-names" {
			assert.Equal(t, "aws-billing,aws-docs", args[i+1])
		}
	}
}

func TestFilterStderr_DeprecationWarnings(t *testing.T) {
	stderr := `(node:1234) [DEP0040] DeprecationWarning: The punycode module is deprecated.
(Use node --trace-deprecation ... to show where the warning was created)
Loaded cached credentials.
Actual error message here`
	result := filterStderr(stderr)
	assert.Equal(t, "Actual error message here", result)
}

func TestFilterStderr_YOLOMode(t *testing.T) {
	stderr := "YOLO mode enabled\nSome real error"
	result := filterStderr(stderr)
	assert.Equal(t, "Some real error", result)
}

func TestFilterStderr_AllFiltered(t *testing.T) {
	stderr := `(node:1234) DeprecationWarning: something
Loaded cached credentials.
YOLO mode enabled`
	result := filterStderr(stderr)
	assert.Empty(t, result)
}

func TestFilterStderr_EmptyInput(t *testing.T) {
	result := filterStderr("")
	assert.Empty(t, result)
}

func TestFilterStderr_MeaningfulOnly(t *testing.T) {
	stderr := "Error: authentication failed\nConnection refused"
	result := filterStderr(stderr)
	assert.Equal(t, "Error: authentication failed\nConnection refused", result)
}

func TestParseResponse_JSONWithResponseField(t *testing.T) {
	input := `{"session_id": "abc123", "response": "The VPC is configured."}`
	result, err := parseResponse([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "The VPC is configured.", result)
}

func TestParseResponse_WhitespaceOnly(t *testing.T) {
	_, err := parseResponse([]byte("   \n  \t  "))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderParseResponse)
}

func TestNewClient_DefaultModel(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {Binary: "/usr/local/bin/gemini"}},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	// Model defaults to provider name when no model is specified.
	assert.Equal(t, ProviderName, client.model)
}
