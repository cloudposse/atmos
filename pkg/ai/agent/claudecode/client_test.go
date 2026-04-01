package claudecode

import (
	"context"
	"os"
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

// formatMessages tests are in pkg/ai/agent/base/messages_tools_test.go (FormatMessagesAsPrompt).

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

// resolveToolchainPATH tests are in pkg/ai/agent/base/config_test.go (ResolveToolchainPATH).

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

func TestBuildArgs_Basic(t *testing.T) {
	client := &Client{maxTurns: 5}
	args := client.buildArgs("")
	assert.Contains(t, args, "-p")
	assert.Contains(t, args, "--output-format")
	assert.Contains(t, args, "--max-turns")
	assert.Contains(t, args, "5")
	assert.NotContains(t, args, "--max-budget-usd")
	assert.NotContains(t, args, "--append-system-prompt")
	assert.NotContains(t, args, "--mcp-config")
}

func TestBuildArgs_WithBudget(t *testing.T) {
	client := &Client{maxTurns: 5, maxBudget: 2.50}
	args := client.buildArgs("")
	assert.Contains(t, args, "--max-budget-usd")
	assert.Contains(t, args, "2.50")
}

func TestBuildArgs_WithSystemPrompt(t *testing.T) {
	client := &Client{maxTurns: 5}
	args := client.buildArgs("You are an expert")
	assert.Contains(t, args, "--append-system-prompt")
	assert.Contains(t, args, "You are an expert")
}

func TestBuildArgs_WithAllowedTools(t *testing.T) {
	client := &Client{maxTurns: 5, allowedTools: []string{"Read", "Glob"}}
	args := client.buildArgs("")
	// Each tool gets its own --allowedTools flag.
	toolCount := 0
	for _, a := range args {
		if a == "--allowedTools" {
			toolCount++
		}
	}
	assert.Equal(t, 2, toolCount)
	assert.Contains(t, args, "Read")
	assert.Contains(t, args, "Glob")
}

func TestBuildArgs_WithMCPConfig(t *testing.T) {
	client := &Client{maxTurns: 5, mcpConfigPath: "/tmp/mcp.json"}
	args := client.buildArgs("")
	assert.Contains(t, args, "--mcp-config")
	assert.Contains(t, args, "/tmp/mcp.json")
	assert.Contains(t, args, "--dangerously-skip-permissions")
}

func TestBuildArgs_AllOptions(t *testing.T) {
	client := &Client{
		maxTurns:      10,
		maxBudget:     1.00,
		allowedTools:  []string{"Read"},
		mcpConfigPath: "/tmp/mcp.json",
	}
	args := client.buildArgs("system prompt")
	assert.Contains(t, args, "--max-turns")
	assert.Contains(t, args, "--max-budget-usd")
	assert.Contains(t, args, "--append-system-prompt")
	assert.Contains(t, args, "--allowedTools")
	assert.Contains(t, args, "--mcp-config")
	assert.Contains(t, args, "--dangerously-skip-permissions")
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
