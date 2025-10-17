package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
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
	name   string
	config *schema.Identity
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
		return nil, fmt.Errorf("%w: failed to get role credentials: %v", errUtils.ErrAuthenticationFailed, err)
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

	// Get provider name for AWS file paths.
	providerName, err := i.GetProviderName()
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

// GetProviderName extracts the provider name from the identity configuration.
func (i *permissionSetIdentity) GetProviderName() (string, error) {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	return "", fmt.Errorf("%w: permission set identity %q has no valid via provider configuration", errUtils.ErrInvalidIdentityConfig, i.name)
}

// PostAuthenticate sets up AWS files after authentication.
func (i *permissionSetIdentity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	// Setup AWS files using shared AWS cloud package.
	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %v", errUtils.ErrAwsAuth, err)
	}
	if err := awsCloud.SetEnvironmentVariables(stackInfo, providerName, identityName); err != nil {
		return fmt.Errorf("%w: failed to set environment variables: %v", errUtils.ErrAwsAuth, err)
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
		return "", fmt.Errorf("%w: failed to list accounts: %v", errUtils.ErrAwsAuth, err)
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
		config.WithCredentialsProvider(awssdk.CredentialsProviderFunc(func(ctx context.Context) (awssdk.Credentials, error) {
			return awssdk.Credentials{
				AccessKeyID: awsBase.AccessKeyID, // This is actually the SSO access token
			}, nil
		})),
	}

	// Add custom endpoint resolver if configured
	if resolverOpt := awsCloud.GetResolverConfigOption(i.config, nil); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrInvalidIdentityConfig, err)
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
		expiration = time.Unix(resp.RoleCredentials.Expiration/1000, 0).Format(time.RFC3339)
	}
	return &types.AWSCredentials{
		AccessKeyID:     awssdk.ToString(resp.RoleCredentials.AccessKeyId),
		SecretAccessKey: awssdk.ToString(resp.RoleCredentials.SecretAccessKey),
		SessionToken:    awssdk.ToString(resp.RoleCredentials.SessionToken),
		Region:          region,
		Expiration:      expiration,
	}, nil
}

// Logout removes identity-specific credential storage.
func (i *permissionSetIdentity) Logout(ctx context.Context) error {
	// AWS permission-set identities don't have identity-specific storage.
	// File cleanup is handled by the provider's Logout method.
	// Keyring cleanup is handled by AuthManager.
	log.Debug("Logout called for permission-set identity (no identity-specific cleanup)", "identity", i.name)
	return nil
}
