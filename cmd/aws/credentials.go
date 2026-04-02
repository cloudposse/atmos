package aws

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/identity"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// validateAWSCredentials performs an early check that AWS credentials are available and valid.
// It uses STS GetCallerIdentity which is lightweight and always works if credentials are valid.
// This should be called before any AWS API calls to provide a clear error message.
func validateAWSCredentials(ctx context.Context, region string) error {
	log.Debug("Validating AWS credentials via STS GetCallerIdentity")

	callerIdentity, err := identity.GetCallerIdentity(ctx, region, "", 0, nil)
	if err != nil {
		return errUtils.Build(errUtils.ErrAWSCredentialsNotValid).
			WithExplanation(fmt.Sprintf("Unable to verify AWS credentials: %s", err)).
			WithHint("Ensure AWS credentials are configured (e.g., via environment variables, ~/.aws/credentials, or SSO)").
			WithHint("Run `aws sts get-caller-identity` to verify your credentials").
			WithHint("If using Atmos auth, run `atmos auth login` to refresh credentials").
			WithExitCode(1).
			Err()
	}

	log.Debug("AWS credentials validated successfully",
		"account", callerIdentity.Account,
		"arn", callerIdentity.Arn,
	)

	return nil
}
