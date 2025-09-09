package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/identities/aws"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

const (
	logKeyIdentity           = "identity"
	identityNameKey          = "identityName"
	buildAuthenticationChain = "buildAuthenticationChain"
	buildChainRecursive      = "buildChainRecursive"
)

var (
	ErrNoCredentialsFound          = errors.New("no credentials found for identity")
	ErrExpiredCredentials          = errors.New("credentials for identity are expired or invalid")
	ErrNilParam                    = errors.New("parameter cannot be nil")
	ErrInitializingProviders       = errors.New("failed to initialize providers")
	ErrInitializingIdentities      = errors.New("failed to initialize identities")
	ErrInitializingCredentialStore = errors.New("failed to initialize credential store")
	ErrCircularDependency          = errors.New("circular dependency detected in identity chain")
	ErrIdentityNotFound            = errors.New("identity not found")
	ErrNoDefaultIdentity           = errors.New("no default identity configured for authentication")
	ErrMultipleDefaultIdentities   = errors.New("multiple default identities found")
	ErrTerraformPreHook            = errors.New("terraform pre-hook failed")
	ErrNoIdentitiesAvailable       = errors.New("no identities available")
)

// manager implements the Authmanager interface.
type manager struct {
	config          *schema.AuthConfig
	providers       map[string]types.Provider
	identities      map[string]types.Identity
	credentialStore types.CredentialStore
	validator       types.Validator
	stackInfo       *schema.ConfigAndStacksInfo
	// chain holds the most recently constructed authentication chain
	// where index 0 is the provider name, followed by identities in order.
	chain []string
}

// NewAuthManager creates a new Authmanager instance.
func NewAuthManager(
	config *schema.AuthConfig,
	credentialStore types.CredentialStore,
	validator types.Validator,
	stackInfo *schema.ConfigAndStacksInfo,
) (types.AuthManager, error) {
	if config == nil {
		errUtils.CheckErrorAndPrint(ErrNilParam, "config", "auth config cannot be nil")
		return nil, ErrNilParam
	}
	if credentialStore == nil {
		errUtils.CheckErrorAndPrint(ErrNilParam, "credentialStore", "credential store cannot be nil")
		return nil, ErrNilParam
	}
	if validator == nil {
		errUtils.CheckErrorAndPrint(ErrNilParam, "validator", "validator cannot be nil")
		return nil, ErrNilParam
	}

	m := &manager{
		config:          config,
		providers:       make(map[string]types.Provider),
		identities:      make(map[string]types.Identity),
		credentialStore: credentialStore,
		validator:       validator,
		stackInfo:       stackInfo,
	}

	// Initialize providers
	if err := m.initializeProviders(); err != nil {
		errUtils.CheckErrorAndPrint(ErrInitializingProviders, "initializeProviders", "failed to initialize providers")
		return nil, ErrInitializingProviders
	}

	// Initialize identities
	if err := m.initializeIdentities(); err != nil {
		errUtils.CheckErrorAndPrint(ErrInitializingIdentities, "initializeIdentities", "failed to initialize identities")
		return nil, ErrInitializingIdentities
	}

	return m, nil
}

// GetStackInfo returns the associated stack info pointer (may be nil).
func (m *manager) GetStackInfo() *schema.ConfigAndStacksInfo {
	return m.stackInfo
}

// Authenticate performs hierarchical authentication for the specified identity.
func (m *manager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	// We expect the identity name to be provided by the caller.
	if identityName == "" {
		errUtils.CheckErrorAndPrint(ErrNilParam, identityNameKey, "no identity specified")
		return nil, ErrNilParam
	}
	if _, exists := m.identities[identityName]; !exists {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, identityNameKey, "Identity specified was not found in the auth config.")
		return nil, errUtils.ErrInvalidAuthConfig
	}

	// Build the complete authentication chain
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, buildAuthenticationChain, "Check your atmos.yaml.")
		return nil, errUtils.ErrInvalidAuthConfig
	}
	// Persist the chain for later retrieval by providers or callers
	m.chain = chain
	log.Debug("Authentication chain discovered", logKeyIdentity, identityName, "chainLength", len(chain), "chain", chain)

	// Perform hierarchical credential validation (bottom-up)
	finalCreds, err := m.authenticateHierarchical(ctx, identityName)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrAuthenticationFailed, "authenticateHierarchical", "")
		return nil, errUtils.ErrAuthenticationFailed
	}

	// Call post-authentication hook on the identity (now part of Identity interface).
	if identity, exists := m.identities[identityName]; exists {
		providerName, perr := identity.GetProviderName()
		if perr != nil {
			errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "GetProviderName", "")
			return nil, errUtils.ErrInvalidAuthConfig
		}
		if err := identity.PostAuthenticate(ctx, m.stackInfo, providerName, identityName, finalCreds); err != nil {
			errUtils.CheckErrorAndPrint(errUtils.ErrAuthenticationFailed, "PostAuthenticate", "")
			return nil, errUtils.ErrAuthenticationFailed
		}
	}

	return m.buildWhoamiInfo(identityName, finalCreds), nil
}

// GetChain returns the most recently built authentication chain.
// The chain is in the format: [providerName, identity1, identity2, ..., targetIdentity].
func (m *manager) GetChain() []string {
	return m.chain
}

// Whoami returns information about the specified identity's credentials.
func (m *manager) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	if _, exists := m.identities[identityName]; !exists {
		return nil, fmt.Errorf(errUtils.ErrStringWrappingFormat, ErrIdentityNotFound, fmt.Sprintf("`%q`", identityName))
	}

	// Try to retrieve credentials for the resolved identity.
	creds, err := m.credentialStore.Retrieve(identityName)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrStringWrappingFormat, ErrNoCredentialsFound, fmt.Sprintf("`%q`", identityName))
	}

	// Check if credentials are expired
	if expired, err := m.credentialStore.IsExpired(identityName); err != nil || expired {
		return nil, fmt.Errorf(errUtils.ErrStringWrappingFormat, ErrExpiredCredentials, fmt.Sprintf("`%q`", identityName))
	}

	return m.buildWhoamiInfo(identityName, creds), nil
}

// Validate validates the entire auth configuration.
func (m *manager) Validate() error {
	return m.validator.ValidateAuthConfig(m.config)
}

// GetDefaultIdentity returns the name of the default identity, if any.
func (m *manager) GetDefaultIdentity() (string, error) {
	// Find all default identities
	var defaultIdentities []string
	for name, identity := range m.config.Identities {
		if identity.Default {
			defaultIdentities = append(defaultIdentities, name)
		}
	}

	// Handle different scenarios based on number of default identities found
	switch len(defaultIdentities) {
	case 0:
		// No default identities found
		if telemetry.IsCI() {
			return "", ErrNoDefaultIdentity
		}
		// In interactive mode, prompt user to choose from all identities
		return m.promptForIdentity("No default identity configured. Please choose an identity:", m.ListIdentities())

	case 1:
		// Exactly one default identity found - use it
		return defaultIdentities[0], nil

	default:
		// Multiple default identities found.
		if telemetry.IsCI() {
			return "", fmt.Errorf(errUtils.ErrStringWrappingFormat, ErrMultipleDefaultIdentities, fmt.Sprintf("`%q`", defaultIdentities))
		}
		// In interactive mode, prompt user to choose from default identities
		return m.promptForIdentity("Multiple default identities found. Please choose one:", defaultIdentities)
	}
}

// promptForIdentity prompts the user to select an identity from the given list.
func (m *manager) promptForIdentity(message string, identities []string) (string, error) {
	if len(identities) == 0 {
		return "", fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrInvalidAuthConfig, ErrNoIdentitiesAvailable)
	}

	var selectedIdentity string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Options(huh.NewOptions(identities...)...).
				Value(&selectedIdentity),
		),
	)

	if err := form.Run(); err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrUnsupportedInputType, "promptForIdentity", "")
		return "", errUtils.ErrUnsupportedInputType
	}

	return selectedIdentity, nil
}

// ListIdentities returns all available identity names.
func (m *manager) ListIdentities() []string {
	var names []string
	for name := range m.config.Identities {
		names = append(names, name)
	}
	return names
}

// ListProviders returns all available provider names.
func (m *manager) ListProviders() []string {
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
		provider, err := NewProvider(name, &providerConfig)
		if err != nil {
			errUtils.CheckErrorAndPrint(errUtils.ErrInvalidProviderConfig, "initializeProviders", "")
			return errUtils.ErrInvalidProviderConfig
		}
		m.providers[name] = provider
	}
	return nil
}

// initializeIdentities creates identity instances from configuration.
func (m *manager) initializeIdentities() error {
	for name, identityConfig := range m.config.Identities {
		identity, err := NewIdentity(name, &identityConfig)
		if err != nil {
			errUtils.CheckErrorAndPrint(errUtils.ErrInvalidIdentityConfig, "initializeIdentities", "")
			return errUtils.ErrInvalidIdentityConfig
		}
		m.identities[name] = identity
	}
	return nil
}

// getProviderForIdentity returns the provider name for the given identity
// Uses the identity's GetProviderName() method to eliminate complex conditionals.
func (m *manager) getProviderForIdentity(identityName string) string {
	// First try to find by identity name
	if identity, exists := m.identities[identityName]; exists {
		providerName, err := identity.GetProviderName()
		if err != nil {
			log.Debug("Failed to get provider name for identity", logKeyIdentity, identityName, "error", err)
			return ""
		}
		return providerName
	}

	// If not found by name, try to find by alias
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

// GetProviderForIdentity returns the provider name for the given identity
// Recursively resolves through identity chains to find the root provider.
func (m *manager) GetProviderForIdentity(identityName string) string {
	return m.getProviderForIdentity(identityName)
}

// GetProviderKindForIdentity returns the provider kind for the given identity
// by building the authentication chain and getting the root provider's kind.
func (m *manager) GetProviderKindForIdentity(identityName string) (string, error) {
	// Build the complete authentication chain
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, buildAuthenticationChain, "")
		return "", errUtils.ErrInvalidAuthConfig
	}

	if len(chain) == 0 {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, buildAuthenticationChain, "")
		return "", errUtils.ErrInvalidAuthConfig
	}

	// The first element in the chain is the root provider name
	providerName := chain[0]

	// Look up the provider configuration and return its kind
	if provider, exists := m.config.Providers[providerName]; exists {
		return provider.Kind, nil
	}

	if identity, exists := m.config.Identities[providerName]; exists {
		return identity.Kind, nil
	}

	errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "GetProviderKindForIdentity", fmt.Sprintf("provider %q not found in configuration", providerName))
	return "", errUtils.ErrInvalidAuthConfig
}

// authenticateHierarchical performs hierarchical authentication with bottom-up validation.
func (m *manager) authenticateHierarchical(ctx context.Context, targetIdentity string) (types.ICredentials, error) {
	// Step 1: Bottom-up validation - check cached credentials from target to root
	validFromIndex := m.findFirstValidCachedCredentials()

	if validFromIndex != -1 {
		log.Debug("Found valid cached credentials", "validFromIndex", validFromIndex, "chainStep", m.getChainStepName(validFromIndex))

		// If target identity (last element in chain) has valid cached credentials, use them
		if validFromIndex == len(m.chain)-1 {
			last := m.chain[len(m.chain)-1]
			if cachedCreds, err := m.credentialStore.Retrieve(last); err == nil {
				log.Debug("Using cached credentials for target identity", logKeyIdentity, targetIdentity)
				return cachedCreds, nil
			}
		}
	}

	// Step 2: Selective re-authentication from first invalid point down to target
	return m.authenticateFromIndex(ctx, validFromIndex)
}

// findFirstValidCachedCredentials checks cached credentials from bottom to top of chain
// Returns the index of the first valid cached credentials, or -1 if none found.
func (m *manager) findFirstValidCachedCredentials() int {
	// Check from target identity (bottom) up to provider (top)
	for i := len(m.chain) - 1; i >= 0; i-- {
		identityName := m.chain[i]

		// Check if we have cached credentials for this level
		cachedCreds, err := m.credentialStore.Retrieve(identityName)
		if err != nil {
			continue
		}

		if valid, expTime := m.isCredentialValid(identityName, cachedCreds); valid {
			if expTime != nil {
				log.Debug("Found valid cached credentials", "chainIndex", i, identityNameKey, identityName, "expiration", *expTime)
			} else {
				// Non-AWS credentials or no expiration info, assume valid
				log.Debug("Found valid cached credentials (non-AWS)", "chainIndex", i, identityNameKey, identityName)
			}
			return i
		}
	}
	return -1 // No valid cached credentials found
}

// isCredentialValid checks if the cached credentials are valid and not expired.
// Returns whether the credentials are valid and, if AWS expiration is present and valid, the parsed expiration time.
func (m *manager) isCredentialValid(identityName string, cachedCreds types.ICredentials) (bool, *time.Time) {
	expired, err := m.credentialStore.IsExpired(identityName)
	if err != nil || expired {
		return false, nil
	}

	if expTime, err := cachedCreds.GetExpiration(); err == nil && expTime != nil {
		if expTime.After(time.Now().Add(5 * time.Minute)) {
			return true, expTime
		}
	}

	return true, nil
}

// authenticateFromIndex performs authentication starting from the given index in the chain.
func (m *manager) authenticateFromIndex(ctx context.Context, startIndex int) (types.ICredentials, error) {
	// Todo Ideally this wouldn't be here, and would be handled by an identity interface function
	// Handle special case: standalone AWS user identity
	if aws.IsStandaloneAWSUserChain(m.chain, m.config.Identities) {
		return aws.AuthenticateStandaloneAWSUser(ctx, m.chain[0], m.identities)
	}

	// Handle regular provider-based authentication chains
	return m.authenticateProviderChain(ctx, startIndex)
}

// authenticateProviderChain handles authentication for provider-based identity chains.
func (m *manager) authenticateProviderChain(ctx context.Context, startIndex int) (types.ICredentials, error) {
	var currentCreds types.ICredentials
	var err error

	// Determine actual starting point for authentication
	actualStartIndex := m.determineStartingIndex(startIndex)

	// Retrieve cached credentials if starting from a cached point.
	// Important: if we start from index N (>0) because cached creds exist at that step,
	// those creds are the OUTPUT of identity N and should be used as the base for the NEXT step (N+1).
	if actualStartIndex > 0 {
		currentCreds, actualStartIndex = m.fetchCachedCredentials(actualStartIndex)
	}

	// Step 1: Authenticate with provider if needed
	if actualStartIndex == 0 { //nolint:nestif
		// Allow provider to inspect the chain and prepare pre-auth preferences
		if provider, exists := m.providers[m.chain[0]]; exists {
			if err := provider.PreAuthenticate(m); err != nil {
				errUtils.CheckErrorAndPrint(errUtils.ErrAuthenticationFailed, "PreAuthenticate", "")
				return nil, errUtils.ErrAuthenticationFailed
			}
		}
		currentCreds, err = m.authenticateWithProvider(ctx, m.chain[0])
		if err != nil {
			return nil, err
		}
		actualStartIndex = 1
	}

	// Step 2: Authenticate through identity chain
	return m.authenticateIdentityChain(ctx, actualStartIndex, currentCreds)
}

func (m *manager) fetchCachedCredentials(startIndex int) (types.ICredentials, int) {
	currentCreds, err := m.retrieveCachedCredentials(m.chain, startIndex)
	if err != nil {
		log.Debug("Failed to retrieve cached credentials, starting from provider", "error", err)
		return nil, 0
	}
	// Skip re-authenticating the identity at startIndex, since we already have its output
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

// retrieveCachedCredentials retrieves cached credentials from the specified starting point.
func (m *manager) retrieveCachedCredentials(chain []string, startIndex int) (types.ICredentials, error) {
	identityName := chain[startIndex]
	currentCreds, err := m.credentialStore.Retrieve(identityName)
	if err != nil {
		return nil, err
	}

	log.Debug("Starting authentication from cached credentials", "startIndex", startIndex)
	return currentCreds, nil
}

// authenticateWithProvider handles provider authentication.
func (m *manager) authenticateWithProvider(ctx context.Context, providerName string) (types.ICredentials, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "authenticateWithProvider", "")
		return nil, errUtils.ErrInvalidAuthConfig
	}

	log.Debug("Authenticating with provider", "provider", providerName)
	credentials, err := provider.Authenticate(ctx)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrAuthenticationFailed, "authenticateWithProvider", "")
		return nil, errUtils.ErrAuthenticationFailed
	}

	// Cache provider credentials
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
	bold := lipgloss.NewStyle().Bold(true)

	log.Debug("Authenticating identity chain", "chainLength", len(m.chain), "startIndex", startIndex, "chain", m.chain)

	currentCreds := initialCreds

	// Step 2: Authenticate through identity chain starting from startIndex
	for i := startIndex; i < len(m.chain); i++ {
		identityStep := m.chain[i]
		identity, exists := m.identities[identityStep]
		if !exists {
			errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "authenticateIdentityChain", fmt.Sprintf("identity %q not found in chain step %d", identityStep, i))
			return nil, errUtils.ErrInvalidAuthConfig
		}

		log.Debug("Authenticating identity step", "step", i, logKeyIdentity, identityStep, "kind", identity.Kind())

		// Each identity receives credentials from the previous step
		nextCreds, err := identity.Authenticate(ctx, currentCreds)
		if err != nil {
			return nil, fmt.Errorf("identity %q authentication failed at chain step %d: %w", identityStep, i, err)
		}

		currentCreds = nextCreds

		// Cache credentials for this level
		if err := m.credentialStore.Store(identityStep, currentCreds); err != nil {
			log.Debug("Failed to cache credentials", "identityStep", identityStep, "error", err)
		} else {
			log.Debug("Cached credentials", "identityStep", identityStep)
		}

		log.Info("Chained identity", "from", bold.Render(m.getChainStepName(i-1)), "to", bold.Render(identityStep))
	}

	return currentCreds, nil
}

// buildAuthenticationChain builds the authentication chain from target identity to source provider
// Returns a slice where [0] is the provider name, [1..n] are identity names in authentication order.
func (m *manager) buildAuthenticationChain(identityName string) ([]string, error) {
	var chain []string
	visited := make(map[string]bool)

	// Recursively build the chain
	err := m.buildChainRecursive(identityName, &chain, visited)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, buildAuthenticationChain, fmt.Sprintf("failed to build authentication chain for identity %q: %v", identityName, err))
		return nil, errUtils.ErrInvalidAuthConfig
	}

	// Reverse the chain so provider is first, then identities in authentication order
	for i := 0; i < len(chain)/2; i++ {
		j := len(chain) - 1 - i
		chain[i], chain[j] = chain[j], chain[i]
	}

	return chain, nil
}

// buildChainRecursive recursively builds the authentication chain.
func (m *manager) buildChainRecursive(identityName string, chain *[]string, visited map[string]bool) error {
	// Check for circular dependencies
	if visited[identityName] {
		errUtils.CheckErrorAndPrint(ErrCircularDependency, buildChainRecursive, fmt.Sprintf("circular dependency detected in identity chain involving %q", identityName))
		return ErrCircularDependency
	}
	visited[identityName] = true

	// Find the identity
	identity, exists := m.config.Identities[identityName]
	if !exists {
		// Try to find by alias
		for name, ident := range m.config.Identities {
			if ident.Alias == identityName {
				identity = ident
				identityName = name // Use the actual name for the chain
				exists = true
				break
			}
		}
	}

	if !exists {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, buildChainRecursive, fmt.Sprintf("identity %q not found", identityName))
		return errUtils.ErrInvalidAuthConfig
	}

	// AWS User identities don't require via configuration - they are standalone
	if identity.Via == nil {
		if identity.Kind == "aws/user" {
			// AWS User is standalone - just add it to the chain and return
			*chain = append(*chain, identityName)
			return nil
		}
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidIdentityConfig, buildChainRecursive, fmt.Sprintf("identity %q has no via configuration", identityName))
		return errUtils.ErrInvalidIdentityConfig
	}

	// Add current identity to chain
	*chain = append(*chain, identityName)

	// If this identity points to a provider, add it and stop
	if identity.Via.Provider != "" {
		*chain = append(*chain, identity.Via.Provider)
		return nil
	}

	// If this identity points to another identity, recurse
	if identity.Via.Identity != "" {
		return m.buildChainRecursive(identity.Via.Identity, chain, visited)
	}

	errUtils.CheckErrorAndPrint(errUtils.ErrInvalidIdentityConfig, buildChainRecursive, fmt.Sprintf("identity %q has invalid via configuration", identityName))
	return errUtils.ErrInvalidIdentityConfig
}

// buildWhoamiInfo creates a WhoamiInfo struct from identity and credentials.
func (m *manager) buildWhoamiInfo(identityName string, creds types.ICredentials) *types.WhoamiInfo {
	providerName := m.getProviderForIdentity(identityName)

	info := &types.WhoamiInfo{
		Provider:    providerName,
		Identity:    identityName,
		LastUpdated: time.Now(),
	}

	// Populate high-level fields from the concrete credential type
	info.Credentials = creds
	creds.BuildWhoamiInfo(info)
	if expTime, err := creds.GetExpiration(); err == nil && expTime != nil {
		info.Expiration = expTime
	}
	// Get environment variables
	if identity, exists := m.identities[identityName]; exists {
		if env, err := identity.Environment(); err == nil {
			info.Environment = env
		}
	}

	// Store credentials in the keystore and set a reference handle
	// Use the identity name as the opaque handle for retrieval. Only clear
	// in-memory credentials if storage succeeded to avoid losing access.
	if err := m.credentialStore.Store(identityName, creds); err == nil {
		info.CredentialsRef = identityName
		// Clear raw credentials to avoid accidental serialization of secrets
		info.Credentials = nil
	}

	return info
}
