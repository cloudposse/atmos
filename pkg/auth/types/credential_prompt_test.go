package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCredentialPromptResult_Get(t *testing.T) {
	t.Run("returns value when key exists", func(t *testing.T) {
		result := &CredentialPromptResult{
			Values: map[string]string{
				"access_key_id":     "AKIATEST",
				"secret_access_key": "SECRET123",
			},
		}

		assert.Equal(t, "AKIATEST", result.Get("access_key_id"))
		assert.Equal(t, "SECRET123", result.Get("secret_access_key"))
	})

	t.Run("returns empty string for nonexistent key", func(t *testing.T) {
		result := &CredentialPromptResult{
			Values: map[string]string{
				"access_key_id": "AKIATEST",
			},
		}

		assert.Equal(t, "", result.Get("nonexistent"))
	})

	t.Run("handles nil map", func(t *testing.T) {
		result := &CredentialPromptResult{}
		assert.Equal(t, "", result.Get("any_field"))
	})

	t.Run("handles nil Values explicitly", func(t *testing.T) {
		result := &CredentialPromptResult{Values: nil}
		assert.Equal(t, "", result.Get("any_field"))
	})
}

func TestCredentialField_Validator(t *testing.T) {
	t.Run("field with nil validator", func(t *testing.T) {
		field := CredentialField{
			Name:      "test",
			Required:  true,
			Validator: nil,
		}
		assert.Nil(t, field.Validator)
	})

	t.Run("field with custom validator", func(t *testing.T) {
		customErr := errors.New("custom error")
		field := CredentialField{
			Name:     "test",
			Required: true,
			Validator: func(s string) error {
				if s == "" {
					return customErr
				}
				return nil
			},
		}

		assert.NotNil(t, field.Validator)
		assert.Error(t, field.Validator(""))
		assert.NoError(t, field.Validator("value"))
	})
}

func TestCredentialPromptSpec(t *testing.T) {
	t.Run("spec with all fields", func(t *testing.T) {
		spec := CredentialPromptSpec{
			IdentityName: "test-identity",
			CloudType:    "aws",
			Fields: []CredentialField{
				{Name: "field1", Title: "Field 1", Required: true},
				{Name: "field2", Title: "Field 2", Required: false, Secret: true},
			},
		}

		assert.Equal(t, "test-identity", spec.IdentityName)
		assert.Equal(t, "aws", spec.CloudType)
		assert.Len(t, spec.Fields, 2)
		assert.Equal(t, "field1", spec.Fields[0].Name)
		assert.True(t, spec.Fields[0].Required)
		assert.False(t, spec.Fields[0].Secret)
		assert.Equal(t, "field2", spec.Fields[1].Name)
		assert.False(t, spec.Fields[1].Required)
		assert.True(t, spec.Fields[1].Secret)
	})

	t.Run("spec with empty fields", func(t *testing.T) {
		spec := CredentialPromptSpec{
			IdentityName: "empty",
			CloudType:    "gcp",
			Fields:       []CredentialField{},
		}

		assert.Equal(t, "empty", spec.IdentityName)
		assert.Equal(t, "gcp", spec.CloudType)
		assert.Empty(t, spec.Fields)
	})
}

func TestCredentialField_Properties(t *testing.T) {
	t.Run("field with all properties", func(t *testing.T) {
		field := CredentialField{
			Name:        "access_key",
			Title:       "AWS Access Key",
			Description: "Enter your AWS access key ID",
			Required:    true,
			Secret:      false,
			Default:     "AKIA...",
		}

		assert.Equal(t, "access_key", field.Name)
		assert.Equal(t, "AWS Access Key", field.Title)
		assert.Equal(t, "Enter your AWS access key ID", field.Description)
		assert.True(t, field.Required)
		assert.False(t, field.Secret)
		assert.Equal(t, "AKIA...", field.Default)
	})

	t.Run("secret field properties", func(t *testing.T) {
		field := CredentialField{
			Name:     "secret_key",
			Title:    "Secret Key",
			Required: true,
			Secret:   true,
		}

		assert.True(t, field.Secret)
		assert.Empty(t, field.Default) // Secret fields typically don't have defaults.
	})
}
