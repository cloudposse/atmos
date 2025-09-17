package ci

import (
	log "github.com/charmbracelet/log"
	"github.com/cloudposse/gotcha/pkg/config"
	"github.com/cloudposse/gotcha/pkg/constants"
)

// IntegrationFactory is a function that creates a CI integration.
type IntegrationFactory func(*log.Logger) Integration

// integrations is the registry of available CI integrations.
// This will be populated by init() functions in integration packages.
var integrations = map[string]IntegrationFactory{}

// DetectIntegration automatically detects and returns the appropriate CI integration.
// This returns an integration only if we have an implementation for the detected CI provider.
// Returns nil if no supported integration is available, even if IsCI() returns true.
func DetectIntegration(logger *log.Logger) Integration {
	// Check for manual override via environment variable or config
	if provider := config.GetCIProvider(); provider != "" {
		if factory, ok := integrations[provider]; ok {
			integration := factory(logger)
			if integration.IsAvailable() {
				logger.Debug("Using manually specified CI provider", constants.ProviderField, provider)
				return integration
			}
			logger.Warn("Manually specified CI provider not available", constants.ProviderField, provider)
		} else {
			logger.Warn("Unknown CI provider specified", constants.ProviderField, provider)
		}
	}

	// Check for mock integration first if GOTCHA_USE_MOCK is set
	if config.UseMock() {
		if factory, ok := integrations["mock"]; ok {
			integration := factory(logger)
			if integration.IsAvailable() {
				logger.Debug("Using mock CI integration", constants.ProviderField, "mock")
				return integration
			}
		}
	}

	// Auto-detect based on environment
	// Try integrations in a specific order for predictable behavior
	// Only providers with actual implementations are listed here
	orderedProviders := []string{
		GitHub, // Currently the only implemented provider
		// Future providers will be added here as they are implemented
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
// This is used for output formatting decisions (colors, progress bars, etc.)
// and is distinct from whether we have an integration for that CI provider.
// For example, we may be running in GitLab CI (IsCI() returns true) but
// not have a GitLab integration (DetectIntegration() returns nil).
func IsCI() bool {
	// Use the centralized config package for CI detection
	return config.IsCI()
}
