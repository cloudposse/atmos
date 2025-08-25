package exec

import (
	"context"
	"encoding/json"
	"fmt"
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
	if azureCLIAuth == "" {
		azureCLIAuth = v.GetString("azure_cli_auth")
	}

	// If we have all required Service Principal credentials, use them
	if clientID != "" && clientSecret != "" && tenantID != "" {
		log.Debug("Using Azure Service Principal credentials", "registry", registry, "acrName", acrName)
		return getACRAuthViaServicePrincipal(registry, acrName, clientID, clientSecret, tenantID)
	}

	// Fallback to Azure CLI if available and enabled
	if azureCLIAuth != "" || hasAzureCLI() {
		log.Debug("Using Azure CLI authentication", "registry", registry, "acrName", acrName)
		return getACRAuthViaCLI(registry)
	}

	// Try using Azure Default Credential (Managed Identity, Azure CLI, etc.)
	log.Debug("Using Azure Default Credential", "registry", registry, "acrName", acrName)
	return getACRAuthViaDefaultCredential(registry, acrName)
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
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure token: %w", err)
	}

	// For ACR, we use the token as the password with "00000000-0000-0000-0000-000000000000" as username
	// This is the standard pattern for ACR authentication with AAD tokens
	log.Debug("Successfully obtained ACR credentials via Service Principal", "registry", registry, "acrName", acrName)

	return &authn.Basic{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: token.Token,
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
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure token: %w", err)
	}

	// For ACR, we use the token as the password with "00000000-0000-0000-0000-000000000000" as username
	log.Debug("Successfully obtained ACR credentials via Default Credential", "registry", registry, "acrName", acrName)

	return &authn.Basic{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: token.Token,
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
