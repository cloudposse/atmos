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
	yaml "gopkg.in/yaml.v3"

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

func TestDetectClients_ClaudeCodeViaClaudeDir(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	assert.Empty(t, DetectClients(base, home, ScopeProject))
	assert.Empty(t, DetectClients(base, home, ScopeUser))

	require.NoError(t, os.MkdirAll(filepath.Join(base, ".claude", "agents"), 0o755))
	assert.Equal(t, []string{ClientClaudeCode}, DetectClients(base, home, ScopeProject))

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	assert.Equal(t, []string{ClientClaudeCode}, DetectClients(base, home, ScopeUser))
}

func TestInstallJSONTarget_HTTPAndStdio(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientVSCode}),
		WithOverwrite(true),
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
	// No toolchain PATH is ever injected by the installer -- installed configs
	// must stay portable across contributors' machines, so aws-docs has no env at all.
	assert.NotContains(t, parsed.Servers["aws-docs"], "env")
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
	result, err = overwrite.Install(map[string]schema.MCPServerConfig{
		"test": {Command: "new"},
	})
	require.NoError(t, err)
	assert.Contains(t, result.UpdatedServers, "claude-code:test")
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"new"`)
}

func TestInstallJSONTarget_AddedThenUnchanged(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
		WithOnConflict(func(Target, string) (bool, error) {
			t.Fatal("OnConflict must not be called for a new or identical entry")
			return false, nil
		}),
	)
	require.NoError(t, err)

	servers := map[string]schema.MCPServerConfig{"test": {Command: "atmos"}}
	result, err := installer.Install(servers)
	require.NoError(t, err)
	assert.Contains(t, result.AddedServers, "claude-code:test")
	assert.Empty(t, result.UpdatedServers)
	assert.Empty(t, result.UnchangedServers)

	// Re-running with identical config must report Unchanged and must not
	// trigger the OnConflict callback (it would panic the test if it did) or
	// touch the file.
	before, err := os.ReadFile(filepath.Join(base, ".mcp.json"))
	require.NoError(t, err)

	result, err = installer.Install(servers)
	require.NoError(t, err)
	assert.Empty(t, result.AddedServers)
	assert.Empty(t, result.UpdatedServers)
	assert.Contains(t, result.UnchangedServers, "claude-code:test")

	after, err := os.ReadFile(filepath.Join(base, ".mcp.json"))
	require.NoError(t, err)
	assert.Equal(t, before, after, "re-installing an identical entry must not rewrite the file")
}

func TestInstallTOMLTarget_AddedUpdatedUnchanged(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientCodex}),
	)
	require.NoError(t, err)

	servers := map[string]schema.MCPServerConfig{
		"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
	}
	result, err := installer.Install(servers)
	require.NoError(t, err)
	assert.Contains(t, result.AddedServers, "codex:atmos-pro")

	// Re-running with identical config reports Unchanged and doesn't rewrite the file.
	path := filepath.Join(base, ".codex", "config.toml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	result, err = installer.Install(servers)
	require.NoError(t, err)
	assert.Contains(t, result.UnchangedServers, "codex:atmos-pro")
	assert.Empty(t, result.UpdatedServers)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	// Changing the config and forcing overwrite reports Updated.
	forced, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientCodex}),
		WithOverwrite(true),
	)
	require.NoError(t, err)
	result, err = forced.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp/v2"},
	})
	require.NoError(t, err)
	assert.Contains(t, result.UpdatedServers, "codex:atmos-pro")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `url = "https://atmos-pro.com/mcp/v2"`)
}

func TestInstallDryRun_ReportsPerServerStatusWithoutWriting(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"existing":{"command":"old"}}}`), 0o600))

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
		WithDryRun(true),
	)
	require.NoError(t, err)

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"existing": {Command: "new"},
		"fresh":    {Command: "atmos"},
	})
	require.NoError(t, err)
	assert.Contains(t, result.AddedServers, "claude-code:fresh")
	assert.Contains(t, result.UpdatedServers, "claude-code:existing")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{"mcpServers":{"existing":{"command":"old"}}}`, string(data), "dry-run must not write")
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

func TestUninstallJSONTarget_RemovesEntries(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"keep":{"command":"a"},"drop":{"command":"b"}}}`), 0o600))

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
	)
	require.NoError(t, err)

	result, err := installer.Uninstall([]string{"drop"})
	require.NoError(t, err)
	assert.Contains(t, result.RemovedServers, "claude-code:drop")
	assert.Contains(t, result.UpdatedFiles, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Contains(t, parsed.Servers, "keep")
	assert.NotContains(t, parsed.Servers, "drop")
}

func TestUninstallJSONTarget_LeavesEmptyMapNotDeletedFile(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"only":{"command":"a"}}}`), 0o600))

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
	)
	require.NoError(t, err)

	result, err := installer.Uninstall([]string{"only"})
	require.NoError(t, err)
	assert.Contains(t, result.RemovedServers, "claude-code:only")

	data, err := os.ReadFile(path)
	require.NoError(t, err, "file must not be deleted")
	var parsed struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Empty(t, parsed.Servers)
}

func TestUninstallTOMLTarget_ReusesRemoveTOMLServer(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("[mcp_servers.keep]\ncommand = \"a\"\n\n[mcp_servers.drop]\ncommand = \"b\"\n"), 0o600))

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientCodex}),
	)
	require.NoError(t, err)

	result, err := installer.Uninstall([]string{"drop"})
	require.NoError(t, err)
	assert.Contains(t, result.RemovedServers, "codex:drop")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "[mcp_servers.keep]")
	assert.NotContains(t, content, "[mcp_servers.drop]")
}

func TestUninstall_NotFoundServersReported(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"keep":{"command":"a"}}}`), 0o600))

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
	)
	require.NoError(t, err)

	result, err := installer.Uninstall([]string{"missing"})
	require.NoError(t, err)
	assert.Contains(t, result.NotFoundServers, "claude-code:missing")
	assert.Empty(t, result.RemovedServers)
	assert.Empty(t, result.UpdatedFiles)
}

func TestUninstall_NotFoundWhenFileDoesNotExist(t *testing.T) {
	base := t.TempDir()

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
	)
	require.NoError(t, err)

	result, err := installer.Uninstall([]string{"anything"})
	require.NoError(t, err)
	assert.Contains(t, result.NotFoundServers, "claude-code:anything")
}

func TestUninstall_DryRun(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".mcp.json")
	original := `{"mcpServers":{"drop":{"command":"a"}}}`
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientClaudeCode}),
		WithDryRun(true),
	)
	require.NoError(t, err)

	result, err := installer.Uninstall([]string{"drop"})
	require.NoError(t, err)
	assert.Contains(t, result.RemovedServers, "claude-code:drop", "dry-run still reports what would be removed")
	assert.Empty(t, result.UpdatedFiles, "dry-run must not write")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(data))
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

// --- Part C: expanded client list -------------------------------------

func TestNew_WithAllClients_SelectsAllSupportedClients(t *testing.T) {
	installer, err := New(
		WithBasePath(t.TempDir()),
		WithHomeDir(t.TempDir()),
		WithAllClients(true),
	)
	require.NoError(t, err)
	assert.ElementsMatch(t, SupportedClients, installer.opts.Clients,
		"--all-clients must resolve to every client in the (now expanded) SupportedClients registry")
}

func TestNormalizeClient_NewClientsAndAliases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"claude-desktop literal", "claude-desktop", ClientClaudeDesktop},
		{"windsurf literal", "windsurf", ClientWindsurf},
		{"cline literal", "cline", ClientCline},
		{"cline-cli literal", "cline-cli", ClientClineCLI},
		{"zed literal", "zed", ClientZed},
		{"opencode literal", "opencode", ClientOpenCode},
		{"opencode alias", "open-code", ClientOpenCode},
		{"goose literal", "goose", ClientGoose},
		{"antigravity literal", "antigravity", ClientAntigravity},
		{"mcporter literal", "mcporter", ClientMCPorter},
		{"uppercase and whitespace normalize", "  ZED  ", ClientZed},
		// The VS Code extension keeps its historical alias.
		{"github-copilot still means the VS Code extension", "github-copilot", ClientVSCode},
		// github-copilot-cli used to alias onto ClientVSCode; it now means
		// the standalone `copilot` CLI product instead -- a deliberate
		// behavior change (see the comment on ClientCopilotCLI).
		{"github-copilot-cli now means the standalone copilot-cli client", "github-copilot-cli", ClientCopilotCLI},
		{"copilot-cli literal", "copilot-cli", ClientCopilotCLI},
		{"copilot alias", "copilot", ClientCopilotCLI},
		{"unknown client", "not-a-real-client", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeClient(tt.input))
		})
	}
}

func TestResolveTarget_NewProjectAndUserScopeClients(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	tests := []struct {
		name       string
		client     string
		wantRoot   string
		wantFormat string
		projectHas string // expected suffix of the project-scope path.
		userHas    string // expected suffix of the user-scope path.
	}{
		{
			name:       "zed",
			client:     ClientZed,
			wantRoot:   "context_servers",
			wantFormat: "json",
			projectHas: filepath.Join(".zed", "settings.json"),
			userHas:    filepath.Join("zed", "settings.json"),
		},
		{
			name:       "opencode",
			client:     ClientOpenCode,
			wantRoot:   "mcp",
			wantFormat: "json",
			projectHas: "opencode.json",
			userHas:    filepath.Join("opencode", "opencode.json"),
		},
		{
			name:       "goose",
			client:     ClientGoose,
			wantRoot:   "extensions",
			wantFormat: "yaml",
			projectHas: filepath.Join(".goose", "config.yaml"),
			userHas:    filepath.Join("goose", "config.yaml"),
		},
		{
			name:       "mcporter",
			client:     ClientMCPorter,
			wantRoot:   "mcpServers",
			wantFormat: "json",
			projectHas: filepath.Join("config", "mcporter.json"),
			userHas:    filepath.Join(".mcporter", "mcporter.json"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := ResolveTarget(base, home, ScopeProject, tt.client)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRoot, project.Root)
			assert.Equal(t, tt.wantFormat, project.Format)
			assert.True(t, strings.HasSuffix(filepath.ToSlash(project.Path), filepath.ToSlash(tt.projectHas)),
				"project path %q must end with %q", project.Path, tt.projectHas)

			user, err := ResolveTarget(base, home, ScopeUser, tt.client)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRoot, user.Root)
			assert.Equal(t, tt.wantFormat, user.Format)
			assert.True(t, strings.HasSuffix(filepath.ToSlash(user.Path), filepath.ToSlash(tt.userHas)),
				"user path %q must end with %q", user.Path, tt.userHas)
		})
	}
}

func TestResolveTarget_GlobalOnlyClients(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	tests := []struct {
		name    string
		client  string
		userHas string // expected suffix of the user-scope path.
	}{
		{"claude-desktop", ClientClaudeDesktop, "claude_desktop_config.json"},
		{"windsurf", ClientWindsurf, filepath.Join("windsurf", "mcp_config.json")},
		{"cline", ClientCline, filepath.Join("saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")},
		{"cline-cli", ClientClineCLI, filepath.Join(".cline", "data", "settings", "cline_mcp_settings.json")},
		{"copilot-cli", ClientCopilotCLI, filepath.Join(".copilot", "mcp-config.json")},
		{"antigravity", ClientAntigravity, filepath.Join(".gemini", "antigravity", "mcp_config.json")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Project scope is rejected outright, not silently resolved to
			// a useless (or empty) path.
			_, err := ResolveTarget(base, home, ScopeProject, tt.client)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUnsupportedScope)
			assert.Contains(t, err.Error(), "user-scope")

			// User scope resolves to the client's well-known global path.
			target, err := ResolveTarget(base, home, ScopeUser, tt.client)
			require.NoError(t, err)
			assert.Equal(t, "mcpServers", target.Root)
			assert.Equal(t, "json", target.Format)
			assert.True(t, strings.HasSuffix(filepath.ToSlash(target.Path), filepath.ToSlash(tt.userHas)),
				"user path %q must end with %q", target.Path, tt.userHas)
		})
	}
}

func TestDetectClients_GlobalOnlyClientNotDetectedAtProjectScope(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	// A global-only client can never appear in a project-scope detection
	// pass -- ResolveTarget rejects it outright, so DetectClients must skip
	// it rather than error out.
	assert.NotContains(t, DetectClients(base, home, ScopeProject), ClientClaudeDesktop)

	claudeDesktopDir := filepath.Dir(claudeDesktopUserPath(home))
	require.NoError(t, os.MkdirAll(claudeDesktopDir, 0o755))
	require.NoError(t, os.WriteFile(claudeDesktopUserPath(home), []byte(`{}`), 0o600))
	assert.Contains(t, DetectClients(base, home, ScopeUser), ClientClaudeDesktop)
}

func TestInstallJSONTarget_ClaudeDesktopGlobalScope(t *testing.T) {
	home := t.TempDir()
	installer, err := New(
		WithHomeDir(home),
		WithScope(ScopeUser),
		WithClients([]string{ClientClaudeDesktop}),
	)
	require.NoError(t, err)

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"aws-docs": {Command: "uvx", Args: []string{"awslabs.aws-documentation-mcp-server@latest"}},
	})
	require.NoError(t, err)
	require.Len(t, result.CreatedFiles, 1)

	data, err := os.ReadFile(claudeDesktopUserPath(home))
	require.NoError(t, err)
	var parsed struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "uvx", parsed.Servers["aws-docs"]["command"])
}

func TestInstallJSONTarget_ZedContextServersRoot(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientZed}),
	)
	require.NoError(t, err)

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
		"aws-docs":  {Command: "uvx", Args: []string{"awslabs.aws-documentation-mcp-server@latest"}},
	})
	require.NoError(t, err)
	require.Len(t, result.CreatedFiles, 1)

	data, err := os.ReadFile(filepath.Join(base, ".zed", "settings.json"))
	require.NoError(t, err)
	var parsed struct {
		Servers map[string]map[string]any `json:"context_servers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "https://atmos-pro.com/mcp", parsed.Servers["atmos-pro"]["url"])
	assert.Equal(t, "uvx", parsed.Servers["aws-docs"]["command"])

	// A pre-existing settings.json with unrelated Zed settings must survive
	// the merge untouched -- context_servers is the only key rewritten.
	require.NoError(t, os.WriteFile(filepath.Join(base, ".zed", "settings.json"),
		[]byte(`{"context_servers":{},"theme":"One Dark"}`), 0o600))
	overwrite, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientZed}),
		WithOverwrite(true),
	)
	require.NoError(t, err)
	_, err = overwrite.Install(map[string]schema.MCPServerConfig{
		"aws-docs": {Command: "uvx"},
	})
	require.NoError(t, err)
	data, err = os.ReadFile(filepath.Join(base, ".zed", "settings.json"))
	require.NoError(t, err)
	var merged map[string]any
	require.NoError(t, json.Unmarshal(data, &merged))
	assert.Equal(t, "One Dark", merged["theme"], "unrelated Zed settings must be preserved")
}

func TestInstallJSONTarget_OpenCodeBespokeEntryShape(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientOpenCode}),
	)
	require.NoError(t, err)

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp", Headers: map[string]string{"Authorization": "Bearer x"}},
		"aws-docs":  {Command: "uvx", Args: []string{"awslabs.aws-documentation-mcp-server@latest"}, Env: map[string]string{"FOO": "bar"}},
	})
	require.NoError(t, err)
	require.Len(t, result.CreatedFiles, 1)

	data, err := os.ReadFile(filepath.Join(base, "opencode.json"))
	require.NoError(t, err)
	var parsed struct {
		MCP map[string]map[string]any `json:"mcp"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))

	remote := parsed.MCP["atmos-pro"]
	assert.Equal(t, "remote", remote["type"])
	assert.Equal(t, "https://atmos-pro.com/mcp", remote["url"])
	assert.Equal(t, true, remote["enabled"])
	assert.NotContains(t, remote, "command", "remote entries must not carry a command field")

	local := parsed.MCP["aws-docs"]
	assert.Equal(t, "local", local["type"])
	assert.Equal(t, true, local["enabled"])
	command, ok := local["command"].([]any)
	require.True(t, ok, "OpenCode's command field must be a combined array, not a bare string")
	require.Len(t, command, 2)
	assert.Equal(t, "uvx", command[0])
	assert.Equal(t, "awslabs.aws-documentation-mcp-server@latest", command[1])
	env, ok := local["environment"].(map[string]any)
	require.True(t, ok, "env vars must live under \"environment\", not \"env\"")
	assert.Equal(t, "bar", env["FOO"])
}

func TestInstallYAMLTarget_GooseExtensionsRoot(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientGoose}),
	)
	require.NoError(t, err)

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
		"aws-docs":  {Command: "uvx", Args: []string{"awslabs.aws-documentation-mcp-server@latest"}, Env: map[string]string{"FOO": "bar"}},
	})
	require.NoError(t, err)
	require.Len(t, result.CreatedFiles, 1)

	path := filepath.Join(base, ".goose", "config.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed struct {
		Extensions map[string]map[string]any `yaml:"extensions"`
	}
	require.NoError(t, yaml.Unmarshal(data, &parsed))

	local := parsed.Extensions["aws-docs"]
	assert.Equal(t, "stdio", local["type"])
	assert.Equal(t, "uvx", local["cmd"])
	assert.Equal(t, true, local["enabled"])
	envs, ok := local["envs"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "bar", envs["FOO"])

	remote := parsed.Extensions["atmos-pro"]
	assert.Equal(t, "streamable_http", remote["type"])
	assert.Equal(t, "https://atmos-pro.com/mcp", remote["uri"])
	assert.Equal(t, true, remote["enabled"])

	// Re-running with identical config must report Unchanged and not
	// rewrite the file, exactly like the JSON/TOML targets.
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	result, err = installer.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
		"aws-docs":  {Command: "uvx", Args: []string{"awslabs.aws-documentation-mcp-server@latest"}, Env: map[string]string{"FOO": "bar"}},
	})
	require.NoError(t, err)
	assert.Contains(t, result.UnchangedServers, "goose:aws-docs")
	assert.Contains(t, result.UnchangedServers, "goose:atmos-pro")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestUninstallYAMLTarget_RemovesGooseExtension(t *testing.T) {
	base := t.TempDir()
	installer, err := New(
		WithBasePath(base),
		WithHomeDir(t.TempDir()),
		WithClients([]string{ClientGoose}),
	)
	require.NoError(t, err)

	_, err = installer.Install(map[string]schema.MCPServerConfig{
		"keep": {Command: "a"},
		"drop": {Command: "b"},
	})
	require.NoError(t, err)

	result, err := installer.Uninstall([]string{"drop"})
	require.NoError(t, err)
	assert.Contains(t, result.RemovedServers, "goose:drop")

	path := filepath.Join(base, ".goose", "config.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed struct {
		Extensions map[string]map[string]any `yaml:"extensions"`
	}
	require.NoError(t, yaml.Unmarshal(data, &parsed))
	assert.Contains(t, parsed.Extensions, "keep")
	assert.NotContains(t, parsed.Extensions, "drop")

	// Uninstalling a server that was never there is reported, not an error.
	result, err = installer.Uninstall([]string{"never-existed"})
	require.NoError(t, err)
	assert.Contains(t, result.NotFoundServers, "goose:never-existed")
}

func TestDetectClients_OpenCodeRequiresConfigFileNotProjectRootDir(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	// opencode.json lives directly at the project root, so an empty
	// project must not falsely detect OpenCode just because the project
	// root itself (trivially) exists.
	assert.NotContains(t, DetectClients(base, home, ScopeProject), ClientOpenCode)

	require.NoError(t, os.WriteFile(filepath.Join(base, "opencode.json"), []byte(`{}`), 0o600))
	assert.Contains(t, DetectClients(base, home, ScopeProject), ClientOpenCode)
}

func TestDetectClients_MCPorterRequiresConfigFileNotGenericConfigDir(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	// A "config/" directory is common in unrelated projects (e.g. Atmos
	// stack config dirs); it must not be treated as an mcporter signal on
	// its own.
	require.NoError(t, os.MkdirAll(filepath.Join(base, "config"), 0o755))
	assert.NotContains(t, DetectClients(base, home, ScopeProject), ClientMCPorter)

	require.NoError(t, os.WriteFile(filepath.Join(base, "config", "mcporter.json"), []byte(`{}`), 0o600))
	assert.Contains(t, DetectClients(base, home, ScopeProject), ClientMCPorter)
}

func TestSupportedClients_AllResolveAtTheirSupportedScopes(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()

	for _, client := range SupportedClients {
		t.Run(client, func(t *testing.T) {
			_, userErr := ResolveTarget(base, home, ScopeUser, client)
			assert.NoError(t, userErr, "every supported client must resolve at user scope")

			_, projectErr := ResolveTarget(base, home, ScopeProject, client)
			if globalOnlyClients[client] {
				assert.Error(t, projectErr)
			} else {
				assert.NoError(t, projectErr)
			}
		})
	}
}
