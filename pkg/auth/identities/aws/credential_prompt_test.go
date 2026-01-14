package aws

import (
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestValidateRequired(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty string returns error",
			input:   "",
			wantErr: true,
		},
		{
			name:    "non-empty string returns nil",
			input:   "AKIAIOSFODNN7EXAMPLE",
			wantErr: false,
		},
		{
			name:    "whitespace only is valid",
			input:   "   ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequired(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrMissingInput)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateSessionDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty string is valid (optional)",
			input:   "",
			wantErr: false,
		},
		{
			name:    "valid hours format",
			input:   "12h",
			wantErr: false,
		},
		{
			name:    "valid minutes format",
			input:   "30m",
			wantErr: false,
		},
		{
			name:    "valid seconds as integer",
			input:   "3600",
			wantErr: false,
		},
		{
			name:    "valid combined format",
			input:   "1h30m",
			wantErr: false,
		},
		{
			name:    "valid day format",
			input:   "1d",
			wantErr: false,
		},
		{
			name:    "invalid format returns error",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "negative duration is invalid",
			input:   "-1h",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionDuration(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidDuration)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBuildAWSCredentialSpec(t *testing.T) {
	t.Run("without MFA ARN creates 4 fields", func(t *testing.T) {
		spec := buildAWSCredentialSpec("test-identity", "")

		assert.Equal(t, "test-identity", spec.IdentityName)
		assert.Equal(t, "AWS", spec.CloudType)
		assert.Len(t, spec.Fields, 4)

		// Verify field names.
		assert.Equal(t, FieldAccessKeyID, spec.Fields[0].Name)
		assert.Equal(t, FieldSecretAccessKey, spec.Fields[1].Name)
		assert.Equal(t, FieldMfaArn, spec.Fields[2].Name)
		assert.Equal(t, FieldSessionDuration, spec.Fields[3].Name)

		// MFA ARN should be optional input (no default).
		assert.Equal(t, "", spec.Fields[2].Default)
		assert.False(t, spec.Fields[2].Required)
	})

	t.Run("with MFA ARN creates 4 fields with pre-populated MFA", func(t *testing.T) {
		mfaArn := "arn:aws:iam::123456789012:mfa/user"
		spec := buildAWSCredentialSpec("test-identity", mfaArn)

		assert.Equal(t, "test-identity", spec.IdentityName)
		assert.Equal(t, "AWS", spec.CloudType)
		assert.Len(t, spec.Fields, 4)

		// MFA ARN should have default value.
		assert.Equal(t, FieldMfaArn, spec.Fields[2].Name)
		assert.Equal(t, mfaArn, spec.Fields[2].Default)
	})

	t.Run("access key and secret key are required and secret", func(t *testing.T) {
		spec := buildAWSCredentialSpec("test-identity", "")

		// Access Key ID - required, not secret.
		assert.True(t, spec.Fields[0].Required)
		assert.False(t, spec.Fields[0].Secret)

		// Secret Access Key - required and secret.
		assert.True(t, spec.Fields[1].Required)
		assert.True(t, spec.Fields[1].Secret)
	})
}

func TestCredentialPromptSpec(t *testing.T) {
	t.Run("CredentialPromptResult Get returns value", func(t *testing.T) {
		result := &types.CredentialPromptResult{
			Values: map[string]string{
				"access_key_id": "AKIATEST",
				"secret":        "SECRET123",
			},
		}

		assert.Equal(t, "AKIATEST", result.Get("access_key_id"))
		assert.Equal(t, "SECRET123", result.Get("secret"))
		assert.Equal(t, "", result.Get("nonexistent"))
	})

	t.Run("CredentialPromptResult Get handles nil map", func(t *testing.T) {
		result := &types.CredentialPromptResult{}
		assert.Equal(t, "", result.Get("any_field"))
	})
}

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

func TestBuildAWSCredentialSpec_FieldDetails(t *testing.T) {
	t.Run("without MFA ARN has description for MFA field", func(t *testing.T) {
		spec := buildAWSCredentialSpec("test-identity", "")

		// MFA ARN field should have description when not pre-configured.
		mfaField := spec.Fields[2]
		assert.Equal(t, FieldMfaArn, mfaField.Name)
		assert.Equal(t, "MFA ARN (optional)", mfaField.Title)
		assert.Contains(t, mfaField.Description, "arn:aws:iam::")
		assert.False(t, mfaField.Required)
	})

	t.Run("with MFA ARN has no description", func(t *testing.T) {
		mfaArn := "arn:aws:iam::123456789012:mfa/user"
		spec := buildAWSCredentialSpec("test-identity", mfaArn)

		// MFA ARN field should have no description when pre-configured.
		mfaField := spec.Fields[2]
		assert.Equal(t, FieldMfaArn, mfaField.Name)
		assert.Equal(t, "MFA ARN (from configuration)", mfaField.Title)
		assert.Empty(t, mfaField.Description)
		assert.Equal(t, mfaArn, mfaField.Default)
	})

	t.Run("session duration field has validator", func(t *testing.T) {
		spec := buildAWSCredentialSpec("test-identity", "")

		// Session duration field should have a validator.
		sessionField := spec.Fields[3]
		assert.Equal(t, FieldSessionDuration, sessionField.Name)
		assert.NotNil(t, sessionField.Validator)
		assert.False(t, sessionField.Required)

		// Verify the validator works correctly.
		assert.NoError(t, sessionField.Validator("12h"))
		assert.NoError(t, sessionField.Validator(""))
		assert.Error(t, sessionField.Validator("invalid"))
	})

	t.Run("access key field has validator", func(t *testing.T) {
		spec := buildAWSCredentialSpec("test-identity", "")

		accessKeyField := spec.Fields[0]
		assert.NotNil(t, accessKeyField.Validator)
		assert.NoError(t, accessKeyField.Validator("AKIAIOSFODNN7EXAMPLE"))
		assert.Error(t, accessKeyField.Validator(""))
	})

	t.Run("secret key field has validator", func(t *testing.T) {
		spec := buildAWSCredentialSpec("test-identity", "")

		secretKeyField := spec.Fields[1]
		assert.NotNil(t, secretKeyField.Validator)
		assert.NoError(t, secretKeyField.Validator("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"))
		assert.Error(t, secretKeyField.Validator(""))
	})
}

func TestFieldConstants(t *testing.T) {
	// Verify field constants match expected values.
	assert.Equal(t, "access_key_id", FieldAccessKeyID)
	assert.Equal(t, "secret_access_key", FieldSecretAccessKey)
	assert.Equal(t, "mfa_arn", FieldMfaArn)
	assert.Equal(t, "session_duration", FieldSessionDuration)
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
