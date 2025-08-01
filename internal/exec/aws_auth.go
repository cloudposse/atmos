package exec

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/aws_utils/auth"
	"github.com/cloudposse/atmos/internal/tui/picker"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/cloudposse/atmos/internal/authstore"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"

	"github.com/zalando/go-keyring"

	"github.com/spf13/cobra"
)

const (
	service = "atmos-aws-auth"
)

func ExecuteAwsAuthCommand(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}
	Auth := atmosConfig.Auth
	if Auth == nil {
		return errors.New("no auth config found")
	}

	profile, err := flags.GetString("profile")
	if err != nil {
		return err
	}
	if profile == "" {
		items := []string{}
		for k, _ := range Auth {
			items = append(items, k)
		}
		choose, err := picker.NewSimplePicker("Choose an Auth Config", items).Choose()
		if err != nil {
			return err
		}
		log.Info("Selected profile", "profile", choose)
		profile = choose
	}

	config := Auth[profile]
	// TODO validate config
	return ExecuteAwsAuth(profile, config)
}

func ExecuteAwsAuth(alias string, config schema.AwsAuthConfig) error {
	ctx := context.Background()
	store := authstore.NewKeyringAuthStore()
	keyringKey := fmt.Sprintf("%s-%s", alias, config.Profile)
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
		fmt.Printf("Existing valid token for %s found. Skipping login.\n", alias)
		credentials = cred
	} else {
		// 1.B No valid token, log in
		// 1.B.1. Start SSO flow
		tokenOut := auth.SsoSync(config.StartUrl, config.Region)
		accessToken := *tokenOut.AccessToken
		fmt.Println("✅ Logged in! Token acquired.")

		newCred := &authstore.AuthCredential{
			Method:    authstore.MethodSSO,
			Token:     accessToken,
			ExpiresAt: time.Now().Add(time.Duration(tokenOut.ExpiresIn) * time.Second),
		}

		credentials = newCred
		err = store.Set(keyringKey, newCred)
		if err != nil {
			panic(fmt.Errorf("failed to save token: %w", err))
		}
	}

	accounts, err := listAccounts(ctx, ssoClient, credentials.Token)
	if err != nil {
		panic(fmt.Errorf("failed to list accounts: %w", err))
	}

	for i := 0; i < len(accounts); i++ {
		fmt.Println("Account:", *accounts[i].AccountName)
		roles, err := listAccountRoles(ctx, ssoClient, credentials.Token, *accounts[i].AccountId)
		if err != nil {
			panic(fmt.Errorf("failed to list roles: %w", err))
		}
		for j := 0; j < len(roles); j++ {
			fmt.Println("  Role:", *roles[j].RoleName)
		}
	}

	return nil
}

func listAccounts(ctx context.Context, client *sso.Client, token string) ([]types.AccountInfo, error) {
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

func listAccountRoles(ctx context.Context, client *sso.Client, token, accountID string) ([]types.RoleInfo, error) {
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

func StoreSSOToken(profile string, token string) error {
	return keyring.Set(service, profile, token)
}

func GetSSOToken(profile string) (string, error) {
	return keyring.Get(service, profile)
}

func DeleteSSOToken(profile string) error {
	return keyring.Delete(service, profile)
}
