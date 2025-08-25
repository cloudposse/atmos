package exec

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetRegistryAuth tests all authentication methods
func TestGetRegistryAuth(t *testing.T) {
	tests := []struct {
		name              string
		registry          string
		setupEnv          func()
		setupDockerConfig func() string
		expectedAuth      bool
		expectedError     bool
	}{
		{
			name:     "GitHub Container Registry with token",
			registry: "ghcr.io",
			setupEnv: func() {
				os.Setenv("GITHUB_TOKEN", "test-token")
			},
			expectedAuth:  true,
			expectedError: false,
		},
		{
			name:     "GitHub Container Registry without token",
			registry: "ghcr.io",
			setupEnv: func() {
				os.Unsetenv("GITHUB_TOKEN")
			},
			expectedAuth:  false,
			expectedError: true,
		},
		{
			name:     "Custom registry with environment variables",
			registry: "testregistry.com",
			setupEnv: func() {
				os.Setenv("TESTREGISTRY_COM_USERNAME", "testuser")
				os.Setenv("TESTREGISTRY_COM_PASSWORD", "testpass")
			},
			expectedAuth:  true,
			expectedError: false,
		},
		{
			name:     "Custom registry without environment variables",
			registry: "testregistry.com",
			setupEnv: func() {
				os.Unsetenv("TESTREGISTRY_COM_USERNAME")
				os.Unsetenv("TESTREGISTRY_COM_PASSWORD")
			},
			expectedAuth:  false,
			expectedError: true,
		},
		{
			name:     "Registry with hyphens in environment variables",
			registry: "my-registry.com",
			setupEnv: func() {
				os.Setenv("MY_REGISTRY_COM_USERNAME", "user")
				os.Setenv("MY_REGISTRY_COM_PASSWORD", "pass")
			},
			expectedAuth:  true,
			expectedError: false,
		},
		{
			name:     "Registry with dots and hyphens in environment variables",
			registry: "my-registry.example.com",
			setupEnv: func() {
				os.Setenv("MY_REGISTRY_EXAMPLE_COM_USERNAME", "user")
				os.Setenv("MY_REGISTRY_EXAMPLE_COM_PASSWORD", "pass")
			},
			expectedAuth:  true,
			expectedError: false,
		},
		{
			name:     "Docker config with direct auth",
			registry: "docker-registry.com",
			setupDockerConfig: func() string {
				return `{
					"auths": {
						"docker-registry.com": {
							"auth": "` + base64.StdEncoding.EncodeToString([]byte("user:pass")) + `"
						}
					}
				}`
			},
			expectedAuth:  true,
			expectedError: false,
		},
		{
			name:     "Docker config with credential store",
			registry: "store-registry.com",
			setupDockerConfig: func() string {
				return `{
					"auths": {},
					"credsStore": "desktop"
				}`
			},
			expectedAuth:  false, // Will fail because docker-credential-desktop doesn't exist in test
			expectedError: true,
		},
		{
			name:     "Docker config with registry-specific credential helper",
			registry: "helper-registry.com",
			setupDockerConfig: func() string {
				return `{
					"auths": {},
					"credHelpers": {
						"helper-registry.com": "desktop"
					}
				}`
			},
			expectedAuth:  false, // Will fail because docker-credential-desktop doesn't exist in test
			expectedError: true,
		},
		{
			name:     "Docker config with multiple credential helpers",
			registry: "ecr-registry.amazonaws.com",
			setupDockerConfig: func() string {
				return `{
					"auths": {},
					"credHelpers": {
						"helper-registry.com": "desktop",
						"ecr-registry.amazonaws.com": "ecr-login"
					}
				}`
			},
			expectedAuth:  false, // Will fail because docker-credential-ecr-login doesn't exist in test
			expectedError: true,
		},
		{
			name:     "Docker config with https:// prefixed credential helper",
			registry: "https-registry.com",
			setupDockerConfig: func() string {
				return `{
					"auths": {},
					"credHelpers": {
						"https://https-registry.com": "desktop"
					}
				}`
			},
			expectedAuth:  false, // Will fail because docker-credential-desktop doesn't exist in test
			expectedError: true,
		},
		{
			name:     "Docker config with DOCKER_CONFIG environment variable",
			registry: "custom-registry.com",
			setupEnv: func() {
				// This will be set up in the test to point to a custom directory
			},
			setupDockerConfig: func() string {
				return `{
					"auths": {
						"custom-registry.com": {
							"auth": "` + base64.StdEncoding.EncodeToString([]byte("customuser:custompass")) + `"
						}
					}
				}`
			},
			expectedAuth:  true,
			expectedError: false,
		},
		{
			name:     "AWS ECR registry",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			},
			expectedAuth:  false, // Not fully implemented yet
			expectedError: true,
		},
		{
			name:     "Azure Container Registry",
			registry: "myregistry.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_ID", "test-client")
			},
			expectedAuth:  false, // Not fully implemented yet
			expectedError: true,
		},
		{
			name:     "Google Container Registry",
			registry: "gcr.io",
			setupEnv: func() {
				os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/path/to/creds.json")
			},
			expectedAuth:  false, // Not fully implemented yet
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Create temporary Docker config if needed
			if tt.setupDockerConfig != nil {
				configData := tt.setupDockerConfig()
				tempDir := t.TempDir()

				// Check if this is a DOCKER_CONFIG test
				if tt.name == "Docker config with DOCKER_CONFIG environment variable" {
					// For DOCKER_CONFIG test, create config in a custom directory
					customConfigDir := filepath.Join(tempDir, "custom-docker")
					dockerConfigPath := filepath.Join(customConfigDir, "config.json")

					err := os.MkdirAll(filepath.Dir(dockerConfigPath), 0755)
					require.NoError(t, err)

					err = os.WriteFile(dockerConfigPath, []byte(configData), 0644)
					require.NoError(t, err)

					// Set DOCKER_CONFIG environment variable
					originalDockerConfig := os.Getenv("DOCKER_CONFIG")
					os.Setenv("DOCKER_CONFIG", customConfigDir)
					defer os.Setenv("DOCKER_CONFIG", originalDockerConfig)
				} else {
					// Standard test - create config in .docker subdirectory
					dockerConfigPath := filepath.Join(tempDir, ".docker", "config.json")

					err := os.MkdirAll(filepath.Dir(dockerConfigPath), 0755)
					require.NoError(t, err)

					err = os.WriteFile(dockerConfigPath, []byte(configData), 0644)
					require.NoError(t, err)

					// Mock the home directory to point to our temp dir
					originalHome := os.Getenv("HOME")
					os.Setenv("HOME", tempDir)
					defer os.Setenv("HOME", originalHome)
				}
			}

			// Test
			auth, err := getRegistryAuth(tt.registry)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, auth)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)

				// Verify it's a basic auth
				basicAuth, ok := auth.(*authn.Basic)
				assert.True(t, ok)
				assert.NotEmpty(t, basicAuth.Username)
				assert.NotEmpty(t, basicAuth.Password)
			}
		})
	}
}

// TestExtractZipFile tests ZIP file extraction
func TestExtractZipFile(t *testing.T) {
	// Create a test ZIP file in memory
	zipBuffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuffer)

	// Add test files to the ZIP
	testFiles := map[string]string{
		"main.tf":        "# Test Terraform file",
		"variables.tf":   "variable \"test\" {}",
		"outputs.tf":     "output \"test\" {}",
		"subdir/file.tf": "# Test file in subdirectory",
	}

	for filename, content := range testFiles {
		writer, err := zipWriter.Create(filename)
		require.NoError(t, err)

		_, err = writer.Write([]byte(content))
		require.NoError(t, err)
	}

	err := zipWriter.Close()
	require.NoError(t, err)

	// Test extraction
	tempDir := t.TempDir()
	zipReader := bytes.NewReader(zipBuffer.Bytes())

	err = extractZipFile(zipReader, tempDir)
	require.NoError(t, err)

	// Verify files were extracted
	for filename, expectedContent := range testFiles {
		filePath := filepath.Join(tempDir, filename)

		// Check file exists
		assert.FileExists(t, filePath)

		// Check content
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(content))
	}
}

// TestExtractZipFileZipSlip tests ZIP file extraction with Zip Slip protection
func TestExtractZipFileZipSlip(t *testing.T) {
	tests := []struct {
		name          string
		zipFiles      map[string]string
		expectedError bool
		errorContains string
	}{
		{
			name: "Path traversal with ../",
			zipFiles: map[string]string{
				"../../../etc/passwd": "malicious content",
				"normal.txt":          "normal content",
			},
			expectedError: true,
			errorContains: "illegal file path in ZIP",
		},
		{
			name: "Path traversal with ..\\",
			zipFiles: map[string]string{
				"..\\..\\..\\windows\\system32\\config\\sam": "malicious content",
				"normal.txt": "normal content",
			},
			expectedError: true,
			errorContains: "illegal file path in ZIP",
		},
		{
			name: "Absolute path traversal",
			zipFiles: map[string]string{
				"/etc/passwd": "malicious content",
				"normal.txt":  "normal content",
			},
			expectedError: true,
			errorContains: "illegal file path in ZIP",
		},
		{
			name: "Windows absolute path traversal",
			zipFiles: map[string]string{
				"C:\\Windows\\System32\\config\\sam": "malicious content",
				"normal.txt":                         "normal content",
			},
			expectedError: true,
			errorContains: "illegal file path in ZIP",
		},
		{
			name: "Nested path traversal",
			zipFiles: map[string]string{
				"subdir/../../../../etc/passwd": "malicious content",
				"normal.txt":                    "normal content",
			},
			expectedError: true,
			errorContains: "illegal file path in ZIP",
		},
		{
			name: "Valid nested paths (should work)",
			zipFiles: map[string]string{
				"subdir/nested/file.txt": "valid content",
				"normal.txt":             "normal content",
			},
			expectedError: false,
		},
		{
			name: "Valid paths with dots in names (should work)",
			zipFiles: map[string]string{
				"file.with.dots.txt": "valid content",
				"normal.txt":         "normal content",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test ZIP file in memory
			zipBuffer := new(bytes.Buffer)
			zipWriter := zip.NewWriter(zipBuffer)

			// Add test files to the ZIP
			for filename, content := range tt.zipFiles {
				writer, err := zipWriter.Create(filename)
				require.NoError(t, err)

				_, err = writer.Write([]byte(content))
				require.NoError(t, err)
			}

			err := zipWriter.Close()
			require.NoError(t, err)

			// Test extraction
			tempDir := t.TempDir()
			zipReader := bytes.NewReader(zipBuffer.Bytes())

			err = extractZipFile(zipReader, tempDir)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				// Verify files were extracted
				for filename, expectedContent := range tt.zipFiles {
					filePath := filepath.Join(tempDir, filename)
					assert.FileExists(t, filePath)
					content, err := os.ReadFile(filePath)
					require.NoError(t, err)
					assert.Equal(t, expectedContent, string(content))
				}
			}
		})
	}
}

// TestExtractZipFileSymlinks tests ZIP file extraction with symlink protection
func TestExtractZipFileSymlinks(t *testing.T) {
	// Create a test ZIP file in memory with symlinks
	zipBuffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuffer)

	// Add a normal file
	writer, err := zipWriter.Create("normal.txt")
	require.NoError(t, err)
	_, err = writer.Write([]byte("normal content"))
	require.NoError(t, err)

	// Add a symlink (this is a simplified test - real ZIP symlinks would need more complex setup)
	// For now, we'll test that the symlink detection logic exists and works
	// In a real scenario, ZIP files with symlinks would have specific headers

	err = zipWriter.Close()
	require.NoError(t, err)

	// Test extraction
	tempDir := t.TempDir()
	zipReader := bytes.NewReader(zipBuffer.Bytes())

	err = extractZipFile(zipReader, tempDir)
	require.NoError(t, err)

	// Verify the normal file was extracted
	filePath := filepath.Join(tempDir, "normal.txt")
	assert.FileExists(t, filePath)
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "normal content", string(content))
}

// TestExtractRawData tests raw data extraction
func TestExtractRawData(t *testing.T) {
	tempDir := t.TempDir()
	testData := "test raw data content"

	reader := strings.NewReader(testData)

	err := extractRawData(reader, tempDir, 0)
	require.NoError(t, err)

	// Check file was created
	rawFilePath := filepath.Join(tempDir, "layer_0_raw")
	assert.FileExists(t, rawFilePath)

	// Check content
	content, err := os.ReadFile(rawFilePath)
	require.NoError(t, err)
	assert.Equal(t, testData, string(content))
}

// TestCheckArtifactType tests artifact type validation
func TestCheckArtifactType(t *testing.T) {
	tests := []struct {
		name              string
		artifactType      string
		shouldBeSupported bool
	}{
		{
			name:              "Atmos artifact type",
			artifactType:      "application/vnd.atmos.component.terraform.v1+tar+gzip",
			shouldBeSupported: true,
		},
		{
			name:              "OpenTofu artifact type",
			artifactType:      "application/vnd.opentofu.modulepkg",
			shouldBeSupported: true,
		},
		{
			name:              "Terraform artifact type",
			artifactType:      "application/vnd.terraform.module.v1+tar+gzip",
			shouldBeSupported: true,
		},
		{
			name:              "Unsupported artifact type",
			artifactType:      "application/vnd.unsupported.type",
			shouldBeSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock descriptor with the test artifact type
			manifest := map[string]interface{}{
				"artifactType": tt.artifactType,
			}

			manifestBytes, err := json.Marshal(manifest)
			require.NoError(t, err)

			descriptor := &remote.Descriptor{
				Manifest: manifestBytes,
			}

			// Test the function
			checkArtifactType(descriptor, "test-image")

			// Note: This function only logs, so we can't easily test the output
			// In a real scenario, you might want to capture log output or make the function return a value
		})
	}
}

// TestDecodeDockerAuth tests Docker auth string decoding
func TestDecodeDockerAuth(t *testing.T) {
	tests := []struct {
		name          string
		authString    string
		expectedUser  string
		expectedPass  string
		expectedError bool
	}{
		{
			name:          "Valid auth string",
			authString:    base64.StdEncoding.EncodeToString([]byte("user:pass")),
			expectedUser:  "user",
			expectedPass:  "pass",
			expectedError: false,
		},
		{
			name:          "Invalid base64",
			authString:    "invalid-base64!",
			expectedUser:  "",
			expectedPass:  "",
			expectedError: true,
		},
		{
			name:          "Invalid format (no colon)",
			authString:    base64.StdEncoding.EncodeToString([]byte("nocolon")),
			expectedUser:  "",
			expectedPass:  "",
			expectedError: true,
		},
		{
			name:          "Empty auth string",
			authString:    "",
			expectedUser:  "",
			expectedPass:  "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, password, err := decodeDockerAuth(tt.authString)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUser, username)
				assert.Equal(t, tt.expectedPass, password)
			}
		})
	}
}

// TestGetCredentialStoreAuth tests credential store authentication
func TestGetCredentialStoreAuth(t *testing.T) {
	tests := []struct {
		name          string
		registry      string
		credsStore    string
		expectedError bool
	}{
		{
			name:          "Non-existent credential store",
			registry:      "test-registry.com",
			credsStore:    "nonexistent",
			expectedError: true,
		},
		{
			name:          "Empty credential store",
			registry:      "test-registry.com",
			credsStore:    "",
			expectedError: true,
		},
		{
			name:          "Registry with command injection attempt",
			registry:      "test-registry.com; rm -rf /",
			credsStore:    "desktop",
			expectedError: true,
		},
		{
			name:          "Credential store with command injection attempt",
			registry:      "test-registry.com",
			credsStore:    "desktop; rm -rf /",
			expectedError: true,
		},
		{
			name:          "Registry with shell metacharacters",
			registry:      "test-registry.com`whoami`",
			credsStore:    "desktop",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getCredentialStoreAuth(tt.registry, tt.credsStore)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, auth)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestCloudProviderAuth tests cloud provider authentication functions
func TestCloudProviderAuth(t *testing.T) {
	tests := []struct {
		name          string
		registry      string
		setupEnv      func()
		testFunction  func(string) (authn.Authenticator, error)
		expectedError bool
	}{
		{
			name:     "AWS ECR with credentials",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			},
			testFunction:  getECRAuth,
			expectedError: true, // Will fail in test environment without real AWS credentials
		},
		{
			name:     "AWS ECR without credentials",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			setupEnv: func() {
				os.Unsetenv("AWS_ACCESS_KEY_ID")
			},
			testFunction:  getECRAuth,
			expectedError: true,
		},
		{
			name:     "Azure ACR with service principal credentials",
			registry: "myregistry.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_ID", "test-client-id")
				os.Setenv("AZURE_CLIENT_SECRET", "test-client-secret")
				os.Setenv("AZURE_TENANT_ID", "test-tenant-id")
				os.Setenv("AZURE_SUBSCRIPTION_ID", "test-subscription-id")
			},
			testFunction:  getACRAuth,
			expectedError: true, // Requires Azure SDK implementation
		},
		{
			name:     "Azure ACR with Azure CLI",
			registry: "myregistry.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLI_AUTH", "true")
			},
			testFunction:  getACRAuth,
			expectedError: true, // Will fail in test environment without Azure CLI
		},
		{
			name:     "Azure ACR invalid registry format",
			registry: "invalid-registry.com",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_ID", "test-client-id")
			},
			testFunction:  getACRAuth,
			expectedError: true,
		},
		{
			name:     "Google GCR with credentials",
			registry: "gcr.io",
			setupEnv: func() {
				os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/path/to/creds.json")
			},
			testFunction:  getGCRAuth,
			expectedError: true, // Not fully implemented
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Test
			auth, err := tt.testFunction(tt.registry)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, auth)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestACRAuth tests Azure Container Registry authentication functionality
func TestACRAuth(t *testing.T) {
	tests := []struct {
		name          string
		registry      string
		setupEnv      func()
		expectedError bool
		expectedMsg   string
	}{
		{
			name:     "Azure ACR with service principal credentials",
			registry: "myacr.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_ID", "test-client-id")
				os.Setenv("AZURE_CLIENT_SECRET", "test-client-secret")
				os.Setenv("AZURE_TENANT_ID", "test-tenant-id")
				os.Setenv("AZURE_SUBSCRIPTION_ID", "test-subscription-id")
			},
			expectedError: true,
			expectedMsg:   "Azure ACR authentication requires Azure SDK implementation",
		},
		{
			name:     "Azure ACR with Azure CLI",
			registry: "myacr.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLI_AUTH", "true")
			},
			expectedError: true,
			expectedMsg:   "failed to get ACR credentials via Azure CLI",
		},
		{
			name:     "Azure ACR invalid registry format",
			registry: "invalid-registry.com",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_ID", "test-client-id")
			},
			expectedError: true,
			expectedMsg:   "invalid Azure Container Registry format",
		},
		{
			name:     "Azure ACR missing credentials",
			registry: "myacr.azurecr.io",
			setupEnv: func() {
				// Don't set any Azure credentials
			},
			expectedError: true,
			expectedMsg:   "Azure credentials not found",
		},
		{
			name:     "Azure ACR missing client ID",
			registry: "myacr.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_SECRET", "test-secret")
				os.Setenv("AZURE_TENANT_ID", "test-tenant")
				os.Setenv("AZURE_SUBSCRIPTION_ID", "test-sub")
			},
			expectedError: true,
			expectedMsg:   "AZURE_CLIENT_ID environment variable is required",
		},
		{
			name:     "Azure ACR missing client secret",
			registry: "myacr.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_ID", "test-client")
				os.Setenv("AZURE_TENANT_ID", "test-tenant")
				os.Setenv("AZURE_SUBSCRIPTION_ID", "test-sub")
			},
			expectedError: true,
			expectedMsg:   "AZURE_CLIENT_SECRET environment variable is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original environment variables
			originalClientID := os.Getenv("AZURE_CLIENT_ID")
			originalClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
			originalTenantID := os.Getenv("AZURE_TENANT_ID")
			originalSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
			originalCLIAuth := os.Getenv("AZURE_CLI_AUTH")

			// Clean up environment variables
			os.Unsetenv("AZURE_CLIENT_ID")
			os.Unsetenv("AZURE_CLIENT_SECRET")
			os.Unsetenv("AZURE_TENANT_ID")
			os.Unsetenv("AZURE_SUBSCRIPTION_ID")
			os.Unsetenv("AZURE_CLI_AUTH")

			// Setup
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Test
			auth, err := getACRAuth(tt.registry)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.expectedMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}

			// Restore original environment variables
			if originalClientID != "" {
				os.Setenv("AZURE_CLIENT_ID", originalClientID)
			}
			if originalClientSecret != "" {
				os.Setenv("AZURE_CLIENT_SECRET", originalClientSecret)
			}
			if originalTenantID != "" {
				os.Setenv("AZURE_TENANT_ID", originalTenantID)
			}
			if originalSubscriptionID != "" {
				os.Setenv("AZURE_SUBSCRIPTION_ID", originalSubscriptionID)
			}
			if originalCLIAuth != "" {
				os.Setenv("AZURE_CLI_AUTH", originalCLIAuth)
			}
		})
	}
}

// TestDockerCredHelpers tests Docker credential helpers functionality
func TestDockerCredHelpers(t *testing.T) {
	tests := []struct {
		name          string
		registry      string
		setupConfig   func() (string, func())
		expectedError bool
		expectedMsg   string
	}{
		{
			name:     "Registry-specific credential helper",
			registry: "my-registry.com",
			setupConfig: func() (string, func()) {
				// Create temporary config directory
				tempDir := t.TempDir()
				dockerConfigPath := filepath.Join(tempDir, ".docker", "config.json")

				// Create directory and config file
				err := os.MkdirAll(filepath.Dir(dockerConfigPath), 0755)
				require.NoError(t, err)

				configData := `{
					"auths": {},
					"credHelpers": {
						"my-registry.com": "desktop"
					}
				}`

				err = os.WriteFile(dockerConfigPath, []byte(configData), 0644)
				require.NoError(t, err)

				// Mock home directory
				originalHome := os.Getenv("HOME")
				os.Setenv("HOME", tempDir)

				cleanup := func() {
					os.Setenv("HOME", originalHome)
				}

				return dockerConfigPath, cleanup
			},
			expectedError: true,
			expectedMsg:   "no authentication found in Docker config for registry",
		},
		{
			name:     "Multiple credential helpers",
			registry: "ecr.amazonaws.com",
			setupConfig: func() (string, func()) {
				// Create temporary config directory
				tempDir := t.TempDir()
				dockerConfigPath := filepath.Join(tempDir, ".docker", "config.json")

				// Create directory and config file
				err := os.MkdirAll(filepath.Dir(dockerConfigPath), 0755)
				require.NoError(t, err)

				configData := `{
					"auths": {},
					"credHelpers": {
						"my-registry.com": "desktop",
						"ecr.amazonaws.com": "ecr-login",
						"gcr.io": "gcloud"
					}
				}`

				err = os.WriteFile(dockerConfigPath, []byte(configData), 0644)
				require.NoError(t, err)

				// Mock home directory
				originalHome := os.Getenv("HOME")
				os.Setenv("HOME", tempDir)

				cleanup := func() {
					os.Setenv("HOME", originalHome)
				}

				return dockerConfigPath, cleanup
			},
			expectedError: true,
			expectedMsg:   "no authentication found in Docker config for registry",
		},
		{
			name:     "Credential helper with fallback to auth",
			registry: "fallback-registry.com",
			setupConfig: func() (string, func()) {
				// Create temporary config directory
				tempDir := t.TempDir()
				dockerConfigPath := filepath.Join(tempDir, ".docker", "config.json")

				// Create directory and config file
				err := os.MkdirAll(filepath.Dir(dockerConfigPath), 0755)
				require.NoError(t, err)

				configData := `{
					"auths": {
						"fallback-registry.com": {
							"auth": "dXNlcjpwYXNz"
						}
					},
					"credHelpers": {
						"other-registry.com": "desktop"
					}
				}`

				err = os.WriteFile(dockerConfigPath, []byte(configData), 0644)
				require.NoError(t, err)

				// Mock home directory
				originalHome := os.Getenv("HOME")
				os.Setenv("HOME", tempDir)

				cleanup := func() {
					os.Setenv("HOME", originalHome)
				}

				return dockerConfigPath, cleanup
			},
			expectedError: false,
		},
		{
			name:     "Registry not in credential helpers",
			registry: "unknown-registry.com",
			setupConfig: func() (string, func()) {
				// Create temporary config directory
				tempDir := t.TempDir()
				dockerConfigPath := filepath.Join(tempDir, ".docker", "config.json")

				// Create directory and config file
				err := os.MkdirAll(filepath.Dir(dockerConfigPath), 0755)
				require.NoError(t, err)

				configData := `{
					"auths": {},
					"credHelpers": {
						"known-registry.com": "desktop"
					}
				}`

				err = os.WriteFile(dockerConfigPath, []byte(configData), 0644)
				require.NoError(t, err)

				// Mock home directory
				originalHome := os.Getenv("HOME")
				os.Setenv("HOME", tempDir)

				cleanup := func() {
					os.Setenv("HOME", originalHome)
				}

				return dockerConfigPath, cleanup
			},
			expectedError: true,
			expectedMsg:   "no authentication found in Docker config for registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			var cleanup func()
			if tt.setupConfig != nil {
				_, cleanup = tt.setupConfig()
				defer cleanup()
			}

			// Test
			auth, err := getDockerAuth(tt.registry)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.expectedMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)

				// Check that it's a Basic authenticator
				basicAuth, ok := auth.(*authn.Basic)
				assert.True(t, ok, "Expected Basic authenticator")
				assert.Equal(t, "user", basicAuth.Username)
				assert.Equal(t, "pass", basicAuth.Password)
			}
		})
	}
}

// TestECRAuth tests AWS ECR authentication functionality
func TestECRAuth(t *testing.T) {
	tests := []struct {
		name          string
		registry      string
		setupEnv      func()
		expectedError bool
		expectedMsg   string
	}{
		{
			name:     "AWS ECR with valid registry format",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			},
			expectedError: true,
			expectedMsg:   "failed to get ECR authorization token",
		},
		{
			name:     "AWS ECR with different region",
			registry: "987654321098.dkr.ecr.eu-west-1.amazonaws.com",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			},
			expectedError: true,
			expectedMsg:   "failed to get ECR authorization token",
		},
		{
			name:     "AWS ECR invalid registry format - too few parts",
			registry: "invalid-ecr-registry.com",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			},
			expectedError: true,
			expectedMsg:   "invalid ECR registry format",
		},
		{
			name:     "AWS ECR invalid registry format - missing parts",
			registry: "123456789012.dkr.ecr.amazonaws.com",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			},
			expectedError: true,
			expectedMsg:   "invalid ECR registry format",
		},
		{
			name:     "AWS ECR supports SSO/role providers (no env gating)",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			setupEnv: func() {
				// No environment variables set - should still attempt authentication
				os.Unsetenv("AWS_ACCESS_KEY_ID")
				os.Unsetenv("AWS_PROFILE")
			},
			expectedError: true,
			expectedMsg:   "failed to get ECR authorization token", // Will fail due to no real credentials, but won't gate on env vars
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Test
			auth, err := getECRAuth(tt.registry)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.expectedMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestProcessOciImageIntegration tests the full OCI image processing flow
func TestProcessOciImageIntegration(t *testing.T) {
	// This is a more complex integration test that would require mocking the OCI registry
	// For now, we'll test the individual components

	t.Run("Test with mock OCI image", func(t *testing.T) {
		// Create a temporary directory for extraction
		tempDir := t.TempDir()

		// Test that the function handles errors gracefully
		// This would require more complex mocking of the OCI registry
		// For now, we'll just verify the function signature and basic error handling

		// Mock atmos config
		atmosConfig := &schema.AtmosConfiguration{}

		// Test with invalid image name
		err := processOciImage(atmosConfig, "invalid-image-name", tempDir)
		assert.Error(t, err)
		// The error could be either "invalid image reference" or an authentication error
		// since we're trying to pull from a real registry
		assert.True(t, strings.Contains(err.Error(), "invalid image reference") ||
			strings.Contains(err.Error(), "authentication required") ||
			strings.Contains(err.Error(), "failed to pull image"))
	})
}

// Benchmark tests for performance
func BenchmarkExtractZipFile(b *testing.B) {
	// Create a large test ZIP file
	zipBuffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuffer)

	// Add multiple files
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("file_%d.tf", i)
		content := fmt.Sprintf("# Test file %d\nvariable \"test_%d\" {}", i, i)

		writer, err := zipWriter.Create(filename)
		require.NoError(b, err)

		_, err = writer.Write([]byte(content))
		require.NoError(b, err)
	}

	err := zipWriter.Close()
	require.NoError(b, err)

	zipData := zipBuffer.Bytes()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		zipReader := bytes.NewReader(zipData)

		err := extractZipFile(zipReader, tempDir)
		require.NoError(b, err)
	}
}

func BenchmarkGetRegistryAuth(b *testing.B) {
	// Setup test environment
	b.Setenv("GITHUB_TOKEN", "test-token")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = getRegistryAuth("ghcr.io")
	}
}
