package gemini

import (
	"context"

	"google.golang.org/genai"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// ProviderName is the name of this provider for configuration lookup.
	ProviderName = "gemini"
	// DefaultMaxTokens is the default maximum number of tokens in AI responses.
	DefaultMaxTokens = 8192
	// DefaultModel is the default Gemini model.
	DefaultModel = "gemini-2.0-flash-exp"
	// DefaultAPIKeyEnv is the default environment variable for the API key.
	DefaultAPIKeyEnv = "GEMINI_API_KEY"
)

// Client provides a simplified interface to the Google Gemini API for Atmos.
type Client struct {
	client *genai.Client
	config *base.Config
}

// NewClient creates a new Gemini client from Atmos configuration.
func NewClient(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "gemini.NewClient")()

	// Extract AI configuration using shared utility.
	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model:     DefaultModel,
		APIKeyEnv: DefaultAPIKeyEnv,
		MaxTokens: DefaultMaxTokens,
	})

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	// Get API key from environment using shared utility (replaces viper.BindEnv).
	apiKey := base.GetAPIKey(config.APIKeyEnv)
	if apiKey == "" {
		return nil, errUtils.Build(errUtils.ErrAIAPIKeyNotFound).
			WithContext("env_var", config.APIKeyEnv).
			WithHint("Set the " + config.APIKeyEnv + " environment variable").
			Err()
	}

	// Create Gemini client.
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAIClientCreation).
			WithCause(err).
			WithContext("provider", ProviderName).
			Err()
	}

	return &Client{
		client: client,
		config: config,
	}, nil
}

// SendMessage sends a message to the AI and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "gemini.Client.SendMessage")()

	// Use the convenience function to create content from text.
	content := genai.NewContentFromText(message, genai.RoleUser)

	response, err := c.client.Models.GenerateContent(ctx, c.config.Model, []*genai.Content{content}, nil)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			Err()
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
	defer perf.Track(nil, "gemini.Client.SendMessageWithTools")()

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
		return nil, errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("tools_count", len(availableTools)).
			Err()
	}

	// Parse response.
	return parseGeminiResponse(response)
}

// SendMessageWithHistory sends messages with full conversation history.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "gemini.Client.SendMessageWithHistory")()

	// Convert messages to Gemini format.
	geminiContents := convertMessagesToGeminiFormat(messages)

	// Send messages.
	response, err := c.client.Models.GenerateContent(ctx, c.config.Model, geminiContents, nil)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			Err()
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
	defer perf.Track(nil, "gemini.Client.SendMessageWithToolsAndHistory")()

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
		return nil, errUtils.Build(errUtils.ErrAISendMessage).
			WithCause(err).
			WithContext("provider", ProviderName).
			WithContext("model", c.config.Model).
			WithContext("messages_count", len(messages)).
			WithContext("tools_count", len(availableTools)).
			Err()
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
	defer perf.Track(nil, "gemini.Client.SendMessageWithSystemPromptAndTools")()

	// Build messages with system prompts prepended using shared utility.
	systemMessages := base.PrependSystemMessages(systemPrompt, atmosMemory, messages)

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
