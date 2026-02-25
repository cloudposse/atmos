package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestURLRegistry_SingleIndexFile tests fetching from a single registry.yaml index file.
func TestURLRegistry_SingleIndexFile(t *testing.T) {
	// Mock server serving a single index file with multiple packages.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/registry.yaml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`
packages:
  - type: github_release
    repo_owner: stedolan
    repo_name: jq
    url: "jq-{{.OS}}-{{.Arch}}"
    binary_name: jq
  - type: github_release
    repo_owner: mikefarah
    repo_name: yq
    url: "yq_{{.OS}}_{{.Arch}}"
    binary_name: yq
`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create URLRegistry pointing to the index file (no ref).
	reg := NewURLRegistry(server.URL+"/registry.yaml", "")

	// Verify it's detected as an index file.
	if !reg.isIndexURL {
		t.Fatal("Expected registry to be detected as index file")
	}

	// Test fetching jq from index.
	jq, err := reg.GetTool("stedolan", "jq")
	if err != nil {
		t.Fatalf("Failed to get jq from index: %v", err)
	}
	if jq.RepoName != "jq" || jq.RepoOwner != "stedolan" {
		t.Errorf("Got jq=%+v, want stedolan/jq", jq)
	}

	// Test fetching yq from index.
	yq, err := reg.GetTool("mikefarah", "yq")
	if err != nil {
		t.Fatalf("Failed to get yq from index: %v", err)
	}
	if yq.RepoName != "yq" || yq.RepoOwner != "mikefarah" {
		t.Errorf("Got yq=%+v, want mikefarah/yq", yq)
	}

	// Test tool not found in index.
	_, err = reg.GetTool("nonexistent", "tool")
	if err == nil {
		t.Error("Expected error for nonexistent tool, got nil")
	}
}

// TestURLRegistry_DirectoryStructure tests fetching from per-package registry files.
func TestURLRegistry_DirectoryStructure(t *testing.T) {
	// Mock server serving per-package registry files.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pkgs/stedolan/jq/registry.yaml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`
packages:
  - type: github_release
    repo_owner: stedolan
    repo_name: jq
    url: "jq-{{.OS}}-{{.Arch}}"
    binary_name: jq
`))
		case "/pkgs/jq/registry.yaml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`
packages:
  - type: github_release
    repo_owner: stedolan
    repo_name: jq
    url: "jq-{{.OS}}-{{.Arch}}"
    binary_name: jq
`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create URLRegistry pointing to directory (no .yaml extension, no ref).
	reg := NewURLRegistry(server.URL+"/pkgs", "")

	// Verify it's NOT detected as an index file.
	if reg.isIndexURL {
		t.Fatal("Expected registry to NOT be detected as index file")
	}

	// Test fetching jq using directory pattern.
	jq, err := reg.GetTool("stedolan", "jq")
	if err != nil {
		t.Fatalf("Failed to get jq from directory: %v", err)
	}
	if jq.RepoName != "jq" || jq.RepoOwner != "stedolan" {
		t.Errorf("Got jq=%+v, want stedolan/jq", jq)
	}

	// Verify caching works.
	jqCached, err := reg.GetTool("stedolan", "jq")
	if err != nil {
		t.Fatalf("Failed to get cached jq: %v", err)
	}
	if jqCached != jq {
		t.Error("Expected cached tool to be same instance")
	}
}

// TestURLRegistry_FileExtensionDetection tests pattern detection based on file extension.
func TestURLRegistry_FileExtensionDetection(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantIsIndex bool
	}{
		{
			name:        "yaml extension",
			url:         "https://example.com/registry.yaml",
			wantIsIndex: true,
		},
		{
			name:        "yml extension",
			url:         "https://example.com/registry.yml",
			wantIsIndex: true,
		},
		{
			name:        "no extension with slash",
			url:         "https://example.com/pkgs/",
			wantIsIndex: false,
		},
		{
			name:        "no extension without slash",
			url:         "https://example.com/pkgs",
			wantIsIndex: false,
		},
		{
			name:        "file protocol",
			url:         "file://./custom-registry.yaml",
			wantIsIndex: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &URLRegistry{
				baseURL:    tt.url,
				cache:      make(map[string]*Tool),
				indexCache: make(map[string]*Tool),
				isIndexURL: false, // Will be set by NewURLRegistry logic
			}

			// Manually apply detection logic (same as NewURLRegistry).
			reg.isIndexURL = hasYAMLExtension(tt.url)

			if reg.isIndexURL != tt.wantIsIndex {
				t.Errorf("For URL %q: got isIndexURL=%v, want %v", tt.url, reg.isIndexURL, tt.wantIsIndex)
			}
		})
	}
}

// TestURLRegistry_GetMetadata tests registry metadata retrieval.
func TestURLRegistry_GetMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`packages: []`))
	}))
	defer server.Close()

	reg := NewURLRegistry(server.URL+"/registry.yaml", "")

	ctx := context.Background()
	metadata, err := reg.GetMetadata(ctx)
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	if metadata.Type != "aqua" {
		t.Errorf("Got type=%q, want 'aqua'", metadata.Type)
	}
	if metadata.Source != server.URL+"/registry.yaml" {
		t.Errorf("Got source=%q, want %q", metadata.Source, server.URL+"/registry.yaml")
	}
}

// TestURLRegistry_LoadIndexError tests error handling when index loading fails.
func TestURLRegistry_LoadIndexError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create URLRegistry with .yaml extension (should try to load index).
	reg := NewURLRegistry(server.URL+"/missing.yaml", "")

	// loadIndex should have failed, so isIndexURL should be false.
	if reg.isIndexURL {
		t.Error("Expected isIndexURL to be false after failed index load")
	}
}

// Helper function to check YAML extension.
func hasYAMLExtension(url string) bool {
	return len(url) > 5 && (url[len(url)-5:] == ".yaml" || url[len(url)-4:] == ".yml")
}

// TestApplyGitHubRef tests the ref substitution logic for GitHub URLs.
func TestApplyGitHubRef(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		ref      string
		expected string
	}{
		{
			name:     "empty ref returns original URL",
			baseURL:  "https://github.com/owner/repo",
			ref:      "",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "github URL with ref converts to raw URL",
			baseURL:  "https://github.com/owner/repo",
			ref:      "v1.2.3",
			expected: "https://raw.githubusercontent.com/owner/repo/v1.2.3/registry.yaml",
		},
		{
			name:     "github URL with path and ref",
			baseURL:  "https://github.com/myorg/registries/pkgs/registry.yaml",
			ref:      "abc123def",
			expected: "https://raw.githubusercontent.com/myorg/registries/abc123def/pkgs/registry.yaml",
		},
		{
			name:     "github URL with nested path and ref",
			baseURL:  "https://github.com/org/repo/path/to/registry.yaml",
			ref:      "v2.0.0",
			expected: "https://raw.githubusercontent.com/org/repo/v2.0.0/path/to/registry.yaml",
		},
		{
			name:     "non-GitHub URL unchanged",
			baseURL:  "https://example.com/registry.yaml",
			ref:      "v1.0.0",
			expected: "https://example.com/registry.yaml",
		},
		{
			name:     "raw.githubusercontent.com URL unchanged (already has ref)",
			baseURL:  "https://raw.githubusercontent.com/owner/repo/main/registry.yaml",
			ref:      "v1.0.0",
			expected: "https://raw.githubusercontent.com/owner/repo/main/registry.yaml",
		},
		{
			name:     "malformed github URL too short",
			baseURL:  "https://github.com/short",
			ref:      "v1.0.0",
			expected: "https://github.com/short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyGitHubRef(tt.baseURL, tt.ref)
			if result != tt.expected {
				t.Errorf("applyGitHubRef(%q, %q) = %q, want %q", tt.baseURL, tt.ref, result, tt.expected)
			}
		})
	}
}

// TestURLRegistry_WithRef tests that ref is properly applied when creating a URLRegistry.
func TestURLRegistry_WithRef(t *testing.T) {
	// Track which URL was requested.
	var requestedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    url: "tool-{{.OS}}-{{.Arch}}"
`))
	}))
	defer server.Close()

	// Simulate a GitHub raw URL structure (the server URL replaces raw.githubusercontent.com).
	// Since we can't actually use raw.githubusercontent.com in tests, we verify the ref field is stored.
	reg := NewURLRegistry(server.URL+"/registry.yaml", "v1.0.0")

	// Verify ref is stored.
	if reg.ref != "v1.0.0" {
		t.Errorf("Expected ref to be 'v1.0.0', got %q", reg.ref)
	}

	// The URL should still work (it's not a GitHub raw URL, so no transformation).
	if reg.baseURL != server.URL+"/registry.yaml" {
		t.Errorf("Expected baseURL to be unchanged for non-GitHub URL, got %q", reg.baseURL)
	}

	// Verify we can fetch tools.
	tool, err := reg.GetTool("test", "tool")
	if err != nil {
		t.Fatalf("Failed to get tool: %v", err)
	}
	if tool.RepoName != "tool" {
		t.Errorf("Expected tool name 'tool', got %q", tool.RepoName)
	}

	// Verify the request was made (index was loaded).
	if requestedURL != "/registry.yaml" {
		t.Errorf("Expected request to /registry.yaml, got %q", requestedURL)
	}
}

// TestURLRegistry_GetToolWithVersion tests version-specific overrides.
func TestURLRegistry_GetToolWithVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
packages:
  - type: github_release
    repo_owner: test
    repo_name: tool
    url: "tool-{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz"
    format: tar.gz
    version_overrides:
      - version_constraint: 'semver("<= 1.0.0")'
        asset: "tool-legacy-{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz"
        format: zip
`))
	}))
	defer server.Close()

	reg := NewURLRegistry(server.URL+"/registry.yaml", "")

	t.Run("version matching override", func(t *testing.T) {
		tool, err := reg.GetTool("test", "tool")
		if err != nil {
			t.Fatalf("Failed to get tool: %v", err)
		}

		// Apply version override manually via GetToolWithVersion.
		toolWithVersion, err := reg.GetToolWithVersion("test", "tool", "0.9.0")
		if err != nil {
			t.Fatalf("Failed to get tool with version: %v", err)
		}
		if toolWithVersion.Version != "0.9.0" {
			t.Errorf("Expected version 0.9.0, got %q", toolWithVersion.Version)
		}
		// Verify override fields were applied for version <= 1.0.0.
		if !strings.Contains(toolWithVersion.Asset, "legacy") {
			t.Errorf("Expected legacy asset override, got %q", toolWithVersion.Asset)
		}
		if toolWithVersion.Format != "zip" {
			t.Errorf("Expected format override 'zip', got %q", toolWithVersion.Format)
		}
		_ = tool
	})

	t.Run("version not matching override uses defaults", func(t *testing.T) {
		toolWithVersion, err := reg.GetToolWithVersion("test", "tool", "2.0.0")
		if err != nil {
			t.Fatalf("Failed to get tool with version: %v", err)
		}
		if toolWithVersion.Version != "2.0.0" {
			t.Errorf("Expected version 2.0.0, got %q", toolWithVersion.Version)
		}
		if strings.Contains(toolWithVersion.Asset, "legacy") {
			t.Errorf("Expected default asset (non-legacy), got %q", toolWithVersion.Asset)
		}
		if toolWithVersion.Format != "tar.gz" {
			t.Errorf("Expected default format 'tar.gz', got %q", toolWithVersion.Format)
		}
	})

	t.Run("tool not found returns error", func(t *testing.T) {
		_, err := reg.GetToolWithVersion("nonexistent", "tool", "1.0.0")
		if err == nil {
			t.Error("Expected error for nonexistent tool, got nil")
		}
	})
}

// TestURLRegistry_GetLatestVersion tests that URL registries don't support version queries.
func TestURLRegistry_GetLatestVersion(t *testing.T) {
	reg := &URLRegistry{
		baseURL:    "https://example.com/registry.yaml",
		cache:      make(map[string]*Tool),
		indexCache: make(map[string]*Tool),
	}

	_, err := reg.GetLatestVersion("test", "tool")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// TestURLRegistry_LoadLocalConfig tests that LoadLocalConfig is a no-op.
func TestURLRegistry_LoadLocalConfig(t *testing.T) {
	reg := &URLRegistry{
		baseURL:    "https://example.com/registry.yaml",
		cache:      make(map[string]*Tool),
		indexCache: make(map[string]*Tool),
	}

	err := reg.LoadLocalConfig("/nonexistent/path")
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
}

// TestURLRegistry_Search tests that Search returns empty for URL registries.
func TestURLRegistry_Search(t *testing.T) {
	reg := &URLRegistry{
		baseURL:    "https://example.com/registry.yaml",
		cache:      make(map[string]*Tool),
		indexCache: make(map[string]*Tool),
	}

	results, err := reg.Search(context.Background(), "terraform")
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d", len(results))
	}
}

// TestURLRegistry_ListAll tests that ListAll returns empty for URL registries.
func TestURLRegistry_ListAll(t *testing.T) {
	reg := &URLRegistry{
		baseURL:    "https://example.com/registry.yaml",
		cache:      make(map[string]*Tool),
		indexCache: make(map[string]*Tool),
	}

	results, err := reg.ListAll(context.Background())
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d", len(results))
	}
}

// TestURLRegistry_VersionConstraintEvaluation tests the URL registry's version constraint functions.
func TestURLRegistry_VersionConstraintEvaluation(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string
		want       bool
		wantErr    bool
	}{
		{"empty constraint", "", "1.0.0", false, false},
		{"literal true", "true", "1.0.0", true, false},
		{"literal false", "false", "1.0.0", false, false},
		{"quoted true", `"true"`, "1.0.0", true, false},
		{"quoted false", `"false"`, "1.0.0", false, false},
		{"exact version match", `Version == "v1.0.0"`, "v1.0.0", true, false},
		{"exact version mismatch", `Version == "v1.0.0"`, "v2.0.0", false, false},
		{"semver >=", `semver(">= 1.0.0")`, "1.5.0", true, false},
		{"semver >= fails", `semver(">= 2.0.0")`, "1.5.0", false, false},
		{"semver <=", `semver("<= 1.0.0")`, "0.9.0", true, false},
		{"semver range", `semver(">= 1.0.0, < 2.0.0")`, "1.5.0", true, false},
		{"semver range outside", `semver(">= 1.0.0, < 2.0.0")`, "2.5.0", false, false},
		{"version not equal", `Version != "v1.0.0"`, "v2.0.0", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateVersionConstraint(tt.constraint, tt.version)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateVersionConstraint(%q, %q) = %v, want %v", tt.constraint, tt.version, got, tt.want)
			}
		})
	}
}

// TestCompareSemver_URLRegistry tests semver comparison in the URL registry.
func TestCompareSemver_URLRegistry(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string
		want       bool
	}{
		{"greater than or equal true", ">= 1.0.0", "1.5.0", true},
		{"greater than or equal false", ">= 2.0.0", "1.5.0", false},
		{"less than or equal true", "<= 2.0.0", "1.5.0", true},
		{"less than or equal false", "<= 1.0.0", "1.5.0", false},
		{"greater than true", "> 1.0.0", "1.5.0", true},
		{"greater than false", "> 2.0.0", "1.5.0", false},
		{"less than true", "< 2.0.0", "1.5.0", true},
		{"less than false", "< 1.0.0", "1.5.0", false},
		{"equal true", "= 1.5.0", "1.5.0", true},
		{"equal false", "= 1.0.0", "1.5.0", false},
		{"not equal true", "!= 1.0.0", "1.5.0", true},
		{"not equal false", "!= 1.5.0", "1.5.0", false},
		{"comma-separated AND", ">= 1.0.0, < 2.0.0", "1.5.0", true},
		{"comma-separated AND fails", ">= 1.0.0, < 2.0.0", "2.5.0", false},
		{"invalid version", ">= 1.0.0", "not-a-version", false},
		{"invalid constraint", ">= not-a-version", "1.5.0", false},
		{"unknown operator", "~> 1.0.0", "1.5.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareSemver(tt.constraint, tt.version)
			if got != tt.want {
				t.Errorf("compareSemver(%q, %q) = %v, want %v", tt.constraint, tt.version, got, tt.want)
			}
		})
	}
}

// TestApplyOverrideFields tests that override fields are applied correctly.
func TestApplyOverrideFields(t *testing.T) {
	t.Run("applies asset and format", func(t *testing.T) {
		tool := &Tool{
			Asset:  "default-asset",
			Format: "tar.gz",
		}
		override := &VersionOverride{
			VersionConstraint: "true",
			Asset:             "override-asset",
			Format:            "zip",
		}
		applyOverrideFields(tool, override, "1.0.0")
		if tool.Asset != "override-asset" {
			t.Errorf("Expected asset 'override-asset', got %q", tool.Asset)
		}
		if tool.Format != "zip" {
			t.Errorf("Expected format 'zip', got %q", tool.Format)
		}
	})

	t.Run("preserves defaults when override is empty", func(t *testing.T) {
		tool := &Tool{
			Asset:  "default-asset",
			Format: "tar.gz",
		}
		override := &VersionOverride{
			VersionConstraint: "true",
			// No overrides set.
		}
		applyOverrideFields(tool, override, "1.0.0")
		if tool.Asset != "default-asset" {
			t.Errorf("Expected asset to remain 'default-asset', got %q", tool.Asset)
		}
		if tool.Format != "tar.gz" {
			t.Errorf("Expected format to remain 'tar.gz', got %q", tool.Format)
		}
	})

	t.Run("applies files and replacements", func(t *testing.T) {
		tool := &Tool{
			Asset: "default",
		}
		override := &VersionOverride{
			VersionConstraint: "true",
			Files:             []File{{Name: "binary", Src: "dist/binary"}},
			Replacements:      map[string]string{"amd64": "x86_64"},
		}
		applyOverrideFields(tool, override, "1.0.0")
		if len(tool.Files) != 1 || tool.Files[0].Name != "binary" {
			t.Errorf("Expected files to be applied, got %v", tool.Files)
		}
		if tool.Replacements["amd64"] != "x86_64" {
			t.Errorf("Expected replacements to be applied, got %v", tool.Replacements)
		}
	})
}
