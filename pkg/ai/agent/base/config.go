// Package base provides shared utilities for AI provider implementations.
package base

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProviderDefaults contains default values for a provider.
type ProviderDefaults struct {
	Model         string
	DefaultAPIKey string
	MaxTokens     int
	BaseURL       string
}

// Config holds common configuration for AI clients.
type Config struct {
	Enabled   bool
	Model     string
	APIKey    string //nolint:gosec // G117: not a hardcoded credential, populated from config
	MaxTokens int
	BaseURL   string
}

// GetProviderConfig returns the provider-specific configuration from AtmosConfiguration.
// Returns nil if no provider configuration is found.
func GetProviderConfig(atmosConfig *schema.AtmosConfiguration, providerName string) *schema.AIProviderConfig {
	if atmosConfig.AI.Providers == nil {
		return nil
	}

	providerConfig, exists := atmosConfig.AI.Providers[providerName]
	if !exists || providerConfig == nil {
		return nil
	}

	return providerConfig
}

// ExtractConfig extracts AI configuration from AtmosConfiguration for a specific provider.
// It applies the provider-specific defaults and overrides from the configuration.
func ExtractConfig(atmosConfig *schema.AtmosConfiguration, providerName string, defaults ProviderDefaults) *Config {
	config := &Config{
		Enabled:   false,
		Model:     defaults.Model,
		APIKey:    defaults.DefaultAPIKey,
		MaxTokens: defaults.MaxTokens,
		BaseURL:   defaults.BaseURL,
	}

	// Check if AI is enabled.
	if atmosConfig.AI.Enabled {
		config.Enabled = true
	}

	// Apply provider-specific overrides.
	applyProviderOverrides(config, GetProviderConfig(atmosConfig, providerName))

	return config
}

// applyProviderOverrides applies provider-specific configuration overrides to the config.
func applyProviderOverrides(config *Config, providerConfig *schema.AIProviderConfig) {
	if providerConfig == nil {
		return
	}

	if providerConfig.Model != "" {
		config.Model = providerConfig.Model
	}
	if providerConfig.ApiKey != "" {
		config.APIKey = providerConfig.ApiKey
	}
	if providerConfig.MaxTokens > 0 {
		config.MaxTokens = providerConfig.MaxTokens
	}
	if providerConfig.BaseURL != "" {
		config.BaseURL = providerConfig.BaseURL
	}
}
