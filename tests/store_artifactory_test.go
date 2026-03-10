package tests

import (
	"encoding/json"
	"os"
	"testing"

	jfroglog "github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

func init() {
	// Enable JFrog SDK debug logging.
	jfroglog.SetLogger(jfroglog.NewLogger(jfroglog.DEBUG, os.Stderr))
}

// TestArtifactoryStoreIntegration tests the Artifactory store with a mock HTTP server
// that implements enough of the Artifactory Generic repository API to verify
// the store's Set and Get operations work correctly.
func TestArtifactoryStoreIntegration(t *testing.T) {
	// Skip if running short tests.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create mock Artifactory server.
	mockServer := httpmock.NewArtifactoryMockServer(t)

	// Create store pointing to mock server.
	prefix := "atmos"
	delimiter := "/"
	artifactoryStore, err := store.NewArtifactoryStore(store.ArtifactoryStoreOptions{
		URL:            mockServer.URL(),
		RepoName:       "test-repo",
		AccessToken:    strPtr("test-token"),
		Prefix:         &prefix,
		StackDelimiter: &delimiter,
	})
	require.NoError(t, err)
	require.NotNil(t, artifactoryStore)

	t.Run("Set and Get simple value", func(t *testing.T) {
		// Arrange.
		stack := "dev"
		component := "vpc"
		key := "vpc_id"
		value := "vpc-12345"

		// Act - Set the value.
		err := artifactoryStore.Set(stack, component, key, value)
		require.NoError(t, err)

		// Assert - Verify the file was stored in the mock.
		expectedPath := "test-repo/atmos/dev/vpc/vpc_id"
		storedData, exists := mockServer.GetFile(expectedPath)
		require.True(t, exists, "File should exist at path: %s", expectedPath)

		// Verify stored content is JSON.
		var storedValue string
		err = json.Unmarshal(storedData, &storedValue)
		require.NoError(t, err)
		assert.Equal(t, value, storedValue)

		// Act - Get the value back.
		result, err := artifactoryStore.Get(stack, component, key)
		require.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("Set and Get complex value", func(t *testing.T) {
		// Arrange.
		stack := "prod"
		component := "network"
		key := "config"
		value := map[string]interface{}{
			"vpc_id":     "vpc-67890",
			"subnet_ids": []interface{}{"subnet-1", "subnet-2", "subnet-3"},
			"tags": map[string]interface{}{
				"Environment": "production",
				"Team":        "platform",
			},
		}

		// Act - Set the value.
		err := artifactoryStore.Set(stack, component, key, value)
		require.NoError(t, err)

		// Act - Get the value back.
		result, err := artifactoryStore.Get(stack, component, key)
		require.NoError(t, err)

		// Assert - Compare as JSON to handle type differences.
		expectedJSON, _ := json.Marshal(value)
		actualJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(actualJSON))
	})

	t.Run("Get non-existent key returns error", func(t *testing.T) {
		// Act.
		result, err := artifactoryStore.Get("nonexistent", "component", "key")

		// Assert.
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("Set with nested component path", func(t *testing.T) {
		// Arrange.
		stack := "staging"
		component := "app/backend/api"
		key := "endpoint"
		value := "https://api.example.com"

		// Act.
		err := artifactoryStore.Set(stack, component, key, value)
		require.NoError(t, err)

		// Assert - Verify the file path is correct.
		expectedPath := "test-repo/atmos/staging/app/backend/api/endpoint"
		_, exists := mockServer.GetFile(expectedPath)
		require.True(t, exists, "File should exist at path: %s", expectedPath)

		// Verify we can retrieve it.
		result, err := artifactoryStore.Get(stack, component, key)
		require.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("Set with multi-level stack", func(t *testing.T) {
		// Arrange.
		stack := "plat/ue2/dev"
		component := "vpc"
		key := "cidr"
		value := "10.0.0.0/16"

		// Act.
		err := artifactoryStore.Set(stack, component, key, value)
		require.NoError(t, err)

		// Assert - Verify the file path uses the stack delimiter.
		expectedPath := "test-repo/atmos/plat/ue2/dev/vpc/cidr"
		_, exists := mockServer.GetFile(expectedPath)
		require.True(t, exists, "File should exist at path: %s", expectedPath)

		// Verify we can retrieve it.
		result, err := artifactoryStore.Get(stack, component, key)
		require.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("Set empty stack returns error", func(t *testing.T) {
		err := artifactoryStore.Set("", "component", "key", "value")
		assert.Error(t, err)
	})

	t.Run("Set empty component returns error", func(t *testing.T) {
		err := artifactoryStore.Set("stack", "", "key", "value")
		assert.Error(t, err)
	})

	t.Run("Set empty key returns error", func(t *testing.T) {
		err := artifactoryStore.Set("stack", "component", "", "value")
		assert.Error(t, err)
	})

	t.Run("Set nil value returns error", func(t *testing.T) {
		err := artifactoryStore.Set("stack", "component", "key", nil)
		assert.Error(t, err)
	})
}

// TestArtifactoryStoreGetKey tests the GetKey method for direct key access.
func TestArtifactoryStoreGetKey(t *testing.T) {
	// Skip if running short tests.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create mock Artifactory server.
	mockServer := httpmock.NewArtifactoryMockServer(t)

	// Create store pointing to mock server.
	prefix := "config"
	artifactoryStore, err := store.NewArtifactoryStore(store.ArtifactoryStoreOptions{
		URL:         mockServer.URL(),
		RepoName:    "test-repo",
		AccessToken: strPtr("test-token"),
		Prefix:      &prefix,
	})
	require.NoError(t, err)

	t.Run("GetKey retrieves direct key", func(t *testing.T) {
		// Arrange - Pre-populate the mock with a file.
		value := map[string]interface{}{
			"setting1": "value1",
			"setting2": 42,
		}
		content, _ := json.Marshal(value)
		mockServer.SetFile("test-repo/config/global-settings.json", content)

		// Act.
		result, err := artifactoryStore.GetKey("global-settings")
		require.NoError(t, err)

		// Assert.
		expectedJSON, _ := json.Marshal(value)
		actualJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(actualJSON))
	})

	t.Run("GetKey with nested path", func(t *testing.T) {
		// Arrange.
		value := "production-api-key"
		content, _ := json.Marshal(value)
		mockServer.SetFile("test-repo/config/secrets/api-key.json", content)

		// Act.
		result, err := artifactoryStore.GetKey("secrets/api-key")
		require.NoError(t, err)

		// Assert.
		assert.Equal(t, value, result)
	})
}

// TestArtifactoryStoreWithoutPrefix tests store behavior without a prefix.
func TestArtifactoryStoreWithoutPrefix(t *testing.T) {
	// Skip if running short tests.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create mock Artifactory server.
	mockServer := httpmock.NewArtifactoryMockServer(t)

	// Create store without prefix.
	delimiter := "/"
	artifactoryStore, err := store.NewArtifactoryStore(store.ArtifactoryStoreOptions{
		URL:            mockServer.URL(),
		RepoName:       "my-repo",
		AccessToken:    strPtr("test-token"),
		StackDelimiter: &delimiter,
	})
	require.NoError(t, err)

	t.Run("Set without prefix", func(t *testing.T) {
		// Arrange.
		stack := "dev"
		component := "app"
		key := "version"
		value := "1.0.0"

		// Act.
		err := artifactoryStore.Set(stack, component, key, value)
		require.NoError(t, err)

		// Assert - Path should have empty prefix (just repo/stack/component/key).
		expectedPath := "my-repo/dev/app/version"
		_, exists := mockServer.GetFile(expectedPath)
		require.True(t, exists, "File should exist at path: %s\nAll files: %v", expectedPath, mockServer.ListFiles())
	})
}

func strPtr(s string) *string {
	return &s
}
