package azure

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// acrTenantGUIDPattern matches an Entra ID tenant ID (a GUID without braces).
var acrTenantGUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// acrTenantDomainPattern matches a tenant domain name (for example
// contoso.onmicrosoft.com or contoso.com): two or more DNS labels of letters,
// digits and inner hyphens. Single-label values such as "common" or
// "organizations" are not tenants and do not match.
var acrTenantDomainPattern = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// acrTenantDomainMaxLength is the maximum length of a DNS name.
const acrTenantDomainMaxLength = 253

// acrDefaultPublicClientID is the Azure CLI public client ID, which the Atmos
// device-code and interactive providers use unless spec.client_id overrides it.
// It is the fallback when the identity's token does not name its client.
const acrDefaultPublicClientID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46"

//go:generate go run go.uber.org/mock/mockgen -destination=mock_acr_tenant_token_client_test.go -package=azure -source=acr_tenant.go

// acrTenantTokenClient is the part of the MSAL public client used to mint an
// AAD token in the registry's tenant from the identity's cached sign-in.
type acrTenantTokenClient interface {
	Accounts(ctx context.Context) ([]public.Account, error)
	AcquireTokenSilent(ctx context.Context, scopes []string, opts ...public.AcquireSilentOption) (public.AuthResult, error)
}

// newACRTenantTokenClient builds an MSAL public client for clientID at authority,
// backed by the realm's MSAL token cache: the same cache, and so the same refresh
// token, the Atmos device-code and interactive providers sign in with.
// Overridable in tests so no real Azure endpoint is called.
var newACRTenantTokenClient = func(clientID, authority, realm string) (acrTenantTokenClient, error) {
	msalCache, err := NewMSALCache("", realm)
	if err != nil {
		return nil, err
	}
	client, err := public.New(clientID, public.WithAuthority(authority), public.WithCache(msalCache))
	if err != nil {
		return nil, err
	}
	return client, nil
}

// ValidateACRTenantID checks an azure/acr spec.registry.tenant_id value: a tenant
// ID (GUID) or a tenant domain name such as contoso.onmicrosoft.com.
func ValidateACRTenantID(tenantID string) error {
	if acrTenantGUIDPattern.MatchString(tenantID) {
		return nil
	}
	if len(tenantID) <= acrTenantDomainMaxLength && acrTenantDomainPattern.MatchString(tenantID) {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: %q", errUtils.ErrACRInvalidTenant, tenantID)).
		WithHint("Set `spec.registry.tenant_id` to the registry's Entra ID tenant ID (a GUID) or a verified domain of that tenant, such as `contoso.onmicrosoft.com`.").
		Err()
}

// CredentialsForACRTenant returns Azure credentials whose access token is issued by
// tenantID, for the ACR OAuth2 token exchange of a registry in that tenant
// (spec.registry.tenant_id).
//
// The exchange sends the token together with the tenant that issued it, the way
// `az acr login` does, so overriding only the exchange's tenant field would not
// reach another tenant's registry. When tenantID is the identity's own tenant the
// credentials are returned unchanged. Otherwise a token for the Azure Resource
// Manager scope of the identity's cloud is minted silently in tenantID from the
// realm's MSAL cache, which only the device-code and interactive providers sign in
// to; other credentials fail with ErrACRTenantOverrideUnsupported. The input
// credentials are never modified.
func CredentialsForACRTenant(ctx context.Context, creds types.ICredentials, tenantID, realm string) (types.ICredentials, error) {
	defer perf.Track(nil, "azure.CredentialsForACRTenant")()

	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok || azureCreds == nil {
		return nil, fmt.Errorf("%w: expected Azure credentials", errUtils.ErrACRAuthFailed)
	}
	if err := ValidateACRTenantID(tenantID); err != nil {
		return nil, err
	}

	claims, _ := extractJWTClaims(azureCreds.AccessToken)
	if isTokenTenant(tenantID, azureCreds.TenantID, claims) {
		return creds, nil
	}

	switch azureCreds.AuthMethod {
	case types.AzureAuthMethodDeviceCode, types.AzureAuthMethodInteractive:
		return acquireACRTenantCredentials(ctx, azureCreds, claims, tenantID, realm)
	default:
		return nil, tenantOverrideUnsupportedError(azureCreds, tenantID)
	}
}

// isTokenTenant reports whether tenantID is the tenant the identity's token was
// issued by: the credentials' tenant or the token's tid claim.
func isTokenTenant(tenantID, credsTenantID string, claims map[string]interface{}) bool {
	if strings.EqualFold(tenantID, credsTenantID) {
		return true
	}
	tid, _ := claims["tid"].(string)
	return tid != "" && strings.EqualFold(tenantID, tid)
}

// acquireACRTenantCredentials mints an ARM-scoped token in tenantID for the cached
// account of the identity and returns a copy of the credentials carrying it.
func acquireACRTenantCredentials(ctx context.Context, azureCreds *types.AzureCredentials, claims map[string]interface{}, tenantID, realm string) (types.ICredentials, error) {
	cloudEnv := GetCloudEnvironment(azureCreds.CloudEnvironment)
	authority := "https://" + cloudEnv.LoginEndpoint + "/" + tenantID
	clientID := tokenClientID(claims)

	client, err := newACRTenantTokenClient(clientID, authority, realm)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create MSAL client for tenant %s: %w", errUtils.ErrACRAuthFailed, tenantID, err)
	}

	account, err := cachedAccountFor(ctx, client, azureCreds)
	if err != nil {
		return nil, tenantTokenError(err, tenantID, cloudEnv.Name)
	}

	result, err := client.AcquireTokenSilent(ctx, []string{cloudEnv.ManagementScope}, public.WithSilentAccount(account))
	if err != nil {
		return nil, tenantTokenError(err, tenantID, cloudEnv.Name)
	}

	log.Debug("Acquired AAD token in the ACR registry tenant", LogFieldTenantID, tenantID, LogFieldExpiresOn, result.ExpiresOn)

	out := *azureCreds
	out.AccessToken = result.AccessToken
	out.Expiration = result.ExpiresOn.Format(time.RFC3339)
	out.TenantID = tenantID
	// A domain tenant_id is sent to the exchange as the issuing tenant's ID.
	if newClaims, err := extractJWTClaims(result.AccessToken); err == nil {
		if tid, ok := newClaims["tid"].(string); ok && tid != "" {
			out.TenantID = tid
		}
	}
	return &out, nil
}

// tokenClientID returns the client the identity's token was issued to (appid in v1
// tokens, azp in v2 tokens), so the registry token is minted for the same client
// whose refresh token is cached. It falls back to the Azure CLI client.
func tokenClientID(claims map[string]interface{}) string {
	for _, claim := range []string{"appid", "azp"} {
		if id, ok := claims[claim].(string); ok && id != "" {
			return id
		}
	}
	return acrDefaultPublicClientID
}

// cachedAccountFor finds the identity's account in the MSAL cache: by its MSAL home
// account ID when the provider recorded it, otherwise by the identity's tenant.
func cachedAccountFor(ctx context.Context, client acrTenantTokenClient, azureCreds *types.AzureCredentials) (public.Account, error) {
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return public.Account{}, err
	}
	if len(accounts) == 0 {
		return public.Account{}, errUtils.ErrAzureNoAccountsInCache
	}
	for i := range accounts {
		if azureCreds.HomeAccountID != "" && accounts[i].HomeAccountID == azureCreds.HomeAccountID {
			return accounts[i], nil
		}
	}
	if azureCreds.HomeAccountID == "" {
		for i := range accounts {
			if strings.EqualFold(accounts[i].Realm, azureCreds.TenantID) {
				return accounts[i], nil
			}
		}
	}
	return public.Account{}, fmt.Errorf("%w: %s", errUtils.ErrAzureNoAccountForTenant, azureCreds.TenantID)
}

// tenantTokenError explains a failure to mint a token in the registry's tenant.
func tenantTokenError(cause error, tenantID, cloudName string) error {
	return errUtils.Build(errUtils.ErrACRAuthFailed).
		WithCause(cause).
		WithContext("tenant_id", tenantID).
		WithContext("cloud_environment", cloudName).
		WithExplanation("Could not get an Azure AD token in the registry's tenant (`spec.registry.tenant_id`) from the identity's cached sign-in.").
		WithHint("Run `atmos auth login` for the integration's identity, and make sure the signed-in account is a member or guest of that tenant. If the tenant requires its own sign-in (for example Conditional Access or MFA), configure a provider with that tenant's `tenant_id` instead.").
		Err()
}

// tenantOverrideUnsupportedError explains that the identity's credentials cannot
// mint a token in another tenant.
func tenantOverrideUnsupportedError(azureCreds *types.AzureCredentials, tenantID string) error {
	method := azureCreds.AuthMethod
	if method == "" {
		method = "unknown"
	}
	return errUtils.Build(fmt.Errorf("%w: tenant %s", errUtils.ErrACRTenantOverrideUnsupported, tenantID)).
		WithContext("identity_tenant_id", azureCreds.TenantID).
		WithContext("auth_method", method).
		WithExplanation("The ACR token exchange needs an Azure AD token issued by the registry's tenant. Atmos can only mint one in another tenant for identities whose provider is `azure/device-code` or `azure/interactive`.").
		WithHint("Use an identity whose provider `tenant_id` is the registry's tenant and remove `spec.registry.tenant_id`, or sign in through an `azure/device-code` or `azure/interactive` provider.").
		Err()
}
