package codexcli

import (
	"context"
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

func TestExtractResult_JSONL(t *testing.T) {
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
