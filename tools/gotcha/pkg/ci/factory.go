package ci

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// IntegrationFactory is a function that creates a CI integration.
type IntegrationFactory func(*log.Logger) Integration

// integrations is the registry of available CI integrations.
// This will be populated by init() functions in integration packages.
var integrations = map[string]IntegrationFactory{}

// DetectIntegration automatically detects and returns the appropriate CI integration.
func DetectIntegration(logger *log.Logger) Integration {
	// Check for manual override via environment variable or config
	_ = viper.BindEnv("GOTCHA_CI_PROVIDER", "CI_PROVIDER")
	if provider := viper.GetString("GOTCHA_CI_PROVIDER"); provider != "" {
		if factory, ok := integrations[provider]; ok {
			integration := factory(logger)
			if integration.IsAvailable() {
				logger.Debug("Using manually specified CI provider", "provider", provider)
				return integration
			}
			logger.Warn("Manually specified CI provider not available", "provider", provider)
		} else {
			logger.Warn("Unknown CI provider specified", "provider", provider)
		}
	}

	// Check for mock integration first if GOTCHA_USE_MOCK is set
	if os.Getenv("GOTCHA_USE_MOCK") == "true" {
		if factory, ok := integrations["mock"]; ok {
			integration := factory(logger)
			if integration.IsAvailable() {
				logger.Debug("Using mock CI integration", "provider", "mock")
				return integration
			}
		}
	}

	// Auto-detect based on environment
	// Try integrations in a specific order for predictable behavior
	orderedProviders := []string{
		GitHub,
		// Add other providers here as they are implemented
	}

	for _, provider := range orderedProviders {
		if factory, ok := integrations[provider]; ok {
			integration := factory(logger)
			if integration.IsAvailable() {
				logger.Debug("Auto-detected CI provider", "provider", provider)
				return integration
			}
		}
	}

	logger.Debug("No CI integration detected in current environment")
	return nil
}

// GetIntegration returns a specific CI integration by provider name.
func GetIntegration(provider string, logger *log.Logger) Integration {
	if factory, ok := integrations[provider]; ok {
		return factory(logger)
	}
	return nil
}

// RegisterIntegration registers a new CI integration factory.
// This allows for extensibility and testing.
func RegisterIntegration(provider string, factory IntegrationFactory) {
	integrations[provider] = factory
}

// GetSupportedProviders returns a list of all supported CI providers.
func GetSupportedProviders() []string {
	providers := make([]string, 0, len(integrations))
	for provider := range integrations {
		providers = append(providers, provider)
	}
	return providers
}

// IsCI detects if running in any CI environment.
func IsCI() bool {
	// Check common CI environment variables
	ciEnvVars := []string{
		"CI",
		"CONTINUOUS_INTEGRATION",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"BITBUCKET_PIPELINES",
		"SYSTEM_TEAMFOUNDATIONCOLLECTIONURI", // Azure DevOps
		"JENKINS_URL",
		"CIRCLECI",
		"TRAVIS",
	}

	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	return false
}
