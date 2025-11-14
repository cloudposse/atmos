package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Default client ID for Atmos Azure authentication (Azure CLI public client).
	defaultAzureClientID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46"

	// Default timeout for device code authentication.
	deviceCodeTimeout = 15 * time.Minute
)

// isInteractive checks if we're running in an interactive terminal.
// For device code flow, we need stderr to be a TTY so the user can see the authentication URL.
func isInteractive() bool {
	return term.IsTTYSupportForStderr()
}

// deviceCodeProvider implements Azure Entra ID device code authentication.
type deviceCodeProvider struct {
	name           string
	config         *schema.Provider
	tenantID       string
	subscriptionID string
	location       string
	clientID       string
	cacheStorage   CacheStorage
}

// deviceCodeConfig holds extracted Azure configuration from provider spec.
type deviceCodeConfig struct {
	TenantID       string
	SubscriptionID string
	Location       string
	ClientID       string
}

// extractDeviceCodeConfig extracts Azure config from provider spec.
func extractDeviceCodeConfig(spec map[string]interface{}) deviceCodeConfig {
	config := deviceCodeConfig{
		ClientID: defaultAzureClientID, // Default value.
	}

	if spec == nil {
		return config
	}

	if tid, ok := spec["tenant_id"].(string); ok {
		config.TenantID = tid
	}
	if sid, ok := spec["subscription_id"].(string); ok {
		config.SubscriptionID = sid
	}
	if loc, ok := spec["location"].(string); ok {
		config.Location = loc
	}
	if cid, ok := spec["client_id"].(string); ok && cid != "" {
		config.ClientID = cid
	}

	return config
}

// NewDeviceCodeProvider creates a new Azure device code provider.
func NewDeviceCodeProvider(name string, config *schema.Provider) (*deviceCodeProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "azure/device-code" {
		return nil, fmt.Errorf("%w: invalid provider kind for Azure device code provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract Azure-specific config from Spec.
	cfg := extractDeviceCodeConfig(config.Spec)

	// Tenant ID is required.
	if cfg.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required in spec for Azure device code provider", errUtils.ErrInvalidProviderConfig)
	}

	return &deviceCodeProvider{
		name:           name,
		config:         config,
		tenantID:       cfg.TenantID,
		subscriptionID: cfg.SubscriptionID,
		location:       cfg.Location,
		clientID:       cfg.ClientID,
		cacheStorage:   &defaultCacheStorage{},
	}, nil
}

// Kind returns the provider kind.
func (p *deviceCodeProvider) Kind() string {
	return "azure/device-code"
}

// Name returns the configured provider name.
func (p *deviceCodeProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for device code provider.
func (p *deviceCodeProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// createMSALClient creates a MSAL public client with persistent token cache.
// The cache is stored in ~/.azure/msal_token_cache.json for Azure CLI compatibility.
func (p *deviceCodeProvider) createMSALClient() (public.Client, error) {
	// Create MSAL cache for token persistence.
	msalCache, err := azureCloud.NewMSALCache("")
	if err != nil {
		return public.Client{}, fmt.Errorf("failed to create MSAL cache: %w", err)
	}

	// Create MSAL public client with cache.
	// This client will automatically persist refresh tokens.
	client, err := public.New(
		p.clientID,
		public.WithAuthority(fmt.Sprintf("https://login.microsoftonline.com/%s", p.tenantID)),
		public.WithCache(msalCache),
	)
	if err != nil {
		return public.Client{}, fmt.Errorf("failed to create MSAL client: %w", err)
	}

	log.Debug("Created MSAL client",
		"clientID", p.clientID,
		azureCloud.LogFieldTenantID, p.tenantID)

	return client, nil
}

// acquireTokenByDeviceCode performs device code authentication flow using MSAL.
// It displays the device code to the user and waits for authentication to complete.
func (p *deviceCodeProvider) acquireTokenByDeviceCode(ctx context.Context, client *public.Client, scopes []string) (string, time.Time, error) {
	// Create a context with timeout.
	authCtx, cancel := context.WithTimeout(ctx, deviceCodeTimeout)
	defer cancel()

	// Start device code flow.
	deviceCode, err := client.AcquireTokenByDeviceCode(authCtx, scopes)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: failed to start device code flow: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Display device code to user.
	displayDeviceCodePrompt(deviceCode.Result.UserCode, deviceCode.Result.VerificationURL)

	// If not a TTY (e.g., piped output or CI environment), use simple polling without spinner.
	if !isInteractive() {
		result, err := deviceCode.AuthenticationResult(authCtx)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("%w: device code authentication failed: %w", errUtils.ErrAuthenticationFailed, err)
		}
		return result.AccessToken, result.ExpiresOn, nil
	}

	// Use spinner for interactive terminals.
	return waitForAuthWithSpinner(authCtx, &deviceCode)
}

// findAccountForTenant finds the account that matches the configured tenant ID.
// Returns the matching account or an error if no match is found.
func (p *deviceCodeProvider) findAccountForTenant(accounts []public.Account) (public.Account, error) {
	if len(accounts) == 0 {
		return public.Account{}, errUtils.ErrAzureNoAccountsInCache
	}

	// Try to find account matching the tenant ID.
	for i := range accounts {
		// Match by tenant ID in the home account ID (format: objectId.tenantId).
		if accounts[i].Realm == p.tenantID {
			log.Debug("Found account matching tenant ID",
				"username", accounts[i].PreferredUsername,
				azureCloud.LogFieldTenantID, p.tenantID)
			return accounts[i], nil
		}
	}

	return public.Account{}, fmt.Errorf("%w: %s", errUtils.ErrAzureNoAccountForTenant, p.tenantID)
}

// Authenticate performs Azure device code authentication using MSAL.
// This implementation uses MSAL directly to enable refresh token persistence,
// making it a true drop-in replacement for `az login`.
func (p *deviceCodeProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.deviceCodeProvider.Authenticate")()

	// Create MSAL client with persistent cache.
	// This client automatically manages token caching and refresh tokens.
	client, err := p.createMSALClient()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create MSAL client: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Try silent authentication first (uses cached tokens/refresh tokens).
	accounts, err := client.Accounts(ctx)
	if err != nil {
		log.Debug("Failed to get cached accounts, will proceed with device code flow", "error", err)
	}

	// Try silent token acquisition from cached account.
	tokens := p.trySilentTokenAcquisition(ctx, &client, accounts)

	// If silent acquisition failed, use device code flow.
	if tokens.accessToken == "" {
		tokens, err = p.acquireTokensViaDeviceCode(ctx, &client)
		if err != nil {
			return nil, err
		}
	}

	// Update Azure CLI token cache so Terraform can use it automatically.
	// This makes Atmos auth work exactly like `az login`.
	// Note: MSAL already persisted tokens (including refresh tokens) to ~/.azure/msal_token_cache.json.
	if err := p.updateAzureCLICache(&tokenCacheUpdate{
		AccessToken:       tokens.accessToken,
		ExpiresAt:         tokens.expiresOn,
		GraphToken:        tokens.graphToken,
		GraphExpiresAt:    tokens.graphExpiresOn,
		KeyVaultToken:     tokens.keyVaultToken,
		KeyVaultExpiresAt: tokens.keyVaultExpiresOn,
	}); err != nil {
		log.Debug("Failed to update Azure CLI token cache", "error", err)
	}

	return p.createCredentials(&tokens)
}

// tokenAcquisitionResult holds tokens acquired from Azure.
type tokenAcquisitionResult struct {
	accessToken       string
	graphToken        string
	keyVaultToken     string
	expiresOn         time.Time
	graphExpiresOn    time.Time
	keyVaultExpiresOn time.Time
}

// trySilentTokenAcquisition attempts to acquire tokens silently from cached account.
func (p *deviceCodeProvider) trySilentTokenAcquisition(ctx context.Context, client *public.Client, accounts []public.Account) tokenAcquisitionResult {
	result := tokenAcquisitionResult{}

	if len(accounts) == 0 {
		return result
	}

	// Find the account that matches our configured tenant ID.
	account, err := p.findAccountForTenant(accounts)
	if err != nil {
		log.Debug("No matching account found for tenant, will proceed with device code flow",
			azureCloud.LogFieldTenantID, p.tenantID,
			"error", err)
		return result
	}

	log.Debug("Found cached account, attempting silent token acquisition",
		"account", account.PreferredUsername,
		azureCloud.LogFieldTenantID, p.tenantID)

	// Try to get management token silently.
	mgmtResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://management.azure.com/.default"},
		public.WithSilentAccount(account),
	)
	if err != nil {
		log.Debug("Silent token acquisition failed, will proceed with device code flow", "error", err)
		return result
	}

	result.accessToken = mgmtResult.AccessToken
	result.expiresOn = mgmtResult.ExpiresOn
	log.Debug("Successfully acquired management token silently", "expiresOn", result.expiresOn)

	// Try to get Graph token silently.
	graphResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://graph.microsoft.com/.default"},
		public.WithSilentAccount(account),
	)
	if err == nil {
		result.graphToken = graphResult.AccessToken
		result.graphExpiresOn = graphResult.ExpiresOn
		log.Debug("Successfully acquired Graph token silently", azureCloud.LogFieldExpiresOn, result.graphExpiresOn)
	} else {
		log.Debug("Failed to get Graph token silently, will skip", "error", err)
	}

	// Try to get KeyVault token silently.
	kvResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://vault.azure.net/.default"},
		public.WithSilentAccount(account),
	)
	if err == nil {
		result.keyVaultToken = kvResult.AccessToken
		result.keyVaultExpiresOn = kvResult.ExpiresOn
		log.Debug("Successfully acquired KeyVault token silently", "expiresOn", result.keyVaultExpiresOn)
	} else {
		log.Debug("Failed to get KeyVault token silently, will skip", "error", err)
	}

	return result
}

// acquireTokensViaDeviceCode performs device code flow and acquires additional tokens.
func (p *deviceCodeProvider) acquireTokensViaDeviceCode(ctx context.Context, client *public.Client) (tokenAcquisitionResult, error) {
	result := tokenAcquisitionResult{}

	// Check if we're in a headless environment - device code flow requires user interaction.
	if !isInteractive() {
		return result, fmt.Errorf("%w: Azure device code flow requires an interactive terminal (no TTY detected). Use managed identity or service principal authentication in headless environments", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("Starting Azure device code authentication",
		"provider", p.name,
		"tenant", p.tenantID,
		"clientID", p.clientID,
	)

	// Start device code flow for management scope.
	accessToken, expiresOn, err := p.acquireTokenByDeviceCode(ctx, client,
		[]string{"https://management.azure.com/.default"})
	if err != nil {
		return result, err
	}

	result.accessToken = accessToken
	result.expiresOn = expiresOn
	log.Debug("Authentication successful", "expiration", expiresOn)

	// Get the authenticated account for subsequent silent acquisitions.
	accounts, err := client.Accounts(ctx)
	if err != nil || len(accounts) == 0 {
		log.Debug("Failed to get authenticated account, will skip Graph and KeyVault tokens", "error", err)
		return result, nil
	}

	// Acquire additional API tokens for azuread and azurerm providers.
	p.acquireAdditionalTokens(ctx, client, accounts, &result)

	return result, nil
}

// acquireAdditionalTokens acquires Graph and KeyVault tokens after device code authentication.
func (p *deviceCodeProvider) acquireAdditionalTokens(ctx context.Context, client *public.Client, accounts []public.Account, result *tokenAcquisitionResult) {
	// Find the account that matches our tenant ID.
	account, err := p.findAccountForTenant(accounts)
	if err != nil {
		log.Debug("No matching account found after device code authentication, will skip Graph and KeyVault tokens",
			azureCloud.LogFieldTenantID, p.tenantID,
			"error", err)
		return
	}

	// Request Graph API token for azuread provider (silently, using refresh token).
	log.Debug("Requesting Graph API token for azuread provider")
	graphResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://graph.microsoft.com/.default"},
		public.WithSilentAccount(account),
	)
	if err != nil {
		log.Debug("Failed to get Graph API token, azuread provider may not work", "error", err)
	} else {
		result.graphToken = graphResult.AccessToken
		result.graphExpiresOn = graphResult.ExpiresOn
		log.Debug("Successfully obtained Graph API token",
			azureCloud.LogFieldExpiresOn, result.graphExpiresOn,
			"tokenLength", len(result.graphToken))
	}

	// Request KeyVault token for azurerm provider KeyVault operations (silently).
	log.Debug("Requesting KeyVault token for azurerm provider")
	kvResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://vault.azure.net/.default"},
		public.WithSilentAccount(account),
	)
	if err != nil {
		log.Debug("Failed to get KeyVault token, KeyVault operations may not work", "error", err)
	} else {
		result.keyVaultToken = kvResult.AccessToken
		result.keyVaultExpiresOn = kvResult.ExpiresOn
		log.Debug("Successfully obtained KeyVault token",
			"expiresOn", result.keyVaultExpiresOn,
			"tokenLength", len(result.keyVaultToken))
	}
}

// createCredentials creates Azure credentials from acquired tokens.
// Currently returns nil error but signature matches GetCredentials interface.
//
//nolint:unparam // error return required for future extensibility and interface compatibility
func (p *deviceCodeProvider) createCredentials(tokens *tokenAcquisitionResult) (authTypes.ICredentials, error) {
	creds := &authTypes.AzureCredentials{
		AccessToken:    tokens.accessToken,
		TokenType:      "Bearer",
		Expiration:     tokens.expiresOn.Format(time.RFC3339),
		TenantID:       p.tenantID,
		SubscriptionID: p.subscriptionID,
		Location:       p.location,
	}

	// Add Graph API token if available.
	if tokens.graphToken != "" {
		creds.GraphAPIToken = tokens.graphToken
		creds.GraphAPIExpiration = tokens.graphExpiresOn.Format(time.RFC3339)
		log.Debug("Added Graph API token to credentials",
			"graphTokenLength", len(tokens.graphToken),
			"graphExpiration", tokens.graphExpiresOn.Format(time.RFC3339))
	} else {
		log.Debug("Graph API token is empty, not adding to credentials")
	}

	// Add KeyVault API token if available.
	if tokens.keyVaultToken != "" {
		creds.KeyVaultToken = tokens.keyVaultToken
		creds.KeyVaultExpiration = tokens.keyVaultExpiresOn.Format(time.RFC3339)
		log.Debug("Added KeyVault API token to credentials",
			"keyVaultTokenLength", len(tokens.keyVaultToken),
			"keyVaultExpiration", tokens.keyVaultExpiresOn.Format(time.RFC3339))
	} else {
		log.Debug("KeyVault API token is empty, not adding to credentials")
	}

	return creds, nil
}

// Validate checks the provider configuration and returns an error if required fields
// (tenant_id, client_id) are missing or invalid.
func (p *deviceCodeProvider) Validate() error {
	if p.tenantID == "" {
		return fmt.Errorf("%w: tenant_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns Azure-specific environment variables for this provider,
// including AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID, and AZURE_LOCATION if configured.
func (p *deviceCodeProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	if p.tenantID != "" {
		env["AZURE_TENANT_ID"] = p.tenantID
	}
	if p.subscriptionID != "" {
		env["AZURE_SUBSCRIPTION_ID"] = p.subscriptionID
	}
	if p.location != "" {
		env["AZURE_LOCATION"] = p.location
	}
	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes (Terraform, etc.)
// by merging provider configuration with the base environment and setting Azure-specific variables
// (ARM_TENANT_ID, ARM_SUBSCRIPTION_ID, ARM_USE_OIDC). Returns the prepared environment map and error.
// Note: access token is set later by SetEnvironmentVariables which loads from credential store.
func (p *deviceCodeProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	// Use shared Azure environment preparation.
	return azureCloud.PrepareEnvironment(azureCloud.PrepareEnvironmentConfig{
		Environ:        environ,
		SubscriptionID: p.subscriptionID,
		TenantID:       p.tenantID,
		Location:       p.location,
	}), nil
}

// Logout removes cached device code tokens from disk by deleting the MSAL token cache file.
// Returns an error if the cache deletion fails.
func (p *deviceCodeProvider) Logout(ctx context.Context) error {
	log.Debug("Logout Azure device code provider", "provider", p.name)
	return p.deleteCachedToken()
}

// GetFilesDisplayPath returns the user-facing display path for credential files
// stored by this provider (e.g., "~/.azure/atmos/provider-name").
func (p *deviceCodeProvider) GetFilesDisplayPath() string {
	return "~/.azure/atmos/" + p.name
}
