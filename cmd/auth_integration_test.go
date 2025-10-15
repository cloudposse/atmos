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

// TestAuthCommandCompletion tests shell completion for auth commands.
func TestAuthCommandCompletion(t *testing.T) {
	// Save current directory.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err := os.Chdir(origDir)
		require.NoError(t, err)
	}()

	// Change to demo-auth directory with valid auth config.
	testDir := "../examples/demo-auth"
	err = os.Chdir(testDir)
	require.NoError(t, err)

	t.Run("identity flag completion for auth command", func(t *testing.T) {
		// Test that identity completion function returns expected identities.
		completions, directive := identityFlagCompletion(authCmd, []string{}, "")

		// Verify we get the expected identities from demo-auth config.
		assert.NotEmpty(t, completions)
		assert.Contains(t, completions, "oidc")
		assert.Contains(t, completions, "sso")
		assert.Contains(t, completions, "superuser")
		assert.Contains(t, completions, "saml")
		assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp
	})

	t.Run("identity flag completion for auth exec command", func(t *testing.T) {
		// Test that identity completion works for auth exec command.
		completions, directive := identityFlagCompletion(authExecCmd, []string{}, "")

		// Verify we get identities.
		assert.NotEmpty(t, completions)
		assert.Contains(t, completions, "oidc")
		assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp
	})

	t.Run("format flag completion for auth env command", func(t *testing.T) {
		// Get the completion function for the format flag.
		completionFunc, exists := authEnvCmd.GetFlagCompletionFunc("format")
		require.True(t, exists, "Format flag should have completion function")

		// Call the completion function.
		completions, directive := completionFunc(authEnvCmd, []string{}, "")

		// Verify we get the expected formats.
		assert.Equal(t, 3, len(completions))
		assert.Contains(t, completions, "json")
		assert.Contains(t, completions, "bash")
		assert.Contains(t, completions, "dotenv")
		assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp
	})

	t.Run("auth command has persistent identity flag", func(t *testing.T) {
		// Verify auth command has identity flag.
		flag := authCmd.PersistentFlags().Lookup("identity")
		assert.NotNil(t, flag)
		assert.Equal(t, "i", flag.Shorthand)

		// Verify completion function is registered.
		completionFunc, exists := authCmd.GetFlagCompletionFunc("identity")
		assert.True(t, exists)
		assert.NotNil(t, completionFunc)
	})

	t.Run("auth subcommands inherit identity completion", func(t *testing.T) {
		// Test that subcommands like auth login can access parent's persistent flag.
		flag := authLoginCmd.InheritedFlags().Lookup("identity")
		assert.NotNil(t, flag, "auth login should inherit identity flag")
	})
}

// TestAuthEnvFormatCompletion specifically tests the format flag completion.
func TestAuthEnvFormatCompletion(t *testing.T) {
	t.Run("format flag provides correct completions", func(t *testing.T) {
		// Get the completion function for the format flag.
		completionFunc, exists := authEnvCmd.GetFlagCompletionFunc("format")
		require.True(t, exists, "Format flag should have completion function registered")

		// Call the completion function.
		completions, directive := completionFunc(authEnvCmd, []string{}, "")

		// Verify all supported formats are present.
		assert.ElementsMatch(t, SupportedFormats, completions)
		assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp
	})

	t.Run("format flag respects partial input", func(t *testing.T) {
		// Get the completion function.
		completionFunc, exists := authEnvCmd.GetFlagCompletionFunc("format")
		require.True(t, exists)

		// Call with partial input.
		completions, _ := completionFunc(authEnvCmd, []string{}, "js")

		// Should still return all formats (filtering is done by shell).
		assert.ElementsMatch(t, SupportedFormats, completions)
	})
}
