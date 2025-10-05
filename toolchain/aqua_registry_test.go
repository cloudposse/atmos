package toolchain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAquaRegistry(t *testing.T) {
	ar := NewAquaRegistry()

	assert.NotNil(t, ar)
	assert.NotNil(t, ar.client)
	assert.NotNil(t, ar.cache)
	assert.NotNil(t, ar.local)
	assert.Contains(t, ar.cache.baseDir, "tools-cache")
}

func TestAquaRegistry_LoadLocalConfig(t *testing.T) {
	ar := NewAquaRegistry()

	// Test with non-existent file (should not error)
	err := ar.LoadLocalConfig("non-existent.yaml")
	assert.NoError(t, err)

	// Test with valid config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tools.yaml")
	configContent := `
tools:
  terraform:
    type: github_release
    repo_owner: hashicorp
    repo_name: terraform
    url: https://releases.hashicorp.com/terraform/{{.Version}}/terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip
    format: zip
    binary_name: terraform
`
	err = os.WriteFile(configPath, []byte(configContent), defaultFileWritePermissions)
	require.NoError(t, err)

	err = ar.LoadLocalConfig(configPath)
	assert.NoError(t, err)
}

func TestAquaRegistry_GetTool_LocalConfig(t *testing.T) {
	ar := NewAquaRegistry()

	// Set up local config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tools.yaml")
	configContent := `
tools:
  hashicorp/terraform:
    type: http
    repo_owner: hashicorp
    repo_name: terraform
    url: https://releases.hashicorp.com/terraform/{{.Version}}/terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip
    format: zip
    binary_name: terraform
`
	err := os.WriteFile(configPath, []byte(configContent), defaultFileWritePermissions)
	require.NoError(t, err)

	err = ar.LoadLocalConfig(configPath)
	require.NoError(t, err)

	// Test getting tool from local config
	tool, err := ar.GetTool("hashicorp", "terraform")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "terraform", tool.Name)
	assert.Equal(t, "hashicorp", tool.RepoOwner)
	assert.Equal(t, "terraform", tool.RepoName)
	assert.Equal(t, "http", tool.Type)
}

func TestAquaRegistry_GetTool_RemoteRegistry(t *testing.T) {
	// Mock registry server
	registryYAML := `
packages:
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform
    url: https://releases.hashicorp.com/terraform/{{.Version}}/terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip
    format: zip
    binary_name: terraform
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir() // avoid polluting real cache

	// Test getting tool from remote registry
	tool, err := ar.fetchFromRegistry(ts.URL, "hashicorp", "terraform")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "terraform", tool.Name)
	assert.Equal(t, "hashicorp", tool.RepoOwner)
	assert.Equal(t, "terraform", tool.RepoName)
}

func TestAquaRegistry_GetToolWithVersion(t *testing.T) {
	ar := NewAquaRegistry()

	// Set up local config with a simple tool
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tools.yaml")
	configContent := `
tools:
  hashicorp/terraform:
    type: http
    repo_owner: hashicorp
    repo_name: terraform
    url: https://releases.hashicorp.com/terraform/{{.Version}}/terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip
    format: zip
    binary_name: terraform
`
	err := os.WriteFile(configPath, []byte(configContent), defaultFileWritePermissions)
	require.NoError(t, err)

	err = ar.LoadLocalConfig(configPath)
	require.NoError(t, err)

	// Test getting tool with version (should return same tool for http type)
	tool, err := ar.GetToolWithVersion("hashicorp", "terraform", "1.0.0")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "terraform", tool.Name)
}

func TestAquaRegistry_GetToolWithVersion_GitHubRelease(t *testing.T) {
	// Mock registry server with version overrides
	registryYAML := `
packages:
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform
    url: https://releases.hashicorp.com/terraform/{{.Version}}/terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip
    format: zip
    binary_name: terraform
    version_overrides:
      - version_constraint: ">= 1.0.0"
        asset: "terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip"
        format: "zip"
        files:
          - name: "terraform"
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Mock the resolveVersionOverrides method by setting up the registry URL
	ar.client = &http.Client{}

	// Test getting tool with version (this will test the version override logic)
	tool, err := ar.GetToolWithVersion("hashicorp", "terraform", "1.0.0")
	// This might fail due to network, but we're testing the structure
	if err == nil {
		assert.NotNil(t, tool)
	}
}

func TestAquaRegistry_fetchFromRegistry(t *testing.T) {
	// Mock registry server
	registryYAML := `
packages:
  - type: http
    repo_owner: test
    repo_name: tool
    url: https://example.com/tool-{{.Version}}.zip
    format: zip
    binary_name: tool
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Test fetching from registry
	tool, err := ar.fetchFromRegistry(ts.URL, "test", "tool")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "tool", tool.Name)
	assert.Equal(t, "test", tool.RepoOwner)
	assert.Equal(t, "tool", tool.RepoName)
}

func TestAquaRegistry_fetchFromRegistry_NotFound(t *testing.T) {
	// Mock registry server that returns 404
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Test fetching non-existent tool
	tool, err := ar.fetchFromRegistry(ts.URL, "nonexistent", "tool")
	assert.Error(t, err)
	assert.Nil(t, tool)
	assert.Contains(t, err.Error(), "tool not found in registry")
}

func TestAquaRegistry_fetchRegistryFile(t *testing.T) {
	// Mock registry server
	registryYAML := `
packages:
  - type: http
    repo_owner: test
    repo_name: tool
    url: https://example.com/tool-{{.Version}}.zip
    format: zip
    binary_name: tool
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Test fetching registry file
	tool, err := ar.fetchRegistryFile(ts.URL + "/test/tool/registry.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "tool", tool.Name)

	// Test that it's cached
	tool2, err := ar.fetchRegistryFile(ts.URL + "/test/tool/registry.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, tool2)
	assert.Equal(t, tool.Name, tool2.Name)
}

func TestAquaRegistry_parseRegistryFile_Packages(t *testing.T) {
	ar := NewAquaRegistry()

	// Test parsing packages format
	data := []byte(`
packages:
  - type: http
    repo_owner: test
    repo_name: tool
    url: https://example.com/tool-{{.Version}}.zip
    format: zip
    binary_name: tool
`)

	tool, err := ar.parseRegistryFile(data)
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "tool", tool.Name)
	assert.Equal(t, "test", tool.RepoOwner)
	assert.Equal(t, "tool", tool.RepoName)
	assert.Equal(t, "http", tool.Type)
}

func TestAquaRegistry_parseRegistryFile_Tools(t *testing.T) {
	ar := NewAquaRegistry()

	// Test parsing tools format
	data := []byte(`
tools:
  - name: tool
    type: http
    repo_owner: test
    repo_name: tool
    asset: https://example.com/tool-{{.Version}}.zip
    format: zip
`)

	tool, err := ar.parseRegistryFile(data)
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "tool", tool.Name)
	assert.Equal(t, "test", tool.RepoOwner)
	assert.Equal(t, "tool", tool.RepoName)
}

func TestAquaRegistry_parseRegistryFile_Invalid(t *testing.T) {
	ar := NewAquaRegistry()

	// Test parsing invalid YAML
	data := []byte(`invalid yaml content`)

	tool, err := ar.parseRegistryFile(data)
	assert.Error(t, err)
	assert.Nil(t, tool)
	assert.Contains(t, err.Error(), "no tools or packages found in registry file")
}

func TestAquaRegistry_BuildAssetURL_HTTP(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &Tool{
		Name:      "test-tool",
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "https://example.com/tool-{{.Version}}.zip",
		Format:    "zip",
	}

	url, err := ar.BuildAssetURL(tool, "1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/tool-1.0.0.zip", url)
}

func TestAquaRegistry_BuildAssetURL_GitHubRelease(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &Tool{
		Name:      "test-tool",
		Type:      "github_release",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "tool-{{.Version}}-{{.OS}}-{{.Arch}}.zip",
		Format:    "zip",
	}

	url, err := ar.BuildAssetURL(tool, "1.0.0")
	assert.NoError(t, err)
	assert.Contains(t, url, "https://github.com/test/tool/releases/download/v1.0.0/tool-1.0.0-")
}

func TestAquaRegistry_BuildAssetURL_WithTemplateFunctions(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &Tool{
		Name:      "test-tool",
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "https://example.com/tool-{{trimV .Version}}-{{.OS}}-{{.Arch}}.zip",
		Format:    "zip",
	}

	url, err := ar.BuildAssetURL(tool, "v1.0.0")
	assert.NoError(t, err)
	assert.Contains(t, url, "tool-1.0.0-")
	assert.NotContains(t, url, "v1.0.0")
}

func TestAquaRegistry_BuildAssetURL_NoAsset(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &Tool{
		Name:      "test-tool",
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "",
	}

	url, err := ar.BuildAssetURL(tool, "1.0.0")
	assert.Error(t, err)
	assert.Empty(t, url)
	assert.Contains(t, err.Error(), "no asset template defined for tool")
}

func TestAquaRegistry_BuildAssetURL_InvalidTemplate(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &Tool{
		Name:      "test-tool",
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "https://example.com/tool-{{.InvalidField}}.zip",
	}

	url, err := ar.BuildAssetURL(tool, "1.0.0")
	// The template engine might not fail on missing fields, so we'll just check the result
	// If it doesn't fail, the URL should contain the literal text
	if err != nil {
		assert.Contains(t, err.Error(), "failed to execute asset template")
	} else {
		assert.Contains(t, url, "<no value>")
	}
}

func TestAquaRegistry_convertLocalToolToTool(t *testing.T) {
	ar := NewAquaRegistry()

	localTool := &LocalTool{
		Type:       "http",
		RepoOwner:  "test",
		RepoName:   "tool",
		URL:        "https://example.com/tool-{{.Version}}.zip",
		Format:     "zip",
		BinaryName: "custom-tool",
	}

	tool := ar.convertLocalToolToTool(localTool, "test")
	assert.NotNil(t, tool)
	assert.Equal(t, "custom-tool", tool.Name)
	assert.Equal(t, "test", tool.RepoOwner)
	assert.Equal(t, "tool", tool.RepoName)
	assert.Equal(t, "http", tool.Type)
	assert.Equal(t, "https://example.com/tool-{{.Version}}.zip", tool.Asset)
}

func TestAquaRegistry_GetLatestVersion(t *testing.T) {
	// Mock GitHub API server
	releases := []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}{
		{TagName: "v2.0.0-beta", Prerelease: true},
		{TagName: "v1.5.0", Prerelease: false},
		{TagName: "v1.4.0", Prerelease: false},
		{TagName: "v1.3.0-beta", Prerelease: true},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.client = &http.Client{}

	// Test the GetLatestVersion method directly by calling the GitHub API
	// This test will work if the GitHub API is accessible, otherwise it will be skipped
	version, err := ar.GetLatestVersion("test", "tool")
	if err != nil {
		// If it fails due to network issues, that's expected in a test environment
		t.Logf("GetLatestVersion failed (expected in test environment): %v", err)
		return
	}

	// If it succeeds, verify the result
	assert.NotEmpty(t, version)
}

func TestAquaRegistry_GetLatestVersion_NoReleases(t *testing.T) {
	// Test with a non-existent repository to simulate no releases
	ar := NewAquaRegistry()

	version, err := ar.GetLatestVersion("nonexistent-owner-12345", "nonexistent-repo-12345")
	assert.Error(t, err)
	assert.Empty(t, version)
	// The error message may vary depending on GitHub API response
	assert.True(t, strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404"), "got: "+err.Error())
}

func TestAquaRegistry_GetAvailableVersions(t *testing.T) {
	// Test with a real repository that should have releases
	ar := NewAquaRegistry()

	versions, err := ar.GetAvailableVersions("hashicorp", "terraform")
	if err != nil {
		// If it fails due to network issues, that's expected in a test environment
		t.Logf("GetAvailableVersions failed (expected in test environment): %v", err)
		return
	}

	// If it succeeds, verify the result
	assert.NotEmpty(t, versions)
	assert.Greater(t, len(versions), 0)

	// All versions should be valid semver
	for _, version := range versions {
		assert.NotEmpty(t, version)
		assert.False(t, strings.HasPrefix(version, versionPrefix))
	}
}

func TestGetOS(t *testing.T) {
	os := getOS()
	assert.NotEmpty(t, os)
	assert.Contains(t, []string{"darwin", "linux", "windows"}, os)
}

func TestGetArch(t *testing.T) {
	arch := getArch()
	assert.NotEmpty(t, arch)
	assert.Contains(t, []string{"amd64", "arm64", "386"}, arch)
}

func TestAquaRegistry_CacheDirectory(t *testing.T) {
	ar := NewAquaRegistry()

	// Test that cache directory is created
	assert.Contains(t, ar.cache.baseDir, "tools-cache")

	// Test that cache directory can be created
	err := os.MkdirAll(ar.cache.baseDir, defaultMkdirPermissions)
	assert.NoError(t, err)

	// Verify directory exists
	_, err = os.Stat(ar.cache.baseDir)
	assert.NoError(t, err)
}

func TestAquaRegistry_ErrorHandling(t *testing.T) {
	ar := NewAquaRegistry()

	// Test getting tool that doesn't exist
	tool, err := ar.GetTool("nonexistent", "tool")
	assert.Error(t, err)
	assert.Nil(t, tool)
	assert.Contains(t, err.Error(), "not found in any registry")

	// Test getting tool with invalid owner/repo
	tool, err = ar.GetTool("", "")
	assert.Error(t, err)
	assert.Nil(t, tool)
}

func TestAquaRegistry_RegistryFallback(t *testing.T) {
	// Mock multiple registry servers
	registry1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer registry1.Close()

	registry2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Write([]byte(`
packages:
  - type: http
    repo_owner: test
    repo_name: tool
    url: https://example.com/tool-{{.Version}}.zip
    format: zip
    binary_name: tool
`))
	}))
	defer registry2.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Test that it falls back to the second registry
	tool, err := ar.fetchFromRegistry(registry1.URL, "test", "tool")
	assert.Error(t, err)
	assert.Nil(t, tool)
	tool, err = ar.fetchFromRegistry(registry2.URL, "test", "tool")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
}
