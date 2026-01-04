package aws

import (
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	authUtils "github.com/cloudposse/atmos/pkg/auth/utils"
	"github.com/cloudposse/atmos/pkg/ui"
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

// promptCredentialsGeneric is the generic implementation that builds a form from the spec.
func promptCredentialsGeneric(spec types.CredentialPromptSpec) (map[string]string, error) {
	_ = ui.Writeln("")
	_ = ui.Warning(fmt.Sprintf("%s credentials are required for identity: %s", spec.CloudType, spec.IdentityName))
	_ = ui.Writeln("")

	// Build form fields from spec.
	values := make(map[string]*string)
	formFields := buildFormFields(spec.Fields, values)

	form := huh.NewForm(huh.NewGroup(formFields...)).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("%w: credential input cancelled: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Build result map.
	result := make(map[string]string)
	for name, valuePtr := range values {
		result[name] = *valuePtr
	}

	return result, nil
}

// buildFormFields converts credential field specs into huh form fields.
func buildFormFields(fields []types.CredentialField, values map[string]*string) []huh.Field {
	var formFields []huh.Field

	for i := range fields {
		field := &fields[i]
		value := field.Default
		values[field.Name] = &value

		// For pre-populated required fields, show as note instead of input.
		if isPrePopulatedNote(field) {
			formFields = append(formFields, huh.NewNote().
				Title(field.Title+" (from configuration)").
				Description(field.Default))
			continue
		}

		formFields = append(formFields, buildInputField(field, values[field.Name]))
	}

	return formFields
}

// isPrePopulatedNote returns true if the field should be displayed as a read-only note.
func isPrePopulatedNote(field *types.CredentialField) bool {
	return field.Required && field.Description == "" && field.Default != "" && !field.Secret
}

// buildInputField creates a huh input field from a credential field spec.
func buildInputField(field *types.CredentialField, valuePtr *string) *huh.Input {
	input := huh.NewInput().
		Title(field.Title).
		Value(valuePtr)

	if field.Description != "" {
		input = input.Description(field.Description)
	}

	if field.Secret {
		input = input.EchoMode(huh.EchoModePassword)
	}

	input = applyValidator(input, field)

	return input
}

// applyValidator adds the appropriate validator to an input field.
func applyValidator(input *huh.Input, field *types.CredentialField) *huh.Input {
	if field.Validator != nil {
		return input.Validate(field.Validator)
	}
	if field.Required {
		return input.Validate(validateRequired)
	}
	return input
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

	// Store credentials in keyring.
	store := credentials.NewCredentialStore()
	if err := store.Store(identityName, creds); err != nil {
		return nil, fmt.Errorf("%w: failed to store credentials: %w", errUtils.ErrAwsAuth, err)
	}

	_ = ui.Success("Credentials saved to keyring: " + identityName)
	_ = ui.Writeln("")

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
