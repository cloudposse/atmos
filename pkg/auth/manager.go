package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/factory"
	"github.com/cloudposse/atmos/pkg/auth/identities/aws"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	_ "github.com/cloudposse/atmos/pkg/auth/integrations/aws" // Register aws/ecr integration.
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
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
	// The minCredentialValidityBuffer is the minimum duration credentials must be valid.
	// AWS can invalidate credentials before their stated expiration time.
	minCredentialValidityBuffer = 15 * time.Minute
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// SkipProviderLogoutKey is the context key for skipping provider.Logout calls.
	skipProviderLogoutKey contextKey = "skipProviderLogout"
	// skipIntegrationsKey is the context key for skipping auto-triggered integrations.
	// Used when ExecuteIntegration explicitly runs an integration to avoid duplicate execution.
	skipIntegrationsKey contextKey = "skipIntegrations"
)

// isInteractive checks if we're running in an interactive terminal.
// Interactive mode requires stdin to be a TTY (for user input) and must not be in CI.
// We don't check stdout because users should be able to pipe output (e.g., | cat)
// while still interacting via stdin.
func isInteractive() bool {
	return term.IsTTYSupportForStdin() && !telemetry.IsCI()
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

// resolveIdentityName performs case-insensitive identity name lookup.
// If an exact match exists, it returns the exact match.
// Otherwise, it looks up the lowercase version in the case map.
// Returns the resolved name and whether it was found.
func (m *manager) resolveIdentityName(inputName string) (string, bool) {
	// First try exact match (fast path for already-lowercase names)
	if _, exists := m.identities[inputName]; exists {
		return inputName, true
	}

	// Try case-insensitive lookup using the case map
	if m.config.IdentityCaseMap != nil {
		lowercaseName := strings.ToLower(inputName)
		if _, exists := m.config.IdentityCaseMap[lowercaseName]; exists {
			// Verify the identity actually exists with the lowercase key
			if _, identityExists := m.identities[lowercaseName]; identityExists {
				return lowercaseName, true
			}
		}
	}

	return "", false
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

	// Resolve identity name case-insensitively
	resolvedName, found := m.resolveIdentityName(identityName)
	if !found {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, identityNameKey, "Identity specified was not found in the auth config.")
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}
	// Use the resolved lowercase name for internal lookups
	identityName = resolvedName

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

	// Perform credential chain authentication (bottom-up).
	finalCreds, err := m.authenticateChain(ctx, identityName)
	if err != nil {
		wrappedErr := fmt.Errorf("%w: failed to authenticate via credential chain for identity %q: %w", errUtils.ErrAuthenticationFailed, identityName, err)
		errUtils.CheckErrorAndPrint(wrappedErr, "Authenticate Credential Chain", "")
		return nil, wrappedErr
	}

	// Call post-authentication hook on the identity (now part of Identity interface).
	if identity, exists := m.identities[identityName]; exists {
		// Get the root provider name from the authentication chain.
		// The chain is [provider, identity1, identity2, ..., targetIdentity].
		// We use the first element (root provider) for file storage to ensure
		// consistent directory structure regardless of identity chaining.
		rootProviderName := m.chain[0]

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
			ProviderName: rootProviderName,
			IdentityName: identityName,
			Credentials:  finalCreds,
			Manager:      m,
		}); err != nil {
			wrappedErr := fmt.Errorf("%w: post-authentication failed: %w", errUtils.ErrAuthenticationFailed, err)
			errUtils.CheckErrorAndPrint(wrappedErr, "Post Authenticate", "")
			return nil, wrappedErr
		}

		// Trigger linked integrations (non-fatal).
		m.triggerIntegrations(ctx, identityName, finalCreds)
	}

	return m.buildWhoamiInfo(identityName, finalCreds), nil
}

// AuthenticateProvider performs authentication directly with a provider.
// This is used for provider-level operations like SSO auto-provisioning where
// you want to authenticate to a provider without specifying a particular identity.
func (m *manager) AuthenticateProvider(ctx context.Context, providerName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.Manager.AuthenticateProvider")()

	log.Debug("Starting provider authentication", logKeyProvider, providerName)

	// Resolve provider name case-insensitively.
	resolvedProviderName := ""
	for name := range m.providers {
		if strings.EqualFold(name, providerName) {
			resolvedProviderName = name
			break
		}
	}

	if resolvedProviderName == "" {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrProviderNotFound, fmt.Sprintf(backtickedFmt, providerName))
	}

	// Use resolved name for authentication.
	providerName = resolvedProviderName

	// Authenticate with the provider.
	credentials, err := m.authenticateWithProvider(ctx, providerName)
	if err != nil {
		return nil, err
	}

	// Build chain with just the provider.
	m.chain = []string{providerName}

	// Build and return whoami info for the provider.
	return m.buildProviderWhoamiInfo(providerName, credentials), nil
}

// buildProviderWhoamiInfo builds WhoamiInfo for a provider (without an identity).
func (m *manager) buildProviderWhoamiInfo(providerName string, creds types.ICredentials) *types.WhoamiInfo {
	info := &types.WhoamiInfo{
		Provider: providerName,
		Identity: "", // No identity for provider-only auth.
	}

	// Add provider-specific fields if available.
	if awsCreds, ok := creds.(*types.AWSCredentials); ok {
		info.Region = awsCreds.Region
		// Parse expiration string to time.Time if present.
		if awsCreds.Expiration != "" {
			if expTime, err := time.Parse(time.RFC3339, awsCreds.Expiration); err == nil {
				info.Expiration = &expTime
			}
		}
	}

	return info
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

	// Resolve identity name case-insensitively
	resolvedName, found := m.resolveIdentityName(identityName)
	if !found {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}
	// Use the resolved lowercase name for internal lookups
	identityName = resolvedName

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
// This does NOT trigger interactive authentication flows (no SSO prompts, no credential prompts).
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
	// Use a non-interactive context to prevent credential prompts during whoami.
	nonInteractiveCtx := types.WithAllowPrompts(ctx, false)
	authInfo, authErr := m.Authenticate(nonInteractiveCtx, identityName)
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
// If forceSelect is true and terminal is interactive, always shows identity selector.
func (m *manager) GetDefaultIdentity(forceSelect bool) (string, error) {
	defer perf.Track(nil, "auth.Manager.GetDefaultIdentity")()

	// If forceSelect is true, user explicitly requested identity selection.
	if forceSelect {
		// Check if we're in interactive mode (have TTY).
		if !isInteractive() {
			// User requested interactive selection but we don't have a TTY.
			return "", errUtils.ErrIdentitySelectionRequiresTTY
		}
		// We have a TTY - show selector.
		return m.promptForIdentity("Select an identity:", m.ListIdentities())
	}

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

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Description("Press ctrl+c or esc to exit").
				Options(huh.NewOptions(sortedIdentities...)...).
				Value(&selectedIdentity),
		),
	).WithKeyMap(keyMap)

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

// ListIdentities returns all available identity names with their original case.
// Uses IdentityCaseMap to preserve the original case from YAML config,
// working around Viper's lowercase conversion of map keys.
func (m *manager) ListIdentities() []string {
	defer perf.Track(nil, "auth.Manager.ListIdentities")()

	var names []string
	for lowercaseName := range m.config.Identities {
		// Use original case from IdentityCaseMap if available, otherwise use lowercase.
		if m.config.IdentityCaseMap != nil {
			if originalName, exists := m.config.IdentityCaseMap[lowercaseName]; exists {
				names = append(names, originalName)
				continue
			}
		}
		// Fallback to lowercase name if case map not available.
		names = append(names, lowercaseName)
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

// ensureIdentityHasManager ensures the identity has the authentication chain context.
// This is needed when using cached credentials where the chain was not built in this session.
// Building the chain and setting manager reference allows identity to resolve the root provider.
func (m *manager) ensureIdentityHasManager(identityName string) error {
	// If we don't have config, we can't build the chain - skip this step.
	// This happens in unit tests where manager is created without full config.
	if m.config == nil {
		return nil
	}

	// If chain is already built for this identity, we're good.
	if len(m.chain) > 0 && m.chain[len(m.chain)-1] == identityName {
		// Chain exists - still need to ensure identity has manager reference.
		// Call PostAuthenticate with minimal params to set manager field.
		return m.setIdentityManager(identityName)
	}

	// If chain exists, check if the requested identity is part of it.
	// If the identity is in the chain, we can reuse it. Otherwise, rebuild.
	if len(m.chain) > 0 {
		// Check if identityName is present in the existing chain.
		identityInChain := false
		for _, chainIdentity := range m.chain {
			if chainIdentity == identityName {
				identityInChain = true
				break
			}
		}

		if identityInChain {
			// Identity is in the existing chain - set manager reference.
			// This happens when loading cached credentials for an intermediate identity
			// (e.g., permission set) while authenticating a target identity (e.g., assume role).
			return m.setIdentityManager(identityName)
		}

		// Identity is NOT in the existing chain - must rebuild for this identity.
		// Clear the stale chain and fall through to rebuild.
		m.chain = nil
	}

	// Build the authentication chain so GetProviderForIdentity() can resolve the root provider.
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		return fmt.Errorf("failed to build authentication chain: %w", err)
	}

	// Store the chain in the manager so GetProviderForIdentity() can use it.
	m.chain = chain

	// Set manager reference on the identity.
	return m.setIdentityManager(identityName)
}

// setIdentityManager sets the manager reference on AWS identities.
// This allows the identity to use manager.GetProviderForIdentity() to resolve the root provider.
// For AWS permission set and assume role identities, we can set the manager field directly.
func (m *manager) setIdentityManager(identityName string) error {
	identity, exists := m.identities[identityName]
	if !exists {
		return nil // Identity not found, skip
	}

	// Get root provider name from the chain.
	if len(m.chain) == 0 {
		return errUtils.ErrAuthenticationChainNotBuilt
	}
	rootProviderName := m.chain[0]

	// Type assert to AWS permission set identity and set manager directly.
	// This is safe because we're only dealing with permission set identities here.
	type awsIdentityWithManager interface {
		SetManagerAndProvider(types.AuthManager, string)
	}

	if awsIdentity, ok := identity.(awsIdentityWithManager); ok {
		awsIdentity.SetManagerAndProvider(m, rootProviderName)
	}

	// If identity doesn't implement SetManagerAndProvider, that's OK - it may not need it.
	return nil
}

// authenticateChain performs credential chain authentication with bottom-up validation.
func (m *manager) authenticateChain(ctx context.Context, targetIdentity string) (types.ICredentials, error) {
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
				log.Debug("Found valid cached credentials", logKeyChainIndex, i, identityNameKey, identityName, "expiration", *expTime)
			} else {
				// Credentials without expiration (API keys, long-lived tokens, etc.).
				log.Debug("Found valid cached credentials", logKeyChainIndex, i, identityNameKey, identityName, "expiration", "none")
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
					"expiration", *expTime,
					"expired_ago", -timeUntilExpiry)
			} else {
				log.Debug("Skipping credentials expiring too soon in chain (within safety buffer)",
					logKeyChainIndex, i,
					identityNameKey, identityName,
					"expiration", *expTime,
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
		return nil, fmt.Errorf("%w: %s", errUtils.ErrIdentityNotInConfig, identityName)
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

// GetEnvironmentVariables returns the environment variables for an identity
// without performing authentication or validation.
func (m *manager) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	defer perf.Track(nil, "auth.Manager.GetEnvironmentVariables")()

	// Verify identity exists.
	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}

	// Ensure the identity has access to manager for resolving provider information.
	// This builds the authentication chain and sets manager reference so the identity
	// can resolve the root provider for file-based credentials.
	// This is best-effort - if it fails, the identity will fall back to config-based resolution.
	_ = m.ensureIdentityHasManager(identityName)

	// Get environment variables from the identity.
	env, err := identity.Environment()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get environment variables: %w", errUtils.ErrAuthManager, err)
	}

	return env, nil
}

// PrepareShellEnvironment prepares environment variables for subprocess execution.
// Takes current environment list and returns it with auth credentials configured.
// This calls identity.PrepareEnvironment() internally to configure file-based credentials.
func (m *manager) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	defer perf.Track(nil, "auth.Manager.PrepareShellEnvironment")()

	// Verify identity exists.
	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrIdentityNotFound, fmt.Sprintf(backtickedFmt, identityName))
	}

	// Ensure the identity has access to manager for resolving provider information.
	// This is best-effort - if it fails, the identity will fall back to config-based resolution.
	_ = m.ensureIdentityHasManager(identityName)

	// Convert input environment list to map for identity.PrepareEnvironment().
	envMap := environListToMap(currentEnv)

	// Call identity.PrepareEnvironment() to configure auth credentials.
	// This is provider-specific (AWS sets AWS_SHARED_CREDENTIALS_FILE, AWS_PROFILE, etc.).
	preparedEnvMap, err := identity.PrepareEnvironment(ctx, envMap)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare shell environment for identity %q: %w", identityName, err)
	}

	// Convert map back to list for subprocess execution.
	return mapToEnvironList(preparedEnvMap), nil
}

// environListToMap converts environment variable list to map.
// Input: ["KEY=value", "FOO=bar"]
// Output: {"KEY": "value", "FOO": "bar"}
func environListToMap(envList []string) map[string]string {
	envMap := make(map[string]string, len(envList))
	for _, envVar := range envList {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			envMap[key] = value
		}
	}
	return envMap
}

// mapToEnvironList converts environment variable map to list.
// Input: {"KEY": "value", "FOO": "bar"}
// Output: ["KEY=value", "FOO=bar"]
func mapToEnvironList(envMap map[string]string) []string {
	envList := make([]string, 0, len(envMap))
	for key, value := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", key, value))
	}
	return envList
}

// triggerIntegrations executes integrations that reference this identity with auto_provision enabled.
// This is a non-fatal operation - integration failures don't block authentication.
// Skipped when context contains skipIntegrationsKey (used by ExecuteIntegration to avoid duplicate execution).
func (m *manager) triggerIntegrations(ctx context.Context, identityName string, creds types.ICredentials) {
	defer perf.Track(nil, "auth.Manager.triggerIntegrations")()

	// Check if integrations should be skipped (when called from ExecuteIntegration).
	if ctx.Value(skipIntegrationsKey) != nil {
		log.Debug("Skipping auto-triggered integrations (explicit execution)", logKeyIdentity, identityName)
		return
	}

	// Find integrations that reference this identity and have auto_provision enabled.
	linkedIntegrations := m.findIntegrationsForIdentity(identityName, true)
	if len(linkedIntegrations) == 0 {
		return
	}

	log.Debug("Triggering linked integrations", logKeyIdentity, identityName, "count", len(linkedIntegrations))

	// Execute each linked integration.
	for _, integrationName := range linkedIntegrations {
		if err := m.executeIntegration(ctx, integrationName, creds); err != nil {
			// Non-fatal: log warning and continue.
			log.Warn("Integration failed", "integration", integrationName, "error", err)
		}
	}
}

// findIntegrationsForIdentity returns integration names that reference the given identity.
// If autoProvisionOnly is true, only returns integrations with auto_provision enabled (defaults to true).
func (m *manager) findIntegrationsForIdentity(identityName string, autoProvisionOnly bool) []string {
	if m.config.Integrations == nil {
		return nil
	}

	var result []string
	for name, integration := range m.config.Integrations {
		// Check if this integration references the given identity.
		if integration.Via == nil || integration.Via.Identity != identityName {
			continue
		}

		// If autoProvisionOnly, check if auto_provision is enabled (defaults to true).
		if autoProvisionOnly {
			autoProvision := true // Default to true when not specified.
			if integration.Spec != nil && integration.Spec.AutoProvision != nil {
				autoProvision = *integration.Spec.AutoProvision
			}
			if !autoProvision {
				continue
			}
		}

		result = append(result, name)
	}
	return result
}

// executeIntegration executes a single integration by name.
func (m *manager) executeIntegration(ctx context.Context, integrationName string, creds types.ICredentials) error {
	defer perf.Track(nil, "auth.Manager.executeIntegration")()

	// Look up integration config.
	if m.config.Integrations == nil {
		return fmt.Errorf("%w: no integrations configured", errUtils.ErrIntegrationNotFound)
	}

	integrationConfig, exists := m.config.Integrations[integrationName]
	if !exists {
		return fmt.Errorf("%w: %s", errUtils.ErrIntegrationNotFound, integrationName)
	}

	// Create integration instance.
	integration, err := integrations.Create(&integrations.IntegrationConfig{
		Name:   integrationName,
		Config: &integrationConfig,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	// Execute the integration.
	if err := integration.Execute(ctx, creds); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	log.Debug("Integration executed successfully", "integration", integrationName)
	return nil
}

// ExecuteIntegration exposes integration execution for the standalone ecr-login command.
// This authenticates the integration's linked identity first, then executes the integration.
// Auto-triggered integrations are skipped during authentication to avoid duplicate execution.
func (m *manager) ExecuteIntegration(ctx context.Context, integrationName string) error {
	defer perf.Track(nil, "auth.Manager.ExecuteIntegration")()

	// Look up integration config.
	if m.config.Integrations == nil {
		return fmt.Errorf("%w: no integrations configured", errUtils.ErrIntegrationNotFound)
	}

	integrationConfig, exists := m.config.Integrations[integrationName]
	if !exists {
		return fmt.Errorf("%w: %s", errUtils.ErrIntegrationNotFound, integrationName)
	}

	// Get the identity from via.identity.
	if integrationConfig.Via == nil || integrationConfig.Via.Identity == "" {
		return fmt.Errorf("%w: integration '%s' has no identity configured", errUtils.ErrIntegrationFailed, integrationName)
	}
	identityName := integrationConfig.Via.Identity

	// Authenticate the linked identity with integrations skipped.
	// We skip auto-triggered integrations because we'll execute this specific integration explicitly below.
	// This prevents duplicate execution when the requested integration is also auto-provisioned.
	ctxSkipIntegrations := context.WithValue(ctx, skipIntegrationsKey, true)
	whoami, err := m.Authenticate(ctxSkipIntegrations, identityName)
	if err != nil {
		return fmt.Errorf("failed to authenticate identity '%s': %w", identityName, err)
	}

	// Use credentials from authentication result.
	if whoami.Credentials == nil {
		return fmt.Errorf("failed to get credentials for identity '%s': credentials not available", identityName)
	}

	log.Debug("Authenticated identity for integration", "identity", identityName, "whoami", whoami.Identity)

	// Create and execute the integration.
	integration, err := integrations.Create(&integrations.IntegrationConfig{
		Name:   integrationName,
		Config: &integrationConfig,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	return integration.Execute(ctx, whoami.Credentials)
}

// ExecuteIdentityIntegrations executes all integrations that reference this identity.
// This authenticates the identity first, then executes all integrations linked to it.
// Auto-triggered integrations are skipped during authentication to avoid duplicate execution.
func (m *manager) ExecuteIdentityIntegrations(ctx context.Context, identityName string) error {
	defer perf.Track(nil, "auth.Manager.ExecuteIdentityIntegrations")()

	// Verify the identity exists.
	_, exists := m.config.Identities[identityName]
	if !exists {
		return fmt.Errorf("%w: %s", errUtils.ErrIdentityNotFound, identityName)
	}

	// Find all integrations that reference this identity (not just auto_provision ones).
	linkedIntegrations := m.findIntegrationsForIdentity(identityName, false)
	if len(linkedIntegrations) == 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrNoLinkedIntegrations, identityName)
	}

	// Authenticate the identity with integrations skipped.
	// We skip auto-triggered integrations because we'll execute all linked integrations explicitly below.
	// This prevents duplicate execution for integrations that are also auto-provisioned.
	ctxSkipIntegrations := context.WithValue(ctx, skipIntegrationsKey, true)
	whoami, err := m.Authenticate(ctxSkipIntegrations, identityName)
	if err != nil {
		return fmt.Errorf("failed to authenticate identity '%s': %w", identityName, err)
	}

	// Use credentials from authentication result.
	if whoami.Credentials == nil {
		return fmt.Errorf("failed to get credentials for identity '%s': credentials not available", identityName)
	}

	log.Debug("Authenticated identity for integrations", "identity", identityName, "whoami", whoami.Identity)

	// Execute each linked integration.
	for _, integrationName := range linkedIntegrations {
		if err := m.executeIntegration(ctx, integrationName, whoami.Credentials); err != nil {
			return fmt.Errorf("integration '%s' failed: %w", integrationName, err)
		}
	}

	return nil
}

// GetIntegration returns the integration config by name.
func (m *manager) GetIntegration(integrationName string) (*schema.Integration, error) {
	if m.config.Integrations == nil {
		return nil, fmt.Errorf("%w: no integrations configured", errUtils.ErrIntegrationNotFound)
	}

	integrationConfig, exists := m.config.Integrations[integrationName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrIntegrationNotFound, integrationName)
	}

	return &integrationConfig, nil
}
