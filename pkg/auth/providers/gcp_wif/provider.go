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
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"

	errUtils "github.com/cloudposse/atmos/errors"
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
)

// Provider implements the gcp/workload-identity-federation provider.
type Provider struct {
	name string
	spec *types.GCPWorkloadIdentityFederationProviderSpec
}

// New creates a new WIF provider from the given spec.
func New(spec *types.GCPWorkloadIdentityFederationProviderSpec) (*Provider, error) {
	defer perf.Track(nil, "gcp_wif.New")()

	if spec == nil {
		return nil, fmt.Errorf("%w: WIF provider spec cannot be nil", errUtils.ErrInvalidProviderConfig)
	}
	return &Provider{spec: spec}, nil
}

// SetName sets the provider name.
func (p *Provider) SetName(name string) {
	p.name = name
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
		return "", fmt.Errorf("token_source not configured")
	}

	switch p.spec.TokenSource.Type {
	case TokenSourceTypeEnvironment, "":
		return p.getTokenFromEnv()
	case TokenSourceTypeFile:
		return p.getTokenFromFile()
	case TokenSourceTypeURL:
		return p.getTokenFromURL(ctx)
	default:
		return "", fmt.Errorf("unknown token source type: %s", p.spec.TokenSource.Type)
	}
}

func (p *Provider) getTokenFromEnv() (string, error) {
	envVar := p.spec.TokenSource.EnvironmentVariable
	if envVar == "" {
		// No default - environment_variable must be explicitly configured.
		// Note: For GitHub Actions, use token_source.type=url instead, which
		// fetches the OIDC token from ACTIONS_ID_TOKEN_REQUEST_URL.
		return "", fmt.Errorf("environment_variable must be specified for token_source.type=environment")
	}
	token := os.Getenv(envVar)
	if token == "" {
		return "", fmt.Errorf("environment variable %s is empty", envVar)
	}
	return token, nil
}

func (p *Provider) getTokenFromFile() (string, error) {
	if p.spec.TokenSource.FilePath == "" {
		return "", fmt.Errorf("file_path not configured")
	}
	data, err := os.ReadFile(p.spec.TokenSource.FilePath)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (p *Provider) getTokenFromURL(ctx context.Context) (string, error) {
	defer perf.Track(nil, "gcp_wif.getTokenFromURL")()

	tokenURL := p.spec.TokenSource.URL
	if tokenURL == "" {
		// GitHub Actions default.
		tokenURL = os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	}
	if tokenURL == "" {
		return "", fmt.Errorf("token URL not configured")
	}

	// Add audience parameter if specified.
	if p.spec.TokenSource.Audience != "" {
		u, err := url.Parse(tokenURL)
		if err != nil {
			return "", fmt.Errorf("parse token URL: %w", err)
		}
		q := u.Query()
		q.Set("audience", p.spec.TokenSource.Audience)
		u.RawQuery = q.Encode()
		tokenURL = u.String()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Add bearer token if provided.
	bearerToken := p.spec.TokenSource.RequestToken
	if bearerToken == "" {
		bearerToken = os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request failed: %s: %s", resp.Status, string(body))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return result.Value, nil
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stsEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create STS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("STS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("STS error: %s: %s", resp.Status, string(body))
	}

	var stsResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stsResp); err != nil {
		return nil, fmt.Errorf("decode STS response: %w", err)
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

	// Create IAM Credentials client with the federated token.
	svc, err := iamcredentials.NewService(ctx,
		option.WithTokenSource(oauth2.StaticTokenSource(federatedToken)),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create IAM credentials service: %w", err)
	}

	name := fmt.Sprintf("projects/-/serviceAccounts/%s", p.spec.ServiceAccountEmail)
	req := &iamcredentials.GenerateAccessTokenRequest{
		Scope: p.getScopes(),
	}

	resp, err := svc.Projects.ServiceAccounts.GenerateAccessToken(name, req).Context(ctx).Do()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate access token: %w", err)
	}

	expiry, err := time.Parse(time.RFC3339, resp.ExpireTime)
	if err != nil {
		expiry = time.Now().Add(1 * time.Hour) // Default 1 hour.
	}

	return resp.AccessToken, expiry, nil
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
		env["GOOGLE_CLOUD_PROJECT"] = p.spec.ProjectID
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
		result["GOOGLE_CLOUD_PROJECT"] = p.spec.ProjectID
	}
	return result, nil
}

// Logout is a no-op for WIF provider.
func (p *Provider) Logout(ctx context.Context) error {
	return nil
}

// GetFilesDisplayPath returns empty string (no local files).
func (p *Provider) GetFilesDisplayPath() string {
	return ""
}
