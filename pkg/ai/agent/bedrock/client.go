package bedrock

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// ProviderName is the name of this provider for configuration lookup.
	ProviderName = "bedrock"
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
	config *base.Config
	region string
}

// NewClient creates a new AWS Bedrock client from Atmos configuration.
func NewClient(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "bedrock.NewClient")()

	// Extract AI configuration using shared utility.
	// Note: Bedrock uses BaseURL to specify the AWS region.
	cfg := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		BaseURL:   DefaultRegion, // Region is stored in BaseURL.
	})

	if !cfg.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get region from BaseURL (Bedrock-specific mapping).
	region := cfg.BaseURL
	if region == "" {
		region = DefaultRegion
	}

	// Load AWS configuration.
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAILoadAWSConfig).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("region", region).
			Err()
	}

	// Create Bedrock Runtime client.
	client := bedrockruntime.NewFromConfig(awsCfg)

	return &Client{
		client: client,
		config: cfg,
		region: region,
	}, nil
}

// SendMessage sends a message to AWS Bedrock and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "bedrock.Client.SendMessage")()

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
		return "", errUtils.Build(errUtils.ErrAIMarshalRequest).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
	}

	// Invoke model.
	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			Err()
	}

	// Parse response.
	var responseBody struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(response.Body, &responseBody); err != nil {
		return "", errUtils.Build(errUtils.ErrAIUnmarshalResponse).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
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
	defer perf.Track(nil, "bedrock.Client.SendMessageWithTools")()

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
		return nil, errUtils.Build(errUtils.ErrAIMarshalRequest).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
	}

	// Invoke model.
	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
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
	return parseBedrockResponse(response.Body)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "bedrock.Client.SendMessageWithHistory")()

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
		return "", errUtils.Build(errUtils.ErrAIMarshalRequest).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
	}

	// Invoke model.
	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			Err()
	}

	// Unmarshal response.
	var responseBody struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(response.Body, &responseBody); err != nil {
		return "", errUtils.Build(errUtils.ErrAIUnmarshalResponse).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
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
	defer perf.Track(nil, "bedrock.Client.SendMessageWithToolsAndHistory")()

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
		return nil, errUtils.Build(errUtils.ErrAIMarshalRequest).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
	}

	// Invoke model.
	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
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
	return parseBedrockResponse(response.Body)
}

// SendMessageWithSystemPromptAndTools sends messages with system prompt, conversation history, and available tools.
// For Bedrock, caching happens automatically with up to 90% discount.
// The system prompt and atmosMemory are prepended as system messages.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	availableTools []tools.Tool,
) (*types.Response, error) {
	defer perf.Track(nil, "bedrock.Client.SendMessageWithSystemPromptAndTools")()

	// Build messages with system prompts prepended using shared utility.
	systemMessages := base.PrependSystemMessages(systemPrompt, atmosMemory, messages)

	// Call existing method with system messages prepended.
	// Bedrock automatically caches content with up to 90% discount (simplified auto mode).
	return c.SendMessageWithToolsAndHistory(ctx, systemMessages, availableTools)
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
			// Skip system messages as they should be sent via the "system" parameter.
		}
	}

	return bedrockMessages
}

// convertToolsToBedrockFormat converts our Tool interface to Bedrock/Anthropic's format.
func convertToolsToBedrockFormat(availableTools []tools.Tool) []map[string]interface{} {
	bedrockTools := make([]map[string]interface{}, 0, len(availableTools))

	for _, tool := range availableTools {
		// Build input schema using shared utility.
		info := base.ExtractToolInfo(tool)

		// Create tool definition in Anthropic/Bedrock format.
		toolDef := map[string]interface{}{
			"name":        info.Name,
			"description": info.Description,
			"input_schema": map[string]interface{}{
				"type":       "object",
				"properties": info.Properties,
				"required":   info.Required,
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
		Usage      struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(responseBody, &apiResponse); err != nil {
		return nil, errUtils.Build(errUtils.ErrAIUnmarshalResponse).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
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

	// Extract usage information.
	if apiResponse.Usage.InputTokens > 0 || apiResponse.Usage.OutputTokens > 0 {
		result.Usage = &types.Usage{
			InputTokens:         apiResponse.Usage.InputTokens,
			OutputTokens:        apiResponse.Usage.OutputTokens,
			TotalTokens:         apiResponse.Usage.InputTokens + apiResponse.Usage.OutputTokens,
			CacheReadTokens:     0, // Bedrock/Anthropic doesn't provide cache tokens in this format.
			CacheCreationTokens: 0,
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
	return c.region
}
