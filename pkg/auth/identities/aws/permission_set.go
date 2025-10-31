package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	principalName        = "name"
	principalAccount     = "account"
	principalAccountName = "name"
	principalAccountID   = "id"
)

// permissionSetIdentity implements AWS permission set identity.
type permissionSetIdentity struct {
	name             string
	config           *schema.Identity
	manager          types.AuthManager // Auth manager for resolving root provider
	rootProviderName string            // Cached root provider name from PostAuthenticate
}

// NewPermissionSetIdentity creates a new AWS permission set identity.
func NewPermissionSetIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config.Kind != "aws/permission-set" {
		return nil, fmt.Errorf("%w: invalid identity kind for permission set: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	return &permissionSetIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *permissionSetIdentity) Kind() string {
	return "aws/permission-set"
}

// Authenticate performs authentication using permission set.
//

func (i *permissionSetIdentity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	// Note: Caching is now handled at the manager level to prevent duplicates.

	awsBase, ok := baseCreds.(*types.AWSCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: base AWS credentials are required for permission-set", errUtils.ErrInvalidIdentityConfig)
	}

	log.Debug("Permission set authentication started.", "identity", i.name)

	// Get permission set and account info.
	permissionSetName, err := i.getPermissionSetName()
	if err != nil {
		return nil, err
	}
	accountName, accountID, err := i.getAccountDetails()
	if err != nil {
		return nil, err
	}

	// Create SSO client and resolve account ID if needed.
	ssoClient, err := i.newSSOClient(ctx, awsBase)
	if err != nil {
		return nil, err
	}
	accountID, err = i.resolveAccountID(ctx, ssoClient, accountName, accountID, awsBase.AccessKeyID)
	if err != nil {
		return nil, err
	}

	// Get role credentials for the permission set.
	roleCredsResp, err := ssoClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccountId:   awssdk.String(accountID),
		RoleName:    awssdk.String(permissionSetName),
		AccessToken: awssdk.String(awsBase.AccessKeyID),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get role credentials: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Convert to our credential format.
	creds, err := i.buildCredsFromRole(roleCredsResp, awsBase.Region)
	if err != nil {
		return nil, err
	}

	log.Debug("Permission set authentication successful.", "identity", i.name)

	// Note: Caching handled at manager level.
	return creds, nil
}

// Validate validates the identity configuration.
func (i *permissionSetIdentity) Validate() error {
	if i.config.Principal == nil {
		return fmt.Errorf("%w: principal is required", errUtils.ErrInvalidIdentityConfig)
	}

	// Check permission set name in principal or spec (backward compatibility).
	var permissionSetName string
	var ok bool
	if permissionSetName, ok = i.config.Principal["name"].(string); !ok || permissionSetName == "" {
		return fmt.Errorf("%w: permission set name is required in principal", errUtils.ErrInvalidIdentityConfig)
	}

	// Check account info in principal.
	var accountSpec map[string]interface{}
	if accountSpec, ok = i.config.Principal["account"].(map[string]interface{}); !ok {
		return fmt.Errorf("%w: account specification is required in principal", errUtils.ErrInvalidIdentityConfig)
	}

	accountName, okName := accountSpec["name"].(string)
	accountID, okID := accountSpec["id"].(string)
	if !okName && !okID {
		return fmt.Errorf("%w: account name or account ID is required", errUtils.ErrInvalidIdentityConfig)
	}
	if accountName == "" && accountID == "" {
		return fmt.Errorf("%w: account name or account ID is required", errUtils.ErrInvalidIdentityConfig)
	}
	return nil
}

// Environment returns environment variables for this identity.
func (i *permissionSetIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Get root provider name for file storage.
	providerName, err := i.resolveRootProviderName()
	if err != nil {
		return nil, err
	}

	// Get AWS file environment variables.
	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return nil, errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}
	awsEnvVars := awsFileManager.GetEnvironmentVariables(providerName, i.name)

	// Convert to map format.
	for _, envVar := range awsEnvVars {
		env[envVar.Key] = envVar.Value
	}

	// Add environment variables from identity config.
	for _, envVar := range i.config.Env {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For AWS permission set identities, we use the shared AWS PrepareEnvironment helper
// which configures credential files, profile, region, and disables IMDS fallback.
func (i *permissionSetIdentity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.permissionSetIdentity.PrepareEnvironment")()

	// Get root provider name for file storage.
	providerName, err := i.resolveRootProviderName()
	if err != nil {
		return environ, fmt.Errorf("failed to get provider name: %w", err)
	}

	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return environ, fmt.Errorf("failed to create AWS file manager: %w", err)
	}

	credentialsFile := awsFileManager.GetCredentialsPath(providerName)
	configFile := awsFileManager.GetConfigPath(providerName)

	// Get region from identity config if available.
	region := ""
	if i.config.Principal != nil {
		if r, ok := i.config.Principal["region"].(string); ok {
			region = r
		}
	}

	// Use shared AWS environment preparation helper.
	return awsCloud.PrepareEnvironment(environ, i.name, credentialsFile, configFile, region), nil
}

// GetProviderName extracts the provider name from the identity configuration.
func (i *permissionSetIdentity) GetProviderName() (string, error) {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	return "", fmt.Errorf("%w: permission set identity %q has no valid via provider configuration", errUtils.ErrInvalidAuthConfig, i.name)
}

// resolveRootProviderName resolves the root provider name for file storage.
// Tries manager first (if available), then falls back to cached value or config.
func (i *permissionSetIdentity) resolveRootProviderName() (string, error) {
	// Try manager first (available after PostAuthenticate).
	if i.manager != nil {
		if providerName := i.manager.GetProviderForIdentity(i.name); providerName != "" {
			return providerName, nil
		}
	}

	// Fall back to cached value or config.
	return i.getRootProviderFromVia()
}

// getRootProviderFromVia gets the root provider name using available information.
// This is used when manager is not available (e.g., LoadCredentials before PostAuthenticate).
// Only returns the cached value from PostAuthenticate. Does NOT fall back to via.provider
// because via.provider is the immediate parent, not necessarily the root provider in the chain.
func (i *permissionSetIdentity) getRootProviderFromVia() (string, error) {
	// Try cached value set during PostAuthenticate or SetManagerAndProvider.
	if i.rootProviderName != "" {
		return i.rootProviderName, nil
	}

	// Can't determine root provider without authentication chain.
	// The manager.ensureIdentityHasManager() should have set this before calling Environment().
	return "", fmt.Errorf("%w: cannot determine root provider for identity %q before authentication", errUtils.ErrInvalidAuthConfig, i.name)
}

// SetManagerAndProvider sets the manager and root provider name on the identity.
// This is used when loading cached credentials to allow the identity to resolve provider information.
func (i *permissionSetIdentity) SetManagerAndProvider(manager types.AuthManager, rootProviderName string) {
	i.manager = manager
	i.rootProviderName = rootProviderName
}

// PostAuthenticate sets up AWS files and populates auth context after authentication.
func (i *permissionSetIdentity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	// Guard against nil parameters to avoid panics.
	if params == nil {
		return fmt.Errorf("%w: PostAuthenticate parameters cannot be nil", errUtils.ErrInvalidAuthConfig)
	}
	if params.Credentials == nil {
		return fmt.Errorf("%w: credentials are required", errUtils.ErrInvalidAuthConfig)
	}

	// Store manager reference and root provider name for resolving in file operations.
	i.manager = params.Manager
	i.rootProviderName = params.ProviderName

	// Setup AWS files using shared AWS cloud package.
	if err := awsCloud.SetupFiles(params.ProviderName, params.IdentityName, params.Credentials, ""); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %w", errUtils.ErrAwsAuth, err)
	}

	// Populate auth context (single source of truth for runtime credentials).
	if err := awsCloud.SetAuthContext(&awsCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: params.ProviderName,
		IdentityName: params.IdentityName,
		Credentials:  params.Credentials,
		BasePath:     "",
	}); err != nil {
		return fmt.Errorf("%w: failed to set auth context: %w", errUtils.ErrAwsAuth, err)
	}

	// Derive environment variables from auth context for spawned processes.
	if err := awsCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo); err != nil {
		return fmt.Errorf("%w: failed to set environment variables: %w", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// getPermissionSetName extracts the permission set name from the identity principal.
func (i *permissionSetIdentity) getPermissionSetName() (string, error) {
	if name, ok := i.config.Principal[principalName].(string); ok && name != "" {
		return name, nil
	}
	return "", fmt.Errorf("%w: permission set name is required in principal", errUtils.ErrInvalidIdentityConfig)
}

// getAccountDetails extracts the account name/ID from the identity principal.
func (i *permissionSetIdentity) getAccountDetails() (string, string, error) {
	accountSpec, ok := i.config.Principal[principalAccount].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("%w: account specification is required in principal", errUtils.ErrInvalidIdentityConfig)
	}
	name, _ := accountSpec[principalAccountName].(string)
	id, _ := accountSpec[principalAccountID].(string)
	if name == "" && id == "" {
		return "", "", fmt.Errorf("%w: account name or account ID is required", errUtils.ErrInvalidIdentityConfig)
	}
	return name, id, nil
}

// resolveAccountID ensures we have an account ID; if only name provided, it looks it up via SSO.
func (i *permissionSetIdentity) resolveAccountID(ctx context.Context, ssoClient *sso.Client, accountName, accountID, accessToken string) (string, error) {
	if accountID != "" || accountName == "" {
		return accountID, nil
	}

	accountsResp, err := ssoClient.ListAccounts(ctx, &sso.ListAccountsInput{AccessToken: awssdk.String(accessToken)})
	if err != nil {
		return "", fmt.Errorf("%w: failed to list accounts: %w", errUtils.ErrAwsAuth, err)
	}
	for _, account := range accountsResp.AccountList {
		if awssdk.ToString(account.AccountName) == accountName {
			return awssdk.ToString(account.AccountId), nil
		}
	}
	return "", fmt.Errorf("%w: account %q not found", errUtils.ErrAwsAuth, accountName)
}

// newSSOClient creates an AWS SSO client from base credentials.
func (i *permissionSetIdentity) newSSOClient(ctx context.Context, awsBase *types.AWSCredentials) (*sso.Client, error) {
	// Build config options
	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(awsBase.Region),
		// SSO API operations (ListAccounts, GetRoleCredentials) use access token authentication,
		// not AWS signature authentication. Use anonymous credentials to avoid signing errors.
		config.WithCredentialsProvider(awssdk.AnonymousCredentials{}),
	}

	// Add custom endpoint resolver if configured
	if resolverOpt := awsCloud.GetResolverConfigOption(i.config, nil); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	// Load config with isolated environment to avoid conflicts with external AWS env vars.
	cfg, err := awsCloud.LoadIsolatedAWSConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %w", errUtils.ErrInvalidIdentityConfig, err)
	}
	return sso.NewFromConfig(cfg), nil
}

// buildCredsFromRole converts GetRoleCredentialsOutput to AWSCredentials.
func (i *permissionSetIdentity) buildCredsFromRole(resp *sso.GetRoleCredentialsOutput, region string) (*types.AWSCredentials, error) {
	if resp.RoleCredentials == nil {
		return nil, fmt.Errorf("%w: empty role credentials response", errUtils.ErrAuthenticationFailed)
	}
	expiration := ""
	if resp.RoleCredentials.Expiration != 0 {
		// AWS SSO returns expiration as milliseconds since epoch.
		expirationTime := time.Unix(resp.RoleCredentials.Expiration/1000, 0)
		expiration = expirationTime.Format(time.RFC3339)

		// Debug: Log the raw expiration value and converted time.
		log.Debug("SSO credential expiration",
			"identity", i.name,
			"raw_milliseconds", resp.RoleCredentials.Expiration,
			"converted_time", expirationTime,
			"formatted", expiration,
			"time_until_expiry", time.Until(expirationTime))
	}
	return &types.AWSCredentials{
		AccessKeyID:     awssdk.ToString(resp.RoleCredentials.AccessKeyId),
		SecretAccessKey: awssdk.ToString(resp.RoleCredentials.SecretAccessKey),
		SessionToken:    awssdk.ToString(resp.RoleCredentials.SessionToken),
		Region:          region,
		Expiration:      expiration,
	}, nil
}

// CredentialsExist checks if credentials exist for this identity.
func (i *permissionSetIdentity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "aws.permissionSetIdentity.CredentialsExist")()

	// Get root provider name for file storage.
	providerName, err := i.resolveRootProviderName()
	if err != nil {
		return false, err
	}

	mgr, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return false, err
	}

	credPath := mgr.GetCredentialsPath(providerName)

	// Load and parse the credentials file to verify the identity section exists.
	cfg, err := awsCloud.LoadINIFile(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("load credentials file: %w", err)
	}

	// Check if this identity's section exists in the credentials file.
	sec, err := cfg.GetSection(i.name)
	if err != nil {
		// Section doesn't exist - credentials don't exist for this identity.
		return false, nil
	}

	// Verify the section has actual credential keys (not just an empty section).
	if strings.TrimSpace(sec.Key("aws_access_key_id").String()) == "" {
		return false, nil
	}

	return true, nil
}

// LoadCredentials loads AWS credentials from files using environment variables.
// This is used with noop keyring to enable credential validation in whoami.
func (i *permissionSetIdentity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.permissionSetIdentity.LoadCredentials")()

	// Get environment variables that specify where credentials are stored.
	env, err := i.Environment()
	if err != nil {
		return nil, fmt.Errorf("failed to get environment variables: %w", err)
	}

	// Load credentials from files using AWS SDK.
	creds, err := loadAWSCredentialsFromEnvironment(ctx, env)
	if err != nil {
		return nil, err
	}

	return creds, nil
}

// Logout removes identity-specific credential storage.
func (i *permissionSetIdentity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.permissionSetIdentity.Logout")()

	log.Debug("Logout permission-set identity", "identity", i.name, "provider", i.rootProviderName)

	// Get base_path from provider spec if configured (requires manager to lookup provider config).
	// For now, use empty string (default XDG path) since SetupFiles uses empty string too.
	basePath := ""

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		log.Debug("Failed to create file manager for logout", "identity", i.name, "error", err)
		return fmt.Errorf("failed to create AWS file manager: %w", err)
	}

	// Remove this identity's profile from the provider's config files.
	if err := fileManager.DeleteIdentity(ctx, i.rootProviderName, i.name); err != nil {
		log.Debug("Failed to delete identity files", "identity", i.name, "error", err)
		return fmt.Errorf("failed to delete identity files: %w", err)
	}

	log.Debug("Successfully deleted permission-set identity", "identity", i.name)
	return nil
}
