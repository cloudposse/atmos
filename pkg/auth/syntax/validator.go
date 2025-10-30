package syntax

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateSyntax performs lightweight syntactic validation of auth configuration.
// It checks that provider and identity kinds are valid without attempting to create
// instances or validate actual authentication. This is suitable for early validation
// during config loading without circular dependencies on the factory package.
func ValidateSyntax(authConfig *schema.AuthConfig) error {
	defer perf.Track(nil, "validation.ValidateSyntax")()

	if authConfig == nil {
		return fmt.Errorf("%w: auth config cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	// Skip validation if auth config is empty.
	if len(authConfig.Providers) == 0 && len(authConfig.Identities) == 0 {
		return nil
	}

	// Validate providers.
	for name := range authConfig.Providers {
		provider := authConfig.Providers[name]
		if err := validateProviderKind(provider.Kind); err != nil {
			return fmt.Errorf("%w: provider %q validation failed: %w", errUtils.ErrInvalidProviderConfig, name, err)
		}
	}

	// Validate identities.
	for name := range authConfig.Identities {
		identity := authConfig.Identities[name]
		if err := validateIdentityKind(identity.Kind); err != nil {
			return fmt.Errorf("%w: identity %q validation failed: %w", errUtils.ErrInvalidIdentityConfig, name, err)
		}
	}

	return nil
}

// validateProviderKind checks if a provider kind is valid.
func validateProviderKind(kind string) error {
	validKinds := map[string]bool{
		types.ProviderKindAWSIAMIdentityCenter: true,
		types.ProviderKindAWSSAML:              true,
		types.ProviderKindAzureOIDC:            true,
		types.ProviderKindGCPOIDC:              true,
		types.ProviderKindGitHubOIDC:           true,
		"mock":                                 true, // Mock provider for testing.
	}

	if !validKinds[kind] {
		return fmt.Errorf("%w: %q is not a valid provider kind", errUtils.ErrInvalidProviderKind, kind)
	}

	return nil
}

// validateIdentityKind checks if an identity kind is valid.
func validateIdentityKind(kind string) error {
	validKinds := map[string]bool{
		types.ProviderKindAWSUser:          true,
		types.ProviderKindAWSAssumeRole:    true,
		types.ProviderKindAWSPermissionSet: true,
		"mock":                             true, // Mock identity for testing.
	}

	if !validKinds[kind] {
		return fmt.Errorf("%w: %q is not a valid identity kind", errUtils.ErrInvalidIdentityKind, kind)
	}

	return nil
}
