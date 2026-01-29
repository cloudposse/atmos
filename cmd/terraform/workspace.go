package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
)

// workspaceParser handles flag parsing for workspace command.
var workspaceParser *flags.StandardParser

// workspaceCmd represents the terraform workspace command.
var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage Terraform workspaces",
	Long: `Manage Terraform workspaces for organizing multiple states within a single configuration.

The 'atmos terraform workspace' command initializes Terraform for the current configuration,
selects the specified workspace, and creates it if it does not already exist.

It runs the following sequence of Terraform commands:
1. 'terraform init -reconfigure' to initialize the working directory.
2. 'terraform workspace select' to switch to the specified workspace.
3. If the workspace does not exist, it runs 'terraform workspace new' to create and select it.

This ensures that the workspace is properly set up for Terraform operations.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/workspace
  https://opentofu.org/docs/cli/commands/workspace`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := workspaceParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
}

// workspaceSubcmds defines the terraform workspace sub-subcommands.
// Each entry is registered as a Cobra child command of workspaceCmd,
// enabling proper command tree routing instead of hardcoded argument parsing.
var workspaceSubcmds = []struct {
	name  string
	short string
}{
	{"list", "List Terraform workspaces"},
	{"select", "Select a Terraform workspace"},
	{"new", "Create a new Terraform workspace"},
	{"delete", "Delete a Terraform workspace"},
	{"show", "Show the name of the current Terraform workspace"},
}

// newWorkspacePassthroughSubcommand creates a workspace sub-subcommand that binds
// both the shared terraform parser and the workspace-specific parser before delegating
// to the workspace execution flow.
func newWorkspacePassthroughSubcommand(name, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                name + " [component] -s [stack]",
		Short:              short,
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
		RunE: func(_ *cobra.Command, args []string) error {
			v := viper.GetViper()

			// Bind flags using workspaceCmd to read persistent flags.
			if err := terraformParser.BindFlagsToViper(workspaceCmd, v); err != nil {
				return err
			}
			if err := workspaceParser.BindFlagsToViper(workspaceCmd, v); err != nil {
				return err
			}

			opts := ParseTerraformRunOptions(v)
			argsForWorkspace := append([]string{name}, args...)
			return terraformRunWithOptions(terraformCmd, workspaceCmd, argsForWorkspace, opts)
		},
	}
	RegisterTerraformCompletions(cmd)
	return cmd
}

func init() {
	// Create parser with workspace-specific flags.
	workspaceParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
	)

	// Register workspace-specific flags as persistent so sub-subcommands inherit them.
	workspaceParser.RegisterPersistentFlags(workspaceCmd)

	// Bind flags to Viper for environment variable support.
	if err := workspaceParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register sub-subcommands for workspace (e.g., "workspace list", "workspace select").
	for _, sub := range workspaceSubcmds {
		workspaceCmd.AddCommand(newWorkspacePassthroughSubcommand(sub.name, sub.short))
	}

	// Register completions for workspaceCmd.
	RegisterTerraformCompletions(workspaceCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "workspace", WorkspaceCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(workspaceCmd)
}
