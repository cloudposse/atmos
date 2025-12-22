package cmd

import (
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/identities/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	authUtils "github.com/cloudposse/atmos/pkg/auth/utils"
	"github.com/cloudposse/atmos/pkg/ui"
)

func init() {
	// Register the credential prompting function with the aws identity package.
	// This allows the auth login flow to prompt for credentials when they're missing or invalid.
	aws.PromptCredentialsFunc = promptForAWSCredentials
}

// promptForAWSCredentials prompts the user for AWS credentials and stores them in the keyring.
// This is called when credentials are missing or have been invalidated.
func promptForAWSCredentials(identityName string, mfaArn string) (*types.AWSCredentials, error) {
	_ = ui.Writeln("")
	_ = ui.Warning("AWS credentials are required for identity: " + identityName)
	_ = ui.Writeln("")

	// Build and run the credential form.
	creds, err := runCredentialForm(mfaArn)
	if err != nil {
		return nil, err
	}

	// Store credentials in keyring.
	store := credentials.NewCredentialStore()
	if err := store.Store(identityName, creds); err != nil {
		return nil, fmt.Errorf("%w: failed to store credentials: %w", errUtils.ErrAwsAuth, err)
	}

	_ = ui.Success("Credentials saved to keyring: " + identityName)
	_ = ui.Writeln("")

	return creds, nil
}

// runCredentialForm displays the credential input form and returns the collected credentials.
func runCredentialForm(mfaArn string) (*types.AWSCredentials, error) {
	var accessKeyID, secretAccessKey, mfaArnInput, sessionDuration string

	// If MFA ARN is provided from YAML, pre-populate it.
	mfaArnInput = mfaArn

	// Build form fields.
	formFields := buildCredentialFormFields(&accessKeyID, &secretAccessKey, &mfaArnInput, &sessionDuration, mfaArn)

	form := huh.NewForm(huh.NewGroup(formFields...)).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("%w: credential input cancelled: %w", errUtils.ErrAuthenticationFailed, err)
	}

	return &types.AWSCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		MfaArn:          mfaArnInput,
		SessionDuration: sessionDuration,
	}, nil
}

// buildCredentialFormFields creates the form fields for credential input.
func buildCredentialFormFields(accessKeyID, secretAccessKey, mfaArnInput, sessionDuration *string, mfaArn string) []huh.Field {
	var formFields []huh.Field

	// Access Key ID - required.
	formFields = append(formFields, huh.NewInput().
		Title("AWS Access Key ID").
		Value(accessKeyID).
		Validate(validateRequired))

	// Secret Access Key - required, password mode.
	formFields = append(formFields, huh.NewInput().
		Title("AWS Secret Access Key").
		Value(secretAccessKey).
		EchoMode(huh.EchoModePassword).
		Validate(validateRequired))

	// MFA ARN - optional, pre-populated if from YAML.
	if mfaArn != "" {
		formFields = append(formFields, huh.NewNote().
			Title("MFA ARN (from configuration)").
			Description(mfaArn))
	} else {
		formFields = append(formFields, huh.NewInput().
			Title("MFA ARN (optional)").
			Description("e.g., arn:aws:iam::123456789012:mfa/user").
			Value(mfaArnInput))
	}

	// Session Duration - optional.
	formFields = append(formFields, huh.NewInput().
		Title("Session Duration (optional, default: 12h)").
		Description("How long before you need to re-enter MFA. Examples: 1h, 12h, 36h (max with MFA)").
		Value(sessionDuration).
		Validate(validateSessionDurationFormat))

	return formFields
}

// validateRequired validates that a field is not empty.
func validateRequired(s string) error {
	if s == "" {
		return errUtils.ErrMissingInput
	}
	return nil
}

// validateSessionDurationFormat validates the session duration format.
func validateSessionDurationFormat(s string) error {
	if s == "" {
		return nil // Optional field.
	}
	_, err := authUtils.ParseDurationFlexible(s)
	if err != nil {
		return fmt.Errorf("%w: use formats like 3600, 1h, 12h, 1d", errUtils.ErrInvalidDuration)
	}
	return nil
}
