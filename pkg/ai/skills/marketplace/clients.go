package marketplace

import (
	"os"
	"path/filepath"
)

// Supported AI clients for skill distribution.
const (
	ClientClaudeCode = "claude-code"
	ClientVSCode     = "vscode" // Also covers GitHub Copilot in VS Code.
	ClientGemini     = "gemini"
)

// SupportedClients lists every AI client `atmos ai skill install`/`uninstall`
// can distribute a skill to.
//
// Deliberately excluded: Cursor (no native project-local skill file format --
// curated marketplace only, per docs/prd/atmos-agent-skills.md) and Codex
// (needs an AGENTS.md index/routing-table merge, not a plain directory copy --
// a structurally different write strategy left for follow-up work).
var SupportedClients = []string{ClientClaudeCode, ClientVSCode, ClientGemini}

// clientSkillDir returns the well-known project-relative directory a client
// auto-discovers installed skills from.
func clientSkillDir(basePath, client string) string {
	switch client {
	case ClientClaudeCode:
		return filepath.Join(basePath, ".claude", "skills")
	case ClientVSCode:
		return filepath.Join(basePath, ".github", "skills")
	case ClientGemini:
		return filepath.Join(basePath, ".gemini", "skills")
	default:
		return ""
	}
}

// clientSignalDir returns the directory whose presence indicates a client is
// used by this project -- the same signal directories pkg/mcp/install uses
// for MCP client detection. Detection intentionally checks .vscode/ (not
// .github/, which exists in most GitHub-hosted repos regardless of editor and
// would over-trigger).
func clientSignalDir(basePath, client string) string {
	switch client {
	case ClientClaudeCode:
		return filepath.Join(basePath, ".claude")
	case ClientVSCode:
		return filepath.Join(basePath, ".vscode")
	case ClientGemini:
		return filepath.Join(basePath, ".gemini")
	default:
		return ""
	}
}

// DetectClients returns which supported clients appear to be in use in
// basePath, based on well-known signal directories.
func DetectClients(basePath string) []string {
	var detected []string
	for _, client := range SupportedClients {
		if _, err := os.Stat(clientSignalDir(basePath, client)); err == nil {
			detected = append(detected, client)
		}
	}
	return detected
}
