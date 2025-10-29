package auth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Logout removes credentials for the specified identity only.
// Provider and chain credentials are preserved for use by other identities.
func (m *manager) Logout(ctx context.Context, identityName string) error {
	defer perf.Track(nil, "auth.Manager.Logout")()

	// Validate identity exists in configuration.
	identity, exists := m.identities[identityName]
	if !exists {
		return fmt.Errorf("%w: identity %q", errUtils.ErrIdentityNotInConfig, identityName)
	}

	log.Debug("Logout identity", logKeyIdentity, identityName)

	var errs []error

	// Step 1: Delete keyring entry for this identity only.
	if err := m.credentialStore.Delete(identityName); err != nil {
		log.Debug("Failed to delete keyring entry (may not exist)", logKeyIdentity, identityName, "error", err)
		errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, identityName, err))
	} else {
		log.Debug("Deleted keyring entry", logKeyIdentity, identityName)
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

	log.Info("Logout completed", logKeyIdentity, identityName, "errors", len(errs))

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
func (m *manager) LogoutProvider(ctx context.Context, providerName string) error {
	defer perf.Track(nil, "auth.Manager.LogoutProvider")()

	// Validate provider exists in configuration.
	provider, exists := m.providers[providerName]
	if !exists {
		return fmt.Errorf("%w: provider %q", errUtils.ErrProviderNotInConfig, providerName)
	}

	log.Debug("Logout provider", logKeyProvider, providerName)

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

	// Logout each identity (removes keyring entries and identity-specific files).
	for _, identityName := range identityNames {
		if err := m.Logout(ctx, identityName); err != nil {
			log.Debug("Failed to logout identity", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrIdentityLogout, identityName, err))
		}
	}

	// Delete provider credentials from keyring.
	if err := m.credentialStore.Delete(providerName); err != nil {
		log.Debug("Failed to delete provider keyring entry", logKeyProvider, providerName, "error", err)
		errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, providerName, err))
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

	log.Info("Provider logout completed", logKeyProvider, providerName, "identities", len(identityNames), "errors", len(errs))

	if len(errs) > 0 {
		return errors.Join(append([]error{errUtils.ErrLogoutFailed}, errs...)...)
	}

	return nil
}

// LogoutAll removes all cached credentials for all identities.
func (m *manager) LogoutAll(ctx context.Context) error {
	defer perf.Track(nil, "auth.Manager.LogoutAll")()

	log.Debug("Logout all identities")

	var errs []error

	// Logout each identity.
	for identityName := range m.config.Identities {
		if err := m.Logout(ctx, identityName); err != nil {
			log.Debug("Failed to logout identity", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf("%w for identity %q: %w", errUtils.ErrIdentityLogout, identityName, err))
		}
	}

	log.Info("Logout all completed", "identities", len(m.config.Identities), "errors", len(errs))

	if len(errs) > 0 {
		return errors.Join(append([]error{errUtils.ErrLogoutFailed}, errs...)...)
	}

	return nil
}
