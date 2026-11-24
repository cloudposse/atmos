package types

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
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
	// CloudEnvironment is the Azure cloud environment name ("public", "usgovernment", "china").
	// Used to select correct endpoints when writing MSAL cache entries.
	CloudEnvironment string `json:"cloud_environment,omitempty"`
	// AKSToken is an AAD access token scoped to the AKS-managed server
	// application, used by `atmos azure aks token` to authenticate kubectl.
	AKSToken string `json:"aks_token,omitempty"`
	// AKSTokenExpiration is the RFC3339 expiration timestamp for AKSToken.
	AKSTokenExpiration string `json:"aks_token_expiration,omitempty"`
	// AuthMethod records which provider kind minted these credentials
	// (AzureAuthMethodCLI, AzureAuthMethodDeviceCode, AzureAuthMethodOIDC).
	// Credentials that originated from the Azure CLI must not be written back
	// into the CLI's own cache files.
	AuthMethod string `json:"auth_method,omitempty"`
	// HomeAccountID is the MSAL home account identifier ("{home-oid}.{home-tenant-id}").
	// For guest (B2B) users the home tenant differs from the tenant being accessed;
	// deriving the value from the target tenant instead creates a duplicate Azure CLI
	// cache Account entry with the same username, which breaks every az command
	// (https://github.com/Azure/azure-cli/issues/20168). Populated by providers that
	// receive it from MSAL.
	HomeAccountID string `json:"home_account_id,omitempty"`

	// validationClientOptions overrides the Resource Manager client options Validate uses.
	// Runtime only (tests point it at a local server); it is never serialized.
	validationClientOptions *arm.ClientOptions
}

// Azure credential auth methods (values of AzureCredentials.AuthMethod).
const (
	AzureAuthMethodCLI         = "cli"
	AzureAuthMethodDeviceCode  = "device_code"
	AzureAuthMethodInteractive = "interactive"
	AzureAuthMethodOIDC        = "oidc"
)

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
//
// Like AWSCredentials.Validate, only a definitive answer marks the credentials invalid:
// expired credentials fail with ErrExpiredCredentials without a network call, and a token
// Resource Manager rejects fails with ErrAuthenticationFailed. A validation that cannot be
// performed (no subscription to query, Resource Manager unreachable, an answer about the
// subscription rather than the token) wraps ErrCredentialValidationUnavailable, so callers
// fall back to the cached expiration time.
func (c *AzureCredentials) Validate(ctx context.Context) (*ValidationInfo, error) {
	if c.IsExpired() {
		return nil, fmt.Errorf("%w: Azure credentials expired at %s", errUtils.ErrExpiredCredentials, c.Expiration)
	}

	// Without a subscription there is nothing to ask Resource Manager about.
	if c.SubscriptionID == "" {
		return nil, fmt.Errorf("%w: no Azure subscription is set, so the credentials cannot be checked against Azure Resource Manager", errUtils.ErrCredentialValidationUnavailable)
	}

	// Create a token credential from the access token.
	tokenCred := &StaticTokenCredential{
		Token: azcore.AccessToken{
			Token:     c.AccessToken,
			ExpiresOn: time.Time{}, // Will be validated via API call
		},
	}

	// Create subscriptions client with cloud configuration. The status recorder keeps the HTTP
	// status of the last answer, so a 401 whose body cannot be read is still recognized.
	options := *c.armValidationOptions()
	status := &azureStatusRecorder{}
	options.PerRetryPolicies = append(append([]policy.Policy{}, options.PerRetryPolicies...), status)
	subscriptionsClient, err := armsubscriptions.NewClient(tokenCred, &options)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create Azure subscriptions client: %w", errUtils.ErrCredentialValidationUnavailable, err)
	}

	callCtx, cancel := context.WithTimeout(ctx, azureValidationTimeout)
	defer cancel()

	// Get subscription details to validate credentials.
	result, err := subscriptionsClient.Get(callCtx, c.SubscriptionID, nil)
	if err != nil {
		if isAzureTokenRejection(err, status.statusCode) {
			return nil, fmt.Errorf("%w: failed to validate Azure credentials: %w", errUtils.ErrAuthenticationFailed, err)
		}
		return nil, fmt.Errorf("%w: Azure Resource Manager could not confirm the credentials: %w", errUtils.ErrCredentialValidationUnavailable, err)
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

// StaticTokenCredential implements azcore.TokenCredential for static access
// tokens. Exported so other packages (e.g. pkg/auth/cloud/azure) can build
// ARM SDK clients from an already-acquired Atmos credential without
// duplicating this wrapper.
type StaticTokenCredential struct {
	Token azcore.AccessToken
}

// GetToken returns the static access token.
func (c *StaticTokenCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return c.Token, nil
}
