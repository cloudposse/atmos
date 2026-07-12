package ai

import (
	"context"
	"fmt"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/claudecode"
	"github.com/cloudposse/atmos/pkg/ai/agent/codexcli"
	"github.com/cloudposse/atmos/pkg/ai/agent/copilotcli"
	"github.com/cloudposse/atmos/pkg/ai/agent/geminicli"
	"github.com/cloudposse/atmos/pkg/ai/registry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cliProviderPriority lists CLI providers in auto-detection priority order, along with
// the binary that must be found on PATH for that provider to be selected.
var cliProviderPriority = []struct {
	Provider string
	Binary   string
}{
	{claudecode.ProviderName, claudecode.DefaultBinary},
	{codexcli.ProviderName, codexcli.DefaultBinary},
	{copilotcli.ProviderName, copilotcli.DefaultBinary},
	{geminicli.ProviderName, geminicli.DefaultBinary},
}

// NewClient creates a new AI client based on the provider configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (Client, error) {
	return NewClientWithContext(context.Background(), atmosConfig)
}

// NewClientWithContext creates a new AI client with the given context.
func NewClientWithContext(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (Client, error) {
	// Get provider from config.
	provider := GetProvider(atmosConfig)

	// Get factory for provider.
	factory, err := registry.GetFactory(provider)
	if err != nil {
		return nil, err
	}

	// Create client using factory.
	return factory(ctx, atmosConfig)
}

// GetProvider returns the AI provider to use: the explicitly configured provider if set,
// otherwise the first CLI provider (claude-code, codex-cli, copilot-cli, gemini-cli) whose
// binary is found on PATH, otherwise "anthropic".
func GetProvider(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig.AI.DefaultProvider != "" {
		return atmosConfig.AI.DefaultProvider
	}

	if detected := detectCLIProvider(exec.LookPath); detected != "" {
		return detected
	}

	// Default to anthropic.
	return "anthropic"
}

// detectCLIProvider returns the name of the first CLI provider whose binary is found via
// lookup, or "" if none are found. Lookup is injected so tests don't depend on the real PATH.
func detectCLIProvider(lookup func(string) (string, error)) string {
	for _, p := range cliProviderPriority {
		if _, err := lookup(p.Binary); err == nil {
			return p.Provider
		}
	}
	return ""
}

// cliProviders is the set of provider names that invoke a local CLI binary as a subprocess.
var cliProviders = map[string]bool{
	claudecode.ProviderName: true,
	codexcli.ProviderName:   true,
	copilotcli.ProviderName: true,
	geminicli.ProviderName:  true,
}

// IsCLIProvider returns true if the named provider invokes a local CLI binary rather than
// calling an API directly.
func IsCLIProvider(providerName string) bool {
	return cliProviders[providerName]
}

// GetProviderConfig returns the configuration for a specific provider.
func GetProviderConfig(atmosConfig *schema.AtmosConfiguration, provider string) (*schema.AIProviderConfig, error) {
	if atmosConfig.AI.Providers == nil {
		return nil, fmt.Errorf("%w: no providers configured", errUtils.ErrAIUnsupportedProvider)
	}

	config, exists := atmosConfig.AI.Providers[provider]
	if !exists {
		return nil, fmt.Errorf("%w: provider %s not configured in atmos.yaml", errUtils.ErrAIUnsupportedProvider, provider)
	}

	return config, nil
}
