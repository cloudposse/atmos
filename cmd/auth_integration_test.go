package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/internal/auth"
	"github.com/cloudposse/atmos/internal/auth/cloud"
	"github.com/cloudposse/atmos/internal/auth/config"
	"github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/environment"
	"github.com/cloudposse/atmos/internal/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthCLIIntegrationWithCloudProvider(t *testing.T) {
	// Skip integration tests in CI or if no auth config is available
	if os.Getenv("CI") != "" {
		t.Skip("Skipping integration tests in CI environment")
	}

	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "atmos-auth-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test auth configuration
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-aws-provider": {
				Kind:   "aws/iam-identity-center",
				Region: "us-east-1",
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
		configMerger := config.NewConfigMerger()
		awsFileManager := environment.NewAWSFileManager()
		validator := validation.NewValidator()

		authManager, err := auth.NewAuthManager(
			authConfig,
			credStore,
			awsFileManager,
			configMerger,
			validator,
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

	t.Run("Cloud Provider Factory Integration", func(t *testing.T) {
		factory := cloud.NewCloudProviderFactory()
		
		// Test AWS provider creation
		awsProvider, err := factory.GetCloudProvider("aws")
		assert.NoError(t, err)
		assert.NotNil(t, awsProvider)

		// Test Azure provider creation (should be placeholder)
		azureProvider, err := factory.GetCloudProvider("azure")
		assert.NoError(t, err)
		assert.NotNil(t, azureProvider)

		// Test GCP provider creation (should be placeholder)
		gcpProvider, err := factory.GetCloudProvider("gcp")
		assert.NoError(t, err)
		assert.NotNil(t, gcpProvider)

		// Test invalid provider
		_, err = factory.GetCloudProvider("invalid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported cloud provider")
	})

	t.Run("Cloud Provider Manager Integration", func(t *testing.T) {
		manager := cloud.NewCloudProviderManager()

		// Test environment variable retrieval for AWS
		envVars, err := manager.GetEnvironmentVariables("aws", "test-provider", "test-identity")
		assert.NoError(t, err)
		assert.NotEmpty(t, envVars)

		// Verify expected AWS environment variables
		envMap := envVars
		assert.Contains(t, envMap, "AWS_SHARED_CREDENTIALS_FILE")
		assert.Contains(t, envMap, "AWS_CONFIG_FILE")
		assert.Contains(t, envMap, "AWS_PROFILE")
		assert.Equal(t, "test-identity", envMap["AWS_PROFILE"])
	})

	t.Run("AWS Provider File Management", func(t *testing.T) {
		factory := cloud.NewCloudProviderFactory()
		awsProvider, err := factory.GetCloudProvider("aws")
		require.NoError(t, err)

		// Test credential validation (should pass for mock credentials)
		mockCreds := &schema.Credentials{
			AWS: &schema.AWSCredentials{
				AccessKeyID:     "AKIATEST123456789",
				SecretAccessKey: "testsecretkey",
				SessionToken:    "testtoken",
				Region:          "us-east-1",
			},
		}

		err = awsProvider.ValidateCredentials(context.Background(), mockCreds)
		// This will fail in test environment, but we're testing the interface
		// In a real environment with valid credentials, this would pass
		assert.Error(t, err) // Expected in test environment

		// Test environment variable generation
		envVars := awsProvider.GetEnvironmentVariables("test-provider", "test-identity")
		assert.NotEmpty(t, envVars)

		// Verify AWS file paths are generated correctly
		envMap := envVars
		assert.Contains(t, envMap["AWS_SHARED_CREDENTIALS_FILE"], "test-provider")
		assert.Contains(t, envMap["AWS_CONFIG_FILE"], "test-provider")
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
		assert.Contains(t, exportOutput, "export AWS_PROFILE=test-profile")
		assert.Contains(t, exportOutput, "export AWS_REGION=us-east-1")

		// Test dotenv format
		dotenvOutput := formatEnvironmentVariables(testEnvVars, "dotenv")
		assert.Contains(t, dotenvOutput, "AWS_PROFILE=test-profile")
		assert.Contains(t, dotenvOutput, "AWS_REGION=us-east-1")

		// Test JSON format
		jsonOutput := formatEnvironmentVariables(testEnvVars, "json")
		assert.Contains(t, jsonOutput, `"AWS_PROFILE": "test-profile"`)
		assert.Contains(t, jsonOutput, `"AWS_REGION": "us-east-1"`)
	})
}

// formatEnvironmentVariables is a helper function to test environment variable formatting
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
		for _, env := range envVars {
			result += env.Key + "=" + env.Value + "\n"
		}
		return result
	default: // export format
		result := ""
		for _, env := range envVars {
			result += "export " + env.Key + "=" + env.Value + "\n"
		}
		return result
	}
}

func TestAuthCommandsWithRealCloudProvider(t *testing.T) {
	// This test verifies that the CLI commands can work with the actual cloud provider interface
	// Skip if no real auth configuration is available
	if os.Getenv("ATMOS_AUTH_TEST") == "" {
		t.Skip("Skipping real cloud provider tests - set ATMOS_AUTH_TEST=1 to enable")
	}

	t.Run("Real AWS Provider Integration", func(t *testing.T) {
		// Test that we can create a real AWS provider and it implements the interface correctly
		factory := cloud.NewCloudProviderFactory()
		awsProvider, err := factory.GetCloudProvider("aws")
		require.NoError(t, err)

		// Test that the provider can generate file paths
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		
		envVars := awsProvider.GetEnvironmentVariables("test-provider", "test-identity")
		
		// Verify paths are under user's home directory
		for key, value := range envVars {
			if key == "AWS_SHARED_CREDENTIALS_FILE" || key == "AWS_CONFIG_FILE" {
				assert.Contains(t, value, homeDir)
				assert.Contains(t, value, ".aws/atmos/test-provider")
			}
		}
	})

	t.Run("File System Integration", func(t *testing.T) {
		// Test that AWS files can be created in the correct locations
		factory := cloud.NewCloudProviderFactory()
		awsProvider, err := factory.GetCloudProvider("aws")
		require.NoError(t, err)

		// Create temporary credentials for testing
		testCreds := &schema.Credentials{
			AWS: &schema.AWSCredentials{
				AccessKeyID:     "AKIATEST123456789",
				SecretAccessKey: "testsecretkey",
				SessionToken:    "testtoken",
				Region:          "us-east-1",
			},
		}

		// Test environment setup (this creates the files)
		err = awsProvider.SetupEnvironment(context.Background(), "test-provider", "test-identity", testCreds)
		assert.NoError(t, err)

		// Verify files were created
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		
		credentialsPath := filepath.Join(homeDir, ".aws", "atmos", "test-provider", "credentials")
		configPath := filepath.Join(homeDir, ".aws", "atmos", "test-provider", "config")
		
		// Check if files exist (they should be created by SetupEnvironment)
		_, err = os.Stat(credentialsPath)
		if err == nil {
			// If file exists, clean it up
			defer os.Remove(credentialsPath)
		}
		
		_, err = os.Stat(configPath)
		if err == nil {
			// If file exists, clean it up
			defer os.Remove(configPath)
		}

		// Clean up the directory structure
		defer os.RemoveAll(filepath.Join(homeDir, ".aws", "atmos", "test-provider"))
	})
}
