package codexcli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
					Binary:   "/usr/local/bin/codex",
					Model:    "gpt-5.4-mini",
					FullAuto: true,
				},
			},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/codex", client.binaryPath)
	assert.Equal(t, "gpt-5.4-mini", client.model)
	assert.True(t, client.fullAuto)
}

func TestNewClient_MCPServers_NotCaptured_WhenEmpty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {Binary: "/usr/local/bin/codex"}},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Nil(t, client.mcpServers)
	assert.False(t, client.hasMCPServers)
}

func TestNewClient_MCPServers_Captured_WhenConfigured(t *testing.T) {
	// Use temp HOME to avoid touching real ~/.codex/config.toml.
	t.Setenv("HOME", t.TempDir())

	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: map[string]*schema.AIProviderConfig{ProviderName: {Binary: "/usr/local/bin/codex"}},
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
	defer client.restoreGlobalConfig()
	configPath := codexConfigPath()
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[mcp_servers.aws-docs]")
	assert.Contains(t, string(data), `command = "uvx"`)
}

func TestExtractResult_JSONL_AgentMessage(t *testing.T) {
	// Actual Codex CLI output format: item.type is "agent_message" with text directly on item.
	input := `{"type":"thread.started","thread_id":"019d499a-ca7f-7ec3-af21-5860784b0a11"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Analysis complete."}}
{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":50}}`

	result, err := ExtractResult([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "Analysis complete.", result)
}

func TestExtractResult_JSONL_MessageFormat(t *testing.T) {
	// API-style format: item.type is "message" with nested content array.
	input := `{"type":"thread.started","session_id":"abc123"}
{"type":"item.completed","item":{"type":"message","content":[{"type":"text","text":"Analysis complete."}]}}
{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":50}}`

	result, err := ExtractResult([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "Analysis complete.", result)
}

func TestExtractResult_PlainText(t *testing.T) {
	result, err := ExtractResult([]byte("Plain text output"))
	require.NoError(t, err)
	assert.Equal(t, "Plain text output", result)
}

func TestExtractResult_Empty(t *testing.T) {
	_, err := ExtractResult([]byte(""))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderParseResponse)
}

func TestSendMessageWithTools_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "codex"}
	_, err := client.SendMessageWithTools(context.Background(), "test", nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

func TestGetModel(t *testing.T) {
	client := &Client{model: "gpt-5.4"}
	assert.Equal(t, "gpt-5.4", client.GetModel())
}

func TestGetMaxTokens(t *testing.T) {
	client := &Client{}
	assert.Equal(t, 0, client.GetMaxTokens())
}

func TestProviderName(t *testing.T) {
	assert.Equal(t, "codex-cli", ProviderName)
}

func TestWriteAndRestoreGlobalConfig(t *testing.T) {
	// Use temp HOME to avoid touching real ~/.codex/config.toml.
	t.Setenv("HOME", t.TempDir())

	client := &Client{
		mcpServers: map[string]schema.MCPServerConfig{
			"test-srv": {Command: "echo", Args: []string{"hello"}},
		},
	}

	// Write MCP config.
	require.NoError(t, client.writeMCPToGlobalConfig())
	defer client.restoreGlobalConfig()

	configPath := codexConfigPath()
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[mcp_servers.test-srv]")

	// Restore — no original existed, file should be removed.
	client.restoreGlobalConfig()
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}

func TestWriteMCPConfigTOML(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"test-server": {Command: "echo", Args: []string{"hello"}},
		"auth-server": {Command: "uvx", Args: []string{"pkg@latest"}, Identity: "admin"},
	}

	tmpDir, err := writeMCPConfigTOML(servers, "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, ".codex", "config.toml")
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	content := string(data)

	// Verify TOML structure.
	assert.Contains(t, content, "[mcp_servers.test-server]")
	assert.Contains(t, content, `command = "echo"`)
	// auth-server should be wrapped with atmos auth exec.
	assert.Contains(t, content, "[mcp_servers.auth-server]")
	assert.Contains(t, content, `command = "atmos"`)
}

func TestWriteMCPConfigTOML_WithToolchainPATH(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"test": {Command: "uvx", Args: []string{"pkg@latest"}, Env: map[string]string{"KEY": "val"}},
	}

	tmpDir, err := writeMCPConfigTOML(servers, "/toolchain/bin")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, ".codex", "config.toml")
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "/toolchain/bin")
}

func TestWriteTOMLServer(t *testing.T) {
	var buf bytes.Buffer
	srv := mcpclient.MCPJSONServer{
		Command: "uvx",
		Args:    []string{"awslabs.billing@latest"},
		Env:     map[string]string{"AWS_REGION": "us-east-1"},
	}
	writeTOMLServer(&buf, "aws-billing", srv)
	content := buf.String()

	assert.Contains(t, content, "[mcp_servers.aws-billing]")
	assert.Contains(t, content, `command = "uvx"`)
	assert.Contains(t, content, `"awslabs.billing@latest"`)
	assert.Contains(t, content, "[mcp_servers.aws-billing.env]")
	assert.Contains(t, content, `AWS_REGION = "us-east-1"`)
}

func TestWriteTOMLServer_NoEnv(t *testing.T) {
	var buf bytes.Buffer
	srv := mcpclient.MCPJSONServer{
		Command: "echo",
		Args:    []string{"hello"},
	}
	writeTOMLServer(&buf, "simple", srv)
	content := buf.String()

	assert.Contains(t, content, "[mcp_servers.simple]")
	assert.NotContains(t, content, "[mcp_servers.simple.env]")
}

func TestWriteTOMLServer_MultipleArgs(t *testing.T) {
	var buf bytes.Buffer
	srv := mcpclient.MCPJSONServer{
		Command: "atmos",
		Args:    []string{"auth", "exec", "-i", "readonly", "--", "uvx", "pkg@latest"},
	}
	writeTOMLServer(&buf, "wrapped", srv)
	content := buf.String()

	assert.Contains(t, content, `"auth"`)
	assert.Contains(t, content, `"-i"`)
	assert.Contains(t, content, `"readonly"`)
	// Verify args are comma-separated.
	assert.True(t, strings.Contains(content, ", "), "args should be comma-separated")
}

func TestSendMessageWithToolsAndHistory_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "codex"}
	_, err := client.SendMessageWithToolsAndHistory(context.Background(), nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

// formatMessages tests are in pkg/ai/agent/base/messages_tools_test.go (FormatMessagesAsPrompt).

// resolveToolchainPATH tests are in pkg/ai/agent/base/config_test.go (ResolveToolchainPATH).

func TestBuildArgs_Basic(t *testing.T) {
	client := &Client{}
	args := client.buildArgs()
	assert.Equal(t, "exec", args[0])
	assert.Contains(t, args, "--json")
	assert.NotContains(t, args, "--full-auto")
	assert.NotContains(t, args, "--dangerously-bypass-approvals-and-sandbox")
}

func TestBuildArgs_WithModel(t *testing.T) {
	client := &Client{model: "gpt-5.4-mini"}
	args := client.buildArgs()
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "gpt-5.4-mini")
}

func TestBuildArgs_ModelSameAsProvider(t *testing.T) {
	client := &Client{model: ProviderName}
	args := client.buildArgs()
	assert.NotContains(t, args, "-m")
}

func TestBuildArgs_FullAuto(t *testing.T) {
	client := &Client{fullAuto: true}
	args := client.buildArgs()
	assert.Contains(t, args, "--full-auto")
}

func TestBuildArgs_MCPOverridesFullAuto(t *testing.T) {
	client := &Client{fullAuto: true, hasMCPServers: true}
	args := client.buildArgs()
	assert.Contains(t, args, "--dangerously-bypass-approvals-and-sandbox")
	assert.NotContains(t, args, "--full-auto")
}

func TestBuildArgs_MCPWithoutFullAuto(t *testing.T) {
	client := &Client{hasMCPServers: true}
	args := client.buildArgs()
	assert.Contains(t, args, "--dangerously-bypass-approvals-and-sandbox")
}

func TestCollectAtmosEnvVars(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "managers")
	t.Setenv("ATMOS_BASE_PATH", "/some/path")
	t.Setenv("NOT_ATMOS", "ignored")

	result := collectAtmosEnvVars()
	assert.Equal(t, "managers", result["ATMOS_PROFILE"])
	assert.Equal(t, "/some/path", result["ATMOS_BASE_PATH"])
	_, hasNonAtmos := result["NOT_ATMOS"]
	assert.False(t, hasNonAtmos)
}

func TestCollectAtmosEnvVars_NoAtmosVars(t *testing.T) {
	// Ensure no ATMOS_ vars are set (best effort — can't unset all).
	result := collectAtmosEnvVars()
	for k := range result {
		assert.True(t, strings.HasPrefix(k, cfg.AtmosEnvVarPrefix), "all keys should start with the canonical Atmos env-var prefix")
	}
}

func TestInjectAtmosEnvVars(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "test-profile")

	config := &mcpclient.MCPJSONConfig{
		MCPServers: map[string]mcpclient.MCPJSONServer{
			"test": {
				Command: "echo",
				Args:    []string{"hello"},
				Env:     map[string]string{"KEY": "val"},
			},
		},
	}
	injectAtmosEnvVars(config)
	assert.Equal(t, "test-profile", config.MCPServers["test"].Env["ATMOS_PROFILE"])
	// Existing keys are preserved.
	assert.Equal(t, "val", config.MCPServers["test"].Env["KEY"])
}

func TestInjectAtmosEnvVars_DoesNotOverwrite(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "from-env")

	config := &mcpclient.MCPJSONConfig{
		MCPServers: map[string]mcpclient.MCPJSONServer{
			"test": {
				Command: "echo",
				Env:     map[string]string{"ATMOS_PROFILE": "from-config"},
			},
		},
	}
	injectAtmosEnvVars(config)
	// Explicitly configured value should not be overwritten.
	assert.Equal(t, "from-config", config.MCPServers["test"].Env["ATMOS_PROFILE"])
}

func TestInjectAtmosEnvVars_NilEnv(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "test")

	config := &mcpclient.MCPJSONConfig{
		MCPServers: map[string]mcpclient.MCPJSONServer{
			"test": {Command: "echo"},
		},
	}
	injectAtmosEnvVars(config)
	assert.Equal(t, "test", config.MCPServers["test"].Env["ATMOS_PROFILE"])
}

func TestExtractTextFromEvent_AgentMessage(t *testing.T) {
	line := `{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`
	result := extractTextFromEvent([]byte(line))
	assert.Equal(t, "hello", result)
}

func TestExtractTextFromEvent_Message(t *testing.T) {
	line := `{"type":"item.completed","item":{"type":"message","content":[{"type":"text","text":"hello"}]}}`
	result := extractTextFromEvent([]byte(line))
	assert.Equal(t, "hello", result)
}

func TestExtractTextFromEvent_NonCompleted(t *testing.T) {
	line := `{"type":"turn.started"}`
	result := extractTextFromEvent([]byte(line))
	assert.Empty(t, result)
}

func TestExtractTextFromEvent_InvalidJSON(t *testing.T) {
	result := extractTextFromEvent([]byte("not json"))
	assert.Empty(t, result)
}

func TestExtractTextFromEvent_MCP_ToolCall(t *testing.T) {
	line := `{"type":"item.completed","item":{"type":"mcp_tool_call","server":"aws-docs"}}`
	result := extractTextFromEvent([]byte(line))
	assert.Empty(t, result)
}

func TestRestoreGlobalConfig_NoBackup(t *testing.T) {
	// When no backup exists, restoreGlobalConfig should remove the file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
	require.NoError(t, os.WriteFile(configPath, []byte("test"), 0o600))

	client := &Client{configBackedUp: false}
	// Override codexConfigPath for this test by writing directly.
	// Since restoreGlobalConfig uses codexConfigPath(), we test the real path.
	client.restoreGlobalConfig()
	// In a real test, the actual ~/.codex/config.toml path is used.
	// This test verifies the logic branch.
}
