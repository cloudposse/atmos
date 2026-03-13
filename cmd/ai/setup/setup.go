package setup

import (
	"strings"

	"github.com/spf13/cobra"

	aiflags "github.com/cloudposse/atmos/cmd/ai/flags"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/analyze"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// InitAI parses AI flags from os.Args, validates configuration, and starts output capture.
// Returns a ready-to-use Context or an error if validation fails.
// The caller MUST call ctx.Cleanup() (via defer) to restore stdout/stderr.
func InitAI(atmosConfig *schema.AtmosConfiguration) (*analyze.Context, error) {
	defer perf.Track(nil, "setup.InitAI")()

	aiEnabled := aiflags.HasAIFlag()
	skillNames := aiflags.ParseSkillFlag()

	// Validate --skill requires --ai.
	if len(skillNames) > 0 && !aiEnabled {
		skillList := strings.Join(skillNames, ",")
		return nil, errUtils.Build(errUtils.ErrAISkillRequiresAIFlag).
			WithExplanation("The --skill flag provides domain-specific context for AI analysis, but AI analysis is not enabled. Use --skill together with --ai.").
			WithHintf("Add --ai to enable AI analysis:\n  atmos <command> --ai --skill %s", skillList).
			WithHintf("Or use environment variables:\n  ATMOS_AI=true ATMOS_SKILL=%s atmos <command>", skillList).
			Err()
	}

	if !aiEnabled {
		return analyze.NewDisabledContext(), nil
	}

	return analyze.Setup(atmosConfig, skillNames, analyze.BuildCommandName())
}

// IsAISubcommand checks whether cmd is the "atmos ai" subcommand (or one of its children).
// Used after Cobra execution to skip AI analysis for AI commands (avoid double processing).
func IsAISubcommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "ai" && c.Parent() != nil && c.Parent().Parent() == nil {
			return true
		}
	}
	return false
}
