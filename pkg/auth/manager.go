package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/factory"
	"github.com/cloudposse/atmos/pkg/auth/identities/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	logKeyIdentity           = "identity"
	logKeyProvider           = "provider"
	logKeyChainIndex         = "chainIndex"
	identityNameKey          = "identityName"
	buildAuthenticationChain = "buildAuthenticationChain"
	buildChainRecursive      = "buildChainRecursive"
	backtickedFmt            = "`%s`"
	errFormatWithString      = "%w: %s"
	errFormatWrapTwo         = "%w for %q: %w"
	logKeyExpiration         = "expiration"
)

const (
	// MinCredentialValidityBuffer is the minimum duration credentials must be valid.
	// AWS can invalidate credentials before their stated expiration time.
	minCredentialValidityBuffer = 15 * time.Minute
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// SkipProviderLogoutKey is the context key for skipping provider.Logout calls.
	skipProviderLogoutKey contextKey = "skipProviderLogout"
)

// isInteractive checks if we're running in an interactive terminal (has stdin TTY).
// This is used to determine if we can prompt the user for input.
func isInteractive() bool {
	return term.IsTTYSupportForStdin()
}

// manager implements the AuthManager interface.
type manager struct {
	config          *schema.AuthConfig
	providers       map[string]types.Provider
	identities      map[string]types.Identity
	credentialStore types.CredentialStore
	validator       types.Validator
	stackInfo       *schema.ConfigAndStacksInfo
	// chain holds the most recently constructed authentication chain.
	// where index 0 is the provider name, followed by identities in order.
	chain []string
}

// NewAuthManager creates a new AuthManager instance.
func NewAuthManager(
	config *schema.AuthConfig,
	credentialStore types.CredentialStore,
	validator types.Validator,
	stackInfo *schema.ConfigAndStacksInfo,
) (types.AuthManager, error) {
	if config == nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrNilParam, "Config", "auth config cannot be nil")
		return nil, errUtils.ErrNilParam
	}
	if credentialStore == nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrNilParam, "Credential Store", "credential store cannot be nil")
		return nil, errUtils.ErrNilParam
	}
	if validator == nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrNilParam, "Validator", "validator cannot be nil")
		return nil, errUtils.ErrNilParam
	}

	m := &manager{
		config:          config,
		providers:       make(map[string]types.Provider),
		identities:      make(map[string]types.Identity),
		credentialStore: credentialStore,
		validator:       validator,
		stackInfo:       stackInfo,
	}

	// Initialize providers.
	if err := m.initializeProviders(); err != nil {
		wrappedErr := fmt.Errorf("failed to initialize providers: %w", err)
		errUtils.CheckErrorAndPrint(wrappedErr, "Initialize Providers", "")
		return nil, wrappedErr
	}

	// Initialize identities.
	if err := m.initializeIdentities(); err != nil {
		wrappedErr := fmt.Errorf("failed to initialize identities: %w", err)
		errUtils.CheckErrorAndPrint(wrappedErr, "Initialize Identities", "")
		return nil, wrappedErr
	}

	return m, nil
}

// GetStackInfo returns the associated stack info pointer (may be nil).
func (m *manager) GetStackInfo() *schema.ConfigAndStacksInfo {
	defer perf.Track(nil, "auth.GetStackInfo")()

	return m.stackInfo
}

// Authenticate performs hierarchical authentication for the specified identity.
func (m *manager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.Manager.Authenticate")()

	// We expect the identity name to be provided by the caller.
	if identityName == "" {
		errUtils.CheckErrorAndPrint(errUtils.ErrNilParam, identityNameKey, "no identity specified")
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrNilParam, identityNameKey)
	}
	if _, exists := m.identities[identityName]; !exists {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, identityNameKey, "Identity specified was not found in the auth config.")
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}

	// Build the complete authentication chain.
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to build authentication chain for identity %q: %w", identityName, err)
		errUtils.CheckErrorAndPrint(wrappedErr, buildAuthenticationChain, "")
		return nil, wrappedErr
	}
	// Persist the chain for later retrieval by providers or callers.
	m.chain = chain
	log.Debug("Authentication chain discovered", logKeyIdentity, identityName, "chainLength", len(chain), "chain", chain)

	// Perform hierarchical credential validation (bottom-up).
	finalCreds, err := m.authenticateChain(ctx, identityName)
	if err != nil {
		wrappedErr := fmt.Errorf("%w: failed to authenticate hierarchically for identity %q: %w", errUtils.ErrAuthenticationFailed, identityName, err)
		errUtils.CheckErrorAndPrint(wrappedErr, "Authenticate Hierarchical", "")
		return nil, wrappedErr
	}

	// Call post-authentication hook on the identity (now part of Identity interface).
	if identity, exists := m.identities[identityName]; exists {
		providerName, perr := identity.GetProviderName()
		if perr != nil {
			wrappedErr := fmt.Errorf("%w: failed to get provider name: %w", errUtils.ErrInvalidAuthConfig, perr)
			errUtils.CheckErrorAndPrint(wrappedErr, "Get Provider Name", "")
			return nil, wrappedErr
		}

		// Get or create auth context in stackInfo.
		var authContext *schema.AuthContext
		if m.stackInfo != nil {
			if m.stackInfo.AuthContext == nil {
				m.stackInfo.AuthContext = &schema.AuthContext{}
			}
			authContext = m.stackInfo.AuthContext
		}

		if err := identity.PostAuthenticate(ctx, &types.PostAuthenticateParams{
			AuthContext:  authContext,
			StackInfo:    m.stackInfo,
			ProviderName: providerName,
			IdentityName: identityName,
			Credentials:  finalCreds,
		}); err != nil {
			wrappedErr := fmt.Errorf("%w: post-authentication failed: %w", errUtils.ErrAuthenticationFailed, err)
			errUtils.CheckErrorAndPrint(wrappedErr, "Post Authenticate", "")
			return nil, wrappedErr
		}
	}

	return m.buildWhoamiInfo(identityName, finalCreds), nil
}

// GetChain returns the most recently built authentication chain.
// The chain is in the format: [providerName, identity1, identity2, ..., targetIdentity].
func (m *manager) GetChain() []string {
	return m.chain
}

// GetCachedCredentials retrieves valid cached credentials without triggering authentication.
// This is a passive check that:
//  1. Checks keyring for cached credentials
//  2. Tries loading from identity-managed storage (AWS files, etc.)
//  3. Returns error if credentials are not found, expired, or invalid
//
// This method does NOT trigger any authentication flows or prompts.
func (m *manager) GetCachedCredentials(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.Manager.GetCachedCredentials")()

	if _, exists := m.identities[identityName]; !exists {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}

	// Retrieve credentials with automatic fallback from keyring to identity storage.
	creds, err := m.loadCredentialsWithFallback(ctx, identityName)
	if err != nil {
		// Credentials not found or error occurred.
		providerName := "unknown"
		if prov, provErr := m.identities[identityName].GetProviderName(); provErr == nil {
			providerName = prov
		}
		return nil, fmt.Errorf("%w: identity=%s, provider=%s, credential_store=%s: %w",
			errUtils.ErrNoCredentialsFound,
			identityName,
			providerName,
			m.credentialStore.Type(),
			err)
	}

	// Check if credentials are expired.
	if creds.IsExpired() {
		log.Debug("Cached credentials are expired", logKeyIdentity, identityName)
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrExpiredCredentials, fmt.Sprintf(backtickedFmt, identityName))
	}

	return m.buildWhoamiInfo(identityName, creds), nil
}

// Whoami returns information about the specified identity's credentials.
// First checks for cached credentials via GetCachedCredentials, then falls back
// to chain authentication (using cached provider credentials to derive identity credentials).
// This does NOT trigger interactive authentication flows (no SSO prompts).
func (m *manager) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.Manager.Whoami")()

	// First, try to get cached credentials (passive check).
	info, err := m.GetCachedCredentials(ctx, identityName)
	if err == nil {
		return info, nil
	}

	log.Debug("GetCachedCredentials failed, attempting chain authentication", logKeyIdentity, identityName, "error", err)

	// If cached credentials aren't available, try to authenticate through the chain.
	// This handles cases where provider credentials exist (e.g., in AWS files)
	// and can be used to derive the identity credentials without interactive prompts.
	authInfo, authErr := m.Authenticate(ctx, identityName)
	if authErr == nil {
		log.Debug("Successfully authenticated through chain", logKeyIdentity, identityName)
		return authInfo, nil
	}

	log.Debug("Chain authentication failed", logKeyIdentity, identityName, "error", authErr)

	// Return the original GetCachedCredentials error since chain auth also failed.
	return nil, err
}

// Validate validates the entire auth configuration.
func (m *manager) Validate() error {
	defer perf.Track(nil, "auth.Manager.Validate")()

	if err := m.validator.ValidateAuthConfig(m.config); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return nil
}

// GetDefaultIdentity returns the name of the default identity, if any.
func (m *manager) GetDefaultIdentity() (string, error) {
	defer perf.Track(nil, "auth.Manager.GetDefaultIdentity")()

	// Find all default identities.
	var defaultIdentities []string
	for name, identity := range m.config.Identities {
		if identity.Default {
			defaultIdentities = append(defaultIdentities, name)
		}
	}

	// Handle different scenarios based on number of default identities found.
	switch len(defaultIdentities) {
	case 0:
		// No default identities found.
		if !isInteractive() {
			return "", errUtils.ErrNoDefaultIdentity
		}
		// In interactive mode, prompt user to choose from all identities.
		return m.promptForIdentity("No default identity configured. Please choose an identity:", m.ListIdentities())

	case 1:
		// Exactly one default identity found - use it.
		return defaultIdentities[0], nil

	default:
		// Multiple default identities found.
		if !isInteractive() {
			return "", fmt.Errorf(errFormatWithString, errUtils.ErrMultipleDefaultIdentities, fmt.Sprintf(backtickedFmt, defaultIdentities))
		}
		// In interactive mode, prompt user to choose from default identities.
		return m.promptForIdentity("Multiple default identities found. Please choose one:", defaultIdentities)
	}
}

// promptForIdentity prompts the user to select an identity from the given list.
func (m *manager) promptForIdentity(message string, identities []string) (string, error) {
	if len(identities) == 0 {
		return "", errUtils.ErrNoIdentitiesAvailable
	}

	// Sort identities alphabetically for consistent ordering.
	sortedIdentities := make([]string, len(identities))
	copy(sortedIdentities, identities)
	sort.Strings(sortedIdentities)

	var selectedIdentity string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Options(huh.NewOptions(sortedIdentities...)...).
				Value(&selectedIdentity),
		),
	)

	if err := form.Run(); err != nil {
		// Check if user aborted (Ctrl+C, ESC, etc.).
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		errUtils.CheckErrorAndPrint(err, "Prompt for Identity", "")
		return "", fmt.Errorf("%w: %w", errUtils.ErrUnsupportedInputType, err)
	}

	return selectedIdentity, nil
}

// ListIdentities returns all available identity names.
func (m *manager) ListIdentities() []string {
	defer perf.Track(nil, "auth.Manager.ListIdentities")()

	var names []string
	for name := range m.config.Identities {
		names = append(names, name)
	}
	return names
}

// ListProviders returns all available provider names.
func (m *manager) ListProviders() []string {
	defer perf.Track(nil, "auth.Manager.ListProviders")()

	var names []string
	for name := range m.config.Providers {
		names = append(names, name)
	}
	return names
}

// initializeProviders creates provider instances from configuration.
func (m *manager) initializeProviders() error {
	//nolint:gocritic // rangeValCopy: map stores structs; address of map element can't be taken. Passing copy to factory is intended.
	for name, providerConfig := range m.config.Providers {
		provider, err := factory.NewProvider(name, &providerConfig)
		if err != nil {
			errUtils.CheckErrorAndPrint(err, "Initialize Providers", "")
			return fmt.Errorf("%w: provider=%s: %w", errUtils.ErrInvalidProviderConfig, name, err)
		}
		m.providers[name] = provider
	}
	return nil
}

// initializeIdentities creates identity instances from configuration.
func (m *manager) initializeIdentities() error {
	for name, identityConfig := range m.config.Identities {
		identity, err := factory.NewIdentity(name, &identityConfig)
		if err != nil {
			errUtils.CheckErrorAndPrint(err, "Initialize Identities", "")
			return fmt.Errorf("%w: identity=%s: %w", errUtils.ErrInvalidIdentityConfig, name, err)
		}
		m.identities[name] = identity
	}
	return nil
}

// getProviderForIdentity returns the provider name for the given identity.
// Uses the identity's GetProviderName() method to eliminate complex conditionals.
func (m *manager) getProviderForIdentity(identityName string) string {
	// First try to find by identity name.
	if identity, exists := m.identities[identityName]; exists {
		providerName, err := identity.GetProviderName()
		if err != nil {
			log.Debug("Failed to get provider name for identity", logKeyIdentity, identityName, "error", err)
			return ""
		}
		return providerName
	}

	// If not found by name, try to find by alias.
	for name, identity := range m.identities {
		if m.config.Identities[name].Alias == identityName {
			providerName, err := identity.GetProviderName()
			if err != nil {
				log.Debug("Failed to get provider name for identity alias", logKeyIdentity, identityName, "actualName", name, "error", err)
				return ""
			}
			return providerName
		}
	}

	return ""
}

// GetProviderForIdentity returns the provider name for the given identity.
// Recursively resolves through identity chains to find the root provider.
func (m *manager) GetProviderForIdentity(identityName string) string {
	defer perf.Track(nil, "auth.Manager.GetProviderForIdentity")()

	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil || len(chain) == 0 {
		return ""
	}
	if aws.IsStandaloneAWSUserChain(chain, m.config.Identities) {
		return "aws-user"
	}
	return chain[0]
}

// GetFilesDisplayPath returns the display path for credential files for a provider.
// Returns the configured path if set, otherwise a default path.
func (m *manager) GetFilesDisplayPath(providerName string) string {
	defer perf.Track(nil, "auth.Manager.GetFilesDisplayPath")()

	// Get provider instance.
	provider, exists := m.providers[providerName]
	if !exists {
		// Default path if provider not found (XDG base directory).
		return "~/.config/atmos"
	}

	// Delegate to provider to get display path.
	return provider.GetFilesDisplayPath()
}

// GetProviderKindForIdentity returns the provider kind for the given identity. By building the authentication chain and getting the root provider's kind.
func (m *manager) GetProviderKindForIdentity(identityName string) (string, error) {
	defer perf.Track(nil, "auth.Manager.GetProviderKindForIdentity")()

	// Build the complete authentication chain.
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to get provider kind for identity %q: %w", identityName, err)
		errUtils.CheckErrorAndPrint(wrappedErr, buildAuthenticationChain, "")
		return "", wrappedErr
	}

	if len(chain) == 0 {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, buildAuthenticationChain, "")
		return "", fmt.Errorf("%w: empty chain", errUtils.ErrInvalidAuthConfig)
	}

	// The first element in the chain is the root provider name.
	providerName := chain[0]

	// Look up the provider configuration and return its kind.
	if provider, exists := m.config.Providers[providerName]; exists {
		return provider.Kind, nil
	}

	if identity, exists := m.config.Identities[providerName]; exists {
		return identity.Kind, nil
	}

	errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "GetProviderKindForIdentity", fmt.Sprintf("provider %q not found in configuration", providerName))
	return "", fmt.Errorf("%w: provider %q not found in configuration", errUtils.ErrInvalidAuthConfig, providerName)
}

// authenticateChain performs hierarchical authentication with bottom-up validation.
func (m *manager) authenticateChain(ctx context.Context, targetIdentity string) (types.ICredentials, error) {
	// Step 1: Bottom-up validation - check cached credentials from target to root.
	validFromIndex := m.findFirstValidCachedCredentials()

	if validFromIndex != -1 {
		log.Debug("Found valid cached credentials", "validFromIndex", validFromIndex, "chainStep", m.getChainStepName(validFromIndex))

		// If target identity (last element in chain) has valid cached credentials, use them.
		if validFromIndex == len(m.chain)-1 {
			last := m.chain[len(m.chain)-1]
			if cachedCreds, err := m.credentialStore.Retrieve(last); err == nil {
				log.Debug("Using cached credentials for target identity", logKeyIdentity, targetIdentity)
				return cachedCreds, nil
			}
		}
	}

	// Step 2: Selective re-authentication from first invalid point down to target.
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
				log.Debug("Found valid cached credentials", logKeyChainIndex, i, identityNameKey, identityName, "expiration", *expTime)
			} else {
				// Credentials without expiration (API keys, long-lived tokens, etc.).
				log.Debug("Found valid cached credentials", logKeyChainIndex, i, identityNameKey, identityName, "expiration", "none")
			}
			return i
		}

		// Credentials exist but are expired - log and continue to next in chain.
		if expTime != nil {
			log.Debug("Credentials are expired", logKeyChainIndex, i, identityNameKey, identityName, "expiration", *expTime)
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
		// Expiration exists but is too close or already expired -> treat as invalid.
		log.Debug("Credentials expiring soon or already expired",
			logKeyIdentity, identityName,
			logKeyExpiration, expTime,
			"time_until_expiry", time.Until(*expTime),
			"required_buffer", minCredentialValidityBuffer)
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
	actualStartIndex := m.determineStartingIndex(startIndex)

	// Retrieve cached credentials if starting from a cached point.
	// Important: if we start from index N (>0) because cached creds exist at that step,
	// those creds are the OUTPUT of identity N and should be used as the base for the NEXT step (N+1).
	if actualStartIndex > 0 {
		currentCreds, actualStartIndex = m.fetchCachedCredentials(actualStartIndex)
	}

	// Step 1: Authenticate with provider if needed.
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
	currentCreds, err := m.getChainCredentials(m.chain, startIndex)
	if err != nil {
		log.Debug("Failed to retrieve cached credentials, starting from provider", "error", err)
		return nil, 0
	}
	// Skip re-authenticating the identity at startIndex, since we already have its output.
	return currentCreds, startIndex + 1
}

// GetIdentities returns the map of identities.
func (m *manager) GetIdentities() map[string]schema.Identity {
	return m.config.Identities
}

// GetProviders returns the map of providers.
func (m *manager) GetProviders() map[string]schema.Provider {
	return m.config.Providers
}

// GetConfig returns the config.
func (m *manager) GetConfig() *schema.ConfigAndStacksInfo {
	return m.stackInfo
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
func (m *manager) loadCredentialsWithFallback(ctx context.Context, identityName string) (types.ICredentials, error) {
	// Fast path: Try keyring cache first.
	creds, err := m.credentialStore.Retrieve(identityName)
	if err == nil {
		log.Debug("Retrieved credentials from keyring", logKeyIdentity, identityName)
		return creds, nil
	}

	// If keyring returned an error other than "not found", propagate it.
	if !errors.Is(err, credentials.ErrCredentialsNotFound) {
		return nil, fmt.Errorf("keyring error for identity %q: %w", identityName, err)
	}

	// Slow path: Fall back to identity storage (AWS files, etc.).
	// This handles cases where credentials were created outside of Atmos
	// (e.g., via AWS CLI/SSO) but are available in standard credential files.
	log.Debug("Credentials not in keyring, trying identity storage", logKeyIdentity, identityName)

	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrIdentityNotInConfig, identityName)
	}

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

	// Cache provider credentials.
	if err := m.credentialStore.Store(providerName, credentials); err != nil {
		log.Debug("Failed to cache provider credentials", "error", err)
	} else {
		log.Debug("Cached provider credentials", "providerName", providerName)
	}

	log.Debug("Provider authenticated", "provider", providerName)
	return credentials, nil
}

// Helper functions for logging.
func (m *manager) getChainStepName(index int) string {
	if index < len(m.chain) {
		return m.chain[index]
	}
	return "unknown"
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

		// Cache credentials for this level.
		if err := m.credentialStore.Store(identityStep, currentCreds); err != nil {
			log.Debug("Failed to cache credentials", "identityStep", identityStep, "error", err)
		} else {
			log.Debug("Cached credentials", "identityStep", identityStep)
		}

		log.Info("Chained identity", "from", m.getChainStepName(i-1), "to", identityStep)
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

// GetEnvironmentVariables returns the environment variables for an identity
// without performing authentication or validation.
func (m *manager) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	defer perf.Track(nil, "auth.Manager.GetEnvironmentVariables")()

	// Verify identity exists.
	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}

	// Get environment variables from the identity.
	env, err := identity.Environment()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get environment variables for identity %q: %w",
			errUtils.ErrInvalidAuthConfig, identityName, err)
	}

	return env, nil
}
