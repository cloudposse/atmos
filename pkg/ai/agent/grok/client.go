package grok

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/cloudposse/atmos/pkg/schema"
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
		return nil, fmt.Errorf("AI features are disabled in configuration")
	}

	// Get API key from environment.
	apiKey := os.Getenv(config.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("API key not found in environment variable: %s", config.APIKeyEnv)
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
		Model:     "grok-beta",
		APIKeyEnv: "XAI_API_KEY",
		MaxTokens: 4096,
		BaseURL:   "https://api.x.ai/v1",
	}

	// Check if AI settings exist in the configuration.
	if atmosConfig.Settings.AI != nil {
		if enabled, ok := atmosConfig.Settings.AI["enabled"].(bool); ok {
			config.Enabled = enabled
		}
		if model, ok := atmosConfig.Settings.AI["model"].(string); ok {
			config.Model = model
		}
		if apiKeyEnv, ok := atmosConfig.Settings.AI["api_key_env"].(string); ok {
			config.APIKeyEnv = apiKeyEnv
		}
		if maxTokens, ok := atmosConfig.Settings.AI["max_tokens"].(int); ok {
			config.MaxTokens = maxTokens
		}
		if baseURL, ok := atmosConfig.Settings.AI["base_url"].(string); ok {
			config.BaseURL = baseURL
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
		Model:     openai.ChatModel(c.config.Model),
		MaxTokens: openai.Int(int64(c.config.MaxTokens)),
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Extract text from response.
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return response.Choices[0].Message.Content, nil
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
