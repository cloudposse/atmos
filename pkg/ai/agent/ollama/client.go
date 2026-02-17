package ollama

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
	ProviderName = "ollama"
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
	config *base.Config
}

// NewClient creates a new Ollama client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "ollama.NewClient")()

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
	// For Ollama, API key is optional for local usage.
	apiKey := base.GetAPIKey(config.APIKeyEnv)
	if apiKey == "" {
		apiKey = "ollama" // Dummy key for local Ollama instances.
	}

	// Create Ollama client using OpenAI-compatible API.
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
	defer perf.Track(nil, "ollama.Client.SendMessage")()

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
	}

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
	defer perf.Track(nil, "ollama.Client.SendMessageWithTools")()

	// Convert our tools to OpenAI's format using shared utility.
	ollamaTools := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

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
	defer perf.Track(nil, "ollama.Client.SendMessageWithHistory")()

	// Convert messages to OpenAI format using shared utility.
	ollamaMessages := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	params := openai.ChatCompletionNewParams{
		Messages:  ollamaMessages,
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
	}

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
	defer perf.Track(nil, "ollama.Client.SendMessageWithToolsAndHistory")()

	// Convert messages to OpenAI format using shared utility.
	ollamaMessages := openaicompat.ConvertMessagesToOpenAIFormat(messages)

	// Convert tools to OpenAI format using shared utility.
	ollamaTools := openaicompat.ConvertToolsToOpenAIFormat(availableTools)

	params := openai.ChatCompletionNewParams{
		Messages:  ollamaMessages,
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
		Tools:     ollamaTools,
	}

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
// For Ollama (local), there are no API costs and no caching needed.
// The system prompt and atmosMemory are prepended as system messages.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	availableTools []tools.Tool,
) (*types.Response, error) {
	defer perf.Track(nil, "ollama.Client.SendMessageWithSystemPromptAndTools")()

	// Build messages with system prompts prepended using shared utility.
	systemMessages := base.PrependSystemMessages(systemPrompt, atmosMemory, messages)

	// Call existing method with system messages prepended.
	// Ollama runs locally, so no API costs or caching concerns.
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
