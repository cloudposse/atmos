package gcp_service_account

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// IdentityKind is the kind identifier for this identity.
	IdentityKind = types.IdentityKindGCPServiceAccount // "gcp/service-account"

	// DefaultScope is the default OAuth scope.
	DefaultScope = "https://www.googleapis.com/auth/cloud-platform"

	// DefaultLifetime is the default token lifetime.
	DefaultLifetime = "3600s" // 1 hour.
)

// Identity implements the gcp/service-account identity.
type Identity struct {
	name              string
	realm             string
	principal         *types.GCPServiceAccountIdentityPrincipal
	provider          types.Provider
	config            *schema.Identity
	iamServiceFactory IAMCredentialsServiceFactory
}

// New creates a new service account identity.
func New(principal *types.GCPServiceAccountIdentityPrincipal) (*Identity, error) {
	defer perf.Track(nil, "gcp_service_account.New")()

	if principal == nil {
		return nil, fmt.Errorf("%w: service account principal cannot be nil", errUtils.ErrInvalidIdentityConfig)
	}
	return &Identity{
		principal:         principal,
		realm:             "default",
		iamServiceFactory: newIAMCredentialsService,
	}, nil
}

// SetName sets the identity name.
func (i *Identity) SetName(name string) {
	i.name = name
}

// SetRealm sets the credential realm for filesystem isolation.
func (i *Identity) SetRealm(realm string) {
	i.realm = realm
}

func (i *Identity) realmOrDefault() string {
	if i.realm == "" {
		return "default"
	}
	return i.realm
}

// Kind returns the identity kind.
func (i *Identity) Kind() string {
	return IdentityKind
}

// Name returns the identity name.
func (i *Identity) Name() string {
	if i.name != "" {
		return i.name
	}
	return i.Kind()
}

// SetProvider sets the upstream provider for this identity.
func (i *Identity) SetProvider(provider types.Provider) {
	i.provider = provider
}

// SetConfig sets the identity configuration (for Via.Provider resolution).
func (i *Identity) SetConfig(config *schema.Identity) {
	i.config = config
}

// GetProviderName returns the provider name for this identity.
// Returns Via.Provider from config, falling back to provider.Name() or empty string.
func (i *Identity) GetProviderName() (string, error) {
	defer perf.Track(nil, "gcp_service_account.GetProviderName")()

	// First, try to get provider from config (preferred for whoami/reporting).
	if i.config != nil && i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	// Fall back to provider instance if set.
	if i.provider != nil {
		return i.provider.Name(), nil
	}
	return "", nil
}

// Validate validates the identity configuration.
func (i *Identity) Validate() error {
	defer perf.Track(nil, "gcp_service_account.Validate")()

	if i.principal == nil {
		return fmt.Errorf("%w: principal is nil", errUtils.ErrInvalidIdentityConfig)
	}
	if i.principal.ServiceAccountEmail == "" {
		return fmt.Errorf("%w: service_account_email is required", errUtils.ErrInvalidIdentityConfig)
	}
	// Validate email format (basic check).
	if !strings.Contains(i.principal.ServiceAccountEmail, "@") ||
		!strings.HasSuffix(i.principal.ServiceAccountEmail, ".iam.gserviceaccount.com") {
		return fmt.Errorf("%w: invalid service_account_email format: %s",
			errUtils.ErrInvalidIdentityConfig, i.principal.ServiceAccountEmail)
	}
	return nil
}

// Authenticate obtains credentials by impersonating the service account using upstream base credentials.
func (i *Identity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "gcp_service_account.Authenticate")()

	if err := i.Validate(); err != nil {
		return nil, err
	}

	if baseCreds == nil {
		return nil, fmt.Errorf("%w: no credentials from provider for identity", errUtils.ErrAuthenticationFailed)
	}

	gcpCreds, ok := baseCreds.(*types.GCPCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: provider did not return GCP credentials", errUtils.ErrAuthenticationFailed)
	}

	// Impersonate the target service account.
	accessToken, expiry, err := i.impersonateServiceAccount(ctx, gcpCreds.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("%w: impersonation failed: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Determine project ID.
	projectID := i.principal.ProjectID
	if projectID == "" {
		projectID = extractProjectFromEmail(i.principal.ServiceAccountEmail)
	}

	return &types.GCPCredentials{
		AccessToken:         accessToken,
		TokenExpiry:         expiry,
		ProjectID:           projectID,
		ServiceAccountEmail: i.principal.ServiceAccountEmail,
		Scopes:              i.getScopes(),
	}, nil
}

// IAMCredentialsService provides access to IAM Credentials API token generation.
type IAMCredentialsService interface {
	GenerateAccessToken(ctx context.Context, name string, req *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error)
}

// IAMCredentialsServiceFactory creates IAM credentials service instances for a token.
type IAMCredentialsServiceFactory func(ctx context.Context, accessToken string) (IAMCredentialsService, error)

type iamCredentialsService struct {
	svc *iamcredentials.Service
}

func (s *iamCredentialsService) GenerateAccessToken(ctx context.Context, name string, req *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error) {
	return s.svc.Projects.ServiceAccounts.GenerateAccessToken(name, req).Context(ctx).Do()
}

func newIAMCredentialsService(ctx context.Context, accessToken string) (IAMCredentialsService, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	svc, err := iamcredentials.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	return &iamCredentialsService{svc: svc}, nil
}

// impersonateServiceAccount uses IAM Credentials API to generate an access token.
func (i *Identity) impersonateServiceAccount(ctx context.Context, upstreamToken string) (string, time.Time, error) {
	defer perf.Track(nil, "gcp_service_account.impersonateServiceAccount")()

	factory := i.iamServiceFactory
	if factory == nil {
		factory = newIAMCredentialsService
	}
	svc, err := factory(ctx, upstreamToken)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create IAM credentials service: %w", err)
	}

	name := fmt.Sprintf("projects/-/serviceAccounts/%s", i.principal.ServiceAccountEmail)
	req := &iamcredentials.GenerateAccessTokenRequest{
		Scope:     i.getScopes(),
		Delegates: i.formatDelegates(),
		Lifetime:  i.getLifetime(),
	}

	resp, err := svc.GenerateAccessToken(ctx, name, req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate access token: %w", err)
	}

	expiry, err := time.Parse(time.RFC3339, resp.ExpireTime)
	if err != nil {
		expiry = time.Now().Add(1 * time.Hour)
	}

	return resp.AccessToken, expiry, nil
}

func (i *Identity) getScopes() []string {
	if len(i.principal.Scopes) > 0 {
		return i.principal.Scopes
	}
	return []string{DefaultScope}
}

func (i *Identity) getLifetime() string {
	if i.principal.Lifetime != "" {
		return i.principal.Lifetime
	}
	return DefaultLifetime
}

// formatDelegates formats delegate emails for the API.
// The API expects the format: projects/-/serviceAccounts/{email}.
func (i *Identity) formatDelegates() []string {
	if len(i.principal.Delegates) == 0 {
		return nil
	}
	delegates := make([]string, len(i.principal.Delegates))
	for idx, email := range i.principal.Delegates {
		delegates[idx] = fmt.Sprintf("projects/-/serviceAccounts/%s", email)
	}
	return delegates
}

// extractProjectFromEmail extracts project ID from SA email.
// Format: name@project-id.iam.gserviceaccount.com.
func extractProjectFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	domain := parts[1]
	// Remove .iam.gserviceaccount.com suffix.
	projectID := strings.TrimSuffix(domain, ".iam.gserviceaccount.com")
	return projectID
}

// Environment returns environment variables for this identity.
func (i *Identity) Environment() (map[string]string, error) {
	defer perf.Track(nil, "gcp_service_account.Environment")()

	env := make(map[string]string)
	projectID := i.principal.ProjectID
	if projectID == "" {
		projectID = extractProjectFromEmail(i.principal.ServiceAccountEmail)
	}
	if projectID != "" {
		env["GOOGLE_CLOUD_PROJECT"] = projectID
		env["GCLOUD_PROJECT"] = projectID
		env["CLOUDSDK_CORE_PROJECT"] = projectID
	}
	return env, nil
}

// Paths returns credential file paths for this identity.
func (i *Identity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// This loads credentials from files and sets GOOGLE_OAUTH_ACCESS_TOKEN along with
// project/region environment variables.
func (i *Identity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "gcp_service_account.PrepareEnvironment")()

	out := make(map[string]string, len(environ))
	for k, v := range environ {
		out[k] = v
	}

	// Load credentials from files to get the access token.
	providerName, err := i.GetProviderName()
	if err != nil {
		return nil, fmt.Errorf("%w: resolve provider name: %v", errUtils.ErrInvalidIdentityConfig, err)
	}
	if providerName == "" {
		return nil, fmt.Errorf("%w: provider name is required for identity %q", errUtils.ErrInvalidIdentityConfig, i.Name())
	}
	creds, err := gcp.LoadCredentialsFromFiles(ctx, nil, i.realmOrDefault(), providerName, i.Name())
	if err != nil {
		return nil, fmt.Errorf("%w: load credentials: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Credentials must exist and be valid - if not, the user needs to run `atmos auth login` first.
	if creds == nil || creds.AccessToken == "" || creds.IsExpired() {
		return nil, fmt.Errorf("%w: no valid credentials found for identity %q; run 'atmos auth login' first", errUtils.ErrAuthenticationFailed, i.Name())
	}

	// Build GCP auth context with credentials and project info.
	projectID := i.principal.ProjectID
	if projectID == "" {
		projectID = creds.ProjectID
	}
	if projectID == "" {
		projectID = extractProjectFromEmail(i.principal.ServiceAccountEmail)
	}
	gcpAuth := &schema.GCPAuthContext{
		ProjectID:   projectID,
		AccessToken: creds.AccessToken,
	}

	// Get all GCP environment variables using the centralized function.
	// This ensures GOOGLE_OAUTH_ACCESS_TOKEN is set when we have an access token.
	gcpEnv, err := gcp.GetEnvironmentVariablesForIdentity(i.realmOrDefault(), providerName, i.Name(), gcpAuth)
	if err != nil {
		return nil, fmt.Errorf("get GCP environment variables: %w", err)
	}

	for k, v := range gcpEnv {
		out[k] = v
	}

	return out, nil
}

// PostAuthenticate sets up credential files and auth context after successful authentication.
func (i *Identity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "gcp_service_account.PostAuthenticate")()

	if params == nil || params.Credentials == nil {
		return nil
	}

	gcpCreds, ok := params.Credentials.(*types.GCPCredentials)
	if !ok {
		return fmt.Errorf("%w: expected GCP credentials", errUtils.ErrAuthenticationFailed)
	}

	// ConfigAndStacksInfo does not embed AtmosConfiguration; pass nil and rely on gcp.Setup behavior.
	var atmosConfig *schema.AtmosConfiguration
	if err := gcp.Setup(ctx, atmosConfig, i.realmOrDefault(), params.ProviderName, i.Name(), gcpCreds); err != nil {
		return err
	}
	if params.AuthContext != nil {
		if err := gcp.SetAuthContext(params.AuthContext, i.realmOrDefault(), params.ProviderName, i.Name(), gcpCreds); err != nil {
			return err
		}
	}
	return nil
}

// Logout removes identity credential files.
func (i *Identity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "gcp_service_account.Logout")()

	providerName, err := i.GetProviderName()
	if err != nil {
		return fmt.Errorf("%w: resolve provider name: %w", errUtils.ErrInvalidIdentityConfig, err)
	}
	if providerName == "" {
		return fmt.Errorf("%w: provider name is required for identity %q", errUtils.ErrInvalidIdentityConfig, i.Name())
	}
	return gcp.Cleanup(ctx, nil, i.realmOrDefault(), providerName, i.Name())
}

// CredentialsExist checks if valid credentials exist for this identity.
func (i *Identity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "gcp_service_account.CredentialsExist")()

	providerName, err := i.GetProviderName()
	if err != nil {
		return false, fmt.Errorf("%w: resolve provider name: %w", errUtils.ErrInvalidIdentityConfig, err)
	}
	if providerName == "" {
		return false, fmt.Errorf("%w: provider name is required for identity %q", errUtils.ErrInvalidIdentityConfig, i.Name())
	}
	return gcp.CredentialsExist(context.Background(), nil, i.realmOrDefault(), providerName, i.Name())
}

// LoadCredentials loads credentials from identity-managed storage.
func (i *Identity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "gcp_service_account.LoadCredentials")()

	providerName, err := i.GetProviderName()
	if err != nil {
		return nil, fmt.Errorf("%w: resolve provider name: %w", errUtils.ErrInvalidIdentityConfig, err)
	}
	if providerName == "" {
		return nil, fmt.Errorf("%w: provider name is required for identity %q", errUtils.ErrInvalidIdentityConfig, i.Name())
	}
	creds, err := gcp.LoadCredentialsFromFiles(ctx, nil, i.realmOrDefault(), providerName, i.Name())
	if err != nil {
		return nil, fmt.Errorf("%w: load credentials: %w", errUtils.ErrAuthenticationFailed, err)
	}
	if creds == nil {
		return nil, fmt.Errorf("%w: missing credentials for %s", errUtils.ErrNoCredentialsFound, i.Name())
	}
	if creds.IsExpired() {
		return nil, fmt.Errorf("%w: expired credentials for %s", errUtils.ErrExpiredCredentials, i.Name())
	}
	return creds, nil
}
