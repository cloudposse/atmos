package aws

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
