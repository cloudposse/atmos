package aws_utils

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestLoadAWSConfig(t *testing.T) {
	// Check for AWS profile precondition
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")
	tests := []struct {
		name       string
		region     string
		roleArn    string
		setupEnv   func()
		cleanupEnv func()
		wantErr    bool
	}{
		{
			name:    "basic config without region or role",
			region:  "",
			roleArn: "",
			setupEnv: func() {
				t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
			},
			cleanupEnv: func() {
				os.Unsetenv("AWS_ACCESS_KEY_ID")
				os.Unsetenv("AWS_SECRET_ACCESS_KEY")
			},
			wantErr: false,
		},
		{
			name:    "config with custom region",
			region:  "us-east-2",
			roleArn: "",
			setupEnv: func() {
				t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
			},
			cleanupEnv: func() {
				os.Unsetenv("AWS_ACCESS_KEY_ID")
				os.Unsetenv("AWS_SECRET_ACCESS_KEY")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear AWS_PROFILE to prevent conflicts with local AWS configuration.
			t.Setenv("AWS_PROFILE", "")

			// Setup
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Cleanup
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			// Execute
			cfg, err := LoadAWSConfig(context.Background(), tt.region, tt.roleArn, time.Minute*15)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.region != "" {
					assert.Equal(t, tt.region, cfg.Region)
				}
			}
		})
	}
}

func TestLoadAWSConfigWithAuth(t *testing.T) {
	tests := []struct {
		name        string
		region      string
		authContext *schema.AWSAuthContext
		scenario    string // Test scenario for setup logic: "mismatched-profile", "explicit-files", or ""
		wantRegion  string
		wantErr     bool
	}{
		{
			name:        "without auth context",
			region:      "us-east-1",
			authContext: nil,
			wantRegion:  "us-east-1",
			wantErr:     false,
		},
		{
			name:   "with auth context and explicit region",
			region: "us-west-2",
			authContext: &schema.AWSAuthContext{
				Profile: "test-profile",
				Region:  "eu-west-1",
			},
			wantRegion: "us-west-2", // Explicit region takes precedence.
			wantErr:    false,
		},
		{
			name:   "with auth context using context region",
			region: "",
			authContext: &schema.AWSAuthContext{
				Profile: "test-profile",
				Region:  "ap-southeast-1",
			},
			wantRegion: "ap-southeast-1", // Uses auth context region.
			wantErr:    false,
		},
		{
			name:   "with auth context without region",
			region: "",
			authContext: &schema.AWSAuthContext{
				Profile: "test-profile",
				Region:  "",
			},
			wantRegion: "", // No region specified.
			wantErr:    false,
		},
		{
			name:   "non-existent credentials file",
			region: "us-east-1",
			authContext: &schema.AWSAuthContext{
				Profile:         "test-profile",
				Region:          "us-east-1",
				CredentialsFile: "/non/existent/credentials",
				ConfigFile:      "/non/existent/config",
			},
			wantRegion: "",
			wantErr:    true,
		},
		{
			name:   "invalid profile name in auth context",
			region: "us-east-1",
			authContext: &schema.AWSAuthContext{
				Profile: "nonexistent-profile",
				Region:  "us-east-1",
			},
			scenario:   "mismatched-profile",
			wantRegion: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear AWS environment variables to avoid conflicts.
			t.Setenv("AWS_PROFILE", "")
			t.Setenv("AWS_REGION", "")
			t.Setenv("AWS_DEFAULT_REGION", "")
			t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

			// Create a local copy of authContext to avoid mutating test table.
			var authContextCopy *schema.AWSAuthContext
			if tt.authContext != nil {
				// Copy the struct to avoid race conditions.
				authContextCopy = &schema.AWSAuthContext{
					Profile: tt.authContext.Profile,
					Region:  tt.authContext.Region,
				}

				// Handle different test scenarios.
				switch {
				case tt.authContext.CredentialsFile != "":
					// For error test cases with explicit file paths, use them.
					authContextCopy.CredentialsFile = tt.authContext.CredentialsFile
					authContextCopy.ConfigFile = tt.authContext.ConfigFile
				case tt.scenario == "mismatched-profile":
					// Create valid files but with a different profile name.
					tempDir := t.TempDir()
					credFile := filepath.Join(tempDir, "credentials")
					configFile := filepath.Join(tempDir, "config")

					// Write credential file with different profile.
					credContent := "[different-profile]\n"
					credContent += "aws_access_key_id = test-key\n"
					credContent += "aws_secret_access_key = test-secret\n"
					require.NoError(t, os.WriteFile(credFile, []byte(credContent), 0o600))

					// Write config file with different profile.
					cfgContent := "[profile different-profile]\n"
					cfgContent += "region = us-east-1\n"
					require.NoError(t, os.WriteFile(configFile, []byte(cfgContent), 0o600))

					authContextCopy.CredentialsFile = credFile
					authContextCopy.ConfigFile = configFile
				case !tt.wantErr:
					// Create valid credentials for happy-path tests.
					tempDir := t.TempDir()
					credFile := filepath.Join(tempDir, "credentials")
					configFile := filepath.Join(tempDir, "config")

					// Write minimal credential file.
					credContent := "[" + authContextCopy.Profile + "]\n"
					credContent += "aws_access_key_id = test-key\n"
					credContent += "aws_secret_access_key = test-secret\n"
					require.NoError(t, os.WriteFile(credFile, []byte(credContent), 0o600))

					// Write minimal config file.
					cfgContent := "[profile " + authContextCopy.Profile + "]\n"
					if authContextCopy.Region != "" {
						cfgContent += "region = " + authContextCopy.Region + "\n"
					}
					require.NoError(t, os.WriteFile(configFile, []byte(cfgContent), 0o600))

					// Set file paths on the copy.
					authContextCopy.CredentialsFile = credFile
					authContextCopy.ConfigFile = configFile
				}
			}

			// Execute.
			cfg, err := LoadAWSConfigWithAuth(
				context.Background(),
				tt.region,
				"", // No role ARN for these tests.
				time.Minute*15,
				authContextCopy,
			)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRegion, cfg.Region)
			}
		})
	}
}

func TestLoadAWSConfig_BackwardCompatibility(t *testing.T) {
	// Test that LoadAWSConfig is equivalent to LoadAWSConfigWithAuth(nil)
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

	region := "us-east-1"

	cfg1, err1 := LoadAWSConfig(context.Background(), region, "", time.Minute*15)
	cfg2, err2 := LoadAWSConfigWithAuth(context.Background(), region, "", time.Minute*15, nil)

	assert.Equal(t, err1 == nil, err2 == nil, "Both functions should have same error state")
	if err1 == nil && err2 == nil {
		assert.Equal(t, cfg1.Region, cfg2.Region, "Both functions should return same region")
	}
}
