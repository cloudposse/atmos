package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/identities/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// logKeyExpirationChain is the log key for expiration values in chain operations.
const logKeyExpirationChain = "expiration"

// authenticateChain performs credential chain authentication with bottom-up validation.
func (m *manager) authenticateChain(ctx context.Context, _ string) (types.ICredentials, error) {
	// Step 1: Bottom-up validation - check cached credentials from target to root.
	validFromIndex := m.findFirstValidCachedCredentials()

	if validFromIndex != -1 {
		log.Debug("Found valid cached credentials", "validFromIndex", validFromIndex, "chainStep", m.getChainStepName(validFromIndex))
	}

	// Step 2: Selective re-authentication from first invalid point down to target.
	// CRITICAL: Always re-authenticate through the full chain, even if the target identity
	// has cached credentials. This ensures assume-role identities perform the actual
	// AssumeRole API call rather than using potentially incorrect cached credentials
	// (e.g., permission set creds incorrectly cached as assume-role creds).
	return m.authenticateFromIndex(ctx, validFromIndex)
}

// findFirstValidCachedCredentials checks cached credentials from bottom to top of chain.
// Returns the index of the first valid cached credentials, or -1 if none found.
func (m *manager) findFirstValidCachedCredentials() int {
	// Check from target identity (bottom) up to provider (top).
	for i := len(m.chain) - 1; i >= 0; i-- {
		identityName := m.chain[i]
		log.Debug("Checking cached credentials", logKeyChainIndex, i, identityNameKey, identityName)

		// Retrieve credentials with automatic keyring → identity storage fallback.
		cachedCreds, err := m.loadCredentialsWithFallback(context.Background(), identityName)
		if err != nil {
			log.Debug("Failed to retrieve credentials", logKeyChainIndex, i, identityNameKey, identityName, "error", err)
			continue
		}

		// Validate credentials are not expired.
		valid, expTime := m.isCredentialValid(identityName, cachedCreds)
		if valid {
			if expTime != nil {
				log.Debug("Found valid cached credentials", logKeyChainIndex, i, identityNameKey, identityName, logKeyExpirationChain, *expTime)
			} else {
				// Credentials without expiration (API keys, long-lived tokens, etc.).
				log.Debug("Found valid cached credentials", logKeyChainIndex, i, identityNameKey, identityName, logKeyExpirationChain, "none")
			}
			return i
		}

		// Credentials exist but are expired or expiring too soon - log and continue to next in chain.
		if expTime != nil {
			timeUntilExpiry := time.Until(*expTime)
			if timeUntilExpiry <= 0 {
				log.Debug("Skipping expired credentials in chain",
					logKeyChainIndex, i,
					identityNameKey, identityName,
					logKeyExpirationChain, *expTime,
					"expired_ago", -timeUntilExpiry)
			} else {
				log.Debug("Skipping credentials expiring too soon in chain (within safety buffer)",
					logKeyChainIndex, i,
					identityNameKey, identityName,
					logKeyExpirationChain, *expTime,
					"time_until_expiry", timeUntilExpiry,
					"required_buffer", minCredentialValidityBuffer)
			}
		} else {
			// This shouldn't happen - isCredentialValid returns valid=true when expTime=nil.
			log.Debug("Credentials are invalid", logKeyChainIndex, i, identityNameKey, identityName)
		}
	}
	return -1 // No valid cached credentials found
}

// isCredentialValid checks if the cached credentials are valid and not expired.
// Returns whether the credentials are valid and, if AWS expiration is present and valid, the parsed expiration time.
func (m *manager) isCredentialValid(identityName string, cachedCreds types.ICredentials) (bool, *time.Time) {
	// Check expiration from the credentials object itself, not the keyring.
	// This allows us to validate credentials loaded from any source (keyring, files, etc.).
	if expTime, err := cachedCreds.GetExpiration(); err == nil && expTime != nil {
		if expTime.After(time.Now().Add(minCredentialValidityBuffer)) {
			return true, expTime
		}
		// Expiration exists but is too close or already expired -> treat as invalid for long-running operations.
		timeUntilExpiry := time.Until(*expTime)
		if timeUntilExpiry <= 0 {
			log.Debug("Credentials are expired",
				logKeyIdentity, identityName,
				logKeyExpiration, expTime,
				"expired_ago", -timeUntilExpiry)
		} else {
			log.Debug("Credentials expiring too soon for safe use in long-running operations",
				logKeyIdentity, identityName,
				logKeyExpiration, expTime,
				"time_until_expiry", timeUntilExpiry,
				"required_buffer", minCredentialValidityBuffer,
				"recommendation", "re-authenticate to ensure operation completes successfully")
		}
		return false, expTime
	}
	// Non-expiring credentials (no expiration info) -> assume valid.
	return true, nil
}

// authenticateFromIndex performs authentication starting from the given index in the chain.
func (m *manager) authenticateFromIndex(ctx context.Context, startIndex int) (types.ICredentials, error) {
	// TODO: Ideally this wouldn't be here, and would be handled by an identity interface function.
	// Handle special case: standalone AWS user identity.
	if aws.IsStandaloneAWSUserChain(m.chain, m.config.Identities) {
		return aws.AuthenticateStandaloneAWSUser(ctx, m.chain[0], m.identities)
	}

	// Handle regular provider-based authentication chains.
	return m.authenticateProviderChain(ctx, startIndex)
}

// authenticateProviderChain handles authentication for provider-based identity chains.
func (m *manager) authenticateProviderChain(ctx context.Context, startIndex int) (types.ICredentials, error) {
	var currentCreds types.ICredentials
	var err error

	// Determine actual starting point for authentication.
	// When startIndex is -1 (no valid cached credentials), this returns 0 (start from provider).
	// When startIndex is >= 0, this returns the same index (valid cached credentials exist).
	actualStartIndex := m.determineStartingIndex(startIndex)

	// Retrieve cached credentials if starting from a cached point.
	// Important: Only fetch cached credentials if we had valid ones (startIndex >= 0).
	// If startIndex was -1 (no valid cached creds), actualStartIndex becomes 0 but we should NOT
	// fetch cached credentials - we should start fresh from provider authentication.
	if startIndex >= 0 && actualStartIndex >= 0 {
		currentCreds, actualStartIndex = m.fetchCachedCredentials(actualStartIndex)
	}

	// Step 1: Authenticate with provider if needed.
	// Only authenticate provider if we don't have cached provider credentials.
	if actualStartIndex == 0 { //nolint:nestif
		// Allow provider to inspect the chain and prepare pre-auth preferences.
		if provider, exists := m.providers[m.chain[0]]; exists {
			if err := provider.PreAuthenticate(m); err != nil {
				errUtils.CheckErrorAndPrint(err, "Pre Authenticate", "")
				return nil, fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, m.chain[0], err)
			}
		}
		currentCreds, err = m.authenticateWithProvider(ctx, m.chain[0])
		if err != nil {
			return nil, err
		}
		actualStartIndex = 1
	}

	// Step 2: Authenticate through identity chain.
	return m.authenticateIdentityChain(ctx, actualStartIndex, currentCreds)
}

func (m *manager) fetchCachedCredentials(startIndex int) (types.ICredentials, int) {
	// Guard against nil credential store (can happen in unit tests).
	if m.credentialStore == nil {
		log.Debug("No credential store available, starting from provider")
		return nil, 0
	}

	currentCreds, err := m.getChainCredentials(m.chain, startIndex)
	if err != nil {
		log.Debug("Failed to retrieve cached credentials, starting from provider", "error", err)
		return nil, 0
	}
	// Return cached credentials as OUTPUT of step at startIndex, so authentication
	// continues from the NEXT step (startIndex + 1). The cached credentials become
	// the input to the next identity in the chain.
	// Example: If permission set creds are cached at index 1, return them with startIndex=2
	// so the assume-role identity at index 2 uses the permission set creds as input.
	return currentCreds, startIndex + 1
}

// determineStartingIndex determines where to start authentication based on cached credentials.
func (m *manager) determineStartingIndex(startIndex int) int {
	if startIndex == -1 {
		return 0 // Start from provider if no valid cached credentials
	}
	return startIndex
}

// loadCredentialsWithFallback retrieves credentials with keyring → identity storage fallback.
// This is the single source of truth for credential retrieval across all auth operations.
// It ensures consistent behavior whether credentials are in keyring or identity storage.
//
// For AWS credentials, session tokens (with expiration) are stored in files but NOT in keyring.
// Keyring stores long-lived IAM credentials for re-authentication. This method prefers
// session credentials from files when they exist and are not expired, so users can see
// accurate expiration times in whoami output.
func (m *manager) loadCredentialsWithFallback(ctx context.Context, identityName string) (types.ICredentials, error) {
	// Fast path: Try keyring cache first.
	keyringCreds, keyringErr := m.credentialStore.Retrieve(identityName)
	if keyringErr == nil {
		log.Debug("Retrieved credentials from keyring", logKeyIdentity, identityName)

		// For AWS credentials without session token, also check files for session credentials.
		// Session tokens have expiration info that users want to see in whoami.
		if awsCreds, ok := keyringCreds.(*types.AWSCredentials); ok && awsCreds.SessionToken == "" {
			fileCreds := m.loadSessionCredsFromFiles(ctx, identityName)
			if fileCreds != nil {
				log.Debug("Using session credentials from files (have expiration)", logKeyIdentity, identityName)
				return fileCreds, nil
			}
		}
		return keyringCreds, nil
	}

	// If keyring returned an error other than "not found", propagate it.
	if !errors.Is(keyringErr, credentials.ErrCredentialsNotFound) {
		return nil, fmt.Errorf("keyring error for identity %q: %w", identityName, keyringErr)
	}

	// Slow path: Fall back to identity storage (AWS files, etc.).
	// This handles cases where credentials were created outside of Atmos
	// (e.g., via AWS CLI/SSO) but are available in standard credential files.
	log.Debug("Credentials not in keyring, trying identity storage", logKeyIdentity, identityName)

	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrIdentityNotInConfig, identityName)
	}

	// Ensure the identity has access to manager for resolving provider information.
	// This builds the authentication chain and sets manager reference so the identity
	// can resolve the root provider for file-based credentials.
	// This is best-effort - if it fails, LoadCredentials will fail with a clear error.
	_ = m.ensureIdentityHasManager(identityName)

	// Delegate to identity's LoadCredentials method.
	// Each identity type knows how to load its own credentials from storage.
	loadedCreds, loadErr := identity.LoadCredentials(ctx)
	if loadErr != nil {
		return nil, fmt.Errorf("failed to load credentials from identity storage for %q: %w", identityName, loadErr)
	}

	if loadedCreds == nil {
		return nil, fmt.Errorf("%w: credentials loaded from storage are nil for identity %q", errUtils.ErrNoCredentialsFound, identityName)
	}

	log.Debug("Successfully loaded credentials from identity storage", logKeyIdentity, identityName)
	return loadedCreds, nil
}

// loadSessionCredsFromFiles attempts to load session credentials from identity storage files.
// Returns nil if loading fails or credentials are expired.
func (m *manager) loadSessionCredsFromFiles(ctx context.Context, identityName string) types.ICredentials {
	identity, exists := m.identities[identityName]
	if !exists {
		return nil
	}

	_ = m.ensureIdentityHasManager(identityName)

	fileCreds, err := identity.LoadCredentials(ctx)
	if err != nil {
		log.Debug("Could not load credentials from files", logKeyIdentity, identityName, "error", err)
		return nil
	}

	if fileCreds == nil || fileCreds.IsExpired() {
		return nil
	}

	// Only return if these are session credentials (have session token).
	if awsCreds, ok := fileCreds.(*types.AWSCredentials); ok && awsCreds.SessionToken != "" {
		return fileCreds
	}

	return nil
}

// getChainCredentials retrieves cached credentials from the specified starting point.
func (m *manager) getChainCredentials(chain []string, startIndex int) (types.ICredentials, error) {
	identityName := chain[startIndex]
	creds, err := m.loadCredentialsWithFallback(context.Background(), identityName)
	if err != nil {
		return nil, err
	}

	log.Debug("Starting authentication from cached credentials", "startIndex", startIndex, logKeyIdentity, identityName)
	return creds, nil
}

// authenticateWithProvider handles provider authentication.
func (m *manager) authenticateWithProvider(ctx context.Context, providerName string) (types.ICredentials, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		wrappedErr := fmt.Errorf("provider %q not registered: %w", providerName, errUtils.ErrInvalidAuthConfig)
		errUtils.CheckErrorAndPrint(wrappedErr, "Authenticate with Provider", "")
		return nil, wrappedErr
	}

	log.Debug("Authenticating with provider", "provider", providerName)
	credentials, err := provider.Authenticate(ctx)
	if err != nil {
		errUtils.CheckErrorAndPrint(err, "Authenticate with Provider", "")
		return nil, fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, providerName, err)
	}

	// Cache provider credentials, but skip session tokens.
	// Session tokens are temporary and should not overwrite long-lived credentials.
	if isSessionToken(credentials) {
		log.Debug("Skipping keyring cache for session token provider credentials", logKeyProvider, providerName)
	} else {
		if err := m.credentialStore.Store(providerName, credentials); err != nil {
			log.Debug("Failed to cache provider credentials", "error", err)
		} else {
			log.Debug("Cached provider credentials", "providerName", providerName)
		}
	}

	// Run provisioning if provider supports it (non-fatal).
	m.provisionIdentities(ctx, providerName, provider, credentials)

	log.Debug("Provider authenticated", "provider", providerName)
	return credentials, nil
}

// provisionIdentities runs identity provisioning if the provider supports it.
// This is a non-fatal operation - failures are logged but don't block authentication.
func (m *manager) provisionIdentities(ctx context.Context, providerName string, provider types.Provider, credentials types.ICredentials) {
	defer perf.Track(nil, "auth.Manager.provisionIdentities")()

	// Set up auth-specific logging and restore on exit.
	defer m.setupAuthLogging()()

	// Check if provider implements the Provisioner interface.
	provisioner, ok := provider.(types.Provisioner)
	if !ok {
		log.Debug("Provider does not support provisioning, skipping", logKeyProvider, providerName)
		return
	}

	// Run provisioning.
	log.Debug("Running identity provisioning", logKeyProvider, providerName)
	result, err := provisioner.ProvisionIdentities(ctx, credentials)
	if err != nil {
		log.Warn("Failed to provision identities, skipping", logKeyProvider, providerName, "error", err)
		return
	}

	// Skip if no identities provisioned.
	if result == nil || len(result.Identities) == 0 {
		log.Debug("No identities provisioned", logKeyProvider, providerName)
		return
	}

	// Guard against nil Counts to prevent panic from incomplete ProvisioningResult implementations.
	accounts := 0
	roles := 0
	if result.Metadata.Counts != nil {
		accounts = result.Metadata.Counts.Accounts
		roles = result.Metadata.Counts.Roles
	}

	log.Debug("Provisioned identities from provider",
		logKeyProvider, providerName,
		"accounts", accounts,
		"roles", roles,
		"identities", len(result.Identities))

	// Write provisioned identities to cache.
	if err := m.writeProvisionedIdentities(result); err != nil {
		log.Warn("Failed to write provisioned identities, skipping", logKeyProvider, providerName, "error", err)
		return
	}

	log.Debug("Successfully provisioned and cached identities", logKeyProvider, providerName, "count", len(result.Identities))
}

// setupAuthLogging configures auth-specific logging (prefix and level).
// Returns a cleanup function that restores the original settings.
func (m *manager) setupAuthLogging() func() {
	// Save current state.
	currentLevel := log.GetLevel()

	// Set auth prefix.
	log.SetPrefix("atmos-auth")

	// Set auth log level from config if specified.
	if m.config != nil && m.config.Logs.Level != "" {
		if authLogLevel, err := log.ParseLogLevel(m.config.Logs.Level); err == nil {
			// Convert Atmos LogLevel string to charm.Level.
			switch authLogLevel {
			case log.LogLevelTrace:
				log.SetLevel(log.TraceLevel)
			case log.LogLevelDebug:
				log.SetLevel(log.DebugLevel)
			case log.LogLevelInfo:
				log.SetLevel(log.InfoLevel)
			case log.LogLevelWarning:
				log.SetLevel(log.WarnLevel)
			case log.LogLevelError:
				log.SetLevel(log.ErrorLevel)
			case log.LogLevelOff:
				log.SetLevel(log.FatalLevel)
			}
		}
	}

	// Return cleanup function.
	return func() {
		log.SetLevel(currentLevel)
		log.SetPrefix("")
	}
}

// writeProvisionedIdentities writes provisioned identities to the cache directory.
func (m *manager) writeProvisionedIdentities(result *types.ProvisioningResult) error {
	defer perf.Track(nil, "auth.Manager.writeProvisionedIdentities")()

	writer, err := types.NewProvisioningWriter()
	if err != nil {
		return fmt.Errorf("failed to create provisioning writer: %w", err)
	}

	filePath, err := writer.Write(result)
	if err != nil {
		return fmt.Errorf("failed to write provisioned identities: %w", err)
	}

	log.Debug("Wrote provisioned identities to cache", "path", filePath)
	return nil
}

// Helper functions for logging.
func (m *manager) getChainStepName(index int) string {
	if index < len(m.chain) {
		return m.chain[index]
	}
	return "unknown"
}

// isSessionToken checks if credentials are temporary session tokens.
// Session tokens are identified by the presence of a SessionToken field.
// These should not be cached in keyring as they overwrite long-lived credentials.
func isSessionToken(creds types.ICredentials) bool {
	if awsCreds, ok := creds.(*types.AWSCredentials); ok {
		return awsCreds.SessionToken != ""
	}
	// Add other credential types as needed.
	return false
}

// authenticateIdentityChain performs sequential authentication through an identity chain.
func (m *manager) authenticateIdentityChain(ctx context.Context, startIndex int, initialCreds types.ICredentials) (types.ICredentials, error) {
	log.Debug("Authenticating identity chain", "chainLength", len(m.chain), "startIndex", startIndex, "chain", m.chain)

	currentCreds := initialCreds

	// Step 2: Authenticate through identity chain starting from startIndex.
	for i := startIndex; i < len(m.chain); i++ {
		identityStep := m.chain[i]
		identity, exists := m.identities[identityStep]
		if !exists {
			wrappedErr := fmt.Errorf("%w: identity %q not found in chain step %d", errUtils.ErrInvalidAuthConfig, identityStep, i)
			errUtils.CheckErrorAndPrint(wrappedErr, "Authenticate Identity Chain", "")
			return nil, wrappedErr
		}

		log.Debug("Authenticating identity step", "step", i, logKeyIdentity, identityStep, "kind", identity.Kind())

		// Each identity receives credentials from the previous step.
		nextCreds, err := identity.Authenticate(ctx, currentCreds)
		if err != nil {
			return nil, fmt.Errorf("%w: identity=%s step=%d: %w", errUtils.ErrAuthenticationFailed, identityStep, i, err)
		}

		currentCreds = nextCreds

		// Cache credentials for this level, but skip session tokens.
		// Session tokens are already persisted to provider-specific storage (e.g., AWS files)
		// and can be loaded via identity.LoadCredentials().
		// Caching session tokens in keyring would overwrite long-lived credentials
		// that are needed for subsequent authentication attempts.
		if isSessionToken(currentCreds) {
			log.Debug("Skipping keyring cache for session tokens", "identityStep", identityStep)
		} else {
			if err := m.credentialStore.Store(identityStep, currentCreds); err != nil {
				log.Debug("Failed to cache credentials", "identityStep", identityStep, "error", err)
			} else {
				log.Debug("Cached credentials", "identityStep", identityStep)
			}
		}

		log.Debug("Chained identity", "from", m.getChainStepName(i-1), "to", identityStep)
	}

	return currentCreds, nil
}

// buildAuthenticationChain builds the authentication chain from target identity to source provider.
// Returns a slice where [0] is the provider name, [1..n] are identity names in authentication order.
func (m *manager) buildAuthenticationChain(identityName string) ([]string, error) {
	var chain []string
	visited := make(map[string]bool)

	// Recursively build the chain.
	err := m.buildChainRecursive(identityName, &chain, visited)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to build authentication chain for identity %q: %w", identityName, err)
		errUtils.CheckErrorAndPrint(wrappedErr, buildAuthenticationChain, "")
		return nil, wrappedErr
	}

	// Reverse the chain so provider is first, then identities in authentication order.
	for i := 0; i < len(chain)/2; i++ {
		j := len(chain) - 1 - i
		chain[i], chain[j] = chain[j], chain[i]
	}

	return chain, nil
}

// buildChainRecursive recursively builds the authentication chain.
func (m *manager) buildChainRecursive(identityName string, chain *[]string, visited map[string]bool) error {
	// Check for circular dependencies.
	if visited[identityName] {
		errUtils.CheckErrorAndPrint(errUtils.ErrCircularDependency, buildChainRecursive, fmt.Sprintf("circular dependency detected in identity chain involving %q", identityName))
		return fmt.Errorf("%w: circular dependency detected in identity chain involving %q", errUtils.ErrCircularDependency, identityName)
	}
	visited[identityName] = true

	// Find the identity.
	identity, exists := m.config.Identities[identityName]

	if !exists {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, buildChainRecursive, fmt.Sprintf("identity %q not found", identityName))
		return fmt.Errorf("%w: identity %q not found", errUtils.ErrInvalidAuthConfig, identityName)
	}

	// AWS User identities don't require via configuration - they are standalone.
	if identity.Via == nil {
		if identity.Kind == "aws/user" {
			// AWS User is standalone - just add it to the chain and return.
			*chain = append(*chain, identityName)
			return nil
		}
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidIdentityConfig, buildChainRecursive, fmt.Sprintf("identity %q has no via configuration", identityName))
		return fmt.Errorf("%w: identity %q has no via configuration", errUtils.ErrInvalidIdentityConfig, identityName)
	}

	// Add current identity to chain.
	*chain = append(*chain, identityName)

	// If this identity points to a provider, add it and stop.
	if identity.Via.Provider != "" {
		*chain = append(*chain, identity.Via.Provider)
		return nil
	}

	// If this identity points to another identity, recurse.
	if identity.Via.Identity != "" {
		return m.buildChainRecursive(identity.Via.Identity, chain, visited)
	}

	errUtils.CheckErrorAndPrint(errUtils.ErrInvalidIdentityConfig, buildChainRecursive, fmt.Sprintf("identity %q has invalid via configuration", identityName))
	return fmt.Errorf("%w: identity %q has invalid via configuration", errUtils.ErrInvalidIdentityConfig, identityName)
}
