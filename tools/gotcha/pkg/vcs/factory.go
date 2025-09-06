package vcs

import (
	"os"
	
	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// ProviderFactory is a function that creates a VCS provider.
type ProviderFactory func(*log.Logger) Provider

// providers is the registry of available VCS providers.
// This will be populated by init() functions in provider packages.
var providers = map[Platform]ProviderFactory{}

// DetectProvider automatically detects and returns the appropriate VCS provider.
func DetectProvider(logger *log.Logger) Provider {
	// Check for manual override via environment variable or config
	_ = viper.BindEnv("GOTCHA_VCS_PLATFORM", "VCS_PLATFORM")
	if platform := viper.GetString("GOTCHA_VCS_PLATFORM"); platform != "" {
		if factory, ok := providers[Platform(platform)]; ok {
			provider := factory(logger)
			if provider.IsAvailable() {
				logger.Debug("Using manually specified VCS provider", "platform", platform)
				return provider
			}
			logger.Warn("Manually specified VCS provider not available", "platform", platform)
		} else {
			logger.Warn("Unknown VCS platform specified", "platform", platform)
		}
	}
	
	// Check for mock provider first if GOTCHA_USE_MOCK is set
	if os.Getenv("GOTCHA_USE_MOCK") == "true" {
		if factory, ok := providers[Platform("mock")]; ok {
			provider := factory(logger)
			if provider.IsAvailable() {
				logger.Debug("Using mock VCS provider", "platform", "mock")
				return provider
			}
		}
	}
	
	// Auto-detect based on environment
	// Try providers in a specific order for predictable behavior
	orderedPlatforms := []Platform{
		PlatformGitHub,
		// Add other platforms here as they are implemented
	}
	
	for _, platform := range orderedPlatforms {
		if factory, ok := providers[platform]; ok {
			provider := factory(logger)
			if provider.IsAvailable() {
				logger.Debug("Auto-detected VCS provider", "platform", platform)
				return provider
			}
		}
	}
	
	logger.Debug("No VCS provider detected in current environment")
	return nil
}

// GetProvider returns a specific VCS provider by platform.
func GetProvider(platform Platform, logger *log.Logger) Provider {
	if factory, ok := providers[platform]; ok {
		return factory(logger)
	}
	return nil
}

// RegisterProvider registers a new VCS provider factory.
// This allows for extensibility and testing.
func RegisterProvider(platform Platform, factory ProviderFactory) {
	providers[platform] = factory
}

// GetSupportedPlatforms returns a list of all supported VCS platforms.
func GetSupportedPlatforms() []Platform {
	platforms := make([]Platform, 0, len(providers))
	for platform := range providers {
		platforms = append(platforms, platform)
	}
	return platforms
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