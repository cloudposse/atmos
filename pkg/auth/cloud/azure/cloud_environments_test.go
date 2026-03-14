package azure

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCloudEnvironment(t *testing.T) {
	tests := []struct {
		name              string
		envName           string
		expectedName      string
		expectedLogin     string
		expectedMgmt      string
		expectedGraph     string
		expectedKeyVault  string
		expectedBlobSufx  string
		expectedPortalURL string
	}{
		{
			name:              "public cloud by name",
			envName:           "public",
			expectedName:      "public",
			expectedLogin:     "login.microsoftonline.com",
			expectedMgmt:      "https://management.azure.com/.default",
			expectedGraph:     "https://graph.microsoft.com/.default",
			expectedKeyVault:  "https://vault.azure.net/.default",
			expectedBlobSufx:  "blob.core.windows.net",
			expectedPortalURL: "https://portal.azure.com/",
		},
		{
			name:              "US government cloud",
			envName:           "usgovernment",
			expectedName:      "usgovernment",
			expectedLogin:     "login.microsoftonline.us",
			expectedMgmt:      "https://management.usgovcloudapi.net/.default",
			expectedGraph:     "https://graph.microsoft.us/.default",
			expectedKeyVault:  "https://vault.usgovcloudapi.net/.default",
			expectedBlobSufx:  "blob.core.usgovcloudapi.net",
			expectedPortalURL: "https://portal.azure.us/",
		},
		{
			name:              "China cloud",
			envName:           "china",
			expectedName:      "china",
			expectedLogin:     "login.chinacloudapi.cn",
			expectedMgmt:      "https://management.chinacloudapi.cn/.default",
			expectedGraph:     "https://microsoftgraph.chinacloudapi.cn/.default",
			expectedKeyVault:  "https://vault.azure.cn/.default",
			expectedBlobSufx:  "blob.core.chinacloudapi.cn",
			expectedPortalURL: "https://portal.azure.cn/",
		},
		{
			name:              "empty string defaults to public",
			envName:           "",
			expectedName:      "public",
			expectedLogin:     "login.microsoftonline.com",
			expectedMgmt:      "https://management.azure.com/.default",
			expectedGraph:     "https://graph.microsoft.com/.default",
			expectedKeyVault:  "https://vault.azure.net/.default",
			expectedBlobSufx:  "blob.core.windows.net",
			expectedPortalURL: "https://portal.azure.com/",
		},
		{
			name:              "unknown name defaults to public",
			envName:           "nonexistent",
			expectedName:      "public",
			expectedLogin:     "login.microsoftonline.com",
			expectedMgmt:      "https://management.azure.com/.default",
			expectedGraph:     "https://graph.microsoft.com/.default",
			expectedKeyVault:  "https://vault.azure.net/.default",
			expectedBlobSufx:  "blob.core.windows.net",
			expectedPortalURL: "https://portal.azure.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := GetCloudEnvironment(tt.envName)
			require.NotNil(t, env)
			assert.Equal(t, tt.expectedName, env.Name)
			assert.Equal(t, tt.expectedLogin, env.LoginEndpoint)
			assert.Equal(t, tt.expectedMgmt, env.ManagementScope)
			assert.Equal(t, tt.expectedGraph, env.GraphAPIScope)
			assert.Equal(t, tt.expectedKeyVault, env.KeyVaultScope)
			assert.Equal(t, tt.expectedBlobSufx, env.BlobStorageSuffix)
			assert.Equal(t, tt.expectedPortalURL, env.PortalURL)
		})
	}
}

func TestPublicCloudPreset(t *testing.T) {
	// PublicCloud should be the same object as GetCloudEnvironment("public").
	assert.Equal(t, PublicCloud, GetCloudEnvironment("public"))
	assert.Equal(t, "public", PublicCloud.Name)
}

func TestKnownCloudEnvironments(t *testing.T) {
	names := KnownCloudEnvironments()
	sort.Strings(names)

	assert.Equal(t, []string{"china", "public", "usgovernment"}, names)
}

func TestCloudEnvironmentEndpointsAreDistinct(t *testing.T) {
	// Each cloud environment must have unique endpoints.
	envs := []*CloudEnvironment{
		GetCloudEnvironment("public"),
		GetCloudEnvironment("usgovernment"),
		GetCloudEnvironment("china"),
	}

	// Collect all login endpoints, management scopes, etc.
	logins := make(map[string]bool)
	mgmts := make(map[string]bool)
	blobs := make(map[string]bool)
	portals := make(map[string]bool)

	for _, env := range envs {
		assert.False(t, logins[env.LoginEndpoint], "duplicate login endpoint: %s", env.LoginEndpoint)
		assert.False(t, mgmts[env.ManagementScope], "duplicate management scope: %s", env.ManagementScope)
		assert.False(t, blobs[env.BlobStorageSuffix], "duplicate blob suffix: %s", env.BlobStorageSuffix)
		assert.False(t, portals[env.PortalURL], "duplicate portal URL: %s", env.PortalURL)

		logins[env.LoginEndpoint] = true
		mgmts[env.ManagementScope] = true
		blobs[env.BlobStorageSuffix] = true
		portals[env.PortalURL] = true
	}
}

func TestGetCloudEnvironmentIsCasePreserving(t *testing.T) {
	// Keys are lowercase; uppercase should fall back to public.
	env := GetCloudEnvironment("USGovernment")
	assert.Equal(t, "public", env.Name, "uppercase lookup should fall back to public cloud")
}
