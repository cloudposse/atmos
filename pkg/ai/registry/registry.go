// Package registry provides the AI provider registry for client creation.
package registry

import (
	"context"
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Client defines the interface for AI clients.
// This is defined here to avoid import cycles between pkg/ai and pkg/ai/agent/*.
type Client interface {
	// SendMessage sends a message to the AI and returns the response.
	SendMessage(ctx context.Context, message string) (string, error)

	// SendMessageWithTools sends a message with available tools and handles tool calls.
	SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error)

	// SendMessageWithHistory sends messages with full conversation history.
	SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error)

	// SendMessageWithToolsAndHistory sends messages with full conversation history and available tools.
	SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error)

	// SendMessageWithSystemPromptAndTools sends messages with system prompt, conversation history, and available tools.
	SendMessageWithSystemPromptAndTools(
		ctx context.Context,
		systemPrompt string,
		atmosMemory string,
		messages []types.Message,
		availableTools []tools.Tool,
	) (*types.Response, error)

	// GetModel returns the configured model name.
	GetModel() string

	// GetMaxTokens returns the configured max tokens.
	GetMaxTokens() int
}

// ClientFactory is a function that creates a new AI client.
type ClientFactory func(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (Client, error)

// providerRegistry holds registered AI providers.
type providerRegistry struct {
	mu        sync.RWMutex
	providers map[string]ClientFactory
}

// globalRegistry is the singleton registry for AI providers.
var globalRegistry = &providerRegistry{
	providers: make(map[string]ClientFactory),
}

// Register registers an AI provider with the given name.
// This should be called from provider package init() functions.
func Register(name string, factory ClientFactory) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.providers[name] = factory
}

// GetFactory returns the factory for the given provider name.
func GetFactory(name string) (ClientFactory, error) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	factory, exists := globalRegistry.providers[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s (supported: %s)", errUtils.ErrAIUnsupportedProvider, name, ListProviders())
	}

	return factory, nil
}

// ListProviders returns a sorted list of registered provider names.
func ListProviders() string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	names := make([]string, 0, len(globalRegistry.providers))
	for name := range globalRegistry.providers {
		names = append(names, name)
	}
	sort.Strings(names)

	result := ""
	for i, name := range names {
		if i > 0 {
			result += ", "
		}
		result += name
	}
	return result
}

// IsProviderRegistered returns true if the provider is registered.
func IsProviderRegistered(name string) bool {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	_, exists := globalRegistry.providers[name]
	return exists
}

// ProviderCount returns the number of registered providers.
func ProviderCount() int {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	return len(globalRegistry.providers)
}
