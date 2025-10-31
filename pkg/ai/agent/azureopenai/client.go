package azureopenai

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
	// DefaultModel is the default Azure OpenAI model deployment name.
	DefaultModel = "gpt-4o"
	// DefaultAPIKeyEnv is the default environment variable for Azure OpenAI API key.
	DefaultAPIKeyEnv = "AZURE_OPENAI_API_KEY"
	// DefaultAPIVersion is the default Azure OpenAI API version.
	DefaultAPIVersion = "2024-02-15-preview"
)

// Client provides an interface to Azure OpenAI for Atmos.
type Client struct {
	client *openai.Client
	config *Config
}

// Config holds configuration for the Azure OpenAI client.
type Config struct {
	Enabled    bool
	Model      string
	APIKeyEnv  string
	MaxTokens  int
	BaseURL    string
	APIVersion string
}

// NewClient creates a new Azure OpenAI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	// Extract Azure OpenAI configuration.
	cfg := extractConfig(atmosConfig)

	if !cfg.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Validate required fields.
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("%w: base_url is required for Azure OpenAI (format: https://<resource>.openai.azure.com)", errUtils.ErrAIAPIKeyNotFound)
	}

	// Get API key from environment using viper.
	_ = viper.BindEnv(cfg.APIKeyEnv, cfg.APIKeyEnv)
	apiKey := viper.GetString(cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIAPIKeyNotFound, cfg.APIKeyEnv)
	}

	// Create OpenAI client configured for Azure.
	// Azure OpenAI uses api-key header instead of Authorization Bearer.
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(cfg.BaseURL),
		option.WithHeader("api-version", cfg.APIVersion),
	)

	return &Client{
		client: &client,
		config: cfg,
	}, nil
}

// extractConfig extracts Azure OpenAI configuration from AtmosConfiguration.
func extractConfig(atmosConfig *schema.AtmosConfiguration) *Config {
	// Set defaults.
	cfg := &Config{
		Enabled:    false,
		Model:      DefaultModel,
		APIKeyEnv:  DefaultAPIKeyEnv,
		MaxTokens:  DefaultMaxTokens,
		BaseURL:    "",
		APIVersion: DefaultAPIVersion,
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		cfg.Enabled = atmosConfig.Settings.AI.Enabled
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers["azureopenai"]; exists && providerConfig != nil {
			// Override defaults with provider-specific configuration.
			if providerConfig.Model != "" {
				cfg.Model = providerConfig.Model
			}
			if providerConfig.ApiKeyEnv != "" {
				cfg.APIKeyEnv = providerConfig.ApiKeyEnv
			}
			if providerConfig.MaxTokens > 0 {
				cfg.MaxTokens = providerConfig.MaxTokens
			}
			if providerConfig.BaseURL != "" {
				cfg.BaseURL = providerConfig.BaseURL
			}
			// API version can be passed via a custom field if needed in the future.
			// For now, we use the default or could extend schema.AIProviderConfig.
		}
	}

	return cfg
}

// SendMessage sends a message to Azure OpenAI and returns the response.
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
		return "", fmt.Errorf("failed to send message to Azure OpenAI: %w", err)
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithTools sends a message with available tools.
func (c *Client) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	// Convert our tools to OpenAI's format (Azure OpenAI is OpenAI-compatible).
	azureTools := convertToolsToAzureOpenAIFormat(availableTools)

	// Send message with tools.
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model: c.config.Model,
		Tools: azureTools,
	}

	// Set the appropriate token limit parameter based on the model.
	c.setTokenLimit(&params)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send message with tools to Azure OpenAI: %w", err)
	}

	// Parse response.
	return parseAzureOpenAIResponse(response)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	// Convert messages to Azure OpenAI/OpenAI format.
	azureMessages := convertMessagesToAzureOpenAIFormat(messages)

	params := openai.ChatCompletionNewParams{
		Messages: azureMessages,
		Model:    c.config.Model,
	}

	// Set the appropriate token limit parameter based on the model.
	c.setTokenLimit(&params)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to send messages with history to Azure OpenAI: %w", err)
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
func (c *Client) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	// Convert messages to Azure OpenAI/OpenAI format.
	azureMessages := convertMessagesToAzureOpenAIFormat(messages)

	// Convert tools to Azure OpenAI format.
	azureTools := convertToolsToAzureOpenAIFormat(availableTools)

	params := openai.ChatCompletionNewParams{
		Messages: azureMessages,
		Model:    c.config.Model,
		Tools:    azureTools,
	}

	// Set the appropriate token limit parameter based on the model.
	c.setTokenLimit(&params)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send messages with history and tools to Azure OpenAI: %w", err)
	}

	// Parse response.
	return parseAzureOpenAIResponse(response)
}

// SendMessageWithSystemPromptAndTools sends messages with system prompt, conversation history, and available tools.
// For Azure OpenAI, caching happens automatically with 50-100% discount (5-10 min TTL).
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
	// Azure OpenAI automatically caches content with 50-100% discount (5-10 min TTL).
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

// convertMessagesToAzureOpenAIFormat converts our Message slice to Azure OpenAI/OpenAI's message format.
func convertMessagesToAzureOpenAIFormat(messages []types.Message) []openai.ChatCompletionMessageParamUnion {
	azureMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			azureMessages = append(azureMessages, openai.UserMessage(msg.Content))
		case types.RoleAssistant:
			azureMessages = append(azureMessages, openai.AssistantMessage(msg.Content))
		case types.RoleSystem:
			azureMessages = append(azureMessages, openai.SystemMessage(msg.Content))
		}
	}

	return azureMessages
}

// convertToolsToAzureOpenAIFormat converts our Tool interface to Azure OpenAI's function format.
// Since Azure OpenAI uses the OpenAI SDK, the format is identical to OpenAI.
func convertToolsToAzureOpenAIFormat(availableTools []tools.Tool) []openai.ChatCompletionToolParam {
	azureTools := make([]openai.ChatCompletionToolParam, 0, len(availableTools))

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

		azureTools = append(azureTools, toolParam)
	}

	return azureTools
}

// parseAzureOpenAIResponse parses an Azure OpenAI response into our Response format.
// Since Azure OpenAI uses the OpenAI SDK, the format is identical to OpenAI.
func parseAzureOpenAIResponse(response *openai.ChatCompletion) (*types.Response, error) {
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
			// Azure OpenAI doesn't provide cache tokens.
			CacheReadTokens:     0,
			CacheCreationTokens: 0,
		}
	}

	return result, nil
}

// GetModel returns the configured model deployment name.
func (c *Client) GetModel() string {
	return c.config.Model
}

// GetMaxTokens returns the configured max tokens.
func (c *Client) GetMaxTokens() int {
	return c.config.MaxTokens
}

// GetBaseURL returns the configured Azure OpenAI endpoint.
func (c *Client) GetBaseURL() string {
	return c.config.BaseURL
}

// GetAPIVersion returns the configured API version.
func (c *Client) GetAPIVersion() string {
	return c.config.APIVersion
}
