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
	Enabled             bool
	Model               string
	APIKeyEnv           string
	MaxTokens           int
	CacheEnabled        bool
	CacheSystemPrompt   bool
	CacheProjectMemory  bool
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
		Enabled:             false,
		Model:               "claude-sonnet-4-20250514",
		APIKeyEnv:           "ANTHROPIC_API_KEY",
		MaxTokens:           DefaultMaxTokens,
		CacheEnabled:        true,  // Enable caching by default
		CacheSystemPrompt:   true,  // Cache system prompt by default
		CacheProjectMemory:  true,  // Cache project memory by default
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

			// Extract cache settings.
			// Default behavior: caching enabled (all true)
			// User can explicitly disable by setting cache.enabled: false in config.

			// Cache is a pointer, so we can distinguish:
			// - nil: User didn't configure caching → use defaults (all true)
			// - non-nil: User explicitly configured caching → process settings.

			if providerConfig.Cache != nil {
				// User explicitly configured cache settings.
				if !providerConfig.Cache.Enabled {
					// Explicitly disabled.
					config.CacheEnabled = false
					config.CacheSystemPrompt = false
					config.CacheProjectMemory = false
				} else {
					// Explicitly enabled - use fine-grained settings.
					config.CacheSystemPrompt = providerConfig.Cache.CacheSystemPrompt
					config.CacheProjectMemory = providerConfig.Cache.CacheProjectMemory

					// If no fine-grained settings provided, default both to true.
					if !config.CacheSystemPrompt && !config.CacheProjectMemory {
						config.CacheSystemPrompt = true
						config.CacheProjectMemory = true
					}
				}
			}
			// If Cache is nil, keep defaults (all true).
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

// SendMessageWithHistory sends messages with full conversation history.
func (c *SimpleClient) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	// Convert messages to Anthropic format.
	anthropicMessages := convertMessagesToAnthropicFormat(messages)

	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.config.Model),
		MaxTokens: int64(c.config.MaxTokens),
		Messages:  anthropicMessages,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send messages with history: %w", err)
	}

	// Extract text from response.
	var responseText string
	for i := range response.Content {
		if response.Content[i].Type == "text" {
			responseText += response.Content[i].Text
		}
	}

	return responseText, nil
}

// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
func (c *SimpleClient) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	// Convert messages to Anthropic format.
	anthropicMessages := convertMessagesToAnthropicFormat(messages)

	// Convert tools to Anthropic format.
	anthropicTools := convertToolsToAnthropicFormat(availableTools)

	// Send message with tools and history.
	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.config.Model),
		MaxTokens: int64(c.config.MaxTokens),
		Messages:  anthropicMessages,
		Tools:     anthropicTools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send messages with history and tools: %w", err)
	}

	// Parse response.
	return parseAnthropicResponse(response)
}

// convertMessagesToAnthropicFormat converts our Message slice to Anthropic's MessageParam format.
func convertMessagesToAnthropicFormat(messages []types.Message) []anthropic.MessageParam {
	anthropicMessages := make([]anthropic.MessageParam, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case types.RoleAssistant:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
			// Note: Anthropic doesn't support system messages in the Messages array.
			// System messages should be passed via the System parameter in MessageNewParams.
			// For now, we skip system messages in the conversation history.
		}
	}

	return anthropicMessages
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
		// IMPORTANT: JSON Schema draft 2020-12 requires the "type" field.
		inputSchema := anthropic.ToolInputSchemaParam{
			Type:       "object",
			Properties: properties,
			Required:   required,
		}

		// Create tool param with description.
		// IMPORTANT: The description field is crucial for Claude to know WHEN to call the tool.
		toolParam := anthropic.ToolParam{
			Name:        tool.Name(),
			Description: anthropic.String(tool.Description()),
			InputSchema: inputSchema,
		}

		// Wrap in ToolUnionParam using OfTool field.
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
			OfTool: &toolParam,
		})
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

	// Extract usage information.
	if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
		result.Usage = &types.Usage{
			InputTokens:         response.Usage.InputTokens,
			OutputTokens:        response.Usage.OutputTokens,
			TotalTokens:         response.Usage.InputTokens + response.Usage.OutputTokens,
			CacheReadTokens:     response.Usage.CacheReadInputTokens,
			CacheCreationTokens: response.Usage.CacheCreationInputTokens,
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

// buildSystemPrompt builds a system prompt text block with optional cache control.
// If caching is enabled, it marks the content for caching.
func (c *SimpleClient) buildSystemPrompt(systemPrompt string, enableCache bool) anthropic.TextBlockParam {
	textBlock := anthropic.TextBlockParam{
		Text: systemPrompt,
	}

	// Add cache control if enabled.
	if c.config.CacheEnabled && enableCache {
		textBlock.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	return textBlock
}

// buildSystemPrompts builds multiple system prompt text blocks with cache control.
// This is useful when you have both agent system prompt and project memory.
func (c *SimpleClient) buildSystemPrompts(prompts []struct {
	content string
	cache   bool
}) []anthropic.TextBlockParam {
	result := make([]anthropic.TextBlockParam, 0, len(prompts))

	for _, prompt := range prompts {
		result = append(result, c.buildSystemPrompt(prompt.content, prompt.cache))
	}

	return result
}

// SendMessageWithSystemPromptAndTools sends messages with system prompt, conversation history, and available tools.
// The system prompt can be cached to reduce API costs (up to 90% for repeated content).
// If atmosMemory is provided, it will be cached separately.
func (c *SimpleClient) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	availableTools []tools.Tool,
) (*types.Response, error) {
	// Convert messages to Anthropic format.
	anthropicMessages := convertMessagesToAnthropicFormat(messages)

	// Convert tools to Anthropic format.
	anthropicTools := convertToolsToAnthropicFormat(availableTools)

	// Build system prompts with cache control.
	var systemPrompts []anthropic.TextBlockParam

	// Add agent system prompt (cached if enabled).
	if systemPrompt != "" {
		systemPrompts = append(systemPrompts, c.buildSystemPrompt(systemPrompt, c.config.CacheSystemPrompt))
	}

	// Add ATMOS.md content (cached if enabled).
	if atmosMemory != "" {
		systemPrompts = append(systemPrompts, c.buildSystemPrompt(atmosMemory, c.config.CacheProjectMemory))
	}

	// Build request params.
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.config.Model),
		MaxTokens: int64(c.config.MaxTokens),
		Messages:  anthropicMessages,
		Tools:     anthropicTools,
	}

	// Add system prompts if any.
	if len(systemPrompts) > 0 {
		params.System = systemPrompts
	}

	// Send message.
	response, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send message with system prompt and tools: %w", err)
	}

	// Parse response.
	return parseAnthropicResponse(response)
}
