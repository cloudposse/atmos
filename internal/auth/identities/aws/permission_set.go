package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/internal/auth/cloud/aws"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// permissionSetIdentity implements AWS permission set identity
type permissionSetIdentity struct {
	name   string
	config *schema.Identity
}

// NewPermissionSetIdentity creates a new AWS permission set identity
func NewPermissionSetIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config.Kind != "aws/permission-set" {
		return nil, fmt.Errorf("%w: invalid identity kind for permission set: %s", errUtils.ErrStaticError, config.Kind)
	}

	return &permissionSetIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind
func (i *permissionSetIdentity) Kind() string {
	return "aws/permission-set"
}

// permissionSetCache represents cached permission set credentials
type permissionSetCache struct {
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token"`
	Region          string    `json:"region"`
	Expiration      time.Time `json:"expiration"`
	LastUpdated     time.Time `json:"last_updated"`
}

// Authenticate performs authentication using permission set
func (i *permissionSetIdentity) Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error) {
	// Note: Caching is now handled at the manager level to prevent duplicates

	if baseCreds == nil || baseCreds.AWS == nil {
		return nil, fmt.Errorf("%w: base AWS credentials are required", errUtils.ErrStaticError)
	}

	log.Debug("Permission set authentication started.", "identity", i.name)

	// Get permission set name from principal or spec (backward compatibility)
	var permissionSetName string
	var ok bool
	if permissionSetName, ok = i.config.Principal["name"].(string); !ok || permissionSetName == "" {
		return nil, fmt.Errorf("%w: permission set name is required in principal", errUtils.ErrStaticError)
	}

	// Get account info from principal or spec (backward compatibility)
	var accountSpec map[string]interface{}
	if accountSpec, ok = i.config.Principal["account"].(map[string]interface{}); !ok {
		return nil, fmt.Errorf("%w: account specification is required in principal", errUtils.ErrStaticError)
	}
	if !ok {
		return nil, fmt.Errorf("%w: account specification is required", errUtils.ErrStaticError)
	}

	accountName, ok := accountSpec["name"].(string)
	if !ok || accountName == "" {
		return nil, fmt.Errorf("%w: account name is required", errUtils.ErrStaticError)
	}

	// Create AWS config using the base credentials (SSO access token)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(baseCreds.AWS.Region),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID: baseCreds.AWS.AccessKeyID, // This is actually the SSO access token
			}, nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrStaticError, err)
	}

	// Create SSO client
	ssoClient := sso.NewFromConfig(cfg)

	// List accounts to find the target account ID
	accountsResp, err := ssoClient.ListAccounts(ctx, &sso.ListAccountsInput{
		AccessToken: aws.String(baseCreds.AWS.AccessKeyID), // SSO access token
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list accounts: %v", errUtils.ErrStaticError, err)
	}

	var accountID string
	for _, account := range accountsResp.AccountList {
		if aws.ToString(account.AccountName) == accountName {
			accountID = aws.ToString(account.AccountId)
			break
		}
	}

	if accountID == "" {
		return nil, fmt.Errorf("%w: account %q not found", errUtils.ErrStaticError, accountName)
	}

	// Get role credentials for the permission set
	roleCredsResp, err := ssoClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccessToken: aws.String(baseCreds.AWS.AccessKeyID),
		AccountId:   aws.String(accountID),
		RoleName:    aws.String(permissionSetName),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get role credentials: %v", errUtils.ErrStaticError, err)
	}

	// Convert to our credential format
	expiration := ""
	expirationTime := time.Time{}
	if roleCredsResp.RoleCredentials.Expiration != 0 {
		expirationTime = time.Unix(roleCredsResp.RoleCredentials.Expiration/1000, 0)
		expiration = expirationTime.Format(time.RFC3339)
	}

	creds := &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     aws.ToString(roleCredsResp.RoleCredentials.AccessKeyId),
			SecretAccessKey: aws.ToString(roleCredsResp.RoleCredentials.SecretAccessKey),
			SessionToken:    aws.ToString(roleCredsResp.RoleCredentials.SessionToken),
			Region:          baseCreds.AWS.Region,
			Expiration:      expiration,
		},
	}

	log.Debug("Permission set authentication successful.", "identity", i.name)

	// Note: Caching handled at manager level
	return creds, nil
}

// Validate validates the identity configuration
func (i *permissionSetIdentity) Validate() error {
	if i.config.Principal == nil {
		return fmt.Errorf("%w: principal is required", errUtils.ErrStaticError)
	}

	// Check permission set name in principal or spec (backward compatibility)
	var permissionSetName string
	var ok bool
	if permissionSetName, ok = i.config.Principal["name"].(string); !ok || permissionSetName == "" {
		return fmt.Errorf("%w: permission set name is required in principal", errUtils.ErrStaticError)
	}

	// Check account info in principal
	var accountSpec map[string]interface{}
	if accountSpec, ok = i.config.Principal["account"].(map[string]interface{}); !ok {
		return fmt.Errorf("%w: account specification is required in principal", errUtils.ErrStaticError)
	}

	accountName, ok := accountSpec["name"].(string)
	if !ok || accountName == "" {
		return fmt.Errorf("%w: account name is required", errUtils.ErrStaticError)
	}

	return nil
}

// Environment returns environment variables for this identity
func (i *permissionSetIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Add environment variables from identity config
	for _, envVar := range i.config.Env {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// GetProviderName extracts the provider name from the identity configuration
func (i *permissionSetIdentity) GetProviderName() (string, error) {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	return "", fmt.Errorf("%w: permission set identity %q has no valid via provider configuration", errUtils.ErrStaticError, i.name)
}

// PostAuthenticate sets up AWS files after authentication.
func (i *permissionSetIdentity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds *schema.Credentials) error {
	// Setup AWS files using shared AWS cloud package
	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %v", errUtils.ErrStaticError, err)
	}
	if err := awsCloud.SetEnvironmentVariables(stackInfo, providerName, identityName); err != nil {
		return fmt.Errorf("%w: failed to set environment variables: %v", errUtils.ErrStaticError, err)
	}
	return nil
}
