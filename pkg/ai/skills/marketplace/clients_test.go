package marketplace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSkillDir(t *testing.T) {
	base := filepath.Join("some", "project")

	tests := []struct {
		name   string
		client string
		want   string
	}{
		{"claude-code", ClientClaudeCode, filepath.Join(base, ".claude", "skills")},
		{"vscode", ClientVSCode, filepath.Join(base, ".github", "skills")},
		{"gemini", ClientGemini, filepath.Join(base, ".gemini", "skills")},
		{"unsupported client returns empty", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clientSkillDir(base, tt.client))
		})
	}
}

func TestClientSignalDir(t *testing.T) {
	base := filepath.Join("some", "project")

	tests := []struct {
		name   string
		client string
		want   string
	}{
		{"claude-code", ClientClaudeCode, filepath.Join(base, ".claude")},
		{"vscode", ClientVSCode, filepath.Join(base, ".vscode")},
		{"gemini", ClientGemini, filepath.Join(base, ".gemini")},
		{"unsupported client returns empty", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clientSignalDir(base, tt.client))
		})
	}
}

func TestDetectClients(t *testing.T) {
	t.Run("no signal directories present", func(t *testing.T) {
		base := t.TempDir()
		assert.Empty(t, DetectClients(base))
	})

	t.Run("detects claude-code from .claude", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".claude"), 0o755))
		assert.Equal(t, []string{ClientClaudeCode}, DetectClients(base))
	})

	t.Run("detects vscode from .vscode, not .github", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".github"), 0o755))
		assert.Empty(t, DetectClients(base), ".github/ alone must not be treated as a VS Code signal")

		require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
		assert.Equal(t, []string{ClientVSCode}, DetectClients(base))
	})

	t.Run("detects gemini from .gemini", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".gemini"), 0o755))
		assert.Equal(t, []string{ClientGemini}, DetectClients(base))
	})

	t.Run("detects multiple clients in declared order", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".claude"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(base, ".gemini"), 0o755))

		assert.Equal(t, []string{ClientClaudeCode, ClientVSCode, ClientGemini}, DetectClients(base))
	})
}
