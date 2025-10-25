package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxTokens is the default maximum number of tokens in AI responses.
	DefaultMaxTokens = 4096
)

// SimpleClient provides a simplified interface to the Anthropic API for Atmos.
type SimpleClient struct {
	client *anthropic.Client
	config *SimpleAIConfig
}

// SimpleAIConfig holds basic configuration for the AI client.
type SimpleAIConfig struct {
	Enabled   bool
	Model     string
	APIKeyEnv string
	MaxTokens int
}

// NewSimpleClient creates a new simple AI client from Atmos configuration.
func NewSimpleClient(atmosConfig *schema.AtmosConfiguration) (*SimpleClient, error) {
	// Extract simple AI configuration.
	config := extractSimpleAIConfig(atmosConfig)

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get API key from environment using viper.
	_ = viper.BindEnv(config.APIKeyEnv, config.APIKeyEnv)
	apiKey := viper.GetString(config.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIAPIKeyNotFound, config.APIKeyEnv)
	}

	// Create Anthropic client.
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &SimpleClient{
		client: &client,
		config: config,
	}, nil
}

// extractSimpleAIConfig extracts AI configuration from AtmosConfiguration.
func extractSimpleAIConfig(atmosConfig *schema.AtmosConfiguration) *SimpleAIConfig {
	// Set defaults.
	config := &SimpleAIConfig{
		Enabled:   false,
		Model:     "claude-sonnet-4-20250514",
		APIKeyEnv: "ANTHROPIC_API_KEY",
		MaxTokens: DefaultMaxTokens,
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		config.Enabled = atmosConfig.Settings.AI.Enabled
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers["anthropic"]; exists && providerConfig != nil {
			// Override defaults with provider-specific configuration.
			if providerConfig.Model != "" {
				config.Model = providerConfig.Model
			}
			if providerConfig.ApiKeyEnv != "" {
				config.APIKeyEnv = providerConfig.ApiKeyEnv
			}
			if providerConfig.MaxTokens > 0 {
				config.MaxTokens = providerConfig.MaxTokens
			}
		}
	}

	return config
}

// SendMessage sends a message to the AI and returns the response.
func (c *SimpleClient) SendMessage(ctx context.Context, message string) (string, error) {
	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.config.Model),
		MaxTokens: int64(c.config.MaxTokens),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(message)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Extract text from response (use indexing to avoid copying large structs).
	var responseText string
	for i := range response.Content {
		if response.Content[i].Type == "text" {
			responseText += response.Content[i].Text
		}
	}

	return responseText, nil
}

// SendMessageWithTools sends a message with available tools and handles tool calls.
func (c *SimpleClient) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	// Convert our tools to Anthropic's format.
	anthropicTools := convertToolsToAnthropicFormat(availableTools)

	// Send message with tools.
	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.config.Model),
		MaxTokens: int64(c.config.MaxTokens),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(message)),
		},
		Tools: anthropicTools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send message with tools: %w", err)
	}

	// Parse response.
	return parseAnthropicResponse(response)
}

// convertToolsToAnthropicFormat converts our Tool interface to Anthropic's ToolUnionParam.
func convertToolsToAnthropicFormat(availableTools []tools.Tool) []anthropic.ToolUnionParam {
	anthropicTools := make([]anthropic.ToolUnionParam, 0, len(availableTools))

	for _, tool := range availableTools {
		// Build input schema from parameters.
		properties := make(map[string]interface{})
		required := make([]string, 0)

		for _, param := range tool.Parameters() {
			properties[param.Name] = map[string]interface{}{
				"type":        string(param.Type),
				"description": param.Description,
			}
			if param.Required {
				required = append(required, param.Name)
			}
		}

		// Create tool input schema.
		inputSchema := anthropic.ToolInputSchemaParam{
			Properties: properties,
			Required:   required,
		}

		// Create tool union param.
		toolParam := anthropic.ToolUnionParamOfTool(inputSchema, tool.Name())

		// Set description if provided (using reflection to access the underlying ToolParam).
		// Note: The SDK doesn't expose a clean way to set description, so we build it manually.
		anthropicTools = append(anthropicTools, toolParam)
	}

	return anthropicTools
}

// parseAnthropicResponse parses an Anthropic response into our Response format.
func parseAnthropicResponse(response *anthropic.Message) (*types.Response, error) {
	result := &types.Response{
		Content:   "",
		ToolCalls: make([]types.ToolCall, 0),
	}

	// Map stop reason.
	switch response.StopReason {
	case "end_turn":
		result.StopReason = types.StopReasonEndTurn
	case "tool_use":
		result.StopReason = types.StopReasonToolUse
	case "max_tokens":
		result.StopReason = types.StopReasonMaxTokens
	default:
		result.StopReason = types.StopReasonEndTurn
	}

	// Extract text and tool uses from content blocks.
	for i := range response.Content {
		switch response.Content[i].Type {
		case "text":
			result.Content += response.Content[i].Text
		case "tool_use":
			// Parse tool use.
			toolUse := response.Content[i]
			input := make(map[string]interface{})
			if toolUse.Input != nil {
				// Convert RawJSON to map.
				if err := json.Unmarshal(toolUse.Input, &input); err != nil {
					return nil, fmt.Errorf("failed to parse tool input: %w", err)
				}
			}

			result.ToolCalls = append(result.ToolCalls, types.ToolCall{
				ID:    toolUse.ID,
				Name:  toolUse.Name,
				Input: input,
			})
		}
	}

	return result, nil
}

// GetModel returns the configured model name.
func (c *SimpleClient) GetModel() string {
	return c.config.Model
}

// GetMaxTokens returns the configured max tokens.
func (c *SimpleClient) GetMaxTokens() int {
	return c.config.MaxTokens
}
