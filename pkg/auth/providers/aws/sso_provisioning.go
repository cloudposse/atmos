package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/provisioning"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ssoClient interface for dependency injection in tests.
type ssoClient interface {
	ListAccounts(ctx context.Context, input *sso.ListAccountsInput, opts ...func(*sso.Options)) (*sso.ListAccountsOutput, error)
	ListAccountRoles(ctx context.Context, input *sso.ListAccountRolesInput, opts ...func(*sso.Options)) (*sso.ListAccountRolesOutput, error)
}

// ProvisionIdentities provisions identities from AWS SSO permission sets.
// This method is called after authentication to discover available accounts and roles.
func (p *ssoProvider) ProvisionIdentities(ctx context.Context, creds authTypes.ICredentials) (*provisioning.Result, error) {
	defer perf.Track(nil, "aws.ssoProvider.ProvisionIdentities")()

	// Only provision if enabled.
	if p.config.AutoProvisionIdentities == nil || !*p.config.AutoProvisionIdentities {
		log.Debug("Auto-provisioning disabled for provider", "provider", p.name)
		return nil, nil
	}

	log.Debug("Starting identity provisioning for SSO provider", "provider", p.name)

	// Extract SSO access token from credentials.
	awsCreds, ok := creds.(*authTypes.AWSCredentials)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrSSOProvisioningFailed).
			WithHintf("Invalid credentials type for SSO identity provisioning").
			WithHint("Ensure the provider successfully authenticated before provisioning identities").
			WithHint("Check your AWS SSO configuration in atmos.yaml").
			WithContext("provider", p.name).
			WithExitCode(1).
			Err()
	}

	// Create SSO client if not injected (for testing).
	client := p.ssoClient
	if client == nil {
		client = sso.NewFromConfig(aws.Config{
			Region: p.region,
		})
	}

	return p.provisionIdentitiesWithClient(ctx, client, awsCreds)
}

// provisionIdentitiesWithClient provisions identities using a provided SSO client.
// This enables dependency injection for testing.
func (p *ssoProvider) provisionIdentitiesWithClient(ctx context.Context, ssoClient ssoClient, creds *authTypes.AWSCredentials) (*provisioning.Result, error) {
	accessToken := creds.AccessKeyID // SSO token is stored in AccessKeyID field.

	// List all accounts accessible to this user.
	accounts, err := p.listAccountsWithClient(ctx, ssoClient, accessToken)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrSSOAccountListFailed).
			WithHintf("Failed to list AWS SSO accounts for identity provisioning").
			WithHint("Verify your AWS SSO session is still active with 'aws sso login'").
			WithHint("Ensure your SSO user has permissions to list accounts").
			WithHintf("Check that the SSO start URL '%s' is correct", p.startURL).
			WithContext("provider", p.name).
			WithContext("start_url", p.startURL).
			WithContext("region", p.region).
			WithExitCode(1).
			Err()
	}

	log.Debug("Listed SSO accounts", "count", len(accounts))

	// For each account, list available roles.
	identities := make(map[string]*schema.Identity)
	roleCount := 0

	for _, account := range accounts {
		accountID := aws.ToString(account.AccountId)
		accountName := aws.ToString(account.AccountName)

		roles, err := p.listAccountRolesWithClient(ctx, ssoClient, accessToken, accountID)
		if err != nil {
			log.Warn("Failed to list roles for account, skipping", "account", accountName, "accountID", accountID, "error", err)
			continue
		}

		log.Debug("Listed roles for account", "account", accountName, "accountID", accountID, "count", len(roles))

		// Create an identity for each role.
		for _, role := range roles {
			roleName := aws.ToString(role.RoleName)
			roleCount++

			// Generate unique identity name: account-name/role-name.
			identityName := fmt.Sprintf("%s/%s", accountName, roleName)

			principal := &schema.Principal{
				Name: roleName,
				Account: &schema.Account{
					Name: accountName,
					ID:   accountID,
				},
			}

			identities[identityName] = &schema.Identity{
				Kind:      "aws/permission-set",
				Provider:  p.name,
				Via:       &schema.IdentityVia{Provider: "aws-sso"},
				Principal: principal.ToMap(),
			}
		}
	}

	log.Debug("Provisioned SSO identities", "provider", p.name, "accounts", len(accounts), "roles", roleCount, "identities", len(identities))

	return &provisioning.Result{
		Identities:    identities,
		Provider:      p.name,
		ProvisionedAt: time.Now(),
		Metadata: provisioning.Metadata{
			Source: "aws-sso",
			Counts: &provisioning.Counts{
				Accounts:   len(accounts),
				Roles:      roleCount,
				Identities: len(identities),
			},
			Extra: map[string]interface{}{
				"start_url": p.startURL,
				"region":    p.region,
			},
		},
	}, nil
}

// listAccountsWithClient is a testable version that accepts a client interface.
func (p *ssoProvider) listAccountsWithClient(ctx context.Context, ssoClient ssoClient, accessToken string) ([]ssotypes.AccountInfo, error) {
	defer perf.Track(nil, "aws.ssoProvider.listAccounts")()

	var accounts []ssotypes.AccountInfo
	var nextToken *string

	for {
		output, err := ssoClient.ListAccounts(ctx, &sso.ListAccountsInput{
			AccessToken: aws.String(accessToken),
			NextToken:   nextToken,
		})
		if err != nil {
			return nil, errors.Join(errUtils.ErrSSOAccountListFailed, err)
		}

		accounts = append(accounts, output.AccountList...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return accounts, nil
}

// listAccountRolesWithClient is a testable version that accepts a client interface.
func (p *ssoProvider) listAccountRolesWithClient(ctx context.Context, ssoClient ssoClient, accessToken, accountID string) ([]ssotypes.RoleInfo, error) {
	defer perf.Track(nil, "aws.ssoProvider.listAccountRoles")()

	var roles []ssotypes.RoleInfo
	var nextToken *string

	for {
		output, err := ssoClient.ListAccountRoles(ctx, &sso.ListAccountRolesInput{
			AccessToken: aws.String(accessToken),
			AccountId:   aws.String(accountID),
			NextToken:   nextToken,
		})
		if err != nil {
			return nil, errors.Join(errUtils.ErrSSORoleListFailed, fmt.Errorf("account %s: %w", accountID, err))
		}

		roles = append(roles, output.RoleList...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return roles, nil
}
