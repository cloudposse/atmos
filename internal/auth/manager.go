package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/auth/identities/aws"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// manager implements the AuthManager interface
type manager struct {
	config          *schema.AuthConfig
	providers       map[string]types.Provider
	identities      map[string]types.Identity
	credentialStore types.CredentialStore
	awsFileManager  types.AWSFileManager
	validator       types.Validator
}

// NewAuthManager creates a new AuthManager instance
func NewAuthManager(
	config *schema.AuthConfig,
	credentialStore types.CredentialStore,
	awsFileManager types.AWSFileManager,
	validator types.Validator,
) (types.AuthManager, error) {
	if config == nil {
		return nil, fmt.Errorf("auth config cannot be nil")
	}

	m := &manager{
		config:          config,
		providers:       make(map[string]types.Provider),
		identities:      make(map[string]types.Identity),
		credentialStore: credentialStore,
		awsFileManager:  awsFileManager,
		validator:       validator,
	}

	// Initialize providers
	if err := m.initializeProviders(); err != nil {
		return nil, fmt.Errorf("failed to initialize providers: %w", err)
	}

	// Initialize identities
	if err := m.initializeIdentities(); err != nil {
		return nil, fmt.Errorf("failed to initialize identities: %w", err)
	}

	return m, nil
}

// Authenticate performs hierarchical authentication for the specified identity
func (m *manager) Authenticate(ctx context.Context, identityName string) (*schema.WhoamiInfo, error) {
	log.SetPrefix("[atmos-auth]")
	defer log.SetPrefix("")
	// If no identity specified, use default
	if identityName == "" {
		defaultIdentity, err := m.GetDefaultIdentity()
		if err != nil {
			return nil, fmt.Errorf("no identity specified and no default identity found: %w", err)
		}
		identityName = defaultIdentity
	}

	// Build the complete authentication chain
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		return nil, fmt.Errorf("failed to build authentication chain for identity %q: %w", identityName, err)
	}

	log.Debug("Authentication chain discovered", "identity", identityName, "chainLength", len(chain), "chain", chain)

	// Perform hierarchical credential validation (bottom-up)
	finalCreds, err := m.authenticateHierarchical(ctx, chain, identityName)
	if err != nil {
		return nil, fmt.Errorf("hierarchical authentication failed: %w", err)
	}

	// Setup AWS files if this is an AWS provider (chain[0] is always the provider name)
	providerName := chain[0]
	provider, exists := m.providers[providerName]
	if exists && isAWSProvider(provider.Kind()) {
		if err := m.SetupAWSFiles(ctx, providerName, identityName, finalCreds); err != nil {
			return nil, fmt.Errorf("failed to setup AWS files: %w", err)
		}
	}

	return m.buildWhoamiInfo(identityName, finalCreds), nil
}

// Whoami returns information about the specified identity's credentials
func (m *manager) Whoami(ctx context.Context, identityName string) (*schema.WhoamiInfo, error) {
	// Try to retrieve credentials for this specific identity
	creds, err := m.credentialStore.Retrieve(identityName)
	if err != nil {
		return nil, fmt.Errorf("no credentials found for identity %q: %w", identityName, err)
	}

	// Check if credentials are expired
	if expired, err := m.credentialStore.IsExpired(identityName); err != nil || expired {
		return nil, fmt.Errorf("credentials for identity %q are expired or invalid", identityName)
	}

	return m.buildWhoamiInfo(identityName, creds), nil
}

// Validate validates the entire auth configuration
func (m *manager) Validate() error {
	return m.validator.ValidateAuthConfig(m.config)
}

// SetupAWSFiles writes AWS credentials and config files for the specified identity
func (m *manager) SetupAWSFiles(ctx context.Context, providerName, identityName string, creds *schema.Credentials) error {
	if creds.AWS == nil {
		return fmt.Errorf("no AWS credentials found")
	}

	// Write credentials file to provider directory with identity profile
	if err := m.awsFileManager.WriteCredentials(providerName, identityName, creds.AWS); err != nil {
		return fmt.Errorf("failed to write AWS credentials: %w", err)
	}

	// Write config file to provider directory with identity profile
	region := creds.AWS.Region
	if region == "" {
		// For AWS user identities, get region from identity credentials config
		if providerName == "aws-user" {
			if identity, exists := m.config.Identities[identityName]; exists {
				if r, ok := identity.Credentials["region"].(string); ok && r != "" {
					region = r
				}
			}
		}
		// Fallback to provider config
		if region == "" {
			if provider, exists := m.config.Providers[providerName]; exists {
				region = provider.Region
			}
		}
	}
	if err := m.awsFileManager.WriteConfig(providerName, identityName, region, ""); err != nil {
		return fmt.Errorf("failed to write AWS config: %w", err)
	}

	// Set environment variables using provider name for file paths
	if err := m.awsFileManager.SetEnvironmentVariables(providerName); err != nil {
		return fmt.Errorf("failed to set AWS environment variables: %w", err)
	}

	return nil
}

// GetDefaultIdentity returns the name of the default identity, if any
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
			return "", fmt.Errorf("no default identity configured")
		}
		// In interactive mode, prompt user to choose from all identities
		return m.promptForIdentity("No default identity configured. Please choose an identity:", m.ListIdentities())

	case 1:
		// Exactly one default identity found - use it
		return defaultIdentities[0], nil

	default:
		// Multiple default identities found
		if telemetry.IsCI() {
			return "", fmt.Errorf("multiple default identities found: %v", defaultIdentities)
		}
		// In interactive mode, prompt user to choose from default identities
		return m.promptForIdentity("Multiple default identities found. Please choose one:", defaultIdentities)
	}
}

// promptForIdentity prompts the user to select an identity from the given list
func (m *manager) promptForIdentity(message string, identities []string) (string, error) {
	if len(identities) == 0 {
		return "", fmt.Errorf("no identities available")
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
		return "", fmt.Errorf("failed to select identity: %w", err)
	}

	return selectedIdentity, nil
}

// ListIdentities returns all available identity names
func (m *manager) ListIdentities() []string {
	var names []string
	for name := range m.config.Identities {
		names = append(names, name)
	}
	return names
}

// ListProviders returns all available provider names
func (m *manager) ListProviders() []string {
	var names []string
	for name := range m.config.Providers {
		names = append(names, name)
	}
	return names
}

// initializeProviders creates provider instances from configuration
func (m *manager) initializeProviders() error {
	for name, providerConfig := range m.config.Providers {
		provider, err := NewProvider(name, &providerConfig)
		if err != nil {
			return fmt.Errorf("failed to create provider %q: %w", name, err)
		}
		m.providers[name] = provider
	}
	return nil
}

// initializeIdentities creates identity instances from configuration
func (m *manager) initializeIdentities() error {
	for name, identityConfig := range m.config.Identities {
		identity, err := NewIdentity(name, &identityConfig)
		if err != nil {
			return fmt.Errorf("failed to create identity %q: %w", name, err)
		}
		m.identities[name] = identity
	}
	return nil
}

// getProviderForIdentity returns the provider name for the given identity
// Recursively resolves through identity chains to find the root provider
func (m *manager) getProviderForIdentity(identityName string) string {
	visited := make(map[string]bool)
	return m.getProviderForIdentityRecursive(identityName, visited)
}

// getProviderForIdentityRecursive recursively resolves provider through identity chains
func (m *manager) getProviderForIdentityRecursive(identityName string, visited map[string]bool) string {
	// Check for circular dependencies
	if visited[identityName] {
		return "" // Circular dependency detected
	}
	visited[identityName] = true

	// First try to find by identity name
	identity, exists := m.config.Identities[identityName]
	if !exists {
		// If not found by name, try to find by alias
		for name, ident := range m.config.Identities {
			if ident.Alias == identityName {
				identity = ident
				exists = true
				// Update visited with the actual identity name to prevent cycles
				visited[name] = true
				break
			}
		}
	}

	if !exists {
		return ""
	}

	if identity.Via == nil {
		return ""
	}

	// If this identity points to a provider, return it
	if identity.Via.Provider != "" {
		return identity.Via.Provider
	}

	// If this identity points to another identity, recurse
	if identity.Via.Identity != "" {
		return m.getProviderForIdentityRecursive(identity.Via.Identity, visited)
	}

	return ""
}

// GetProviderForIdentity returns the provider name for the given identity
// Recursively resolves through identity chains to find the root provider
func (m *manager) GetProviderForIdentity(identityName string) string {
	return m.getProviderForIdentity(identityName)
}

// authenticateHierarchical performs hierarchical authentication with bottom-up validation
func (m *manager) authenticateHierarchical(ctx context.Context, chain []string, targetIdentity string) (*schema.Credentials, error) {
	// Step 1: Bottom-up validation - check cached credentials from target to root
	validFromIndex := m.findFirstValidCachedCredentials(chain, targetIdentity)

	if validFromIndex == -1 {
		log.Debug("No valid cached credentials found in chain, full authentication required")
	} else {
		log.Debug("Found valid cached credentials", "validFromIndex", validFromIndex, "chainStep", getChainStepName(chain, validFromIndex))

		// If target identity has valid cached credentials, use them
		if validFromIndex == len(chain)-1 {
			if cachedCreds, err := m.credentialStore.Retrieve(targetIdentity); err == nil {
				log.Debug("Using cached credentials for target identity", "identity", targetIdentity)
				return cachedCreds, nil
			}
		}
	}

	// Step 2: Selective re-authentication from first invalid point down to target
	return m.authenticateFromIndex(ctx, chain, validFromIndex, targetIdentity)
}

// findFirstValidCachedCredentials checks cached credentials from bottom to top of chain
// Returns the index of the first valid cached credentials, or -1 if none found
func (m *manager) findFirstValidCachedCredentials(chain []string, targetIdentity string) int {
	// Check from target identity (bottom) up to provider (top)
	for i := len(chain) - 1; i >= 0; i-- {
		identityName := chain[i]

		// Check if we have cached credentials for this level
		if cachedCreds, err := m.credentialStore.Retrieve(identityName); err == nil {
			// Check if credentials are still valid (>5 minutes remaining)
			if expired, err := m.credentialStore.IsExpired(identityName); err == nil && !expired {
				// Additional check for AWS credentials expiration
				if cachedCreds.AWS != nil && cachedCreds.AWS.Expiration != "" {
					if expTime, err := time.Parse(time.RFC3339, cachedCreds.AWS.Expiration); err == nil {
						if expTime.After(time.Now().Add(5 * time.Minute)) {
							log.Debug("Found valid cached credentials", "chainIndex", i, "identityName", identityName, "expiration", expTime)
							return i
						}
					}
				} else {
					// Non-AWS credentials or no expiration info, assume valid
					log.Debug("Found valid cached credentials (non-AWS)", "chainIndex", i, "identityName", identityName)
					return i
				}
			}
		}
	}
	return -1 // No valid cached credentials found
}

// authenticateFromIndex performs authentication starting from the given index in the chain
func (m *manager) authenticateFromIndex(ctx context.Context, chain []string, startIndex int, targetIdentity string) (*schema.Credentials, error) {
	// Handle special case: standalone AWS user identity
	if aws.IsStandaloneAWSUserChain(chain, m.config.Identities) {
		return aws.AuthenticateStandaloneAWSUser(ctx, chain[0], m.identities)
	}

	// Handle regular provider-based authentication chains
	return m.authenticateProviderChain(ctx, chain, startIndex, targetIdentity)
}

// authenticateProviderChain handles authentication for provider-based identity chains
func (m *manager) authenticateProviderChain(ctx context.Context, chain []string, startIndex int, targetIdentity string) (*schema.Credentials, error) {
	var currentCreds *schema.Credentials
	var err error

	// Determine actual starting point for authentication
	actualStartIndex := m.determineStartingIndex(chain, startIndex)

	// Retrieve cached credentials if starting from a cached point
	if actualStartIndex > 0 {
		currentCreds, err = m.retrieveCachedCredentials(chain, startIndex)
		if err != nil {
			log.Debug("Failed to retrieve cached credentials, starting from provider", "error", err)
			actualStartIndex = 0
		}
	}

	// Step 1: Authenticate with provider if needed
	if actualStartIndex == 0 {
		currentCreds, err = m.authenticateWithProvider(ctx, chain[0])
		if err != nil {
			return nil, err
		}
		actualStartIndex = 1
	}

	// Step 2: Authenticate through identity chain
	return m.authenticateIdentityChain(ctx, chain, actualStartIndex, currentCreds)
}

// determineStartingIndex determines where to start authentication based on cached credentials
func (m *manager) determineStartingIndex(chain []string, startIndex int) int {
	if startIndex == -1 {
		return 0 // Start from provider if no valid cached credentials
	}
	return startIndex
}

// retrieveCachedCredentials retrieves cached credentials from the specified starting point
func (m *manager) retrieveCachedCredentials(chain []string, startIndex int) (*schema.Credentials, error) {
	identityName := chain[startIndex]
	currentCreds, err := m.credentialStore.Retrieve(identityName)
	if err != nil {
		return nil, err
	}

	log.Debug("Starting authentication from cached credentials", "startIndex", startIndex)
	return currentCreds, nil
}

// authenticateWithProvider handles provider authentication
func (m *manager) authenticateWithProvider(ctx context.Context, providerName string) (*schema.Credentials, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return nil, fmt.Errorf("provider %q not found", providerName)
	}

	log.Debug("Authenticating with provider", "provider", providerName)
	credentials, err := provider.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("provider %q authentication failed: %w", providerName, err)
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

// Helper functions for logging
func getChainStepName(chain []string, index int) string {
	if index < len(chain) {
		return chain[index]
	}
	return "unknown"
}

func getPreviousStepName(chain []string, currentIndex int) string {
	if currentIndex > 0 && currentIndex <= len(chain) {
		return chain[currentIndex-1]
	}
	return "unknown"
}

// authenticateIdentityChain performs sequential authentication through an identity chain
func (m *manager) authenticateIdentityChain(ctx context.Context, chain []string, startIndex int, initialCreds *schema.Credentials) (*schema.Credentials, error) {
	bold := lipgloss.NewStyle().Bold(true)

	log.Debug("Authenticating identity chain", "chainLength", len(chain), "startIndex", startIndex, "chain", chain)

	currentCreds := initialCreds

	// Step 2: Authenticate through identity chain starting from startIndex
	for i := startIndex; i < len(chain); i++ {
		identityStep := chain[i]
		identity, exists := m.identities[identityStep]
		if !exists {
			log.Error("❌ Chaining identity %s → %s", bold.Render(getChainStepName(chain, i-1)), bold.Render(identityStep))
			return nil, fmt.Errorf("identity %q not found in chain step %d", identityStep, i)
		}

		log.Debug("Authenticating identity step", "step", i, "identity", identityStep, "kind", identity.Kind())

		// Each identity receives credentials from the previous step
		nextCreds, err := identity.Authenticate(ctx, currentCreds)
		if err != nil {
			log.Error("❌ Chaining identity %s → %s", bold.Render(getChainStepName(chain, i-1)), bold.Render(identityStep))
			return nil, fmt.Errorf("identity %q authentication failed at chain step %d: %w", identityStep, i, err)
		}

		currentCreds = nextCreds

		// Cache credentials for this level
		if err := m.credentialStore.Store(identityStep, currentCreds); err != nil {
			log.Debug("Failed to cache credentials", "identityStep", identityStep, "error", err)
		} else {
			log.Debug("Cached credentials", "identityStep", identityStep)
		}

		log.Infof("✅ Chaining identity %s → %s", bold.Render(getChainStepName(chain, i-1)), bold.Render(identityStep))
	}

	return currentCreds, nil
}

// buildAuthenticationChain builds the authentication chain from target identity to source provider
// Returns a slice where [0] is the provider name, [1..n] are identity names in authentication order
func (m *manager) buildAuthenticationChain(identityName string) ([]string, error) {
	var chain []string
	visited := make(map[string]bool)

	// Recursively build the chain
	err := m.buildChainRecursive(identityName, &chain, visited)
	if err != nil {
		return nil, err
	}

	// Reverse the chain so provider is first, then identities in authentication order
	for i := 0; i < len(chain)/2; i++ {
		j := len(chain) - 1 - i
		chain[i], chain[j] = chain[j], chain[i]
	}

	return chain, nil
}

// buildChainRecursive recursively builds the authentication chain
func (m *manager) buildChainRecursive(identityName string, chain *[]string, visited map[string]bool) error {
	// Check for circular dependencies
	if visited[identityName] {
		return fmt.Errorf("circular dependency detected in identity chain involving %q", identityName)
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
		return fmt.Errorf("identity %q not found", identityName)
	}

	// AWS User identities don't require via configuration - they are standalone
	if identity.Via == nil {
		if identity.Kind == "aws/user" {
			// AWS User is standalone - just add it to the chain and return
			*chain = append(*chain, identityName)
			return nil
		}
		return fmt.Errorf("identity %q has no via configuration", identityName)
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

	return fmt.Errorf("identity %q has invalid via configuration", identityName)
}

// buildWhoamiInfo creates a WhoamiInfo struct from identity and credentials
func (m *manager) buildWhoamiInfo(identityName string, creds *schema.Credentials) *schema.WhoamiInfo {
	providerName := m.getProviderForIdentity(identityName)

	info := &schema.WhoamiInfo{
		Provider:    providerName,
		Identity:    identityName,
		Credentials: creds,
		LastUpdated: time.Now(),
	}

	// Extract additional info from AWS credentials
	if creds.AWS != nil {
		info.Region = creds.AWS.Region
		if creds.AWS.Expiration != "" {
			if expTime, err := time.Parse(time.RFC3339, creds.AWS.Expiration); err == nil {
				info.Expiration = &expTime
			}
		}
	}

	// Get environment variables
	if identity, exists := m.identities[identityName]; exists {
		if env, err := identity.Environment(); err == nil {
			info.Environment = env
		}
	}

	return info
}

// extractIdentityFromAlias extracts the identity name from an alias (format: provider/identity)
func extractIdentityFromAlias(alias string) string {
	for i := len(alias) - 1; i >= 0; i-- {
		if alias[i] == '/' {
			return alias[i+1:]
		}
	}
	return alias
}

// isAWSProvider checks if the provider kind is AWS-related
func isAWSProvider(kind string) bool {
	return kind == "aws/iam-identity-center" || kind == "aws/assume-role"
}
