package claudecode

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewClient_Disabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{Enabled: false},
	}
	_, err := NewClient(atmosConfig)
	assert.ErrorIs(t, err, errUtils.ErrAIDisabledInConfiguration)
}

func TestNewClient_BinaryNotFound(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {
					Binary: "/nonexistent/path/to/claude-binary-xyz",
				},
			},
		},
	}
	// Binary doesn't exist at that path — but we set it explicitly so LookPath isn't used.
	// The client creation should succeed (it only validates LookPath when binary is empty).
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, "/nonexistent/path/to/claude-binary-xyz", client.binaryPath)
}

func TestNewClient_BinaryNotOnPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {},
			},
		},
	}
	// Override PATH to ensure claude is not found.
	t.Setenv("PATH", t.TempDir())

	_, err := NewClient(atmosConfig)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderBinaryNotFound)
}

func TestNewClient_CustomSettings(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {
					Binary:       "/usr/local/bin/claude",
					MaxTurns:     10,
					MaxBudgetUSD: 2.50,
					AllowedTools: []string{"Read", "Glob"},
				},
			},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/claude", client.binaryPath)
	assert.Equal(t, 10, client.maxTurns)
	assert.Equal(t, 2.50, client.maxBudget)
	assert.Equal(t, []string{"Read", "Glob"}, client.allowedTools)
}

func TestParseResponse_ValidJSON(t *testing.T) {
	input := `{
		"type": "result",
		"subtype": "success",
		"result": "The terraform plan shows 3 resources.",
		"cost_usd": 0.003,
		"is_error": false,
		"session_id": "abc123",
		"num_turns": 1
	}`
	result, err := parseResponse([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "The terraform plan shows 3 resources.", result)
}

func TestParseResponse_ErrorResponse(t *testing.T) {
	input := `{
		"type": "result",
		"subtype": "error",
		"result": "Authentication expired",
		"is_error": true
	}`
	_, err := parseResponse([]byte(input))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderExecFailed)
	assert.Contains(t, err.Error(), "Authentication expired")
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	// Non-JSON output is returned as plain text.
	result, err := parseResponse([]byte("Plain text response"))
	require.NoError(t, err)
	assert.Equal(t, "Plain text response", result)
}

func TestParseResponse_EmptyOutput(t *testing.T) {
	_, err := parseResponse([]byte(""))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderParseResponse)
}

func TestSendMessageWithTools_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "claude"}
	_, err := client.SendMessageWithTools(context.Background(), "test", nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

func TestSendMessageWithToolsAndHistory_NotSupported(t *testing.T) {
	client := &Client{binaryPath: "claude"}
	_, err := client.SendMessageWithToolsAndHistory(context.Background(), nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderToolsNotSupported)
}

func TestGetModel(t *testing.T) {
	client := &Client{model: "claude-code"}
	assert.Equal(t, "claude-code", client.GetModel())
}

func TestGetMaxTokens(t *testing.T) {
	client := &Client{}
	assert.Equal(t, 0, client.GetMaxTokens())
}

func TestFormatMessages(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What stacks do we have?"},
		{Role: types.RoleAssistant, Content: "You have 4 stacks."},
		{Role: types.RoleUser, Content: "Describe the vpc component."},
	}
	result := formatMessages(messages)
	assert.Contains(t, result, "What stacks do we have?")
	assert.Contains(t, result, "Assistant: You have 4 stacks.")
	assert.Contains(t, result, "Describe the vpc component.")
}

func TestFormatMessages_Empty(t *testing.T) {
	result := formatMessages(nil)
	assert.Empty(t, result)
}

func TestApplyProviderConfig_Nil(t *testing.T) {
	client := &Client{maxTurns: 5}
	applyProviderConfig(client, nil)
	assert.Equal(t, 5, client.maxTurns) // Unchanged.
}

func TestApplyProviderConfig_AllFields(t *testing.T) {
	client := &Client{maxTurns: 5}
	applyProviderConfig(client, &schema.AIProviderConfig{
		Binary:       "/custom/claude",
		MaxTurns:     10,
		MaxBudgetUSD: 3.50,
		AllowedTools: []string{"Read", "Write"},
	})
	assert.Equal(t, "/custom/claude", client.binaryPath)
	assert.Equal(t, 10, client.maxTurns)
	assert.Equal(t, 3.50, client.maxBudget)
	assert.Equal(t, []string{"Read", "Write"}, client.allowedTools)
}

func TestApplyProviderConfig_PartialFields(t *testing.T) {
	client := &Client{maxTurns: 5, maxBudget: 1.0}
	applyProviderConfig(client, &schema.AIProviderConfig{
		MaxTurns: 20,
		// Binary, MaxBudgetUSD, AllowedTools not set — should not change.
	})
	assert.Equal(t, "", client.binaryPath)
	assert.Equal(t, 20, client.maxTurns)
	assert.Equal(t, 1.0, client.maxBudget) // Unchanged.
}

func TestParseResponse_CostFields(t *testing.T) {
	input := `{
		"type": "result",
		"result": "Analysis done.",
		"cost_usd": 0.005,
		"total_cost_usd": 0.015,
		"duration_ms": 2500,
		"is_error": false,
		"session_id": "sess123",
		"num_turns": 3
	}`
	result, err := parseResponse([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "Analysis done.", result)
}

func TestProviderName(t *testing.T) {
	assert.Equal(t, "claude-code", ProviderName)
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, "claude", DefaultBinary)
	assert.Equal(t, 5, DefaultMaxTurns)
}

func TestResolveToolchainPATH_NoDeps(t *testing.T) {
	// No .tool-versions or toolchain config — should return empty.
	atmosConfig := &schema.AtmosConfiguration{}
	result := resolveToolchainPATH(atmosConfig)
	assert.Empty(t, result)
}

func TestClient_MCPServers_NotCaptured_WhenEmpty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {Binary: "/usr/local/bin/claude"},
			},
		},
		// No MCP servers configured.
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Nil(t, client.mcpServers)
	assert.Empty(t, client.toolchainPATH)
}

func TestClient_MCPServers_Captured_WhenConfigured(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {Binary: "/usr/local/bin/claude"},
			},
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
	assert.Contains(t, client.mcpServers, "aws-docs")
}

func TestParseResponse_WhitespaceOnly(t *testing.T) {
	_, err := parseResponse([]byte("   \n  \t  "))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderParseResponse)
}

func TestParseResponse_EmptyResult(t *testing.T) {
	input := `{"type": "result", "result": "", "is_error": false}`
	result, err := parseResponse([]byte(input))
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestFormatMessages_SingleUser(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}
	result := formatMessages(messages)
	assert.Equal(t, "Hello", result)
}

func TestFormatMessages_SkipsUnknownRoles(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
		{Role: "system", Content: "System message"},
		{Role: types.RoleAssistant, Content: "Hi"},
	}
	result := formatMessages(messages)
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "Assistant: Hi")
	assert.NotContains(t, result, "System message")
}

func TestNewClient_MCPConfigPath_SetWhenServersConfigured(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {Binary: "/usr/local/bin/claude"},
			},
		},
		MCP: schema.MCPSettings{
			Servers: map[string]schema.MCPServerConfig{
				"test": {Command: "echo", Args: []string{"hello"}},
			},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.NotEmpty(t, client.mcpConfigPath)

	// Clean up temp file.
	if client.mcpConfigPath != "" {
		_ = os.Remove(client.mcpConfigPath)
	}
}

func TestNewClient_DefaultMaxTurns(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: true,
			Providers: map[string]*schema.AIProviderConfig{
				ProviderName: {Binary: "/usr/local/bin/claude"},
			},
		},
	}
	client, err := NewClient(atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxTurns, client.maxTurns)
}
