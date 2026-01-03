package ai

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/registry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewClient creates a new AI client based on the provider configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (Client, error) {
	return NewClientWithContext(context.Background(), atmosConfig)
}

// NewClientWithContext creates a new AI client with the given context.
func NewClientWithContext(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (Client, error) {
	// Get provider from config.
	provider := getProvider(atmosConfig)

	// Get factory for provider.
	factory, err := registry.GetFactory(provider)
	if err != nil {
		return nil, err
	}

	// Create client using factory.
	return factory(ctx, atmosConfig)
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
