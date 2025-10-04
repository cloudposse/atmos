package gemini

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/genai"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Client provides a simplified interface to the Google Gemini API for Atmos.
type Client struct {
	client *genai.Client
	config *Config
}

// Config holds basic configuration for the Gemini client.
type Config struct {
	Enabled   bool
	Model     string
	APIKeyEnv string
	MaxTokens int
}

// NewClient creates a new Gemini client from Atmos configuration.
func NewClient(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (*Client, error) {
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

	// Create Gemini client.
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &Client{
		client: client,
		config: config,
	}, nil
}

// extractConfig extracts AI configuration from AtmosConfiguration.
func extractConfig(atmosConfig *schema.AtmosConfiguration) *Config {
	// Set defaults.
	config := &Config{
		Enabled:   false,
		Model:     "gemini-2.0-flash-exp",
		APIKeyEnv: "GEMINI_API_KEY",
		MaxTokens: 8192,
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
	}

	return config
}

// SendMessage sends a message to the AI and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	// Use the convenience function to create content from text.
	content := genai.NewContentFromText(message, genai.RoleUser)

	response, err := c.client.Models.GenerateContent(ctx, c.config.Model, []*genai.Content{content}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Extract text from response.
	if len(response.Candidates) == 0 {
		return "", fmt.Errorf("no response candidates returned")
	}

	if response.Candidates[0].Content == nil || len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	// Get the first text part.
	part := response.Candidates[0].Content.Parts[0]
	if part.Text == "" {
		return "", fmt.Errorf("response part does not contain text")
	}

	return part.Text, nil
}

// GetModel returns the configured model name.
func (c *Client) GetModel() string {
	return c.config.Model
}

// GetMaxTokens returns the configured max tokens.
func (c *Client) GetMaxTokens() int {
	return c.config.MaxTokens
}
