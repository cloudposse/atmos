package list

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
)

var affectedParser *flags.StandardParser

// AffectedOptions contains parsed flags for the affected command.
type AffectedOptions struct {
	global.Flags
	// Output format flags.
	Format    string
	Columns   []string
	Delimiter string
	Sort      string

	// Git comparison flags.
	Ref            string
	SHA            string
	RepoPath       string
	SSHKeyPath     string
	SSHKeyPassword string
	CloneTargetRef bool

	// Content flags.
	IncludeDependents bool
	Stack             string
	ExcludeLocked     bool

	// Processing flags.
	ProcessTemplates bool
	ProcessFunctions bool
	Skip             []string
}

// affectedCmd lists affected Atmos components and stacks.
var affectedCmd = &cobra.Command{
	Use:   "affected",
	Short: "List affected components and stacks",
	Long:  "Display a table of affected components and stacks between Git commits.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()

		// Check Atmos configuration.
		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}
		if err := affectedParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &AffectedOptions{
			Flags:             flags.ParseGlobalFlags(cmd, v),
			Format:            v.GetString("format"),
			Columns:           v.GetStringSlice("columns"),
			Delimiter:         v.GetString("delimiter"),
			Sort:              v.GetString("sort"),
			Ref:               v.GetString("ref"),
			SHA:               v.GetString("sha"),
			RepoPath:          v.GetString("repo-path"),
			SSHKeyPath:        v.GetString("ssh-key"),
			SSHKeyPassword:    v.GetString("ssh-key-password"),
			CloneTargetRef:    v.GetBool("clone-target-ref"),
			IncludeDependents: v.GetBool("include-dependents"),
			Stack:             v.GetString("stack"),
			ExcludeLocked:     v.GetBool("exclude-locked"),
			ProcessTemplates:  v.GetBool("process-templates"),
			ProcessFunctions:  v.GetBool("process-functions"),
			Skip:              v.GetStringSlice("skip"),
		}

		return executeListAffectedCmd(cmd, args, opts)
	},
}

func init() {
	// Mark this subcommand as experimental.
	affectedCmd.Annotations = map[string]string{"experimental": "true"}

	// Create parser using flag wrappers.
	affectedParser = NewListParser(
		WithFormatFlag,
		WithAffectedColumnsFlag,
		WithDelimiterFlag,
		WithSortFlag,
		WithRefFlag,
		WithSHAFlag,
		WithRepoPathFlag,
		WithSSHKeyFlag,
		WithSSHKeyPasswordFlag,
		WithCloneTargetRefFlag,
		WithIncludeDependentsFlag,
		WithStackFlag,
		WithExcludeLockedFlag,
		WithProcessTemplatesFlag,
		WithProcessFunctionsFlag,
		WithSkipFlag,
	)

	// Register flags.
	affectedParser.RegisterFlags(affectedCmd)

	// Bind flags to Viper for environment variable support.
	if err := affectedParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListAffectedCmd(cmd *cobra.Command, args []string, opts *AffectedOptions) error {
	defer perf.Track(nil, "list.executeListAffectedCmd")()

	// Process and validate command line arguments.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return err
	}
	configAndStacksInfo.Command = "list"
	configAndStacksInfo.SubCommand = "affected"

	return list.ExecuteListAffectedCmd(&list.AffectedCommandOptions{
		Info:              &configAndStacksInfo,
		Cmd:               cmd,
		Args:              args,
		ColumnsFlag:       opts.Columns,
		FilterSpec:        "", // Filter spec not yet exposed via flag.
		SortSpec:          opts.Sort,
		Delimiter:         opts.Delimiter,
		Ref:               opts.Ref,
		SHA:               opts.SHA,
		RepoPath:          opts.RepoPath,
		SSHKeyPath:        opts.SSHKeyPath,
		SSHKeyPassword:    opts.SSHKeyPassword,
		CloneTargetRef:    opts.CloneTargetRef,
		IncludeDependents: opts.IncludeDependents,
		Stack:             opts.Stack,
		ProcessTemplates:  opts.ProcessTemplates,
		ProcessFunctions:  opts.ProcessFunctions,
		Skip:              opts.Skip,
		ExcludeLocked:     opts.ExcludeLocked,
	})
}
