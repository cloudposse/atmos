package base

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/types"
)

// PrependSystemMessages prepends system prompt and atmos memory to conversation history.
// This is the common pattern used by all providers in SendMessageWithSystemPromptAndTools.
func PrependSystemMessages(systemPrompt, atmosMemory string, messages []types.Message) []types.Message {
	// Calculate capacity: up to 2 system messages + conversation history.
	capacity := len(messages)
	if systemPrompt != "" {
		capacity++
	}
	if atmosMemory != "" {
		capacity++
	}

	result := make([]types.Message, 0, capacity)

	// Add system prompt if provided.
	if systemPrompt != "" {
		result = append(result, types.Message{
			Role:    types.RoleSystem,
			Content: systemPrompt,
		})
	}

	// Add ATMOS.md content if provided.
	if atmosMemory != "" {
		result = append(result, types.Message{
			Role:    types.RoleSystem,
			Content: atmosMemory,
		})
	}

	// Add conversation history.
	result = append(result, messages...)

	return result
}

// FormatMessagesAsPrompt concatenates conversation messages into a single prompt string.
// User messages are included as-is; assistant messages are prefixed with "Assistant: ".
// Used by CLI providers that pass the entire conversation as a single text prompt.
func FormatMessagesAsPrompt(messages []types.Message) string {
	var parts []string
	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			parts = append(parts, msg.Content)
		case types.RoleAssistant:
			parts = append(parts, "Assistant: "+msg.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}
