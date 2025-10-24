package azureopenai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
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
		Model:     c.config.Model,
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
	}

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
