package cmd

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthCLIIntegrationWithCloudProvider(t *testing.T) {
	// Skip integration tests in CI or if no auth config is available
	if os.Getenv("CI") != "" {
		t.Skipf("Skipping integration tests in CI environment.")
	}

	// Create test auth configuration
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-aws-provider": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://example.awsapps.com/start",
			},
		},
		Identities: map[string]schema.Identity{
			"test-identity": {
				Kind:    "aws/permission-set",
				Default: true,
				Via: &schema.IdentityVia{
					Provider: "test-aws-provider",
				},
				Principal: map[string]interface{}{
					"name": "TestPermissionSet",
					"account": map[string]interface{}{
						"name": "test-account",
					},
				},
			},
			"test-aws-user": {
				Kind: "aws/user",
				Credentials: map[string]interface{}{
					"region": "us-west-2",
				},
			},
		},
	}

	t.Run("AuthManager Integration", func(t *testing.T) {
		// Create auth manager with all dependencies
		credStore := credentials.NewCredentialStore()
		validator := validation.NewValidator()

		authManager, err := auth.NewAuthManager(
			authConfig,
			credStore,
			validator,
			nil,
		)
		require.NoError(t, err)
		assert.NotNil(t, authManager)

		// Test GetDefaultIdentity
		defaultIdentity, err := authManager.GetDefaultIdentity()
		require.NoError(t, err)
		assert.Equal(t, "test-identity", defaultIdentity)

		// Test GetProviderForIdentity
		provider := authManager.GetProviderForIdentity("test-identity")
		assert.Equal(t, "test-aws-provider", provider)

		// Test AWS user identity (should return aws-user as mock provider)
		userProvider := authManager.GetProviderForIdentity("test-aws-user")
		assert.Equal(t, "aws-user", userProvider)
	})

	t.Run("Environment Variable Integration", func(t *testing.T) {
		// Test that environment variables are properly formatted
		testEnvVars := []schema.EnvironmentVariable{
			{Key: "AWS_PROFILE", Value: "test-profile"},
			{Key: "AWS_REGION", Value: "us-east-1"},
			{Key: "AWS_SHARED_CREDENTIALS_FILE", Value: "/path/to/credentials"},
		}

		// Test export format
		exportOutput := formatEnvironmentVariables(testEnvVars, "export")
		assert.Contains(t, exportOutput, "export AWS_PROFILE='test-profile'")
		assert.Contains(t, exportOutput, "export AWS_REGION='us-east-1'")

		// Test dotenv format
		dotenvOutput := formatEnvironmentVariables(testEnvVars, "dotenv")
		assert.Contains(t, dotenvOutput, "AWS_PROFILE='test-profile'")
		assert.Contains(t, dotenvOutput, "AWS_REGION='us-east-1'")

		// Test JSON format
		jsonOutput := formatEnvironmentVariables(testEnvVars, "json")
		assert.Contains(t, jsonOutput, `"AWS_PROFILE": "test-profile"`)
		assert.Contains(t, jsonOutput, `"AWS_REGION": "us-east-1"`)
	})
}

// formatEnvironmentVariables is a helper function to test environment variable formatting.
func formatEnvironmentVariables(envVars []schema.EnvironmentVariable, format string) string {
	switch format {
	case "json":
		envMap := make(map[string]string)
		for _, env := range envVars {
			envMap[env.Key] = env.Value
		}
		// Simple JSON formatting for testing
		result := "{\n"
		for key, value := range envMap {
			result += `  "` + key + `": "` + value + `",` + "\n"
		}
		result += "}"
		return result
	case "dotenv":
		result := ""
		// Deterministic order
		keys := make([]string, 0, len(envVars))
		m := make(map[string]string)
		for _, env := range envVars {
			keys = append(keys, env.Key)
			m[env.Key] = env.Value
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := strings.ReplaceAll(m[k], "'", "'\\''")
			result += k + "='" + v + "'\n"
		}
		return result
	default: // export format
		result := ""
		keys := make([]string, 0, len(envVars))
		m := make(map[string]string)
		for _, env := range envVars {
			keys = append(keys, env.Key)
			m[env.Key] = env.Value
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := strings.ReplaceAll(m[k], "'", "'\\''")
			result += "export " + k + "='" + v + "'\n"
		}
		return result
	}
}
