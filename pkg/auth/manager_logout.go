package auth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Logout removes credentials for the specified identity and its authentication chain.
func (m *manager) Logout(ctx context.Context, identityName string) error {
	defer perf.Track(nil, "auth.Manager.Logout")()

	// Validate identity exists in configuration.
	if _, exists := m.identities[identityName]; !exists {
		return fmt.Errorf("%w: identity %q", errUtils.ErrIdentityNotInConfig, identityName)
	}

	// Build authentication chain to determine all credentials to remove.
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		log.Debug("Failed to build authentication chain for logout", logKeyIdentity, identityName, "error", err)
		// Continue with best-effort cleanup of just the identity.
		chain = []string{identityName}
	}

	log.Debug("Logout authentication chain", logKeyIdentity, identityName, "chain", chain)

	var errs []error
	removedCount := 0

	// Step 1: Delete keyring entries for each step in the chain.
	for _, alias := range chain {
		if err := m.credentialStore.Delete(alias); err != nil {
			log.Debug("Failed to delete keyring entry (may not exist)", "alias", alias, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, alias, err))
		} else {
			log.Debug("Deleted keyring entry", "alias", alias)
			removedCount++
		}
	}

	// Step 2: Call provider-specific cleanup (files, etc.) unless skipped.
	skipProviderLogout, _ := ctx.Value(skipProviderLogoutKey).(bool)
	if len(chain) > 0 && !skipProviderLogout {
		providerName := chain[0]
		m.attemptProviderLogout(ctx, providerName, &errs)
	}

	// Step 3: Call identity-specific cleanup.
	m.attemptIdentityLogout(ctx, identityName, &errs)

	log.Info("Logout completed", logKeyIdentity, identityName, "removed", removedCount, "errors", len(errs))

	// Return success if at least one credential was removed, even if there were errors.
	if removedCount > 0 && len(errs) > 0 {
		return errors.Join(append([]error{errUtils.ErrPartialLogout}, errs...)...)
	}
	if len(errs) > 0 {
		return errors.Join(append([]error{errUtils.ErrLogoutFailed}, errs...)...)
	}

	return nil
}

// attemptProviderLogout calls provider-specific cleanup for the given provider.
func (m *manager) attemptProviderLogout(ctx context.Context, providerName string, errs *[]error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return
	}

	err := provider.Logout(ctx)
	if err == nil {
		log.Debug("Provider logout succeeded", logKeyProvider, providerName)
		return
	}

	// ErrLogoutNotSupported is a successful no-op (exit 0).
	if errors.Is(err, errUtils.ErrLogoutNotSupported) {
		log.Debug("Provider logout not supported (no-op)", logKeyProvider, providerName)
		return
	}

	log.Debug("Provider logout failed", logKeyProvider, providerName, "error", err)
	*errs = append(*errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrProviderLogout, providerName, err))
}

// attemptIdentityLogout calls identity-specific cleanup for the given identity.
func (m *manager) attemptIdentityLogout(ctx context.Context, identityName string, errs *[]error) {
	identity, exists := m.identities[identityName]
	if !exists {
		return
	}

	err := identity.Logout(ctx)
	if err == nil {
		log.Debug("Identity logout succeeded", logKeyIdentity, identityName)
		return
	}

	// ErrLogoutNotSupported is a successful no-op (exit 0).
	if errors.Is(err, errUtils.ErrLogoutNotSupported) {
		log.Debug("Identity logout not supported (no-op)", logKeyIdentity, identityName)
		return
	}

	log.Debug("Identity logout failed", logKeyIdentity, identityName, "error", err)
	*errs = append(*errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrIdentityLogout, identityName, err))
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

// LogoutProvider removes all credentials for the specified provider.
func (m *manager) LogoutProvider(ctx context.Context, providerName string) error {
	defer perf.Track(nil, "auth.Manager.LogoutProvider")()

	// Validate provider exists in configuration.
	if _, exists := m.providers[providerName]; !exists {
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

	// Set context flag to skip provider.Logout during per-identity cleanup.
	ctxWithSkip := context.WithValue(ctx, skipProviderLogoutKey, true)

	// Logout each identity (skipping provider-specific cleanup).
	for _, identityName := range identityNames {
		if err := m.Logout(ctxWithSkip, identityName); err != nil {
			log.Debug("Failed to logout identity", logKeyIdentity, identityName, "error", err)
			errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrIdentityLogout, identityName, err))
		}
	}

	// Delete provider credentials from keyring.
	if err := m.credentialStore.Delete(providerName); err != nil {
		log.Debug("Failed to delete provider keyring entry", logKeyProvider, providerName, "error", err)
		errs = append(errs, fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, providerName, err))
	}

	// Call provider-specific cleanup once for the entire provider.
	m.attemptProviderLogout(ctx, providerName, &errs)

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
