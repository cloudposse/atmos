package prompt

import (
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestIsPrePopulatedNote(t *testing.T) {
	tests := []struct {
		name     string
		field    types.CredentialField
		expected bool
	}{
		{
			name: "required field with default and no description is a note",
			field: types.CredentialField{
				Required: true,
				Default:  "some-value",
				Secret:   false,
			},
			expected: true,
		},
		{
			name: "required field with description is not a note",
			field: types.CredentialField{
				Required:    true,
				Default:     "some-value",
				Description: "help text",
				Secret:      false,
			},
			expected: false,
		},
		{
			name: "secret field is never a note",
			field: types.CredentialField{
				Required: true,
				Default:  "some-value",
				Secret:   true,
			},
			expected: false,
		},
		{
			name: "optional field is not a note",
			field: types.CredentialField{
				Required: false,
				Default:  "some-value",
			},
			expected: false,
		},
		{
			name: "required field without default is not a note",
			field: types.CredentialField{
				Required: true,
				Default:  "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrePopulatedNote(&tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildFormFields(t *testing.T) {
	t.Run("builds form fields for all field types", func(t *testing.T) {
		fields := []types.CredentialField{
			{Name: "required_field", Title: "Required", Required: true},
			{Name: "secret_field", Title: "Secret", Required: true, Secret: true},
			{Name: "optional_field", Title: "Optional", Required: false, Description: "help text"},
			{Name: "prepopulated", Title: "Pre-populated", Required: true, Default: "default-value"},
		}

		values := make(map[string]*string)
		formFields := buildFormFields(fields, values)

		// Should have 4 form fields (3 inputs + 1 note for prepopulated).
		assert.Len(t, formFields, 4)

		// Values map should have entries for all fields.
		assert.Len(t, values, 4)
		assert.Contains(t, values, "required_field")
		assert.Contains(t, values, "secret_field")
		assert.Contains(t, values, "optional_field")
		assert.Contains(t, values, "prepopulated")

		// Prepopulated field should have its default value.
		assert.Equal(t, "default-value", *values["prepopulated"])
	})

	t.Run("empty fields produces empty form", func(t *testing.T) {
		fields := []types.CredentialField{}
		values := make(map[string]*string)
		formFields := buildFormFields(fields, values)

		assert.Empty(t, formFields)
		assert.Empty(t, values)
	})
}

func TestBuildInputField(t *testing.T) {
	t.Run("builds basic input field", func(t *testing.T) {
		field := &types.CredentialField{
			Name:     "test_field",
			Title:    "Test Field",
			Required: true,
		}

		value := ""
		input := buildInputField(field, &value)
		assert.NotNil(t, input)
	})

	t.Run("builds input field with description", func(t *testing.T) {
		field := &types.CredentialField{
			Name:        "test_field",
			Title:       "Test Field",
			Description: "Enter a value",
			Required:    false,
		}

		value := ""
		input := buildInputField(field, &value)
		assert.NotNil(t, input)
	})

	t.Run("builds secret input field", func(t *testing.T) {
		field := &types.CredentialField{
			Name:     "secret_field",
			Title:    "Secret Field",
			Required: true,
			Secret:   true,
		}

		value := ""
		input := buildInputField(field, &value)
		assert.NotNil(t, input)
	})

	t.Run("builds input field with custom validator", func(t *testing.T) {
		customValidator := func(s string) error {
			return nil
		}

		field := &types.CredentialField{
			Name:      "validated_field",
			Title:     "Validated Field",
			Validator: customValidator,
		}

		value := ""
		input := buildInputField(field, &value)
		assert.NotNil(t, input)
	})
}

func TestApplyValidator(t *testing.T) {
	t.Run("uses custom validator when provided", func(t *testing.T) {
		customValidator := func(s string) error {
			if s != "expected" {
				return errUtils.ErrMissingInput
			}
			return nil
		}

		field := &types.CredentialField{
			Required:  true,
			Validator: customValidator,
		}

		value := ""
		baseInput := huh.NewInput().Value(&value)
		result := applyValidator(baseInput, field)
		// Verify the function returns a non-nil input.
		assert.NotNil(t, result)
	})

	t.Run("uses required validator when field is required and no custom validator", func(t *testing.T) {
		field := &types.CredentialField{
			Required:  true,
			Validator: nil,
		}

		value := ""
		baseInput := huh.NewInput().Value(&value)
		result := applyValidator(baseInput, field)
		assert.NotNil(t, result)
	})

	t.Run("no validator for optional field without custom validator", func(t *testing.T) {
		field := &types.CredentialField{
			Required:  false,
			Validator: nil,
		}

		value := ""
		baseInput := huh.NewInput().Value(&value)
		result := applyValidator(baseInput, field)
		assert.NotNil(t, result)
	})
}

func TestPromptUnavailable(t *testing.T) {
	previous := viper.Get("interactive")
	viper.Set("interactive", false)
	t.Cleanup(func() { viper.Set("interactive", previous) })
	values, err := Prompt(types.CredentialPromptSpec{IdentityName: "registry", CloudType: "OCI"})
	require.ErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	require.Nil(t, values)
	require.ErrorIs(t, validateRequired(""), errUtils.ErrMissingInput)
	require.NoError(t, validateRequired("secret"))
}
