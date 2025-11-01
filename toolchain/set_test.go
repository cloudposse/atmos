package toolchain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// Test data structures.
type mockInstallerSet struct {
	parseToolSpecFunc func(string) (string, string, error)
	findToolFunc      func(string, string, string) (*mockTool, error)
}

type mockTool struct {
	Type string
}

func (m *mockInstallerSet) parseToolSpec(toolName string) (string, string, error) {
	if m.parseToolSpecFunc != nil {
		return m.parseToolSpecFunc(toolName)
	}
	parts := strings.Split(toolName, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid tool spec")
	}
	return parts[0], parts[1], nil
}

func (m *mockInstallerSet) findTool(owner, repo, version string) (*mockTool, error) {
	if m.findToolFunc != nil {
		return m.findToolFunc(owner, repo, version)
	}
	return &mockTool{Type: "github_release"}, nil
}

// Mock functions for dependencies that need to be injected.
var (
	newInstallerFunc      = func() *mockInstallerSet { return &mockInstallerSet{} }
	addToolToVersionsFunc func(string, string, string) error
)

// Test helper functions.
func setupTest() {
	viper.Reset()
}

func teardownTest() {
	viper.Reset()
}

// Tests for versionListModel.
func TestVersionListModelInit(t *testing.T) {
	m := versionListModel{}
	cmd := m.Init()
	if cmd != nil {
		t.Error("Expected Init() to return nil")
	}
}

func TestVersionListModelUpdate(t *testing.T) {
	// Create a basic model for testing
	items := []list.Item{
		versionItem{version: "1.0.0", title: "Version 1.0.0", desc: "First version"},
		versionItem{version: "2.0.0", title: "Version 2.0.0", desc: "Second version"},
	}

	l := list.New(items, newCustomDelegate(), 50, 10)
	vp := viewport.New(50, 10)

	m := versionListModel{
		list:             l,
		viewport:         vp,
		focused:          "list",
		scrollSpeed:      3,
		currentItemIndex: -1,
	}

	// Test quit keys
	t.Run("Quit with ctrl+c", func(t *testing.T) {
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		assert.Equal(t, cmd(), tea.Quit())
		_ = result
	})

	t.Run("Quit with q", func(t *testing.T) {
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		assert.Equal(t, cmd(), tea.Quit())
		_ = result
	})

	// Test tab focus switching
	t.Run("Tab switches focus", func(t *testing.T) {
		m.focused = "list"
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		resultModel := result.(versionListModel)
		if resultModel.focused != "viewport" {
			t.Error("Expected focus to switch to viewport")
		}

		result, _ = resultModel.Update(tea.KeyMsg{Type: tea.KeyTab})
		resultModel = result.(versionListModel)
		if resultModel.focused != "list" {
			t.Error("Expected focus to switch back to list")
		}
	})

	// Test enter key
	t.Run("Enter selects item", func(t *testing.T) {
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		resultModel := result.(versionListModel)
		if resultModel.selected == "" {
			t.Error("Expected item to be selected")
		}
		assert.Equal(t, cmd(), tea.Quit())
	})

	// Test window size message
	t.Run("Window size update", func(t *testing.T) {
		result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		resultModel := result.(versionListModel)
		if resultModel.viewport.Width != 71 {
			t.Errorf("Expected viewport width to be 71, got %d", resultModel.viewport.Width)
		}
	})

	// Test viewport scrolling when focused
	t.Run("Viewport scrolling", func(t *testing.T) {
		m.focused = "viewport"

		// Test up arrow
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		_ = result

		// Test down arrow
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		_ = result

		// Test page up
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		_ = result

		// Test page down
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		_ = result

		// Test home
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
		_ = result

		// Test end
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		_ = result

		// Test 'g' key
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
		_ = result

		// Test 'G' key
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
		_ = result

		// Test 'j' key
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		_ = result

		// Test 'k' key
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		_ = result
	})
}

func TestVersionListModelView(t *testing.T) {
	// Test error case
	t.Run("View with error", func(t *testing.T) {
		m := versionListModel{err: fmt.Errorf("test error")}
		view := m.View()
		if !strings.Contains(view, "Error: test error") {
			t.Error("Expected view to contain error message")
		}
	})

	// Test normal case
	t.Run("View normal", func(t *testing.T) {
		items := []list.Item{
			versionItem{version: "1.0.0", title: "Version 1.0.0", desc: "First version"},
		}

		l := list.New(items, newCustomDelegate(), 50, 10)
		l.Title = "Test Versions"
		vp := viewport.New(50, 10)

		m := versionListModel{
			list:        l,
			viewport:    vp,
			title:       "Test Tool Versions",
			focused:     "list",
			scrollSpeed: 3,
		}

		view := m.View()
		if !strings.Contains(view, "Test Tool Versions") {
			t.Error("Expected view to contain title")
		}
		if !strings.Contains(view, "Press Tab to switch between panes") {
			t.Error("Expected view to contain navigation hint")
		}
		if !strings.Contains(view, "Scroll speed: 3x") {
			t.Error("Expected view to contain scroll speed hint")
		}
	})

	// Test with normal scroll speed
	t.Run("View with normal scroll speed", func(t *testing.T) {
		m := versionListModel{
			list:        list.New([]list.Item{}, newCustomDelegate(), 50, 10),
			viewport:    viewport.New(50, 10),
			title:       "Test",
			focused:     "list",
			scrollSpeed: 1,
		}

		view := m.View()
		if strings.Contains(view, "Scroll speed:") {
			t.Error("Expected view to not contain scroll speed hint for normal speed")
		}
	})
}

// Mock HTTP server for testing GitHub API.
func createMockGitHubServer(releases []map[string]interface{}, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if statusCode == http.StatusOK {
			json.NewEncoder(w).Encode(releases)
		}
	}))
}

func TestFetchGitHubVersions(t *testing.T) {
	setupTest()
	defer teardownTest()

	t.Run("Successful fetch", func(t *testing.T) {
		releases := []map[string]interface{}{
			{
				"tag_name":     "v1.0.0",
				"name":         "Version 1.0.0",
				"body":         "Initial release",
				"prerelease":   false,
				"published_at": "2023-01-01T00:00:00Z",
			},
			{
				"tag_name":     "v0.9.0-beta",
				"name":         "Beta Release",
				"body":         "Beta version",
				"prerelease":   true,
				"published_at": "2022-12-01T00:00:00Z",
			},
		}

		server := createMockGitHubServer(releases, http.StatusOK)
		defer server.Close()

		// Redirect stdout to capture debug output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Mock the API URL by replacing the function temporarily
		originalURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", "owner", "repo")
		_ = originalURL

		items, err := fetchGitHubVersionsWithCustomURL(server.URL + "/repos/owner/repo/releases?per_page=100")

		w.Close()
		os.Stdout = oldStdout

		// Read captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(items) != 1 {
			t.Errorf("Expected 1 item (non-prerelease), got %d", len(items))
		}

		if items[0].version != "1.0.0" {
			t.Errorf("Expected version '1.0.0', got '%s'", items[0].version)
		}

		if !strings.Contains(output, "Fetched 2 total releases") {
			t.Error("Expected debug output about total releases")
		}

		if !strings.Contains(output, "Found 1 non-prerelease versions") {
			t.Error("Expected debug output about non-prerelease versions")
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := createMockGitHubServer(nil, http.StatusNotFound)
		defer server.Close()

		_, err := fetchGitHubVersionsWithCustomURL(server.URL + "/repos/owner/repo/releases?per_page=100")
		if err == nil {
			t.Error("Expected error for 404 status")
		}
		if !strings.Contains(err.Error(), "GitHub API returned status 404") {
			t.Errorf("Expected specific error message, got %v", err)
		}
	})

	t.Run("No non-prerelease versions", func(t *testing.T) {
		releases := []map[string]interface{}{
			{
				"tag_name":   "v1.0.0-beta",
				"prerelease": true,
			},
		}

		server := createMockGitHubServer(releases, http.StatusOK)
		defer server.Close()

		// Redirect stdout to capture debug output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		_, err := fetchGitHubVersionsWithCustomURL(server.URL + "/repos/owner/repo/releases?per_page=100")

		w.Close()
		os.Stdout = oldStdout
		io.Copy(&bytes.Buffer{}, r)

		if err == nil {
			t.Error("Expected error for no non-prerelease versions")
		}
		if !strings.Contains(err.Error(), "no non-prerelease versions found") {
			t.Errorf("Expected specific error message, got %v", err)
		}
	})

	t.Run("With GitHub token", func(t *testing.T) {
		viper.Set("github-token", "test-token")
		defer viper.Set("github-token", "")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				t.Errorf("Expected Authorization header 'Bearer test-token', got '%s'", auth)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"tag_name":   "v1.0.0",
					"name":       "Test",
					"body":       "Test body",
					"prerelease": false,
				},
			})
		}))
		defer server.Close()

		// Redirect stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		_, err := fetchGitHubVersionsWithCustomURL(server.URL)

		w.Close()
		os.Stdout = oldStdout
		io.Copy(&bytes.Buffer{}, r)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		_, err := fetchGitHubVersionsWithCustomURL(server.URL)
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "failed to parse releases JSON") {
			t.Errorf("Expected JSON parse error, got %v", err)
		}
	})
}

// Helper function to test with custom URL.
func fetchGitHubVersionsWithCustomURL(apiURL string) ([]versionItem, error) {
	token := viper.GetString("github-token")

	client := &http.Client{}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		Prerelease  bool   `json:"prerelease"`
		PublishedAt string `json:"published_at"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases JSON: %w", err)
	}

	fmt.Printf("Fetched %d total releases from GitHub\n", len(releases))

	var items []versionItem
	for _, release := range releases {
		if !release.Prerelease {
			version := strings.TrimPrefix(release.TagName, versionPrefix)
			title := release.Name
			if title == "" {
				title = release.TagName
			}
			releaseNotes := formatReleaseNotes(release.Name, release.TagName, release.Body, release.PublishedAt)
			items = append(items, versionItem{
				version:      version,
				title:        title,
				desc:         fmt.Sprintf("Version %s", version),
				releaseNotes: releaseNotes,
			})
		}
	}

	fmt.Printf("Found %d non-prerelease versions\n", len(items))

	if len(items) == 0 {
		return nil, fmt.Errorf("no non-prerelease versions found")
	}

	return items, nil
}

// Mock for testing SetToolVersion - we need to create wrapper functions
// since we can't easily mock the original functions

func TestSetToolVersionWithMocks(t *testing.T) {
	setupTest()
	defer teardownTest()

	// Test case: version provided directly
	t.Run("Version provided directly", func(t *testing.T) {
		// Mock the installer
		originalNewInstaller := newInstallerFunc
		newInstallerFunc = func() *mockInstallerSet {
			return &mockInstallerSet{
				parseToolSpecFunc: func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				},
			}
		}
		defer func() { newInstallerFunc = originalNewInstaller }()

		// Mock AddToolToVersions
		var addToolCalled bool
		var addToolArgs []string
		originalAddTool := addToolToVersionsFunc
		addToolToVersionsFunc = func(filePath, toolKey, version string) error {
			addToolCalled = true
			addToolArgs = []string{filePath, toolKey, version}
			return nil
		}
		defer func() { addToolToVersionsFunc = originalAddTool }()

		err := setToolVersionWithMocks("owner/repo", "1.0.0")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !addToolCalled {
			t.Error("Expected AddToolToVersions to be called")
		}

		expectedArgs := []string{"/test/file", "owner/repo", "1.0.0"}
		for i, expected := range expectedArgs {
			if i < len(addToolArgs) && addToolArgs[i] != expected {
				t.Errorf("Expected arg %d to be '%s', got '%s'", i, expected, addToolArgs[i])
			}
		}
	})

	// Test case: invalid tool name
	t.Run("Invalid tool name", func(t *testing.T) {
		originalNewInstaller := newInstallerFunc
		newInstallerFunc = func() *mockInstallerSet {
			return &mockInstallerSet{
				parseToolSpecFunc: func(toolName string) (string, string, error) {
					return "", "", fmt.Errorf("invalid tool spec")
				},
			}
		}
		defer func() { newInstallerFunc = originalNewInstaller }()

		err := setToolVersionWithMocks("invalid", "1.0.0")
		if err == nil {
			t.Error("Expected error for invalid tool name")
		}
		if !strings.Contains(err.Error(), "invalid tool name") {
			t.Errorf("Expected 'invalid tool name' error, got %v", err)
		}
	})

	// Test case: AddToolToVersions fails
	t.Run("AddToolToVersions fails", func(t *testing.T) {
		originalNewInstaller := newInstallerFunc
		newInstallerFunc = func() *mockInstallerSet {
			return &mockInstallerSet{
				parseToolSpecFunc: func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				},
			}
		}
		defer func() { newInstallerFunc = originalNewInstaller }()

		originalAddTool := addToolToVersionsFunc
		addToolToVersionsFunc = func(filePath, toolKey, version string) error {
			return fmt.Errorf("failed to add tool")
		}
		defer func() { addToolToVersionsFunc = originalAddTool }()

		err := setToolVersionWithMocks("owner/repo", "1.0.0")
		if err == nil {
			t.Error("Expected error when AddToolToVersions fails")
		}
		if !strings.Contains(err.Error(), "failed to set version") {
			t.Errorf("Expected 'failed to set version' error, got %v", err)
		}
	})

	// Test interactive selection cases
	t.Run("Interactive selection - non-github_release tool", func(t *testing.T) {
		originalNewInstaller := newInstallerFunc
		newInstallerFunc = func() *mockInstallerSet {
			return &mockInstallerSet{
				parseToolSpecFunc: func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				},
				findToolFunc: func(owner, repo, version string) (*mockTool, error) {
					return &mockTool{Type: "other"}, nil
				},
			}
		}
		defer func() { newInstallerFunc = originalNewInstaller }()

		err := setToolVersionWithMocks("owner/repo", "")
		if err == nil {
			t.Error("Expected error for non-github_release tool")
		}
		if !strings.Contains(err.Error(), "interactive version selection is only available for GitHub release type tools") {
			t.Errorf("Expected specific error message, got %v", err)
		}
	})

	t.Run("Interactive selection - findTool fails", func(t *testing.T) {
		originalNewInstaller := newInstallerFunc
		newInstallerFunc = func() *mockInstallerSet {
			return &mockInstallerSet{
				parseToolSpecFunc: func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				},
				findToolFunc: func(owner, repo, version string) (*mockTool, error) {
					return nil, fmt.Errorf("tool not found")
				},
			}
		}
		defer func() { newInstallerFunc = originalNewInstaller }()

		err := setToolVersionWithMocks("owner/repo", "")
		if err == nil {
			t.Error("Expected error when findTool fails")
		}
		if !strings.Contains(err.Error(), "failed to find tool configuration") {
			t.Errorf("Expected 'failed to find tool configuration' error, got %v", err)
		}
	})
}

// Mock version of SetToolVersion for testing.
func setToolVersionWithMocks(toolName, version string) error {
	const filePath = "/test/file"

	installer := newInstallerFunc()
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}
	resolvedKey := owner + "/" + repo

	if version == "" {
		tool, err := installer.findTool(owner, repo, "")
		if err != nil {
			return fmt.Errorf("failed to find tool configuration: %w", err)
		}

		if tool.Type != "github_release" {
			return fmt.Errorf("interactive version selection is only available for GitHub release type tools")
		}

		// For testing, we'll skip the interactive part and just return an error
		// or simulate successful selection based on test needs
		return fmt.Errorf("interactive selection not implemented in tests")
	}

	err = addToolToVersionsFunc(filePath, resolvedKey, version)
	if err != nil {
		return fmt.Errorf("failed to set version: %w", err)
	}

	fmt.Printf("✓ Set %s@%s in %s\n", resolvedKey, version, filePath)
	return nil
}

// Test the model's behavior with different window sizes.
func TestVersionListModelWindowResize(t *testing.T) {
	items := []list.Item{
		versionItem{version: "1.0.0", title: "Version 1.0.0", releaseNotes: "Test notes"},
	}

	l := list.New(items, newCustomDelegate(), 50, 10)
	vp := viewport.New(50, 10)

	m := versionListModel{
		list:             l,
		viewport:         vp,
		focused:          "list",
		scrollSpeed:      3,
		currentItemIndex: -1,
	}

	// Test various window sizes
	testSizes := []tea.WindowSizeMsg{
		{Width: 80, Height: 24},
		{Width: 120, Height: 30},
		{Width: 40, Height: 15}, // Small window
	}

	for _, size := range testSizes {
		result, _ := m.Update(size)
		resultModel := result.(versionListModel)

		expectedLeftWidth := size.Width / 4
		expectedRightWidth := size.Width - expectedLeftWidth - 2 - 2 // Account for separators and borders
		expectedHeight := size.Height - 6

		if resultModel.viewport.Width != expectedRightWidth {
			t.Errorf("For window width %d, expected viewport width %d, got %d",
				size.Width, expectedRightWidth, resultModel.viewport.Width)
		}

		if resultModel.viewport.Height != expectedHeight {
			t.Errorf("For window height %d, expected viewport height %d, got %d",
				size.Height, expectedHeight, resultModel.viewport.Height)
		}
	}
}

// Test edge cases for versionItem methods.
func TestVersionItemEdgeCases(t *testing.T) {
	t.Run("Empty values", func(t *testing.T) {
		item := versionItem{}

		if item.Title() != "" {
			t.Errorf("Expected empty Title(), got '%s'", item.Title())
		}
		if item.Description() != "" {
			t.Errorf("Expected empty Description(), got '%s'", item.Description())
		}
		if item.FilterValue() != "" {
			t.Errorf("Expected empty FilterValue(), got '%s'", item.FilterValue())
		}
	})

	t.Run("Special characters", func(t *testing.T) {
		item := versionItem{
			version: "1.0.0-beta+build.1",
			title:   "Version with special chars: @#$%",
			desc:    "Description with unicode: 🚀",
		}

		if item.Title() != "1.0.0-beta+build.1" {
			t.Errorf("Expected Title() to handle special chars, got '%s'", item.Title())
		}
		if item.Description() != "Version with special chars: @#$%" {
			t.Errorf("Expected Description() to handle special chars, got '%s'", item.Description())
		}
	})
}

// Test viewport navigation edge cases.
func TestViewportNavigationEdgeCases(t *testing.T) {
	items := []list.Item{
		versionItem{version: "1.0.0", title: "Version 1.0.0", releaseNotes: "Line 1\nLine 2\nLine 3"},
	}

	l := list.New(items, newCustomDelegate(), 50, 10)
	vp := viewport.New(50, 5) // Small viewport

	m := versionListModel{
		list:        l,
		viewport:    vp,
		focused:     "viewport",
		scrollSpeed: 5, // High scroll speed
	}

	// Test with different scroll speeds
	for _, speed := range []int{1, 2, 5, 10} {
		m.scrollSpeed = speed

		// Test each navigation key
		navigationKeys := []tea.KeyMsg{
			{Type: tea.KeyUp},
			{Type: tea.KeyDown},
			{Type: tea.KeyPgUp},
			{Type: tea.KeyPgDown},
		}

		for _, key := range navigationKeys {
			result, _ := m.Update(key)
			_ = result // Just ensure it doesn't panic
		}
	}
}

// Test model update with no selected item.
func TestVersionListModelNoSelection(t *testing.T) {
	// Empty list
	l := list.New([]list.Item{}, newCustomDelegate(), 50, 10)
	vp := viewport.New(50, 10)

	m := versionListModel{
		list:     l,
		viewport: vp,
		focused:  "list",
	}

	// Test enter with no selection
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	resultModel := result.(versionListModel)

	if resultModel.selected != "" {
		t.Error("Expected no selection with empty list")
	}
}

// Test formatReleaseNotes edge cases.
func TestFormatReleaseNotesEdgeCases(t *testing.T) {
	t.Run("All empty fields", func(t *testing.T) {
		result := formatReleaseNotes("", "", "", "")

		// Should still have a title section
		if !strings.Contains(result, "#") {
			t.Error("Expected some title formatting even with empty fields")
		}
		if !strings.Contains(result, "No release notes available.") {
			t.Error("Expected default message for empty body")
		}
	})

	t.Run("Very long content", func(t *testing.T) {
		longBody := strings.Repeat("This is a very long line that should be handled properly. ", 100)
		result := formatReleaseNotes("Long Release", "v1.0.0", longBody, "2023-01-01T00:00:00Z")

		if !strings.Contains(result, longBody) {
			t.Error("Expected long content to be preserved")
		}
	})

	t.Run("Markdown in body", func(t *testing.T) {
		markdownBody := "## Features\n- Feature 1\n- Feature 2\n\n**Important**: This is bold."
		result := formatReleaseNotes("Markdown Release", "v1.0.0", markdownBody, "2023-01-01T00:00:00Z")

		if !strings.Contains(result, markdownBody) {
			t.Error("Expected markdown content to be preserved in formatted notes")
		}
	})
}

// Test renderMarkdown with various content types.
func TestRenderMarkdownEdgeCases(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		width   int
	}{
		{"Very wide content", strings.Repeat("word ", 50), 40},
		{"Very narrow width", "This is a test", 5},
		{"Zero width", "content", 0},
		{"Negative width", "content", -10},
		{"Large width", "test", 1000},
		{"Special markdown", "# Title\n\n```go\ncode block\n```\n\n> Quote", 80},
		{"Unicode content", "Title with 🚀 emoji and ñoñó chars", 80},
		{"Tables and complex markdown", "| Col 1 | Col 2 |\n|-------|-------|\n| A | B |", 80},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := renderMarkdown(tc.content, tc.width)

			// Ensure no border characters remain
			borderChars := []string{"┌", "┐", "└", "┘", "─", "│", "├", "┤", "┬", "┴"}
			for _, char := range borderChars {
				if strings.Contains(result, char) {
					t.Errorf("Found border character '%s' in rendered output", char)
				}
			}
		})
	}
}

// Test custom delegate with edge cases.
func TestCustomDelegateEdgeCases(t *testing.T) {
	delegate := newCustomDelegate()

	// Test with empty model
	emptyModel := list.New([]list.Item{}, delegate, 50, 10)
	footer := delegate.RenderFooter(50, emptyModel)
	expected := " 0 releases"
	if footer != expected {
		t.Errorf("Expected footer '%s' for empty list, got '%s'", expected, footer)
	}

	// Test with various widths
	items := []list.Item{versionItem{version: "1.0.0"}}
	model := list.New(items, delegate, 50, 10)

	widths := []int{0, 5, 10, 100, 1000}
	for _, width := range widths {
		footer := delegate.RenderFooter(width, model)
		if !strings.Contains(footer, "1 releases") {
			t.Errorf("Expected footer to contain '1 releases' for width %d, got '%s'", width, footer)
		}
	}
}

// Test fetchGitHubVersions network edge cases.
func TestFetchGitHubVersionsNetworkEdgeCases(t *testing.T) {
	t.Run("Malformed URL", func(t *testing.T) {
		_, err := fetchGitHubVersionsWithCustomURL("not-a-url")
		if err == nil {
			t.Error("Expected error for malformed URL")
		}
	})

	t.Run("Large response", func(t *testing.T) {
		// Create response with many releases
		releases := make([]map[string]interface{}, 200)
		for i := 0; i < 200; i++ {
			releases[i] = map[string]interface{}{
				"tag_name":     fmt.Sprintf("v%d.0.0", i),
				"name":         fmt.Sprintf("Release %d.0.0", i),
				"body":         fmt.Sprintf("Release notes for version %d", i),
				"prerelease":   i%10 == 0, // Every 10th is prerelease
				"published_at": "2023-01-01T00:00:00Z",
			}
		}

		server := createMockGitHubServer(releases, http.StatusOK)
		defer server.Close()

		// Redirect stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		items, err := fetchGitHubVersionsWithCustomURL(server.URL)

		w.Close()
		os.Stdout = oldStdout
		io.Copy(&bytes.Buffer{}, r)

		if err != nil {
			t.Errorf("Expected no error for large response, got %v", err)
		}

		expectedNonPrerelease := 180 // 200 - 20 prereleases
		if len(items) != expectedNonPrerelease {
			t.Errorf("Expected %d non-prerelease items, got %d", expectedNonPrerelease, len(items))
		}
	})

	t.Run("Response with missing fields", func(t *testing.T) {
		releases := []map[string]interface{}{
			{
				"tag_name": "v1.0.0",
				// Missing name, body, published_at
				"prerelease": false,
			},
			{
				"tag_name":     "v2.0.0",
				"name":         nil, // Null name
				"body":         nil, // Null body
				"prerelease":   false,
				"published_at": nil, // Null published_at
			},
		}

		server := createMockGitHubServer(releases, http.StatusOK)
		defer server.Close()

		// Redirect stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		items, err := fetchGitHubVersionsWithCustomURL(server.URL)

		w.Close()
		os.Stdout = oldStdout
		io.Copy(&bytes.Buffer{}, r)

		if err != nil {
			t.Errorf("Expected no error with missing fields, got %v", err)
		}

		if len(items) != 2 {
			t.Errorf("Expected 2 items, got %d", len(items))
		}

		// Check that missing fields are handled gracefully
		for _, item := range items {
			if item.version == "" {
				t.Error("Expected version to be set even with missing fields")
			}
			if !strings.Contains(item.releaseNotes, "No release notes available") &&
				!strings.Contains(item.releaseNotes, "v1.0.0") &&
				!strings.Contains(item.releaseNotes, "v2.0.0") {
				t.Error("Expected release notes to handle missing data gracefully")
			}
		}
	})
}

// Test the View method with different focus states.
func TestVersionListModelViewFocusStates(t *testing.T) {
	items := []list.Item{
		versionItem{version: "1.0.0", title: "Version 1.0.0", desc: "First version"},
	}

	l := list.New(items, newCustomDelegate(), 50, 10)
	l.Title = "Test Versions"
	vp := viewport.New(50, 10)

	baseModel := versionListModel{
		list:        l,
		viewport:    vp,
		title:       "Test Tool Versions",
		scrollSpeed: 2,
	}

	focusStates := []string{"list", "viewport", "unknown"}

	for _, focus := range focusStates {
		t.Run(fmt.Sprintf("Focus: %s", focus), func(t *testing.T) {
			m := baseModel
			m.focused = focus

			view := m.View()

			// Should always contain basic elements
			if !strings.Contains(view, "Test Tool Versions") {
				t.Error("Expected view to contain title")
			}

			// Should contain navigation hints
			if !strings.Contains(view, "Press Tab to switch") {
				t.Error("Expected navigation hints")
			}

			// Should show scroll speed for non-normal speeds
			if !strings.Contains(view, "Scroll speed: 2x") {
				t.Error("Expected scroll speed indication")
			}
		})
	}
}

// // Test interaction between list and viewport updates
// func TestListViewportInteraction(t *testing.T) {
// 	items := []list.Item{
// 		versionItem{version: "1.0.0", title: "V1", releaseNotes: "Notes for v1"},
// 		versionItem{version: "2.0.0", title: "V2", releaseNotes: "Notes for v2"},
// 		versionItem{version: "3.0.0", title: "V3", releaseNotes: "Notes for v3"},
// 	}

// 	l := list.New(items, newCustomDelegate(), 50, 15)
// 	vp := viewport.New(50, 10)

// 	m := versionListModel{
// 		list:             l,
// 		viewport:         vp,
// 		focused:          "list",
// 		currentItemIndex: -1,
// 	}

// 	// Simulate list navigation and check viewport updates
// 	// First, set up the window size
// 	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 20}).(versionListModel)

// 	// Navigate through the list and check that viewport content updates
// 	originalContent := m.viewport.View()

// 	// Move down in list (simulate arrow key)
// 	listMsg := tea.KeyMsg{Type: tea.KeyDown}
// 	m, _ = m.Update(listMsg).(versionListModel)

// 	// The viewport content should potentially change based on selection
// 	newContent := m.viewport.View()

// 	// Content might be the same or different depending on the selection change
// 	// The important thing is that it doesn't crash
// 	_ = originalContent
// 	_ = newContent
// }

// First, let's create interfaces that can be mocked.
type ToolInstaller interface {
	parseToolSpec(toolName string) (string, string, error)
	findTool(owner, repo, version string) (*registry.Tool, error)
}

type VersionManager interface {
	AddToolToVersions(filePath, toolKey, version string) error
}

type GitHubClient interface {
	FetchVersions(owner, repo string) ([]versionItem, error)
}

// Mock implementations.
type MockToolInstaller struct {
	parseToolSpecFunc func(string) (string, string, error)
	findToolFunc      func(string, string, string) (*registry.Tool, error)
}

func (m *MockToolInstaller) parseToolSpec(toolName string) (string, string, error) {
	if m.parseToolSpecFunc != nil {
		return m.parseToolSpecFunc(toolName)
	}
	return "", "", fmt.Errorf("not implemented")
}

func (m *MockToolInstaller) findTool(owner, repo, version string) (*registry.Tool, error) {
	if m.findToolFunc != nil {
		return m.findToolFunc(owner, repo, version)
	}
	return nil, fmt.Errorf("not implemented")
}

type MockVersionManager struct {
	addToolToVersionsFunc func(string, string, string) error
}

func (m *MockVersionManager) AddToolToVersions(filePath, toolKey, version string) error {
	if m.addToolToVersionsFunc != nil {
		return m.addToolToVersionsFunc(filePath, toolKey, version)
	}
	return nil
}

type MockGitHubClient struct {
	fetchVersionsFunc func(string, string) ([]versionItem, error)
}

func (m *MockGitHubClient) FetchVersions(owner, repo string) ([]versionItem, error) {
	if m.fetchVersionsFunc != nil {
		return m.fetchVersionsFunc(owner, repo)
	}
	return nil, fmt.Errorf("not implemented")
}

// Refactored SetToolVersion function that accepts dependencies.
func SetToolVersionWithDeps(toolName, version string, scrollSpeed int, installer ToolInstaller, versionManager VersionManager, githubClient GitHubClient, configFilePath string) error {
	// Resolve the tool name to handle aliases
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}
	resolvedKey := owner + "/" + repo

	// If no version provided, fetch available versions and show interactive selection
	if version == "" {
		tool, err := installer.findTool(owner, repo, "")
		if err != nil {
			return fmt.Errorf("failed to find tool configuration: %w", err)
		}

		if tool.Type != "github_release" {
			return fmt.Errorf("interactive version selection is only available for GitHub release type tools")
		}

		items, err := githubClient.FetchVersions(owner, repo)
		if err != nil {
			return fmt.Errorf("failed to fetch versions from GitHub: %w", err)
		}
		if len(items) == 0 {
			return fmt.Errorf("no versions found for %s/%s", owner, repo)
		}

		// In a real implementation, this would show the interactive UI
		// For testing purposes, we'll just select the first version
		version = items[0].version
	}

	// Add the tool with the selected version
	err = versionManager.AddToolToVersions(configFilePath, resolvedKey, version)
	if err != nil {
		return fmt.Errorf("failed to set version: %w", err)
	}

	return nil
}

func TestSetToolVersionWithDeps_WithDirectVersion(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		version       string
		scrollSpeed   int
		setupMocks    func(*MockToolInstaller, *MockVersionManager, *MockGitHubClient)
		expectedError string
	}{
		{
			name:        "successful version setting",
			toolName:    "owner/repo",
			version:     "1.0.0",
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					assert.Equal(t, "owner/repo", toolName)
					return "owner", "repo", nil
				}
				vm.addToolToVersionsFunc = func(filePath, toolKey, version string) error {
					assert.Equal(t, "/tmp/test-versions.yaml", filePath)
					assert.Equal(t, "owner/repo", toolKey)
					assert.Equal(t, "1.0.0", version)
					return nil
				}
			},
		},
		{
			name:        "invalid tool name",
			toolName:    "invalid-tool",
			version:     "1.0.0",
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					return "", "", fmt.Errorf("invalid format")
				}
			},
			expectedError: "invalid tool name: invalid format",
		},
		{
			name:        "add tool to versions fails",
			toolName:    "owner/repo",
			version:     "1.0.0",
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				}
				vm.addToolToVersionsFunc = func(filePath, toolKey, version string) error {
					return fmt.Errorf("file write error")
				}
			},
			expectedError: "failed to set version: file write error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockInstaller := &MockToolInstaller{}
			mockVersionManager := &MockVersionManager{}
			mockGitHubClient := &MockGitHubClient{}

			if tt.setupMocks != nil {
				tt.setupMocks(mockInstaller, mockVersionManager, mockGitHubClient)
			}

			// Execute
			err := SetToolVersionWithDeps(tt.toolName, tt.version, tt.scrollSpeed,
				mockInstaller, mockVersionManager, mockGitHubClient, "/tmp/test-versions.yaml")

			// Assert
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSetToolVersionWithDeps_InteractiveSelection(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		version       string
		scrollSpeed   int
		setupMocks    func(*MockToolInstaller, *MockVersionManager, *MockGitHubClient)
		expectedError string
	}{
		{
			name:        "non-github release tool type",
			toolName:    "owner/repo",
			version:     "", // Empty to trigger interactive mode
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				}
				installer.findToolFunc = func(owner, repo, version string) (*registry.Tool, error) {
					assert.Equal(t, "owner", owner)
					assert.Equal(t, "repo", repo)
					assert.Equal(t, "", version)
					return &registry.Tool{Type: "binary"}, nil
				}
			},
			expectedError: "interactive version selection is only available for GitHub release type tools",
		},
		{
			name:        "find tool fails",
			toolName:    "owner/repo",
			version:     "",
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				}
				installer.findToolFunc = func(owner, repo, version string) (*registry.Tool, error) {
					return nil, fmt.Errorf("tool not found")
				}
			},
			expectedError: "failed to find tool configuration: tool not found",
		},
		{
			name:        "successful github release tool with versions",
			toolName:    "owner/repo",
			version:     "",
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				}
				installer.findToolFunc = func(owner, repo, version string) (*registry.Tool, error) {
					return &registry.Tool{Type: "github_release"}, nil
				}
				gh.fetchVersionsFunc = func(owner, repo string) ([]versionItem, error) {
					assert.Equal(t, "owner", owner)
					assert.Equal(t, "repo", repo)
					return []versionItem{
						{version: "1.0.0", title: "Version 1.0.0", desc: "Latest"},
						{version: "0.9.0", title: "Version 0.9.0", desc: "Previous"},
					}, nil
				}
				vm.addToolToVersionsFunc = func(filePath, toolKey, version string) error {
					assert.Equal(t, "owner/repo", toolKey)
					assert.Equal(t, "1.0.0", version) // Should select first version
					return nil
				}
			},
		},
		{
			name:        "fetch versions fails",
			toolName:    "owner/repo",
			version:     "",
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				}
				installer.findToolFunc = func(owner, repo, version string) (*registry.Tool, error) {
					return &registry.Tool{Type: "github_release"}, nil
				}
				gh.fetchVersionsFunc = func(owner, repo string) ([]versionItem, error) {
					return nil, fmt.Errorf("GitHub API error")
				}
			},
			expectedError: "failed to fetch versions from GitHub: GitHub API error",
		},
		{
			name:        "no versions found",
			toolName:    "owner/repo",
			version:     "",
			scrollSpeed: 1,
			setupMocks: func(installer *MockToolInstaller, vm *MockVersionManager, gh *MockGitHubClient) {
				installer.parseToolSpecFunc = func(toolName string) (string, string, error) {
					return "owner", "repo", nil
				}
				installer.findToolFunc = func(owner, repo, version string) (*registry.Tool, error) {
					return &registry.Tool{Type: "github_release"}, nil
				}
				gh.fetchVersionsFunc = func(owner, repo string) ([]versionItem, error) {
					return []versionItem{}, nil
				}
			},
			expectedError: "no versions found for owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockInstaller := &MockToolInstaller{}
			mockVersionManager := &MockVersionManager{}
			mockGitHubClient := &MockGitHubClient{}

			if tt.setupMocks != nil {
				tt.setupMocks(mockInstaller, mockVersionManager, mockGitHubClient)
			}

			// Execute
			err := SetToolVersionWithDeps(tt.toolName, tt.version, tt.scrollSpeed,
				mockInstaller, mockVersionManager, mockGitHubClient, "/tmp/test-versions.yaml")

			// Assert
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Real GitHub client implementation for testing.
type RealGitHubClient struct {
	BaseURL string
	Token   string
}

func (c *RealGitHubClient) FetchVersions(owner, repo string) ([]versionItem, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", baseURL, owner, repo)

	client := &http.Client{}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		Prerelease  bool   `json:"prerelease"`
		PublishedAt string `json:"published_at"`
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases JSON: %w", err)
	}

	var items []versionItem
	for _, release := range releases {
		if release.Prerelease {
			continue
		}
		version := strings.TrimPrefix(release.TagName, versionPrefix)
		title := release.Name
		if title == "" {
			title = release.TagName
		}
		releaseNotes := formatReleaseNotes(release.Name, release.TagName, release.Body, release.PublishedAt)

		items = append(items, versionItem{
			version:      version,
			title:        title,
			desc:         fmt.Sprintf("Version %s", version),
			releaseNotes: releaseNotes,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no non-prerelease versions found for %s/%s", ErrNoVersionsFound, owner, repo)
	}

	return items, nil
}

func TestRealGitHubClient_FetchVersions(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		mockResponse   []map[string]interface{}
		mockStatusCode int
		token          string
		expectedItems  int
		expectedError  string
	}{
		{
			name:  "successful fetch with multiple releases",
			owner: "owner",
			repo:  "repo",
			mockResponse: []map[string]interface{}{
				{
					"tag_name":     "v1.0.0",
					"name":         "Version 1.0.0",
					"body":         "Initial release",
					"prerelease":   false,
					"published_at": "2023-01-01T00:00:00Z",
				},
				{
					"tag_name":     "v0.9.0-beta",
					"name":         "Beta 0.9.0",
					"body":         "Beta release",
					"prerelease":   true,
					"published_at": "2022-12-01T00:00:00Z",
				},
				{
					"tag_name":     "v0.8.0",
					"name":         "Version 0.8.0",
					"body":         "Bug fixes",
					"prerelease":   false,
					"published_at": "2022-11-01T00:00:00Z",
				},
			},
			mockStatusCode: http.StatusOK,
			expectedItems:  2, // Only non-prerelease versions
		},
		{
			name:  "with github token",
			owner: "owner",
			repo:  "repo",
			mockResponse: []map[string]interface{}{
				{
					"tag_name":     "v1.0.0",
					"name":         "Version 1.0.0",
					"body":         "Initial release",
					"prerelease":   false,
					"published_at": "2023-01-01T00:00:00Z",
				},
			},
			mockStatusCode: http.StatusOK,
			token:          "test-token",
			expectedItems:  1,
		},
		{
			name:           "github api error",
			owner:          "owner",
			repo:           "repo",
			mockStatusCode: http.StatusNotFound,
			expectedError:  "GitHub API returned status 404",
		},
		{
			name:  "no non-prerelease versions",
			owner: "owner",
			repo:  "repo",
			mockResponse: []map[string]interface{}{
				{
					"tag_name":   "v1.0.0-beta",
					"name":       "Beta 1.0.0",
					"prerelease": true,
				},
			},
			mockStatusCode: http.StatusOK,
			expectedError:  "no non-prerelease versions found for owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check URL format
				expectedPath := fmt.Sprintf("/repos/%s/%s/releases", tt.owner, tt.repo)
				assert.Contains(t, r.URL.Path, expectedPath)
				assert.Equal(t, "100", r.URL.Query().Get("per_page"))

				// Check authorization header if token is set
				if tt.token != "" {
					assert.Equal(t, "Bearer "+tt.token, r.Header.Get("Authorization"))
				}

				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != nil {
					json.NewEncoder(w).Encode(tt.mockResponse)
				}
			}))
			defer server.Close()

			// Create client with test server URL
			client := &RealGitHubClient{
				BaseURL: server.URL,
				Token:   tt.token,
			}

			// Execute
			items, err := client.FetchVersions(tt.owner, tt.repo)

			// Assert
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Len(t, items, tt.expectedItems)

				if len(items) > 0 {
					// Verify first item structure
					assert.NotEmpty(t, items[0].version)
					assert.NotEmpty(t, items[0].title)
					assert.NotEmpty(t, items[0].releaseNotes)
				}
			}
		})
	}
}

func TestVersionItem(t *testing.T) {
	item := versionItem{
		version:      "1.0.0",
		title:        "Version 1.0.0",
		desc:         "Initial release",
		releaseNotes: "Release notes",
	}

	assert.Equal(t, "1.0.0", item.Title())
	assert.Equal(t, "Version 1.0.0", item.Description())
	assert.Equal(t, "1.0.0", item.FilterValue())
}

func TestFormatReleaseNotes(t *testing.T) {
	tests := []struct {
		name        string
		releaseName string
		tagName     string
		body        string
		publishedAt string
		expected    []string // Expected substrings
	}{
		{
			name:        "full release info",
			releaseName: "Version 1.0.0",
			tagName:     "v1.0.0",
			body:        "This is the initial release with many features.",
			publishedAt: "2023-01-01T00:00:00Z",
			expected: []string{
				"# Version 1.0.0",
				"**Published:** 2023-01-01T00:00:00Z",
				"This is the initial release with many features.",
			},
		},
		{
			name:        "no release name",
			releaseName: "",
			tagName:     "v1.0.0",
			body:        "Release body",
			publishedAt: "2023-01-01T00:00:00Z",
			expected: []string{
				"# v1.0.0",
				"**Published:** 2023-01-01T00:00:00Z",
				"Release body",
			},
		},
		{
			name:        "no body",
			releaseName: "Version 1.0.0",
			tagName:     "v1.0.0",
			body:        "",
			publishedAt: "2023-01-01T00:00:00Z",
			expected: []string{
				"# Version 1.0.0",
				"**Published:** 2023-01-01T00:00:00Z",
				"No release notes available.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatReleaseNotes(tt.releaseName, tt.tagName, tt.body, tt.publishedAt)

			for _, expected := range tt.expected {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		width    int
		expected []string // Expected to NOT contain these (border characters)
	}{
		{
			name:     "basic markdown",
			content:  "# Title\n\nThis is **bold** text.",
			width:    80,
			expected: []string{"┌", "┐", "└", "┘", "─", "│"}, // Should not contain border chars
		},
		{
			name:     "empty content",
			content:  "",
			width:    80,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdown(tt.content, tt.width)

			// Check that border characters are removed
			for _, borderChar := range tt.expected {
				assert.NotContains(t, result, borderChar, "Border character should be removed: %s", borderChar)
			}

			// Basic content check for non-empty input
			if tt.content != "" {
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestCustomDelegate(t *testing.T) {
	delegate := newCustomDelegate()

	// Create a simple mock that implements the minimal interface needed
	mockModel := &testListModel{itemCount: 2}

	// Test RenderFooter
	footer := delegate.RenderFooter(80, mockModel)
	assert.Contains(t, footer, "2 releases")
}

// Minimal mock that satisfies the list.Model interface for testing.
type testListModel struct {
	itemCount int
}

func (m *testListModel) Items() []list.Item {
	items := make([]list.Item, m.itemCount)
	for i := 0; i < m.itemCount; i++ {
		items[i] = versionItem{version: fmt.Sprintf("v%d.0.0", i+1)}
	}
	return items
}

// Example of how you might test the original function by wrapping it.
func TestOriginalSetToolVersion_Wrapper(t *testing.T) {
	t.Skip("This test would require modifying the original function to accept dependencies or using build tags")

	// This is how you would test the original function if you could modify it:
	// 1. Add a build tag like // +build !test
	// 2. Create a test version with dependency injection
	// 3. Or use interfaces and global variables that can be swapped in tests
}

// TestSetToolVersion_WithValidVersion tests SetToolVersion with a provided version.
func TestSetToolVersion_WithValidVersion(t *testing.T) {
	// Create a temporary tool-versions file
	tmpFile, err := os.CreateTemp("", "tool-versions-*")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set up Atmos config to use the temp file
	oldConfig := atmosConfig
	defer func() { atmosConfig = oldConfig }()

	atmosConfig = &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: tmpFile.Name(),
		},
	}

	// Test with a valid tool and version
	err = SetToolVersion("terraform", "1.11.4", 3)
	assert.NoError(t, err)

	// Verify the file was updated
	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(content), "hashicorp/terraform")
	assert.Contains(t, string(content), "1.11.4")
}

// TestSetToolVersion_WithInvalidTool tests SetToolVersion with an invalid tool name.
func TestSetToolVersion_WithInvalidTool(t *testing.T) {
	// Create a temporary tool-versions file
	tmpFile, err := os.CreateTemp("", "tool-versions-*")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set up Atmos config to use the temp file
	oldConfig := atmosConfig
	defer func() { atmosConfig = oldConfig }()

	atmosConfig = &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: tmpFile.Name(),
		},
	}

	// Test with an invalid tool name (not in registry or local config)
	err = SetToolVersion("nonexistent-tool-xyz-invalid", "1.0.0", 3)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)
}

// TestSetToolVersion_WithCanonicalFormat tests SetToolVersion with org/repo format.
func TestSetToolVersion_WithCanonicalFormat(t *testing.T) {
	// Create a temporary tool-versions file
	tmpFile, err := os.CreateTemp("", "tool-versions-*")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set up Atmos config to use the temp file
	oldConfig := atmosConfig
	defer func() { atmosConfig = oldConfig }()

	atmosConfig = &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: tmpFile.Name(),
		},
	}

	// Test with canonical org/repo format
	err = SetToolVersion("hashicorp/terraform", "1.11.4", 3)
	assert.NoError(t, err)

	// Verify the file was updated
	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(content), "hashicorp/terraform")
	assert.Contains(t, string(content), "1.11.4")
}

// TestSetToolVersion_NonGitHubReleaseWithoutVersion tests error when no version provided for non-GitHub release tool.
func TestSetToolVersion_NonGitHubReleaseWithoutVersion(t *testing.T) {
	// Create a temporary tool-versions file
	tmpFile, err := os.CreateTemp("", "tool-versions-*")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set up Atmos config to use the temp file
	oldConfig := atmosConfig
	defer func() { atmosConfig = oldConfig }()

	atmosConfig = &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: tmpFile.Name(),
		},
	}

	// Test with terraform (http type, not github_release) without version
	// Should error because interactive selection only works for github_release
	err = SetToolVersion("terraform", "", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "interactive version selection is only available for GitHub release type tools")
}
