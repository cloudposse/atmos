// Package prompt provides shared secret input forms for auth identities.
package prompt

import (
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth/interactive"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Prompt is the generic implementation that builds a form from the spec.
// It fails fast with ErrAuthPromptUnavailable when stdin or stderr is not a terminal,
// because the form would otherwise be invisible and hang.
func Prompt(spec types.CredentialPromptSpec) (map[string]string, error) {
	defer perf.Track(nil, "prompt.Prompt")()

	if !interactive.Available() {
		return nil, errUtils.ErrAuthPromptUnavailable
	}

	ui.Writeln("")
	ui.Warning(fmt.Sprintf("%s credentials are required for identity: %s", spec.CloudType, spec.IdentityName))
	ui.Writeln("")

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

func validateRequired(s string) error {
	if s == "" {
		return errUtils.ErrMissingInput
	}
	return nil
}
