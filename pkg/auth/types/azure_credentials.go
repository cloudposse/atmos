package types

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"

	errUtils "github.com/cloudposse/atmos/errors"
)

// AzureCredentials defines Azure-specific credential fields.
type AzureCredentials struct {
	AccessToken        string `json:"access_token,omitempty"`
	TokenType          string `json:"token_type,omitempty"`           // Usually "Bearer"
	Expiration         string `json:"expiration,omitempty"`           // RFC3339 timestamp
	TenantID           string `json:"tenant_id,omitempty"`            // Azure AD tenant ID
	SubscriptionID     string `json:"subscription_id,omitempty"`      // Azure subscription ID
	Location           string `json:"location,omitempty"`             // Azure region (e.g., "eastus")
	GraphAPIToken      string `json:"graph_api_token,omitempty"`      // Microsoft Graph API token
	GraphAPIExpiration string `json:"graph_api_expiration,omitempty"` // RFC3339 timestamp for Graph API token
	KeyVaultToken      string `json:"key_vault_token,omitempty"`      // Azure KeyVault API token
	KeyVaultExpiration string `json:"key_vault_expiration,omitempty"` // RFC3339 timestamp for KeyVault token
	// ClientID is set for service principal authentication (OIDC).
	// When set, MSAL cache uses client credentials format instead of user format.
	ClientID string `json:"client_id,omitempty"`
	// IsServicePrincipal indicates this is service principal auth (OIDC/client credentials).
	// Service principal tokens use a different MSAL cache format than user tokens.
	IsServicePrincipal bool `json:"is_service_principal,omitempty"`
	// TokenFilePath is the path to the OIDC token file (e.g., from GitHub Actions).
	// Used for Terraform ARM_USE_OIDC authentication.
	TokenFilePath string `json:"token_file_path,omitempty"`
	// FederatedToken is the actual OIDC/federated token value.
	// This is stored during authentication for use by Azure CLI.
	// In GitHub Actions, this is obtained dynamically, not from a file.
	FederatedToken string `json:"-"` // Don't persist - it's ephemeral.
}

// IsExpired returns true if the credentials are expired.
// This implements the ICredentials interface.
func (c *AzureCredentials) IsExpired() bool {
	if c.Expiration == "" {
		return false
	}
	expTime, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return true
	}
	return !time.Now().Before(expTime)
}

// GetExpiration implements ICredentials for AzureCredentials.
func (c *AzureCredentials) GetExpiration() (*time.Time, error) {
	if c.Expiration == "" {
		return nil, nil
	}
	expTime, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return nil, fmt.Errorf("%w: failed parsing Azure credential expiration: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	// Convert to local timezone for display to user.
	localTime := expTime.Local()
	return &localTime, nil
}

// BuildWhoamiInfo implements ICredentials for AzureCredentials.
func (c *AzureCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	info.Region = c.Location
	if c.SubscriptionID != "" {
		info.Account = c.SubscriptionID
	}
	if t, _ := c.GetExpiration(); t != nil {
		info.Expiration = t
	}
}

// Validate validates Azure credentials by calling Azure Resource Manager API.
// Returns validation info including subscription name, tenant ID, and expiration.
func (c *AzureCredentials) Validate(ctx context.Context) (*ValidationInfo, error) {
	// Check if subscription ID is provided.
	if c.SubscriptionID == "" {
		return nil, fmt.Errorf("%w: subscription ID is required", errUtils.ErrInvalidAuthConfig)
	}

	// Create a token credential from the access token.
	tokenCred := &staticTokenCredential{
		token: azcore.AccessToken{
			Token:     c.AccessToken,
			ExpiresOn: time.Time{}, // Will be validated via API call
		},
	}

	// Create subscriptions client with cloud configuration.
	subscriptionsClient, err := armsubscriptions.NewClient(tokenCred, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create Azure subscriptions client: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Get subscription details to validate credentials.
	result, err := subscriptionsClient.Get(ctx, c.SubscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to validate Azure credentials: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Build validation info from subscription response.
	info := &ValidationInfo{
		Account: c.TenantID,
	}

	// Set principal as subscription ID and name.
	if result.ID != nil {
		info.Principal = *result.ID
	}

	// Add expiration time if available.
	if expTime, err := c.GetExpiration(); err == nil && expTime != nil {
		info.Expiration = expTime
	}

	return info, nil
}

// staticTokenCredential implements azcore.TokenCredential for static access tokens.
type staticTokenCredential struct {
	token azcore.AccessToken
}

// GetToken returns the static access token.
func (c *staticTokenCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return c.token, nil
}
