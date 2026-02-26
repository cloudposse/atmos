package okta

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles sets up Okta token files for the given identity.
// BasePath specifies the base directory for Okta files (from provider's files.base_path).
// If empty, uses the default ~/.config/atmos/{realm}/okta path.
// The realm parameter provides credential isolation between different repositories.
func SetupFiles(providerName, identityName string, creds types.ICredentials, basePath string, realm string) error {
	defer perf.Track(nil, "okta.SetupFiles")()

	oktaCreds, ok := creds.(*types.OktaCredentials)
	if !ok {
		return nil // No Okta credentials to setup.
	}

	// Create Okta file manager with configured or default path.
	fileManager, err := NewOktaFileManager(basePath, realm)
	if err != nil {
		return errors.Join(errUtils.ErrAuthenticationFailed, err)
	}

	// Convert OktaCredentials to OktaTokens for file storage.
	tokens := &OktaTokens{
		AccessToken:           oktaCreds.AccessToken,
		TokenType:             "Bearer",
		ExpiresAt:             oktaCreds.ExpiresAt,
		RefreshToken:          oktaCreds.RefreshToken,
		RefreshTokenExpiresAt: oktaCreds.RefreshTokenExpiresAt,
		IDToken:               oktaCreds.IDToken,
		Scope:                 oktaCreds.Scope,
		OrgURL:                oktaCreds.OrgURL,
	}

	// Write tokens file.
	if err := fileManager.WriteTokens(providerName, tokens); err != nil {
		return fmt.Errorf("failed to write Okta tokens: %w", err)
	}

	log.Debug("Set up Okta files",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"tokens_path", fileManager.GetTokensPath(providerName),
	)

	return nil
}

// SetAuthContextParams contains parameters for SetAuthContext.
type SetAuthContextParams struct {
	AuthContext  *schema.AuthContext
	StackInfo    *schema.ConfigAndStacksInfo
	ProviderName string
	IdentityName string
	Credentials  types.ICredentials
	BasePath     string
	Realm        string
}

// SetAuthContext populates the Okta auth context with Atmos-managed credential information.
// This enables in-process Okta SDK calls to use Atmos-managed credentials.
func SetAuthContext(params *SetAuthContextParams) error {
	defer perf.Track(nil, "okta.SetAuthContext")()

	if params == nil {
		return fmt.Errorf("%w: SetAuthContext parameters cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	authContext := params.AuthContext
	if authContext == nil {
		return nil // No auth context to populate.
	}

	oktaCreds, ok := params.Credentials.(*types.OktaCredentials)
	if !ok || oktaCreds == nil {
		return nil // No Okta credentials to setup.
	}

	// Validate credentials are not expired.
	if oktaCreds.IsExpired() {
		return fmt.Errorf("%w: Okta credentials are expired", errUtils.ErrAuthenticationFailed)
	}

	m, err := NewOktaFileManager(params.BasePath, params.Realm)
	if err != nil {
		return errors.Join(errUtils.ErrAuthenticationFailed, err)
	}

	tokensPath := m.GetTokensPath(params.ProviderName)
	configDir := m.GetProviderDir(params.ProviderName)

	// Populate Okta auth context as the single source of truth.
	authContext.Okta = &schema.OktaAuthContext{
		TokensFile:  tokensPath,
		ConfigDir:   configDir,
		OrgURL:      oktaCreds.OrgURL,
		AccessToken: oktaCreds.AccessToken,
		IDToken:     oktaCreds.IDToken,
	}

	log.Debug("Set Okta auth context",
		"identity", params.IdentityName,
		"tokens_file", tokensPath,
		"config_dir", configDir,
		"org_url", oktaCreds.OrgURL,
	)

	return nil
}

// SetEnvironmentVariables derives Okta environment variables from AuthContext.
// This populates ComponentEnvSection/ComponentEnvList for spawned processes.
// The auth context is the single source of truth; this function derives from it.
//
// Uses PrepareEnvironment helper to ensure consistent environment setup across all commands.
// This clears conflicting credential env vars and sets Okta organization URL and access token.
//
// Parameters:
//   - authContext: Runtime auth context containing Okta credentials
//   - stackInfo: Stack configuration to populate with environment variables
func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "okta.SetEnvironmentVariables")()

	if authContext == nil || authContext.Okta == nil {
		return nil // No auth context to derive from.
	}

	if stackInfo == nil {
		return nil // No stack info to populate.
	}

	oktaAuth := authContext.Okta

	// Convert existing environment section to map for PrepareEnvironment.
	environMap := make(map[string]string)
	if stackInfo.ComponentEnvSection != nil {
		for k, v := range stackInfo.ComponentEnvSection {
			if str, ok := v.(string); ok {
				environMap[k] = str
			}
		}
	}

	// Use shared PrepareEnvironment helper to get properly configured environment.
	environMap = PrepareEnvironment(PrepareEnvironmentConfig{
		Environ:     environMap,
		OrgURL:      oktaAuth.OrgURL,
		AccessToken: oktaAuth.AccessToken,
		ConfigDir:   oktaAuth.ConfigDir,
	})

	// Replace ComponentEnvSection with prepared environment.
	// IMPORTANT: We must completely replace, not merge, to ensure deleted keys stay deleted.
	stackInfo.ComponentEnvSection = make(map[string]any, len(environMap))
	for k, v := range environMap {
		stackInfo.ComponentEnvSection[k] = v
	}

	return nil
}
