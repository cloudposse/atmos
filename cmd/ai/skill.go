package ai

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

//go:embed markdown/atmos_ai_skill.md
var skillLongMarkdown string

// SkillCmd represents the 'atmos ai skill' command.
// Exported for use by skill subpackage.
var SkillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage AI skills",
	Long:  skillLongMarkdown,
}

func init() {
	// Add 'skill' subcommand to 'ai' command.
	aiCmd.AddCommand(SkillCmd)
	internal.RegisterCommandMetadata(
		SkillCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/ai/skill"),
	)
}
