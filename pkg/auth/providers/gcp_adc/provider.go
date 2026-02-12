package gcp_adc

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	oauth2v2 "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ProviderKind is the kind identifier for this provider.
	ProviderKind = types.ProviderKindGCPADC // "gcp/adc"

	// DefaultScope is the default OAuth scope.
	DefaultScope = "https://www.googleapis.com/auth/cloud-platform"
)

// credentialsFinder abstracts google.FindDefaultCredentials for testability.
type credentialsFinder func(ctx context.Context, scopes ...string) (*google.Credentials, error)

// tokenEmailFetcher abstracts the OAuth2 tokeninfo call for testability.
type tokenEmailFetcher func(ctx context.Context, accessToken string) (string, error)

// Provider implements the gcp/adc authentication provider.
type Provider struct {
	name            string
	realm           string
	spec            *types.GCPADCProviderSpec
	findCredentials credentialsFinder
	fetchTokenEmail tokenEmailFetcher
}

// New creates a new ADC provider from the given spec.
func New(spec *types.GCPADCProviderSpec) (*Provider, error) {
	defer perf.Track(nil, "gcp_adc.New")()

	if spec == nil {
		return nil, fmt.Errorf("%w: GCP ADC provider spec cannot be nil", errUtils.ErrInvalidProviderConfig)
	}
	return &Provider{
		spec:            spec,
		findCredentials: google.FindDefaultCredentials,
		fetchTokenEmail: getTokenEmail,
	}, nil
}

// SetName sets the provider name (used by the factory when registering).
func (p *Provider) SetName(name string) {
	p.name = name
}

// SetRealm satisfies the Provider interface. ADC is realm-independent because it
// relies on external credential sources (GOOGLE_APPLICATION_CREDENTIALS, gcloud
// config, or metadata server) and performs no credential file I/O. The value is
// stored but not used in behavior.
func (p *Provider) SetRealm(realm string) {
	p.realm = realm
}

// Kind returns the provider kind.
func (p *Provider) Kind() string {
	return ProviderKind
}

// Name returns the provider name as defined in configuration.
func (p *Provider) Name() string {
	if p.name != "" {
		return p.name
	}
	return ProviderKind
}

// PreAuthenticate is a no-op for GCP ADC provider.
func (p *Provider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// Authenticate obtains credentials from ADC.
func (p *Provider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "gcp_adc.Authenticate")()

	if err := p.Validate(); err != nil {
		return nil, err
	}

	scopes := p.spec.Scopes
	if len(scopes) == 0 {
		scopes = []string{DefaultScope}
	}

	// Find default credentials using ADC chain.
	creds, err := p.findCredentials(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("%w: find default credentials: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Get a token to verify credentials work and get expiry.
	token, err := creds.TokenSource.Token()
	if err != nil {
		authErr := fmt.Errorf("%w: get token from ADC: %w", errUtils.ErrAuthenticationFailed, err)
		if isADCReauthError(err) {
			return nil, errUtils.Build(authErr).
				WithExplanation("Your Google application-default credentials have expired or require reauthentication. Run `gcloud auth application-default login` and try again.").
				Err()
		}
		return nil, authErr
	}

	// Determine project ID (spec override > ADC project > empty).
	projectID := p.spec.ProjectID
	if projectID == "" && creds.ProjectID != "" {
		projectID = creds.ProjectID
	}

	// Optionally get service account email via tokeninfo (best-effort).
	var saEmail string
	if token.AccessToken != "" {
		saEmail, _ = p.fetchTokenEmail(ctx, token.AccessToken)
	}

	return &types.GCPCredentials{
		AccessToken:         token.AccessToken,
		TokenExpiry:         token.Expiry,
		ProjectID:           projectID,
		ServiceAccountEmail: saEmail,
		Scopes:              scopes,
	}, nil
}

func isADCReauthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid_grant") && strings.Contains(msg, "invalid_rapt")
}

// Validate validates the provider configuration.
func (p *Provider) Validate() error {
	if p.spec == nil {
		return fmt.Errorf("%w: GCP ADC provider spec cannot be nil", errUtils.ErrInvalidProviderConfig)
	}
	return nil
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

// Paths returns credential files/directories used by this provider.
// ADC uses GOOGLE_APPLICATION_CREDENTIALS or gcloud config; this provider does not manage paths.
func (p *Provider) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// Returns a copy of environ; ADC is resolved at auth time, so no provider-specific env is required here.
func (p *Provider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "gcp_adc.PrepareEnvironment")()

	out := maps.Clone(environ)
	if out == nil {
		out = make(map[string]string)
	}
	if p.spec != nil && p.spec.ProjectID != "" {
		for key, value := range gcp.ProjectEnvVars(p.spec.ProjectID) {
			out[key] = value
		}
	}
	return out, nil
}

// Logout removes provider-specific credential storage.
// ADC credentials are managed by gcloud or env; this provider does not store credentials.
func (p *Provider) Logout(ctx context.Context) error {
	return nil
}

// GetFilesDisplayPath returns the display path for credential files.
func (p *Provider) GetFilesDisplayPath() string {
	return ""
}

// getTokenEmail retrieves the email associated with an access token via the OAuth2 tokeninfo API.
func getTokenEmail(ctx context.Context, accessToken string) (string, error) {
	defer perf.Track(nil, "gcp_adc.getTokenEmail")()

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	svc, err := oauth2v2.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", err
	}

	tokenInfo, err := svc.Tokeninfo().AccessToken(accessToken).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return tokenInfo.Email, nil
}
