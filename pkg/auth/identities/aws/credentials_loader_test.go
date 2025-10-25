package aws

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/ini.v1"
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
				section.Key("x_atmos_expiration").SetValue("2025-10-24T23:42:49Z")
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
				section.Key("x_atmos_expiration").SetValue("not-a-valid-date")
				section.Key("aws_access_key_id").SetValue("AKIATEST")
				section.Key("aws_secret_access_key").SetValue("test-secret")
				return cfg.SaveTo(path)
			},
			profile:        "test-profile",
			expectedResult: "",
		},
		{
			name: "wrong key name",
			setupFile: func(path string) error {
				cfg := ini.Empty()
				section, _ := cfg.NewSection("test-profile")
				section.Key("x_other_expiration").SetValue("2025-10-24T23:42:49Z")
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
				section.Key("x_atmos_expiration").SetValue("2025-10-24T23:42:49Z")
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

	// Add metadata (same as WriteCredentials does).
	section.Key("x_atmos_expiration").SetValue(expiration)

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
