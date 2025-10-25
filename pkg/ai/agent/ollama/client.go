package ollama

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
	// DefaultModel is the default Ollama model.
	DefaultModel = "llama3.3:70b"
	// DefaultBaseURL is the default Ollama API endpoint.
	DefaultBaseURL = "http://localhost:11434/v1"
	// DefaultAPIKeyEnv is the environment variable for the API key (optional for local Ollama).
	DefaultAPIKeyEnv = "OLLAMA_API_KEY"
)

// Client provides a simplified interface to the Ollama API for Atmos.
// Ollama provides an OpenAI-compatible API, so we use the OpenAI Go SDK.
type Client struct {
	client *openai.Client
	config *Config
}

// Config holds configuration for the Ollama client.
type Config struct {
	Enabled   bool
	Model     string
	APIKeyEnv string
	MaxTokens int
	BaseURL   string
}

// NewClient creates a new Ollama client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	// Extract AI configuration.
	config := extractConfig(atmosConfig)

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get API key from environment using viper (optional for local Ollama).
	_ = viper.BindEnv(config.APIKeyEnv, config.APIKeyEnv)
	apiKey := viper.GetString(config.APIKeyEnv)

	// Create Ollama client using OpenAI-compatible API.
	// If no API key is provided, use a dummy key (Ollama doesn't require auth for local usage).
	if apiKey == "" {
		apiKey = "ollama" // Dummy key for local Ollama instances
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(config.BaseURL),
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
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultBaseURL,
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		config.Enabled = atmosConfig.Settings.AI.Enabled
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers["ollama"]; exists && providerConfig != nil {
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
			if providerConfig.BaseURL != "" {
				config.BaseURL = providerConfig.BaseURL
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
		return "", fmt.Errorf("failed to send message to Ollama: %w", err)
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithTools sends a message with available tools.
func (c *Client) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	// Convert our tools to OpenAI's format (Ollama is OpenAI-compatible).
	ollamaTools := convertToolsToOllamaFormat(availableTools)

	// Send message with tools.
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
		Tools:     ollamaTools,
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send message with tools to Ollama: %w", err)
	}

	// Parse response.
	return parseOllamaResponse(response)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	// Convert messages to Ollama/OpenAI format.
	ollamaMessages := convertMessagesToOllamaFormat(messages)

	params := openai.ChatCompletionNewParams{
		Messages:  ollamaMessages,
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to send messages with history to Ollama: %w", err)
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
func (c *Client) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	// Convert messages to Ollama/OpenAI format.
	ollamaMessages := convertMessagesToOllamaFormat(messages)

	// Convert tools to Ollama format.
	ollamaTools := convertToolsToOllamaFormat(availableTools)

	params := openai.ChatCompletionNewParams{
		Messages:  ollamaMessages,
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
		Tools:     ollamaTools,
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send messages with history and tools to Ollama: %w", err)
	}

	// Parse response.
	return parseOllamaResponse(response)
}

// convertMessagesToOllamaFormat converts our Message slice to Ollama/OpenAI's message format.
func convertMessagesToOllamaFormat(messages []types.Message) []openai.ChatCompletionMessageParamUnion {
	ollamaMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			ollamaMessages = append(ollamaMessages, openai.UserMessage(msg.Content))
		case types.RoleAssistant:
			ollamaMessages = append(ollamaMessages, openai.AssistantMessage(msg.Content))
		case types.RoleSystem:
			ollamaMessages = append(ollamaMessages, openai.SystemMessage(msg.Content))
		}
	}

	return ollamaMessages
}

// convertToolsToOllamaFormat converts our Tool interface to Ollama's function format.
// Since Ollama uses the OpenAI-compatible API, the format is identical to OpenAI.
func convertToolsToOllamaFormat(availableTools []tools.Tool) []openai.ChatCompletionToolParam {
	ollamaTools := make([]openai.ChatCompletionToolParam, 0, len(availableTools))

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

		ollamaTools = append(ollamaTools, toolParam)
	}

	return ollamaTools
}

// parseOllamaResponse parses an Ollama response into our Response format.
// Since Ollama uses the OpenAI-compatible API, the format is identical to OpenAI.
func parseOllamaResponse(response *openai.ChatCompletion) (*types.Response, error) {
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
