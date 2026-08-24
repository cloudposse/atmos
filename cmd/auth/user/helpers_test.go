package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSelectAWSUserIdentities(t *testing.T) {
	tests := []struct {
		name           string
		identities     map[string]schema.Identity
		expectedLen    int
		expectedError  error
		expectedChoice string
	}{
		{
			name:          "nil identities returns error",
			identities:    nil,
			expectedLen:   0,
			expectedError: errUtils.ErrInvalidAuthConfig,
		},
		{
			name:          "empty identities returns error",
			identities:    map[string]schema.Identity{},
			expectedLen:   0,
			expectedError: errUtils.ErrInvalidAuthConfig,
		},
		{
			name: "no aws/user identities returns error",
			identities: map[string]schema.Identity{
				"prod": {Kind: "aws/permission-set"},
				"dev":  {Kind: "aws/iam-identity-center"},
			},
			expectedLen:   0,
			expectedError: errUtils.ErrInvalidAuthConfig,
		},
		{
			name: "single aws/user identity",
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user"},
			},
			expectedLen:   1,
			expectedError: nil,
		},
		{
			name: "multiple aws/user identities",
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user"},
				"dev":   {Kind: "aws/user"},
			},
			expectedLen:   2,
			expectedError: nil,
		},
		{
			name: "mixed identity types",
			identities: map[string]schema.Identity{
				"admin":   {Kind: "aws/user"},
				"prod":    {Kind: "aws/permission-set"},
				"dev":     {Kind: "aws/user"},
				"staging": {Kind: "aws/iam-identity-center"},
			},
			expectedLen:   2,
			expectedError: nil,
		},
		{
			name: "default identity is selected",
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user", Default: true},
				"dev":   {Kind: "aws/user"},
			},
			expectedLen:    2,
			expectedError:  nil,
			expectedChoice: "admin",
		},
		{
			name: "first default wins",
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user", Default: true},
				"dev":   {Kind: "aws/user", Default: true},
			},
			expectedLen:   2,
			expectedError: nil,
			// Note: Map iteration order is not guaranteed, so we just check one is selected.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selectable, defaultChoice, err := selectAWSUserIdentities(tt.identities)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Empty(t, selectable)
			} else {
				assert.NoError(t, err)
				assert.Len(t, selectable, tt.expectedLen)

				if tt.expectedChoice != "" {
					assert.Equal(t, tt.expectedChoice, defaultChoice)
				}
			}
		})
	}
}

func TestExtractAWSUserInfo(t *testing.T) {
	tests := []struct {
		name     string
		identity schema.Identity
		expected awsUserIdentityInfo
	}{
		{
			name:     "empty identity",
			identity: schema.Identity{},
			expected: awsUserIdentityInfo{
				AllInYAML: false,
			},
		},
		{
			name: "all credentials in yaml",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
					"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
					"mfa_arn":           "arn:aws:iam::123456789012:mfa/user",
				},
			},
			expected: awsUserIdentityInfo{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				MfaArn:          "arn:aws:iam::123456789012:mfa/user",
				AllInYAML:       true,
			},
		},
		{
			name: "partial credentials - only access key",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id": "AKIAIOSFODNN7EXAMPLE",
				},
			},
			expected: awsUserIdentityInfo{
				AccessKeyID: "AKIAIOSFODNN7EXAMPLE",
				AllInYAML:   false,
			},
		},
		{
			name: "partial credentials - only secret key",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
			},
			expected: awsUserIdentityInfo{
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				AllInYAML:       false,
			},
		},
		{
			name: "with session duration",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
					"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
				Session: &schema.SessionConfig{
					Duration: "12h",
				},
			},
			expected: awsUserIdentityInfo{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionDuration: "12h",
				AllInYAML:       true,
			},
		},
		{
			name: "with mfa arn only",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"mfa_arn": "arn:aws:iam::123456789012:mfa/user",
				},
			},
			expected: awsUserIdentityInfo{
				MfaArn:    "arn:aws:iam::123456789012:mfa/user",
				AllInYAML: false,
			},
		},
		{
			name: "nil credentials map",
			identity: schema.Identity{
				Credentials: nil,
			},
			expected: awsUserIdentityInfo{
				AllInYAML: false,
			},
		},
		{
			name: "wrong type values are ignored",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id":     123, // wrong type
					"secret_access_key": "valid_secret",
				},
			},
			expected: awsUserIdentityInfo{
				SecretAccessKey: "valid_secret",
				AllInYAML:       false, // access_key_id is empty
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAWSUserInfo(tt.identity)
			assert.Equal(t, tt.expected.AccessKeyID, result.AccessKeyID)
			assert.Equal(t, tt.expected.SecretAccessKey, result.SecretAccessKey)
			assert.Equal(t, tt.expected.MfaArn, result.MfaArn)
			assert.Equal(t, tt.expected.SessionDuration, result.SessionDuration)
			assert.Equal(t, tt.expected.AllInYAML, result.AllInYAML)
		})
	}
}

func TestValidateSessionDuration(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "empty is valid (optional field)",
			input:       "",
			expectError: false,
		},
		{
			name:        "valid hours",
			input:       "12h",
			expectError: false,
		},
		{
			name:        "valid seconds as number",
			input:       "3600",
			expectError: false,
		},
		{
			name:        "valid days",
			input:       "1d",
			expectError: false,
		},
		{
			name:        "valid minutes",
			input:       "30m",
			expectError: false,
		},
		{
			name:        "valid complex duration",
			input:       "1h30m",
			expectError: false,
		},
		{
			name:        "invalid format - text",
			input:       "abc",
			expectError: true,
		},
		{
			name:        "invalid format - unknown unit",
			input:       "12x",
			expectError: true,
		},
		{
			name:        "invalid format - negative",
			input:       "-1h",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionDuration(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFormFieldConfig(t *testing.T) {
	// Test that formFieldConfig struct is properly defined.
	cfg := formFieldConfig{
		YAMLValue:      "test-value",
		InputTitle:     "Test Input",
		NoteTitle:      "Test Note",
		NoteDesc:       "Test Description",
		IsPassword:     true,
		IsOptional:     false,
		DefaultValue:   "default",
		ValidateFunc:   nil,
		DescriptionMsg: "Test description message",
	}

	assert.Equal(t, "test-value", cfg.YAMLValue)
	assert.Equal(t, "Test Input", cfg.InputTitle)
	assert.Equal(t, "Test Note", cfg.NoteTitle)
	assert.Equal(t, "Test Description", cfg.NoteDesc)
	assert.True(t, cfg.IsPassword)
	assert.False(t, cfg.IsOptional)
	assert.Equal(t, "default", cfg.DefaultValue)
	assert.Nil(t, cfg.ValidateFunc)
	assert.Equal(t, "Test description message", cfg.DescriptionMsg)
}

func TestAwsUserIdentityInfo(t *testing.T) {
	// Test that awsUserIdentityInfo struct is properly defined.
	info := awsUserIdentityInfo{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		MfaArn:          "arn:aws:iam::123456789012:mfa/user",
		SessionDuration: "12h",
		AllInYAML:       true,
	}

	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", info.AccessKeyID)
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", info.SecretAccessKey)
	assert.Equal(t, "arn:aws:iam::123456789012:mfa/user", info.MfaArn)
	assert.Equal(t, "12h", info.SessionDuration)
	assert.True(t, info.AllInYAML)
}

// TestBuildCredentialFormField covers all branches of the form-builder helper:
// YAML-managed → Note field, default value pre-fill, password mode, optional,
// custom validate func, and the default required-field validator.
func TestBuildCredentialFormField(t *testing.T) {
	t.Run("YAML-managed value renders a Note and pre-fills value", func(t *testing.T) {
		var captured string
		field := buildCredentialFormField(formFieldConfig{
			YAMLValue:  "AKIA-fixed-in-yaml",
			InputTitle: "AWS Access Key ID",
			NoteTitle:  "AWS Access Key ID (managed by Atmos configuration)",
			NoteDesc:   "AKIA-fixed-in-yaml",
		}, &captured)

		require.NotNil(t, field)
		// The pointer must have been pre-filled with the YAML value so the
		// caller doesn't accidentally overwrite it with a blank prompt result.
		assert.Equal(t, "AKIA-fixed-in-yaml", captured)
	})

	t.Run("missing YAML value with DefaultValue pre-fills the input", func(t *testing.T) {
		var captured string
		field := buildCredentialFormField(formFieldConfig{
			InputTitle:   "Session Duration",
			DefaultValue: "12h",
			IsOptional:   true,
		}, &captured)

		require.NotNil(t, field)
		assert.Equal(t, "12h", captured,
			"DefaultValue must populate the input when no YAML value is set")
	})

	t.Run("password mode does not panic and returns a field", func(t *testing.T) {
		var captured string
		field := buildCredentialFormField(formFieldConfig{
			InputTitle: "AWS Secret Access Key",
			IsPassword: true,
		}, &captured)
		require.NotNil(t, field)
	})

	t.Run("optional field with no default leaves capture empty", func(t *testing.T) {
		var captured string
		field := buildCredentialFormField(formFieldConfig{
			InputTitle: "MFA ARN (optional)",
			IsOptional: true,
		}, &captured)
		require.NotNil(t, field)
		assert.Empty(t, captured)
	})

	t.Run("custom ValidateFunc is wired in without invoking it", func(t *testing.T) {
		var captured string
		field := buildCredentialFormField(formFieldConfig{
			InputTitle:   "Session Duration",
			IsOptional:   true,
			ValidateFunc: func(s string) error { return nil },
		}, &captured)
		require.NotNil(t, field)
	})

	t.Run("description message is wired in", func(t *testing.T) {
		var captured string
		field := buildCredentialFormField(formFieldConfig{
			InputTitle:     "Session Duration",
			DescriptionMsg: "How long before MFA expires",
			IsOptional:     true,
		}, &captured)
		require.NotNil(t, field)
	})
}
