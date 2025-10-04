package ai

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
	"github.com/cloudposse/atmos/pkg/ai/agent/gemini"
	"github.com/cloudposse/atmos/pkg/ai/agent/grok"
	"github.com/cloudposse/atmos/pkg/ai/agent/openai"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewClient creates a new AI client based on the provider configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (Client, error) {
	if atmosConfig.Settings.AI == nil {
		return nil, fmt.Errorf("AI settings not configured")
	}

	// Get provider from config, default to "anthropic".
	provider := "anthropic"
	if p, ok := atmosConfig.Settings.AI["provider"].(string); ok && p != "" {
		provider = p
	}

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
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s (supported: anthropic, openai, gemini, grok)", provider)
	}
}
