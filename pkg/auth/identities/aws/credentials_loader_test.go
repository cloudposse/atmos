package aws

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestReadExpirationFromMetadata(t *testing.T) {
	tests := []struct {
		name           string
		setupFile      func(path string) error
		profile        string
		expectedResult string
	}{
		{
			name: "valid expiration in metadata",
			setupFile: func(path string) error {
				cfg := ini.Empty()
				section, _ := cfg.NewSection("test-profile")
				section.Comment = "atmos: expiration=2025-10-24T23:42:49Z"
				section.Key("aws_access_key_id").SetValue("AKIATEST")
				section.Key("aws_secret_access_key").SetValue("test-secret")
				return cfg.SaveTo(path)
			},
			profile:        "test-profile",
			expectedResult: "2025-10-24T23:42:49Z",
		},
		{
			name: "no metadata comment",
			setupFile: func(path string) error {
				cfg := ini.Empty()
				section, _ := cfg.NewSection("test-profile")
				section.Key("aws_access_key_id").SetValue("AKIATEST")
				section.Key("aws_secret_access_key").SetValue("test-secret")
				return cfg.SaveTo(path)
			},
			profile:        "test-profile",
			expectedResult: "",
		},
		{
			name: "invalid expiration format",
			setupFile: func(path string) error {
				cfg := ini.Empty()
				section, _ := cfg.NewSection("test-profile")
				section.Comment = "atmos: expiration=not-a-valid-date"
				section.Key("aws_access_key_id").SetValue("AKIATEST")
				section.Key("aws_secret_access_key").SetValue("test-secret")
				return cfg.SaveTo(path)
			},
			profile:        "test-profile",
			expectedResult: "",
		},
		{
			name: "wrong comment prefix",
			setupFile: func(path string) error {
				cfg := ini.Empty()
				section, _ := cfg.NewSection("test-profile")
				section.Comment = "other: expiration=2025-10-24T23:42:49Z"
				section.Key("aws_access_key_id").SetValue("AKIATEST")
				section.Key("aws_secret_access_key").SetValue("test-secret")
				return cfg.SaveTo(path)
			},
			profile:        "test-profile",
			expectedResult: "",
		},
		{
			name: "profile not found",
			setupFile: func(path string) error {
				cfg := ini.Empty()
				section, _ := cfg.NewSection("different-profile")
				section.Comment = "atmos: expiration=2025-10-24T23:42:49Z"
				section.Key("aws_access_key_id").SetValue("AKIATEST")
				return cfg.SaveTo(path)
			},
			profile:        "test-profile",
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file.
			tmpDir := t.TempDir()
			credentialsPath := filepath.Join(tmpDir, "credentials")

			// Setup file.
			err := tt.setupFile(credentialsPath)
			assert.NoError(t, err)

			// Test.
			result := readExpirationFromMetadata(credentialsPath, tt.profile)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestReadExpirationFromMetadata_FileNotFound(t *testing.T) {
	result := readExpirationFromMetadata("/nonexistent/path", "test-profile")
	assert.Equal(t, "", result)
}

func TestMetadataRoundTrip(t *testing.T) {
	// This tests the full flow: write metadata -> read metadata.
	tmpDir := t.TempDir()
	credentialsPath := filepath.Join(tmpDir, "credentials")

	// Create a credentials file with metadata using the same logic as WriteCredentials.
	expiration := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

	cfg := ini.Empty()
	section, err := cfg.NewSection("test-profile")
	assert.NoError(t, err)

	// Add metadata comment (same as WriteCredentials does).
	section.Comment = "atmos: expiration=" + expiration

	// Add credentials.
	section.Key("aws_access_key_id").SetValue("AKIATEST")
	section.Key("aws_secret_access_key").SetValue("test-secret")
	section.Key("aws_session_token").SetValue("test-token")

	// Save.
	err = cfg.SaveTo(credentialsPath)
	assert.NoError(t, err)

	// Set file permissions.
	err = os.Chmod(credentialsPath, 0o600)
	assert.NoError(t, err)

	// Read back the expiration.
	result := readExpirationFromMetadata(credentialsPath, "test-profile")
	assert.Equal(t, expiration, result)

	// Verify it's a valid RFC3339 timestamp.
	parsedTime, err := time.Parse(time.RFC3339, result)
	assert.NoError(t, err)
	assert.False(t, parsedTime.IsZero())
}

func TestExtractAWSEnvVars(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		want        awsEnvVars
		wantErr     bool
		errContains string
	}{
		{
			name: "all required vars present",
			env: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/creds",
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_PROFILE":                 "default",
				"AWS_REGION":                  "us-east-1",
			},
			want: awsEnvVars{
				credsFile:  "/path/to/creds",
				configFile: "/path/to/config",
				profile:    "default",
				region:     "us-east-1",
			},
			wantErr: false,
		},
		{
			name: "region optional",
			env: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/creds",
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_PROFILE":                 "default",
			},
			want: awsEnvVars{
				credsFile:  "/path/to/creds",
				configFile: "/path/to/config",
				profile:    "default",
				region:     "",
			},
			wantErr: false,
		},
		{
			name: "missing credentials file",
			env: map[string]string{
				"AWS_CONFIG_FILE": "/path/to/config",
				"AWS_PROFILE":     "default",
			},
			wantErr:     true,
			errContains: "AWS_SHARED_CREDENTIALS_FILE",
		},
		{
			name: "missing config file",
			env: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/creds",
				"AWS_PROFILE":                 "default",
			},
			wantErr:     true,
			errContains: "AWS_CONFIG_FILE",
		},
		{
			name: "missing profile",
			env: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/creds",
				"AWS_CONFIG_FILE":             "/path/to/config",
			},
			wantErr:     true,
			errContains: "AWS_PROFILE",
		},
		{
			name:        "empty env",
			env:         map[string]string{},
			wantErr:     true,
			errContains: "AWS_SHARED_CREDENTIALS_FILE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractAWSEnvVars(tt.env)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrAwsMissingEnvVars)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetupAWSEnv(t *testing.T) {
	tests := []struct {
		name          string
		credsFile     string
		configFile    string
		profile       string
		region        string
		existingEnv   map[string]string
		expectedEnv   map[string]string
		expectedAfter map[string]string // Environment after cleanup
		expectedUnset []string          // Keys that should be unset after cleanup
	}{
		{
			name:        "sets all vars with no existing env",
			credsFile:   "/new/creds",
			configFile:  "/new/config",
			profile:     "new-profile",
			region:      "us-west-2",
			existingEnv: map[string]string{},
			expectedEnv: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/new/creds",
				"AWS_CONFIG_FILE":             "/new/config",
				"AWS_PROFILE":                 "new-profile",
				"AWS_REGION":                  "us-west-2",
			},
			expectedUnset: []string{
				"AWS_SHARED_CREDENTIALS_FILE",
				"AWS_CONFIG_FILE",
				"AWS_PROFILE",
				"AWS_REGION",
			},
		},
		{
			name:       "restores original env vars",
			credsFile:  "/new/creds",
			configFile: "/new/config",
			profile:    "new-profile",
			region:     "us-west-2",
			existingEnv: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/old/creds",
				"AWS_CONFIG_FILE":             "/old/config",
				"AWS_PROFILE":                 "old-profile",
				"AWS_REGION":                  "us-east-1",
			},
			expectedEnv: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/new/creds",
				"AWS_CONFIG_FILE":             "/new/config",
				"AWS_PROFILE":                 "new-profile",
				"AWS_REGION":                  "us-west-2",
			},
			expectedAfter: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/old/creds",
				"AWS_CONFIG_FILE":             "/old/config",
				"AWS_PROFILE":                 "old-profile",
				"AWS_REGION":                  "us-east-1",
			},
		},
		{
			name:        "sets vars without region",
			credsFile:   "/new/creds",
			configFile:  "/new/config",
			profile:     "new-profile",
			region:      "", // Empty region
			existingEnv: map[string]string{},
			expectedEnv: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/new/creds",
				"AWS_CONFIG_FILE":             "/new/config",
				"AWS_PROFILE":                 "new-profile",
			},
			expectedUnset: []string{
				"AWS_SHARED_CREDENTIALS_FILE",
				"AWS_CONFIG_FILE",
				"AWS_PROFILE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save current environment state for all relevant keys.
			savedEnv := make(map[string]string)
			keysToSave := []string{
				"AWS_SHARED_CREDENTIALS_FILE",
				"AWS_CONFIG_FILE",
				"AWS_PROFILE",
				"AWS_REGION",
			}
			for _, key := range keysToSave {
				if val, exists := os.LookupEnv(key); exists {
					savedEnv[key] = val
				}
			}

			// Clear all relevant env vars first.
			for _, key := range keysToSave {
				os.Unsetenv(key)
			}

			// Setup test environment.
			for key, value := range tt.existingEnv {
				t.Setenv(key, value)
			}

			t.Cleanup(func() {
				// Restore original environment.
				for _, key := range keysToSave {
					os.Unsetenv(key)
					if val, exists := savedEnv[key]; exists {
						os.Setenv(key, val)
					}
				}
			})

			// Call setupAWSEnv.
			cleanup := setupAWSEnv(tt.credsFile, tt.configFile, tt.profile, tt.region)

			// Verify environment is set correctly.
			for key, want := range tt.expectedEnv {
				got := os.Getenv(key)
				assert.Equal(t, want, got, "env var %s should be set to %s", key, want)
			}

			// Call cleanup.
			cleanup()

			// Verify environment is restored.
			if tt.expectedAfter != nil {
				for key, want := range tt.expectedAfter {
					got := os.Getenv(key)
					assert.Equal(t, want, got, "env var %s should be restored to %s", key, want)
				}
			}

			// Verify vars are unset if no original values.
			for _, key := range tt.expectedUnset {
				got, exists := os.LookupEnv(key)
				assert.False(t, exists, "env var %s should be unset, but has value %s", key, got)
			}
		})
	}
}

func TestPopulateExpiration(t *testing.T) {
	futureTime := time.Now().Add(1 * time.Hour)
	futureRFC3339 := futureTime.Format(time.RFC3339)

	tests := []struct {
		name               string
		awsCreds           *aws.Credentials
		creds              *types.AWSCredentials
		credsFile          string
		profile            string
		expectedExpiration string
	}{
		{
			name: "uses SDK expiration when available",
			awsCreds: &aws.Credentials{
				AccessKeyID:     "AKIA",
				SecretAccessKey: "SECRET",
				SessionToken:    "TOKEN",
				Expires:         futureTime,
			},
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIA",
				SecretAccessKey: "SECRET",
				SessionToken:    "TOKEN",
			},
			credsFile:          "/path/to/creds",
			profile:            "default",
			expectedExpiration: futureRFC3339,
		},
		{
			name: "no expiration for non-session credentials",
			awsCreds: &aws.Credentials{
				AccessKeyID:     "AKIA",
				SecretAccessKey: "SECRET",
				SessionToken:    "", // No session token
			},
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIA",
				SecretAccessKey: "SECRET",
				SessionToken:    "",
			},
			credsFile:          "/path/to/creds",
			profile:            "default",
			expectedExpiration: "", // Should remain empty
		},
		{
			name: "session token with zero expiration in SDK",
			awsCreds: &aws.Credentials{
				AccessKeyID:     "AKIA",
				SecretAccessKey: "SECRET",
				SessionToken:    "TOKEN",
				Expires:         time.Time{}, // Zero time
			},
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIA",
				SecretAccessKey: "SECRET",
				SessionToken:    "TOKEN",
			},
			credsFile: "/nonexistent/path", // Will fail to read metadata
			profile:   "default",
			// expectedExpiration remains empty since we can't read metadata file
			expectedExpiration: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			populateExpiration(tt.creds, tt.awsCreds, tt.credsFile, tt.profile)
			assert.Equal(t, tt.expectedExpiration, tt.creds.Expiration)
		})
	}
}
