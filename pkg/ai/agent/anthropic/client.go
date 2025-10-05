package anthropic

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/cloudposse/atmos/pkg/schema"
)

// SimpleClient provides a simplified interface to the Anthropic API for Atmos.
type SimpleClient struct {
	client *anthropic.Client
	config *SimpleAIConfig
}

// SimpleAIConfig holds basic configuration for the AI client.
type SimpleAIConfig struct {
	Enabled   bool
	Model     string
	APIKeyEnv string
	MaxTokens int
}

// NewSimpleClient creates a new simple AI client from Atmos configuration.
func NewSimpleClient(atmosConfig *schema.AtmosConfiguration) (*SimpleClient, error) {
	// Extract simple AI configuration
	config := extractSimpleAIConfig(atmosConfig)

	if !config.Enabled {
		return nil, fmt.Errorf("AI features are disabled in configuration")
	}

	// Get API key from environment
	apiKey := os.Getenv(config.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("API key not found in environment variable: %s", config.APIKeyEnv)
	}

	// Create Anthropic client
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
	// Set defaults
	config := &SimpleAIConfig{
		Enabled:   false,
		Model:     "claude-3-5-sonnet-20241022",
		APIKeyEnv: "ANTHROPIC_API_KEY",
		MaxTokens: 4096,
	}

	// Override defaults with configuration from atmos.yaml.
	if atmosConfig.Settings.AI.Enabled {
		config.Enabled = atmosConfig.Settings.AI.Enabled
	}
	if atmosConfig.Settings.AI.Model != "" {
		config.Model = atmosConfig.Settings.AI.Model
	}
	if atmosConfig.Settings.AI.ApiKeyEnv != "" {
		config.APIKeyEnv = atmosConfig.Settings.AI.ApiKeyEnv
	}
	if atmosConfig.Settings.AI.MaxTokens > 0 {
		config.MaxTokens = atmosConfig.Settings.AI.MaxTokens
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

	// Extract text from response
	var responseText string
	for _, content := range response.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return responseText, nil
}

// GetModel returns the configured model name.
func (c *SimpleClient) GetModel() string {
	return c.config.Model
}

// GetMaxTokens returns the configured max tokens.
func (c *SimpleClient) GetMaxTokens() int {
	return c.config.MaxTokens
}
