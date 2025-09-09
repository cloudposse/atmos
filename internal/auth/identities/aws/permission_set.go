package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/internal/auth/cloud/aws"
	"github.com/cloudposse/atmos/internal/auth/types"
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
//nolint:revive // cyclomatic: complexity is acceptable for this orchestration method
func (i *permissionSetIdentity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	// Note: Caching is now handled at the manager level to prevent duplicates

	awsBase, ok := baseCreds.(*types.AWSCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: base AWS credentials are required for permission-set", errUtils.ErrInvalidIdentityConfig)
	}

	log.Debug("Permission set authentication started.", "identity", i.name)

	// Get permission set name from principal or spec (backward compatibility)
	var permissionSetName string
	var ok1 bool
	if permissionSetName, ok1 = i.config.Principal[principalName].(string); !ok1 || permissionSetName == "" {
		return nil, fmt.Errorf("%w: permission set name is required in principal", errUtils.ErrInvalidIdentityConfig)
	}

	// Get account info from principal or spec (backward compatibility)
	var accountSpec map[string]interface{}
	var ok2 bool
	if accountSpec, ok2 = i.config.Principal[principalAccount].(map[string]interface{}); !ok2 {
		return nil, fmt.Errorf("%w: account specification is required in principal", errUtils.ErrInvalidIdentityConfig)
	}

	accountName, okAccountName := accountSpec[principalAccountName].(string)
	accountID, _ := accountSpec[principalAccountID].(string)
	if accountName == "" && accountID == "" {
		return nil, fmt.Errorf("%w: account name or account ID is required", errUtils.ErrInvalidIdentityConfig)
	}

	// Create AWS config using the base credentials (SSO access token)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(awsBase.Region),
		config.WithCredentialsProvider(awssdk.CredentialsProviderFunc(func(ctx context.Context) (awssdk.Credentials, error) {
			return awssdk.Credentials{
				AccessKeyID: awsBase.AccessKeyID, // This is actually the SSO access token
			}, nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrInvalidIdentityConfig, err)
	}

	// Create SSO client
	ssoClient := sso.NewFromConfig(cfg)

	// List accounts to find the target account ID
	accountsResp, err := ssoClient.ListAccounts(ctx, &sso.ListAccountsInput{
		AccessToken: awssdk.String(awsBase.AccessKeyID), // SSO access token
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list accounts: %v", errUtils.ErrAwsAuth, err)
	}

	if okAccountName {
		for _, account := range accountsResp.AccountList {
			if awssdk.ToString(account.AccountName) == accountName {
				accountID = awssdk.ToString(account.AccountId)
				break
			}
		}
		if accountID == "" {
			return nil, fmt.Errorf("%w: account %q not found", errUtils.ErrAwsAuth, accountName)
		}
	}

	// Get role credentials for the permission set
	roleCredsResp, err := ssoClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccountId:   awssdk.String(accountID),
		RoleName:    awssdk.String(permissionSetName),
		AccessToken: awssdk.String(awsBase.AccessKeyID),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get role credentials: %v", errUtils.ErrAuthenticationFailed, err)
	}

  // Convert to our credential format.
  if roleCredsResp.RoleCredentials == nil {
      return nil, fmt.Errorf("%w: empty role credentials response", errUtils.ErrAuthenticationFailed)
  }
  expiration := ""
  if roleCredsResp.RoleCredentials.Expiration != 0 {
      expiration = time.Unix(roleCredsResp.RoleCredentials.Expiration/1000, 0).Format(time.RFC3339)
  }

  creds := &types.AWSCredentials{
      AccessKeyID:     awssdk.ToString(roleCredsResp.RoleCredentials.AccessKeyId),
      SecretAccessKey: awssdk.ToString(roleCredsResp.RoleCredentials.SecretAccessKey),
      SessionToken:    awssdk.ToString(roleCredsResp.RoleCredentials.SessionToken),
      Region:          awsBase.Region,
      Expiration:      expiration,
  }

	log.Debug("Permission set authentication successful.", "identity", i.name)

	// Note: Caching handled at manager level
	return creds, nil
}

// Validate validates the identity configuration.
func (i *permissionSetIdentity) Validate() error {
	if i.config.Principal == nil {
		return fmt.Errorf("%w: principal is required", errUtils.ErrInvalidIdentityConfig)
	}

	// Check permission set name in principal or spec (backward compatibility)
	var permissionSetName string
	var ok bool
	if permissionSetName, ok = i.config.Principal["name"].(string); !ok || permissionSetName == "" {
		return fmt.Errorf("%w: permission set name is required in principal", errUtils.ErrInvalidIdentityConfig)
	}

	// Check account info in principal
	var accountSpec map[string]interface{}
	if accountSpec, ok = i.config.Principal["account"].(map[string]interface{}); !ok {
		return fmt.Errorf("%w: account specification is required in principal", errUtils.ErrInvalidIdentityConfig)
	}

	accountName, okName := accountSpec["name"].(string)
	accountID, okID := accountSpec["id"].(string)
	if !(okName || okID) {
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

	// Add environment variables from identity config
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
	// Setup AWS files using shared AWS cloud package
	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %v", errUtils.ErrAwsAuth, err)
	}
	if err := awsCloud.SetEnvironmentVariables(stackInfo, providerName, identityName); err != nil {
		return fmt.Errorf("%w: failed to set environment variables: %v", errUtils.ErrAwsAuth, err)
	}
	return nil
}
