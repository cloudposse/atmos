package auth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Logout removes credentials for the specified identity only.
// Provider and chain credentials are preserved for use by other identities.
// If deleteKeychain is true, also removes credentials from system keychain.
func (m *manager) Logout(ctx context.Context, identityName string, deleteKeychain bool) error {
	defer perf.Track(nil, "auth.Manager.Logout")()

	// Validate identity exists in configuration.
	identity, exists := m.identities[identityName]
	if !exists {
		return errUtils.Build(errUtils.ErrIdentityNotInConfig).
			WithExplanation(fmt.Sprintf("Identity %q not found in configuration", identityName)).
			WithHint("Run `atmos list identities` to see available identities").
			WithHint("Check your auth configuration in atmos.yaml").
			WithContext("profile", FormatProfile(m.getProfiles())).
			WithContext("identity", identityName).
			Err()
	}

	log.Debug("Logout identity", logKeyIdentity, identityName, "deleteKeychain", deleteKeychain)

	var errs []error

	// Step 1: Delete keyring entry ONLY if deleteKeychain flag is set.
	if deleteKeychain {
		if err := m.credentialStore.Delete(identityName); err != nil {
			log.Debug("Failed to delete keyring entry (may not exist)", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, identityName, err))
		} else {
			log.Debug("Deleted keyring entry", logKeyIdentity, identityName)
		}
	} else {
		log.Debug("Skipping keyring deletion (preserving credentials)", logKeyIdentity, identityName)
	}

	// Step 2: Call identity-specific cleanup (each identity type handles its own file cleanup).
	if err := identity.Logout(ctx); err != nil {
		// ErrLogoutNotSupported is a successful no-op (exit 0).
		if !errors.Is(err, errUtils.ErrLogoutNotSupported) {
			log.Debug("Identity logout failed", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrIdentityLogout, identityName, err))
		} else {
			log.Debug("Identity logout not supported (no-op)", logKeyIdentity, identityName)
		}
	} else {
		log.Debug("Identity logout succeeded", logKeyIdentity, identityName)
	}

	log.Info("Logout completed", logKeyIdentity, identityName, "errors", len(errs), "deletedKeychain", deleteKeychain)

	if len(errs) > 0 {
		return errors.Join(append([]error{errUtils.ErrPartialLogout}, errs...)...)
	}

	return nil
}

// resolveProviderForIdentity follows the Via chain to find the root provider for an identity.
// Returns empty string if no provider is found or if a cycle is detected.
func (m *manager) resolveProviderForIdentity(identityName string) string {
	visited := make(map[string]bool)
	current := identityName

	for {
		// Check for cycles.
		if visited[current] {
			log.Debug("Cycle detected while resolving provider", logKeyIdentity, current)
			return ""
		}
		visited[current] = true

		// Get identity configuration.
		identity, exists := m.config.Identities[current]
		if !exists {
			log.Debug("Missing identity reference while resolving provider", logKeyIdentity, current)
			return ""
		}

		// Check if identity has Via configuration.
		if identity.Via == nil {
			return ""
		}

		// Found a direct provider reference.
		if identity.Via.Provider != "" {
			return identity.Via.Provider
		}

		// Follow the identity chain.
		if identity.Via.Identity != "" {
			current = identity.Via.Identity
			continue
		}

		// No provider or identity reference.
		return ""
	}
}

// LogoutProvider removes all credentials for the specified provider and all identities that use it.
// If deleteKeychain is true, also removes credentials from system keychain.
func (m *manager) LogoutProvider(ctx context.Context, providerName string, deleteKeychain bool) error { //nolint:revive
	defer perf.Track(nil, "auth.Manager.LogoutProvider")()

	// Validate provider exists in configuration.
	provider, exists := m.providers[providerName]
	if !exists {
		return errUtils.Build(errUtils.ErrProviderNotInConfig).
			WithExplanation(fmt.Sprintf("Provider %q not found in configuration", providerName)).
			WithHint("Run `atmos list providers` to see available providers").
			WithHint("Check your auth configuration in atmos.yaml").
			WithContext("profile", FormatProfile(m.getProfiles())).
			WithContext("provider", providerName).
			Err()
	}

	log.Debug("Logout provider", logKeyProvider, providerName, "deleteKeychain", deleteKeychain)

	// Find all identities that use this provider (directly or transitively).
	var identityNames []string
	for name := range m.config.Identities {
		if m.resolveProviderForIdentity(name) == providerName {
			identityNames = append(identityNames, name)
		}
	}

	if len(identityNames) == 0 {
		log.Debug("No identities found for provider", logKeyProvider, providerName)
	}

	var errs []error

	// Logout each identity (pass deleteKeychain flag).
	for _, identityName := range identityNames {
		if err := m.Logout(ctx, identityName, deleteKeychain); err != nil {
			log.Debug("Failed to logout identity", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrIdentityLogout, identityName, err))
		}
	}

	// Delete provider credentials from keyring ONLY if deleteKeychain flag is set.
	if deleteKeychain {
		if err := m.credentialStore.Delete(providerName); err != nil {
			log.Debug("Failed to delete provider keyring entry", logKeyProvider, providerName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, providerName, err))
		}
	} else {
		log.Debug("Skipping provider keyring deletion (preserving credentials)", logKeyProvider, providerName)
	}

	// Call provider-specific cleanup (deletes all provider files).
	if err := provider.Logout(ctx); err != nil {
		// ErrLogoutNotSupported is a successful no-op (exit 0).
		if !errors.Is(err, errUtils.ErrLogoutNotSupported) {
			log.Debug("Provider logout failed", logKeyProvider, providerName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrProviderLogout, providerName, err))
		} else {
			log.Debug("Provider logout not supported (no-op)", logKeyProvider, providerName)
		}
	} else {
		log.Debug("Provider logout succeeded", logKeyProvider, providerName)
	}

	// Clean up auto-provisioned identities cache file if it exists.
	if err := m.removeProvisionedIdentitiesCache(providerName); err != nil {
		log.Debug("Failed to remove provisioned identities cache", logKeyProvider, providerName, "error", err)
		errs = append(errs, fmt.Errorf("failed to remove provisioned identities cache for provider %q: %w", providerName, err))
	}

	log.Info("Provider logout completed", logKeyProvider, providerName, "identities", len(identityNames), "errors", len(errs), "deletedKeychain", deleteKeychain)

	if len(errs) > 0 {
		return errors.Join(append([]error{errUtils.ErrLogoutFailed}, errs...)...)
	}

	return nil
}

// LogoutAll removes all cached credentials for all identities and providers.
// If deleteKeychain is true, also removes credentials from system keychain.
func (m *manager) LogoutAll(ctx context.Context, deleteKeychain bool) error {
	defer perf.Track(nil, "auth.Manager.LogoutAll")()

	log.Debug("Logout all identities and providers", "deleteKeychain", deleteKeychain)

	var errs []error

	// Logout each identity (pass deleteKeychain flag).
	for identityName := range m.config.Identities {
		if err := m.Logout(ctx, identityName, deleteKeychain); err != nil {
			log.Debug("Failed to logout identity", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf("%w for identity %q: %w", errUtils.ErrIdentityLogout, identityName, err))
		}
	}

	// Logout each provider.
	for providerName, provider := range m.providers {
		// Delete provider credentials from keyring ONLY if deleteKeychain flag is set.
		if deleteKeychain {
			if err := m.credentialStore.Delete(providerName); err != nil {
				log.Debug("Failed to delete provider keyring entry", logKeyProvider, providerName, "error", err)
				errs = append(errs, fmt.Errorf("%w for provider %q: %w", errUtils.ErrKeyringDeletion, providerName, err))
			}
		} else {
			log.Debug("Skipping provider keyring deletion (preserving credentials)", logKeyProvider, providerName)
		}

		// Call provider-specific cleanup (deletes all provider files).
		if err := provider.Logout(ctx); err != nil {
			// ErrLogoutNotSupported is a successful no-op (exit 0).
			if !errors.Is(err, errUtils.ErrLogoutNotSupported) {
				log.Debug("Provider logout failed", logKeyProvider, providerName, "error", err)
				errs = append(errs, fmt.Errorf("%w for provider %q: %w", errUtils.ErrProviderLogout, providerName, err))
			} else {
				log.Debug("Provider logout not supported (no-op)", logKeyProvider, providerName)
			}
		} else {
			log.Debug("Provider logout succeeded", logKeyProvider, providerName)
		}

		// Clean up auto-provisioned identities cache file if it exists.
		if err := m.removeProvisionedIdentitiesCache(providerName); err != nil {
			log.Debug("Failed to remove provisioned identities cache", logKeyProvider, providerName, "error", err)
			errs = append(errs, fmt.Errorf("failed to remove provisioned identities cache for provider %q: %w", providerName, err))
		}
	}

	log.Info("Logout all completed", "identities", len(m.config.Identities), "providers", len(m.providers), "errors", len(errs), "deletedKeychain", deleteKeychain)

	if len(errs) > 0 {
		return errors.Join(append([]error{errUtils.ErrLogoutFailed}, errs...)...)
	}

	return nil
}

// removeProvisionedIdentitiesCache removes the auto-provisioned identities cache file for a provider.
// This is called during provider logout to clean up auto-provisioned identities.
func (m *manager) removeProvisionedIdentitiesCache(providerName string) error {
	defer perf.Track(nil, "auth.Manager.removeProvisionedIdentitiesCache")()

	// Create a provisioning writer to get the cache file path.
	writer, err := types.NewProvisioningWriter()
	if err != nil {
		log.Debug("Failed to create provisioning writer", logKeyProvider, providerName, "error", err)
		return fmt.Errorf("failed to create provisioning writer: %w", err)
	}

	// Remove the provisioned identities cache file.
	if err := writer.Remove(providerName); err != nil {
		return fmt.Errorf("failed to remove provisioned identities cache: %w", err)
	}

	log.Debug("Removed provisioned identities cache", logKeyProvider, providerName)
	return nil
}
