package grok

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/agent/base/openaicompat"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// ProviderName is the name of this provider for configuration lookup.
	ProviderName = "grok"
	// DefaultMaxTokens is the default maximum number of tokens in AI responses.
	DefaultMaxTokens = 4096
	// DefaultModel is the default Grok model.
	DefaultModel = "grok-4-latest"
	// DefaultAPIKeyEnv is the default environment variable for the API key.
	DefaultAPIKeyEnv = "XAI_API_KEY"
	// DefaultBaseURL is the default xAI API endpoint.
	DefaultBaseURL = "https://api.x.ai/v1"
)

// Client provides a simplified interface to the xAI Grok API for Atmos.
// Grok API is OpenAI-compatible, so we use the OpenAI SDK with a custom base URL.
type Client struct {
	client *openai.Client
	config *base.Config
}

// NewClient creates a new Grok client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "grok.NewClient")()

	// Extract AI configuration using shared utility.
	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultBaseURL,
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

// SendMessage sends a message to the AI and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "grok.Client.SendMessage")()

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model: c.config.Model,
	}

	// Set the appropriate token limit parameter based on the model.
	openaicompat.SetTokenLimit(&params, c.config.Model, c.config.MaxTokens)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			Err()
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithTools sends a message with available tools.
func (c *Client) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	defer perf.Track(nil, "grok.Client.SendMessageWithTools")()

	// Convert our tools to OpenAI's format using shared utility.
	grokTools := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	// Send message with tools.
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model: c.config.Model,
		Tools: grokTools,
	}

	// Set the appropriate token limit parameter based on the model.
	openaicompat.SetTokenLimit(&params, c.config.Model, c.config.MaxTokens)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("tools_count", len(availableTools)).
			Err()
	}

	// Parse response using shared utility.
	return openaicompat.ParseOpenAIResponse(response)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "grok.Client.SendMessageWithHistory")()

	// Convert messages to OpenAI format using shared utility.
	grokMessages := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	params := openai.ChatCompletionNewParams{
		Messages: grokMessages,
		Model:    c.config.Model,
	}

	// Set the appropriate token limit parameter based on the model.
	openaicompat.SetTokenLimit(&params, c.config.Model, c.config.MaxTokens)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			Err()
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", errUtils.ErrAINoResponseChoices
	}

	return response.Choices[0].Message.Content, nil
}

// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
func (c *Client) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	defer perf.Track(nil, "grok.Client.SendMessageWithToolsAndHistory")()

	// Convert messages to OpenAI format using shared utility.
	grokMessages := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	// Convert tools to OpenAI format using shared utility.
	grokTools := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	params := openai.ChatCompletionNewParams{
		Messages: grokMessages,
		Model:    c.config.Model,
		Tools:    grokTools,
	}

	// Set the appropriate token limit parameter based on the model.
	openaicompat.SetTokenLimit(&params, c.config.Model, c.config.MaxTokens)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			WithContext("tools_count", len(availableTools)).
			Err()
	}

	// Parse response using shared utility.
	return openaicompat.ParseOpenAIResponse(response)
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
	defer perf.Track(nil, "grok.Client.SendMessageWithSystemPromptAndTools")()

	// Build messages with system prompts prepended using shared utility.
	systemMessages := base.PrependSystemMessages(systemPrompt, atmosMemory, messages)

	// Call existing method with system messages prepended.
	// Grok automatically caches content with 75% discount and >90% hit rate.
	return c.SendMessageWithToolsAndHistory(ctx, systemMessages, availableTools)
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
