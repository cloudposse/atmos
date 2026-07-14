package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveTarget_ProjectAndUser(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	project, err := ResolveTarget(base, home, ScopeProject, ClientVSCode)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, ".vscode", "mcp.json"), project.Path)
	assert.Equal(t, "servers", project.Root)

	user, err := ResolveTarget(base, home, ScopeUser, ClientCodex)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".codex", "config.toml"), user.Path)
	assert.Equal(t, "toml", user.Format)
}

func TestValidateScope_SystemUnsupported(t *testing.T) {
	err := ValidateScope("system")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "managed/system")
}

func TestDetectClients_DoesNotDetectRootLevelConfigFromExistingRoot(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	assert.Empty(t, DetectClients(base, home, ScopeProject))
	assert.Empty(t, DetectClients(base, home, ScopeUser))

	require.NoError(t, os.MkdirAll(filepath.Join(base, ".cursor"), 0o755))
	assert.Equal(t, []string{ClientCursor}, DetectClients(base, home, ScopeProject))

	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{}`), 0o600))
	assert.Equal(t, []string{ClientClaudeCode}, DetectClients(base, home, ScopeUser))
}

func TestInstallJSONTarget_HTTPAndStdio(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientVSCode}),
		WithOverwrite(true),
		WithToolchainPath("/toolchain/bin"),
	)
	require.NoError(t, err)

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {
			Type: schema.MCPTransportHTTP,
			URL:  "https://atmos-pro.com/mcp",
		},
		"aws-docs": {
			Command: "uvx",
			Args:    []string{"awslabs.aws-documentation-mcp-server@latest"},
		},
	})
	require.NoError(t, err)
	require.Len(t, result.CreatedFiles, 1)

	data, err := os.ReadFile(filepath.Join(base, ".vscode", "mcp.json"))
	require.NoError(t, err)
	var parsed struct {
		Servers map[string]map[string]any `json:"servers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "http", parsed.Servers["atmos-pro"]["type"])
	assert.Equal(t, "https://atmos-pro.com/mcp", parsed.Servers["atmos-pro"]["url"])
	assert.Equal(t, "uvx", parsed.Servers["aws-docs"]["command"])
	env := parsed.Servers["aws-docs"]["env"].(map[string]any)
	assert.Contains(t, env["PATH"], "/toolchain/bin")
}

func TestInstallTOMLTarget_HTTP(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientCodex}),
	)
	require.NoError(t, err)

	_, err = installer.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {
			Type:    schema.MCPTransportHTTP,
			URL:     "https://atmos-pro.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer ${ATMOS_PRO_TOKEN}"},
		},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(base, ".codex", "config.toml"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "[mcp_servers.atmos-pro]")
	assert.Contains(t, content, `url = "https://atmos-pro.com/mcp"`)
	assert.Contains(t, content, "[mcp_servers.atmos-pro.http_headers]")
	assert.Contains(t, content, `"Authorization" = "Bearer ${ATMOS_PRO_TOKEN}"`)
}

func TestInstallConflictSkipAndOverwrite(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"test":{"command":"old"}}}`), 0o600))

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
	)
	require.NoError(t, err)
	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"test": {Command: "new"},
	})
	require.NoError(t, err)
	assert.Contains(t, result.SkippedServers, "claude-code:test")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"old"`)

	overwrite, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
		WithOverwrite(true),
	)
	require.NoError(t, err)
	_, err = overwrite.Install(map[string]schema.MCPServerConfig{
		"test": {Command: "new"},
	})
	require.NoError(t, err)
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"new"`)
}

func TestInstallGitignoreProjectOnly(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientCursor}),
		WithGitignore(true),
	)
	require.NoError(t, err)

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"test": {Command: "echo"},
	})
	require.NoError(t, err)
	assert.Contains(t, result.GitignoredFiles, ".cursor/mcp.json")

	data, err := os.ReadFile(filepath.Join(base, ".gitignore"))
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), ".cursor/mcp.json"))
}

func TestConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode bits are not meaningful on Windows")
	}
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
	)
	require.NoError(t, err)
	_, err = installer.Install(map[string]schema.MCPServerConfig{
		"test": {Command: "echo"},
	})
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(base, ".mcp.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
