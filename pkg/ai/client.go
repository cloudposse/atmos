package ai

import (
	"context"
)

// Client defines the interface for AI clients.
type Client interface {
	// SendMessage sends a message to the AI and returns the response.
	SendMessage(ctx context.Context, message string) (string, error)

	// GetModel returns the configured model name.
	GetModel() string

	// GetMaxTokens returns the configured max tokens.
	GetMaxTokens() int
}
