package bedrock

import (
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cacheConfig holds prompt caching settings for Claude on Bedrock.
// It reads the same `cache` provider settings as the Anthropic provider, with the same defaults.
type cacheConfig struct {
	enabled                  bool
	cacheSystemPrompt        bool
	cacheProjectInstructions bool
}

// defaultCacheConfig returns the defaults: caching enabled for the system prompt and ATMOS.md.
func defaultCacheConfig() *cacheConfig {
	return &cacheConfig{enabled: true, cacheSystemPrompt: true, cacheProjectInstructions: true}
}

// extractCacheConfig extracts Bedrock cache settings from AtmosConfiguration.
func extractCacheConfig(atmosConfig *schema.AtmosConfiguration) *cacheConfig {
	providerConfig := base.GetProviderConfig(atmosConfig, ProviderName)
	if providerConfig == nil || providerConfig.Cache == nil {
		return defaultCacheConfig()
	}

	settings := providerConfig.Cache
	if !settings.Enabled {
		return &cacheConfig{}
	}

	// Enabled with no fine-grained settings: cache both blocks.
	if !settings.CacheSystemPrompt && !settings.CacheProjectInstructions {
		return defaultCacheConfig()
	}

	return &cacheConfig{
		enabled:                  true,
		cacheSystemPrompt:        settings.CacheSystemPrompt,
		cacheProjectInstructions: settings.CacheProjectInstructions,
	}
}
