package httpmock

import (
	"bytes"
	"crypto/md5"  //nolint:gosec // Checksums for mock testing, not security
	"crypto/sha1" //nolint:gosec // Checksums for mock testing, not security
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	contentTypeJSON   = "application/json"
	headerContentType = "Content-Type"
	bodyPreviewMax    = 500
)

// aqlResponse represents the AQL search response structure.
// Field order matters: Results must come before Range due to JFrog SDK ContentReader bug.
type aqlResponse struct {
	Results []map[string]interface{} `json:"results"`
	Range   aqlRange                 `json:"range"`
}

// aqlRange represents the range metadata in AQL responses.
type aqlRange struct {
	StartPos int `json:"start_pos"`
	EndPos   int `json:"end_pos"`
	Total    int `json:"total"`
}

// ArtifactoryMockServer provides a mock HTTP server that implements
// enough of the JFrog Artifactory Generic repository API to test
// the Atmos Artifactory store integration.
type ArtifactoryMockServer struct {
	Server *httptest.Server
	mu     sync.RWMutex
	files  map[string][]byte // repo/path -> content
	t      *testing.T
	debug  bool // Enable request logging.
}

// NewArtifactoryMockServer creates a mock Artifactory server.
// The server is automatically cleaned up when the test completes.
func NewArtifactoryMockServer(t *testing.T) *ArtifactoryMockServer {
	t.Helper()

	mock := &ArtifactoryMockServer{
		files: make(map[string][]byte),
		t:     t,
		debug: true, // Enable debug logging for troubleshooting.
	}

	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.handleRequest(w, r)
	}))

	t.Cleanup(func() { mock.Server.Close() })
	return mock
}

// handleRequest routes requests to the appropriate handler.
//
//nolint:revive // Multiple HTTP methods and endpoints require switch statement.
func (m *ArtifactoryMockServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Strip /artifactory/ prefix if present (JFrog SDK may add this internally).
	path = strings.TrimPrefix(path, "artifactory/")

	// Debug logging to understand JFrog SDK request patterns.
	if m.debug && m.t != nil {
		var bodyPreview string
		if r.Body != nil && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			if len(body) > 0 && len(body) < bodyPreviewMax {
				bodyPreview = " Body: " + string(body)
			}
		}
		m.t.Logf("Mock request: %s %s (path=%s)%s", r.Method, r.URL.Path, path, bodyPreview)
	}

	// Handle Artifactory system endpoints that the SDK checks.
	if strings.HasPrefix(path, "api/system") {
		m.handleSystemAPI(w, path)
		return
	}

	switch r.Method {
	case http.MethodPut:
		m.handleUpload(w, r, path)
	case http.MethodGet:
		m.handleDownload(w, r, path)
	case http.MethodPost:
		if strings.HasSuffix(path, "api/search/aql") {
			m.handleAQLSearch(w, r)
		} else {
			http.NotFound(w, r)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSystemAPI handles Artifactory system API endpoints.
func (m *ArtifactoryMockServer) handleSystemAPI(w http.ResponseWriter, path string) {
	w.Header().Set(headerContentType, contentTypeJSON)

	switch {
	case strings.HasSuffix(path, "api/system/version"):
		// Return mock Artifactory version.
		response := map[string]interface{}{
			"version":  "7.55.0",
			"revision": "12345",
			"addons":   []string{},
			"license":  "OSS",
		}
		_ = json.NewEncoder(w).Encode(response)
	case strings.HasSuffix(path, "api/system/ping"):
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	default:
		// Return generic system info for other endpoints.
		response := map[string]interface{}{
			"version": "7.55.0",
		}
		_ = json.NewEncoder(w).Encode(response)
	}
}

// handleUpload handles PUT requests to upload files.
func (m *ArtifactoryMockServer) handleUpload(w http.ResponseWriter, r *http.Request, path string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	m.mu.Lock()
	m.files[path] = body
	m.mu.Unlock()

	// Return Artifactory-style response.
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"repo":        strings.Split(path, "/")[0],
		"path":        "/" + strings.Join(strings.Split(path, "/")[1:], "/"),
		"created":     "2024-01-01T00:00:00.000Z",
		"createdBy":   "test",
		"downloadUri": m.Server.URL + "/" + path,
		"mimeType":    "application/json",
		"size":        len(body),
	}
	_ = json.NewEncoder(w).Encode(response)
}

// handleDownload handles GET requests to download files.
func (m *ArtifactoryMockServer) handleDownload(w http.ResponseWriter, r *http.Request, path string) {
	m.mu.RLock()
	content, exists := m.files[path]
	m.mu.RUnlock()

	if !exists {
		http.NotFound(w, r)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// buildAQLResults builds results from files matching the AQL query.
func (m *ArtifactoryMockServer) buildAQLResults(query string) []map[string]interface{} {
	var results []map[string]interface{}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for filePath, content := range m.files {
		if !m.matchesAQLQuery(filePath, query) {
			continue
		}

		parts := strings.Split(filePath, "/")
		repo := parts[0]
		name := parts[len(parts)-1]
		var pathStr string
		if len(parts) > 2 {
			pathStr = strings.Join(parts[1:len(parts)-1], "/")
		} else {
			pathStr = "."
		}

		// Calculate checksums from actual content.
		md5Sum := md5.Sum(content)   //nolint:gosec
		sha1Sum := sha1.Sum(content) //nolint:gosec
		sha256Sum := sha256.Sum256(content)

		results = append(results, map[string]interface{}{
			"repo":        repo,
			"path":        pathStr,
			"name":        name,
			"type":        "file",
			"size":        len(content),
			"actual_md5":  hex.EncodeToString(md5Sum[:]),
			"actual_sha1": hex.EncodeToString(sha1Sum[:]),
			"sha256":      hex.EncodeToString(sha256Sum[:]),
			"modified":    "2024-01-01T00:00:00.000Z",
			"created":     "2024-01-01T00:00:00.000Z",
		})
	}

	return results
}

// handleAQLSearch handles POST requests to the AQL search API.
// The JFrog SDK uses AQL to find files matching a pattern.
func (m *ArtifactoryMockServer) handleAQLSearch(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	query := string(body)
	results := m.buildAQLResults(query)

	// Build response using struct to ensure proper JSON encoding.
	// Field order matters: Results must come before Range due to JFrog SDK ContentReader bug
	// where it doesn't properly skip nested objects when searching for the target key.
	response := aqlResponse{
		Results: results,
		Range: aqlRange{
			StartPos: 0,
			EndPos:   len(results),
			Total:    len(results),
		},
	}
	responseJSON, _ := json.Marshal(response)

	// Debug log the AQL response.
	if m.debug && m.t != nil {
		m.t.Logf("AQL response: %d results", len(results))
		m.t.Logf("AQL response body: %s", string(responseJSON))
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(responseJSON)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseJSON)
}

// matchesAQLQuery performs a simple match between a file path and an AQL query.
// AQL queries from JFrog SDK look like:
// items.find({"repo":"test-repo","path":{"$match":"atmos/dev/vpc"},"name":"vpc_id"}).
func (m *ArtifactoryMockServer) matchesAQLQuery(filePath, query string) bool {
	parts := strings.Split(filePath, "/")
	if len(parts) < 2 {
		return false
	}

	repo := parts[0]
	name := parts[len(parts)-1]
	var pathPart string
	if len(parts) > 2 {
		pathPart = strings.Join(parts[1:len(parts)-1], "/")
	}

	// Check if repo matches.
	if !strings.Contains(query, `"`+repo+`"`) {
		return false
	}

	// Check if name matches.
	if !strings.Contains(query, `"`+name+`"`) {
		return false
	}

	// If there's a path component, check if it matches.
	if pathPart != "" {
		// The SDK uses $match for path patterns.
		if strings.Contains(query, `"$match":"`+pathPart+`"`) {
			return true
		}
		// Also check for exact path match.
		if strings.Contains(query, `"path":"`+pathPart+`"`) {
			return true
		}
		// Check if path appears anywhere in query (more permissive).
		if strings.Contains(query, pathPart) {
			return true
		}
	}

	return true
}

// URL returns the mock server URL.
func (m *ArtifactoryMockServer) URL() string {
	return m.Server.URL
}

// SetFile directly sets a file in the mock store (useful for test setup).
func (m *ArtifactoryMockServer) SetFile(path string, content []byte) {
	m.mu.Lock()
	m.files[path] = content
	m.mu.Unlock()
}

// GetFile retrieves a file from the mock store (useful for test assertions).
func (m *ArtifactoryMockServer) GetFile(path string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	content, exists := m.files[path]
	return content, exists
}

// ListFiles returns all files currently stored in the mock.
func (m *ArtifactoryMockServer) ListFiles() map[string][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string][]byte, len(m.files))
	for k, v := range m.files {
		result[k] = v
	}
	return result
}

// Clear removes all files from the mock store.
func (m *ArtifactoryMockServer) Clear() {
	m.mu.Lock()
	m.files = make(map[string][]byte)
	m.mu.Unlock()
}
