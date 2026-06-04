package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
)

// proCommitCmd executes 'pro commit' CLI command.
var proCommitCmd = &cobra.Command{
	Use:                "commit",
	Short:              "Commit changes via Atmos Pro GitHub App",
	Long:               `Detects changed files and commits them server-side via Atmos Pro's GitHub App, ensuring commits trigger CI workflows.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		checkAtmosConfig(WithStackValidation(false))

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		flags := cmd.Flags()

		message, err := flags.GetString("message")
		if err != nil {
			return err
		}

		comment, err := flags.GetString("comment")
		if err != nil {
			return err
		}

		addPattern, err := flags.GetString("add")
		if err != nil {
			return err
		}

		stageAll, err := flags.GetBool("all")
		if err != nil {
			return err
		}

		// Validate mutually exclusive flags.
		if addPattern != "" && stageAll {
			return errUtils.Build(errUtils.ErrStagingFlagConflict).
				WithHint("Use --add to stage specific files, or --all to stage everything, but not both.").
				Err()
		}

		return pro.ExecuteCommit(&atmosConfig, message, comment, addPattern, stageAll)
	},
}

func init() {
	proCommitCmd.Flags().StringP("message", "m", "", "Commit message (required, max 500 chars)")
	proCommitCmd.Flags().String("comment", "", "Optional PR comment to post alongside the commit (max 2000 chars)")
	proCommitCmd.Flags().String("add", "", "File pattern to stage before committing (e.g. \"*.tf\")")
	proCommitCmd.Flags().BoolP("all", "A", false, "Stage all changes before committing (git add -A)")

	err := proCommitCmd.MarkFlagRequired("message")
	if err != nil {
		panic(err)
	}

	proCmd.AddCommand(proCommitCmd)
}
