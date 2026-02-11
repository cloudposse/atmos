package gcp_wif

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ProviderKind is the kind identifier for this provider.
	ProviderKind = types.ProviderKindGCPWorkloadIdentityFederation

	// DefaultScope is the default OAuth scope.
	DefaultScope = "https://www.googleapis.com/auth/cloud-platform"

	// Google STS endpoint for token exchange.
	stsEndpoint = "https://sts.googleapis.com/v1/token"

	// Token source types.
	TokenSourceTypeEnvironment = "environment"
	TokenSourceTypeFile        = "file"
	TokenSourceTypeURL         = "url"

	// Token request timeout in seconds.
	TokenRequestTimeout = 10
)

// iamServiceFactory abstracts IAM credential service creation for testability.
type iamServiceFactory func(ctx context.Context, accessToken string) (gcp.IAMCredentialsService, error)

// Provider implements the gcp/workload-identity-federation provider.
type Provider struct {
	name              string
	spec              *types.GCPWorkloadIdentityFederationProviderSpec
	httpClient        *http.Client
	stsURL            string
	iamServiceFactory iamServiceFactory
}

// New creates a new WIF provider from the given spec.
func New(spec *types.GCPWorkloadIdentityFederationProviderSpec) (*Provider, error) {
	defer perf.Track(nil, "gcp_wif.New")()

	if spec == nil {
		return nil, fmt.Errorf("%w: WIF provider spec cannot be nil", errUtils.ErrInvalidProviderConfig)
	}
	return &Provider{
		spec:              spec,
		httpClient:        &http.Client{Timeout: TokenRequestTimeout * time.Second},
		stsURL:            stsEndpoint,
		iamServiceFactory: gcp.NewIAMCredentialsService,
	}, nil
}

// SetName sets the provider name.
func (p *Provider) SetName(name string) {
	p.name = name
}

// WithHTTPClient sets the HTTP client used for token requests.
func (p *Provider) WithHTTPClient(client *http.Client) *Provider {
	p.httpClient = client
	return p
}

// Kind returns the provider kind.
func (p *Provider) Kind() string {
	return ProviderKind
}

// Name returns the provider name.
func (p *Provider) Name() string {
	if p.name != "" {
		return p.name
	}
	return p.Kind()
}

// poolID returns the workload identity pool ID (prefers _id field, falls back to legacy).
func (p *Provider) poolID() string {
	if p.spec.WorkloadIdentityPoolID != "" {
		return p.spec.WorkloadIdentityPoolID
	}
	return p.spec.WorkloadIdentityPool
}

// providerID returns the workload identity provider ID (prefers _id field, falls back to legacy).
func (p *Provider) providerID() string {
	if p.spec.WorkloadIdentityProviderID != "" {
		return p.spec.WorkloadIdentityProviderID
	}
	return p.spec.WorkloadIdentityProvider
}

// Validate validates the provider configuration.
func (p *Provider) Validate() error {
	if p.spec == nil {
		return fmt.Errorf("%w: spec is nil", errUtils.ErrInvalidProviderConfig)
	}
	if p.spec.ProjectNumber == "" {
		return fmt.Errorf("%w: project_number is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.poolID() == "" {
		return fmt.Errorf("%w: workload_identity_pool_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.providerID() == "" {
		return fmt.Errorf("%w: workload_identity_provider_id is required", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// PreAuthenticate performs pre-authentication checks.
func (p *Provider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// Authenticate obtains GCP credentials via WIF token exchange.
func (p *Provider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "gcp_wif.Authenticate")()

	if err := p.Validate(); err != nil {
		return nil, err
	}

	// Step 1: Get the OIDC token from the configured source.
	oidcToken, err := p.getOIDCToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: get OIDC token: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Step 2: Exchange OIDC token for federated access token via STS.
	federatedToken, err := p.exchangeToken(ctx, oidcToken)
	if err != nil {
		return nil, fmt.Errorf("%w: token exchange: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Step 3: If service account specified, impersonate to get final token.
	var accessToken string
	var expiry time.Time
	var saEmail string

	if p.spec.ServiceAccountEmail != "" {
		accessToken, expiry, err = p.impersonateServiceAccount(ctx, federatedToken)
		if err != nil {
			return nil, fmt.Errorf("%w: impersonate SA: %w", errUtils.ErrAuthenticationFailed, err)
		}
		saEmail = p.spec.ServiceAccountEmail
	} else {
		accessToken = federatedToken.AccessToken
		expiry = federatedToken.Expiry
	}

	return &types.GCPCredentials{
		AccessToken:         accessToken,
		TokenExpiry:         expiry,
		ProjectID:           p.spec.ProjectID,
		ServiceAccountEmail: saEmail,
		Scopes:              p.getScopes(),
	}, nil
}

// getOIDCToken retrieves the OIDC token from the configured source.
func (p *Provider) getOIDCToken(ctx context.Context) (string, error) {
	defer perf.Track(nil, "gcp_wif.getOIDCToken")()

	if p.spec.TokenSource == nil {
		return "", fmt.Errorf("%w: token_source not configured", errUtils.ErrInvalidProviderConfig)
	}

	switch p.spec.TokenSource.Type {
	case TokenSourceTypeEnvironment, "":
		return p.getTokenFromEnv()
	case TokenSourceTypeFile:
		return p.getTokenFromFile()
	case TokenSourceTypeURL:
		return p.getTokenFromURL(ctx)
	default:
		return "", fmt.Errorf("%w: unknown token source type: %s", errUtils.ErrInvalidProviderConfig, p.spec.TokenSource.Type)
	}
}

func (p *Provider) getTokenFromEnv() (string, error) {
	envVar := p.spec.TokenSource.EnvironmentVariable
	if envVar == "" {
		// No default - environment_variable must be explicitly configured.
		// Note: For GitHub Actions, use token_source.type=url instead, which fetches the OIDC token from ACTIONS_ID_TOKEN_REQUEST_URL.
		return "", fmt.Errorf("%w: environment_variable must be specified for token_source.type=environment", errUtils.ErrInvalidProviderConfig)
	}
	token := strings.TrimSpace(os.Getenv(envVar))
	if token == "" {
		return "", fmt.Errorf("%w: environment variable %s is empty", errUtils.ErrAuthenticationFailed, envVar)
	}
	return token, nil
}

func (p *Provider) getTokenFromFile() (string, error) {
	if p.spec.TokenSource.FilePath == "" {
		return "", fmt.Errorf("%w: file_path not configured", errUtils.ErrInvalidProviderConfig)
	}
	data, err := os.ReadFile(p.spec.TokenSource.FilePath)
	if err != nil {
		return "", fmt.Errorf("%w: read token file: %w", errUtils.ErrAuthenticationFailed, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("%w: token is empty", errUtils.ErrAuthenticationFailed)
	}
	return token, nil
}

func (p *Provider) getTokenFromURL(ctx context.Context) (string, error) {
	defer perf.Track(nil, "gcp_wif.getTokenFromURL")()

	tokenURL := p.spec.TokenSource.URL
	fromEnv := false
	if tokenURL == "" {
		// GitHub Actions default.
		tokenURL = os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		fromEnv = true
	}
	if tokenURL == "" {
		return "", fmt.Errorf("%w: token URL not configured", errUtils.ErrInvalidProviderConfig)
	}

	u, err := url.Parse(tokenURL)
	if err != nil {
		return "", fmt.Errorf("%w: parse token URL: %w", errUtils.ErrInvalidProviderConfig, err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("%w: token URL must use https", errUtils.ErrInvalidProviderConfig)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("%w: token URL host is required", errUtils.ErrInvalidProviderConfig)
	}
	allowedHosts := p.spec.TokenSource.AllowedHosts
	if fromEnv && len(allowedHosts) == 0 {
		allowedHosts = []string{"token.actions.githubusercontent.com"}
	}
	if len(allowedHosts) > 0 && !hostAllowed(u, allowedHosts) {
		return "", fmt.Errorf("%w: token URL host %q is not allowed; set token_source.allowed_hosts to override", errUtils.ErrInvalidProviderConfig, u.Hostname())
	}

	// Add audience parameter if specified.
	if p.spec.TokenSource.Audience != "" {
		q := u.Query()
		q.Set("audience", p.spec.TokenSource.Audience)
		u.RawQuery = q.Encode()
		tokenURL = u.String()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: create request: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Add bearer token if provided.
	bearerToken := p.spec.TokenSource.RequestToken
	if bearerToken == "" {
		bearerToken = os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := p.getHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: request token: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: token request failed: %s: %s", errUtils.ErrAuthenticationFailed, resp.Status, string(body))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%w: decode response: %w", errUtils.ErrAuthenticationFailed, err)
	}
	token := strings.TrimSpace(result.Value)
	if token == "" {
		return "", fmt.Errorf("%w: token is empty", errUtils.ErrAuthenticationFailed)
	}
	return token, nil
}

func hostAllowed(u *url.URL, allowedHosts []string) bool {
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	for _, allowed := range allowedHosts {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == "" {
			continue
		}
		if strings.EqualFold(allowed, u.Host) || strings.EqualFold(allowed, host) {
			return true
		}
	}
	return false
}

// exchangeToken exchanges an OIDC token for a Google federated token via STS.
func (p *Provider) exchangeToken(ctx context.Context, oidcToken string) (*oauth2.Token, error) {
	defer perf.Track(nil, "gcp_wif.exchangeToken")()

	audience := fmt.Sprintf(
		"//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s",
		p.spec.ProjectNumber,
		p.poolID(),
		p.providerID(),
	)

	// Use configured scopes or default.
	scopes := p.getScopes()

	data := url.Values{
		"grant_type":           {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"audience":             {audience},
		"scope":                {strings.Join(scopes, " ")},
		"requested_token_type": {"urn:ietf:params:oauth:token-type:access_token"},
		"subject_token_type":   {"urn:ietf:params:oauth:token-type:jwt"},
		"subject_token":        {oidcToken},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.getStsURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: create STS request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.getHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: STS request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: STS error: %s: %s", errUtils.ErrAuthenticationFailed, resp.Status, string(body))
	}

	var stsResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stsResp); err != nil {
		return nil, fmt.Errorf("%w: decode STS response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	return &oauth2.Token{
		AccessToken: stsResp.AccessToken,
		TokenType:   stsResp.TokenType,
		Expiry:      time.Now().Add(time.Duration(stsResp.ExpiresIn) * time.Second),
	}, nil
}

// impersonateServiceAccount uses IAM Credentials API to get an access token for the SA.
func (p *Provider) impersonateServiceAccount(ctx context.Context, federatedToken *oauth2.Token) (string, time.Time, error) {
	defer perf.Track(nil, "gcp_wif.impersonateServiceAccount")()

	factory := p.iamServiceFactory
	if factory == nil {
		factory = gcp.NewIAMCredentialsService
	}
	svc, err := factory(ctx, federatedToken.AccessToken)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: create IAM credentials service: %w", errUtils.ErrAuthenticationFailed, err)
	}

	accessToken, expiry, err := gcp.ImpersonateServiceAccount(
		ctx,
		svc,
		p.spec.ServiceAccountEmail,
		p.getScopes(),
		nil,
		"",
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %w", errUtils.ErrAuthenticationFailed, err)
	}

	return accessToken, expiry, nil
}

func (p *Provider) getScopes() []string {
	if len(p.spec.Scopes) > 0 {
		return p.spec.Scopes
	}
	return []string{DefaultScope}
}

// Environment returns environment variables for this provider.
func (p *Provider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	if p.spec != nil && p.spec.ProjectID != "" {
		for key, value := range gcp.ProjectEnvVars(p.spec.ProjectID) {
			env[key] = value
		}
	}
	return env, nil
}

// Paths returns file paths used by this provider.
func (p *Provider) Paths() ([]types.Path, error) {
	return nil, nil
}

// PrepareEnvironment prepares the environment for authentication.
func (p *Provider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "gcp_wif.PrepareEnvironment")()

	result := maps.Clone(environ)
	if result == nil {
		result = make(map[string]string)
	}
	if p.spec != nil && p.spec.ProjectID != "" {
		for key, value := range gcp.ProjectEnvVars(p.spec.ProjectID) {
			result[key] = value
		}
	}
	return result, nil
}

func (p *Provider) getHTTPClient() *http.Client {
	if p.httpClient != nil {
		return p.httpClient
	}
	return &http.Client{Timeout: TokenRequestTimeout * time.Second}
}

func (p *Provider) getStsURL() string {
	if p.stsURL != "" {
		return p.stsURL
	}
	return stsEndpoint
}

// Logout is a no-op for WIF provider.
func (p *Provider) Logout(ctx context.Context) error {
	return nil
}

// GetFilesDisplayPath returns empty string (no local files).
func (p *Provider) GetFilesDisplayPath() string {
	return ""
}
