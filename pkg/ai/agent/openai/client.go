package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
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

// Client provides a simplified interface to the OpenAI API for Atmos.
type Client struct {
	client *openai.Client
	config *Config
}

// Config holds basic configuration for the OpenAI client.
type Config struct {
	Enabled   bool
	Model     string
	APIKeyEnv string
	MaxTokens int
}

// NewClient creates a new OpenAI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	// Extract AI configuration
	config := extractConfig(atmosConfig)

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get API key from environment using viper
	_ = viper.BindEnv(config.APIKeyEnv, config.APIKeyEnv)
	apiKey := viper.GetString(config.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIAPIKeyNotFound, config.APIKeyEnv)
	}

	// Create OpenAI client
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &Client{
		client: &client,
		config: config,
	}, nil
}

// extractConfig extracts AI configuration from AtmosConfiguration.
func extractConfig(atmosConfig *schema.AtmosConfiguration) *Config {
	// Set defaults.
	config := &Config{
		Enabled:   false,
		Model:     "gpt-4o",
		APIKeyEnv: "OPENAI_API_KEY",
		MaxTokens: DefaultMaxTokens,
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		config.Enabled = atmosConfig.Settings.AI.Enabled
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers["openai"]; exists && providerConfig != nil {
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
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithTools sends a message with available tools.
func (c *Client) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	// Convert our tools to OpenAI's format.
	openaiTools := convertToolsToOpenAIFormat(availableTools)

	// Send message with tools.
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
		Tools:     openaiTools,
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send message with tools: %w", err)
	}

	// Parse response.
	return parseOpenAIResponse(response)
}

// convertToolsToOpenAIFormat converts our Tool interface to OpenAI's function format.
func convertToolsToOpenAIFormat(availableTools []tools.Tool) []openai.ChatCompletionToolParam {
	openaiTools := make([]openai.ChatCompletionToolParam, 0, len(availableTools))

	for _, tool := range availableTools {
		// Build properties and required fields from parameters.
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

		// Create function parameters.
		params := openai.FunctionParameters{
			"type":       "object",
			"properties": properties,
			"required":   required,
		}

		// Create tool param with function definition.
		toolParam := openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        tool.Name(),
				Description: openai.String(tool.Description()),
				Parameters:  params,
			},
		}

		openaiTools = append(openaiTools, toolParam)
	}

	return openaiTools
}

// parseOpenAIResponse parses an OpenAI response into our Response format.
func parseOpenAIResponse(response *openai.ChatCompletion) (*types.Response, error) {
	result := &types.Response{
		Content:   "",
		ToolCalls: make([]types.ToolCall, 0),
	}

	// Check if we have choices.
	if len(response.Choices) == 0 {
		return nil, errUtils.ErrAINoResponseChoices
	}

	choice := response.Choices[0]

	// Map finish reason to stop reason.
	switch choice.FinishReason {
	case "stop":
		result.StopReason = types.StopReasonEndTurn
	case "tool_calls":
		result.StopReason = types.StopReasonToolUse
	case "length":
		result.StopReason = types.StopReasonMaxTokens
	default:
		result.StopReason = types.StopReasonEndTurn
	}

	// Extract text content.
	result.Content = choice.Message.Content

	// Extract tool calls if present.
	if len(choice.Message.ToolCalls) > 0 {
		for _, toolCall := range choice.Message.ToolCalls {
			// Parse function arguments.
			var args map[string]interface{}
			if toolCall.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
				}
			}

			result.ToolCalls = append(result.ToolCalls, types.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: args,
			})
		}
	}

	return result, nil
}

// GetModel returns the configured model name.
func (c *Client) GetModel() string {
	return c.config.Model
}

// GetMaxTokens returns the configured max tokens.
func (c *Client) GetMaxTokens() int {
	return c.config.MaxTokens
}
