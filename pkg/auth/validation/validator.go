package validation

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/factory"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validator implements the Validator interface.
type validator struct{}

// NewValidator creates a new validator instance.
func NewValidator() types.Validator {
	return &validator{}
}

// ErrIdentityCycle signals a circular dependency in an identity chain.
var ErrIdentityCycle = errors.New("identity cycle detected")

// ValidateAuthConfig validates the entire auth configuration.
func (v *validator) ValidateAuthConfig(config *schema.AuthConfig) error {
	defer perf.Track(nil, "validation.ValidateAuthConfig")()

	if config == nil {
		return fmt.Errorf("%w: auth config cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	// Validate logs.
	if err := v.ValidateLogsConfig(&config.Logs); err != nil {
		return fmt.Errorf("%w: logs configuration validation failed: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	// Validate providers.
	//nolint:gocritic // rangeValCopy: map stores structs; address of map element can't be taken. Passing copy to factory is intended.
	for name, provider := range config.Providers {
		if err := v.ValidateProvider(name, &provider); err != nil {
			return fmt.Errorf("%w: provider %q validation failed: %v", errUtils.ErrInvalidAuthConfig, name, err)
		}
	}

	// Validate identities.

	for name, identity := range config.Identities {
		if err := v.ValidateIdentity(name, &identity, convertProviders(config.Providers)); err != nil {
			return fmt.Errorf("%w: identity %q validation failed: %v", errUtils.ErrInvalidAuthConfig, name, err)
		}
	}

	// Validate chains.
	if err := v.ValidateChains(convertIdentities(config.Identities), convertProviders(config.Providers)); err != nil {
		return fmt.Errorf("%w: identity chain validation failed: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	return nil
}

// ValidateLogsConfig validates the logs configuration.
func (v *validator) ValidateLogsConfig(logs *schema.Logs) error {
	defer perf.Track(nil, "validation.ValidateLogsConfig")()

	if logs.Level == "" {
		// Default to Info if not specified.
		return nil
	}

	validLevels := []string{"Debug", "Info", "Warn", "Error"}
	for _, validLevel := range validLevels {
		if logs.Level == validLevel {
			return nil
		}
	}

	return fmt.Errorf("%w: invalid log level %q, must be one of: %s", errUtils.ErrInvalidAuthConfig, logs.Level, strings.Join(validLevels, ", "))
}

// ValidateProvider validates a provider configuration.
func (v *validator) ValidateProvider(name string, provider *schema.Provider) error {
	defer perf.Track(nil, "validation.ValidateProvider")()

	if name == "" {
		return fmt.Errorf("%w: provider name cannot be empty", errUtils.ErrInvalidProviderConfig)
	}

	if provider.Kind == "" {
		return fmt.Errorf("%w: provider kind is required", errUtils.ErrInvalidProviderConfig)
	}

	// Create provider instance and use its Validate method.
	providerInstance, err := factory.NewProvider(name, provider)
	if err != nil {
		return err
	}

	return providerInstance.Validate()
}

// ValidateIdentity validates an identity configuration.
func (v *validator) ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error {
	defer perf.Track(nil, "validation.ValidateIdentity")()

	if name == "" {
		return fmt.Errorf("%w: identity name cannot be empty", errUtils.ErrInvalidIdentityConfig)
	}

	if identity.Kind == "" {
		return fmt.Errorf("%w: identity kind is required", errUtils.ErrInvalidIdentityConfig)
	}

	// Validate via configuration - AWS User identities don't require via provider.
	if err := v.validateViaConfiguration(identity, providers); err != nil {
		return err
	}

	// Create identity instance and use its Validate method.
	identityInstance, err := factory.NewIdentity(name, identity)
	if err != nil {
		return err
	}

	return identityInstance.Validate()
}

// validateViaConfiguration validates the optional Via provider/identity references for an identity.
func (v *validator) validateViaConfiguration(identity *schema.Identity, providers map[string]*schema.Provider) error {
	if identity.Kind == "aws/user" || identity.Via == nil {
		return nil
	}

	// Enforce mutual exclusivity: exactly one of Provider or Identity must be set.
	hasProvider := identity.Via.Provider != ""
	hasIdentity := identity.Via.Identity != ""

	if !hasProvider && !hasIdentity {
		return fmt.Errorf("%w: exactly one of via.provider or via.identity must be set", errUtils.ErrInvalidIdentityConfig)
	}

	if hasProvider && hasIdentity {
		return fmt.Errorf("%w: via.provider and via.identity are mutually exclusive; only one can be set", errUtils.ErrInvalidIdentityConfig)
	}

	// Validate that referenced provider exists.
	if hasProvider {
		if _, exists := providers[identity.Via.Provider]; !exists {
			return fmt.Errorf("%w: referenced provider %q does not exist", errUtils.ErrInvalidAuthConfig, identity.Via.Provider)
		}
	}

	return nil
}

// ValidateChains validates identity chains for cycles and invalid references.
func (v *validator) ValidateChains(identities map[string]*schema.Identity, _ map[string]*schema.Provider) error {
	defer perf.Track(nil, "validation.ValidateChains")()
	// Build dependency graph.
	graph := make(map[string][]string)

	for name, identity := range identities {
		if identity.Via != nil {
			if identity.Via.Identity != "" {
				if _, ok := identities[identity.Via.Identity]; !ok {
					return fmt.Errorf("%w: referenced identity %q does not exist", errUtils.ErrInvalidAuthConfig, identity.Via.Identity)
				}
				graph[name] = append(graph[name], identity.Via.Identity)
			}
		}
	}

	// Check for cycles using DFS.
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for name := range identities {
		if !visited[name] {
			if v.hasCycle(name, graph, visited, recStack) {
				// Return a domain-specific sentinel to enable precise error checks by callers/tests.
				return fmt.Errorf("%w: circular dependency detected in identity chain involving %q", ErrIdentityCycle, name)
			}
		}
	}

	return nil
}

// hasCycle performs DFS to detect cycles in the dependency graph.
func (v *validator) hasCycle(node string, graph map[string][]string, visited, recStack map[string]bool) bool {
	visited[node] = true
	recStack[node] = true

	for _, neighbor := range graph[node] {
		if !visited[neighbor] {
			if v.hasCycle(neighbor, graph, visited, recStack) {
				return true
			}
		} else if recStack[neighbor] {
			return true
		}
	}

	recStack[node] = false
	return false
}

// Helper functions to convert map types.
func convertProviders(providers map[string]schema.Provider) map[string]*schema.Provider {
	result := make(map[string]*schema.Provider)
	//nolint:gocritic // rangeValCopy: map stores structs; address of map element can't be taken. Passing copy to factory is intended.
	for k, v := range providers {
		provider := v
		result[k] = &provider
	}
	return result
}

func convertIdentities(identities map[string]schema.Identity) map[string]*schema.Identity {
	result := make(map[string]*schema.Identity)
	for k, v := range identities {
		identity := v
		result[k] = &identity
	}
	return result
}
