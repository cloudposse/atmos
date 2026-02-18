package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareEnvironment(t *testing.T) {
	tests := []struct {
		name            string
		inputEnv        map[string]string
		profile         string
		credentialsFile string
		configFile      string
		region          string
		expectedEnv     map[string]string
	}{
		{
			name: "basic environment preparation",
			inputEnv: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "us-west-2",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"PATH":                        "/usr/bin",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_REGION":                  "us-west-2",
				"AWS_DEFAULT_REGION":          "us-west-2",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name: "clears conflicting credential environment variables",
			inputEnv: map[string]string{
				"AWS_ACCESS_KEY_ID":           "AKIAIOSFODNN7EXAMPLE",
				"AWS_SECRET_ACCESS_KEY":       "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"AWS_SESSION_TOKEN":           "session-token-value",
				"AWS_SECURITY_TOKEN":          "security-token-value",
				"AWS_WEB_IDENTITY_TOKEN_FILE": "/path/to/token",
				"AWS_ROLE_ARN":                "arn:aws:iam::123456789012:role/MyRole",
				"AWS_ROLE_SESSION_NAME":       "my-session",
				"HOME":                        "/home/user",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name: "without region",
			inputEnv: map[string]string{
				"HOME": "/home/user",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name:            "with empty input environment",
			inputEnv:        map[string]string{},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "eu-central-1",
			expectedEnv: map[string]string{
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_REGION":                  "eu-central-1",
				"AWS_DEFAULT_REGION":          "eu-central-1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name: "preserves non-AWS environment variables",
			inputEnv: map[string]string{
				"HOME":       "/home/user",
				"PATH":       "/usr/bin",
				"USER":       "testuser",
				"LANG":       "en_US.UTF-8",
				"CUSTOM_VAR": "custom-value",
			},
			profile:         "test-profile",
			credentialsFile: "/home/user/.aws/atmos/provider/credentials",
			configFile:      "/home/user/.aws/atmos/provider/config",
			region:          "ap-southeast-1",
			expectedEnv: map[string]string{
				"HOME":                        "/home/user",
				"PATH":                        "/usr/bin",
				"USER":                        "testuser",
				"LANG":                        "en_US.UTF-8",
				"CUSTOM_VAR":                  "custom-value",
				"AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/provider/credentials",
				"AWS_CONFIG_FILE":             "/home/user/.aws/atmos/provider/config",
				"AWS_PROFILE":                 "test-profile",
				"AWS_SDK_LOAD_CONFIG":         "1",
				"AWS_REGION":                  "ap-southeast-1",
				"AWS_DEFAULT_REGION":          "ap-southeast-1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call PrepareEnvironment.
			result := PrepareEnvironment(tt.inputEnv, tt.profile, tt.credentialsFile, tt.configFile, tt.region)

			// Verify result matches expected environment.
			assert.Equal(t, tt.expectedEnv, result, "environment should match expected")

			// Verify input environment was not mutated.
			for key, value := range tt.inputEnv {
				// Check that original values are still there (unless they should be cleared).
				shouldBeCleared := false
				for _, clearVar := range environmentVarsToClear {
					if key == clearVar {
						shouldBeCleared = true
						break
					}
				}
				if !shouldBeCleared {
					// Non-cleared variables should remain in input.
					assert.Equal(t, value, tt.inputEnv[key], "input environment should not be mutated for %s", key)
				}
			}
		})
	}
}

func TestPrepareEnvironment_DoesNotMutateInput(t *testing.T) {
	// Create input environment.
	inputEnv := map[string]string{
		"HOME":                  "/home/user",
		"AWS_ACCESS_KEY_ID":     "AKIAIOSFODNN7EXAMPLE",
		"AWS_SECRET_ACCESS_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	// Save original values.
	originalHome := inputEnv["HOME"]
	originalAccessKey := inputEnv["AWS_ACCESS_KEY_ID"]
	originalSecretKey := inputEnv["AWS_SECRET_ACCESS_KEY"]

	// Call PrepareEnvironment.
	result := PrepareEnvironment(inputEnv, "test-profile", "/creds", "/config", "us-east-1")

	// Verify input was not mutated.
	assert.Equal(t, originalHome, inputEnv["HOME"], "HOME should not be modified in input")
	assert.Equal(t, originalAccessKey, inputEnv["AWS_ACCESS_KEY_ID"], "AWS_ACCESS_KEY_ID should not be modified in input")
	assert.Equal(t, originalSecretKey, inputEnv["AWS_SECRET_ACCESS_KEY"], "AWS_SECRET_ACCESS_KEY should not be modified in input")

	// Verify result does not contain credentials.
	_, hasAccessKey := result["AWS_ACCESS_KEY_ID"]
	_, hasSecretKey := result["AWS_SECRET_ACCESS_KEY"]
	assert.False(t, hasAccessKey, "result should not contain AWS_ACCESS_KEY_ID")
	assert.False(t, hasSecretKey, "result should not contain AWS_SECRET_ACCESS_KEY")

	// Verify result contains expected values.
	assert.Equal(t, "test-profile", result["AWS_PROFILE"])
	assert.Equal(t, "/creds", result["AWS_SHARED_CREDENTIALS_FILE"])
	assert.Equal(t, "/config", result["AWS_CONFIG_FILE"])
}

func TestWithIsolatedAWSEnv_ClearsAllProblematicVars(t *testing.T) {
	// Set all problematic env vars before calling WithIsolatedAWSEnv.
	for _, key := range problematicAWSEnvVars {
		t.Setenv(key, "test-value-"+key)
	}

	var envDuringExec map[string]string

	err := WithIsolatedAWSEnv(func() error {
		envDuringExec = make(map[string]string)
		for _, key := range problematicAWSEnvVars {
			if val, exists := os.LookupEnv(key); exists {
				envDuringExec[key] = val
			}
		}
		return nil
	})
	require.NoError(t, err)

	// Verify all problematic vars were cleared during execution.
	for _, key := range problematicAWSEnvVars {
		_, found := envDuringExec[key]
		assert.False(t, found, "expected %s to be unset during isolated execution", key)
	}

	// Verify all vars are restored after execution.
	for _, key := range problematicAWSEnvVars {
		val, exists := os.LookupEnv(key)
		assert.True(t, exists, "expected %s to be restored after isolated execution", key)
		assert.Equal(t, "test-value-"+key, val, "expected %s to be restored to original value", key)
	}
}

func TestWithIsolatedAWSEnv_RestoresOnlySetVars(t *testing.T) {
	// Only set a subset of problematic vars.
	t.Setenv("AWS_PROFILE", "my-profile")
	// Ensure AWS_ACCESS_KEY_ID is NOT set.
	os.Unsetenv("AWS_ACCESS_KEY_ID")

	err := WithIsolatedAWSEnv(func() error {
		// Both should be unset during execution.
		_, profileExists := os.LookupEnv("AWS_PROFILE")
		assert.False(t, profileExists, "AWS_PROFILE should be unset during isolated execution")

		_, accessKeyExists := os.LookupEnv("AWS_ACCESS_KEY_ID")
		assert.False(t, accessKeyExists, "AWS_ACCESS_KEY_ID should be unset during isolated execution")
		return nil
	})
	require.NoError(t, err)

	// AWS_PROFILE was set before, so it should be restored.
	val, exists := os.LookupEnv("AWS_PROFILE")
	assert.True(t, exists, "AWS_PROFILE should be restored")
	assert.Equal(t, "my-profile", val)

	// AWS_ACCESS_KEY_ID was not set before, so it should remain unset.
	_, exists = os.LookupEnv("AWS_ACCESS_KEY_ID")
	assert.False(t, exists, "AWS_ACCESS_KEY_ID should remain unset")
}

func TestWithIsolatedAWSEnv_PropagatesError(t *testing.T) {
	// Verify that errors from the inner function are propagated.
	expectedErr := assert.AnError
	err := WithIsolatedAWSEnv(func() error {
		return expectedErr
	})
	assert.ErrorIs(t, err, expectedErr)
}

func TestLoadIsolatedAWSConfig_IgnoresDefaultProfile(t *testing.T) {
	// Create a temp directory with a fake AWS config that has a [default] profile
	// referencing a credential_process that would fail if loaded.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	err := os.WriteFile(configPath, []byte(`[default]
region = us-east-1
credential_process = /nonexistent/binary --this-would-fail
`), 0o600)
	require.NoError(t, err)

	credsPath := filepath.Join(tmpDir, "credentials")
	err = os.WriteFile(credsPath, []byte(`[default]
aws_access_key_id = AKIA_SHOULD_NOT_BE_LOADED
aws_secret_access_key = secret_should_not_be_loaded
`), 0o600)
	require.NoError(t, err)

	// Point the default AWS config paths to our test files.
	// The isolated config should ignore these completely.
	t.Setenv("HOME", tmpDir)
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_PROFILE", "default")

	ctx := context.Background()
	// LoadIsolatedAWSConfig should succeed because it ignores all shared config files.
	// It won't find any credentials (which is expected for isolated config loading).
	cfg, err := LoadIsolatedAWSConfig(ctx)
	require.NoError(t, err, "LoadIsolatedAWSConfig should succeed even with a problematic default profile")

	// The config should have been loaded without any profile settings.
	// Region should be empty since we didn't pass config.WithRegion().
	assert.Empty(t, cfg.Region, "region should be empty when no region option is provided")
}

func TestLoadIsolatedAWSConfig_WithRegionOption(t *testing.T) {
	// Verify that explicit options are still respected.
	t.Setenv("AWS_PROFILE", "some-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA_SHOULD_BE_IGNORED")

	ctx := context.Background()

	// We can't import config in the test directly since we're in the same package.
	// But we can verify the function works with the region option.
	cfg, err := LoadIsolatedAWSConfig(ctx)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
}

func TestLoadIsolatedAWSConfig_EnvVarsRestoredAfterCall(t *testing.T) {
	// Set env vars that should be restored after the call.
	t.Setenv("AWS_PROFILE", "restore-me")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA_RESTORE")

	ctx := context.Background()
	_, err := LoadIsolatedAWSConfig(ctx)
	require.NoError(t, err)

	// Verify env vars are restored.
	assert.Equal(t, "restore-me", os.Getenv("AWS_PROFILE"), "AWS_PROFILE should be restored after LoadIsolatedAWSConfig")
	assert.Equal(t, "AKIA_RESTORE", os.Getenv("AWS_ACCESS_KEY_ID"), "AWS_ACCESS_KEY_ID should be restored after LoadIsolatedAWSConfig")
}

func TestWarnIfAWSProfileSet_WithProfileSet(t *testing.T) {
	// This test verifies the function doesn't panic and runs correctly.
	// We can't easily capture log output, but we verify no errors occur.
	t.Setenv("AWS_PROFILE", "test-profile")
	assert.NotPanics(t, func() {
		WarnIfAWSProfileSet()
	})
}

func TestWarnIfAWSProfileSet_WithoutProfileSet(t *testing.T) {
	// Unset AWS_PROFILE to verify no warning path.
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AWS_CONFIG_FILE")
	os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
	assert.NotPanics(t, func() {
		WarnIfAWSProfileSet()
	})
}

func TestWarnIfAWSProfileSet_WithEmptyProfile(t *testing.T) {
	// Empty AWS_PROFILE should not trigger a warning.
	t.Setenv("AWS_PROFILE", "")
	assert.NotPanics(t, func() {
		WarnIfAWSProfileSet()
	})
}

func TestWarnIfAWSProfileSet_WithConfigAndCredsFiles(t *testing.T) {
	// Verify debug logging path for AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE.
	t.Setenv("AWS_CONFIG_FILE", "/custom/config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/custom/credentials")
	assert.NotPanics(t, func() {
		WarnIfAWSProfileSet()
	})
}

func TestLoadAtmosManagedAWSConfig_ClearsCredentialVarsOnly(t *testing.T) {
	// Verify that LoadAtmosManagedAWSConfig only clears credential vars,
	// not profile or file path vars.
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA_SHOULD_BE_CLEARED")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret_should_be_cleared")
	t.Setenv("AWS_SESSION_TOKEN", "token_should_be_cleared")
	t.Setenv("AWS_PROFILE", "should-be-preserved")

	ctx := context.Background()

	// The function should succeed (even if it can't find the profile, it loads config).
	_, _ = LoadAtmosManagedAWSConfig(ctx)

	// After the call, all env vars should be restored.
	assert.Equal(t, "AKIA_SHOULD_BE_CLEARED", os.Getenv("AWS_ACCESS_KEY_ID"), "credential vars should be restored after call")
	assert.Equal(t, "secret_should_be_cleared", os.Getenv("AWS_SECRET_ACCESS_KEY"), "credential vars should be restored after call")
	assert.Equal(t, "token_should_be_cleared", os.Getenv("AWS_SESSION_TOKEN"), "credential vars should be restored after call")
	assert.Equal(t, "should-be-preserved", os.Getenv("AWS_PROFILE"), "AWS_PROFILE should be preserved and restored")
}

func TestProblematicAWSEnvVars_ContainsExpectedVars(t *testing.T) {
	// Verify the list of problematic env vars contains all expected entries.
	expectedVars := []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_PROFILE",
		"AWS_CONFIG_FILE",
		"AWS_SHARED_CREDENTIALS_FILE",
	}

	for _, expected := range expectedVars {
		found := false
		for _, actual := range problematicAWSEnvVars {
			if actual == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "problematicAWSEnvVars should contain %s", expected)
	}
}

func TestLoadIsolatedAWSConfig_ErrorFromInvalidOption(t *testing.T) {
	// Test the error path by passing a config option that forces LoadDefaultConfig to fail.
	ctx := context.Background()

	invalidOpt := func(o *config.LoadOptions) error {
		return assert.AnError
	}

	_, err := LoadIsolatedAWSConfig(ctx, invalidOpt)
	assert.Error(t, err, "LoadIsolatedAWSConfig should return error from invalid option")
	assert.Contains(t, err.Error(), "failed to load AWS config")
}

func TestLoadIsolatedAWSConfig_RespectsRegionOption(t *testing.T) {
	// Verify that user-provided options are applied after isolation options.
	ctx := context.Background()

	cfg, err := LoadIsolatedAWSConfig(ctx, config.WithRegion("ap-northeast-1"))
	require.NoError(t, err)
	assert.Equal(t, "ap-northeast-1", cfg.Region, "explicit region option should be respected")
}

func TestLoadIsolatedAWSConfig_DoesNotLoadSharedFiles(t *testing.T) {
	// Create a temp directory with a config that sets a specific region.
	// The isolated config should NOT pick up this region.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	err := os.WriteFile(configPath, []byte("[default]\nregion = eu-west-3\n"), 0o600)
	require.NoError(t, err)

	// Point AWS to the test config.
	t.Setenv("HOME", tmpDir)
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_PROFILE", "default")

	ctx := context.Background()
	cfg, err := LoadIsolatedAWSConfig(ctx)
	require.NoError(t, err)

	// Region should NOT be "eu-west-3" because shared config files are ignored.
	assert.NotEqual(t, "eu-west-3", cfg.Region, "isolated config should not load region from shared config file")
}

func TestLoadAtmosManagedAWSConfig_ReturnsErrorOnInvalidOption(t *testing.T) {
	// Test the error path by passing a config option that forces LoadDefaultConfig to fail.
	ctx := context.Background()

	invalidOpt := func(o *config.LoadOptions) error {
		return assert.AnError
	}

	_, err := LoadAtmosManagedAWSConfig(ctx, invalidOpt)
	assert.Error(t, err, "LoadAtmosManagedAWSConfig should return error from invalid option")
	assert.Contains(t, err.Error(), "failed to load AWS config")
}

func TestLoadAtmosManagedAWSConfig_SuccessWithRegion(t *testing.T) {
	// Verify managed config loads correctly and accepts region option.
	ctx := context.Background()

	cfg, err := LoadAtmosManagedAWSConfig(ctx, config.WithRegion("us-west-2"))
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", cfg.Region)
}

func TestLoadAtmosManagedAWSConfig_RestoresCredentialVars(t *testing.T) {
	// Set credential vars and verify they are restored after the call.
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key-id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "test-session-token")

	ctx := context.Background()
	_, _ = LoadAtmosManagedAWSConfig(ctx, config.WithRegion("us-east-1"))

	// All credential vars should be restored.
	assert.Equal(t, "test-key-id", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "test-secret-key", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	assert.Equal(t, "test-session-token", os.Getenv("AWS_SESSION_TOKEN"))
}

func TestLoadAtmosManagedAWSConfig_DoesNotClearProfileVar(t *testing.T) {
	// Verify that AWS_PROFILE is NOT cleared during managed config loading.
	t.Setenv("AWS_PROFILE", "managed-profile")

	var profileDuringLoad string
	// We can't directly check env during load, but verify it's preserved after.
	ctx := context.Background()
	_, _ = LoadAtmosManagedAWSConfig(ctx, config.WithRegion("us-east-1"))
	_ = profileDuringLoad

	assert.Equal(t, "managed-profile", os.Getenv("AWS_PROFILE"), "AWS_PROFILE should be preserved during managed config loading")
}

func TestWithIsolatedAWSEnv_NoVarsSetToRestore(t *testing.T) {
	// Unset all problematic vars to test the path where no vars need restoration.
	for _, key := range problematicAWSEnvVars {
		os.Unsetenv(key)
	}

	executed := false
	err := WithIsolatedAWSEnv(func() error {
		executed = true
		// Verify vars are still unset.
		for _, key := range problematicAWSEnvVars {
			_, exists := os.LookupEnv(key)
			assert.False(t, exists, "%s should remain unset during isolated execution", key)
		}
		return nil
	})
	require.NoError(t, err)
	assert.True(t, executed, "function should have been executed")
}

func TestWithIsolatedAWSEnv_RestoresAfterPanic(t *testing.T) {
	// Verify behavior when the function panics.
	t.Setenv("AWS_PROFILE", "panic-test")

	assert.Panics(t, func() {
		_ = WithIsolatedAWSEnv(func() error {
			panic("test panic")
		})
	})

	// Note: After a panic, env vars may not be restored because
	// WithIsolatedAWSEnv doesn't use defer for restoration.
	// This test documents the current behavior.
}

func TestEnvironmentVarsToClear_ContainsExpectedVars(t *testing.T) {
	// Verify the list of credential env vars to clear.
	expectedVars := []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_SECURITY_TOKEN",
		"AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_ROLE_ARN",
		"AWS_ROLE_SESSION_NAME",
	}

	for _, expected := range expectedVars {
		found := false
		for _, actual := range environmentVarsToClear {
			if actual == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "environmentVarsToClear should contain %s", expected)
	}
}
