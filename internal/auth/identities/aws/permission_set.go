package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/cloudposse/atmos/internal/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// permissionSetIdentity implements AWS permission set identity
type permissionSetIdentity struct {
	name   string
	config *schema.Identity
}

// NewPermissionSetIdentity creates a new AWS permission set identity
func NewPermissionSetIdentity(name string, config *schema.Identity) (auth.Identity, error) {
	if config.Kind != "aws/permission-set" {
		return nil, fmt.Errorf("invalid identity kind for permission set: %s", config.Kind)
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

// Authenticate performs authentication using permission set
func (i *permissionSetIdentity) Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error) {
	if baseCreds == nil || baseCreds.AWS == nil {
		return nil, fmt.Errorf("base AWS credentials are required")
	}

	// Get permission set name from spec
	permissionSetName, ok := i.config.Spec["name"].(string)
	if !ok || permissionSetName == "" {
		return nil, fmt.Errorf("permission set name is required in spec")
	}

	// Get account info from spec
	accountSpec, ok := i.config.Spec["account"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("account specification is required")
	}

	accountName, ok := accountSpec["name"].(string)
	if !ok || accountName == "" {
		return nil, fmt.Errorf("account name is required")
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
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create SSO client
	ssoClient := sso.NewFromConfig(cfg)

	// List accounts to find the target account ID
	accountsResp, err := ssoClient.ListAccounts(ctx, &sso.ListAccountsInput{
		AccessToken: aws.String(baseCreds.AWS.AccessKeyID), // SSO access token
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	var accountID string
	for _, account := range accountsResp.AccountList {
		if aws.ToString(account.AccountName) == accountName {
			accountID = aws.ToString(account.AccountId)
			break
		}
	}

	if accountID == "" {
		return nil, fmt.Errorf("account %q not found", accountName)
	}

	// Get role credentials for the permission set
	roleCredsResp, err := ssoClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccessToken: aws.String(baseCreds.AWS.AccessKeyID),
		AccountId:   aws.String(accountID),
		RoleName:    aws.String(permissionSetName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get role credentials: %w", err)
	}

	// Convert to our credential format
	expiration := ""
	if roleCredsResp.RoleCredentials.Expiration != 0 {
		expiration = time.Unix(roleCredsResp.RoleCredentials.Expiration/1000, 0).Format(time.RFC3339)
	}

	return &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     aws.ToString(roleCredsResp.RoleCredentials.AccessKeyId),
			SecretAccessKey: aws.ToString(roleCredsResp.RoleCredentials.SecretAccessKey),
			SessionToken:    aws.ToString(roleCredsResp.RoleCredentials.SessionToken),
			Region:          baseCreds.AWS.Region,
			Expiration:      expiration,
		},
	}, nil
}

// Validate validates the identity configuration
func (i *permissionSetIdentity) Validate() error {
	if i.config.Spec == nil {
		return fmt.Errorf("spec is required")
	}

	permissionSetName, ok := i.config.Spec["name"].(string)
	if !ok || permissionSetName == "" {
		return fmt.Errorf("permission set name is required in spec")
	}

	accountSpec, ok := i.config.Spec["account"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("account specification is required")
	}

	accountName, ok := accountSpec["name"].(string)
	if !ok || accountName == "" {
		return fmt.Errorf("account name is required")
	}

	return nil
}

// Environment returns environment variables for this identity
func (i *permissionSetIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)
	
	// Add environment variables from identity config
	for _, envVar := range i.config.Environment {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// Merge merges this identity configuration with component-level overrides
func (i *permissionSetIdentity) Merge(component *schema.Identity) auth.Identity {
	merged := &permissionSetIdentity{
		name: i.name,
		config: &schema.Identity{
			Kind:        i.config.Kind,
			Default:     component.Default, // Component can override default
			Via:         i.config.Via,
			Spec:        make(map[string]interface{}),
			Alias:       i.config.Alias,
			Environment: i.config.Environment,
		},
	}

	// Merge spec
	for k, v := range i.config.Spec {
		merged.config.Spec[k] = v
	}
	for k, v := range component.Spec {
		merged.config.Spec[k] = v // Component overrides
	}

	// Merge environment variables
	merged.config.Environment = append(merged.config.Environment, component.Environment...)

	// Override alias if provided
	if component.Alias != "" {
		merged.config.Alias = component.Alias
	}

	return merged
}
