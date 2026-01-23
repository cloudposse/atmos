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
	"sync"
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
	// ServicePrincipalTimeout is the timeout for HTTP requests.
	ServicePrincipalTimeout = 30 * time.Second

	// LogKeyProvider is the key used for provider name in log messages.
	logKeyProvider = "provider"
	// EnvValueFalse is the value "false" used for boolean environment variables.
	envValueFalse = "false"
)

// servicePrincipalProvider implements Azure Service Principal authentication.
// This provider uses client credentials (client_id + client_secret or client_certificate)
// to obtain Azure access tokens, suitable for automated/CI environments.
type servicePrincipalProvider struct {
	name                      string
	config                    *schema.Provider
	tenantID                  string
	clientID                  string
	clientSecret              string
	clientCertificatePath     string
	clientCertificatePassword string
	subscriptionID            string
	location                  string

	// httpClient is the HTTP client used for requests. If nil, a default client is used.
	httpClient httpClient.Client
	// tokenEndpoint can be overridden for testing. If empty, uses Azure AD endpoint.
	tokenEndpoint string
}

// servicePrincipalConfig holds extracted Azure service principal configuration from provider spec.
type servicePrincipalConfig struct {
	TenantID                  string
	ClientID                  string
	ClientSecret              string
	ClientCertificatePath     string
	ClientCertificatePassword string
	SubscriptionID            string
	Location                  string
}

// extractServicePrincipalConfig extracts Azure service principal config from provider spec.
func extractServicePrincipalConfig(spec map[string]interface{}) servicePrincipalConfig {
	config := servicePrincipalConfig{}

	if spec == nil {
		return config
	}

	if tid, ok := spec["tenant_id"].(string); ok {
		config.TenantID = tid
	}
	if cid, ok := spec["client_id"].(string); ok {
		config.ClientID = cid
	}
	if cs, ok := spec["client_secret"].(string); ok {
		config.ClientSecret = cs
	}
	if ccp, ok := spec["client_certificate_path"].(string); ok {
		config.ClientCertificatePath = ccp
	}
	if ccpw, ok := spec["client_certificate_password"].(string); ok {
		config.ClientCertificatePassword = ccpw
	}
	if sid, ok := spec["subscription_id"].(string); ok {
		config.SubscriptionID = sid
	}
	if loc, ok := spec["location"].(string); ok {
		config.Location = loc
	}

	return config
}

// NewServicePrincipalProvider creates a new Azure service principal provider.
func NewServicePrincipalProvider(name string, config *schema.Provider) (*servicePrincipalProvider, error) {
	defer perf.Track(nil, "azure.NewServicePrincipalProvider")()

	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "azure/service-principal" {
		return nil, fmt.Errorf("%w: invalid provider kind for Azure service principal provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract Azure-specific config from Spec.
	cfg := extractServicePrincipalConfig(config.Spec)

	// Tenant ID and Client ID are required.
	if cfg.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required in spec for Azure service principal provider", errUtils.ErrInvalidProviderConfig)
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: client_id is required in spec for Azure service principal provider", errUtils.ErrInvalidProviderConfig)
	}

	// Client secret can come from config or environment variable.
	// AZURE_CLIENT_SECRET is a standard Azure SDK env var for authentication.
	clientSecret := cfg.ClientSecret
	if clientSecret == "" {
		//nolint:forbidigo // AZURE_CLIENT_SECRET is a standard Azure SDK env var, not Atmos config
		clientSecret = os.Getenv("AZURE_CLIENT_SECRET")
	}

	// Client certificate path can come from config or environment variable.
	// AZURE_CLIENT_CERTIFICATE_PATH is a standard Azure SDK env var for certificate authentication.
	clientCertificatePath := cfg.ClientCertificatePath
	if clientCertificatePath == "" {
		//nolint:forbidigo // AZURE_CLIENT_CERTIFICATE_PATH is a standard Azure SDK env var, not Atmos config
		clientCertificatePath = os.Getenv("AZURE_CLIENT_CERTIFICATE_PATH")
	}

	// Client certificate password can come from config or environment variable.
	// AZURE_CLIENT_CERTIFICATE_PASSWORD is a standard Azure SDK env var for PFX certificates.
	clientCertificatePassword := cfg.ClientCertificatePassword
	if clientCertificatePassword == "" {
		//nolint:forbidigo // AZURE_CLIENT_CERTIFICATE_PASSWORD is a standard Azure SDK env var, not Atmos config
		clientCertificatePassword = os.Getenv("AZURE_CLIENT_CERTIFICATE_PASSWORD")
	}

	return &servicePrincipalProvider{
		name:                      name,
		config:                    config,
		tenantID:                  cfg.TenantID,
		clientID:                  cfg.ClientID,
		clientSecret:              clientSecret,
		clientCertificatePath:     clientCertificatePath,
		clientCertificatePassword: clientCertificatePassword,
		subscriptionID:            cfg.SubscriptionID,
		location:                  cfg.Location,
	}, nil
}

// Kind returns the provider kind.
func (p *servicePrincipalProvider) Kind() string {
	return "azure/service-principal"
}

// Name returns the configured provider name.
func (p *servicePrincipalProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for Azure service principal provider.
func (p *servicePrincipalProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// getHTTPClient returns the HTTP client to use for requests.
func (p *servicePrincipalProvider) getHTTPClient() httpClient.Client {
	if p.httpClient != nil {
		return p.httpClient
	}
	return httpClient.NewDefaultClient(httpClient.WithTimeout(ServicePrincipalTimeout))
}

// getTokenEndpoint returns the token endpoint URL.
func (p *servicePrincipalProvider) getTokenEndpoint() string {
	if p.tokenEndpoint != "" {
		return p.tokenEndpoint
	}
	return fmt.Sprintf(azureADTokenEndpoint, p.tenantID)
}

// Authenticate performs Azure service principal authentication using client credentials.
// This supports two authentication methods:
// 1. Client secret: acquires tokens for Azure CLI and Terraform compatibility.
// 2. Client certificate: returns credentials for Terraform (token exchange not supported).
func (p *servicePrincipalProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.servicePrincipalProvider.Authenticate")()

	log.Debug("Authenticating with Azure service principal",
		logKeyProvider, p.name, "tenant", p.tenantID, "client", p.clientID,
		"hasCertificate", p.clientCertificatePath != "")

	// Certificate-only authentication path (Terraform only, no token exchange).
	if p.clientCertificatePath != "" && p.clientSecret == "" {
		return p.buildCertificateCredentials(), nil
	}

	// Client secret authentication path (full token exchange).
	return p.authenticateWithClientSecret(ctx)
}

// buildCertificateCredentials creates credentials for certificate-only authentication.
// Token exchange is not supported; Terraform providers use ARM_CLIENT_CERTIFICATE_PATH directly.
func (p *servicePrincipalProvider) buildCertificateCredentials() *authTypes.AzureCredentials {
	log.Debug("Using certificate-only authentication",
		logKeyProvider, p.name, "certificatePath", p.clientCertificatePath,
		"note", "Token exchange not supported; Terraform will authenticate directly")

	return &authTypes.AzureCredentials{
		TenantID:           p.tenantID,
		SubscriptionID:     p.subscriptionID,
		Location:           p.location,
		ClientID:           p.clientID,
		IsServicePrincipal: true,
	}
}

// authenticateWithClientSecret performs token exchange using client credentials.
// Acquires tokens for Management API, Graph API, and KeyVault API scopes.
func (p *servicePrincipalProvider) authenticateWithClientSecret(ctx context.Context) (*authTypes.AzureCredentials, error) {
	if p.clientSecret == "" {
		return nil, fmt.Errorf("%w: client_secret or client_certificate_path is required for Azure service principal provider", errUtils.ErrAuthenticationFailed)
	}

	tokenResp, err := p.exchangeToken(ctx, azureManagementScope)
	if err != nil {
		return nil, err
	}

	expiresOn := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
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

	p.acquireAdditionalTokens(ctx, creds)

	log.Debug("Successfully authenticated with Azure service principal",
		logKeyProvider, p.name, "tenant", p.tenantID, "subscription", p.subscriptionID,
		"expiresOn", expiresOn.Format(time.RFC3339),
		"hasGraphToken", creds.GraphAPIToken != "", "hasKeyVaultToken", creds.KeyVaultToken != "")

	return creds, nil
}

// acquireAdditionalTokens acquires Graph API and KeyVault tokens in parallel.
// These tokens are optional - failures are logged but don't block authentication.
func (p *servicePrincipalProvider) acquireAdditionalTokens(ctx context.Context, creds *authTypes.AzureCredentials) {
	var wg sync.WaitGroup
	var mu sync.Mutex // Protects creds writes.

	wg.Add(2) //nolint:mnd

	// Acquire Microsoft Graph API token (required for azuread provider).
	go func() {
		defer wg.Done()
		graphResp, err := p.exchangeToken(ctx, azureGraphAPIScope)
		if err != nil {
			log.Debug("Failed to acquire Graph API token (azuread provider may not work)", "error", err)
			return
		}
		expiresOn := time.Now().Add(time.Duration(graphResp.ExpiresIn) * time.Second)
		mu.Lock()
		creds.GraphAPIToken = graphResp.AccessToken
		creds.GraphAPIExpiration = expiresOn.Format(time.RFC3339)
		mu.Unlock()
		log.Debug("Acquired Graph API token", "expiresOn", creds.GraphAPIExpiration)
	}()

	// Acquire Azure KeyVault API token (optional, for KeyVault operations).
	go func() {
		defer wg.Done()
		kvResp, err := p.exchangeToken(ctx, azureKeyVaultScope)
		if err != nil {
			log.Debug("Failed to acquire KeyVault API token (KeyVault operations may not work)", "error", err)
			return
		}
		expiresOn := time.Now().Add(time.Duration(kvResp.ExpiresIn) * time.Second)
		mu.Lock()
		creds.KeyVaultToken = kvResp.AccessToken
		creds.KeyVaultExpiration = expiresOn.Format(time.RFC3339)
		mu.Unlock()
		log.Debug("Acquired KeyVault API token", "expiresOn", creds.KeyVaultExpiration)
	}()

	wg.Wait()
}

// exchangeToken exchanges client credentials for an Azure access token for the specified scope.
func (p *servicePrincipalProvider) exchangeToken(ctx context.Context, scope string) (*tokenResponse, error) {
	defer perf.Track(nil, "azure.servicePrincipalProvider.exchangeToken")()

	tokenEndpoint := p.getTokenEndpoint()

	// Build request body using client_secret authentication.
	data := url.Values{}
	data.Set("grant_type", grantTypeClientCredentials)
	data.Set("client_id", p.clientID)
	data.Set("client_secret", p.clientSecret)
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
		return nil, fmt.Errorf("%w: failed to exchange client credentials: %w", errUtils.ErrAuthenticationFailed, err)
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

	log.Debug("Successfully exchanged client credentials for Azure access token",
		"scope", scope,
		"tokenType", tokenResp.TokenType,
		"expiresIn", tokenResp.ExpiresIn,
	)

	return &tokenResp, nil
}

// Validate checks the provider configuration.
func (p *servicePrincipalProvider) Validate() error {
	if p.tenantID == "" {
		return fmt.Errorf("%w: tenant_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}
	// Note: client_secret/client_certificate_path validation happens at authentication time
	// to allow environment variable configuration (AZURE_CLIENT_SECRET, AZURE_CLIENT_CERTIFICATE_PATH).
	return nil
}

// Environment returns Azure-specific environment variables for this provider.
func (p *servicePrincipalProvider) Environment() (map[string]string, error) {
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
// For service principal providers, we use client credentials auth (not CLI mode).
func (p *servicePrincipalProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "azure.servicePrincipalProvider.PrepareEnvironment")()

	// Use shared Azure environment preparation.
	result := azureCloud.PrepareEnvironment(azureCloud.PrepareEnvironmentConfig{
		Environ:        environ,
		SubscriptionID: p.subscriptionID,
		TenantID:       p.tenantID,
		Location:       p.location,
	})

	// Explicitly disable CLI and OIDC auth modes for service principal authentication.
	// ARM_USE_CLI=true only works for user accounts, not service principals.
	// ARM_USE_OIDC=true is for federated credentials, not client secret/certificate.
	// The azurerm/azapi providers will use the ARM_CLIENT_ID/ARM_CLIENT_SECRET credentials we set below.
	result["ARM_USE_CLI"] = envValueFalse
	result["ARM_USE_OIDC"] = envValueFalse

	// Set client credentials for Terraform providers (azurerm, azapi).
	if p.clientID != "" {
		result["ARM_CLIENT_ID"] = p.clientID
	}
	if p.clientSecret != "" {
		result["ARM_CLIENT_SECRET"] = p.clientSecret
	}

	// Set certificate credentials for Terraform providers.
	// Azure SDK and Terraform support both PEM and PFX certificate formats.
	if p.clientCertificatePath != "" {
		result["ARM_CLIENT_CERTIFICATE_PATH"] = p.clientCertificatePath
		result["AZURE_CLIENT_CERTIFICATE_PATH"] = p.clientCertificatePath
		if p.clientCertificatePassword != "" {
			result["ARM_CLIENT_CERTIFICATE_PASSWORD"] = p.clientCertificatePassword
			result["AZURE_CLIENT_CERTIFICATE_PASSWORD"] = p.clientCertificatePassword
		}
	}

	log.Debug("Azure service principal environment prepared",
		"ARM_USE_CLI", envValueFalse, "ARM_USE_OIDC", envValueFalse,
		"ARM_CLIENT_ID", p.clientID, "hasCertificate", p.clientCertificatePath != "",
		"subscription", p.subscriptionID, "tenant", p.tenantID)

	return result, nil
}

// Logout clears cached credentials for this provider.
func (p *servicePrincipalProvider) Logout(ctx context.Context) error {
	log.Debug("Azure service principal provider logout", logKeyProvider, p.name)
	// Service principal credentials are typically managed externally.
	// Return ErrLogoutNotSupported to indicate successful no-op (exit 0).
	return errUtils.ErrLogoutNotSupported
}

// Paths returns credential files/directories used by this provider.
// Azure service principal provider does not manage credential files directly.
func (p *servicePrincipalProvider) Paths() ([]authTypes.Path, error) {
	return []authTypes.Path{}, nil
}

// GetFilesDisplayPath returns empty string (no files managed directly by this provider).
func (p *servicePrincipalProvider) GetFilesDisplayPath() string {
	return "" // Service principal provider does not manage credential files directly.
}
