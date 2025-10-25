package ai

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
)

// Client defines the interface for AI clients.
type Client interface {
	// SendMessage sends a message to the AI and returns the response.
	SendMessage(ctx context.Context, message string) (string, error)

	// SendMessageWithTools sends a message with available tools and handles tool calls.
	SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error)

	// GetModel returns the configured model name.
	GetModel() string

	// GetMaxTokens returns the configured max tokens.
	GetMaxTokens() int
}
