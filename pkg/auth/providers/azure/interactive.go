package azure

import (
	"context"
	"fmt"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// interactiveProvider implements Azure Entra ID interactive browser authentication
// (authorization code + PKCE on a localhost redirect) — the same flow `az login`
// uses. Unlike the device code flow, it works in tenants where Conditional Access
// blocks device code (e.g. the Microsoft-managed "Block device code flow" policy,
// AADSTS 530035), because the browser session carries full Conditional Access
// context.
//
// It embeds deviceCodeProvider to reuse the MSAL client, silent token
// acquisition, additional-token fan-out, and Azure CLI cache write-back; only
// the interactive acquisition step differs.
type interactiveProvider struct {
	*deviceCodeProvider

	// Seams for testing: the MSAL interactive acquisition needs a live identity
	// provider and a browser, so tests inject fakes here (per the repo's
	// dependency-injection-for-testability convention).
	acquireInteractive func(ctx context.Context, client *public.Client, scopes []string) (public.AuthResult, error)
	checkInteractive   func() bool
}

// NewInteractiveProvider creates a new Azure interactive browser provider.
func NewInteractiveProvider(name string, config *schema.Provider) (*interactiveProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != authTypes.ProviderKindAzureInteractive {
		return nil, fmt.Errorf("%w: invalid provider kind for Azure interactive provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// The spec shape is shared with azure/device-code.
	cfg := extractDeviceCodeConfig(config.Spec)

	// Tenant ID is required.
	if cfg.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required in spec for Azure interactive provider", errUtils.ErrInvalidProviderConfig)
	}

	// Validate cloud_environment if specified.
	if err := azureCloud.ValidateCloudEnvironment(cfg.CloudEnvironment); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidProviderConfig, err)
	}

	p := &interactiveProvider{
		deviceCodeProvider: &deviceCodeProvider{
			name:           name,
			config:         config,
			tenantID:       cfg.TenantID,
			subscriptionID: cfg.SubscriptionID,
			location:       cfg.Location,
			clientID:       cfg.ClientID,
			cloudEnv:       azureCloud.GetCloudEnvironment(cfg.CloudEnvironment),
			cacheStorage:   &defaultCacheStorage{},
			authMethod:     authTypes.AzureAuthMethodInteractive,
		},
	}
	p.acquireInteractive = func(ctx context.Context, client *public.Client, scopes []string) (public.AuthResult, error) {
		return client.AcquireTokenInteractive(ctx, scopes)
	}
	p.checkInteractive = isInteractive
	return p, nil
}

// Kind returns the provider kind.
func (p *interactiveProvider) Kind() string {
	return authTypes.ProviderKindAzureInteractive
}

// Authenticate performs Azure interactive browser authentication using MSAL.
// It tries silent acquisition from the persisted MSAL cache first (refresh
// tokens survive restarts), then opens the default browser for an
// authorization-code + PKCE sign-in.
func (p *interactiveProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.interactiveProvider.Authenticate")()

	log.Debug(
		"Authenticating with Azure interactive browser flow",
		"provider", p.name,
		azureCloud.LogFieldTenantID, p.tenantID,
		"clientID", p.clientID,
	)

	client, err := p.createMSALClient()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create MSAL client: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Try silent authentication first (uses cached tokens/refresh tokens).
	accounts, err := client.Accounts(ctx)
	if err != nil {
		log.Debug("Failed to get cached accounts, will proceed with interactive flow", "error", err)
	}
	tokens := p.trySilentTokenAcquisition(ctx, &client, accounts)

	// If silent acquisition failed, open the browser.
	if tokens.accessToken == "" {
		tokens, err = p.acquireTokensInteractively(ctx, &client)
		if err != nil {
			return nil, err
		}
	}

	// Update Azure CLI token cache so Terraform can use it automatically.
	// Note: MSAL already persisted tokens (including refresh tokens) to the
	// realm-scoped Atmos MSAL cache.
	if err := p.updateAzureCLICache(&tokenCacheUpdate{
		AccessToken:       tokens.accessToken,
		ExpiresAt:         tokens.expiresOn,
		GraphToken:        tokens.graphToken,
		GraphExpiresAt:    tokens.graphExpiresOn,
		KeyVaultToken:     tokens.keyVaultToken,
		KeyVaultExpiresAt: tokens.keyVaultExpiresOn,
		HomeAccountID:     tokens.homeAccountID,
	}); err != nil {
		log.Debug("Failed to update Azure CLI token cache", "error", err)
	}

	return p.createCredentials(&tokens)
}

// acquireTokensInteractively opens the default browser for an
// authorization-code + PKCE sign-in and then acquires the additional API
// tokens (Graph, Key Vault) silently.
func (p *interactiveProvider) acquireTokensInteractively(ctx context.Context, client *public.Client) (tokenAcquisitionResult, error) {
	result := tokenAcquisitionResult{}

	// The flow needs a browser and a local redirect listener; refuse in
	// headless environments where neither can work.
	if !p.checkInteractive() {
		return result, fmt.Errorf("%w: Azure interactive browser authentication requires an interactive session (no TTY detected). Use azure/oidc in CI/CD, or azure/device-code for browser-less sessions where tenant policy allows it", errUtils.ErrAuthenticationFailed)
	}

	authCtx, cancel := context.WithTimeout(ctx, deviceCodeTimeout)
	defer cancel()

	log.Debug("Opening browser for Azure sign-in", azureCloud.LogFieldTenantID, p.tenantID)
	displayBrowserPrompt()

	mgmtResult, err := p.acquireInteractive(authCtx, client, []string{p.cloudEnv.ManagementScope})
	if err != nil {
		return result, fmt.Errorf("%w: interactive browser authentication failed: %w", errUtils.ErrAuthenticationFailed, err)
	}

	result.accessToken = mgmtResult.AccessToken
	result.expiresOn = mgmtResult.ExpiresOn
	result.homeAccountID = mgmtResult.Account.HomeAccountID
	log.Debug("Authentication successful", azureCloud.LogFieldExpiresOn, result.expiresOn)

	// Acquire additional API tokens for azuread and azurerm providers.
	accounts, err := client.Accounts(ctx)
	if err != nil || len(accounts) == 0 {
		log.Debug("Failed to get authenticated account, will skip Graph and KeyVault tokens", "error", err)
		return result, nil
	}
	p.acquireAdditionalTokens(ctx, client, accounts, &result)

	return result, nil
}
