package exec

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

// getACRAuth attempts to get Azure Container Registry authentication.
func getACRAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Extract ACR name from registry URL first
	// Expected formats: <acr-name>.azurecr.{io|cn|us}
	acrName := ""
	for _, suf := range []string{".azurecr.io", ".azurecr.us", ".azurecr.cn"} {
		if strings.HasSuffix(registry, suf) {
			acrName = strings.TrimSuffix(registry, suf)
			break
		}
	}
	if acrName == "" {
		return nil, fmt.Errorf("invalid Azure Container Registry format: %s (expected <name>.azurecr.{io|us|cn})", registry)
	}

	// Create a Viper instance for environment variable access
	v := viper.New()
	bindEnv(v, "azure_client_id", "ATMOS_AZURE_CLIENT_ID", "AZURE_CLIENT_ID")
	bindEnv(v, "azure_client_secret", "ATMOS_AZURE_CLIENT_SECRET", "AZURE_CLIENT_SECRET")
	bindEnv(v, "azure_tenant_id", "ATMOS_AZURE_TENANT_ID", "AZURE_TENANT_ID")

	// Check for Azure Service Principal credentials first
	clientID := atmosConfig.Settings.OCI.AzureClientID
	clientSecret := atmosConfig.Settings.OCI.AzureClientSecret
	tenantID := atmosConfig.Settings.OCI.AzureTenantID

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

	// If we have all required Service Principal credentials, use them
	if clientID != "" && clientSecret != "" && tenantID != "" {
		log.Debug("Using Azure Service Principal credentials", "registry", registry, "acrName", acrName)
		return getACRAuthViaServicePrincipal(registry, acrName, clientID, clientSecret, tenantID)
	}

	// Prefer Azure Default Credential (Managed Identity, Workload Identity, etc.)
	if auth, err := getACRAuthViaDefaultCredential(registry, acrName); err == nil {
		return auth, nil
	} else {
		log.Debug("Azure Default Credential failed; considering CLI as fallback", "registry", registry, "error", err)
	}

	return nil, fmt.Errorf("no valid Azure authentication found for %s", registry)
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
		return "", fmt.Errorf("failed to create token exchange request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute token exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10)) // 2KB
		return "", fmt.Errorf("token exchange failed: status=%d body=%q", resp.StatusCode, string(body))
	}

	// Parse the response
	var tokenResponse struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("failed to decode token exchange response: %w", err)
	}

	// Return the refresh token (preferred) or access token as fallback
	if tokenResponse.RefreshToken != "" {
		return tokenResponse.RefreshToken, nil
	}
	if tokenResponse.AccessToken != "" {
		return tokenResponse.AccessToken, nil
	}

	return "", fmt.Errorf("no token received from ACR OAuth2 exchange")
}

// extractTenantIDFromToken extracts the tenant ID from a JWT token.
func extractTenantIDFromToken(tokenString string) (string, error) {
	// JWT tokens have 3 parts separated by dots
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT token format")
	}

	// Decode the payload (second part)
	payload := parts[1]
	// Decode Base64URL (no padding)
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	// Parse JSON payload
	var payloadData struct {
		TID string `json:"tid"`
	}

	if err := json.Unmarshal(decoded, &payloadData); err != nil {
		return "", fmt.Errorf("failed to parse JWT payload: %w", err)
	}

	if payloadData.TID == "" {
		return "", fmt.Errorf("tenant ID not found in JWT token")
	}

	return payloadData.TID, nil
}

// getACRAuthViaServicePrincipal attempts to get ACR credentials using Azure Service Principal
func getACRAuthViaServicePrincipal(registry, acrName, clientID, clientSecret, tenantID string) (authn.Authenticator, error) {
	// Create Azure credential using Service Principal
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	// Get AAD token for ACR scope
	ctx := context.Background()
	aad, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure token: %w", err)
	}

	// Exchange AAD token for ACR refresh token
	refresh, err := exchangeAADForACRRefreshToken(ctx, registry, tenantID, aad.Token)
	if err != nil {
		return nil, fmt.Errorf("acr token exchange failed: %w", err)
	}
	log.Debug("Obtained ACR refresh token via Service Principal", "registry", registry, "acrName", acrName)
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
		return nil, fmt.Errorf("failed to create Azure default credential: %w", err)
	}

	// Get AAD token for ACR scope
	ctx := context.Background()
	aad, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure token: %w", err)
	}

	// Exchange AAD token for ACR refresh token
	// Tenant is optional here; if unknown, pass empty and let ACR infer.
	refresh, err := exchangeAADForACRRefreshToken(ctx, registry, "", aad.Token)
	if err != nil {
		return nil, fmt.Errorf("acr token exchange failed: %w", err)
	}
	log.Debug("Obtained ACR refresh token via Default Credential", "registry", registry, "acrName", acrName)

	return &authn.Basic{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: refresh,
	}, nil
}
