// Package agentskills exposes the bundled Atmos agent skills to the Atmos
// binary. It exists in the agent-skills/ directory so that go:embed can pick
// up the sibling skills/ subtree (go:embed cannot traverse ".." or follow
// symlinks, so the embed directive must live alongside the files it captures).
//
// Only the Atmos Go binary imports this package. AI tools that consume skills
// (Claude Code, Copilot, Codex, Gemini, Grok, etc.) ignore the Go source and
// read the same skills/ directory as flat files, per the Agent Skills open
// standard.
package agentskills

import "embed"

// SkillsFS contains every built-in Atmos skill. The "all:" prefix includes
// files that would otherwise be excluded (hidden files, underscore-prefixed
// files) — not required today but defensive against future skill layouts.
//
//go:embed all:skills
var SkillsFS embed.FS
