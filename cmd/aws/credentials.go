package aws

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/aws/identity"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveAuthContext resolves an Atmos Auth identity to an AWSAuthContext.
// If identityName is empty, returns nil (use default AWS credential chain).
func resolveAuthContext(atmosConfig *schema.AtmosConfiguration, identityName string) (*schema.AWSAuthContext, error) {
	if identityName == "" {
		return nil, nil
	}

	log.Debug("Resolving Atmos Auth identity for AWS security", "identity", identityName)

	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAWSCredentialsNotValid).
			WithExplanation(fmt.Sprintf("Failed to initialize auth manager: %s", err)).
			WithHint(fmt.Sprintf("Check the auth configuration in atmos.yaml for identity %q", identityName)).
			WithHint("Run `atmos auth list` to see configured identities").
			WithExitCode(1).
			Err()
	}

	envVars, err := authManager.GetEnvironmentVariables(identityName)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAWSCredentialsNotValid).
			WithExplanation(fmt.Sprintf("Failed to resolve identity %q: %s", identityName, err)).
			WithHint(fmt.Sprintf("Run `atmos auth login --identity %s` to authenticate", identityName)).
			WithHint("Run `atmos auth list` to see available identities").
			WithExitCode(1).
			Err()
	}

	authCtx := &schema.AWSAuthContext{
		CredentialsFile: envVars["AWS_SHARED_CREDENTIALS_FILE"],
		ConfigFile:      envVars["AWS_CONFIG_FILE"],
		Profile:         envVars["AWS_PROFILE"],
		Region:          envVars["AWS_REGION"],
	}

	log.Debug("Resolved Atmos Auth identity",
		"identity", identityName,
		"profile", authCtx.Profile,
		"region", authCtx.Region,
	)

	return authCtx, nil
}

// validateAWSCredentials performs an early check that AWS credentials are available and valid.
// It uses STS GetCallerIdentity which is lightweight and always works if credentials are valid.
// If authCtx is provided, credentials from the Atmos Auth identity are used.
func validateAWSCredentials(ctx context.Context, region string, authCtx *schema.AWSAuthContext) error {
	log.Debug("Validating AWS credentials via STS GetCallerIdentity")

	callerIdentity, err := identity.GetCallerIdentity(ctx, region, "", 0, authCtx)
	if err != nil {
		hint := "Ensure AWS credentials are configured (e.g., via environment variables, ~/.aws/credentials, or SSO)"
		if authCtx != nil {
			hint = "Run `atmos auth login` to refresh credentials for the configured identity"
		}
		return errUtils.Build(errUtils.ErrAWSCredentialsNotValid).
			WithExplanation(fmt.Sprintf("Unable to verify AWS credentials: %s", err)).
			WithHint(hint).
			WithHint("Run `aws sts get-caller-identity` to verify your credentials").
			WithExitCode(1).
			Err()
	}

	log.Debug("AWS credentials validated successfully",
		"account", callerIdentity.Account,
		"arn", callerIdentity.Arn,
	)

	return nil
}
