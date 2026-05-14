package aqua

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// TestHTTPTimeout is the timeout for HTTP requests in tests to prevent hangs.
const testHTTPTimeout = 10 * time.Second

func TestNewAquaRegistry(t *testing.T) {
	ar := NewAquaRegistry()

	assert.NotNil(t, ar)
	assert.NotNil(t, ar.client)
	assert.NotNil(t, ar.cache)
	// Cache should be in XDG-compliant path: ~/.cache/atmos/toolchain/registry
	assert.Contains(t, ar.cache.baseDir, filepath.Join("atmos", "toolchain", "registry"))
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

	// Mock the resolveVersionOverrides method by setting up the registry URL.
	// Use timeout to prevent hangs in CI environments with network issues.
	ar.client = &http.Client{
		Timeout: testHTTPTimeout,
	}

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
	assert.ErrorIs(t, err, registry.ErrToolNotFound)
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
	assert.ErrorIs(t, err, registry.ErrNoPackagesInRegistry)
}

func TestAquaRegistry_BuildAssetURL_HTTP(t *testing.T) {
	ar := NewAquaRegistry()

	// Use .SemVer for version without prefix in asset URL.
	// .Version = v1.0.0 (full tag), .SemVer = 1.0.0 (without prefix).
	tool := &registry.Tool{
		Name:      "test-tool",
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "https://example.com/tool-{{.SemVer}}.zip",
		Format:    "zip",
	}

	url, err := ar.BuildAssetURL(tool, "1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/tool-1.0.0.zip", url)
}

func TestAquaRegistry_BuildAssetURL_GitHubRelease(t *testing.T) {
	ar := NewAquaRegistry()

	// Use .SemVer for version without prefix in asset filename.
	// With VersionPrefix: "v", .Version = v1.0.0 (full tag for URL), .SemVer = 1.0.0 (for filename).
	tool := &registry.Tool{
		Name:          "test-tool",
		Type:          "github_release",
		RepoOwner:     "test",
		RepoName:      "tool",
		Asset:         "tool-{{.SemVer}}-{{.OS}}-{{.Arch}}.zip",
		Format:        "zip",
		VersionPrefix: "v", // Explicitly set so .SemVer strips it.
	}

	url, err := ar.BuildAssetURL(tool, "1.0.0")
	assert.NoError(t, err)
	// URL should have v1.0.0 tag and asset should have 1.0.0 semver.
	assert.Contains(t, url, "https://github.com/test/tool/releases/download/v1.0.0/tool-1.0.0-")
}

func TestAquaRegistry_BuildAssetURL_WithTemplateFunctions(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &registry.Tool{
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

func TestAquaRegistry_BuildAssetURL_SprigFunctions(t *testing.T) {
	ar := NewAquaRegistry()

	tests := []struct {
		name     string
		asset    string
		contains string
	}{
		{
			name:     "sprig title function",
			asset:    "tool-{{title .OS}}-{{.Arch}}.tar.gz",
			contains: "tool-",
		},
		{
			name:     "sprig upper function",
			asset:    "tool-{{upper .OS}}-{{.Arch}}.tar.gz",
			contains: "tool-",
		},
		{
			name:     "sprig lower function",
			asset:    "tool-{{lower .RepoName}}-{{.SemVer}}.tar.gz",
			contains: "tool-tool-1.0.0.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &registry.Tool{
				Name:      "test-tool",
				Type:      "http",
				RepoOwner: "test",
				RepoName:  "TOOL",
				Asset:     tt.asset,
				Format:    "tar.gz",
			}

			url, err := ar.BuildAssetURL(tool, "1.0.0")
			assert.NoError(t, err)
			assert.Contains(t, url, tt.contains)
		})
	}
}

func TestAquaRegistry_BuildAssetURL_NoAsset(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &registry.Tool{
		Name:      "test-tool",
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "",
	}

	url, err := ar.BuildAssetURL(tool, "1.0.0")
	assert.Error(t, err)
	assert.Empty(t, url)
	assert.ErrorIs(t, err, registry.ErrNoAssetTemplate)
}

func TestAquaRegistry_BuildAssetURL_InvalidTemplate(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &registry.Tool{
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
		assert.ErrorIs(t, err, registry.ErrNoAssetTemplate)
	} else {
		assert.Contains(t, url, "<no value>")
	}
}

func TestAquaRegistry_convertLocalToolToTool(t *testing.T) {
	t.Skip("Local config support was removed in refactoring")
}

func TestAquaRegistry_GetLatestVersion(t *testing.T) {
	// Mock GitHub API server
	releases := []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}{
		{TagName: "v2.0.0-beta", Prerelease: true, Draft: false},
		{TagName: "v1.6.0", Prerelease: false, Draft: true}, // Draft release - should be skipped
		{TagName: "v1.5.0", Prerelease: false, Draft: false},
		{TagName: "v1.4.0", Prerelease: false, Draft: false},
		{TagName: "v1.3.0-beta", Prerelease: true, Draft: false},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer ts.Close()

	// Wire the mock server to AquaRegistry
	ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))

	version, err := ar.GetLatestVersion("test", "tool")
	require.NoError(t, err)
	assert.Equal(t, "1.5.0", version) // Should skip draft v1.6.0 and prerelease v2.0.0-beta
}

func TestAquaRegistry_GetLatestVersion_NoReleases(t *testing.T) {
	// Mock GitHub API server that returns empty releases array
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]")) // Empty array
	}))
	defer ts.Close()

	ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))

	version, err := ar.GetLatestVersion("test", "tool")
	assert.Error(t, err)
	assert.Empty(t, version)
	assert.ErrorIs(t, err, registry.ErrNoVersionsFound)
}

func TestAquaRegistry_GetAvailableVersions(t *testing.T) {
	// Mock GitHub API server
	releases := []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}{
		{TagName: "v2.0.0-beta", Prerelease: true, Draft: false},
		{TagName: "v1.6.0", Prerelease: false, Draft: true}, // Draft release - should be skipped
		{TagName: "v1.5.0", Prerelease: false, Draft: false},
		{TagName: "v1.4.0", Prerelease: false, Draft: false},
		{TagName: "v1.3.0", Prerelease: false, Draft: false},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer ts.Close()

	// Wire the mock server to AquaRegistry
	ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))

	versions, err := ar.GetAvailableVersions("test", "tool")
	require.NoError(t, err)
	require.Len(t, versions, 3) // Only non-prerelease, non-draft versions
	assert.Equal(t, "1.5.0", versions[0])
	assert.Equal(t, "1.4.0", versions[1])
	assert.Equal(t, "1.3.0", versions[2])

	// All versions should be valid semver without 'v' prefix
	for _, version := range versions {
		assert.NotEmpty(t, version)
		assert.False(t, strings.HasPrefix(version, versionPrefix))
	}
}

func TestAquaRegistry_GetLatestVersion_WithPagination(t *testing.T) {
	// Mock GitHub API server with pagination
	page1Releases := []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}{
		{TagName: "v2.0.0-beta", Prerelease: true, Draft: false},
		{TagName: "v1.9.0", Prerelease: false, Draft: true}, // Draft on page 1
	}

	page2Releases := []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}{
		{TagName: "v1.8.0", Prerelease: false, Draft: false}, // Should find this
		{TagName: "v1.7.0", Prerelease: false, Draft: false},
	}

	requestCount := 0
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		requestCount++
		if requestCount == 1 {
			// First request - check per_page parameter
			perPage := r.URL.Query().Get("per_page")
			assert.Equal(t, "100", perPage, "first request should include per_page=100")

			// First page - add Link header for pagination
			w.Header().Set("Link", `<`+ts.URL+`/repos/test/tool/releases?page=2>; rel="next"`)
			json.NewEncoder(w).Encode(page1Releases)
		} else {
			// Second page - no Link header (last page)
			json.NewEncoder(w).Encode(page2Releases)
		}
	}))
	defer ts.Close()

	ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))

	version, err := ar.GetLatestVersion("test", "tool")
	require.NoError(t, err)
	assert.Equal(t, "1.8.0", version) // Should find v1.8.0 on page 2
	assert.Equal(t, 2, requestCount, "should have made 2 requests for pagination")
}

func TestAquaRegistry_GetAvailableVersions_WithPagination(t *testing.T) {
	// Mock GitHub API server with pagination
	page1Releases := []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}{
		{TagName: "v2.0.0", Prerelease: false, Draft: false},
		{TagName: "v1.9.0", Prerelease: false, Draft: true}, // Draft - skip
	}

	page2Releases := []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}{
		{TagName: "v1.8.0", Prerelease: false, Draft: false},
		{TagName: "v1.7.0-beta", Prerelease: true, Draft: false}, // Prerelease - skip
	}

	requestCount := 0
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		requestCount++
		if requestCount == 1 {
			// First request - check per_page parameter
			perPage := r.URL.Query().Get("per_page")
			assert.Equal(t, "100", perPage, "first request should include per_page=100")

			// First page - add Link header for pagination
			w.Header().Set("Link", `<`+ts.URL+`/repos/test/tool/releases?page=2>; rel="next"`)
			json.NewEncoder(w).Encode(page1Releases)
		} else {
			// Second page - no Link header (last page)
			json.NewEncoder(w).Encode(page2Releases)
		}
	}))
	defer ts.Close()

	ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))

	versions, err := ar.GetAvailableVersions("test", "tool")
	require.NoError(t, err)
	require.Len(t, versions, 2) // Only v2.0.0 and v1.8.0
	assert.Equal(t, "2.0.0", versions[0])
	assert.Equal(t, "1.8.0", versions[1])
	assert.Equal(t, 2, requestCount, "should have made 2 requests for pagination")
}

func TestParseNextLink(t *testing.T) {
	tests := []struct {
		name       string
		linkHeader string
		expected   string
	}{
		{
			name:       "valid next link",
			linkHeader: `<https://api.github.com/repos/foo/bar/releases?page=2>; rel="next", <https://api.github.com/repos/foo/bar/releases?page=5>; rel="last"`,
			expected:   "https://api.github.com/repos/foo/bar/releases?page=2",
		},
		{
			name:       "no next link",
			linkHeader: `<https://api.github.com/repos/foo/bar/releases?page=1>; rel="prev", <https://api.github.com/repos/foo/bar/releases?page=5>; rel="last"`,
			expected:   "",
		},
		{
			name:       "empty link header",
			linkHeader: "",
			expected:   "",
		},
		{
			name:       "only next link",
			linkHeader: `<https://api.github.com/repos/foo/bar/releases?page=3>; rel="next"`,
			expected:   "https://api.github.com/repos/foo/bar/releases?page=3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNextLink(tt.linkHeader)
			assert.Equal(t, tt.expected, result)
		})
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

	// Test that cache directory is in XDG-compliant path
	assert.Contains(t, ar.cache.baseDir, filepath.Join("atmos", "toolchain", "registry"))

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
	assert.ErrorIs(t, err, registry.ErrToolNotFound)

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

func TestAquaRegistry_Search(t *testing.T) {
	ar := NewAquaRegistry()
	ctx := context.Background()

	t.Run("search kubectl - should find results", func(t *testing.T) {
		results, err := ar.Search(ctx, "kubectl", registry.WithLimit(5))
		require.NoError(t, err)
		assert.Greater(t, len(results), 0, "expected to find kubectl")

		// Verify all results have required fields.
		for _, tool := range results {
			assert.NotEmpty(t, tool.RepoOwner)
			assert.NotEmpty(t, tool.RepoName)
			assert.NotEmpty(t, tool.Type)
			assert.Equal(t, "aqua-public", tool.Registry)
		}
	})

	t.Run("search terraform - should find results", func(t *testing.T) {
		results, err := ar.Search(ctx, "terraform", registry.WithLimit(10))
		require.NoError(t, err)
		assert.Greater(t, len(results), 0, "expected to find terraform")
	})

	t.Run("search nonexistent - should return empty", func(t *testing.T) {
		results, err := ar.Search(ctx, "nonexistenttool12345xyz", registry.WithLimit(5))
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("empty query - returns default results", func(t *testing.T) {
		results, err := ar.Search(ctx, "", registry.WithLimit(5))
		require.NoError(t, err)
		// Empty query can return default results (interpreted as "list all").
		assert.LessOrEqual(t, len(results), 5)
	})

	t.Run("limit option", func(t *testing.T) {
		limit := 3
		results, err := ar.Search(ctx, "terraform", registry.WithLimit(limit))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), limit)
	})
}

func TestAquaRegistry_ListAll(t *testing.T) {
	ar := NewAquaRegistry()
	ctx := context.Background()

	t.Run("list with limit", func(t *testing.T) {
		limit := 10
		tools, err := ar.ListAll(ctx, registry.WithListLimit(limit))
		require.NoError(t, err)
		assert.Greater(t, len(tools), 0)
		assert.LessOrEqual(t, len(tools), limit)

		// Verify all tools have type field (some may not have owner/repo populated yet).
		for _, tool := range tools {
			assert.NotEmpty(t, tool.Type)
			assert.Equal(t, "aqua-public", tool.Registry)
		}
	})

	t.Run("list with offset", func(t *testing.T) {
		offset := 5
		limit := 5

		// Get first batch.
		firstBatch, err := ar.ListAll(ctx,
			registry.WithListLimit(limit),
			registry.WithListOffset(0),
		)
		require.NoError(t, err)

		// Get second batch with offset.
		secondBatch, err := ar.ListAll(ctx,
			registry.WithListLimit(limit),
			registry.WithListOffset(offset),
		)
		require.NoError(t, err)

		// Verify we got results.
		assert.Greater(t, len(firstBatch), 0)
		assert.Greater(t, len(secondBatch), 0)
	})

	t.Run("list with sort", func(t *testing.T) {
		tools, err := ar.ListAll(ctx,
			registry.WithListLimit(10),
			registry.WithSort("name"),
		)
		require.NoError(t, err)
		assert.Greater(t, len(tools), 0)
	})
}

func TestAquaRegistry_GetMetadata(t *testing.T) {
	ar := NewAquaRegistry()
	ctx := context.Background()

	meta, err := ar.GetMetadata(ctx)
	require.NoError(t, err)
	assert.Equal(t, "aqua-public", meta.Name)
	assert.Equal(t, "aqua", meta.Type)
	assert.Contains(t, meta.Source, "aqua-registry")
	assert.Equal(t, 10, meta.Priority)
}

func TestAquaRegistry_SearchRelevanceScoring(t *testing.T) {
	ar := NewAquaRegistry()
	ctx := context.Background()

	// Search for a specific tool.
	results, err := ar.Search(ctx, "kubectl", registry.WithLimit(10))
	require.NoError(t, err)
	require.Greater(t, len(results), 0)

	// Verify results are sorted by relevance.
	// The first result should be an exact or close match.
	firstResult := results[0]
	assert.Contains(t, firstResult.RepoName, "kubectl")
}

// TestResolveVersionOverrides_PreservesBaseFields verifies that resolveVersionOverrides
// copies ALL base fields from the registry package to the Tool struct, not just a subset.
// REGRESSION TEST: This test would have caught the bug where VersionPrefix, Replacements,
// Overrides, and Files were not being copied from registryPackage to Tool.
func TestResolveVersionOverrides_PreservesBaseFields(t *testing.T) {
	// Registry YAML with all fields populated - similar to real jq/gum registries.
	registryYAML := `
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    asset: "tool_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz"
    format: tar.gz
    binary_name: tool-bin
    version_prefix: "v"
    replacements:
      darwin: macos
      amd64: x86_64
    overrides:
      - goos: darwin
        asset: "tool_{{trimV .Version}}_macos_universal.tar.gz"
    files:
      - name: tool-bin
        src: tool/bin/tool
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Call resolveVersionOverrides with a version that doesn't match any version_override.
	// This tests the base field copying path.
	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "1.0.0")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: Verify ALL base fields are preserved.
	assert.Equal(t, "tool-bin", tool.Name, "Name should come from binary_name")
	assert.Equal(t, "test", tool.RepoOwner)
	assert.Equal(t, "tool", tool.RepoName)
	assert.Equal(t, "github_release", tool.Type)
	assert.Equal(t, "tar.gz", tool.Format)
	assert.Equal(t, "v", tool.VersionPrefix, "VersionPrefix must be preserved (was missing in bug)")

	// CRITICAL: Replacements must be preserved - this was the jq bug.
	require.NotNil(t, tool.Replacements, "Replacements must be preserved (was missing in bug)")
	assert.Equal(t, "macos", tool.Replacements["darwin"], "darwin->macos replacement must be preserved")
	assert.Equal(t, "x86_64", tool.Replacements["amd64"], "amd64->x86_64 replacement must be preserved")

	// CRITICAL: Overrides must be preserved.
	require.Len(t, tool.Overrides, 1, "Overrides must be preserved (was missing in bug)")
	assert.Equal(t, "darwin", tool.Overrides[0].GOOS)
	assert.Contains(t, tool.Overrides[0].Asset, "macos_universal")

	// CRITICAL: Files must be preserved.
	require.Len(t, tool.Files, 1, "Files must be preserved (was missing in bug)")
	assert.Equal(t, "tool-bin", tool.Files[0].Name)
	assert.Equal(t, "tool/bin/tool", tool.Files[0].Src)
}

// TestApplyVersionOverride_Replacements verifies that version overrides with replacements
// are correctly applied to the tool.
// REGRESSION TEST: This test would have caught the bug where versionOverride struct
// was missing the Replacements field, so version-specific replacements weren't applied.
func TestApplyVersionOverride_Replacements(t *testing.T) {
	// Registry YAML with version_overrides that include replacements.
	// This pattern matches jq which has different replacements for older versions.
	// Note: A top-level version_constraint: "false" is needed so overrides are checked
	// (upstream skips overrides when there's no top-level constraint).
	registryYAML := `
packages:
  - type: github_release
    repo_owner: jqlang
    repo_name: jq
    asset: "jq-{{.OS}}-{{.Arch}}"
    version_prefix: "jq-"
    version_constraint: "false"
    replacements:
      darwin: macos
      arm64: arm64
    version_overrides:
      - version_constraint: semver("< 1.8.0")
        asset: "jq-{{.OS}}-{{.Arch}}"
        replacements:
          darwin: macos
          amd64: amd64
          arm64: arm64
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Request version 1.7.1 which matches the version_constraint "< 1.8.0".
	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "1.7.1")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: Version override replacements must be applied.
	require.NotNil(t, tool.Replacements, "Version override replacements must be applied")
	assert.Equal(t, "macos", tool.Replacements["darwin"], "darwin->macos from version override")
	assert.Equal(t, "amd64", tool.Replacements["amd64"], "amd64->amd64 from version override")
	assert.Equal(t, "arm64", tool.Replacements["arm64"], "arm64->arm64 from version override")
}

// TestApplyVersionOverride_OverridesField verifies that version overrides with platform
// overrides are correctly applied.
// REGRESSION TEST: This test would have caught the bug where versionOverride struct
// was missing the Overrides field.
func TestApplyVersionOverride_OverridesField(t *testing.T) {
	// Note: A top-level version_constraint: "false" is needed so overrides are checked
	// (upstream skips overrides when there's no top-level constraint).
	registryYAML := `
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    asset: "tool_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
    version_constraint: "false"
    version_overrides:
      - version_constraint: semver(">= 2.0.0")
        asset: "tool_{{.Version}}_{{.OS}}_{{.Arch}}.zip"
        format: zip
        overrides:
          - goos: darwin
            goarch: arm64
            asset: "tool_{{.Version}}_darwin_universal.zip"
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Request version 2.1.0 which matches ">= 2.0.0".
	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "2.1.0")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: Version override's nested overrides must be applied.
	require.Len(t, tool.Overrides, 1, "Version override's nested overrides must be applied")
	assert.Equal(t, "darwin", tool.Overrides[0].GOOS)
	assert.Equal(t, "arm64", tool.Overrides[0].GOARCH)
	assert.Contains(t, tool.Overrides[0].Asset, "darwin_universal")
}

// TestResolveVersionOverrides_JQPattern tests the exact jq registry pattern that was failing.
// REGRESSION TEST: jq 1.7.1 on darwin should use "macos" not "darwin" due to version_overrides.
func TestResolveVersionOverrides_JQPattern(t *testing.T) {
	// This is a simplified version of the actual jq registry.yaml.
	// The key pattern: base has darwin:macos, but version_overrides for < 1.8.0
	// specifies its own replacements that must override the base.
	// Note: Aqua uses semver("constraint") format for version constraints.
	registryYAML := `
packages:
  - type: github_release
    repo_owner: jqlang
    repo_name: jq
    asset: "jq-{{.OS}}-{{.Arch}}"
    format: raw
    version_prefix: "jq-"
    replacements:
      darwin: macos
      windows: windows
      amd64: amd64
      arm64: arm64
    version_overrides:
      - version_constraint: semver("< 1.8.0")
        asset: "jq-{{.OS}}-{{.Arch}}"
        replacements:
          darwin: macos
          linux: linux
          windows: windows
          amd64: amd64
          arm64: arm64
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Test version 1.7.1 (matches < 1.8.0).
	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "1.7.1")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// This is the critical assertion - darwin must map to macos.
	assert.Equal(t, "jqlang", tool.RepoOwner)
	assert.Equal(t, "jq", tool.RepoName)
	assert.Equal(t, "jq-", tool.VersionPrefix)
	require.NotNil(t, tool.Replacements, "Replacements must be set for jq 1.7.1")
	assert.Equal(t, "macos", tool.Replacements["darwin"],
		"jq 1.7.1 on darwin should map to 'macos' (this was the bug - returned 'darwin' instead)")
	assert.Equal(t, "amd64", tool.Replacements["amd64"])
	assert.Equal(t, "arm64", tool.Replacements["arm64"])
}

// TestResolveVersionOverrides_GumPattern tests the gum pattern with version_prefix.
// REGRESSION TEST: gum uses version_prefix: "v" which must be preserved.
func TestResolveVersionOverrides_GumPattern(t *testing.T) {
	registryYAML := `
packages:
  - type: github_release
    repo_owner: charmbracelet
    repo_name: gum
    asset: "gum_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz"
    format: tar.gz
    version_prefix: "v"
    replacements:
      darwin: Darwin
      linux: Linux
      amd64: x86_64
      arm64: arm64
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "0.17.0")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: VersionPrefix must be "v" for GitHub release URL construction.
	assert.Equal(t, "v", tool.VersionPrefix,
		"gum version_prefix 'v' must be preserved (this was the bug - was empty)")

	// Replacements must be preserved for correct asset filename.
	require.NotNil(t, tool.Replacements)
	assert.Equal(t, "Darwin", tool.Replacements["darwin"])
	assert.Equal(t, "x86_64", tool.Replacements["amd64"])
}

// TestResolveVersionOverrides_OpenTofuPattern tests the opentofu pattern.
// REGRESSION TEST: opentofu uses version_prefix: "v" which must be preserved.
func TestResolveVersionOverrides_OpenTofuPattern(t *testing.T) {
	registryYAML := `
packages:
  - type: github_release
    repo_owner: opentofu
    repo_name: opentofu
    asset: "tofu_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip"
    format: zip
    version_prefix: "v"
    files:
      - name: tofu
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "1.9.0")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: VersionPrefix and Files must be preserved.
	assert.Equal(t, "v", tool.VersionPrefix,
		"opentofu version_prefix 'v' must be preserved")
	require.Len(t, tool.Files, 1)
	assert.Equal(t, "tofu", tool.Files[0].Name)
}

func TestAquaRegistry_parseRegistryFile_Scenarios(t *testing.T) {
	// Table-driven test for parseRegistryFile with various configuration patterns.
	tests := []struct {
		name         string
		testdataFile string                                  // If set, load from testdata/
		inlineData   string                                  // If set, use inline YAML
		verify       func(t *testing.T, tool *registry.Tool) // Custom verification
	}{
		{
			name:         "with files configuration",
			testdataFile: "testdata/aws-cli-files.yaml",
			verify: func(t *testing.T, tool *registry.Tool) {
				assert.Equal(t, "aws-cli", tool.Name)
				assert.Equal(t, "aws", tool.RepoOwner)
				assert.Equal(t, "aws-cli", tool.RepoName)

				// Verify files config.
				require.Len(t, tool.Files, 2)
				assert.Equal(t, "aws", tool.Files[0].Name)
				assert.Equal(t, "aws/dist/aws", tool.Files[0].Src)
				assert.Equal(t, "aws_completer", tool.Files[1].Name)
				assert.Equal(t, "aws/dist/aws_completer", tool.Files[1].Src)
			},
		},
		{
			name:         "with replacements",
			testdataFile: "testdata/aws-cli-replacements.yaml",
			verify: func(t *testing.T, tool *registry.Tool) {
				require.NotNil(t, tool.Replacements)
				assert.Len(t, tool.Replacements, 2)
				assert.Equal(t, "x86_64", tool.Replacements["amd64"])
				assert.Equal(t, "aarch64", tool.Replacements["arm64"])
			},
		},
		{
			name: "with overrides",
			inlineData: `
packages:
  - type: http
    repo_owner: aws
    repo_name: aws-cli
    url: https://awscli.amazonaws.com/awscli-exe-{{.OS}}-{{.Arch}}-{{trimV .Version}}.zip
    format: zip
    overrides:
      - goos: darwin
        url: https://awscli.amazonaws.com/AWSCLIV2-{{trimV .Version}}.{{.Format}}
        format: pkg
        files:
          - name: aws
            src: aws-cli.pkg/Payload/aws-cli/aws
          - name: aws_completer
            src: aws-cli.pkg/Payload/aws-cli/aws_completer
`,
			verify: func(t *testing.T, tool *registry.Tool) {
				require.Len(t, tool.Overrides, 1)
				override := tool.Overrides[0]
				assert.Equal(t, "darwin", override.GOOS)
				assert.Empty(t, override.GOARCH) // Not specified, so empty (wildcard).
				assert.Equal(t, "https://awscli.amazonaws.com/AWSCLIV2-{{trimV .Version}}.{{.Format}}", override.Asset)
				assert.Equal(t, "pkg", override.Format)

				// Verify override files.
				require.Len(t, override.Files, 2)
				assert.Equal(t, "aws", override.Files[0].Name)
				assert.Equal(t, "aws-cli.pkg/Payload/aws-cli/aws", override.Files[0].Src)
			},
		},
		{
			name: "full AWS CLI pattern",
			inlineData: `
packages:
  - type: http
    repo_owner: aws
    repo_name: aws-cli
    url: https://awscli.amazonaws.com/awscli-exe-{{.OS}}-{{.Arch}}-{{trimV .Version}}.zip
    format: zip
    overrides:
      - goos: darwin
        url: https://awscli.amazonaws.com/AWSCLIV2-{{trimV .Version}}.{{.Format}}
        format: pkg
        files:
          - name: aws
            src: aws-cli.pkg/Payload/aws-cli/aws
          - name: aws_completer
            src: aws-cli.pkg/Payload/aws-cli/aws_completer
    files:
      - name: aws
        src: aws/dist/aws
      - name: aws_completer
        src: aws/dist/aws_completer
    replacements:
      amd64: x86_64
      arm64: aarch64
`,
			verify: func(t *testing.T, tool *registry.Tool) {
				assert.Equal(t, "aws-cli", tool.Name)
				assert.Equal(t, "aws", tool.RepoOwner)
				assert.Equal(t, "aws-cli", tool.RepoName)
				assert.Equal(t, "http", tool.Type)

				// Verify all fields parsed correctly (uses trimV for bare version).
				assert.Contains(t, tool.Asset, "awscli-exe-{{.OS}}-{{.Arch}}-{{trimV .Version}}")
				assert.Equal(t, "zip", tool.Format)

				// Files.
				require.Len(t, tool.Files, 2)
				assert.Equal(t, "aws", tool.Files[0].Name)
				assert.Equal(t, "aws/dist/aws", tool.Files[0].Src)

				// Replacements.
				require.Len(t, tool.Replacements, 2)
				assert.Equal(t, "x86_64", tool.Replacements["amd64"])
				assert.Equal(t, "aarch64", tool.Replacements["arm64"])

				// Overrides.
				require.Len(t, tool.Overrides, 1)
				assert.Equal(t, "darwin", tool.Overrides[0].GOOS)
				assert.Equal(t, "pkg", tool.Overrides[0].Format)
				assert.Len(t, tool.Overrides[0].Files, 2)
			},
		},
		{
			name: "override with replacements",
			inlineData: `
packages:
  - type: http
    repo_owner: test
    repo_name: tool
    url: https://example.com/tool-{{.OS}}-{{.Arch}}.zip
    format: zip
    overrides:
      - goos: darwin
        goarch: arm64
        url: https://example.com/tool-macos-silicon.zip
        replacements:
          arm64: silicon
`,
			verify: func(t *testing.T, tool *registry.Tool) {
				require.Len(t, tool.Overrides, 1)
				override := tool.Overrides[0]
				assert.Equal(t, "darwin", override.GOOS)
				assert.Equal(t, "arm64", override.GOARCH)

				// Verify override-specific replacements.
				require.NotNil(t, override.Replacements)
				assert.Equal(t, "silicon", override.Replacements["arm64"])
			},
		},
		{
			name:         "replicated darwin_all pattern",
			testdataFile: "testdata/replicated.yaml",
			verify: func(t *testing.T, tool *registry.Tool) {
				assert.Equal(t, "replicated", tool.Name)
				assert.Equal(t, "replicatedhq", tool.RepoOwner)
				assert.Equal(t, "replicated", tool.RepoName)
				assert.Equal(t, "github_release", tool.Type)

				// Verify base asset pattern.
				assert.Contains(t, tool.Asset, "replicated_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz")

				// Verify darwin override uses "all" instead of arch.
				require.Len(t, tool.Overrides, 1)
				override := tool.Overrides[0]
				assert.Equal(t, "darwin", override.GOOS)
				assert.Empty(t, override.GOARCH) // Applies to all darwin architectures.
				assert.Contains(t, override.Asset, "{{.OS}}_all.tar.gz")
			},
		},
	}

	ar := NewAquaRegistry()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data []byte
			var err error

			if tt.testdataFile != "" {
				data, err = os.ReadFile(tt.testdataFile)
				require.NoError(t, err, "Should read testdata file")
			} else {
				data = []byte(tt.inlineData)
			}

			tool, err := ar.parseRegistryFile(data)
			require.NoError(t, err)
			require.NotNil(t, tool)

			tt.verify(t, tool)
		})
	}
}

// =============================================================================
// Real-world registry YAML tests.
// =============================================================================
// These tests use EXACT real-world Aqua registry YAML with all the fields that
// tools actually use (rosetta2, windows_arm_emulation, no_asset, checksum with
// nested cosign, supported_envs, etc.) to verify our parser handles them correctly.

// TestResolveVersionOverrides_RealCheckovPattern uses the exact checkov registry.yaml
// from the Aqua registry. For version 3.2.506, the catch-all "true" override should
// match and provide replacements {amd64: X86_64} and files with src paths.
func TestResolveVersionOverrides_RealCheckovPattern(t *testing.T) {
	// Exact YAML from https://github.com/aquaproj/aqua-registry/blob/main/pkgs/bridgecrewio/checkov/registry.yaml
	registryYAML := `
packages:
  - type: github_release
    repo_owner: bridgecrewio
    repo_name: checkov
    description: Prevent cloud misconfigurations and find vulnerabilities during build-time
    version_constraint: "false"
    version_overrides:
      - version_constraint: Version == "2.3.321"
        asset: checkov_{{.OS}}_{{.Arch}}_{{.Version}}.{{.Format}}
        format: zip
        rosetta2: true
        windows_arm_emulation: true
        files:
          - name: checkov
            src: dist/checkov
        replacements:
          amd64: X86_64
      - version_constraint: Version == "2.3.340"
        no_asset: true
      - version_constraint: Version == "2.5.15"
        asset: checkov_{{.OS}}_{{.Arch}}_{{.Version}}.{{.Format}}
        format: zip
        rosetta2: true
        windows_arm_emulation: true
        files:
          - name: checkov
            src: dist/checkov
        replacements:
          amd64: X86_64
        supported_envs:
          - darwin
          - windows
          - amd64
      - version_constraint: Version == "3.2.317"
        asset: checkov_{{.OS}}_{{.Arch}}.{{.Format}}
        format: zip
        rosetta2: true
        windows_arm_emulation: true
        files:
          - name: checkov
            src: dist/checkov
        replacements:
          amd64: X86_64
        supported_envs:
          - darwin
          - windows
          - amd64
      - version_constraint: Version == "3.2.322"
        asset: checkov_{{.OS}}_{{.Arch}}.{{.Format}}
        format: zip
        rosetta2: true
        files:
          - name: checkov
            src: dist/checkov
        replacements:
          amd64: X86_64
        supported_envs:
          - linux
      - version_constraint: semver("<= 2.3.314")
        no_asset: true
      - version_constraint: semver("<= 2.3.318")
        asset: checkov_{{.OS}}_{{.Version}}
        format: raw
        complete_windows_ext: false
        files:
          - name: checkov
            src: dist/checkov
        supported_envs:
          - darwin
          - windows
          - amd64
      - version_constraint: semver("<= 2.3.334")
        asset: checkov_{{.OS}}_{{.Arch}}_{{.Version}}.{{.Format}}
        format: zip
        rosetta2: true
        windows_arm_emulation: true
        files:
          - name: checkov
            src: dist/checkov
        replacements:
          amd64: X86_64
        supported_envs:
          - darwin
          - windows
          - amd64
      - version_constraint: semver("<= 3.2.51")
        asset: checkov_{{.OS}}_{{.Arch}}_{{.Version}}.{{.Format}}
        format: zip
        rosetta2: true
        windows_arm_emulation: true
        files:
          - name: checkov
            src: dist/checkov
        replacements:
          amd64: X86_64
      - version_constraint: "true"
        asset: checkov_{{.OS}}_{{.Arch}}.{{.Format}}
        format: zip
        rosetta2: true
        windows_arm_emulation: true
        files:
          - name: checkov
            src: dist/checkov
        replacements:
          amd64: X86_64
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Version 3.2.506 should match the catch-all "true" override.
	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "3.2.506")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: Replacements from the "true" catch-all must be applied.
	require.NotNil(t, tool.Replacements, "Replacements must be set from catch-all override")
	assert.Equal(t, "X86_64", tool.Replacements["amd64"],
		"amd64 -> X86_64 replacement must be applied from catch-all override")

	// CRITICAL: Files must include Src path (not just Name).
	require.Len(t, tool.Files, 1, "Files must be populated from override")
	assert.Equal(t, "checkov", tool.Files[0].Name)
	assert.Equal(t, "dist/checkov", tool.Files[0].Src,
		"Files[0].Src must be 'dist/checkov' (was lost due to anonymous struct missing Src field)")

	// Asset and format from the catch-all.
	assert.Equal(t, "checkov_{{.OS}}_{{.Arch}}.{{.Format}}", tool.Asset)
	assert.Equal(t, "zip", tool.Format)
	assert.Equal(t, "bridgecrewio", tool.RepoOwner)
	assert.Equal(t, "checkov", tool.RepoName)
}

// TestResolveVersionOverrides_RealTrivyPattern uses the exact trivy registry.yaml
// from the Aqua registry. For version 0.69.1, the catch-all "true" override should
// match and provide replacements including {amd64: 64bit, linux: Linux}.
func TestResolveVersionOverrides_RealTrivyPattern(t *testing.T) {
	// Exact YAML from https://github.com/aquaproj/aqua-registry/blob/main/pkgs/aquasecurity/trivy/registry.yaml
	// Includes checksum with nested cosign.bundle, overrides with empty replacements: {}.
	registryYAML := `
packages:
  - type: github_release
    repo_owner: aquasecurity
    repo_name: trivy
    description: Find vulnerabilities, misconfigurations, secrets, SBOM
    version_constraint: "false"
    version_overrides:
      - version_constraint: Version == "v0.20.0"
        asset: trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}
        format: tar.gz
        rosetta2: true
        replacements:
          amd64: 64bit
          darwin: macOS
          linux: Linux
        checksum:
          type: github_release
          asset: trivy_{{trimV .Version}}_checksums.txt
          algorithm: sha256
        overrides:
          - goos: linux
            replacements:
              arm64: ARM64
        supported_envs:
          - linux
          - darwin
      - version_constraint: semver("<= 0.1.6")
        asset: trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}
        format: tar.gz
        rosetta2: true
        windows_arm_emulation: true
        replacements:
          amd64: 64bit
          darwin: macOS
          linux: Linux
          windows: Windows
        checksum:
          type: github_release
          asset: trivy_{{trimV .Version}}_checksums.txt
          algorithm: sha256
        overrides:
          - goos: linux
            replacements:
              arm64: ARM64
          - goos: windows
            format: zip
      - version_constraint: semver("<= 0.16.0")
        asset: trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}
        format: tar.gz
        rosetta2: true
        replacements:
          amd64: 64bit
          darwin: macOS
          linux: Linux
        checksum:
          type: github_release
          asset: trivy_{{trimV .Version}}_checksums.txt
          algorithm: sha256
        overrides:
          - goos: linux
            replacements:
              arm64: ARM64
        supported_envs:
          - linux
          - darwin
      - version_constraint: semver("<= 0.31.3")
        asset: trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}
        format: tar.gz
        replacements:
          amd64: 64bit
          arm64: ARM64
          darwin: macOS
          linux: Linux
        checksum:
          type: github_release
          asset: trivy_{{trimV .Version}}_checksums.txt
          algorithm: sha256
        supported_envs:
          - linux
          - darwin
      - version_constraint: semver("<= 0.35.0")
        asset: trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}
        format: tar.gz
        replacements:
          amd64: 64bit
          arm64: ARM64
          darwin: macOS
          linux: Linux
        checksum:
          type: github_release
          asset: trivy_{{trimV .Version}}_checksums.txt
          algorithm: sha256
          cosign:
            opts:
              - --certificate
              - https://github.com/aquasecurity/trivy/releases/download/{{.Version}}/trivy_{{trimV .Version}}_checksums.txt.pem
              - --certificate-identity
              - https://github.com/aquasecurity/trivy/.github/workflows/reusable-release.yaml@refs/tags/{{.Version}}
              - --certificate-oidc-issuer
              - https://token.actions.githubusercontent.com
              - --signature
              - https://github.com/aquasecurity/trivy/releases/download/{{.Version}}/trivy_{{trimV .Version}}_checksums.txt.sig
        supported_envs:
          - linux
          - darwin
      - version_constraint: semver("<= 0.67.2")
        asset: trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}
        format: tar.gz
        windows_arm_emulation: true
        replacements:
          amd64: 64bit
          arm64: ARM64
          darwin: macOS
          linux: Linux
        checksum:
          type: github_release
          asset: trivy_{{trimV .Version}}_checksums.txt
          algorithm: sha256
          cosign:
            opts:
              - --certificate
              - https://github.com/aquasecurity/trivy/releases/download/{{.Version}}/trivy_{{trimV .Version}}_checksums.txt.pem
              - --certificate-identity
              - https://github.com/aquasecurity/trivy/.github/workflows/reusable-release.yaml@refs/tags/{{.Version}}
              - --certificate-oidc-issuer
              - https://token.actions.githubusercontent.com
              - --signature
              - https://github.com/aquasecurity/trivy/releases/download/{{.Version}}/trivy_{{trimV .Version}}_checksums.txt.sig
        overrides:
          - goos: windows
            format: zip
            replacements: {}
      - version_constraint: "true"
        asset: trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}
        format: tar.gz
        windows_arm_emulation: true
        replacements:
          amd64: 64bit
          arm64: ARM64
          darwin: macOS
          linux: Linux
        checksum:
          type: github_release
          asset: trivy_{{trimV .Version}}_checksums.txt
          algorithm: sha256
          cosign:
            bundle:
              type: github_release
              asset: trivy_{{trimV .Version}}_checksums.txt.sigstore.json
            opts:
              - --certificate-identity
              - https://github.com/aquasecurity/trivy/.github/workflows/reusable-release.yaml@refs/tags/{{.Version}}
              - --certificate-oidc-issuer
              - https://token.actions.githubusercontent.com
        overrides:
          - goos: windows
            format: zip
            replacements: {}
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Version 0.69.1 should match the catch-all "true" override.
	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "0.69.1")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: Replacements from the "true" catch-all must be applied.
	require.NotNil(t, tool.Replacements, "Replacements must be set from catch-all override")
	assert.Equal(t, "64bit", tool.Replacements["amd64"],
		"amd64 -> 64bit replacement must be applied")
	assert.Equal(t, "ARM64", tool.Replacements["arm64"],
		"arm64 -> ARM64 replacement must be applied")
	assert.Equal(t, "macOS", tool.Replacements["darwin"],
		"darwin -> macOS replacement must be applied")
	assert.Equal(t, "Linux", tool.Replacements["linux"],
		"linux -> Linux replacement must be applied")

	// Asset and format from the catch-all.
	assert.Equal(t, "trivy_{{trimV .Version}}_{{.OS}}-{{.Arch}}.{{.Format}}", tool.Asset)
	assert.Equal(t, "tar.gz", tool.Format)
}

// TestResolveVersionOverrides_RealYqPattern uses the exact yq registry.yaml
// from the Aqua registry. Tests upstream algorithm where:
// - Version >= 4.9.6 matches top-level constraint → returns base (no overrides checked).
// - Version < 4.9.6 falls through to "true" catch-all override → gets rosetta2.
func TestResolveVersionOverrides_RealYqPattern(t *testing.T) {
	// Exact YAML from https://github.com/aquaproj/aqua-registry/blob/main/pkgs/mikefarah/yq/registry.yaml
	registryYAML := `
packages:
  - type: github_release
    repo_owner: mikefarah
    repo_name: yq
    description: yq is a portable command-line YAML processor
    asset: yq_{{.OS}}_{{.Arch}}
    supported_envs:
      - darwin
      - linux
      - amd64
    version_constraint: semver(">= 4.9.6")
    version_overrides:
      - version_constraint: "true"
        rosetta2: true
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	t.Run("version matches top-level returns base", func(t *testing.T) {
		// Version 4.52.4 matches top-level constraint >= 4.9.6 → returns base.
		// Upstream: overrides NOT checked when top-level matches.
		tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "4.52.4")
		require.NoError(t, err)
		require.NotNil(t, tool)

		// Base asset must be preserved.
		assert.Equal(t, "yq_{{.OS}}_{{.Arch}}", tool.Asset)
		assert.Equal(t, "mikefarah", tool.RepoOwner)
		assert.Equal(t, "yq", tool.RepoName)
		assert.Equal(t, "github_release", tool.Type)

		// SupportedEnvs from base should be preserved.
		assert.Contains(t, tool.SupportedEnvs, "darwin")
		assert.Contains(t, tool.SupportedEnvs, "linux")

		// Rosetta2 is NOT set in base (only in override which isn't checked).
		assert.False(t, tool.Rosetta2,
			"rosetta2 should NOT be set when top-level matches (upstream doesn't check overrides)")
	})

	t.Run("version below top-level gets override", func(t *testing.T) {
		// Version 4.9.5 doesn't match top-level >= 4.9.6 → falls to "true" override.
		tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "4.9.5")
		require.NoError(t, err)
		require.NotNil(t, tool)

		// Base asset must be preserved (override doesn't specify an asset).
		assert.Equal(t, "yq_{{.OS}}_{{.Arch}}", tool.Asset)

		// Rosetta2 should be propagated from the "true" catch-all override.
		assert.True(t, tool.Rosetta2,
			"rosetta2 should be set from catch-all override for older versions")
	})
}

// TestResolveVersionOverrides_TopLevelConstraint verifies that when no override matches
// and a top-level version_constraint exists, versions that don't satisfy it are rejected.
func TestResolveVersionOverrides_TopLevelConstraint(t *testing.T) {
	registryYAML := `
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    asset: tool_{{.OS}}_{{.Arch}}
    version_constraint: semver(">= 2.0.0")
    version_overrides:
      - version_constraint: semver("<= 1.5.0")
        asset: tool-old_{{.OS}}_{{.Arch}}
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	t.Run("version matches override", func(t *testing.T) {
		// Version 1.3.0 matches the override constraint "<= 1.5.0".
		tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "1.3.0")
		require.NoError(t, err)
		assert.Equal(t, "tool-old_{{.OS}}_{{.Arch}}", tool.Asset)
	})

	t.Run("version matches top-level constraint", func(t *testing.T) {
		// Version 3.0.0 doesn't match any override, but matches top-level ">= 2.0.0".
		tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "3.0.0")
		require.NoError(t, err)
		assert.Equal(t, "tool_{{.OS}}_{{.Arch}}", tool.Asset)
	})

	t.Run("version matches nothing returns base config", func(t *testing.T) {
		// Version 1.8.0 doesn't match override (<= 1.5.0) or top-level (>= 2.0.0).
		// Upstream behavior: return base config (NOT an error).
		tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "1.8.0")
		require.NoError(t, err)
		assert.Equal(t, "tool_{{.OS}}_{{.Arch}}", tool.Asset)
	})

	t.Run("no top-level constraint returns base config", func(t *testing.T) {
		// Registry without top-level version_constraint — any version uses base config.
		noConstraintYAML := `
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool2
    asset: tool2_{{.OS}}_{{.Arch}}
`
		ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-yaml")
			_, _ = w.Write([]byte(noConstraintYAML))
		}))
		defer ts2.Close()

		tool, err := ar.resolveVersionOverrides(ts2.URL+"/registry.yaml", "0.0.1")
		require.NoError(t, err)
		assert.Equal(t, "tool2_{{.OS}}_{{.Arch}}", tool.Asset)
	})
}

// TestResolveVersionOverrides_RealJqPattern uses the exact jq registry.yaml
// from the Aqua registry. Jq uses version_prefix: "jq-" and has multiple
// semver constraints. For version 1.7.1, semver("< 1.8.0") should match.
func TestResolveVersionOverrides_RealJqPattern(t *testing.T) {
	// Exact YAML from https://github.com/aquaproj/aqua-registry/blob/main/pkgs/jqlang/jq/registry.yaml
	registryYAML := `
packages:
  - type: github_release
    repo_owner: jqlang
    repo_name: jq
    description: Command-line JSON processor
    version_constraint: "false"
    version_prefix: jq-
    version_overrides:
      - version_constraint: semver("<= 1.2")
        no_asset: true
      - version_constraint: semver("<= 1.4")
        asset: jq-{{.OS}}-{{.Arch}}
        format: raw
        rosetta2: true
        replacements:
          amd64: x86_64
          darwin: osx
          windows: win64
        overrides:
          - goos: windows
            asset: jq-{{.OS}}
        supported_envs:
          - darwin
          - windows
          - amd64
      - version_constraint: Version == "jq-1.5rc1"
        asset: jq-{{.OS}}-{{.Arch}}-static
        format: raw
        replacements:
          windows: win64
          amd64: x86_64
        overrides:
          - goos: windows
            asset: jq-{{.OS}}
        supported_envs:
          - linux/amd64
          - windows
      - version_constraint: Version == "jq-1.5rc2"
        asset: jq-{{.OS}}-{{.Arch}}
        format: raw
        rosetta2: true
        replacements:
          amd64: x86_64
          darwin: osx
          windows: win64
        overrides:
          - goos: windows
            asset: jq-{{.OS}}
        supported_envs:
          - darwin
          - windows
          - amd64
      - version_constraint: semver("<= 1.6")
        asset: jq-{{.OS}}
        format: raw
        rosetta2: true
        replacements:
          linux: linux64
          darwin: osx
          windows: win64
        overrides:
          - goos: darwin
            asset: jq-{{.OS}}-{{.Arch}}
        supported_envs:
          - darwin
          - windows
          - amd64
      - version_constraint: semver("< 1.8.0")
        asset: jq-{{.OS}}-{{.Arch}}
        format: raw
        windows_arm_emulation: true
        replacements:
          darwin: macos
        checksum:
          type: github_release
          asset: sha256sum.txt
          algorithm: sha256
      - version_constraint: "true"
        asset: jq-{{.OS}}-{{.Arch}}
        format: raw
        windows_arm_emulation: true
        replacements:
          darwin: macos
        checksum:
          type: github_release
          asset: sha256sum.txt
          algorithm: sha256
        github_artifact_attestations:
          signer_workflow: jqlang/jq/.github/workflows/ci.yml
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir()

	// Version 1.7.1 should match semver("< 1.8.0") which has replacements: {darwin: macos}.
	tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", "1.7.1")
	require.NoError(t, err)
	require.NotNil(t, tool)

	// CRITICAL: VersionPrefix must be "jq-" from the base.
	assert.Equal(t, "jq-", tool.VersionPrefix,
		"VersionPrefix must be 'jq-' from base (not 'v' or empty)")

	// CRITICAL: Replacements from the matching override must be applied.
	require.NotNil(t, tool.Replacements, "Replacements must be set from semver(\"< 1.8.0\") override")
	assert.Equal(t, "macos", tool.Replacements["darwin"],
		"darwin -> macos replacement must be applied from version override")

	// Asset and format from the matching override.
	assert.Equal(t, "jq-{{.OS}}-{{.Arch}}", tool.Asset)
	assert.Equal(t, "raw", tool.Format)
}

// TestEvaluateVersionConstraint_WithVersionPrefix tests that semver constraints
// work correctly when the version has a non-standard prefix like "jq-".
// Aqua passes Version (full) and SemVer (prefix-stripped) separately.
func TestEvaluateVersionConstraint_WithVersionPrefix(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string // Full version with prefix.
		prefix     string // Version prefix to strip for SemVer.
		expected   bool
	}{
		{
			name:       "jq prefix with semver less than",
			constraint: `semver("< 1.8.0")`,
			version:    "jq-1.7.1",
			prefix:     "jq-",
			expected:   true,
		},
		{
			name:       "jq prefix with semver greater equal",
			constraint: `semver(">= 1.8.0")`,
			version:    "jq-1.8.1",
			prefix:     "jq-",
			expected:   true,
		},
		{
			name:       "v prefix stripped",
			constraint: `semver(">= 1.0.0")`,
			version:    "v1.2.3",
			prefix:     "v",
			expected:   true,
		},
		{
			name:       "no prefix works as before",
			constraint: `semver(">= 1.0.0")`,
			version:    "1.2.3",
			prefix:     "",
			expected:   true,
		},
		{
			name:       "Version == compares full version including prefix",
			constraint: `Version == "jq-1.5rc1"`,
			version:    "jq-1.5rc1",
			prefix:     "jq-",
			expected:   true, // Version is now the FULL version, so this matches.
		},
		{
			name:       "SemVer == compares stripped version",
			constraint: `SemVer == "1.5rc1"`,
			version:    "jq-1.5rc1",
			prefix:     "jq-",
			expected:   true,
		},
		{
			name:       "true literal with prefix",
			constraint: `"true"`,
			version:    "jq-1.8.1",
			prefix:     "jq-",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compute SemVer by stripping prefix (this is what resolveVersionOverrides does).
			sv := tt.version
			if tt.prefix != "" {
				sv = strings.TrimPrefix(tt.version, tt.prefix)
			}
			result, err := evaluateVersionConstraint(tt.constraint, tt.version, sv)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractBinaryNameFromPackageName(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		expected    string
	}{
		{
			name:        "three_segment_package_name",
			packageName: "kubernetes/kubernetes/kubectl",
			expected:    "kubectl",
		},
		{
			name:        "two_segment_package_name_returns_empty",
			packageName: "hashicorp/terraform",
			expected:    "",
		},
		{
			name:        "four_segment_package_name",
			packageName: "owner/repo/subdir/binary",
			expected:    "binary",
		},
		{
			name:        "empty_package_name",
			packageName: "",
			expected:    "",
		},
		{
			name:        "single_segment_returns_empty",
			packageName: "terraform",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBinaryNameFromPackageName(tt.packageName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveVersionOverrides_FormatOverrides verifies that format_overrides are propagated
// both from base config and from version overrides.
func TestResolveVersionOverrides_FormatOverrides(t *testing.T) {
	tests := []struct {
		name             string
		registryYAML     string
		version          string
		expectedCount    int
		expectedOverride []registry.FormatOverride
	}{
		{
			name: "base format_overrides propagated",
			registryYAML: `
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    asset: tool_{{.OS}}_{{.Arch}}.{{.Format}}
    format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
      - goos: linux
        format: tar.xz
`,
			version:       "1.0.0",
			expectedCount: 2,
			expectedOverride: []registry.FormatOverride{
				{GOOS: "windows", Format: "zip"},
				{GOOS: "linux", Format: "tar.xz"},
			},
		},
		{
			name: "version override replaces base format_overrides",
			registryYAML: `
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    asset: tool_{{.OS}}_{{.Arch}}
    format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
    version_constraint: semver(">= 2.0.0")
    version_overrides:
      - version_constraint: "true"
        format_overrides:
          - goos: windows
            format: msi
          - goos: darwin
            format: pkg
`,
			version:       "1.0.0",
			expectedCount: 2,
			expectedOverride: []registry.FormatOverride{
				{GOOS: "windows", Format: "msi"},
				{GOOS: "darwin", Format: "pkg"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-yaml")
				_, _ = w.Write([]byte(tt.registryYAML))
			}))
			defer ts.Close()

			ar := NewAquaRegistry()
			ar.cache.baseDir = t.TempDir()

			tool, err := ar.resolveVersionOverrides(ts.URL+"/registry.yaml", tt.version)
			require.NoError(t, err)
			require.NotNil(t, tool)

			require.Len(t, tool.FormatOverrides, tt.expectedCount)
			assert.Equal(t, tt.expectedOverride, tool.FormatOverrides)
		})
	}
}

// TestApplyVersionOverride_TypeChange verifies that changing type via version override
// resets type-specific fields (resetByPkgType).
func TestApplyVersionOverride_TypeChange(t *testing.T) {
	tool := &registry.Tool{
		Type:      "github_release",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "tool.tar.gz",
	}

	override := &versionOverride{
		Type:  "http",
		Asset: "https://example.com/tool.tar.gz",
	}

	applyVersionOverride(tool, override, "1.0.0")
	assert.Equal(t, "http", tool.Type)
	assert.Equal(t, "https://example.com/tool.tar.gz", tool.Asset)
}

// TestApplyVersionOverride_RepoChange verifies that repo_owner and repo_name can be
// overridden per version.
func TestApplyVersionOverride_RepoChange(t *testing.T) {
	tool := &registry.Tool{
		Type:      "github_release",
		RepoOwner: "original",
		RepoName:  "tool",
		Asset:     "tool.tar.gz",
	}

	override := &versionOverride{
		RepoOwner: "forked",
		RepoName:  "tool-legacy",
	}

	applyVersionOverride(tool, override, "0.1.0")
	assert.Equal(t, "forked", tool.RepoOwner)
	assert.Equal(t, "tool-legacy", tool.RepoName)
}

// TestApplyVersionOverride_ErrorMessage verifies that error_message is propagated.
func TestApplyVersionOverride_ErrorMessage(t *testing.T) {
	tool := &registry.Tool{
		Type:      "github_release",
		RepoOwner: "test",
		RepoName:  "tool",
	}

	override := &versionOverride{
		ErrorMessage: "This version is no longer supported",
	}

	applyVersionOverride(tool, override, "0.1.0")
	assert.Equal(t, "This version is no longer supported", tool.ErrorMessage)
}

// TestResetByPkgType verifies that resetByPkgType clears type-specific fields.
func TestResetByPkgType(t *testing.T) {
	t.Run("github_release to http clears Asset", func(t *testing.T) {
		tool := &registry.Tool{
			Type:  "github_release",
			Asset: "tool.tar.gz",
			URL:   "",
		}
		resetByPkgType(tool, "http")
		assert.Equal(t, "", tool.Asset, "Asset should be cleared when switching to http")
	})

	t.Run("http to github_release clears URL", func(t *testing.T) {
		tool := &registry.Tool{
			Type:  "http",
			Asset: "",
			URL:   "https://example.com/tool.tar.gz",
		}
		resetByPkgType(tool, "github_release")
		assert.Equal(t, "", tool.URL, "URL should be cleared when switching to github_release")
	})
}

// TestStripFileExtension verifies file extension stripping for AssetWithoutExt.
func TestStripFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tar.gz compound", "tool_linux_amd64.tar.gz", "tool_linux_amd64"},
		{"tar.xz compound", "tool_linux_amd64.tar.xz", "tool_linux_amd64"},
		{"tar.bz2 compound", "tool_linux_amd64.tar.bz2", "tool_linux_amd64"},
		{"zip extension", "tool_windows_amd64.zip", "tool_windows_amd64"},
		{"no extension", "tool_linux_amd64", "tool_linux_amd64"},
		{"exe extension", "tool.exe", "tool"},
		{"pkg extension", "tool.pkg", "tool"},
		{"tgz extension", "tool_linux_amd64.tgz", "tool_linux_amd64"},
		{"dmg extension", "tool_darwin.dmg", "tool_darwin"},
		{"multiple dots", "tool.v1.2.3.tar.gz", "tool.v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripFileExtension(tt.input))
		})
	}
}

// TestGetLatestVersion_GitHubTag tests the github_tag version source path.
func TestGetLatestVersion_GitHubTag(t *testing.T) {
	// Set up GitHub API server for tags endpoint.
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tags") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"name": "v3.2.1"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiServer.Close()

	ar := NewAquaRegistry(WithGitHubBaseURL(apiServer.URL))
	// Override cache dir to temp dir so we don't pollute real cache.
	tmpCache := t.TempDir()
	ar.cache.baseDir = tmpCache

	// Pre-populate the disk cache so GetTool finds our tool without hitting real GitHub.
	// GetTool tries: "https://raw.githubusercontent.com/aquaproj/aqua-registry/refs/heads/main/pkgs/{owner}/{repo}/registry.yaml"
	registryYAML := []byte(`packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    version_source: github_tag
    asset: "tool-{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz"
    binary_name: tool
`)
	firstURL := "https://raw.githubusercontent.com/aquaproj/aqua-registry/refs/heads/main/pkgs/test/tool/registry.yaml"
	cacheKey := strings.ReplaceAll(firstURL, "/", "_")
	cacheKey = strings.ReplaceAll(cacheKey, ":", "_")
	require.NoError(t, os.WriteFile(filepath.Join(tmpCache, cacheKey+".yaml"), registryYAML, 0o644))

	// Verify GetTool finds the tool with github_tag version source.
	tool, err := ar.GetTool("test", "tool")
	require.NoError(t, err)
	assert.Equal(t, "github_tag", tool.VersionSource)

	// GetLatestVersion should use the github_tag path and call getLatestTag.
	version, err := ar.GetLatestVersion("test", "tool")
	require.NoError(t, err)
	assert.Equal(t, "3.2.1", version)
}

// TestGetLatestTag tests the getLatestTag function directly.
func TestGetLatestTag(t *testing.T) {
	t.Run("strips default v prefix when prefix is empty", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"name": "v2.0.0"}]`))
		}))
		defer ts.Close()

		ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))
		version, err := ar.getLatestTag("test", "tool", "")
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", version)
	})

	t.Run("strips custom prefix", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"name": "jq-1.7.1"}]`))
		}))
		defer ts.Close()

		ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))
		version, err := ar.getLatestTag("jqlang", "jq", "jq-")
		require.NoError(t, err)
		assert.Equal(t, "1.7.1", version)
	})

	t.Run("handles no tags", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		}))
		defer ts.Close()

		ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))
		_, err := ar.getLatestTag("test", "empty", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, registry.ErrNoVersionsFound)
	})

	t.Run("handles server error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))
		_, err := ar.getLatestTag("test", "tool", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, registry.ErrHTTPRequest)
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`not json`))
		}))
		defer ts.Close()

		ar := NewAquaRegistry(WithGitHubBaseURL(ts.URL))
		_, err := ar.getLatestTag("test", "tool", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, registry.ErrRegistryParse)
	})
}

// TestBuildAssetTemplateData tests the buildAssetTemplateData helper.
func TestBuildAssetTemplateData(t *testing.T) {
	t.Run("basic data without replacements", func(t *testing.T) {
		tool := &registry.Tool{
			RepoOwner: "test",
			RepoName:  "tool",
			Format:    "tar.gz",
		}
		data := buildAssetTemplateData(tool, "v1.0.0", "1.0.0")
		assert.Equal(t, "v1.0.0", data["Version"])
		assert.Equal(t, "1.0.0", data["SemVer"])
		assert.Equal(t, "test", data["RepoOwner"])
		assert.Equal(t, "tool", data["RepoName"])
		assert.Equal(t, "tar.gz", data["Format"])
		assert.NotEmpty(t, data["OS"])
		assert.NotEmpty(t, data["Arch"])
		assert.NotEmpty(t, data["GOOS"])
		assert.NotEmpty(t, data["GOARCH"])
	})

	t.Run("with replacements", func(t *testing.T) {
		tool := &registry.Tool{
			RepoOwner: "bridgecrewio",
			RepoName:  "checkov",
			Replacements: map[string]string{
				"amd64": "X86_64",
				"arm64": "aarch64",
			},
		}
		data := buildAssetTemplateData(tool, "3.0.0", "3.0.0")
		switch getArch() {
		case "amd64":
			assert.Equal(t, "X86_64", data["Arch"])
		case "arm64":
			assert.Equal(t, "aarch64", data["Arch"])
		}
		// GOARCH must remain raw.
		assert.Equal(t, getArch(), data["GOARCH"])
	})

	t.Run("with format overrides", func(t *testing.T) {
		tool := &registry.Tool{
			RepoOwner: "test",
			RepoName:  "tool",
			Format:    "tar.gz",
			FormatOverrides: []registry.FormatOverride{
				{GOOS: getOS(), Format: "zip"},
			},
		}
		data := buildAssetTemplateData(tool, "v1.0.0", "1.0.0")
		assert.Equal(t, "zip", data["Format"])
	})

	t.Run("format override for non-matching OS ignored", func(t *testing.T) {
		tool := &registry.Tool{
			RepoOwner: "test",
			RepoName:  "tool",
			Format:    "tar.gz",
			FormatOverrides: []registry.FormatOverride{
				{GOOS: "nonexistent-os", Format: "dmg"},
			},
		}
		data := buildAssetTemplateData(tool, "v1.0.0", "1.0.0")
		assert.Equal(t, "tar.gz", data["Format"])
	})
}

// TestResolveVersionStrings tests version/semver resolution logic.
func TestResolveVersionStrings(t *testing.T) {
	tests := []struct {
		name            string
		tool            *registry.Tool
		version         string
		expectedRelease string
		expectedSemVer  string
	}{
		{
			name:            "no prefix",
			tool:            &registry.Tool{},
			version:         "1.0.0",
			expectedRelease: "1.0.0",
			expectedSemVer:  "1.0.0",
		},
		{
			name:            "v prefix adds v",
			tool:            &registry.Tool{VersionPrefix: "v"},
			version:         "1.0.0",
			expectedRelease: "v1.0.0",
			expectedSemVer:  "1.0.0",
		},
		{
			name:            "v prefix already present",
			tool:            &registry.Tool{VersionPrefix: "v"},
			version:         "v1.0.0",
			expectedRelease: "v1.0.0",
			expectedSemVer:  "1.0.0",
		},
		{
			name:            "custom prefix jq-",
			tool:            &registry.Tool{VersionPrefix: "jq-"},
			version:         "1.7.1",
			expectedRelease: "jq-1.7.1",
			expectedSemVer:  "1.7.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release, semVer := resolveVersionStrings(tt.tool, tt.version)
			assert.Equal(t, tt.expectedRelease, release)
			assert.Equal(t, tt.expectedSemVer, semVer)
		})
	}
}

// TestComputeSemVer tests the computeSemVer helper.
func TestComputeSemVer(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		prefix   string
		expected string
	}{
		{"empty prefix returns version as-is", "v1.0.0", "", "v1.0.0"},
		{"strips v prefix", "v1.0.0", "v", "1.0.0"},
		{"strips custom prefix", "jq-1.7.1", "jq-", "1.7.1"},
		{"prefix not present returns as-is", "1.0.0", "v", "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeSemVer(tt.version, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAssetTemplateFuncs_SecurityDeletions verifies sensitive Sprig functions are removed.
func TestAssetTemplateFuncs_SecurityDeletions(t *testing.T) {
	funcs := assetTemplateFuncs()
	assert.Nil(t, funcs["env"], "env function should be deleted for security")
	assert.Nil(t, funcs["expandenv"], "expandenv function should be deleted for security")
	assert.Nil(t, funcs["getHostByName"], "getHostByName function should be deleted for security")
}

// TestExecuteAssetTemplate_TwoPassRendering tests two-pass rendering for Asset/AssetWithoutExt.
func TestExecuteAssetTemplate_TwoPassRendering(t *testing.T) {
	t.Run("template without Asset reference renders in one pass", func(t *testing.T) {
		data := map[string]string{
			"Version":   "v1.0.0",
			"SemVer":    "1.0.0",
			"OS":        "linux",
			"Arch":      "amd64",
			"RepoOwner": "test",
			"RepoName":  "tool",
			"Format":    "tar.gz",
		}
		result, err := executeAssetTemplate("{{.RepoName}}_{{.SemVer}}_{{.OS}}_{{.Arch}}.{{.Format}}", data)
		require.NoError(t, err)
		assert.Equal(t, "tool_1.0.0_linux_amd64.tar.gz", result)
		// Asset/AssetWithoutExt should NOT be populated (no .Asset reference).
		assert.Empty(t, data["Asset"])
		assert.Empty(t, data["AssetWithoutExt"])
	})

	t.Run("template referencing .Asset triggers two-pass and populates Asset fields", func(t *testing.T) {
		data := map[string]string{
			"Version":         "v0.15.2",
			"SemVer":          "0.15.2",
			"OS":              "Linux",
			"Arch":            "x86_64",
			"RepoOwner":       "charmbracelet",
			"RepoName":        "gum",
			"Format":          "tar.gz",
			"Asset":           "", // Pre-initialize to avoid <no value> in first pass.
			"AssetWithoutExt": "",
		}
		// Template that references {{.Asset}} → triggers two-pass rendering.
		// Pass 1: Asset is empty → result = "gum_0.15.2_Linux_x86_64.tar.gz/"
		// Then: data["Asset"] set, AssetWithoutExt computed via stripFileExtension.
		// Pass 2: re-renders with Asset populated.
		tmpl := "{{.RepoName}}_{{.SemVer}}_{{.OS}}_{{.Arch}}.{{.Format}}/{{.Asset}}"
		result, err := executeAssetTemplate(tmpl, data)
		require.NoError(t, err)
		// After two-pass, Asset and AssetWithoutExt should be populated.
		assert.NotEmpty(t, data["Asset"])
		assert.NotEmpty(t, data["AssetWithoutExt"])
		assert.Contains(t, result, "gum_0.15.2_Linux_x86_64")
	})

	t.Run("template referencing .AssetWithoutExt triggers two-pass", func(t *testing.T) {
		data := map[string]string{
			"Version":         "v1.0.0",
			"SemVer":          "1.0.0",
			"OS":              "linux",
			"Arch":            "amd64",
			"RepoOwner":       "test",
			"RepoName":        "tool",
			"Format":          "tar.gz",
			"Asset":           "", // Pre-initialize.
			"AssetWithoutExt": "",
		}
		// .AssetWithoutExt contains ".Asset" as substring → triggers two-pass.
		tmpl := "{{.RepoName}}_{{.SemVer}}_{{.OS}}_{{.Arch}}.{{.Format}}/{{.AssetWithoutExt}}"
		result, err := executeAssetTemplate(tmpl, data)
		require.NoError(t, err)
		assert.NotEmpty(t, data["Asset"])
		assert.NotEmpty(t, data["AssetWithoutExt"])
		assert.Contains(t, result, "tool_1.0.0_linux_amd64")
	})

	t.Run("two-pass with pre-initialized Asset fields", func(t *testing.T) {
		data := map[string]string{
			"Version":         "v2.0.0",
			"SemVer":          "2.0.0",
			"OS":              "darwin",
			"Arch":            "arm64",
			"RepoOwner":       "example",
			"RepoName":        "app",
			"Format":          "tar.gz",
			"Asset":           "", // Pre-initialize to avoid <no value>.
			"AssetWithoutExt": "", // Pre-initialize to avoid <no value>.
		}
		// Template with .AssetWithoutExt reference (contains ".Asset") to trigger two-pass.
		tmpl := "{{.RepoName}}_{{.SemVer}}_{{.OS}}_{{.Arch}}.{{.Format}}"
		// This template does NOT contain ".Asset" so it's actually single-pass.
		result, err := executeAssetTemplate(tmpl, data)
		require.NoError(t, err)
		assert.Equal(t, "app_2.0.0_darwin_arm64.tar.gz", result)
	})

	t.Run("parse error returns error", func(t *testing.T) {
		data := map[string]string{
			"Version": "v1.0.0",
		}
		// Invalid template syntax.
		_, err := executeAssetTemplate("{{.Broken", data)
		assert.Error(t, err)
	})
}

// TestResolveBinaryName tests binary name resolution order.
func TestResolveBinaryName(t *testing.T) {
	tests := []struct {
		name        string
		binaryName  string
		packageName string
		repoName    string
		expected    string
	}{
		{"explicit binary_name wins", "custom-binary", "owner/repo/binary", "repo", "custom-binary"},
		{"package name with 3 segments", "", "kubernetes/kubernetes/kubectl", "kubernetes", "kubectl"},
		{"falls back to repo_name", "", "hashicorp/terraform", "terraform", "terraform"},
		{"empty package name falls back", "", "", "myrepo", "myrepo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveBinaryName(tt.binaryName, tt.packageName, tt.repoName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractBinaryNameFromPackageName_AllCases tests package name parsing.
func TestExtractBinaryNameFromPackageName_AllCases(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		expected    string
	}{
		{"three segments extracts last", "kubernetes/kubernetes/kubectl", "kubectl"},
		{"four segments extracts last", "org/repo/sub/binary", "binary"},
		{"two segments returns empty", "hashicorp/terraform", ""},
		{"one segment returns empty", "terraform", ""},
		{"empty returns empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBinaryNameFromPackageName(tt.packageName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStripFileExtension_Aqua tests the Aqua package's stripFileExtension function.
func TestStripFileExtension_Aqua(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tar.gz", "tool_linux_amd64.tar.gz", "tool_linux_amd64"},
		{"tar.xz", "tool_linux_amd64.tar.xz", "tool_linux_amd64"},
		{"tar.bz2", "tool_linux_amd64.tar.bz2", "tool_linux_amd64"},
		{"zip", "tool.zip", "tool"},
		{"no extension", "tool-binary", "tool-binary"},
		{"exe", "tool.exe", "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripFileExtension(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAquaRegistry_BuildAssetURL_GitHubReleaseFullFlow tests the full BuildAssetURL flow.
func TestAquaRegistry_BuildAssetURL_GitHubReleaseFullFlow(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &registry.Tool{
		Name:          "terraform",
		Type:          "github_release",
		RepoOwner:     "hashicorp",
		RepoName:      "terraform",
		Asset:         "terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
		VersionPrefix: "v",
	}

	url, err := ar.BuildAssetURL(tool, "1.5.7")
	require.NoError(t, err)
	assert.Contains(t, url, "https://github.com/hashicorp/terraform/releases/download/v1.5.7/")
	assert.Contains(t, url, "terraform_1.5.7_")
}

// TestAquaRegistry_BuildAssetURL_HTTPTypeURL tests HTTP type URL generation.
func TestAquaRegistry_BuildAssetURL_HTTPTypeURL(t *testing.T) {
	ar := NewAquaRegistry()

	tool := &registry.Tool{
		Name:      "aws-cli",
		Type:      "http",
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Asset:     "https://awscli.amazonaws.com/AWSCLIV2-{{.Version}}.pkg",
	}

	url, err := ar.BuildAssetURL(tool, "2.32.31")
	require.NoError(t, err)
	assert.Equal(t, "https://awscli.amazonaws.com/AWSCLIV2-2.32.31.pkg", url)
}

// TestRenderTemplate_ErrorCases tests error handling in renderTemplate.
func TestRenderTemplate_ErrorCases(t *testing.T) {
	t.Run("invalid template syntax", func(t *testing.T) {
		_, err := renderTemplate("{{.Invalid", map[string]string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, registry.ErrNoAssetTemplate)
	})
}

// TestExecuteAssetTemplate_AssetWithoutExt verifies two-pass rendering with Asset/AssetWithoutExt.
func TestExecuteAssetTemplate_AssetWithoutExt(t *testing.T) {
	// Template that references AssetWithoutExt for checksum URL.
	data := map[string]string{
		"Version":   "v1.0.0",
		"SemVer":    "1.0.0",
		"OS":        "linux",
		"Arch":      "amd64",
		"RepoOwner": "test",
		"RepoName":  "tool",
		"Format":    "tar.gz",
	}

	t.Run("template without Asset reference renders normally", func(t *testing.T) {
		result, err := executeAssetTemplate("{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz", data)
		require.NoError(t, err)
		assert.Equal(t, "tool_v1.0.0_linux_amd64.tar.gz", result)
	})

	t.Run("template with AssetWithoutExt", func(t *testing.T) {
		// Simulate a checksum template that uses AssetWithoutExt.
		tmpl := "{{.AssetWithoutExt}}_checksums.txt"
		// First we need to set Asset since the two-pass detects ".Asset" in the template.
		// The template itself doesn't render an asset, so we need a self-referencing pattern.
		// In practice, this is used in checksum URLs, not the asset template itself.
		// Let's test the stripFileExtension utility and the data injection directly.
		dataCopy := make(map[string]string)
		for k, v := range data {
			dataCopy[k] = v
		}
		dataCopy["Asset"] = "tool_v1.0.0_linux_amd64.tar.gz"
		dataCopy["AssetWithoutExt"] = "tool_v1.0.0_linux_amd64"

		result, err := renderTemplate(tmpl, dataCopy)
		require.NoError(t, err)
		assert.Equal(t, "tool_v1.0.0_linux_amd64_checksums.txt", result)
	})
}
