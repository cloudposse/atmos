package okta

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	oktaCloud "github.com/cloudposse/atmos/pkg/auth/cloud/okta"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Logging keys.
	logKeyIdentity = "identity"
	logKeyProvider = "provider"
)

// apiAccessIdentity implements Okta API access identity.
// This identity uses Okta credentials to access Okta APIs (users, groups, applications, etc.).
type apiAccessIdentity struct {
	name   string
	config *schema.Identity
	realm  string // Credential isolation realm set by auth manager.
}

// NewAPIAccessIdentity creates a new Okta API access identity.
func NewAPIAccessIdentity(name string, config *schema.Identity) (*apiAccessIdentity, error) {
	defer perf.Track(nil, "okta.NewAPIAccessIdentity")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity config is required", errUtils.ErrInvalidIdentityConfig)
	}
	if config.Kind != "okta/api-access" {
		return nil, fmt.Errorf("%w: invalid identity kind for Okta API access identity: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	return &apiAccessIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *apiAccessIdentity) Kind() string {
	return "okta/api-access"
}

// SetRealm sets the credential isolation realm for this identity.
func (i *apiAccessIdentity) SetRealm(realm string) {
	i.realm = realm
}

// GetProviderName returns the provider name for this identity.
func (i *apiAccessIdentity) GetProviderName() (string, error) {
	if i.config.Via == nil || i.config.Via.Provider == "" {
		return "", fmt.Errorf("%w: Okta API access identity requires via.provider", errUtils.ErrInvalidIdentityConfig)
	}
	return i.config.Via.Provider, nil
}

// Authenticate performs authentication using the provided base credentials.
// For Okta API access identity, we use the provider credentials directly.
func (i *apiAccessIdentity) Authenticate(ctx context.Context, baseCreds authTypes.ICredentials) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "okta.apiAccessIdentity.Authenticate")()

	log.Debug("Authenticating Okta API access identity",
		logKeyIdentity, i.name,
	)

	// Verify base credentials are Okta credentials.
	oktaCreds, ok := baseCreds.(*authTypes.OktaCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: Okta API access identity requires Okta credentials from provider", errUtils.ErrAuthenticationFailed)
	}

	// For API access, we use the provider credentials directly.
	// No additional transformation needed.
	log.Debug("Successfully authenticated Okta API access identity",
		logKeyIdentity, i.name,
		"org_url", oktaCreds.OrgURL,
	)

	return oktaCreds, nil
}

// Validate validates the identity configuration.
func (i *apiAccessIdentity) Validate() error {
	if i.config.Via == nil || i.config.Via.Provider == "" {
		return fmt.Errorf("%w: via.provider is required", errUtils.ErrInvalidIdentityConfig)
	}
	return nil
}

// Environment returns environment variables for this identity.
func (i *apiAccessIdentity) Environment() (map[string]string, error) {
	// Okta environment variables are set by the provider and PrepareEnvironment.
	return make(map[string]string), nil
}

// PrepareEnvironment prepares environment variables for external processes.
func (i *apiAccessIdentity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	_ = ctx // Context available for future use.

	// Copy environment without modifications.
	// The provider already set OKTA_ORG_URL, etc.
	result := make(map[string]string)
	for k, v := range environ {
		result[k] = v
	}

	return result, nil
}

// PostAuthenticate is called after successful authentication with the final credentials.
func (i *apiAccessIdentity) PostAuthenticate(ctx context.Context, params *authTypes.PostAuthenticateParams) error {
	defer perf.Track(nil, "okta.apiAccessIdentity.PostAuthenticate")()

	log.Debug("Post-authenticate for Okta API access identity",
		logKeyIdentity, i.name,
	)

	// Setup Okta files (tokens.json).
	if err := oktaCloud.SetupFiles(params.ProviderName, params.IdentityName, params.Credentials, "", params.Realm); err != nil {
		return fmt.Errorf("failed to setup Okta files: %w", err)
	}

	// Populate Okta auth context.
	setupParams := &oktaCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: params.ProviderName,
		IdentityName: params.IdentityName,
		Credentials:  params.Credentials,
		BasePath:     "", // Use default path.
		Realm:        params.Realm,
	}
	if err := oktaCloud.SetAuthContext(setupParams); err != nil {
		return fmt.Errorf("failed to set Okta auth context: %w", err)
	}

	// Set environment variables in stack info.
	if err := oktaCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo); err != nil {
		return fmt.Errorf("failed to set Okta environment variables: %w", err)
	}

	log.Debug("Post-authenticate complete for Okta API access identity",
		logKeyIdentity, i.name,
	)

	return nil
}

// Logout removes identity-specific credential storage.
func (i *apiAccessIdentity) Logout(ctx context.Context) error {
	_ = ctx // Context available for future use.

	log.Debug("Logout Okta API access identity", logKeyIdentity, i.name)
	// Credentials are managed by keyring, files cleaned up by provider.
	return nil
}

// CredentialsExist checks if credentials exist for this identity.
func (i *apiAccessIdentity) CredentialsExist() (bool, error) {
	// Check if Okta tokens file exists.
	providerName, err := i.GetProviderName()
	if err != nil {
		return false, err
	}

	fileManager, err := oktaCloud.NewOktaFileManager("", i.realm)
	if err != nil {
		return false, err
	}

	return fileManager.TokensExist(providerName), nil
}

// Paths returns credential files/directories used by this identity.
func (i *apiAccessIdentity) Paths() ([]authTypes.Path, error) {
	// Get provider name.
	providerName, err := i.GetProviderName()
	if err != nil {
		return nil, err
	}

	// Create file manager to get provider-namespaced paths.
	fileManager, err := oktaCloud.NewOktaFileManager("", i.realm)
	if err != nil {
		return nil, err
	}

	return []authTypes.Path{
		{
			Location: fileManager.GetTokensPath(providerName),
			Type:     authTypes.PathTypeFile,
			Required: true,
			Purpose:  fmt.Sprintf("Okta tokens file for identity %s", i.name),
			Metadata: map[string]string{
				"read_only": "true",
			},
		},
	}, nil
}

// LoadCredentials loads credentials from identity-managed storage.
func (i *apiAccessIdentity) LoadCredentials(ctx context.Context) (authTypes.ICredentials, error) {
	_ = ctx // Context available for future use.

	// Load from Okta tokens file.
	providerName, err := i.GetProviderName()
	if err != nil {
		return nil, err
	}

	fileManager, err := oktaCloud.NewOktaFileManager("", i.realm)
	if err != nil {
		return nil, err
	}

	tokens, err := fileManager.LoadTokens(providerName)
	if err != nil {
		return nil, err
	}

	// Convert tokens to credentials.
	// OrgURL is now persisted in the tokens file for cache-only whoami.
	creds := &authTypes.OktaCredentials{
		OrgURL:                tokens.OrgURL,
		AccessToken:           tokens.AccessToken,
		IDToken:               tokens.IDToken,
		RefreshToken:          tokens.RefreshToken,
		ExpiresAt:             tokens.ExpiresAt,
		RefreshTokenExpiresAt: tokens.RefreshTokenExpiresAt,
		Scope:                 tokens.Scope,
	}

	return creds, nil
}

// Ensure apiAccessIdentity implements Identity interface.
var _ authTypes.Identity = (*apiAccessIdentity)(nil)
