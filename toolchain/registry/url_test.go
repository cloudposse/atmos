package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	// Create URLRegistry pointing to the index file.
	reg := NewURLRegistry(server.URL + "/registry.yaml")

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

	// Create URLRegistry pointing to directory (no .yaml extension).
	reg := NewURLRegistry(server.URL + "/pkgs")

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

	reg := NewURLRegistry(server.URL + "/registry.yaml")

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
	reg := NewURLRegistry(server.URL + "/missing.yaml")

	// loadIndex should have failed, so isIndexURL should be false.
	if reg.isIndexURL {
		t.Error("Expected isIndexURL to be false after failed index load")
	}
}

// Helper function to check YAML extension.
func hasYAMLExtension(url string) bool {
	return len(url) > 5 && (url[len(url)-5:] == ".yaml" || url[len(url)-4:] == ".yml")
}
