package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/auth/authstore"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// ssoAuthStore is the structure we persist in the keyring as JSON
type ssoAuthStore struct {
	Token       string                     `json:"token"`
	ExpiresAt   time.Time                  `json:"expires_at"`
	LastUpdated time.Time                  `json:"last_updated"`
	TokenOutput *ssooidc.CreateTokenOutput `json:"token_output"`
}

type awsIamIdentityCenter struct {
	Common          schema.ProviderDefaultConfig `yaml:",inline"`
	schema.Identity `yaml:",inline"`

	// SSO
	RoleName    string `yaml:"role_name,omitempty" json:"role_name,omitempty" mapstructure:"role_name,omitempty"`
	AccountId   string `yaml:"account_id,omitempty" json:"account_id,omitempty" mapstructure:"account_id,omitempty"`
	AccountName string `yaml:"account_name,omitempty" json:"account_name,omitempty" mapstructure:"account_name,omitempty"`

	// Store token info for AssumeRole step
	token     string
	accountId string
	roleName  string
}

func NewAwsIamIdentityCenterFactory(provider string, identity string, config schema.AuthConfig) (LoginMethod, error) {
	data := &awsIamIdentityCenter{
		Identity: NewIdentity(),
	}
	b, err := yaml.Marshal(config.Providers[provider])
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(b, data)
	setDefaults(&data.Common, provider, config)
	data.Identity.Identity = identity
	return data, err
}

// Validate checks if the configuration is valid
func (config *awsIamIdentityCenter) Validate() error {
	if config.Common.Url == "" {
		return fmt.Errorf("url is required for AWS IAM Identity Center")
	}

	if config.Common.Profile == "" {
		return fmt.Errorf("profile is required for AWS IAM Identity Center")
	}

	// Validate we have enough information to determine the role
	if config.RoleArnToAssume == "" && config.RoleName == "" {
		return fmt.Errorf("either role or role_name must be specified for AWS IAM Identity Center")
	}

	// Validate we have enough information to determine the account
	if config.RoleArnToAssume == "" && config.AccountId == "" && config.AccountName == "" {
		return fmt.Errorf("either role, account_id, or account_name must be specified for AWS IAM Identity Center")
	}

	return nil
}

// Login authenticates with SSO and gets the token
func (config *awsIamIdentityCenter) Login() error {
	store := authstore.NewKeyringAuthStore()
	keyringKey := config.Provider
	log.Info("Logging in using IAM Identity Center", "Region", config.Common.Region, "ProviderName", config.Provider, "Identity", config.Identity.Identity, "Profile", config.Common.Profile)

	var credentials ssoAuthStore
	// Check if we already have a valid token
	err := store.GetInto(keyringKey, &credentials)
	if err == nil && time.Until(credentials.ExpiresAt) > 30*time.Minute {
		// Valid token, proceed
		log.Debug("Cached token found", "alias", config.Provider)
		config.token = credentials.Token
	} else {
		// No valid token, log in
		// Start SSO flow
		tokenOut, err := SsoSyncE(config.Common.Url, config.Common.Region)
		if err != nil {
			return err
		}

		accessToken := *tokenOut.AccessToken
		log.Debug("✅ Logged in! Token acquired.")

		// Store the login data
		credentials = ssoAuthStore{
			Token:       accessToken,
			ExpiresAt:   time.Now().Add(time.Duration(tokenOut.ExpiresIn) * time.Second),
			LastUpdated: time.Now(),
			TokenOutput: tokenOut,
		}
		err = store.SetAny(keyringKey, &credentials)
		if err != nil {
			log.Warn("failed to save token:", "error", err)
		}

		config.token = accessToken
	}

	log.Info("✅ Successfully authenticated with AWS IAM Identity Center")
	return nil
}

// AssumeRole uses the token from Login to assume an AWS role
func (config *awsIamIdentityCenter) AssumeRole() error {
	if config.token == "" {
		return fmt.Errorf("no SSO token available, please login first")
	}

	ctx := context.Background()
	ssoClient := sso.New(sso.Options{
		Region: config.Common.Region,
	})

	// Determine account ID and role name
	var accountId, roleName string

	roleName = config.RoleName
	accountId = config.AccountId

	// Support passing in an account_name
	if accountId == "" {
		accounts, _ := getAccountsInfo(ctx, ssoClient, config.token)
		for _, account := range accounts {
			if *account.AccountName == config.AccountName {
				accountId = *account.AccountId
				break
			}
		}
	}

	// Get role credentials
	roleCredentials, err := ssoClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccessToken: aws.String(config.token),
		AccountId:   aws.String(accountId),
		RoleName:    aws.String(roleName),
	})
	log.Debug("Role Credentials", "role_name", roleName, "accountid", accountId, "role", config.RoleArnToAssume)
	if err != nil {
		var roles []string
		r, _ := getAccountRoles(ctx, ssoClient, config.token, accountId)
		for _, role := range r {
			roles = append(roles, *role.RoleName)
		}
		log.Error("Failed to get role credentials", "error", err, "account_id", accountId, "role_name", roleName, "available roles", roles)
		return err
	}

	// Write credentials to AWS credentials file
	WriteAwsCredentials(
		config.Common.Profile,
		*roleCredentials.RoleCredentials.AccessKeyId,
		*roleCredentials.RoleCredentials.SecretAccessKey,
		*roleCredentials.RoleCredentials.SessionToken,
		config.Provider,
	)

	log.Info("✅ Successfully assumed role!",
		"credentials", "~/.aws/credentials",
		"profile", config.Common.Profile,
		"account", accountId,
		"role", roleName,
	)

	return nil
}

func getAccountsInfo(ctx context.Context, client *sso.Client, token string) ([]types.AccountInfo, error) {
	var accounts []types.AccountInfo
	input := &sso.ListAccountsInput{AccessToken: aws.String(token)}
	paginator := sso.NewListAccountsPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, page.AccountList...)
	}
	return accounts, nil
}

func getAccountRoles(ctx context.Context, client *sso.Client, token, accountID string) ([]types.RoleInfo, error) {
	var roles []types.RoleInfo
	input := &sso.ListAccountRolesInput{
		AccessToken: aws.String(token),
		AccountId:   aws.String(accountID),
	}
	paginator := sso.NewListAccountRolesPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		roles = append(roles, page.RoleList...)
	}
	return roles, nil
}

func (i *awsIamIdentityCenter) SetEnvVars(info *schema.ConfigAndStacksInfo) error {
	log.Info("Setting AWS environment variables")

	err := CreateAwsFilesAndUpdateEnvVars(info, i.Identity.Identity, i.Common.Profile,  i.Provider, i.Common.Region, i.RoleArnToAssume)
	if err != nil {
		return err
	}

	// Merge identity-specific env overrides (preserve key casing)
	MergeIdentityEnvOverrides(info, i.Env)

	err = UpdateAwsAtmosConfig(i.Provider, i.Identity.Identity, i.Common.Profile, i.Common.Region, i.RoleArnToAssume)
	if err != nil {
		return err
	}

	return nil
}

func (i *awsIamIdentityCenter) Logout() error {
	return RemoveAwsCredentials(i.Common.Profile)
}

func RoleToAccountId(role string) string {
	roleArn, err := arn.Parse(role)
	if err != nil {
		log.Fatal(err)
	}
	return roleArn.AccountID
}

func SsoSyncE(startUrl, region string) (*ssooidc.CreateTokenOutput, error) {
	log.Debug("Syncing with SSO", "startUrl", startUrl, "region", region)
	oidc, regOut, ctx, err := getSsoOidcClients(region)

	// 3. Start device authorization
	authOut, err := oidc.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     regOut.ClientId,
		ClientSecret: regOut.ClientSecret,
		StartUrl:     aws.String(startUrl),
	})
	if err != nil {
		err = fmt.Errorf("start device auth failed: %w", err)
		log.Error(err)
		return nil, err
	}
	err = utils.OpenUrl(*authOut.VerificationUriComplete)
	if err != nil {
		log.Debug(err)
		utils.PrintfMarkdown("🔐 Please visit %s and enter code: %s", *authOut.VerificationUriComplete, *authOut.UserCode)
	}
	// 4. Poll for token
	var tokenOut *ssooidc.CreateTokenOutput
	for {
		tokenOut, err = oidc.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     regOut.ClientId,
			ClientSecret: regOut.ClientSecret,
			DeviceCode:   authOut.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})

		if err == nil {
			break // success
		}

		var authPending *ssooidctypes.AuthorizationPendingException
		var slowDown *ssooidctypes.SlowDownException

		switch {
		case errors.As(err, &authPending):
			// Keep polling — user hasn't logged in yet
			time.Sleep(time.Duration(authOut.Interval) * time.Second)
		case errors.As(err, &slowDown):
			// AWS asked us to slow down
			time.Sleep(time.Duration(authOut.Interval+2) * time.Second)
		default:
			return nil, err
		}
	}

	return tokenOut, nil
}

func getSsoOidcClients(region string) (*ssooidc.Client, *ssooidc.RegisterClientOutput, context.Context, error) {
	// 1. Load config for SSO region
	ctx := context.Background()
	oidc := ssooidc.New(ssooidc.Options{
		Region: region,
	})

	// 2. Register client
	regOut, err := oidc.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("atmos-sso"),
		ClientType: aws.String("public"),
	})
	if err != nil {
		err = fmt.Errorf("failed to register client: %w", err)
		log.Error(err)
		return nil, nil, nil, err
	}
	return oidc, regOut, ctx, nil
}
