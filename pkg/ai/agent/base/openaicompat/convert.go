// Package openaicompat provides shared utilities for OpenAI-compatible API providers.
// This includes OpenAI, Grok, Ollama, and Azure OpenAI which all use the OpenAI SDK.
package openaicompat

import (
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
)

// ConvertMessagesToOpenAIFormat converts Message slice to OpenAI's message format.
// This is shared by OpenAI, Grok, Ollama, and Azure OpenAI providers.
func ConvertMessagesToOpenAIFormat(messages []types.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			result = append(result, openai.UserMessage(msg.Content))
		case types.RoleAssistant:
			result = append(result, openai.AssistantMessage(msg.Content))
		case types.RoleSystem:
			result = append(result, openai.SystemMessage(msg.Content))
		}
	}

	return result
}

// ConvertToolsToOpenAIFormat converts Tool interface slice to OpenAI's tool format.
// This is shared by OpenAI, Grok, Ollama, and Azure OpenAI providers.
func ConvertToolsToOpenAIFormat(availableTools []tools.Tool) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(availableTools))

	for _, tool := range availableTools {
		info := base.ExtractToolInfo(tool)

		params := openai.FunctionParameters{
			"type":       "object",
			"properties": info.Properties,
			"required":   info.Required,
		}

		toolParam := openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        info.Name,
				Description: openai.String(info.Description),
				Parameters:  params,
			},
		}

		result = append(result, toolParam)
	}

	return result
}

// ParseOpenAIResponse parses an OpenAI ChatCompletion response into our Response format.
// This is shared by OpenAI, Grok, Ollama, and Azure OpenAI providers.
func ParseOpenAIResponse(response *openai.ChatCompletion) (*types.Response, error) {
	result := &types.Response{
		Content:   "",
		ToolCalls: make([]types.ToolCall, 0),
	}

	// Check if we have choices.
	if len(response.Choices) == 0 {
		return nil, errUtils.ErrAINoResponseChoices
	}

	choice := response.Choices[0]

	// Map finish reason to stop reason.
	result.StopReason = mapOpenAIFinishReason(choice.FinishReason)

	// Extract text content.
	result.Content = choice.Message.Content

	// Extract tool calls if present.
	if err := extractOpenAIToolCalls(&choice.Message, result); err != nil {
		return nil, err
	}

	// Extract usage information.
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		result.Usage = &types.Usage{
			InputTokens:         response.Usage.PromptTokens,
			OutputTokens:        response.Usage.CompletionTokens,
			TotalTokens:         response.Usage.TotalTokens,
			CacheReadTokens:     response.Usage.PromptTokensDetails.CachedTokens,
			CacheCreationTokens: 0, // The Chat Completions API does not report cache writes.
		}
	}

	return result, nil
}

// mapOpenAIFinishReason maps an OpenAI finish reason string to our StopReason type.
func mapOpenAIFinishReason(reason string) types.StopReason {
	switch reason {
	case "stop":
		return types.StopReasonEndTurn
	case "tool_calls":
		return types.StopReasonToolUse
	case "length":
		return types.StopReasonMaxTokens
	default:
		return types.StopReasonEndTurn
	}
}

// extractOpenAIToolCalls extracts tool calls from an OpenAI message and adds them to the result.
func extractOpenAIToolCalls(message *openai.ChatCompletionMessage, result *types.Response) error {
	for i := range message.ToolCalls {
		toolCall := &message.ToolCalls[i]
		// Parse function arguments.
		var args map[string]interface{}
		if toolCall.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				return fmt.Errorf("failed to parse tool arguments: %w", err)
			}
		}

		result.ToolCalls = append(result.ToolCalls, types.ToolCall{
			ID:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: args,
		})
	}

	return nil
}

// gpt5PrefixLength is the length of the "gpt-5" model prefix used for model detection.
const gpt5PrefixLength = 5

// RequiresMaxCompletionTokens returns true if the model requires max_completion_tokens parameter.
// Some newer OpenAI models use max_completion_tokens instead of max_tokens.
func RequiresMaxCompletionTokens(model string) bool {
	// These include: gpt-5*, gpt-6*, the o-series (o1, o1-mini, o3, o4-mini, ...), chatgpt-4o-latest.
	if len(model) >= gpt5PrefixLength && model[:gpt5PrefixLength] == "gpt-6" {
		return true
	}

	return isOSeriesModel(model) || requiresMaxCompletionTokensByName(model)
}

// requiresMaxCompletionTokensByName covers the model names that were matched before the o-series and gpt-6 were added.
func requiresMaxCompletionTokensByName(model string) bool {
	if len(model) >= gpt5PrefixLength && model[:gpt5PrefixLength] == "gpt-5" {
		return true
	}

	return model == "o1-preview" || model == "o1-mini" || model == "chatgpt-4o-latest"
}

// isOSeriesModel returns true for OpenAI reasoning models named "o" followed by a digit (o1, o3-mini, o4-mini).
func isOSeriesModel(model string) bool {
	return len(model) >= 2 && model[0] == 'o' && model[1] >= '0' && model[1] <= '9'
}

// SetTokenLimit sets the appropriate token limit parameter based on the model.
// Newer models (gpt-5, gpt-6, the o-series, chatgpt-4o-latest) use max_completion_tokens,
// while older models use max_tokens.
func SetTokenLimit(params *openai.ChatCompletionNewParams, model string, maxTokens int) {
	if RequiresMaxCompletionTokens(model) {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	} else {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
}

// SetDeploymentTokenLimit is SetTokenLimit for providers where the model is a user-chosen deployment name (Azure OpenAI).
// A deployment called "o3-backend" can serve any model, and the default Azure API version does not accept
// max_completion_tokens for the o-series and gpt-6, so only the names matched before those were added switch parameter.
func SetDeploymentTokenLimit(params *openai.ChatCompletionNewParams, deployment string, maxTokens int) {
	if requiresMaxCompletionTokensByName(deployment) {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	} else {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
}
