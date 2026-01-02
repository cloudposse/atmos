package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

func TestRedactHomeDir(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		homeDir  string
		expected string
	}{
		{
			name:     "path starting with home",
			value:    "/home/user/.aws/credentials",
			homeDir:  "/home/user",
			expected: "~/.aws/credentials",
		},
		{
			name:     "exact home match",
			value:    "/home/user",
			homeDir:  "/home/user",
			expected: "~",
		},
		{
			name:     "no home prefix",
			value:    "/etc/config",
			homeDir:  "/home/user",
			expected: "/etc/config",
		},
		{
			name:     "empty home",
			value:    "/home/user/file",
			homeDir:  "",
			expected: "/home/user/file",
		},
		{
			name:     "partial match not replaced",
			value:    "/home/username/.config",
			homeDir:  "/home/user",
			expected: "/home/username/.config",
		},
		{
			name:     "similar prefix not replaced",
			value:    "/home/user2/.config",
			homeDir:  "/home/user",
			expected: "/home/user2/.config",
		},
		{
			name:     "nested path",
			value:    "/home/user/.config/atmos/config.yaml",
			homeDir:  "/home/user",
			expected: "~/.config/atmos/config.yaml",
		},
		{
			name:     "empty value",
			value:    "",
			homeDir:  "/home/user",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Need to handle path separator properly.
			result := redactHomeDir(tt.value, tt.homeDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeEnvMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		homeDir  string
		expected map[string]string
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			homeDir:  "/home/user",
			expected: map[string]string{},
		},
		{
			name: "redacts AWS_SECRET_ACCESS_KEY",
			input: map[string]string{
				"AWS_SECRET_ACCESS_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"AWS_SECRET_ACCESS_KEY": "***REDACTED***",
			},
		},
		{
			name: "redacts TOKEN values",
			input: map[string]string{
				"AUTH_TOKEN": "token123",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"AUTH_TOKEN": "***REDACTED***",
			},
		},
		{
			name: "redacts PASSWORD values",
			input: map[string]string{
				"DB_PASSWORD": "secret123",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"DB_PASSWORD": "***REDACTED***",
			},
		},
		{
			name: "redacts PRIVATE values",
			input: map[string]string{
				"PRIVATE_KEY": "private_key_data",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"PRIVATE_KEY": "***REDACTED***",
			},
		},
		{
			name: "redacts ACCESS_KEY values",
			input: map[string]string{
				"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"AWS_ACCESS_KEY_ID": "***REDACTED***",
			},
		},
		{
			name: "redacts SESSION values",
			input: map[string]string{
				"AWS_SESSION_TOKEN": "session_token_data",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"AWS_SESSION_TOKEN": "***REDACTED***",
			},
		},
		{
			name: "preserves non-sensitive values",
			input: map[string]string{
				"AWS_REGION": "us-east-1",
				"PATH":       "/usr/bin:/bin",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"AWS_REGION": "us-east-1",
				"PATH":       "/usr/bin:/bin",
			},
		},
		{
			name: "case insensitive matching",
			input: map[string]string{
				"my_secret": "should_be_redacted",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"my_secret": "***REDACTED***",
			},
		},
		{
			name: "mixed sensitive and non-sensitive",
			input: map[string]string{
				"AWS_REGION":            "us-east-1",
				"AWS_SECRET_ACCESS_KEY": "secret",
				"HOME":                  "/home/user",
			},
			homeDir: "/home/user",
			expected: map[string]string{
				"AWS_REGION":            "us-east-1",
				"AWS_SECRET_ACCESS_KEY": "***REDACTED***",
				"HOME":                  "~",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeEnvMap(tt.input, tt.homeDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildWhoamiTableRows(t *testing.T) {
	tests := []struct {
		name        string
		whoami      *authTypes.WhoamiInfo
		expectedMin int // minimum number of rows
		checkFields []string
	}{
		{
			name: "minimal info",
			whoami: &authTypes.WhoamiInfo{
				Provider:    "aws-sso",
				Identity:    "prod-admin",
				LastUpdated: time.Now(),
			},
			expectedMin: 3, // Provider, Identity, Last Updated
			checkFields: []string{"Provider", "Identity", "Last Updated"},
		},
		{
			name: "with principal",
			whoami: &authTypes.WhoamiInfo{
				Provider:    "aws-sso",
				Identity:    "prod-admin",
				Principal:   "arn:aws:sts::123456789012:assumed-role/Admin/user",
				LastUpdated: time.Now(),
			},
			expectedMin: 4,
			checkFields: []string{"Provider", "Identity", "Principal", "Last Updated"},
		},
		{
			name: "with account",
			whoami: &authTypes.WhoamiInfo{
				Provider:    "aws-sso",
				Identity:    "prod-admin",
				Account:     "123456789012",
				LastUpdated: time.Now(),
			},
			expectedMin: 4,
			checkFields: []string{"Provider", "Identity", "Account", "Last Updated"},
		},
		{
			name: "with region",
			whoami: &authTypes.WhoamiInfo{
				Provider:    "aws-sso",
				Identity:    "prod-admin",
				Region:      "us-east-1",
				LastUpdated: time.Now(),
			},
			expectedMin: 4,
			checkFields: []string{"Provider", "Identity", "Region", "Last Updated"},
		},
		{
			name: "with expiration",
			whoami: &authTypes.WhoamiInfo{
				Provider:    "aws-sso",
				Identity:    "prod-admin",
				Expiration:  func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
				LastUpdated: time.Now(),
			},
			expectedMin: 4,
			checkFields: []string{"Provider", "Identity", "Expires", "Last Updated"},
		},
		{
			name: "full info",
			whoami: &authTypes.WhoamiInfo{
				Provider:    "aws-sso",
				Identity:    "prod-admin",
				Principal:   "arn:aws:sts::123456789012:assumed-role/Admin/user",
				Account:     "123456789012",
				Region:      "us-east-1",
				Expiration:  func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
				LastUpdated: time.Now(),
			},
			expectedMin: 7,
			checkFields: []string{"Provider", "Identity", "Principal", "Account", "Region", "Expires", "Last Updated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := buildWhoamiTableRows(tt.whoami)

			assert.GreaterOrEqual(t, len(rows), tt.expectedMin)

			// Check that expected fields are present.
			rowLabels := make([]string, len(rows))
			for i, row := range rows {
				if len(row) > 0 {
					rowLabels[i] = row[0]
				}
			}

			for _, field := range tt.checkFields {
				assert.Contains(t, rowLabels, field)
			}
		})
	}
}

func TestFormatExpiration(t *testing.T) {
	tests := []struct {
		name           string
		expiration     time.Time
		threshold      int
		expectRed      bool
		expectContains string
	}{
		{
			name:           "future expiration - normal",
			expiration:     time.Now().Add(2 * time.Hour),
			threshold:      15,
			expectRed:      false,
			expectContains: "1h",
		},
		{
			name:           "expiring soon - warning",
			expiration:     time.Now().Add(10 * time.Minute),
			threshold:      15,
			expectRed:      true,
			expectContains: "m",
		},
		{
			name:           "already expired",
			expiration:     time.Now().Add(-1 * time.Hour),
			threshold:      15,
			expectRed:      false, // expired shows differently
			expectContains: "expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatExpiration(&tt.expiration, tt.threshold)

			// Verify the result contains expected content.
			assert.Contains(t, result, tt.expectContains)

			// Verify date format is included.
			assert.Contains(t, result, "2")
		})
	}
}

func TestValidateCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		setupWhoami    func(*gomock.Controller) *authTypes.WhoamiInfo
		expectedResult bool
	}{
		{
			name: "nil credentials returns false",
			setupWhoami: func(_ *gomock.Controller) *authTypes.WhoamiInfo {
				return &authTypes.WhoamiInfo{
					Identity:    "test",
					Credentials: nil,
				}
			},
			expectedResult: false,
		},
		{
			name: "credentials with validator - success",
			setupWhoami: func(c *gomock.Controller) *authTypes.WhoamiInfo {
				mockCreds := authTypes.NewMockICredentials(c)
				// MockICredentials implements Validate, so it will be used.
				mockCreds.EXPECT().Validate(gomock.Any()).Return(&authTypes.ValidationInfo{
					Principal: "arn:aws:iam::123456789012:user/test",
				}, nil)
				return &authTypes.WhoamiInfo{
					Identity:    "test",
					Credentials: mockCreds,
				}
			},
			expectedResult: true,
		},
		{
			name: "credentials with validator - error",
			setupWhoami: func(c *gomock.Controller) *authTypes.WhoamiInfo {
				mockCreds := authTypes.NewMockICredentials(c)
				mockCreds.EXPECT().Validate(gomock.Any()).Return(nil, errUtils.ErrAuthenticationFailed)
				return &authTypes.WhoamiInfo{
					Identity:    "test",
					Credentials: mockCreds,
				}
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new controller for each test case.
			c := gomock.NewController(t)
			defer c.Finish()
			whoami := tt.setupWhoami(c)
			ctx := context.Background()
			result := validateCredentials(ctx, whoami)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestPopulateWhoamiFromValidation(t *testing.T) {
	tests := []struct {
		name              string
		whoami            *authTypes.WhoamiInfo
		validationInfo    *authTypes.ValidationInfo
		expectedPrincipal string
		expectedAccount   string
		hasExpiration     bool
	}{
		{
			name:              "nil validation info",
			whoami:            &authTypes.WhoamiInfo{},
			validationInfo:    nil,
			expectedPrincipal: "",
			expectedAccount:   "",
			hasExpiration:     false,
		},
		{
			name:              "empty validation info",
			whoami:            &authTypes.WhoamiInfo{},
			validationInfo:    &authTypes.ValidationInfo{},
			expectedPrincipal: "",
			expectedAccount:   "",
			hasExpiration:     false,
		},
		{
			name:   "full validation info",
			whoami: &authTypes.WhoamiInfo{},
			validationInfo: &authTypes.ValidationInfo{
				Principal:  "arn:aws:sts::123456789012:assumed-role/Admin",
				Account:    "123456789012",
				Expiration: func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
			},
			expectedPrincipal: "arn:aws:sts::123456789012:assumed-role/Admin",
			expectedAccount:   "123456789012",
			hasExpiration:     true,
		},
		{
			name:   "partial validation info - only principal",
			whoami: &authTypes.WhoamiInfo{},
			validationInfo: &authTypes.ValidationInfo{
				Principal: "test-principal",
			},
			expectedPrincipal: "test-principal",
			expectedAccount:   "",
			hasExpiration:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			populateWhoamiFromValidation(tt.whoami, tt.validationInfo)

			assert.Equal(t, tt.expectedPrincipal, tt.whoami.Principal)
			assert.Equal(t, tt.expectedAccount, tt.whoami.Account)
			if tt.hasExpiration {
				assert.NotNil(t, tt.whoami.Expiration)
			}
		})
	}
}

func TestCreateWhoamiTable(t *testing.T) {
	rows := [][]string{
		{"Provider", "aws-sso"},
		{"Identity", "prod-admin"},
	}

	table := createWhoamiTable(rows)
	assert.NotNil(t, table)
}

func TestAuthWhoamiCommand_Structure(t *testing.T) {
	assert.Equal(t, "whoami", authWhoamiCmd.Use)
	assert.NotEmpty(t, authWhoamiCmd.Short)
	assert.NotEmpty(t, authWhoamiCmd.Long)
	assert.NotNil(t, authWhoamiCmd.RunE)

	// Check output flag exists.
	outputFlag := authWhoamiCmd.Flags().Lookup("output")
	assert.NotNil(t, outputFlag)
	assert.Equal(t, "o", outputFlag.Shorthand)
}

func TestRedactHomeDirWithOsPathSeparator(t *testing.T) {
	// Test with actual OS path separator.
	homeDir := "/home/user"
	testPath := filepath.Join(homeDir, ".config", "atmos")

	result := redactHomeDir(testPath, homeDir)

	// Result should start with ~.
	assert.True(t, len(result) > 0 && result[0] == '~')
}
