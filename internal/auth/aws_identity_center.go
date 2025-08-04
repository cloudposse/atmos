package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/cloudposse/atmos/internal/auth/authstore"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/zalando/go-keyring"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/charmbracelet/log"
)

type awsIamIdentityCenter struct {
	schema.IdentityDefaultConfig `yaml:",inline"`

	// SSO
	Role string `yaml:"role,omitempty" json:"role,omitempty" mapstructure:"role,omitempty"`
	Url  string `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url,omitempty"`
}

func (config *awsIamIdentityCenter) Login() error {
	log.Info("Logging in using IAM Identity Center")

	log.Debug("Identity Config", "config", config)
	ctx := context.Background()
	store := authstore.NewKeyringAuthStore()
	keyringKey := fmt.Sprintf("%s-%s", config.Alias, config.Profile)
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(config.Region))

	if err != nil {
		panic(fmt.Errorf("failed to load config: %w", err))
	}
	ssoClient := sso.NewFromConfig(cfg)

	var credentials *authstore.AuthCredential
	// 1. Log into Method - Perhaps we already have a valid token
	cred, err := store.Get(keyringKey)
	if err == nil && store.IsValid(cred) {
		// 1.A Valid Token, proceed.
		log.Info("Valid token found", "alias", config.Alias)
		credentials = cred
	} else {
		// 1.B No valid token, log in
		// 1.B.1. Start SSO flow
		tokenOut := SsoSync(config.Url, config.Region)
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
			return fmt.Errorf("failed to save token: %w", err)
		}
	}

	// 2. Authenticate and store to ~/.aws/credentials
	Arn, _ := arn.Parse(config.Role)
	accountId := RoleToAccountId(config.Role)
	roleCredentials, err := ssoClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccessToken: aws.String(credentials.Token),
		AccountId:   aws.String(accountId),
		RoleName:    aws.String(Arn.Resource),
	})
	log.Debug("Role Credentials", "role_name", Arn.Resource, "accountid", accountId, "rolename", config.Role)
	if err != nil {
		var roles []string
		r, _ := getAccountRoles(ctx, ssoClient, credentials.Token, accountId)
		for _, role := range r {
			roles = append(roles, *role.RoleName)
		}
		log.Error("Failed to get role credentials", "error", err, "available roles", roles)
		return err
	}

	// Resolve home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("failed to get user home directory: %w", err))
	}

	// Path to ~/.aws/credentials
	awsCredentialsPath := filepath.Join(homeDir, ".aws", "credentials")
	content := fmt.Sprintf(`
[%s]
aws_access_key_id=%s
aws_secret_access_key=%s
aws_session_token=%s
`, config.Profile, *roleCredentials.RoleCredentials.AccessKeyId, *roleCredentials.RoleCredentials.SecretAccessKey, *roleCredentials.RoleCredentials.SessionToken)
	err = os.WriteFile(awsCredentialsPath, []byte(content), 0600)
	if err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	log.Info("✅ Logged in! Credentials written to ~/.aws/credentials", "profile", config.Profile)

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
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	oidc := ssooidc.NewFromConfig(cfg)

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
		log.Error(err)
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
