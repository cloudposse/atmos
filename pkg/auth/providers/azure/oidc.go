package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// OIDCTimeout is the timeout for HTTP requests.
	OIDCTimeout = 30 * time.Second

	// Azure AD OAuth2 token endpoint format.
	azureADTokenEndpoint = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"

	// Default scope for Azure management API.
	azureManagementScope = "https://management.azure.com/.default"

	// Scope for Microsoft Graph API (required for azuread provider and some az commands).
	azureGraphAPIScope = "https://graph.microsoft.com/.default"

	// Scope for Azure KeyVault API (optional, for KeyVault operations).
	azureKeyVaultScope = "https://vault.azure.net/.default"

	// Grant type for client credentials with federated token.
	grantTypeClientCredentials = "client_credentials"

	// Client assertion type for federated token (OIDC).
	clientAssertionTypeJWT = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

// oidcProvider implements Azure OIDC/Workload Identity Federation authentication.
// This provider is designed for CI/CD environments (GitHub Actions, Azure DevOps, etc.)
// where a federated identity token is exchanged for Azure credentials.
type oidcProvider struct {
	name           string
	config         *schema.Provider
	tenantID       string
	clientID       string
	subscriptionID string
	location       string
	audience       string
	tokenFilePath  string

	// httpClient is the HTTP client used for requests. If nil, a default client is used.
	// Uses the shared httpClient.Client interface from pkg/http for consistency.
	httpClient httpClient.Client
	// tokenEndpoint can be overridden for testing. If empty, uses Azure AD endpoint.
	tokenEndpoint string
}

// oidcConfig holds extracted Azure OIDC configuration from provider spec.
type oidcConfig struct {
	TenantID       string
	ClientID       string
	SubscriptionID string
	Location       string
	Audience       string
	TokenFilePath  string
}

// tokenResponse represents the response from Azure AD token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"` // Seconds until expiration.
	Scope       string `json:"scope,omitempty"`
}

// extractOIDCConfig extracts Azure OIDC config from provider spec.
func extractOIDCConfig(spec map[string]interface{}) oidcConfig {
	config := oidcConfig{}

	if spec == nil {
		return config
	}

	if tid, ok := spec["tenant_id"].(string); ok {
		config.TenantID = tid
	}
	if cid, ok := spec["client_id"].(string); ok {
		config.ClientID = cid
	}
	if sid, ok := spec["subscription_id"].(string); ok {
		config.SubscriptionID = sid
	}
	if loc, ok := spec["location"].(string); ok {
		config.Location = loc
	}
	if aud, ok := spec["audience"].(string); ok {
		config.Audience = aud
	}
	if tfp, ok := spec["token_file_path"].(string); ok {
		config.TokenFilePath = tfp
	}

	return config
}

// NewOIDCProvider creates a new Azure OIDC provider for workload identity federation.
func NewOIDCProvider(name string, config *schema.Provider) (*oidcProvider, error) {
	defer perf.Track(nil, "azure.NewOIDCProvider")()

	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "azure/oidc" {
		return nil, fmt.Errorf("%w: invalid provider kind for Azure OIDC provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract Azure-specific config from Spec.
	cfg := extractOIDCConfig(config.Spec)

	// Tenant ID and Client ID are required.
	if cfg.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required in spec for Azure OIDC provider", errUtils.ErrInvalidProviderConfig)
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: client_id is required in spec for Azure OIDC provider", errUtils.ErrInvalidProviderConfig)
	}

	return &oidcProvider{
		name:           name,
		config:         config,
		tenantID:       cfg.TenantID,
		clientID:       cfg.ClientID,
		subscriptionID: cfg.SubscriptionID,
		location:       cfg.Location,
		audience:       cfg.Audience,
		tokenFilePath:  cfg.TokenFilePath,
	}, nil
}

// Kind returns the provider kind.
func (p *oidcProvider) Kind() string {
	return "azure/oidc"
}

// Name returns the configured provider name.
func (p *oidcProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for Azure OIDC provider.
func (p *oidcProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// getHTTPClient returns the HTTP client to use for requests.
func (p *oidcProvider) getHTTPClient() httpClient.Client {
	if p.httpClient != nil {
		return p.httpClient
	}
	return httpClient.NewDefaultClient(OIDCTimeout)
}

// getTokenEndpoint returns the token endpoint URL.
func (p *oidcProvider) getTokenEndpoint() string {
	if p.tokenEndpoint != "" {
		return p.tokenEndpoint
	}
	return fmt.Sprintf(azureADTokenEndpoint, p.tenantID)
}

// Authenticate performs Azure OIDC authentication by exchanging a federated token
// (from GitHub Actions, Azure DevOps, etc.) for Azure credentials.
// This acquires tokens for multiple scopes to ensure Azure CLI and Terraform compatibility:
// - Management API (ARM operations)
// - Graph API (azuread provider and some az commands)
// - KeyVault API (optional, for KeyVault operations).
func (p *oidcProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.oidcProvider.Authenticate")()

	log.Debug("Authenticating with Azure OIDC",
		"provider", p.name,
		"tenant", p.tenantID,
		"client", p.clientID,
	)

	// Read the federated token.
	federatedToken, err := p.readFederatedToken()
	if err != nil {
		return nil, err
	}

	// Exchange the federated token for the primary Azure Management API token.
	tokenResp, err := p.exchangeToken(ctx, federatedToken, azureManagementScope)
	if err != nil {
		return nil, err
	}

	// Calculate expiration time.
	expiresOn := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Create Azure credentials with the primary management token.
	// Mark as service principal for correct MSAL cache format.
	creds := &authTypes.AzureCredentials{
		AccessToken:        tokenResp.AccessToken,
		TokenType:          tokenResp.TokenType,
		Expiration:         expiresOn.Format(time.RFC3339),
		TenantID:           p.tenantID,
		SubscriptionID:     p.subscriptionID,
		Location:           p.location,
		ClientID:           p.clientID,
		IsServicePrincipal: true,
	}

	// Acquire additional tokens for Azure CLI and Terraform provider compatibility.
	// These are acquired in parallel for efficiency.
	p.acquireAdditionalTokens(ctx, federatedToken, creds)

	log.Debug("Successfully authenticated with Azure OIDC",
		"provider", p.name,
		"tenant", p.tenantID,
		"subscription", p.subscriptionID,
		"expiresOn", expiresOn.Format(time.RFC3339),
		"hasGraphToken", creds.GraphAPIToken != "",
		"hasKeyVaultToken", creds.KeyVaultToken != "",
	)

	return creds, nil
}

// acquireAdditionalTokens acquires Graph API and KeyVault tokens.
// These tokens are optional - failures are logged but don't block authentication.
func (p *oidcProvider) acquireAdditionalTokens(ctx context.Context, federatedToken string, creds *authTypes.AzureCredentials) {
	// Acquire Microsoft Graph API token (required for azuread provider).
	graphResp, err := p.exchangeToken(ctx, federatedToken, azureGraphAPIScope)
	if err != nil {
		log.Debug("Failed to acquire Graph API token (azuread provider may not work)", "error", err)
	} else {
		expiresOn := time.Now().Add(time.Duration(graphResp.ExpiresIn) * time.Second)
		creds.GraphAPIToken = graphResp.AccessToken
		creds.GraphAPIExpiration = expiresOn.Format(time.RFC3339)
		log.Debug("Acquired Graph API token", "expiresOn", creds.GraphAPIExpiration)
	}

	// Acquire Azure KeyVault API token (optional, for KeyVault operations).
	kvResp, err := p.exchangeToken(ctx, federatedToken, azureKeyVaultScope)
	if err != nil {
		log.Debug("Failed to acquire KeyVault API token (KeyVault operations may not work)", "error", err)
	} else {
		expiresOn := time.Now().Add(time.Duration(kvResp.ExpiresIn) * time.Second)
		creds.KeyVaultToken = kvResp.AccessToken
		creds.KeyVaultExpiration = expiresOn.Format(time.RFC3339)
		log.Debug("Acquired KeyVault API token", "expiresOn", creds.KeyVaultExpiration)
	}
}

// readFederatedToken reads the federated token from the configured source.
// Priority:
//  1. Token file path from config
//  2. AZURE_FEDERATED_TOKEN_FILE environment variable
//  3. ACTIONS_ID_TOKEN_REQUEST_URL + ACTIONS_ID_TOKEN_REQUEST_TOKEN (GitHub Actions)
func (p *oidcProvider) readFederatedToken() (string, error) {
	defer perf.Track(nil, "azure.oidcProvider.readFederatedToken")()

	// Try config-specified token file path first.
	if p.tokenFilePath != "" {
		log.Debug("Reading federated token from config path", "path", p.tokenFilePath)
		return p.readTokenFromFile(p.tokenFilePath)
	}

	// Try AZURE_FEDERATED_TOKEN_FILE environment variable.
	if tokenFile := os.Getenv("AZURE_FEDERATED_TOKEN_FILE"); tokenFile != "" {
		log.Debug("Reading federated token from AZURE_FEDERATED_TOKEN_FILE", "path", tokenFile)
		return p.readTokenFromFile(tokenFile)
	}

	// Try GitHub Actions OIDC.
	if p.isGitHubActions() {
		log.Debug("Detected GitHub Actions environment, fetching OIDC token")
		return p.fetchGitHubActionsToken()
	}

	return "", fmt.Errorf("%w: no federated token source found. Set token_file_path in config, AZURE_FEDERATED_TOKEN_FILE environment variable, or run in GitHub Actions with id-token: write permission", errUtils.ErrAuthenticationFailed)
}

// readTokenFromFile reads a federated token from a file.
func (p *oidcProvider) readTokenFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read federated token file %q: %w", errUtils.ErrAuthenticationFailed, path, err)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("%w: federated token file %q is empty", errUtils.ErrAuthenticationFailed, path)
	}

	return token, nil
}

// isGitHubActions checks if we're running in GitHub Actions.
func (p *oidcProvider) isGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// fetchGitHubActionsToken fetches an OIDC token from GitHub Actions.
func (p *oidcProvider) fetchGitHubActionsToken() (string, error) {
	defer perf.Track(nil, "azure.oidcProvider.fetchGitHubActionsToken")()

	requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	if requestURL == "" {
		return "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL not set - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}
	if requestToken == "" {
		return "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_TOKEN not set - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}

	// Determine audience for the GitHub OIDC token.
	audience := p.audience
	if audience == "" {
		// Default audience for Azure AD is "api://AzureADTokenExchange".
		audience = "api://AzureADTokenExchange"
	}

	// Build request URL with audience.
	reqURL, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid ACTIONS_ID_TOKEN_REQUEST_URL: %w", errUtils.ErrAuthenticationFailed, err)
	}
	q := reqURL.Query()
	q.Set("audience", audience)
	reqURL.RawQuery = q.Encode()

	// Create HTTP request.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create GitHub OIDC request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")

	// Execute request using the configured HTTP client.
	resp, err := p.getHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: failed to fetch GitHub OIDC token: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: GitHub OIDC endpoint returned status %d: %s", errUtils.ErrAuthenticationFailed, resp.StatusCode, string(body))
	}

	// Parse response.
	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%w: failed to decode GitHub OIDC response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	if result.Value == "" {
		return "", fmt.Errorf("%w: empty token in GitHub OIDC response", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("Successfully fetched GitHub Actions OIDC token", "audience", audience)
	return result.Value, nil
}

// exchangeToken exchanges a federated token for an Azure access token for the specified scope.
func (p *oidcProvider) exchangeToken(ctx context.Context, federatedToken, scope string) (*tokenResponse, error) {
	defer perf.Track(nil, "azure.oidcProvider.exchangeToken")()

	tokenEndpoint := p.getTokenEndpoint()

	// Build request body.
	data := url.Values{}
	data.Set("grant_type", grantTypeClientCredentials)
	data.Set("client_id", p.clientID)
	data.Set("client_assertion_type", clientAssertionTypeJWT)
	data.Set("client_assertion", federatedToken)
	data.Set("scope", scope)

	// Create HTTP request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create token request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request using the configured HTTP client.
	resp, err := p.getHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to exchange federated token: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read token response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: Azure AD token endpoint returned status %d: %s", errUtils.ErrAuthenticationFailed, resp.StatusCode, string(body))
	}

	// Parse response.
	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("%w: failed to decode Azure AD token response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("%w: empty access token in Azure AD response", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("Successfully exchanged federated token for Azure access token",
		"scope", scope,
		"tokenType", tokenResp.TokenType,
		"expiresIn", tokenResp.ExpiresIn,
	)

	return &tokenResp, nil
}

// Validate checks the provider configuration.
func (p *oidcProvider) Validate() error {
	if p.tenantID == "" {
		return fmt.Errorf("%w: tenant_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns Azure-specific environment variables for this provider.
func (p *oidcProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	if p.tenantID != "" {
		env["AZURE_TENANT_ID"] = p.tenantID
	}
	if p.clientID != "" {
		env["AZURE_CLIENT_ID"] = p.clientID
	}
	if p.subscriptionID != "" {
		env["AZURE_SUBSCRIPTION_ID"] = p.subscriptionID
	}
	if p.location != "" {
		env["AZURE_LOCATION"] = p.location
	}
	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes (Terraform, etc.).
// For OIDC providers, we set ARM_USE_OIDC=true to enable OIDC authentication.
func (p *oidcProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "azure.oidcProvider.PrepareEnvironment")()

	// Use shared Azure environment preparation.
	result := azureCloud.PrepareEnvironment(azureCloud.PrepareEnvironmentConfig{
		Environ:        environ,
		SubscriptionID: p.subscriptionID,
		TenantID:       p.tenantID,
		Location:       p.location,
	})

	// Override ARM_USE_CLI to use OIDC instead.
	delete(result, "ARM_USE_CLI")
	result["ARM_USE_OIDC"] = "true"

	// Set client ID for Terraform providers.
	if p.clientID != "" {
		result["ARM_CLIENT_ID"] = p.clientID
	}

	// Set token file path if available.
	if p.tokenFilePath != "" {
		result["AZURE_FEDERATED_TOKEN_FILE"] = p.tokenFilePath
	} else if tokenFile := os.Getenv("AZURE_FEDERATED_TOKEN_FILE"); tokenFile != "" {
		result["AZURE_FEDERATED_TOKEN_FILE"] = tokenFile
	}

	log.Debug("Azure OIDC environment prepared",
		"ARM_USE_OIDC", "true",
		"ARM_CLIENT_ID", p.clientID,
		"subscription", p.subscriptionID,
		"tenant", p.tenantID,
	)

	return result, nil
}

// Logout is a no-op for Azure OIDC provider (credentials are ephemeral).
func (p *oidcProvider) Logout(ctx context.Context) error {
	log.Debug("Azure OIDC provider logout - no cached credentials to clear", "provider", p.name)
	// Return ErrLogoutNotSupported to indicate successful no-op (exit 0).
	return errUtils.ErrLogoutNotSupported
}

// Paths returns credential files/directories used by this provider.
// Azure OIDC provider doesn't use file-based credentials.
func (p *oidcProvider) Paths() ([]authTypes.Path, error) {
	return []authTypes.Path{}, nil
}

// GetFilesDisplayPath returns empty string (no files managed by this provider).
func (p *oidcProvider) GetFilesDisplayPath() string {
	return "" // OIDC provider doesn't manage files.
}
