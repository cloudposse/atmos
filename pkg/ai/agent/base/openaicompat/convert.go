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
	switch choice.FinishReason {
	case "stop":
		result.StopReason = types.StopReasonEndTurn
	case "tool_calls":
		result.StopReason = types.StopReasonToolUse
	case "length":
		result.StopReason = types.StopReasonMaxTokens
	default:
		result.StopReason = types.StopReasonEndTurn
	}

	// Extract text content.
	result.Content = choice.Message.Content

	// Extract tool calls if present.
	if len(choice.Message.ToolCalls) > 0 {
		for _, toolCall := range choice.Message.ToolCalls {
			// Parse function arguments.
			var args map[string]interface{}
			if toolCall.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
				}
			}

			result.ToolCalls = append(result.ToolCalls, types.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: args,
			})
		}
	}

	// Extract usage information.
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		result.Usage = &types.Usage{
			InputTokens:         response.Usage.PromptTokens,
			OutputTokens:        response.Usage.CompletionTokens,
			TotalTokens:         response.Usage.TotalTokens,
			CacheReadTokens:     0,
			CacheCreationTokens: 0,
		}
	}

	return result, nil
}

// RequiresMaxCompletionTokens returns true if the model requires max_completion_tokens parameter.
// Some newer OpenAI models use max_completion_tokens instead of max_tokens.
func RequiresMaxCompletionTokens(model string) bool {
	// Check for models that use max_completion_tokens.
	// These include: gpt-5*, o1-preview, o1-mini, chatgpt-4o-latest.
	if len(model) >= 5 && model[:5] == "gpt-5" {
		return true
	}
	if model == "o1-preview" || model == "o1-mini" || model == "chatgpt-4o-latest" {
		return true
	}

	return false
}

// SetTokenLimit sets the appropriate token limit parameter based on the model.
// Newer models (gpt-5, o1-preview, o1-mini, chatgpt-4o-latest) use max_completion_tokens,
// while older models use max_tokens.
func SetTokenLimit(params *openai.ChatCompletionNewParams, model string, maxTokens int) {
	if RequiresMaxCompletionTokens(model) {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	} else {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
}
