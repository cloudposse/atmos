package ai

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
	"github.com/cloudposse/atmos/pkg/ai/agent/azureopenai"
	"github.com/cloudposse/atmos/pkg/ai/agent/bedrock"
	"github.com/cloudposse/atmos/pkg/ai/agent/gemini"
	"github.com/cloudposse/atmos/pkg/ai/agent/grok"
	"github.com/cloudposse/atmos/pkg/ai/agent/ollama"
	"github.com/cloudposse/atmos/pkg/ai/agent/openai"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewClient creates a new AI client based on the provider configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (Client, error) {
	// Get provider from config.
	provider := getProvider(atmosConfig)

	// Create client based on provider.
	switch provider {
	case "anthropic":
		return anthropic.NewSimpleClient(atmosConfig)
	case "openai":
		return openai.NewClient(atmosConfig)
	case "gemini":
		// Gemini client requires context for initialization.
		ctx := context.Background()
		return gemini.NewClient(ctx, atmosConfig)
	case "grok":
		return grok.NewClient(atmosConfig)
	case "ollama":
		return ollama.NewClient(atmosConfig)
	case "bedrock":
		// Bedrock client requires context for AWS SDK initialization.
		ctx := context.Background()
		return bedrock.NewClient(ctx, atmosConfig)
	case "azureopenai":
		return azureopenai.NewClient(atmosConfig)
	default:
		return nil, fmt.Errorf("%w: %s (supported: anthropic, openai, gemini, grok, ollama, bedrock, azureopenai)", errUtils.ErrAIUnsupportedProvider, provider)
	}
}

// getProvider returns the active provider name.
func getProvider(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig.Settings.AI.DefaultProvider != "" {
		return atmosConfig.Settings.AI.DefaultProvider
	}

	// Default to anthropic.
	return "anthropic"
}

// GetProviderConfig returns the configuration for a specific provider.
func GetProviderConfig(atmosConfig *schema.AtmosConfiguration, provider string) (*schema.AIProviderConfig, error) {
	if atmosConfig.Settings.AI.Providers == nil {
		return nil, fmt.Errorf("%w: no providers configured", errUtils.ErrAIUnsupportedProvider)
	}

	config, exists := atmosConfig.Settings.AI.Providers[provider]
	if !exists {
		return nil, fmt.Errorf("%w: provider %s not configured in atmos.yaml", errUtils.ErrAIUnsupportedProvider, provider)
	}

	return config, nil
}
