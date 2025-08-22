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
				os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
				os.Setenv("AWS_REGION", "us-west-2")
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
			name:     "AWS ECR invalid registry format",
			registry: "invalid-ecr-registry.com",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
			},
			testFunction:  getECRAuth,
			expectedError: true,
		},
		{
			name:     "Azure ACR with credentials",
			registry: "myregistry.azurecr.io",
			setupEnv: func() {
				os.Setenv("AZURE_CLIENT_ID", "test-client")
			},
			testFunction:  getACRAuth,
			expectedError: true, // Not fully implemented
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
