package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/cloudposse/atmos/internal/auth/authstore"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/zalando/go-keyring"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/charmbracelet/log"
)

type awsIamIdentityCenter struct {
	Common   schema.IdentityProviderDefaultConfig `yaml:",inline"`
	Identity schema.Identity                      `yaml:",inline"`

	// SSO
	Role        string `yaml:"role,omitempty" json:"role,omitempty" mapstructure:"role,omitempty"`
	AccountId   string `yaml:"account_id,omitempty" json:"account_id,omitempty" mapstructure:"account_id,omitempty"`
	AccountName string `yaml:"account_name,omitempty" json:"account_name,omitempty" mapstructure:"account_name,omitempty"`
	RoleName    string `yaml:"role_name,omitempty" json:"role_name,omitempty" mapstructure:"role_name,omitempty"`
}

func (config *awsIamIdentityCenter) Login() error {

	log.Debug("Identity Config", "config", config)
	ctx := context.Background()
	store := authstore.NewKeyringAuthStore()
	keyringKey := fmt.Sprintf("%s-%s", config.Common.Alias, config.Identity.Profile)
	log.Info("Logging in using IAM Identity Center", "Region", config.Common.Region, "Alias", config.Common.Alias, "Profile", config.Identity.Profile)

	ssoClient := sso.New(sso.Options{
		Region: config.Common.Region,
	})

	var credentials *authstore.AuthCredential
	// 1. Log into Method - Perhaps we already have a valid token
	cred, err := store.Get(keyringKey)
	if err == nil && store.IsValid(cred) {
		// 1.A Valid Token, proceed.
		log.Debug("Cached token found", "alias", config.Common.Alias)
		credentials = cred
	} else {
		// 1.B No valid token, log in
		// 1.B.1. Start SSO flow
		tokenOut, err := SsoSyncE(config.Common.Url, config.Common.Region)
		if err != nil {
			return err
		}

		accessToken := *tokenOut.AccessToken
		log.Info("✅ Logged in! Token acquired.")

		// This is our struct to store the login data in a meaningful way
		// We need to store this so we know when the token expires, and for what identity and configuration we are using
		newCred := &authstore.AuthCredential{
			Method:    authstore.MethodSSO,
			Token:     accessToken,
			ExpiresAt: time.Now().Add(time.Duration(tokenOut.ExpiresIn) * time.Second),
		}

		credentials = newCred
		err = store.Set(keyringKey, newCred)
		if err != nil {
			log.Warn("failed to save token:", "error", err)
		}
	}

	// 2. Authenticate and store to ~/.aws/credentials
	var accountId, RoleName string
	// Support passing in a role
	if config.Role != "" {
		Arn, _ := arn.Parse(config.Role)
		RoleName = Arn.Resource
		accountId = RoleToAccountId(config.Role)
	}

	// Support passing in a role_name
	if RoleName == "" {
		RoleName = config.RoleName
	}

	// Support passing in an account_id
	if accountId == "" {
		accountId = config.AccountId
		// Support passing in an account_name
		if accountId == "" {
			accounts, _ := getAccountsInfo(ctx, ssoClient, credentials.Token)
			for _, account := range accounts {
				if *account.AccountName == config.AccountName {
					accountId = *account.AccountId
					break
				}
			}
		}
	}

	// We now have the information we need to get credentials
	roleCredentials, err := ssoClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccessToken: aws.String(credentials.Token),
		AccountId:   aws.String(accountId),
		RoleName:    aws.String(RoleName),
	})
	log.Debug("Role Credentials", "role_name", RoleName, "accountid", accountId, "role", config.Role)
	if err != nil {
		var roles []string
		r, _ := getAccountRoles(ctx, ssoClient, credentials.Token, accountId)
		for _, role := range r {
			roles = append(roles, *role.RoleName)
		}
		log.Error("Failed to get role credentials", "error", err, "account_id", accountId, "role_name", RoleName, "available roles", roles)
		return err
	}

	WriteAwsCredentials(config.Identity.Profile, *roleCredentials.RoleCredentials.AccessKeyId, *roleCredentials.RoleCredentials.SecretAccessKey, *roleCredentials.RoleCredentials.SessionToken, config.Common.Alias)

	log.Info("✅ Logged in! Credentials written to ~/.aws/credentials", "profile", config.Identity.Profile)

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

const (
	service = "atmos-auth"
)

func StoreSSOToken(profile string, token string) error {
	return keyring.Set(service, profile, token)
}

func GetSSOToken(profile string) (string, error) {
	return keyring.Get(service, profile)
}

func DeleteSSOToken(profile string) error {
	return keyring.Delete(service, profile)
}

func (i *awsIamIdentityCenter) Logout() error {
	return nil
}

func (i *awsIamIdentityCenter) Validate() error {
	return nil
}

func RoleToAccountId(role string) string {
	roleArn, err := arn.Parse(role)
	if err != nil {
		log.Fatal(err)
	}
	return roleArn.AccountID
}
func SsoSync(startUrl, region string) *ssooidc.CreateTokenOutput {
	tokenOut, err := SsoSyncE(startUrl, region)
	if err != nil {
		log.Error(err)
	}
	return tokenOut
}

func SsoSyncE(startUrl, region string) (*ssooidc.CreateTokenOutput, error) {
	log.Debug("Syncing with SSO", "startUrl", startUrl, "region", region)
	// 1. Load config for SSO region
	ctx := context.Background()
	//cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	//if err != nil {
	//	err = fmt.Errorf("failed to load config: %w", err)
	//	log.Error(err)
	//	return nil, err
	//}
	//oidc := ssooidc.NewFromConfig(cfg)
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
		return nil, err
	}

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
		log.Warn(err)
		log.Infof("🔐 Please visit %s and enter code: %s", *authOut.VerificationUriComplete, *authOut.UserCode)
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
