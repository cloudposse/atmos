// Package agentskills embeds the official Atmos agent skills into the binary so
// the CLI can list the full catalog and install skills offline — no network or
// Git clone required.
//
// The embed lives next to the skills/ directory (rather than under pkg/) because
// the embed directive cannot reference parent directories with "..".
package agentskills

import "embed"

// Skills contains the bundled official skills under skills/<name>/. Each skill
// has a SKILL.md with YAML frontmatter (per the Agent Skills standard) plus any
// reference files it ships. The "all:" prefix ensures every file is embedded so
// an offline install copies the complete skill, references included.
//
//go:embed all:skills
var Skills embed.FS
