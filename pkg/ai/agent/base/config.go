// Package base provides shared utilities for AI provider implementations.
package base

import (
	"net"
	"net/url"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DefaultRequestTimeout is the default HTTP request timeout for AI provider API calls.
const DefaultRequestTimeout = 60 * time.Second

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

// ValidateProviderBaseURL rejects an http:// base URL when an API key will be sent with the
// request, because the bearer token would travel in cleartext (CWE-319). Loopback hosts
// (localhost, 127.0.0.1, ::1) are allowed so local proxies and test servers keep working, and
// empty or https URLs always pass. This is a pure validation helper shared by credentialed
// OpenAI-compatible providers; unauthenticated local providers (e.g. Ollama) never call it.
func ValidateProviderBaseURL(baseURL string, hasAPIKey bool) error {
	if baseURL == "" || !hasAPIKey {
		return nil
	}
	// A malformed base_url is surfaced later by the HTTP client; this check only guards the
	// specific insecure case: cleartext http to a non-loopback host while sending an API key.
	if u, err := url.Parse(baseURL); err == nil &&
		strings.EqualFold(u.Scheme, "http") && !isLoopbackHost(u.Hostname()) {
		return errUtils.Build(errUtils.ErrAIInsecureBaseURL).
			WithContext("base_url", baseURL).
			WithHint("Use an https:// endpoint, or omit the API key for a local (loopback) provider such as Ollama.").
			Err()
	}
	return nil
}

// isLoopbackHost reports whether host is localhost or a loopback IP literal.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ResolveToolchainPATH extracts the toolchain bin PATH for MCP server subprocesses.
// Returns the PATH string from the toolchain environment, or empty if no toolchain is configured.
func ResolveToolchainPATH(atmosConfig *schema.AtmosConfiguration) string {
	deps, err := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if err != nil || len(deps) == 0 {
		return ""
	}
	tenv, err := dependencies.NewEnvironmentFromDeps(atmosConfig, deps)
	if err != nil || tenv == nil {
		return ""
	}
	for _, envVar := range tenv.EnvVars() {
		if strings.HasPrefix(envVar, "PATH=") {
			return envVar[len("PATH="):]
		}
	}
	return ""
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
