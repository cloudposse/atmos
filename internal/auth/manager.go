package auth

import (
	"context"
	"fmt"
	"time"

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

// Authenticate performs authentication for the specified identity
func (m *manager) Authenticate(ctx context.Context, identityName string) (*schema.WhoamiInfo, error) {
	// If no identity specified, use default
	if identityName == "" {
		defaultIdentity, err := m.GetDefaultIdentity()
		if err != nil {
			return nil, fmt.Errorf("no identity specified and no default identity found: %w", err)
		}
		identityName = defaultIdentity
	}

	// Get the identity
	identity, exists := m.identities[identityName]
	if !exists {
		return nil, fmt.Errorf("identity %q not found", identityName)
	}

	// Check if we have cached, non-expired credentials
	alias := fmt.Sprintf("%s/%s", m.getProviderForIdentity(identityName), identityName)
	if cachedCreds, err := m.credentialStore.Retrieve(alias); err == nil {
		if expired, err := m.credentialStore.IsExpired(alias); err == nil && !expired {
			return m.buildWhoamiInfo(identityName, cachedCreds), nil
		}
	}

	// Get base credentials from provider
	providerName := m.getProviderForIdentity(identityName)
	provider, exists := m.providers[providerName]
	if !exists {
		return nil, fmt.Errorf("provider %q not found for identity %q", providerName, identityName)
	}

	baseCreds, err := provider.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("provider authentication failed: %w", err)
	}

	// Use identity to get final credentials
	finalCreds, err := identity.Authenticate(ctx, baseCreds)
	if err != nil {
		return nil, fmt.Errorf("identity authentication failed: %w", err)
	}

	// Store credentials
	if err := m.credentialStore.Store(alias, finalCreds); err != nil {
		return nil, fmt.Errorf("failed to store credentials: %w", err)
	}

	// Setup AWS files if this is an AWS provider
	if isAWSProvider(provider.Kind()) {
		if err := m.SetupAWSFiles(ctx, providerName, finalCreds); err != nil {
			return nil, fmt.Errorf("failed to setup AWS files: %w", err)
		}
	}

	return m.buildWhoamiInfo(identityName, finalCreds), nil
}

// Whoami returns information about the current effective principal
func (m *manager) Whoami(ctx context.Context) (*schema.WhoamiInfo, error) {
	// Since keyring doesn't support listing, we'll check each configured identity
	// to find the most recent valid credentials
	var mostRecentInfo *schema.WhoamiInfo
	var mostRecentTime time.Time

	for identityName := range m.config.Identities {
		providerName := m.getProviderForIdentity(identityName)
		if providerName == "" {
			continue
		}

		alias := fmt.Sprintf("%s/%s", providerName, identityName)

		// Try to retrieve credentials for this identity
		creds, err := m.credentialStore.Retrieve(alias)
		if err != nil {
			continue // No credentials stored for this identity
		}

		// Check if credentials are expired
		if expired, err := m.credentialStore.IsExpired(alias); err != nil || expired {
			continue // Credentials are expired or can't check expiration
		}

		// Parse expiration time to find the most recent
		if creds.AWS != nil && creds.AWS.Expiration != "" {
			if expTime, err := time.Parse(time.RFC3339, creds.AWS.Expiration); err == nil {
				if expTime.After(mostRecentTime) {
					mostRecentTime = expTime
					mostRecentInfo = m.buildWhoamiInfo(identityName, creds)
				}
			}
		}
	}

	if mostRecentInfo == nil {
		return nil, fmt.Errorf("no active authentication session found")
	}

	return mostRecentInfo, nil
}

// Validate validates the entire auth configuration
func (m *manager) Validate() error {
	return m.validator.ValidateAuthConfig(m.config)
}

// SetupAWSFiles writes AWS credentials and config files for the specified provider
func (m *manager) SetupAWSFiles(ctx context.Context, providerName string, creds *schema.Credentials) error {
	if creds.AWS == nil {
		return fmt.Errorf("no AWS credentials found")
	}

	// Write credentials file
	if err := m.awsFileManager.WriteCredentials(providerName, creds.AWS); err != nil {
		return fmt.Errorf("failed to write AWS credentials: %w", err)
	}

	// Write config file
	region := creds.AWS.Region
	if region == "" {
		// Get region from provider config
		if provider, exists := m.config.Providers[providerName]; exists {
			region = provider.Region
		}
	}
	if err := m.awsFileManager.WriteConfig(providerName, region); err != nil {
		return fmt.Errorf("failed to write AWS config: %w", err)
	}

	// Set environment variables
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
