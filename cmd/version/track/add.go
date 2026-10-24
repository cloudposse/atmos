package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// crudResult reports a configuration edit for scriptable output.
type crudResult struct {
	Name  string `yaml:"name" json:"name"`
	Track string `yaml:"track" json:"track"`
	File  string `yaml:"file" json:"file"`
}

var trackAddCmd = &cobra.Command{
	Use:   "add NAME",
	Short: "Add a dependency entry to atmos.yaml",
	Long:  "Add a dependency entry to a version track in atmos.yaml, preserving comments and formatting. The ecosystem is inferred from the package when not set explicitly (actions/* is github/actions, registry-hosted images are oci, bare tool names are toolchain, owner/repo is github).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.add.RunE")()

		name := args[0]
		pkg, _ := cmd.Flags().GetString("package")
		if pkg == "" {
			pkg = name
		}
		ecosystem, _ := cmd.Flags().GetString("ecosystem")
		if ecosystem == "" {
			ecosystem = manager.InferEcosystem(pkg)
		}
		datasource, _ := cmd.Flags().GetString("datasource")
		provider, _ := cmd.Flags().GetString("provider")
		desired, _ := cmd.Flags().GetString("desired")
		group, _ := cmd.Flags().GetString("group")
		pin, _ := cmd.Flags().GetString("pin")
		include, _ := cmd.Flags().GetStringSlice("include")
		exclude, _ := cmd.Flags().GetStringSlice("exclude")
		prerelease, _ := cmd.Flags().GetBool("prerelease")

		entry := &schema.VersionEntry{
			Ecosystem:  ecosystem,
			Datasource: datasource,
			Provider:   provider,
			Package:    pkg,
			Desired:    desired,
			Group:      group,
			Update:     schema.VersionUpdatePolicy{Pin: pin},
			Include:    include,
			Exclude:    exclude,
		}
		if cmd.Flags().Changed("prerelease") {
			entry.Prerelease = &prerelease
		}
		track := manager.EffectiveTrack(atmosConfig, trackFromArgs(cmd, nil))
		file, err := manager.AddEntry(atmosConfig, track, name, entry)
		if err != nil {
			return err
		}
		return writeFormatted(cmd, crudResult{Name: name, Track: track, File: file})
	},
}

func init() {
	parser := flags.NewStandardParser(trackParserOptions(
		flags.WithStringFlag("package", "", "", "Package coordinate (e.g. actions/checkout, library/nginx, opentofu); defaults to NAME"),
		flags.WithStringFlag("ecosystem", "", "", "Ecosystem (github/actions, github, oci, docker, toolchain, ...); inferred from the package when omitted"),
		flags.WithStringFlag("datasource", "", "", "Datasource override (github-tags, github-releases, oci-tags, ...)"),
		flags.WithStringFlag("provider", "", "", "Provider name from version.providers"),
		flags.WithStringFlag("desired", "", "latest", "Desired version: concrete, SemVer constraint, or latest"),
		flags.WithStringFlag("group", "", "", "Version group name"),
		flags.WithStringFlag("pin", "", "", "Pin policy: digest (alias: sha) locks and renders the immutable identifier"),
		flags.WithStringSliceFlag("include", "", nil, "Candidate version patterns to include (repeatable)"),
		flags.WithStringSliceFlag("exclude", "", nil, "Candidate version patterns to exclude (repeatable)"),
		flags.WithBoolFlag("prerelease", "", false, "Allow prerelease candidates"),
	)...)
	parser.RegisterFlags(trackAddCmd)
	trackCmd.AddCommand(trackAddCmd)
}
