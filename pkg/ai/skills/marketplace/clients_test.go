package marketplace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestClientSkillDir(t *testing.T) {
	base := filepath.Join("some", "project")
	home := filepath.Join("some", "home")

	tests := []struct {
		name   string
		client string
		scope  string
		want   string
	}{
		{"claude-code project scope", ClientClaudeCode, ScopeProject, filepath.Join(base, ".claude", "skills")},
		{"vscode project scope", ClientVSCode, ScopeProject, filepath.Join(base, ".github", "skills")},
		{"gemini project scope", ClientGemini, ScopeProject, filepath.Join(base, ".gemini", "skills")},
		{"unsupported client project scope returns empty", "unknown", ScopeProject, ""},
		{"claude-code user scope", ClientClaudeCode, ScopeUser, filepath.Join(home, ".claude", "skills")},
		{"vscode user scope uses .copilot, not .github", ClientVSCode, ScopeUser, filepath.Join(home, ".copilot", "skills")},
		{"gemini user scope", ClientGemini, ScopeUser, filepath.Join(home, ".gemini", "skills")},
		{"unsupported client user scope returns empty", "unknown", ScopeUser, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clientSkillDir(base, home, tt.scope, tt.client))
		})
	}
}

func TestClientSignalDir(t *testing.T) {
	base := filepath.Join("some", "project")
	home := filepath.Join("some", "home")

	tests := []struct {
		name   string
		client string
		scope  string
		want   string
	}{
		{"claude-code project scope", ClientClaudeCode, ScopeProject, filepath.Join(base, ".claude")},
		{"vscode project scope", ClientVSCode, ScopeProject, filepath.Join(base, ".vscode")},
		{"gemini project scope", ClientGemini, ScopeProject, filepath.Join(base, ".gemini")},
		{"unsupported client project scope returns empty", "unknown", ScopeProject, ""},
		{"claude-code user scope", ClientClaudeCode, ScopeUser, filepath.Join(home, ".claude")},
		{"vscode user scope uses .copilot signal, not .vscode", ClientVSCode, ScopeUser, filepath.Join(home, ".copilot")},
		{"gemini user scope", ClientGemini, ScopeUser, filepath.Join(home, ".gemini")},
		{"unsupported client user scope returns empty", "unknown", ScopeUser, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clientSignalDir(base, home, tt.scope, tt.client))
		})
	}
}

func TestDetectClients(t *testing.T) {
	t.Run("no signal directories present", func(t *testing.T) {
		base := t.TempDir()
		assert.Empty(t, DetectClients(base, "", ScopeProject))
	})

	t.Run("detects claude-code from .claude", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".claude"), 0o755))
		assert.Equal(t, []string{ClientClaudeCode}, DetectClients(base, "", ScopeProject))
	})

	t.Run("detects vscode from .vscode, not .github", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".github"), 0o755))
		assert.Empty(t, DetectClients(base, "", ScopeProject), ".github/ alone must not be treated as a VS Code signal")

		require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
		assert.Equal(t, []string{ClientVSCode}, DetectClients(base, "", ScopeProject))
	})

	t.Run("detects gemini from .gemini", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".gemini"), 0o755))
		assert.Equal(t, []string{ClientGemini}, DetectClients(base, "", ScopeProject))
	})

	t.Run("detects multiple clients in declared order", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".claude"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".gemini"), 0o755))

		assert.Equal(t, []string{ClientClaudeCode, ClientVSCode, ClientGemini}, DetectClients(base, "", ScopeProject))
	})

	t.Run("user scope detects vscode from .copilot, not project .vscode", func(t *testing.T) {
		base := t.TempDir()
		home := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
		assert.Empty(t, DetectClients(base, home, ScopeUser), "project .vscode signal must not leak into user-scope detection")

		require.NoError(t, os.MkdirAll(filepath.Join(home, ".copilot"), 0o755))
		assert.Equal(t, []string{ClientVSCode}, DetectClients(base, home, ScopeUser))
	})

	t.Run("user scope detects claude-code and gemini under home dir", func(t *testing.T) {
		base := t.TempDir()
		home := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(home, ".gemini"), 0o755))

		assert.Equal(t, []string{ClientClaudeCode, ClientGemini}, DetectClients(base, home, ScopeUser))
	})

	t.Run("empty homeDir resolves via homedir.Dir()", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)
		t.Setenv("USERPROFILE", tempHome)
		homedir.Reset()
		t.Cleanup(homedir.Reset)

		require.NoError(t, os.MkdirAll(filepath.Join(tempHome, ".claude"), 0o755))
		assert.Equal(t, []string{ClientClaudeCode}, DetectClients(t.TempDir(), "", ScopeUser))
	})
}

func TestIsSymlink(t *testing.T) {
	t.Run("real symlink", func(t *testing.T) {
		dir := t.TempDir()
		targetFile := filepath.Join(dir, "target.txt")
		require.NoError(t, os.WriteFile(targetFile, []byte("content"), 0o644))

		symlinkPath := filepath.Join(dir, "link")
		if err := os.Symlink(targetFile, symlinkPath); err != nil {
			t.Skipf("Skipping symlink test: %v", err)
		}

		assert.True(t, isSymlink(symlinkPath))
	})

	t.Run("regular file is not a symlink", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "plain.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))

		assert.False(t, isSymlink(filePath))
	})

	t.Run("regular directory is not a symlink", func(t *testing.T) {
		assert.False(t, isSymlink(t.TempDir()))
	})

	t.Run("non-existent path is not a symlink", func(t *testing.T) {
		assert.False(t, isSymlink(filepath.Join(t.TempDir(), "does-not-exist")))
	})

	t.Run("dangling symlink is still a symlink", func(t *testing.T) {
		dir := t.TempDir()
		symlinkPath := filepath.Join(dir, "dangling-link")
		if err := os.Symlink(filepath.Join(dir, "does-not-exist"), symlinkPath); err != nil {
			t.Skipf("Skipping symlink test: %v", err)
		}

		assert.True(t, isSymlink(symlinkPath))
	})
}
