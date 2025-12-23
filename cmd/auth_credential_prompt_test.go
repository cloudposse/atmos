package cmd

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestValidateSessionDurationFormat(t *testing.T) {
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
			err := validateSessionDurationFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidDuration)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBuildCredentialFormFields(t *testing.T) {
	t.Run("without MFA ARN creates 4 fields", func(t *testing.T) {
		var accessKeyID, secretAccessKey, mfaArnInput, sessionDuration string
		fields := buildCredentialFormFields(&accessKeyID, &secretAccessKey, &mfaArnInput, &sessionDuration, "")

		// Should have 4 fields: Access Key, Secret Key, MFA ARN input, Session Duration.
		assert.Len(t, fields, 4)
	})

	t.Run("with MFA ARN creates 4 fields with note", func(t *testing.T) {
		var accessKeyID, secretAccessKey, mfaArnInput, sessionDuration string
		mfaArn := "arn:aws:iam::123456789012:mfa/user"
		fields := buildCredentialFormFields(&accessKeyID, &secretAccessKey, &mfaArnInput, &sessionDuration, mfaArn)

		// Should have 4 fields: Access Key, Secret Key, MFA ARN note, Session Duration.
		assert.Len(t, fields, 4)
	})
}
