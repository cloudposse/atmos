package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
)

// acrLoginUsername is the fixed username ACR expects when the password is an
// OAuth2 refresh token obtained via the token exchange below (docs: any
// value works over basic auth as long as the token type matches; ACR/Docker
// convention uses this all-zero GUID).
const acrLoginUsername = "00000000-0000-0000-0000-000000000000"

// acrRegistrySuffix is the ACR login server suffix in the Azure public cloud.
const acrRegistrySuffix = ".azurecr.io"

// acrRegistryPattern matches normalized (lowercase) ACR login server URLs in every
// Azure cloud: public (myregistry.azurecr.io), US Government (myregistry.azurecr.us),
// and China (myregistry.azurecr.cn).
var acrRegistryPattern = regexp.MustCompile(`^([a-z0-9]+)\.azurecr\.(?:io|us|cn)$`)

// acrHTTPClient is the HTTP client used for the ACR OAuth2 token exchange.
// Overridable in tests.
var acrHTTPClient httpClient.Client = httpClient.NewDefaultClient()

// ACRAuthResult contains ACR authorization information for Docker login.
type ACRAuthResult struct {
	Username  string    // Always acrLoginUsername.
	Password  string    //nolint:gosec // G117: This is an OAuth2 refresh token, not a hardcoded password secret.
	Registry  string    // The registry login server, e.g. myregistry.azurecr.io.
	ExpiresAt time.Time // Refresh token expiration time.
}

// acrExchangeResponse is the response body from the ACR OAuth2 token exchange endpoint.
type acrExchangeResponse struct {
	RefreshToken string `json:"refresh_token"` //nolint:gosec // G117: OAuth2 refresh token field name, not a hardcoded secret.
}

// GetAuthorizationToken exchanges an AAD access token for an ACR refresh
// token via the registry's OAuth2 endpoint, following the same
// username/password Docker login contract as `az acr login`.
// See: https://github.com/Azure/acr/blob/main/docs/AAD-OAuth.md
func GetAuthorizationToken(ctx context.Context, creds types.ICredentials, loginServer string) (*ACRAuthResult, error) {
	defer perf.Track(nil, "azure.GetAuthorizationToken")()

	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: expected Azure credentials", errUtils.ErrACRAuthFailed)
	}

	data := url.Values{}
	data.Set("grant_type", "access_token")
	data.Set("service", loginServer)
	data.Set("tenant", azureCreds.TenantID)
	data.Set("access_token", azureCreds.AccessToken)

	exchangeURL := "https://" + loginServer + "/oauth2/exchange"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exchangeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create ACR token exchange request: %w", errUtils.ErrACRAuthFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := acrHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to exchange ACR token: %w", errUtils.ErrACRAuthFailed, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read ACR token exchange response: %w", errUtils.ErrACRAuthFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: ACR token exchange returned status %d: %s", errUtils.ErrACRAuthFailed, resp.StatusCode, string(body))
	}

	var exchangeResp acrExchangeResponse
	if err := json.Unmarshal(body, &exchangeResp); err != nil {
		return nil, fmt.Errorf("%w: failed to decode ACR token exchange response: %w", errUtils.ErrACRAuthFailed, err)
	}

	if exchangeResp.RefreshToken == "" {
		return nil, fmt.Errorf("%w: empty refresh token in ACR token exchange response", errUtils.ErrACRAuthFailed)
	}

	// The refresh token has no expires_in in the exchange response; decode its
	// own JWT exp claim instead of assuming a fixed TTL.
	expiresAt := refreshTokenExpiry(exchangeResp.RefreshToken)

	return &ACRAuthResult{
		Username:  acrLoginUsername,
		Password:  exchangeResp.RefreshToken,
		Registry:  loginServer,
		ExpiresAt: expiresAt,
	}, nil
}

// refreshTokenExpiry decodes the "exp" claim from an ACR refresh token JWT.
// Returns the zero time if the token can't be decoded or has no exp claim.
func refreshTokenExpiry(refreshToken string) time.Time {
	claims, err := extractJWTClaims(refreshToken)
	if err != nil {
		return time.Time{}
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}
	}
	return time.Unix(int64(exp), 0)
}

// BuildRegistryURL constructs an ACR login server URL from a registry name.
func BuildRegistryURL(name string) string {
	defer perf.Track(nil, "azure.BuildRegistryURL")()

	return name + acrRegistrySuffix
}

// ParseRegistryURL extracts the registry name from an ACR login server URL.
// Returns error if the URL is not a valid ACR registry URL.
func ParseRegistryURL(registryURL string) (name string, err error) {
	defer perf.Track(nil, "azure.ParseRegistryURL")()

	name, _, err = parseLoginServer(registryURL)
	return name, err
}

// NormalizeRegistryURL validates an ACR login server URL from any Azure cloud and
// returns it in the form ACR and Docker expect: lowercase, without a scheme or a
// trailing slash (e.g. "HTTPS://MyRegistry.azurecr.us/" -> "myregistry.azurecr.us").
// Unlike BuildRegistryURL(ParseRegistryURL(u)), it keeps the cloud's suffix.
func NormalizeRegistryURL(registryURL string) (string, error) {
	defer perf.Track(nil, "azure.NormalizeRegistryURL")()

	_, loginServer, err := parseLoginServer(registryURL)
	return loginServer, err
}

// IsACRRegistry checks if a URL is an ACR registry login server URL.
func IsACRRegistry(registryURL string) bool {
	defer perf.Track(nil, "azure.IsACRRegistry")()

	_, _, err := parseLoginServer(registryURL)
	return err == nil
}

// parseLoginServer normalizes an ACR login server URL (DNS names and ACR registry
// names are case-insensitive) and returns the registry name and the login server.
func parseLoginServer(registryURL string) (name, loginServer string, err error) {
	normalized := strings.ToLower(strings.TrimSpace(registryURL))
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimSuffix(normalized, "/")

	matches := acrRegistryPattern.FindStringSubmatch(normalized)
	if len(matches) != 2 { //nolint:mnd
		return "", "", fmt.Errorf("%w: %s", errUtils.ErrACRInvalidRegistry, registryURL)
	}

	return matches[1], normalized, nil
}

// LoadDefaultAzureCredentials loads Azure credentials from the ambient
// environment (the Azure SDK default credential chain: environment
// variables, managed identity, workload identity, Azure CLI). Used for
// explicit-registry mode where the caller wants to use ambient Azure
// credentials instead of Atmos identities.
func LoadDefaultAzureCredentials(ctx context.Context) (*types.AzureCredentials, error) {
	defer perf.Track(nil, "azure.LoadDefaultAzureCredentials")()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create default Azure credential: %w", errUtils.ErrACRAuthFailed, err)
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{PublicCloud.ManagementScope}})
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrACRAuthFailed).
			WithCause(err).
			WithExplanation("Failed to retrieve Azure credentials from the ambient environment (env vars, managed identity, workload identity, or Azure CLI).").
			WithHint("If ambient credentials aren't available in this environment, configure an Atmos identity instead — run `atmos azure acr login --identity <name>`, or add `via.identity` under `auth.integrations` in atmos.yaml.").
			Err()
	}

	tenantID := ""
	if claims, err := extractJWTClaims(token.Token); err == nil {
		if tid, ok := claims["tid"].(string); ok {
			tenantID = tid
		}
	}

	return &types.AzureCredentials{
		AccessToken: token.Token,
		TokenType:   "Bearer",
		Expiration:  token.ExpiresOn.Format(time.RFC3339),
		TenantID:    tenantID,
	}, nil
}
