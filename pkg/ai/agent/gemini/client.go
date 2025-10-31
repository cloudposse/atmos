package gemini

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"google.golang.org/genai"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxTokens is the default maximum number of tokens in AI responses.
	DefaultMaxTokens = 8192
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
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get API key from environment using viper.
	_ = viper.BindEnv(config.APIKeyEnv, config.APIKeyEnv)
	apiKey := viper.GetString(config.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIAPIKeyNotFound, config.APIKeyEnv)
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
		MaxTokens: DefaultMaxTokens,
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		config.Enabled = atmosConfig.Settings.AI.Enabled
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers["gemini"]; exists && providerConfig != nil {
			// Override defaults with provider-specific configuration.
			if providerConfig.Model != "" {
				config.Model = providerConfig.Model
			}
			if providerConfig.ApiKeyEnv != "" {
				config.APIKeyEnv = providerConfig.ApiKeyEnv
			}
			if providerConfig.MaxTokens > 0 {
				config.MaxTokens = providerConfig.MaxTokens
			}
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
		return "", errUtils.ErrAINoResponseCandidates
	}

	if response.Candidates[0].Content == nil || len(response.Candidates[0].Content.Parts) == 0 {
		return "", errUtils.ErrAINoResponseContent
	}

	// Get the first text part.
	part := response.Candidates[0].Content.Parts[0]
	if part.Text == "" {
		return "", errUtils.ErrAIResponseNotText
	}

	return part.Text, nil
}

// SendMessageWithTools sends a message with available tools.
func (c *Client) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	// Convert our tools to Gemini's format.
	geminiTools := convertToolsToGeminiFormat(availableTools)

	// Create content from user message.
	content := genai.NewContentFromText(message, genai.RoleUser)

	// Create GenerateContentConfig with tools.
	config := &genai.GenerateContentConfig{
		Tools: geminiTools,
	}

	// Send message with tools.
	response, err := c.client.Models.GenerateContent(ctx, c.config.Model, []*genai.Content{content}, config)
	if err != nil {
		return nil, fmt.Errorf("failed to send message with tools: %w", err)
	}

	// Parse response.
	return parseGeminiResponse(response)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	// Convert messages to Gemini format.
	geminiContents := convertMessagesToGeminiFormat(messages)

	// Send messages.
	response, err := c.client.Models.GenerateContent(ctx, c.config.Model, geminiContents, nil)
	if err != nil {
		return "", fmt.Errorf("failed to send messages with history: %w", err)
	}

	// Extract text from response.
	if len(response.Candidates) == 0 {
		return "", errUtils.ErrAINoResponseCandidates
	}

	if response.Candidates[0].Content == nil || len(response.Candidates[0].Content.Parts) == 0 {
		return "", errUtils.ErrAINoResponseContent
	}

	// Get the first text part.
	part := response.Candidates[0].Content.Parts[0]
	if part.Text == "" {
		return "", errUtils.ErrAIResponseNotText
	}

	return part.Text, nil
}

// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
func (c *Client) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	// Convert messages to Gemini format.
	geminiContents := convertMessagesToGeminiFormat(messages)

	// Convert tools to Gemini format.
	geminiTools := convertToolsToGeminiFormat(availableTools)

	// Create GenerateContentConfig with tools.
	config := &genai.GenerateContentConfig{
		Tools: geminiTools,
	}

	// Send messages with tools.
	response, err := c.client.Models.GenerateContent(ctx, c.config.Model, geminiContents, config)
	if err != nil {
		return nil, fmt.Errorf("failed to send messages with history and tools: %w", err)
	}

	// Parse response.
	return parseGeminiResponse(response)
}

// SendMessageWithSystemPromptAndTools sends messages with system prompt, conversation history, and available tools.
// For Gemini, caching happens automatically and is free.
// The system prompt and atmosMemory are prepended as system messages (converted to user role).
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	availableTools []tools.Tool,
) (*types.Response, error) {
	// Build messages with system prompts prepended.
	systemMessages := make([]types.Message, 0, 2+len(messages))

	// Add system prompt if provided.
	if systemPrompt != "" {
		systemMessages = append(systemMessages, types.Message{
			Role:    types.RoleSystem,
			Content: systemPrompt,
		})
	}

	// Add ATMOS.md content if provided.
	if atmosMemory != "" {
		systemMessages = append(systemMessages, types.Message{
			Role:    types.RoleSystem,
			Content: atmosMemory,
		})
	}

	// Add conversation history.
	systemMessages = append(systemMessages, messages...)

	// Call existing method with system messages prepended.
	// Gemini automatically caches content (free, any length).
	return c.SendMessageWithToolsAndHistory(ctx, systemMessages, availableTools)
}

// convertMessagesToGeminiFormat converts our Message slice to Gemini's Content format.
func convertMessagesToGeminiFormat(messages []types.Message) []*genai.Content {
	geminiContents := make([]*genai.Content, 0, len(messages))

	for _, msg := range messages {
		// Map role to Gemini role.
		var role string
		switch msg.Role {
		case types.RoleUser:
			role = genai.RoleUser
		case types.RoleAssistant:
			role = genai.RoleModel
		case types.RoleSystem:
			// Gemini doesn't support system messages in the same way.
			// We can prepend system messages as user messages or skip them.
			// For now, treat system messages as user messages.
			role = genai.RoleUser
		default:
			role = genai.RoleUser
		}

		// Create content with text part.
		content := &genai.Content{
			Role: role,
			Parts: []*genai.Part{
				{
					Text: msg.Content,
				},
			},
		}

		geminiContents = append(geminiContents, content)
	}

	return geminiContents
}

// convertToolsToGeminiFormat converts our Tool interface to Gemini's function format.
func convertToolsToGeminiFormat(availableTools []tools.Tool) []*genai.Tool {
	if len(availableTools) == 0 {
		return nil
	}

	// Create function declarations.
	functionDeclarations := make([]*genai.FunctionDeclaration, 0, len(availableTools))

	for _, tool := range availableTools {
		// Build properties and required fields from parameters.
		properties := make(map[string]*genai.Schema)
		required := make([]string, 0)

		for _, param := range tool.Parameters() {
			// Map our parameter type to Gemini Type.
			var geminiType genai.Type
			switch param.Type {
			case "string":
				geminiType = genai.TypeString
			case "number":
				geminiType = genai.TypeNumber
			case "integer":
				geminiType = genai.TypeInteger
			case "boolean":
				geminiType = genai.TypeBoolean
			case "array":
				geminiType = genai.TypeArray
			case "object":
				geminiType = genai.TypeObject
			default:
				geminiType = genai.TypeString // Default to string.
			}

			properties[param.Name] = &genai.Schema{
				Type:        geminiType,
				Description: param.Description,
			}

			if param.Required {
				required = append(required, param.Name)
			}
		}

		// Create function declaration.
		functionDecl := &genai.FunctionDeclaration{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: properties,
				Required:   required,
			},
		}

		functionDeclarations = append(functionDeclarations, functionDecl)
	}

	// Gemini expects tools as an array with a single Tool containing all function declarations.
	return []*genai.Tool{
		{
			FunctionDeclarations: functionDeclarations,
		},
	}
}

// parseGeminiResponse parses a Gemini response into our Response format.
func parseGeminiResponse(response *genai.GenerateContentResponse) (*types.Response, error) {
	result := &types.Response{
		Content:   "",
		ToolCalls: make([]types.ToolCall, 0),
	}

	// Check if we have candidates.
	if len(response.Candidates) == 0 {
		return nil, errUtils.ErrAINoResponseCandidates
	}

	candidate := response.Candidates[0]

	// Map finish reason to stop reason.
	switch candidate.FinishReason {
	case genai.FinishReasonStop:
		result.StopReason = types.StopReasonEndTurn
	case genai.FinishReasonMaxTokens:
		result.StopReason = types.StopReasonMaxTokens
	case genai.FinishReasonSafety, genai.FinishReasonRecitation, genai.FinishReasonOther:
		result.StopReason = types.StopReasonEndTurn
	default:
		result.StopReason = types.StopReasonEndTurn
	}

	// Extract text content if available.
	if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
		// Concatenate all text parts.
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				result.Content += part.Text
			}
		}
	}

	// Extract tool calls using the convenience method.
	functionCalls := response.FunctionCalls()
	if len(functionCalls) > 0 {
		result.StopReason = types.StopReasonToolUse
		for _, funcCall := range functionCalls {
			result.ToolCalls = append(result.ToolCalls, types.ToolCall{
				ID:    funcCall.ID,
				Name:  funcCall.Name,
				Input: funcCall.Args,
			})
		}
	}

	// Extract usage information.
	if response.UsageMetadata != nil {
		result.Usage = &types.Usage{
			InputTokens:         int64(response.UsageMetadata.PromptTokenCount),
			OutputTokens:        int64(response.UsageMetadata.CandidatesTokenCount),
			TotalTokens:         int64(response.UsageMetadata.TotalTokenCount),
			CacheReadTokens:     int64(response.UsageMetadata.CachedContentTokenCount),
			CacheCreationTokens: 0, // Gemini doesn't separately report cache creation tokens.
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
