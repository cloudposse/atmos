package bedrock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxTokens is the default maximum number of tokens in AI responses.
	DefaultMaxTokens = 4096
	// DefaultModel is the default Bedrock model.
	DefaultModel = "anthropic.claude-3-5-sonnet-20241022-v2:0"
	// DefaultRegion is the default AWS region for Bedrock.
	DefaultRegion = "us-east-1"
)

// Client provides an interface to AWS Bedrock for Atmos.
type Client struct {
	client *bedrockruntime.Client
	config *Config
}

// Config holds configuration for the AWS Bedrock client.
type Config struct {
	Enabled   bool
	Model     string
	Region    string
	MaxTokens int
}

// NewClient creates a new AWS Bedrock client from Atmos configuration.
func NewClient(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	// Extract Bedrock configuration.
	cfg := extractConfig(atmosConfig)

	if !cfg.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Load AWS configuration.
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	// Create Bedrock Runtime client.
	client := bedrockruntime.NewFromConfig(awsCfg)

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// extractConfig extracts Bedrock configuration from AtmosConfiguration.
func extractConfig(atmosConfig *schema.AtmosConfiguration) *Config {
	// Set defaults.
	cfg := &Config{
		Enabled:   false,
		Model:     DefaultModel,
		Region:    DefaultRegion,
		MaxTokens: DefaultMaxTokens,
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		cfg.Enabled = atmosConfig.Settings.AI.Enabled
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers["bedrock"]; exists && providerConfig != nil {
			// Override defaults with provider-specific configuration.
			if providerConfig.Model != "" {
				cfg.Model = providerConfig.Model
			}
			if providerConfig.MaxTokens > 0 {
				cfg.MaxTokens = providerConfig.MaxTokens
			}
			if providerConfig.BaseURL != "" {
				// BaseURL is used to specify region for Bedrock.
				cfg.Region = providerConfig.BaseURL
			}
		}
	}

	return cfg
}

// SendMessage sends a message to AWS Bedrock and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	// Prepare request body for Claude models on Bedrock.
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        c.config.MaxTokens,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": message,
			},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Invoke model.
	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to invoke Bedrock model: %w", err)
	}

	// Parse response.
	var responseBody struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(response.Body, &responseBody); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract text from response.
	var responseText string
	for i := range responseBody.Content {
		if responseBody.Content[i].Type == "text" {
			responseText += responseBody.Content[i].Text
		}
	}

	return responseText, nil
}

// GetModel returns the configured model name.
func (c *Client) GetModel() string {
	return c.config.Model
}

// GetMaxTokens returns the configured max tokens.
func (c *Client) GetMaxTokens() int {
	return c.config.MaxTokens
}

// GetRegion returns the configured AWS region.
func (c *Client) GetRegion() string {
	return c.config.Region
}
