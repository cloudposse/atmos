package anthropic

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// ProviderName is the name of this provider for configuration lookup.
	ProviderName = "anthropic"
	// DefaultMaxTokens is the default maximum number of tokens in AI responses.
	DefaultMaxTokens = 4096
	// DefaultModel is the default Anthropic model.
	DefaultModel = "claude-sonnet-4-20250514"
	// DefaultAPIKeyEnv is the default environment variable for the API key.
	DefaultAPIKeyEnv = "ANTHROPIC_API_KEY"
)

// SimpleClient provides a simplified interface to the Anthropic API for Atmos.
type SimpleClient struct {
	client *anthropic.Client
	config *base.Config
	cache  *cacheConfig
}

// cacheConfig holds Anthropic-specific cache settings.
type cacheConfig struct {
	enabled            bool
	cacheSystemPrompt  bool
	cacheProjectMemory bool
}

// NewSimpleClient creates a new simple AI client from Atmos configuration.
func NewSimpleClient(atmosConfig *schema.AtmosConfiguration) (*SimpleClient, error) {
	defer perf.Track(atmosConfig, "anthropic.NewSimpleClient")()

	// Extract AI configuration using shared utility.
	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get API key from environment using shared utility (replaces viper.BindEnv).
	apiKey := base.GetAPIKey(config.APIKeyEnv)
	if apiKey == "" {
		return nil, errUtils.Build(errUtils.ErrAIAPIKeyNotFound).
			WithContext("env_var", config.APIKeyEnv).
			WithHint("Set the " + config.APIKeyEnv + " environment variable").
			Err()
	}

	// Extract Anthropic-specific cache settings.
	cache := extractCacheConfig(atmosConfig)

	// Create Anthropic client.
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &SimpleClient{
		client: &client,
		config: config,
		cache:  cache,
	}, nil
}

// extractCacheConfig extracts Anthropic-specific cache settings from AtmosConfiguration.
func extractCacheConfig(atmosConfig *schema.AtmosConfiguration) *cacheConfig {
	// Default: caching enabled.
	cache := &cacheConfig{
		enabled:            true,
		cacheSystemPrompt:  true,
		cacheProjectMemory: true,
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers[ProviderName]; exists && providerConfig != nil {
			// Extract cache settings.
			// Default behavior: caching enabled (all true).
			// User can explicitly disable by setting cache.enabled: false in config.
			if providerConfig.Cache != nil {
				// User explicitly configured cache settings.
				if !providerConfig.Cache.Enabled {
					// Explicitly disabled.
					cache.enabled = false
					cache.cacheSystemPrompt = false
					cache.cacheProjectMemory = false
				} else {
					// Explicitly enabled - use fine-grained settings.
					cache.cacheSystemPrompt = providerConfig.Cache.CacheSystemPrompt
					cache.cacheProjectMemory = providerConfig.Cache.CacheProjectMemory

					// If no fine-grained settings provided, default both to true.
					if !cache.cacheSystemPrompt && !cache.cacheProjectMemory {
						cache.cacheSystemPrompt = true
						cache.cacheProjectMemory = true
					}
				}
			}
		}
	}

	return cache
}

// SendMessage sends a message to the AI and returns the response.
func (c *SimpleClient) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "anthropic.SimpleClient.SendMessage")()

	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.config.Model),
		MaxTokens: int64(c.config.MaxTokens),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(message)),
		},
	})
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			Err()
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
	defer perf.Track(nil, "anthropic.SimpleClient.SendMessageWithTools")()

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
		return nil, errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("tools_count", len(availableTools)).
			Err()
	}

	// Parse response.
	return parseAnthropicResponse(response)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *SimpleClient) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "anthropic.SimpleClient.SendMessageWithHistory")()

	// Convert messages to Anthropic format.
	anthropicMessages := convertMessagesToAnthropicFormat(messages)

	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.config.Model),
		MaxTokens: int64(c.config.MaxTokens),
		Messages:  anthropicMessages,
	})
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			Err()
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
	defer perf.Track(nil, "anthropic.SimpleClient.SendMessageWithToolsAndHistory")()

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
		return nil, errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			WithContext("tools_count", len(availableTools)).
			Err()
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
		// Build input schema using shared utility.
		info := base.ExtractToolInfo(tool)

		// Create tool input schema.
		// IMPORTANT: JSON Schema draft 2020-12 requires the "type" field.
		inputSchema := anthropic.ToolInputSchemaParam{
			Type:       "object",
			Properties: info.Properties,
			Required:   info.Required,
		}

		// Create tool param with description.
		// IMPORTANT: The description field is crucial for Claude to know WHEN to call the tool.
		toolParam := anthropic.ToolParam{
			Name:        info.Name,
			Description: anthropic.String(info.Description),
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
					return nil, errUtils.Build(errUtils.ErrAIParseToolInput).
						WithCause(err).
						WithContext("provider", "anthropic").
						WithContext("tool_id", toolUse.ID).
						Err()
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
	if c.cache.enabled && enableCache {
		textBlock.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	return textBlock
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
	defer perf.Track(nil, "anthropic.SimpleClient.SendMessageWithSystemPromptAndTools")()

	// Convert messages to Anthropic format.
	anthropicMessages := convertMessagesToAnthropicFormat(messages)

	// Convert tools to Anthropic format.
	anthropicTools := convertToolsToAnthropicFormat(availableTools)

	// Build system prompts with cache control.
	var systemPrompts []anthropic.TextBlockParam

	// Add agent system prompt (cached if enabled).
	if systemPrompt != "" {
		systemPrompts = append(systemPrompts, c.buildSystemPrompt(systemPrompt, c.cache.cacheSystemPrompt))
	}

	// Add ATMOS.md content (cached if enabled).
	if atmosMemory != "" {
		systemPrompts = append(systemPrompts, c.buildSystemPrompt(atmosMemory, c.cache.cacheProjectMemory))
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
		return nil, errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			WithContext("tools_count", len(availableTools)).
			Err()
	}

	// Parse response.
	return parseAnthropicResponse(response)
}
