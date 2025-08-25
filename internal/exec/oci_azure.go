package exec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// getACRAuth attempts to get Azure Container Registry authentication
func getACRAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Extract ACR name from registry URL first
	// Expected format: <acr-name>.azurecr.io
	acrName := ""
	if strings.HasSuffix(registry, ".azurecr.io") {
		acrName = strings.TrimSuffix(registry, ".azurecr.io")
	} else {
		return nil, fmt.Errorf("invalid Azure Container Registry format: %s (expected <name>.azurecr.io)", registry)
	}

	if acrName == "" {
		return nil, fmt.Errorf("could not extract ACR name from registry: %s", registry)
	}

	// Create a Viper instance for environment variable access
	v := viper.New()
	bindEnv(v, "azure_client_id", "ATMOS_AZURE_CLIENT_ID", "AZURE_CLIENT_ID")
	bindEnv(v, "azure_client_secret", "ATMOS_AZURE_CLIENT_SECRET", "AZURE_CLIENT_SECRET")
	bindEnv(v, "azure_tenant_id", "ATMOS_AZURE_TENANT_ID", "AZURE_TENANT_ID")
	bindEnv(v, "azure_cli_auth", "ATMOS_AZURE_CLI_AUTH", "AZURE_CLI_AUTH")

	// Check for Azure Service Principal credentials first
	clientID := atmosConfig.Settings.OCI.AzureClientID
	clientSecret := atmosConfig.Settings.OCI.AzureClientSecret
	tenantID := atmosConfig.Settings.OCI.AzureTenantID
	azureCLIAuth := atmosConfig.Settings.OCI.AzureCLIAuth

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

	// Resolve CLI auth flag (treat explicit true/false properly)
	cliAuth := false
	if azureCLIAuth != "" {
		cliAuth = strings.EqualFold(azureCLIAuth, "true")
	} else {
		cliAuth = v.GetBool("azure_cli_auth")
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

	// CLI only if explicitly enabled and available
	if cliAuth && hasAzureCLI() {
		log.Debug("Using Azure CLI authentication", "registry", registry, "acrName", acrName)
		return getACRAuthViaCLI(registry)
	}

	return nil, fmt.Errorf("no valid Azure authentication found for %s", registry)
}

// exchangeAADForACRRefreshToken exchanges an AAD token for an ACR refresh token
func exchangeAADForACRRefreshToken(ctx context.Context, registry, tenantID, aadToken string) (string, error) {
	// ACR OAuth2 endpoint for token exchange
	oauthURL := fmt.Sprintf("https://%s/oauth2/exchange", registry)

	// Prepare the form data for the token exchange
	formData := url.Values{}
	formData.Set("grant_type", "access_token")
	formData.Set("service", registry)
	formData.Set("tenant", tenantID)
	formData.Set("access_token", aadToken)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", oauthURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token exchange request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute token exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
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

// extractTenantIDFromToken extracts the tenant ID from a JWT token
func extractTenantIDFromToken(tokenString string) (string, error) {
	// JWT tokens have 3 parts separated by dots
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT token format")
	}

	// Decode the payload (second part)
	payload := parts[1]
	// Add padding if needed
	if len(payload)%4 != 0 {
		payload += strings.Repeat("=", 4-len(payload)%4)
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(payload)
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

// hasAzureCLI checks if Azure CLI is available
func hasAzureCLI() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "az", "version")
	return cmd.Run() == nil
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

// getACRAuthViaCLI attempts to get ACR credentials using Azure CLI
func getACRAuthViaCLI(registry string) (authn.Authenticator, error) {
	// Extract ACR name from registry URL
	acrName := ""
	if strings.HasSuffix(registry, ".azurecr.io") {
		acrName = strings.TrimSuffix(registry, ".azurecr.io")
	} else {
		return nil, fmt.Errorf("invalid Azure Container Registry format: %s (expected <name>.azurecr.io)", registry)
	}

	if acrName == "" {
		return nil, fmt.Errorf("could not extract ACR name from registry: %s", registry)
	}

	// Use Azure CLI to get ACR credentials
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "az", "acr", "credential", "show", "--name", acrName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ACR credentials via Azure CLI: %w", err)
	}

	// Parse the JSON output
	var result struct {
		Passwords []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"passwords"`
		Username string `json:"username"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Azure CLI output: %w", err)
	}

	if result.Username == "" {
		return nil, fmt.Errorf("no username returned from Azure CLI")
	}

	if len(result.Passwords) == 0 {
		return nil, fmt.Errorf("no passwords returned from Azure CLI")
	}

	// Use the first password (usually there are two - one for each credential)
	password := result.Passwords[0].Value

	log.Debug("Successfully obtained ACR credentials via Azure CLI", "registry", registry, "acrName", acrName)

	return &authn.Basic{
		Username: result.Username,
		Password: password,
	}, nil
}
