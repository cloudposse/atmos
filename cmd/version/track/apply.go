package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/managers"

	// Register the built-in file managers.
	_ "github.com/cloudposse/atmos/pkg/version/managers/githubactions"
	_ "github.com/cloudposse/atmos/pkg/version/managers/marker"
	_ "github.com/cloudposse/atmos/pkg/version/managers/template"
)

// appliedFile is one row of apply output.
type appliedFile struct {
	Path    string `yaml:"path" json:"path"`
	Manager string `yaml:"manager" json:"manager"`
}

var trackApplyCmd = &cobra.Command{
	Use:     "apply [track]",
	Aliases: []string{"sync"},
	Short:   "Rewrite version-managed files from the lock",
	Long:    "Run the file managers (github-actions workflow refs, marker-annotated files, rendered templates) over the paths configured in version.files (or the managers' default paths) and rewrite them from the locked versions. Use --check to fail without writing when files are out of date (CI).",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.apply.RunE")()

		check, _ := cmd.Flags().GetBool("check")
		dir, _ := cmd.Flags().GetString("dir")
		only, _ := cmd.Flags().GetStringSlice("manager")

		planned, err := managers.Plan(cmd.Context(), &managers.RunOptions{
			Config: atmosConfig,
			Track:  trackFromArgs(cmd, args),
			Dir:    dir,
			Only:   only,
			Render: renderTemplate,
		})
		if err != nil {
			return err
		}
		if check {
			return managers.Check(planned)
		}
		if err := managers.Apply(planned); err != nil {
			return err
		}
		applied := make([]appliedFile, 0, len(planned))
		for i := range planned {
			applied = append(applied, appliedFile{Path: planned[i].Path, Manager: planned[i].Manager})
		}
		return writeFormatted(cmd, applied)
	},
}

func init() {
	parser := flags.NewStandardParser(trackParserOptions(
		flags.WithBoolFlag("check", "", false, "Fail without writing when managed files are out of date (CI)"),
		flags.WithStringFlag("dir", "", "", "Root directory to resolve managed paths from (default: current directory)"),
		flags.WithStringSliceFlag("manager", "", nil, "Limit the run to the named file managers (repeatable)"),
	)...)
	parser.RegisterFlags(trackApplyCmd)
	trackCmd.AddCommand(trackApplyCmd)
}
