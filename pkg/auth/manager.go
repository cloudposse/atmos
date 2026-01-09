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
	"github.com/cloudposse/atmos/pkg/auth/factory"
	"github.com/cloudposse/atmos/pkg/auth/identities/aws"
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
	// Context key for skipping auto-triggered integrations.
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
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrInvalidAuthConfig, err)
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
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrUnsupportedInputType, err)
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
