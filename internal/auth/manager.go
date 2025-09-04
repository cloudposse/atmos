package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// manager implements the AuthManager interface
type manager struct {
	config          *schema.AuthConfig
	providers       map[string]types.Provider
	identities      map[string]types.Identity
	credentialStore types.CredentialStore
	awsFileManager  types.AWSFileManager
	configMerger    types.ConfigMerger
	validator       types.Validator
}

// NewAuthManager creates a new AuthManager instance
func NewAuthManager(
	config *schema.AuthConfig,
	credentialStore types.CredentialStore,
	awsFileManager types.AWSFileManager,
	configMerger types.ConfigMerger,
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
		configMerger:    configMerger,
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
		// Get region from provider config
		if provider, exists := m.config.Providers[providerName]; exists {
			region = provider.Region
		}
	}
	if err := m.awsFileManager.WriteConfig(providerName, identityName, region); err != nil {
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
	for name, identity := range m.config.Identities {
		if identity.Default {
			return name, nil
		}
	}
	return "", fmt.Errorf("no default identity configured")
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
	bold := lipgloss.NewStyle().Bold(true)
	var currentCreds *schema.Credentials
	var err error

	// Determine starting point
	actualStartIndex := startIndex
	if startIndex == -1 {
		actualStartIndex = 0 // Start from provider if no valid cached credentials
	}

	// If we have valid cached credentials, retrieve them as starting point
	if startIndex > 0 {
		identityName := chain[startIndex]
		currentCreds, err = m.credentialStore.Retrieve(identityName)
		if err != nil {
			log.Debug("Failed to retrieve cached credentials, starting from provider", "error", err)
			actualStartIndex = 0
		} else {
			log.Debug("Starting authentication from cached credentials", "startIndex", startIndex)
			actualStartIndex = startIndex + 1 // Start from next step after cached credentials
		}
	}

	// Step 1: Authenticate with provider if needed
	if actualStartIndex == 0 {
		providerName := chain[0]
		provider, exists := m.providers[providerName]
		if !exists {
			return nil, fmt.Errorf("provider %q not found", providerName)
		}

		log.Debug("Authenticating with provider", "provider", providerName)
		currentCreds, err = provider.Authenticate(ctx)
		if err != nil {
			return nil, fmt.Errorf("provider %q authentication failed: %w", providerName, err)
		}

		// Cache provider credentials
		if err := m.credentialStore.Store(providerName, currentCreds); err != nil {
			log.Debug("Failed to cache provider credentials", "error", err)
		} else {
			log.Debug("Cached provider credentials", "providerName", providerName)
		}

		log.Debug("Provider authenticated", "provider", providerName)
		actualStartIndex = 1
	}

	// Step 2: Authenticate through identity chain
	for i := actualStartIndex; i < len(chain); i++ {
		identityStep := chain[i]
		identity, exists := m.identities[identityStep]
		if !exists {
			log.Error("❌ Chaining identity %s → %s", bold.Render(getPreviousStepName(chain, i)), bold.Render(identityStep))
			return nil, fmt.Errorf("identity %q not found in chain step %d", identityStep, i)
		}

		log.Debug("Authenticating identity step", "step", i, "identity", identityStep, "kind", identity.Kind())

		// Each identity receives credentials from the previous step
		nextCreds, err := identity.Authenticate(ctx, currentCreds)
		if err != nil {
			log.Error("❌ Chaining identity %s → %s", bold.Render(getPreviousStepName(chain, i)), bold.Render(identityStep))
			return nil, fmt.Errorf("identity %q authentication failed at chain step %d: %w", identityStep, i, err)
		}

		currentCreds = nextCreds

		// Cache credentials for this level
		if err := m.credentialStore.Store(identityStep, currentCreds); err != nil {
			log.Debug("Failed to cache credentials", "identityStep", identityStep, "error", err)
		} else {
			log.Debug("Cached credentials", "identityStep", identityStep)
		}

		log.Infof("✅ Chaining identity %s → %s", bold.Render(getPreviousStepName(chain, i)), bold.Render(identityStep))
	}

	return currentCreds, nil
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
func (m *manager) authenticateIdentityChain(ctx context.Context, identityName string) (*schema.Credentials, error) {
	bold := lipgloss.NewStyle().Bold(true)

	// Build the authentication chain from target identity back to source provider
	chain, err := m.buildAuthenticationChain(identityName)

	if err != nil {
		return nil, fmt.Errorf("failed to build authentication chain for identity %q: %w", identityName, err)
	}

	log.Debug("Authentication chain built", "identity", identityName, "chainLength", len(chain), "chain", chain)

	var currentCreds *schema.Credentials
	var startIndex int

	// Check if this is an AWS User identity (standalone, no provider)
	if len(chain) == 1 {
		// Single identity in chain - check if it's AWS User
		identityName := chain[0]
		if identity, exists := m.config.Identities[identityName]; exists && identity.Kind == "aws/user" {
			// AWS User identity - authenticate directly without provider
			identityInstance, exists := m.identities[identityName]
			if !exists {
				return nil, fmt.Errorf("AWS User identity %q not found", identityName)
			}

			// AWS User identities authenticate with nil credentials (no provider chain)
			currentCreds, err = identityInstance.Authenticate(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("AWS User identity %q authentication failed: %w", identityName, err)
			}

			log.Debug("AWS User authenticated directly", "identity", identityName, "hasAWSCreds", currentCreds.AWS != nil)
			return currentCreds, nil
		}
	}

	// Standard provider-based authentication chain
	// Step 1: Authenticate with the source provider
	providerName := chain[0]
	provider, exists := m.providers[providerName]
	if !exists {
		return nil, fmt.Errorf("provider %q not found", providerName)
	}

	currentCreds, err = provider.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("provider %q authentication failed: %w", providerName, err)
	}

	log.Debug("Provider authenticated", "provider", providerName, "hasAWSCreds", currentCreds.AWS != nil)
	startIndex = 1

	// Step 2: Authenticate through each identity in the chain
	for i := startIndex; i < len(chain); i++ {
		identityStep := chain[i]
		identity, exists := m.identities[identityStep]
		if !exists {
			log.Error("❌ Chaining identity %s → %s", bold.Render(chain[i-1]), bold.Render(identityStep))
			return nil, fmt.Errorf("identity %q not found in chain step %d", identityStep, i)
		}

		log.Debug("Authenticating identity step", "step", i, "identity", identityStep, "kind", identity.Kind())

		// Each identity receives credentials from the previous step
		nextCreds, err := identity.Authenticate(ctx, currentCreds)
		if err != nil {
			log.Error("❌ Chaining identity %s → %s", bold.Render(chain[i-1]), bold.Render(identityStep))
			return nil, fmt.Errorf("identity %q authentication failed at chain step %d: %w", identityStep, i, err)
		}

		currentCreds = nextCreds
		log.Infof("✅ Chaining identity %s → %s", bold.Render(chain[i-1]), bold.Render(identityStep))
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
	return kind == "aws/iam-identity-center" || kind == "aws/assume-role" || kind == "aws/user"
}
