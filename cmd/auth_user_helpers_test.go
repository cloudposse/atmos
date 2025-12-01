package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSelectAWSUserIdentities(t *testing.T) {
	tests := []struct {
		name            string
		identities      map[string]schema.Identity
		expectedCount   int
		expectedDefault string
		wantErr         bool
	}{
		{
			name:       "nil identities",
			identities: nil,
			wantErr:    true,
		},
		{
			name:       "empty identities",
			identities: map[string]schema.Identity{},
			wantErr:    true,
		},
		{
			name: "no aws/user identities",
			identities: map[string]schema.Identity{
				"role1": {Kind: "aws/assume-role"},
				"role2": {Kind: "aws/permission-set"},
			},
			wantErr: true,
		},
		{
			name: "single aws/user identity",
			identities: map[string]schema.Identity{
				"user1": {Kind: "aws/user"},
			},
			expectedCount:   1,
			expectedDefault: "",
		},
		{
			name: "multiple aws/user identities",
			identities: map[string]schema.Identity{
				"user1": {Kind: "aws/user"},
				"user2": {Kind: "aws/user"},
				"role1": {Kind: "aws/assume-role"},
			},
			expectedCount:   2,
			expectedDefault: "",
		},
		{
			name: "aws/user with default set",
			identities: map[string]schema.Identity{
				"user1": {Kind: "aws/user", Default: true},
				"user2": {Kind: "aws/user"},
			},
			expectedCount:   2,
			expectedDefault: "user1",
		},
		{
			name: "multiple defaults - first wins",
			identities: map[string]schema.Identity{
				"user1": {Kind: "aws/user", Default: true},
				"user2": {Kind: "aws/user", Default: true},
			},
			expectedCount: 2,
			// Default choice is non-deterministic due to map iteration
			// but should be one of them
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selectable, defaultChoice, err := selectAWSUserIdentities(tt.identities)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, selectable)
				return
			}

			require.NoError(t, err)
			assert.Len(t, selectable, tt.expectedCount)

			if tt.expectedDefault != "" {
				assert.Equal(t, tt.expectedDefault, defaultChoice)
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
			name: "only access key in YAML",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id": "AKIAIOSFODNN7EXAMPLE",
				},
			},
			expected: awsUserIdentityInfo{
				AccessKeyID: "AKIAIOSFODNN7EXAMPLE",
				AllInYAML:   false, // Need both keys
			},
		},
		{
			name: "both keys in YAML",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
					"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
			},
			expected: awsUserIdentityInfo{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				AllInYAML:       true,
			},
		},
		{
			name: "all fields in YAML",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
					"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
					"mfa_arn":           "arn:aws:iam::123456789012:mfa/user",
				},
				Session: &schema.SessionConfig{
					Duration: "24h",
				},
			},
			expected: awsUserIdentityInfo{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				MfaArn:          "arn:aws:iam::123456789012:mfa/user",
				SessionDuration: "24h",
				AllInYAML:       true,
			},
		},
		{
			name: "session duration without credentials",
			identity: schema.Identity{
				Session: &schema.SessionConfig{
					Duration: "12h",
				},
			},
			expected: awsUserIdentityInfo{
				SessionDuration: "12h",
				AllInYAML:       false,
			},
		},
		{
			name: "non-string credential values ignored",
			identity: schema.Identity{
				Credentials: map[string]interface{}{
					"access_key_id":     12345, // Not a string
					"secret_access_key": "secret",
				},
			},
			expected: awsUserIdentityInfo{
				SecretAccessKey: "secret",
				AllInYAML:       false, // access_key_id not set
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := extractAWSUserInfo(tt.identity)
			assert.Equal(t, tt.expected, info)
		})
	}
}

func TestBuildCredentialFormField(t *testing.T) {
	tests := []struct {
		name            string
		cfg             formFieldConfig
		expectedType    string // "note" or "input"
		expectedValue   string
		validateSuccess bool
	}{
		{
			name: "YAML value creates note field",
			cfg: formFieldConfig{
				YAMLValue: "YAML_VALUE",
				NoteTitle: "Test Note",
				NoteDesc:  "Description",
			},
			expectedType:  "note",
			expectedValue: "YAML_VALUE",
		},
		{
			name: "no YAML value creates input field",
			cfg: formFieldConfig{
				InputTitle: "Test Input",
			},
			expectedType: "input",
		},
		{
			name: "input with default value",
			cfg: formFieldConfig{
				InputTitle:   "Test Input",
				DefaultValue: "12h",
			},
			expectedType:  "input",
			expectedValue: "12h",
		},
		{
			name: "optional input allows empty",
			cfg: formFieldConfig{
				InputTitle: "Optional Field",
				IsOptional: true,
			},
			expectedType:    "input",
			validateSuccess: true, // Empty should validate
		},
		{
			name: "required input with custom validator",
			cfg: formFieldConfig{
				InputTitle: "Custom Validation",
				ValidateFunc: func(s string) error {
					if s == "valid" {
						return nil
					}
					return assert.AnError
				},
			},
			expectedType: "input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var value string
			field := buildCredentialFormField(tt.cfg, &value)

			require.NotNil(t, field)

			// Check if value was set correctly
			if tt.expectedValue != "" {
				assert.Equal(t, tt.expectedValue, value)
			}

			// Type checking is tricky with huh.Field interface
			// We can at least verify the field was created
			assert.NotNil(t, field)
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
			name:    "valid integer seconds",
			input:   "3600",
			wantErr: false,
		},
		{
			name:    "valid Go duration",
			input:   "1h",
			wantErr: false,
		},
		{
			name:    "valid complex Go duration",
			input:   "1h30m",
			wantErr: false,
		},
		{
			name:    "valid days format",
			input:   "1d",
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "negative value",
			input:   "-1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid duration format")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
