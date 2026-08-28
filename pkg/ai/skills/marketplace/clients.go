package marketplace

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

// Supported AI clients for skill distribution.
const (
	ClientClaudeCode = "claude-code"
	ClientVSCode     = "vscode" // Also covers GitHub Copilot in VS Code.
	ClientGemini     = "gemini"
)

// Distribution scopes for `atmos ai skill install`/`uninstall` -- mirrors
// pkg/mcp/install's ScopeProject/ScopeUser, but defined locally here rather
// than importing pkg/mcp/install just for two strings.
const (
	ScopeProject = "project"
	ScopeUser    = "user"
)

// Repeated directory-name fragments used across the project/user skill and
// signal leaf tables below.
const (
	skillsLeafName = "skills"
	dotClaude      = ".claude"
	dotGemini      = ".gemini"
)

// SupportedClients lists every AI client `atmos ai skill install`/`uninstall`
// can distribute a skill to.
//
// Deliberately excluded: Cursor (no native project-local skill file format --
// curated marketplace only, per docs/prd/atmos-agent-skills.md) and Codex
// (needs an AGENTS.md index/routing-table merge, not a plain directory copy --
// a structurally different write strategy left for follow-up work).
var SupportedClients = []string{ClientClaudeCode, ClientVSCode, ClientGemini}

// projectSkillLeaf returns a client's project-scoped skill directory, relative
// to basePath.
func projectSkillLeaf(client string) string {
	switch client {
	case ClientClaudeCode:
		return filepath.Join(dotClaude, skillsLeafName)
	case ClientVSCode:
		return filepath.Join(".github", skillsLeafName)
	case ClientGemini:
		return filepath.Join(dotGemini, skillsLeafName)
	default:
		return ""
	}
}

// userSkillLeaf returns a client's user-scoped (personal) skill directory,
// relative to homeDir. Note vscode's user-scope leaf is .copilot/skills, NOT
// .github/skills -- GitHub Copilot's personal directory
// (docs.github.com/en/copilot/concepts/agents/about-agent-skills,
// code.visualstudio.com/docs/agent-customization/agent-skills) is a distinct
// path from its project-level one.
func userSkillLeaf(client string) string {
	switch client {
	case ClientClaudeCode:
		return filepath.Join(dotClaude, skillsLeafName)
	case ClientVSCode:
		return filepath.Join(".copilot", skillsLeafName)
	case ClientGemini:
		return filepath.Join(dotGemini, skillsLeafName)
	default:
		return ""
	}
}

// clientSkillDir returns the well-known directory a client auto-discovers
// installed skills from, at the given scope: project-relative to basePath, or
// user-relative to homeDir.
func clientSkillDir(basePath, homeDir, scope, client string) string {
	root, leaf := basePath, projectSkillLeaf(client)
	if scope == ScopeUser {
		root, leaf = homeDir, userSkillLeaf(client)
	}
	if leaf == "" {
		return ""
	}
	return filepath.Join(root, leaf)
}

// projectSignalLeaf returns the project-scoped directory whose presence
// indicates a client is used by this project -- the same signal directories
// pkg/mcp/install uses for MCP client detection. Detection intentionally
// checks .vscode/ (not .github/, which exists in most GitHub-hosted repos
// regardless of editor and would over-trigger).
func projectSignalLeaf(client string) string {
	switch client {
	case ClientClaudeCode:
		return dotClaude
	case ClientVSCode:
		return ".vscode"
	case ClientGemini:
		return dotGemini
	default:
		return ""
	}
}

// userSignalLeaf is projectSignalLeaf's user-scope (global) counterpart, with
// the vscode client's user-scope signal being .copilot (there's no
// project-level ".copilot" concept); claude-code/gemini use the same leaf
// name at both scopes.
func userSignalLeaf(client string) string {
	switch client {
	case ClientClaudeCode:
		return dotClaude
	case ClientVSCode:
		return ".copilot"
	case ClientGemini:
		return dotGemini
	default:
		return ""
	}
}

// clientSignalDir returns the directory whose presence indicates a client is
// used, at the given scope: project-relative to basePath, or user-relative to
// homeDir.
func clientSignalDir(basePath, homeDir, scope, client string) string {
	root, leaf := basePath, projectSignalLeaf(client)
	if scope == ScopeUser {
		root, leaf = homeDir, userSignalLeaf(client)
	}
	if leaf == "" {
		return ""
	}
	return filepath.Join(root, leaf)
}

// DetectClients returns which supported clients appear to be in use, based on
// well-known signal directories, at the given scope, with homeDir resolved
// internally via homedir.Dir() when passed "" (best-effort: on error,
// detection simply proceeds with an empty homeDir, matching
// pkg/mcp/install.DetectClients's lazy-resolve precedent).
func DetectClients(basePath, homeDir, scope string) []string {
	if homeDir == "" {
		if home, err := homedir.Dir(); err == nil {
			homeDir = home
		}
	}

	var detected []string
	for _, client := range SupportedClients {
		if _, err := os.Stat(clientSignalDir(basePath, homeDir, scope, client)); err == nil {
			detected = append(detected, client)
		}
	}
	return detected
}

// isSymlink reports whether path exists and is a symbolic link, without
// following it. Used to guard writes/removals against pre-existing symlinks
// at a distribution or install target -- e.g. this repo's own
// .claude/skills/<name> entries, which intentionally point into
// agent-skills/skills/<name> for contributor auto-discovery and must never
// be written through or deleted.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}
