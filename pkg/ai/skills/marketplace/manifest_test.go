package marketplace_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot resolves the Atmos repo root from the test's working directory.
// Tests run with CWD = the package directory, which is 4 levels deep.
func repoRoot(t *testing.T) string {
	t.Helper()
	// pkg/ai/skills/marketplace  ->  ../../../..
	abs, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	// Sanity-check: repo root must contain the go.mod file.
	_, err = os.Stat(filepath.Join(abs, "go.mod"))
	require.NoError(t, err, "expected repo root at %s but go.mod missing", abs)
	return abs
}

// TestMarketplaceManifestShape validates the root-level marketplace.json that
// Claude Code reads when a user runs `/plugin marketplace add cloudposse/atmos`.
// Drift in the manifest shape breaks remote plugin discovery — this test fails
// loudly before any drift reaches users.
func TestMarketplaceManifestShape(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".claude-plugin", "marketplace.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "marketplace.json must exist at repo root")

	var m struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Owner       struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"owner"`
		Plugins []struct {
			Name        string `json:"name"`
			Source      string `json:"source"`
			Description string `json:"description"`
			Category    string `json:"category"`
		} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "cloudposse", m.Name)
	assert.NotEmpty(t, m.Version)
	assert.NotEmpty(t, m.Description)
	assert.NotEmpty(t, m.Owner.Name)

	// At least the atmos plugin must be declared, pointing at ./agent-skills.
	require.Len(t, m.Plugins, 1, "expected a single atmos plugin entry")
	assert.Equal(t, "atmos", m.Plugins[0].Name)
	assert.Equal(t, "./agent-skills", m.Plugins[0].Source)
	assert.Contains(t, m.Plugins[0].Description, "Atmos")
}

// TestPluginManifestShape validates agent-skills/.claude-plugin/plugin.json.
// Claude Code reads this file after resolving the plugin from the marketplace
// to learn the plugin's identity and permissions.
func TestPluginManifestShape(t *testing.T) {
	path := filepath.Join(repoRoot(t), "agent-skills", ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "plugin.json must exist under agent-skills/.claude-plugin")

	var p struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Description string   `json:"description"`
		License     string   `json:"license"`
		Homepage    string   `json:"homepage"`
		Repository  string   `json:"repository"`
		Keywords    []string `json:"keywords"`
		Author      struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"author"`
	}
	require.NoError(t, json.Unmarshal(data, &p))

	assert.Equal(t, "atmos", p.Name)
	assert.NotEmpty(t, p.Version)
	assert.NotEmpty(t, p.License, "license must be declared for marketplace indexing")
	assert.Contains(t, p.Description, "Atmos Pro", "description should surface Atmos Pro onboarding")
	assert.Contains(t, p.Keywords, "atmos")
	assert.True(t, strings.HasPrefix(p.Homepage, "https://"), "homepage must be an https URL")
	assert.True(t, strings.HasPrefix(p.Repository, "https://"), "repository must be an https URL")
}
