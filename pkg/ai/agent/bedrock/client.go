package bedrock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxTokens is the default maximum number of tokens in AI responses.
	DefaultMaxTokens = 4096
	// DefaultModel is the default Bedrock model.
	DefaultModel = "anthropic.claude-sonnet-4-20250514-v2:0"
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

// SendMessageWithTools sends a message with available tools.
func (c *Client) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	// Convert our tools to Bedrock/Anthropic's format.
	bedrockTools := convertToolsToBedrockFormat(availableTools)

	// Prepare request body for Claude models on Bedrock with tools.
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        c.config.MaxTokens,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": message,
			},
		},
		"tools": bedrockTools,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Invoke model.
	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke Bedrock model with tools: %w", err)
	}

	// Parse response.
	return parseBedrockResponse(response.Body)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	// Convert messages to Bedrock/Anthropic format.
	bedrockMessages := convertMessagesToBedrockFormat(messages)

	// Prepare request body for Claude models on Bedrock.
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        c.config.MaxTokens,
		"messages":          bedrockMessages,
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
		return "", fmt.Errorf("failed to invoke Bedrock model with history: %w", err)
	}

	// Unmarshal response.
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

// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
func (c *Client) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	// Convert messages to Bedrock/Anthropic format.
	bedrockMessages := convertMessagesToBedrockFormat(messages)

	// Convert tools to Bedrock/Anthropic format.
	bedrockTools := convertToolsToBedrockFormat(availableTools)

	// Prepare request body for Claude models on Bedrock with tools.
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        c.config.MaxTokens,
		"messages":          bedrockMessages,
		"tools":             bedrockTools,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Invoke model.
	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke Bedrock model with history and tools: %w", err)
	}

	// Parse response.
	return parseBedrockResponse(response.Body)
}

// convertMessagesToBedrockFormat converts our Message slice to Bedrock/Anthropic's format.
func convertMessagesToBedrockFormat(messages []types.Message) []map[string]string {
	bedrockMessages := make([]map[string]string, 0, len(messages))

	for _, msg := range messages {
		// Bedrock/Anthropic uses "user" and "assistant" roles only.
		// System messages are typically sent via a separate "system" parameter, not in messages array.
		switch msg.Role {
		case types.RoleUser:
			bedrockMessages = append(bedrockMessages, map[string]string{
				"role":    "user",
				"content": msg.Content,
			})
		case types.RoleAssistant:
			bedrockMessages = append(bedrockMessages, map[string]string{
				"role":    "assistant",
				"content": msg.Content,
			})
			// Skip system messages as they should be sent via the "system" parameter
		}
	}

	return bedrockMessages
}

// convertToolsToBedrockFormat converts our Tool interface to Bedrock/Anthropic's format.
func convertToolsToBedrockFormat(availableTools []tools.Tool) []map[string]interface{} {
	bedrockTools := make([]map[string]interface{}, 0, len(availableTools))

	for _, tool := range availableTools {
		// Build input schema with properties and required fields.
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

		// Create tool definition in Anthropic/Bedrock format.
		toolDef := map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"input_schema": map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		}

		bedrockTools = append(bedrockTools, toolDef)
	}

	return bedrockTools
}

// parseBedrockResponse parses a Bedrock response into our Response format.
func parseBedrockResponse(responseBody []byte) (*types.Response, error) {
	// Parse Bedrock/Anthropic response format.
	var apiResponse struct {
		Content []struct {
			Type  string                 `json:"type"`
			Text  string                 `json:"text,omitempty"`
			ID    string                 `json:"id,omitempty"`
			Name  string                 `json:"name,omitempty"`
			Input map[string]interface{} `json:"input,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.Unmarshal(responseBody, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Bedrock response: %w", err)
	}

	result := &types.Response{
		Content:   "",
		ToolCalls: make([]types.ToolCall, 0),
	}

	// Map stop reason.
	switch apiResponse.StopReason {
	case "end_turn":
		result.StopReason = types.StopReasonEndTurn
	case "tool_use":
		result.StopReason = types.StopReasonToolUse
	case "max_tokens":
		result.StopReason = types.StopReasonMaxTokens
	default:
		result.StopReason = types.StopReasonEndTurn
	}

	// Extract content blocks.
	for _, content := range apiResponse.Content {
		switch content.Type {
		case "text":
			result.Content += content.Text
		case "tool_use":
			// Extract tool call.
			result.ToolCalls = append(result.ToolCalls, types.ToolCall{
				ID:    content.ID,
				Name:  content.Name,
				Input: content.Input,
			})
		}
	}

	return result, nil
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
