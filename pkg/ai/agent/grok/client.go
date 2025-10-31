package grok

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

// Client provides a simplified interface to the xAI Grok API for Atmos.
// Grok API is OpenAI-compatible, so we use the OpenAI SDK with a custom base URL.
type Client struct {
	client *openai.Client
	config *Config
}

// Config holds basic configuration for the Grok client.
type Config struct {
	Enabled   bool
	Model     string
	APIKeyEnv string
	MaxTokens int
	BaseURL   string
}

// NewClient creates a new Grok client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	// Extract AI configuration.
	config := extractConfig(atmosConfig)

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get API key from environment using viper.
	_ = viper.BindEnv(config.APIKeyEnv, config.APIKeyEnv)
	apiKey := viper.GetString(config.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIAPIKeyNotFound, config.APIKeyEnv)
	}

	// Create OpenAI client with Grok's base URL.
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
		Model:     "grok-4-latest",
		APIKeyEnv: "XAI_API_KEY",
		MaxTokens: DefaultMaxTokens,
		BaseURL:   "https://api.x.ai/v1",
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		config.Enabled = atmosConfig.Settings.AI.Enabled
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers["grok"]; exists && providerConfig != nil {
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
		Model: c.config.Model,
	}

	// Set the appropriate token limit parameter based on the model.
	c.setTokenLimit(&params)

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
	// Convert our tools to OpenAI's format (Grok is OpenAI-compatible).
	grokTools := convertToolsToGrokFormat(availableTools)

	// Send message with tools.
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model: c.config.Model,
		Tools: grokTools,
	}

	// Set the appropriate token limit parameter based on the model.
	c.setTokenLimit(&params)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send message with tools: %w", err)
	}

	// Parse response.
	return parseGrokResponse(response)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	// Convert messages to Grok/OpenAI format.
	grokMessages := convertMessagesToGrokFormat(messages)

	params := openai.ChatCompletionNewParams{
		Messages: grokMessages,
		Model:    c.config.Model,
	}

	// Set the appropriate token limit parameter based on the model.
	c.setTokenLimit(&params)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to send messages with history: %w", err)
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
func (c *Client) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	// Convert messages to Grok/OpenAI format.
	grokMessages := convertMessagesToGrokFormat(messages)

	// Convert tools to Grok format.
	grokTools := convertToolsToGrokFormat(availableTools)

	params := openai.ChatCompletionNewParams{
		Messages: grokMessages,
		Model:    c.config.Model,
		Tools:    grokTools,
	}

	// Set the appropriate token limit parameter based on the model.
	c.setTokenLimit(&params)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send messages with history and tools: %w", err)
	}

	// Parse response.
	return parseGrokResponse(response)
}

// SendMessageWithSystemPromptAndTools sends messages with system prompt, conversation history, and available tools.
// For Grok, caching happens automatically with 75% discount and >90% hit rate.
// The system prompt and atmosMemory are prepended as system messages.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	availableTools []tools.Tool,
) (*types.Response, error) {
	// Build messages with system prompts prepended.
	systemMessages := make([]types.Message, 0, 2+len(messages))

	// Add system prompt if provided.
	if systemPrompt != "" {
		systemMessages = append(systemMessages, types.Message{
			Role:    types.RoleSystem,
			Content: systemPrompt,
		})
	}

	// Add ATMOS.md content if provided.
	if atmosMemory != "" {
		systemMessages = append(systemMessages, types.Message{
			Role:    types.RoleSystem,
			Content: atmosMemory,
		})
	}

	// Add conversation history.
	systemMessages = append(systemMessages, messages...)

	// Call existing method with system messages prepended.
	// Grok automatically caches content with 75% discount and >90% hit rate.
	return c.SendMessageWithToolsAndHistory(ctx, systemMessages, availableTools)
}

// setTokenLimit sets the appropriate token limit parameter based on the model.
// Newer models (gpt-5, o1-preview, o1-mini, chatgpt-4o-latest) use max_completion_tokens,
// while older models use max_tokens.
func (c *Client) setTokenLimit(params *openai.ChatCompletionNewParams) {
	// Models that require max_completion_tokens instead of max_tokens.
	usesMaxCompletionTokens := c.requiresMaxCompletionTokens()

	if usesMaxCompletionTokens {
		params.MaxCompletionTokens = openai.Int(int64(c.config.MaxTokens))
	} else {
		params.MaxTokens = openai.Int(int64(c.config.MaxTokens))
	}
}

// requiresMaxCompletionTokens returns true if the model requires max_completion_tokens parameter.
func (c *Client) requiresMaxCompletionTokens() bool {
	model := c.config.Model

	// Check for models that use max_completion_tokens.
	// These include: gpt-5*, o1-preview, o1-mini, chatgpt-4o-latest.
	if len(model) >= 5 && model[:5] == "gpt-5" {
		return true
	}
	if model == "o1-preview" || model == "o1-mini" || model == "chatgpt-4o-latest" {
		return true
	}

	return false
}

// convertMessagesToGrokFormat converts our Message slice to Grok/OpenAI's message format.
func convertMessagesToGrokFormat(messages []types.Message) []openai.ChatCompletionMessageParamUnion {
	grokMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			grokMessages = append(grokMessages, openai.UserMessage(msg.Content))
		case types.RoleAssistant:
			grokMessages = append(grokMessages, openai.AssistantMessage(msg.Content))
		case types.RoleSystem:
			grokMessages = append(grokMessages, openai.SystemMessage(msg.Content))
		}
	}

	return grokMessages
}

// convertToolsToGrokFormat converts our Tool interface to Grok's function format.
// Since Grok uses the OpenAI SDK, the format is identical to OpenAI.
func convertToolsToGrokFormat(availableTools []tools.Tool) []openai.ChatCompletionToolParam {
	grokTools := make([]openai.ChatCompletionToolParam, 0, len(availableTools))

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

		grokTools = append(grokTools, toolParam)
	}

	return grokTools
}

// parseGrokResponse parses a Grok response into our Response format.
// Since Grok uses the OpenAI SDK, the format is identical to OpenAI.
func parseGrokResponse(response *openai.ChatCompletion) (*types.Response, error) {
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

	// Extract usage information.
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		result.Usage = &types.Usage{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CompletionTokens,
			TotalTokens:  response.Usage.TotalTokens,
			// Grok doesn't provide cache tokens.
			CacheReadTokens:     0,
			CacheCreationTokens: 0,
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

// GetBaseURL returns the configured base URL.
func (c *Client) GetBaseURL() string {
	return c.config.BaseURL
}
