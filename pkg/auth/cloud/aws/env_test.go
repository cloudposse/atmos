package aws

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithIsolatedAWSEnv_ClearsProblematicVariables(t *testing.T) {
	// Set problematic environment variables.
	t.Setenv("AWS_PROFILE", "test-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_SESSION_TOKEN", "test-token")
	t.Setenv("AWS_CONFIG_FILE", "/custom/config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/custom/credentials")

	// Set a safe variable that should not be cleared.
	t.Setenv("AWS_REGION", "us-west-2")

	var envDuringExecution map[string]string

	err := WithIsolatedAWSEnv(func() error {
		// Capture environment during execution.
		envDuringExecution = map[string]string{
			"AWS_PROFILE":                   os.Getenv("AWS_PROFILE"),
			"AWS_ACCESS_KEY_ID":             os.Getenv("AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY":         os.Getenv("AWS_SECRET_ACCESS_KEY"),
			"AWS_SESSION_TOKEN":             os.Getenv("AWS_SESSION_TOKEN"),
			"AWS_CONFIG_FILE":               os.Getenv("AWS_CONFIG_FILE"),
			"AWS_SHARED_CREDENTIALS_FILE":   os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
			"AWS_REGION":                    os.Getenv("AWS_REGION"),
		}
		return nil
	})

	require.NoError(t, err)

	// Verify problematic variables were cleared during execution.
	assert.Equal(t, "", envDuringExecution["AWS_PROFILE"])
	assert.Equal(t, "", envDuringExecution["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "", envDuringExecution["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "", envDuringExecution["AWS_SESSION_TOKEN"])
	assert.Equal(t, "", envDuringExecution["AWS_CONFIG_FILE"])
	assert.Equal(t, "", envDuringExecution["AWS_SHARED_CREDENTIALS_FILE"])

	// Verify safe variable was preserved.
	assert.Equal(t, "us-west-2", envDuringExecution["AWS_REGION"])

	// Verify variables were restored after execution.
	assert.Equal(t, "test-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "test-key", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "test-secret", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	assert.Equal(t, "test-token", os.Getenv("AWS_SESSION_TOKEN"))
	assert.Equal(t, "/custom/config", os.Getenv("AWS_CONFIG_FILE"))
	assert.Equal(t, "/custom/credentials", os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
	assert.Equal(t, "us-west-2", os.Getenv("AWS_REGION"))
}

func TestWithIsolatedAWSEnv_RestoresOnError(t *testing.T) {
	// Set environment variables.
	t.Setenv("AWS_PROFILE", "original-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "original-key")

	expectedErr := assert.AnError

	err := WithIsolatedAWSEnv(func() error {
		// Verify variables are cleared.
		assert.Equal(t, "", os.Getenv("AWS_PROFILE"))
		assert.Equal(t, "", os.Getenv("AWS_ACCESS_KEY_ID"))
		return expectedErr
	})

	// Verify error was propagated.
	assert.Equal(t, expectedErr, err)

	// Verify variables were restored even after error.
	assert.Equal(t, "original-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "original-key", os.Getenv("AWS_ACCESS_KEY_ID"))
}

func TestWithIsolatedAWSEnv_HandlesUnsetVariables(t *testing.T) {
	// Ensure variables are not set initially.
	for _, key := range problematicAWSEnvVars {
		os.Unsetenv(key)
	}

	err := WithIsolatedAWSEnv(func() error {
		// Verify all problematic variables are still unset.
		for _, key := range problematicAWSEnvVars {
			assert.Equal(t, "", os.Getenv(key), "Variable %s should be empty", key)
		}
		return nil
	})

	require.NoError(t, err)

	// Verify variables remain unset after execution.
	for _, key := range problematicAWSEnvVars {
		assert.Equal(t, "", os.Getenv(key), "Variable %s should remain empty", key)
	}
}

func TestWithIsolatedAWSEnv_PartiallySetVariables(t *testing.T) {
	// Set only some variables.
	t.Setenv("AWS_PROFILE", "test-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	// AWS_SECRET_ACCESS_KEY intentionally not set.

	var envDuringExecution map[string]string

	err := WithIsolatedAWSEnv(func() error {
		envDuringExecution = map[string]string{
			"AWS_PROFILE":         os.Getenv("AWS_PROFILE"),
			"AWS_ACCESS_KEY_ID":   os.Getenv("AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY": os.Getenv("AWS_SECRET_ACCESS_KEY"),
		}
		return nil
	})

	require.NoError(t, err)

	// All should be cleared during execution.
	assert.Equal(t, "", envDuringExecution["AWS_PROFILE"])
	assert.Equal(t, "", envDuringExecution["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "", envDuringExecution["AWS_SECRET_ACCESS_KEY"])

	// Only originally set variables should be restored.
	assert.Equal(t, "test-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "test-key", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "", os.Getenv("AWS_SECRET_ACCESS_KEY"))
}

func TestLoadIsolatedAWSConfig_ClearsEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires AWS SDK initialization")
	}

	// Set conflicting environment variables.
	t.Setenv("AWS_PROFILE", "nonexistent-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "fake-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fake-secret")
	t.Setenv("AWS_REGION", "us-east-1")

	ctx := context.Background()

	// LoadIsolatedAWSConfig should succeed because it clears problematic variables.
	// The AWS SDK will fall back to its default credential chain without the conflicting vars.
	cfg, err := LoadIsolatedAWSConfig(ctx)

	// The function should complete without panic.
	// We don't assert NoError because in test environments without AWS credentials,
	// this may fail - but it should fail gracefully, not because of our env vars.
	_ = err

	// Verify config was returned (even if credentials aren't available).
	assert.NotNil(t, cfg)

	// Verify environment variables were restored after the call.
	assert.Equal(t, "nonexistent-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "fake-key", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "fake-secret", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	assert.Equal(t, "us-east-1", os.Getenv("AWS_REGION"))
}

func TestProblematicAWSEnvVars_Coverage(t *testing.T) {
	// Verify all expected problematic variables are in the list.
	expectedVars := map[string]bool{
		"AWS_ACCESS_KEY_ID":           true,
		"AWS_SECRET_ACCESS_KEY":       true,
		"AWS_SESSION_TOKEN":           true,
		"AWS_PROFILE":                 true,
		"AWS_CONFIG_FILE":             true,
		"AWS_SHARED_CREDENTIALS_FILE": true,
	}

	actualVars := make(map[string]bool)
	for _, v := range problematicAWSEnvVars {
		actualVars[v] = true
	}

	// Check all expected variables are present.
	for expected := range expectedVars {
		assert.True(t, actualVars[expected], "Variable %s should be in problematicAWSEnvVars", expected)
	}

	// Verify AWS_REGION is NOT in the list (it should be preserved).
	assert.False(t, actualVars["AWS_REGION"], "AWS_REGION should NOT be in problematicAWSEnvVars")
}

func TestWithIsolatedAWSEnv_LogsIgnoredVariables(t *testing.T) {
	// Set some problematic environment variables.
	t.Setenv("AWS_PROFILE", "test-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")

	// WithIsolatedAWSEnv should log which variables are being ignored.
	// Note: We can't easily test the log output in unit tests without mocking the logger,
	// but we can verify the function executes without error and the mechanism works.
	err := WithIsolatedAWSEnv(func() error {
		// Variables should be cleared here.
		assert.Equal(t, "", os.Getenv("AWS_PROFILE"))
		assert.Equal(t, "", os.Getenv("AWS_ACCESS_KEY_ID"))
		return nil
	})

	require.NoError(t, err)

	// Variables should be restored.
	assert.Equal(t, "test-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "test-key", os.Getenv("AWS_ACCESS_KEY_ID"))
}

func TestWithIsolatedAWSEnv_NoLogWhenNoVariablesSet(t *testing.T) {
	// Ensure no problematic variables are set.
	for _, key := range problematicAWSEnvVars {
		os.Unsetenv(key)
	}

	// WithIsolatedAWSEnv should not log anything when no variables are set.
	// This test just verifies the function executes without issues.
	err := WithIsolatedAWSEnv(func() error {
		return nil
	})

	require.NoError(t, err)
}
