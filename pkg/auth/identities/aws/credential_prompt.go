package aws

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials/prompt"
	"github.com/cloudposse/atmos/pkg/auth/types"
	authUtils "github.com/cloudposse/atmos/pkg/auth/utils"
)

// AWS credential field names.
const (
	FieldAccessKeyID     = "access_key_id"
	FieldSecretAccessKey = "secret_access_key"
	FieldMfaArn          = "mfa_arn"
	FieldSessionDuration = "session_duration"
)

// GenericPromptCredentialsFunc is the generic credential prompting function.
// It uses CredentialPromptSpec to determine what fields to collect.
var GenericPromptCredentialsFunc types.CredentialPromptFunc

// PromptCredentialsFunc is the AWS-specific credential prompting function.
// It wraps the generic function for backward compatibility.
// When set, it's called when credentials are missing or invalid.
var PromptCredentialsFunc func(identityName string, mfaArn string) (*types.AWSCredentials, error)

func init() {
	// Register the generic credential prompting function.
	// This allows the auth login flow to prompt for credentials when they're missing or invalid.
	GenericPromptCredentialsFunc = promptCredentialsGeneric

	// Set up the AWS-specific wrapper that uses the generic prompting.
	PromptCredentialsFunc = promptForAWSCredentials
}

// promptCredentialsGeneric delegates to the shared authentication form renderer.
func promptCredentialsGeneric(spec types.CredentialPromptSpec) (map[string]string, error) {
	return prompt.Prompt(spec)
}

// promptForAWSCredentials prompts the user for AWS credentials and stores them in the keyring.
// This is the AWS-specific wrapper that uses the generic prompting function.
func promptForAWSCredentials(identityName string, mfaArn string) (*types.AWSCredentials, error) {
	spec := buildAWSCredentialSpec(identityName, mfaArn)

	values, err := promptCredentialsGeneric(spec)
	if err != nil {
		return nil, err
	}

	creds := &types.AWSCredentials{
		AccessKeyID:     values[FieldAccessKeyID],
		SecretAccessKey: values[FieldSecretAccessKey],
		MfaArn:          values[FieldMfaArn],
		SessionDuration: values[FieldSessionDuration],
	}

	// Note: Credentials are stored by the auth manager after authentication succeeds.
	// The manager uses the proper realm for credential isolation.
	// We just return the credentials here - storage is handled at a higher level.

	return creds, nil
}

// buildAWSCredentialSpec creates the credential prompt specification for AWS IAM User.
func buildAWSCredentialSpec(identityName string, mfaArn string) types.CredentialPromptSpec {
	fields := []types.CredentialField{
		{
			Name:      FieldAccessKeyID,
			Title:     "AWS Access Key ID",
			Required:  true,
			Validator: validateRequired,
		},
		{
			Name:      FieldSecretAccessKey,
			Title:     "AWS Secret Access Key",
			Required:  true,
			Secret:    true,
			Validator: validateRequired,
		},
	}

	// MFA ARN - show as note if pre-configured, otherwise as input.
	if mfaArn != "" {
		fields = append(fields, types.CredentialField{
			Name:    FieldMfaArn,
			Title:   "MFA ARN (from configuration)",
			Default: mfaArn,
		})
	} else {
		fields = append(fields, types.CredentialField{
			Name:        FieldMfaArn,
			Title:       "MFA ARN (optional)",
			Description: "e.g., arn:aws:iam::123456789012:mfa/user",
			Required:    false,
		})
	}

	// Session Duration.
	fields = append(fields, types.CredentialField{
		Name:        FieldSessionDuration,
		Title:       "Session Duration (optional, default: 12h)",
		Description: "How long before you need to re-enter MFA. Examples: 1h, 12h, 36h (max with MFA)",
		Required:    false,
		Validator:   validateSessionDuration,
	})

	return types.CredentialPromptSpec{
		IdentityName: identityName,
		CloudType:    "AWS",
		Fields:       fields,
	}
}

// validateRequired validates that a field is not empty.
func validateRequired(s string) error {
	if s == "" {
		return errUtils.ErrMissingInput
	}
	return nil
}

// validateSessionDuration validates the session duration format.
func validateSessionDuration(s string) error {
	if s == "" {
		return nil // Optional field.
	}
	_, err := authUtils.ParseDurationFlexible(s)
	if err != nil {
		return fmt.Errorf("%w: use formats like 3600, 1h, 12h, 1d", errUtils.ErrInvalidDuration)
	}
	return nil
}
