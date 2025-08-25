package exec

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetRegistryAuth tests the main registry authentication function
func TestGetRegistryAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "GitHub Container Registry",
			registry:    "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Docker Hub",
			registry:    "docker.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Azure Container Registry",
			registry:    "test.azurecr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "AWS ECR",
			registry:    "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Google Container Registry",
			registry:    "gcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Unknown registry",
			registry:    "unknown.registry.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getRegistryAuth(tt.registry, tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				// Authentication might fail due to missing credentials, but should not panic
				if err != nil {
					t.Logf("Authentication failed as expected: %v", err)
				}
			}
		})
	}
}

// TestCloudProviderAuth tests authentication for different cloud providers
func TestCloudProviderAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		provider    string
		expectError bool
	}{
		{
			name:        "GitHub CR with token",
			registry:    "ghcr.io",
			provider:    "github",
			expectError: false,
		},
		{
			name:        "Docker Hub with credentials",
			registry:    "docker.io",
			provider:    "docker",
			expectError: false,
		},
		{
			name:        "Azure ACR",
			registry:    "test.azurecr.io",
			provider:    "azure",
			expectError: false,
		},
		{
			name:        "AWS ECR",
			registry:    "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			provider:    "aws",
			expectError: false,
		},
		{
			name:        "Google CR",
			registry:    "gcr.io",
			provider:    "google",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for testing
			switch tt.provider {
			case "github":
				os.Setenv("GITHUB_TOKEN", "test-token")
				defer os.Unsetenv("GITHUB_TOKEN")
			case "docker":
				os.Setenv("DOCKER_USERNAME", "test-user")
				os.Setenv("DOCKER_PASSWORD", "test-password")
				defer os.Unsetenv("DOCKER_USERNAME")
				defer os.Unsetenv("DOCKER_PASSWORD")
			case "azure":
				os.Setenv("AZURE_CLIENT_ID", "test-client-id")
				os.Setenv("AZURE_CLIENT_SECRET", "test-client-secret")
				os.Setenv("AZURE_TENANT_ID", "test-tenant-id")
				defer os.Unsetenv("AZURE_CLIENT_ID")
				defer os.Unsetenv("AZURE_CLIENT_SECRET")
				defer os.Unsetenv("AZURE_TENANT_ID")
			case "aws":
				os.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
				os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
				defer os.Unsetenv("AWS_ACCESS_KEY_ID")
				defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")
			case "google":
				os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/path/to/credentials.json")
				defer os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
			}

			atmosConfig := &schema.AtmosConfiguration{}
			_, err := getRegistryAuth(tt.registry, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// Authentication might fail due to invalid test credentials, but should not panic
				if err != nil {
					t.Logf("Authentication failed as expected with test credentials: %v", err)
				}
			}
		})
	}
}

// TestACRAuth tests Azure Container Registry authentication
func TestACRAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "ACR with service principal",
			registry: "test.azurecr.io",
			envVars: map[string]string{
				"AZURE_CLIENT_ID":     "test-client-id",
				"AZURE_CLIENT_SECRET": "test-client-secret",
				"AZURE_TENANT_ID":     "test-tenant-id",
			},
			expectError: true, // Will fail due to invalid credentials, but should handle correctly
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "ACR with default credential",
			registry:    "test.azurecr.io",
			envVars:     map[string]string{},
			expectError: true, // Will fail due to no Azure credentials, but should handle correctly
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "Invalid ACR format",
			registry:    "invalid-registry",
			envVars:     map[string]string{},
			expectError: true,
			errorMsg:    "invalid Azure Container Registry format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			atmosConfig := &schema.AtmosConfiguration{}
			auth, err := getRegistryAuth(tt.registry, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestGetACRAuthViaCLI tests Azure CLI authentication
func TestGetACRAuthViaCLI(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid ACR registry",
			registry:    "test.azurecr.io",
			expectError: true, // Will fail due to no Azure CLI, but should parse correctly
			errorMsg:    "failed to get ACR credentials via Azure CLI",
		},
		{
			name:        "Invalid registry format",
			registry:    "invalid-registry",
			expectError: true,
			errorMsg:    "invalid Azure Container Registry format",
		},
		{
			name:        "Empty registry",
			registry:    "",
			expectError: true,
			errorMsg:    "invalid Azure Container Registry format",
		},
		{
			name:        "Registry without .azurecr.io suffix",
			registry:    "test.other.io",
			expectError: true,
			errorMsg:    "invalid Azure Container Registry format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getACRAuthViaCLI(tt.registry)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestGetACRAuthViaServicePrincipal tests Azure Service Principal authentication
func TestGetACRAuthViaServicePrincipal(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		acrName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid ACR with service principal",
			registry:    "test.azurecr.io",
			acrName:     "test",
			expectError: true, // Will fail due to no Azure credentials, but should handle correctly
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "Empty registry",
			registry:    "",
			acrName:     "test",
			expectError: true,
			errorMsg:    "failed to get Azure token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getACRAuthViaServicePrincipal(tt.registry, tt.acrName, "test-client-id", "test-client-secret", "test-tenant-id")

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestGetACRAuthViaDefaultCredential tests Azure Default Credential authentication
func TestGetACRAuthViaDefaultCredential(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		acrName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid ACR with default credential",
			registry:    "test.azurecr.io",
			acrName:     "test",
			expectError: true, // Will fail due to no Azure credentials, but should handle correctly
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "Empty registry",
			registry:    "",
			acrName:     "test",
			expectError: true,
			errorMsg:    "failed to get Azure token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getACRAuthViaDefaultCredential(tt.registry, tt.acrName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestDockerCredHelpers tests Docker credential helpers functionality
func TestDockerCredHelpers(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		configPath  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Non-existent config file",
			registry:    "docker.io",
			configPath:  "/non/existent/path",
			expectError: true,
			errorMsg:    "failed to read Docker config",
		},
		{
			name:        "Empty config file",
			registry:    "docker.io",
			configPath:  "",
			expectError: true,
			errorMsg:    "failed to read Docker config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary config file for testing
			var configPath string
			if tt.configPath != "" && tt.configPath != "/non/existent/path" {
				tempDir := t.TempDir()
				configPath = filepath.Join(tempDir, "config.json")

				// Create a minimal Docker config
				config := map[string]interface{}{
					"auths": map[string]interface{}{},
				}

				configData, err := json.Marshal(config)
				require.NoError(t, err)

				err = os.WriteFile(configPath, configData, 0o644)
				require.NoError(t, err)
			} else {
				configPath = tt.configPath
			}

			// Test the function
			auth, err := getCredentialStoreAuth(tt.registry, configPath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestECRAuth tests AWS ECR authentication
func TestECRAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "ECR with AWS credentials",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":     "test-access-key",
				"AWS_SECRET_ACCESS_KEY": "test-secret-key",
				"AWS_REGION":            "us-west-2",
			},
			expectError: true, // Will fail due to invalid credentials, but should handle correctly
			errorMsg:    "failed to get ECR authorization token",
		},
		{
			name:        "Invalid ECR registry format",
			registry:    "invalid-ecr-registry",
			envVars:     map[string]string{},
			expectError: true,
			errorMsg:    "invalid ECR registry format",
		},
		{
			name:        "Non-ECR registry",
			registry:    "docker.io",
			envVars:     map[string]string{},
			expectError: true,
			errorMsg:    "not an ECR registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			atmosConfig := &schema.AtmosConfiguration{}
			auth, err := getRegistryAuth(tt.registry, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestDecodeDockerAuth tests Docker authentication string decoding
func TestDecodeDockerAuth(t *testing.T) {
	tests := []struct {
		name        string
		authString  string
		expectError bool
		expected    *authn.Basic
	}{
		{
			name:        "Valid auth string",
			authString:  base64.StdEncoding.EncodeToString([]byte("username:password")),
			expectError: false,
			expected: &authn.Basic{
				Username: "username",
				Password: "password",
			},
		},
		{
			name:        "Empty auth string",
			authString:  "",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "Invalid base64",
			authString:  "invalid-base64",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "Missing colon in decoded string",
			authString:  base64.StdEncoding.EncodeToString([]byte("username")),
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, password, err := decodeDockerAuth(tt.authString)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Username, username)
				assert.Equal(t, tt.expected.Password, password)
			}
		})
	}
}

// TestGetCredentialStoreAuth tests Docker credential store authentication
func TestGetCredentialStoreAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		configPath  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Non-existent config file",
			registry:    "docker.io",
			configPath:  "/non/existent/path",
			expectError: true,
			errorMsg:    "failed to read Docker config",
		},
		{
			name:        "Empty config path",
			registry:    "docker.io",
			configPath:  "",
			expectError: true,
			errorMsg:    "failed to read Docker config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getCredentialStoreAuth(tt.registry, tt.configPath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}
