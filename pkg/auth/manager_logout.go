package auth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
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
		return fmt.Errorf("%w: identity %q", errUtils.ErrIdentityNotInConfig, identityName)
	}

	// Keyring entries for ambient chains (gcp/adc, azure/cli, the OIDC providers) hold
	// nothing durable — the credentials are re-derived from the environment on every login —
	// so there is nothing to preserve and a leftover entry can only be stale data written by
	// an older Atmos version. Delete it unconditionally rather than making users discover
	// `--keychain`.
	//
	// The two reasons are tracked separately: because ambient chains never write to the
	// keyring, the normal state at logout is that no entry exists. A forced delete that
	// misses is therefore expected, and must not be reported as a partial logout the way
	// an explicitly requested `--keychain` delete would be.
	ambientForced := m.identityChainRootIsAmbient(identityName)
	bestEffort := ambientForced && !deleteKeychain
	deleteKeychain = deleteKeychain || ambientForced

	log.Debug("Logout identity", logKeyIdentity, identityName, "deleteKeychain", deleteKeychain, "bestEffort", bestEffort)

	// Step 1: Delete keyring entry ONLY if deleteKeychain flag is set.
	errs := m.deleteIdentityKeyringEntries(identityName, deleteKeychain, bestEffort)

	// Step 2: Clean up linked integrations (non-fatal).
	m.cleanupIntegrations(ctx, identityName)

	// Step 3: Call identity-specific cleanup (each identity type handles its own file cleanup).
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

// deleteIdentityKeyringEntries removes the identity's realm-scoped keyring entry and, for
// realm-scoped setups, the legacy pre-realm entry left by older Atmos versions.
// Returns any deletion errors for the caller to accumulate; a missing legacy entry is the
// expected case and is not an error.
//
// When bestEffort is set the realm-scoped miss is also expected — the caller forced the
// deletion for an ambient chain that never writes to the keyring — so it is logged rather
// than returned. An explicitly requested `--keychain` deletion still reports failures.
func (m *manager) deleteIdentityKeyringEntries(identityName string, deleteKeychain, bestEffort bool) []error {
	if !deleteKeychain {
		log.Debug("Skipping keyring deletion (preserving credentials)", logKeyIdentity, identityName)
		return nil
	}

	var errs []error

	// Delete realm-scoped entry (current format).
	if err := m.credentialStore.Delete(identityName, m.realm.Value); err != nil {
		if bestEffort {
			log.Debug("No ambient keyring entry to delete", logKeyIdentity, identityName, logKeyRealm, m.realm.Value)
		} else {
			log.Debug("Failed to delete keyring entry (may not exist)", logKeyIdentity, identityName, logKeyRealm, m.realm.Value, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, identityName, err))
		}
	} else {
		log.Debug("Deleted keyring entry", logKeyIdentity, identityName, logKeyRealm, m.realm.Value)
	}

	// Also delete legacy entry (pre-realm format) for backward compatibility cleanup.
	if m.realm.Value != "" {
		if err := m.credentialStore.Delete(identityName, ""); err != nil {
			log.Debug("No legacy keyring entry to delete", logKeyIdentity, identityName)
		} else {
			log.Debug("Deleted legacy keyring entry (pre-realm)", logKeyIdentity, identityName)
		}
	}

	return errs
}

// cleanupIntegrations runs Cleanup() on all integrations linked to the identity.
// Failures are non-fatal and logged as warnings to avoid blocking logout.
func (m *manager) cleanupIntegrations(ctx context.Context, identityName string) {
	defer perf.Track(nil, "auth.Manager.cleanupIntegrations")()

	// Find all integrations that reference this identity (not just auto_provision ones).
	linkedIntegrations := m.findIntegrationsForIdentity(identityName, false)
	if len(linkedIntegrations) == 0 {
		return
	}

	log.Debug("Cleaning up linked integrations", logKeyIdentity, identityName, "count", len(linkedIntegrations))

	for _, integrationName := range linkedIntegrations {
		integrationConfig, exists := m.config.Integrations[integrationName]
		if !exists {
			continue
		}

		integration, err := integrations.Create(&integrations.IntegrationConfig{
			Name:   integrationName,
			Config: &integrationConfig,
			Realm:  m.realm.Value,
		})
		if err != nil {
			log.Warn("Failed to create integration for cleanup", "integration", integrationName, "error", err)
			continue
		}

		if err := integration.Cleanup(ctx); err != nil {
			log.Warn("Integration cleanup failed", "integration", integrationName, "error", err)
		} else {
			log.Debug("Integration cleanup succeeded", "integration", integrationName)
		}
	}
}

// resolveProviderForIdentity follows the Via chain to find the root provider for an identity.
// Returns empty string if no provider is found or if a cycle is detected.
func (m *manager) resolveProviderForIdentity(identityName string) string {
	// The manager may be constructed without config in narrow code paths (and in unit
	// tests that exercise a single method), so treat a missing config as "unresolvable"
	// rather than dereferencing it.
	if m.config == nil {
		return ""
	}

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

// IsAmbientProvider satisfies the optional AmbientProviderReporter interface so callers
// that preview logout (`atmos auth logout --dry-run`) can report the keyring entries that
// will actually be removed. Without it the preview would omit them, because the forcing
// decision lives inside Logout and the caller only sees the user's --keychain flag.
//
// This is deliberately NOT part of types.AuthManager. That interface is already wide and
// has a generated mock plus half a dozen hand-written doubles across the repo; growing it
// for a display concern would churn all of them. Optional interfaces are the established
// pattern here — see types.AmbientProvider, types.StandaloneIdentity, types.Provisioner.
func (m *manager) IsAmbientProvider(providerName string) bool {
	return m.providerIsAmbient(providerName)
}

// providerIsAmbient reports whether the named provider resolves credentials from ambient
// environment state on every authentication (e.g. gcp/adc). Such providers never own
// durable credentials, so their keyring entries are always safe to delete.
func (m *manager) providerIsAmbient(providerName string) bool {
	provider, exists := m.providers[providerName]
	if !exists {
		return false
	}
	return types.ProviderIsAmbient(provider)
}

// identityChainRootIsAmbient reports whether the identity's chain is rooted at an ambient
// provider. Credentials derived from an ambient provider inherit its snapshot of the
// environment's principal, so they are as non-durable as the provider's own token.
func (m *manager) identityChainRootIsAmbient(identityName string) bool {
	providerName := m.resolveProviderForIdentity(identityName)
	if providerName == "" {
		return false
	}
	return m.providerIsAmbient(providerName)
}

// deleteProviderKeyringEntries removes a provider's realm-scoped keyring entry and the
// legacy pre-realm one, shared by LogoutProvider and LogoutAll.
//
// bestEffort has the same meaning as in deleteIdentityKeyringEntries: the deletion was
// forced for an ambient provider that never writes to the keyring, so a miss is the
// expected steady state and is logged rather than returned as a failure.
func (m *manager) deleteProviderKeyringEntries(providerName string, deleteKeychain, bestEffort bool) []error {
	if !deleteKeychain {
		log.Debug("Skipping provider keyring deletion (preserving credentials)", logKeyProvider, providerName)
		return nil
	}

	var errs []error

	// Delete realm-scoped entry (current format).
	if err := m.credentialStore.Delete(providerName, m.realm.Value); err != nil {
		if bestEffort {
			log.Debug("No ambient provider keyring entry to delete", logKeyProvider, providerName, logKeyRealm, m.realm.Value)
		} else {
			log.Debug("Failed to delete provider keyring entry", logKeyProvider, providerName, logKeyRealm, m.realm.Value, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, providerName, err))
		}
	}

	// Also delete legacy entry (pre-realm format) for backward compatibility cleanup.
	if m.realm.Value != "" {
		if err := m.credentialStore.Delete(providerName, ""); err != nil {
			log.Debug("No legacy provider keyring entry to delete", logKeyProvider, providerName)
		} else {
			log.Debug("Deleted legacy provider keyring entry (pre-realm)", logKeyProvider, providerName)
		}
	}

	return errs
}

// LogoutProvider removes all credentials for the specified provider and all identities that use it.
// If deleteKeychain is true, also removes credentials from system keychain.
func (m *manager) LogoutProvider(ctx context.Context, providerName string, deleteKeychain bool) error { //nolint:revive
	defer perf.Track(nil, "auth.Manager.LogoutProvider")()

	// Validate provider exists in configuration.
	provider, exists := m.providers[providerName]
	if !exists {
		return fmt.Errorf("%w: provider %q", errUtils.ErrProviderNotInConfig, providerName)
	}

	// Ambient providers hold nothing durable in the keyring — see Logout for the rationale,
	// including why a forced deletion that finds nothing is not a failure.
	ambientForced := m.providerIsAmbient(providerName)
	// Preserved because the per-identity Logout below must see what the *user* asked for.
	// Passing the already-forced value would make Logout treat an ambient-forced delete as
	// explicitly requested, and report an expected keyring miss as a partial logout.
	userRequestedKeychain := deleteKeychain
	bestEffort := ambientForced && !deleteKeychain
	deleteKeychain = deleteKeychain || ambientForced

	log.Debug("Logout provider", logKeyProvider, providerName, "deleteKeychain", deleteKeychain, "bestEffort", bestEffort)

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

	// Logout each identity, passing the user's original request so each one re-derives its
	// own ambient forcing (an identity's chain root is this provider, so it reaches the
	// same conclusion — but with the correct best-effort semantics).
	for _, identityName := range identityNames {
		if err := m.Logout(ctx, identityName, userRequestedKeychain); err != nil {
			log.Debug("Failed to logout identity", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrIdentityLogout, identityName, err))
		}
	}

	// Delete provider credentials from keyring ONLY if deleteKeychain flag is set.
	errs = append(errs, m.deleteProviderKeyringEntries(providerName, deleteKeychain, bestEffort)...)

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
		// Delete provider credentials from keyring if the deleteKeychain flag is set, or
		// unconditionally for ambient providers — see Logout for the rationale.
		ambientForced := types.ProviderIsAmbient(provider)
		errs = append(errs, m.deleteProviderKeyringEntries(
			providerName, deleteKeychain || ambientForced, ambientForced && !deleteKeychain)...)

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
