package exec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// httpClient is used for outbound HTTP in this package; override in tests.
var httpClient = &http.Client{Timeout: 30 * time.Second}

const (
	// Log field names
	logFieldRegistry = "registry"
)

var (
	// Static errors for Azure authentication
	errInvalidACRRegistryFormat        = errors.New("invalid Azure Container Registry format")
	errNoValidAzureAuth                = errors.New("no valid Azure authentication found")
	errFailedToCreateTokenExchangeReq  = errors.New("failed to create token exchange request")
	errFailedToExecuteTokenExchangeReq = errors.New("failed to execute token exchange request")
	errTokenExchangeFailed             = errors.New("token exchange failed")
	errFailedToDecodeTokenExchangeResp = errors.New("failed to decode token exchange response")
	errNoTokenReceivedFromACR          = errors.New("no token received from ACR OAuth2 exchange")
	errInvalidJWTTokenFormat           = errors.New("invalid JWT token format")
	errFailedToDecodeJWTPayload        = errors.New("failed to decode JWT payload")
	errFailedToParseJWTPayload         = errors.New("failed to parse JWT payload")
	errTenantIDNotFoundInJWT           = errors.New("tenant ID not found in JWT token")
	errFailedToCreateAzureCredential   = errors.New("failed to create Azure credential")
	errFailedToGetAzureToken           = errors.New("failed to get Azure token")
	errACRTokenExchangeFailed          = errors.New("acr token exchange failed")
	errFailedToCreateAzureDefaultCred  = errors.New("failed to create Azure default credential")
)

// extractACRName extracts the ACR name from the registry URL.
func extractACRName(registry string) (string, error) {
	// Expected formats: <acr-name>.azurecr.{io|cn|us}
	for _, suf := range []string{".azurecr.io", ".azurecr.us", ".azurecr.cn"} {
		if strings.HasSuffix(registry, suf) {
			return strings.TrimSuffix(registry, suf), nil
		}
	}
	return "", fmt.Errorf("%w: %s (expected <name>.azurecr.{io|us|cn})", errInvalidACRRegistryFormat, registry)
}

// gatherAzureCredentials collects Azure credentials from config and environment.
func gatherAzureCredentials(atmosConfig *schema.AtmosConfiguration) (clientID, clientSecret, tenantID string) {
	// Create a Viper instance for environment variable access
	v := viper.New()
	bindEnv(v, "azure_client_id", "ATMOS_OCI_AZURE_CLIENT_ID", "AZURE_CLIENT_ID")
	bindEnv(v, "azure_client_secret", "ATMOS_OCI_AZURE_CLIENT_SECRET", "AZURE_CLIENT_SECRET")
	bindEnv(v, "azure_tenant_id", "ATMOS_OCI_AZURE_TENANT_ID", "AZURE_TENANT_ID")

	// Check for Azure Service Principal credentials first
	clientID = atmosConfig.Settings.OCI.AzureClientID
	clientSecret = atmosConfig.Settings.OCI.AzureClientSecret
	tenantID = atmosConfig.Settings.OCI.AzureTenantID

	// Fallback to environment variables for backward compatibility
	if clientID == "" {
		clientID = v.GetString("azure_client_id")
	}
	if clientSecret == "" {
		clientSecret = v.GetString("azure_client_secret")
	}
	if tenantID == "" {
		tenantID = v.GetString("azure_tenant_id")
	}

	return clientID, clientSecret, tenantID
}

// getACRAuth attempts to get Azure Container Registry authentication.
func getACRAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Extract ACR name from registry URL
	acrName, err := extractACRName(registry)
	if err != nil {
		return nil, err
	}

	// Gather Azure credentials
	clientID, clientSecret, tenantID := gatherAzureCredentials(atmosConfig)

	// Try Service Principal authentication if credentials are available
	if clientID != "" && clientSecret != "" && tenantID != "" {
		log.Debug("Using Azure Service Principal credentials", logFieldRegistry, registry, "acrName", acrName)
		return getACRAuthViaServicePrincipal(registry, acrName, clientID, clientSecret, tenantID)
	}

	// Try Azure Default Credential (Managed Identity, Workload Identity, etc.)
	if auth, err := getACRAuthViaDefaultCredential(registry, acrName); err == nil {
		return auth, nil
	} else {
		log.Debug("Azure Default Credential failed", logFieldRegistry, registry, "error", err)
	}

	return nil, fmt.Errorf("%w for %s", errNoValidAzureAuth, registry)
}

// exchangeAADForACRRefreshToken exchanges an AAD token for an ACR refresh token.
func exchangeAADForACRRefreshToken(ctx context.Context, registry, tenantID, aadToken string) (string, error) {
	// ACR OAuth2 endpoint for token exchange
	oauthURL := fmt.Sprintf("https://%s/oauth2/exchange", registry)

	// Prepare the form data for the token exchange
	formData := url.Values{}
	formData.Set("grant_type", "access_token")
	formData.Set("service", registry)
	if tenantID != "" {
		formData.Set("tenant", tenantID)
	}
	formData.Set("access_token", aadToken)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", oauthURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: %w", errFailedToCreateTokenExchangeReq, err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errFailedToExecuteTokenExchangeReq, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10)) // 2KB
		return "", fmt.Errorf("%w: status=%d body=%q", errTokenExchangeFailed, resp.StatusCode, string(body))
	}

	// Parse the response
	var tokenResponse struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("%w: %w", errFailedToDecodeTokenExchangeResp, err)
	}

	// Return the refresh token (preferred) or access token as fallback
	if tokenResponse.RefreshToken != "" {
		return tokenResponse.RefreshToken, nil
	}
	if tokenResponse.AccessToken != "" {
		return tokenResponse.AccessToken, nil
	}

	return "", errNoTokenReceivedFromACR
}

// extractTenantIDFromToken extracts the tenant ID from a JWT token.
func extractTenantIDFromToken(tokenString string) (string, error) {
	// JWT tokens have 3 parts separated by dots
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return "", errInvalidJWTTokenFormat
	}

	// Decode the payload (second part)
	payload := parts[1]
	// Decode Base64URL (no padding)
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errFailedToDecodeJWTPayload, err)
	}

	// Parse JSON payload
	var payloadData struct {
		TID string `json:"tid"`
	}

	if err := json.Unmarshal(decoded, &payloadData); err != nil {
		return "", fmt.Errorf("%w: %w", errFailedToParseJWTPayload, err)
	}

	if payloadData.TID == "" {
		return "", errTenantIDNotFoundInJWT
	}

	return payloadData.TID, nil
}

// getACRAuthViaServicePrincipal attempts to get ACR credentials using Azure Service Principal
func getACRAuthViaServicePrincipal(registry, acrName, clientID, clientSecret, tenantID string) (authn.Authenticator, error) {
	// Create Azure credential using Service Principal
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToCreateAzureCredential, err)
	}

	// Get AAD token for ACR scope
	ctx := context.Background()
	aad, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToGetAzureToken, err)
	}

	// Exchange AAD token for ACR refresh token
	refresh, err := exchangeAADForACRRefreshToken(ctx, registry, tenantID, aad.Token)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errACRTokenExchangeFailed, err)
	}
	log.Debug("Obtained ACR refresh token via Service Principal", logFieldRegistry, registry, "acrName", acrName)
	return &authn.Basic{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: refresh,
	}, nil
}

// getACRAuthViaDefaultCredential attempts to get ACR credentials using Azure Default Credential
func getACRAuthViaDefaultCredential(registry, acrName string) (authn.Authenticator, error) {
	// Create Azure credential using Default Credential (Managed Identity, Azure CLI, etc.)
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToCreateAzureDefaultCred, err)
	}

	// Get AAD token for ACR scope
	ctx := context.Background()
	aad, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToGetAzureToken, err)
	}

	// Exchange AAD token for ACR refresh token
	// Tenant is optional here; if unknown, pass empty and let ACR infer.
	refresh, err := exchangeAADForACRRefreshToken(ctx, registry, "", aad.Token)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errACRTokenExchangeFailed, err)
	}
	log.Debug("Obtained ACR refresh token via Default Credential", logFieldRegistry, registry, "acrName", acrName)

	return &authn.Basic{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: refresh,
	}, nil
}
