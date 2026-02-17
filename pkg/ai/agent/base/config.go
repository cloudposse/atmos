// Package base provides shared utilities for AI provider implementations.
package base

import (
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ProviderDefaults contains default values for a provider.
type ProviderDefaults struct {
	Model     string
	APIKeyEnv string
	MaxTokens int
	BaseURL   string
}

// Config holds common configuration for AI clients.
type Config struct {
	Enabled   bool
	Model     string
	APIKeyEnv string
	MaxTokens int
	BaseURL   string
}

// ExtractConfig extracts AI configuration from AtmosConfiguration for a specific provider.
// It applies the provider-specific defaults and overrides from the configuration.
func ExtractConfig(atmosConfig *schema.AtmosConfiguration, providerName string, defaults ProviderDefaults) *Config {
	config := &Config{
		Enabled:   false,
		Model:     defaults.Model,
		APIKeyEnv: defaults.APIKeyEnv,
		MaxTokens: defaults.MaxTokens,
		BaseURL:   defaults.BaseURL,
	}

	// Check if AI is enabled.
	if atmosConfig.Settings.AI.Enabled {
		config.Enabled = true
	}

	// Get provider-specific configuration from Providers map.
	if atmosConfig.Settings.AI.Providers != nil {
		if providerConfig, exists := atmosConfig.Settings.AI.Providers[providerName]; exists && providerConfig != nil {
			// Override defaults with provider-specific configuration.
			if providerConfig.Model != "" {
				config.Model = providerConfig.Model
			}
			if providerConfig.ApiKeyEnv != "" {
				config.APIKeyEnv = providerConfig.ApiKeyEnv
			}
			if providerConfig.MaxTokens > 0 {
				config.MaxTokens = providerConfig.MaxTokens
			}
			if providerConfig.BaseURL != "" {
				config.BaseURL = providerConfig.BaseURL
			}
		}
	}

	return config
}

// GetAPIKey retrieves the API key from environment using the specified env var name.
// Uses a fresh viper instance with AutomaticEnv to read the environment variable.
func GetAPIKey(envVarName string) string {
	v := viper.New()
	v.AutomaticEnv()
	// Set a default empty string to ensure the key exists for viper to look up.
	v.SetDefault(envVarName, "")
	return v.GetString(envVarName)
}
