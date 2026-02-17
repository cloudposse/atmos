package azureopenai

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
	ProviderName = "azureopenai"
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
	client     *openai.Client
	config     *base.Config
	apiVersion string
}

// NewClient creates a new Azure OpenAI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "azureopenai.NewClient")()

	// Extract AI configuration using shared utility.
	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   "", // Required for Azure, no default.
	})

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Validate required fields.
	if config.BaseURL == "" {
		return nil, errUtils.Build(errUtils.ErrAIBaseURLRequired).
			WithContext("provider", ProviderName).
			WithHint("Set base_url in Azure OpenAI config (format: https://<resource>.openai.azure.com)").
			Err()
	}

	// Get API key from environment using shared utility (replaces viper.BindEnv).
	apiKey := base.GetAPIKey(config.APIKeyEnv)
	if apiKey == "" {
		return nil, errUtils.Build(errUtils.ErrAIAPIKeyNotFound).
			WithContext("env_var", config.APIKeyEnv).
			WithHint("Set the " + config.APIKeyEnv + " environment variable").
			Err()
	}

	// Get API version (Azure-specific, use default).
	apiVersion := DefaultAPIVersion

	// Create OpenAI client configured for Azure.
	// Azure OpenAI uses api-key header instead of Authorization Bearer.
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(config.BaseURL),
		option.WithHeader("api-version", apiVersion),
	)

	return &Client{
		client:     &client,
		config:     config,
		apiVersion: apiVersion,
	}, nil
}

// SendMessage sends a message to Azure OpenAI and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "azureopenai.Client.SendMessage")()

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
	defer perf.Track(nil, "azureopenai.Client.SendMessageWithTools")()

	// Convert our tools to OpenAI's format using shared utility.
	azureTools := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	// Send message with tools.
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model: c.config.Model,
		Tools: azureTools,
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
	defer perf.Track(nil, "azureopenai.Client.SendMessageWithHistory")()

	// Convert messages to OpenAI format using shared utility.
	azureMessages := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	params := openai.ChatCompletionNewParams{
		Messages: azureMessages,
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
	defer perf.Track(nil, "azureopenai.Client.SendMessageWithToolsAndHistory")()

	// Convert messages to OpenAI format using shared utility.
	azureMessages := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	// Convert tools to OpenAI format using shared utility.
	azureTools := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	params := openai.ChatCompletionNewParams{
		Messages: azureMessages,
		Model:    c.config.Model,
		Tools:    azureTools,
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
// For Azure OpenAI, caching happens automatically with 50-100% discount (5-10 min TTL).
// The system prompt and atmosMemory are prepended as system messages.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	availableTools []tools.Tool,
) (*types.Response, error) {
	defer perf.Track(nil, "azureopenai.Client.SendMessageWithSystemPromptAndTools")()

	// Build messages with system prompts prepended using shared utility.
	systemMessages := base.PrependSystemMessages(systemPrompt, atmosMemory, messages)

	// Call existing method with system messages prepended.
	// Azure OpenAI automatically caches content with 50-100% discount (5-10 min TTL).
	return c.SendMessageWithToolsAndHistory(ctx, systemMessages, availableTools)
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
	return c.apiVersion
}
