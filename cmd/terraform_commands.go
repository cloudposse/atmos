package cmd

import (
	"os"

	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/spf13/cobra"
)

// getTerraformCommands returns an array of statically defined Terraform commands with flags
func getTerraformCommands() []*cobra.Command {
	// List of Terraform commands
	return []*cobra.Command{
		{
			Use:     "plan",
			Short:   "Show changes required by the current configuration",
			Long:    "Generate an execution plan, which shows what actions Terraform will take to reach the desired state of the configuration.",
			Example: "atmos terraform plan <component> -s <stack>",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:     "apply",
			Short:   "Apply changes to infrastructure",
			Long:    "Apply the changes required to reach the desired state of the configuration. This will prompt for confirmation before making changes.",
			Example: "atmos terraform apply <component> -s <stack>",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
			PostRunE: func(cmd *cobra.Command, args []string) error {
				return runHooks(h.AfterTerraformApply, cmd, args)
			},
		},
		{
			Use:   "workspace",
			Short: "Manage Terraform workspaces",
			Long: `The 'atmos terraform workspace' command initializes Terraform for the current configuration, selects the specified workspace, and creates it if it does not already exist.

It runs the following sequence of Terraform commands:
1. 'terraform init -reconfigure' to initialize the working directory.
2. 'terraform workspace select' to switch to the specified workspace.
3. If the workspace does not exist, it runs 'terraform workspace new' to create and select it.

This ensures that the workspace is properly set up for Terraform operations.`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "clean",
			Short: "Clean up Terraform state and artifacts.",
			Long: `The 'atmos terraform clean' command removes temporary files, state locks, and other artifacts created during Terraform operations. This helps reset the environment and ensures no leftover data interferes with subsequent runs.

Common use cases:
- Releasing locks on Terraform state files.
- Cleaning up temporary workspaces or plans.
- Preparing the environment for a fresh deployment.`,
		},
		{
			Use:   "deploy",
			Short: "Deploy the specified infrastructure using Terraform",
			Long:  `Deploys infrastructure by running the Terraform apply command with automatic approval. This ensures that the changes defined in your Terraform configuration are applied without requiring manual confirmation, streamlining the deployment process.`,
			PostRunE: func(cmd *cobra.Command, args []string) error {
				return runHooks(h.AfterTerraformApply, cmd, args)
			},
		},
		{
			Use:   "shell",
			Short: "Configure an environment for an Atmos component and start a new shell.",
			Long:  `The 'atmos terraform shell' command configures an environment for a specific Atmos component in a stack and then starts a new shell. In this shell, you can execute all native Terraform commands directly without the need to use Atmos-specific arguments and flags. This allows you to interact with Terraform as you would in a typical setup, but within the configured Atmos environment.`,
		},
		{
			Use:   "version",
			Short: "Show the current Terraform version",
			Long:  "Displays the current version of Terraform installed on the system.",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "varfile",
			Short: "Load variables from a file",
			Long:  "Load variable definitions from a specified file and use them in the configuration.",
		},
		{
			Use:   "write varfile",
			Short: "Write variables to a file",
			Long:  "Write the variables used in the configuration to a specified file for later use or modification.",
		},
		{
			Use:   "destroy",
			Short: "Destroy previously-created infrastructure",
			Long:  "Destroy all the infrastructure managed by Terraform, removing resources as defined in the state file.",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "refresh",
			Short: "Update the state to match remote systems",
			Long:  "Refresh the Terraform state, reconciling the local state with the actual infrastructure state.",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "init",
			Short: "Prepare your working directory for other commands",
			Long:  "Initialize the working directory containing Terraform configuration files. It will download necessary provider plugins and set up the backend. Note, that Atmos will automatically call init for you, when running `plan` and `apply` commands.",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "validate",
			Short: "Check whether the configuration is valid",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "console",
			Short: "Try Terraform expressions at an interactive command prompt",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "fmt",
			Short: "Reformat your configuration in the standard style",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "force-unlock",
			Short: "Release a stuck lock on the current workspace",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "get",
			Short: "Install or upgrade remote Terraform modules",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "graph",
			Short: "Generate a Graphviz graph of the steps in an operation",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "import",
			Short: "Import existing infrastructure into Terraform state.",
			Long: `The 'atmos terraform import' command imports existing infrastructure resources into Terraform's state.

Before executing the command, it searches for the 'region' variable in the specified component and stack configuration.
If the 'region' variable is found, it sets the 'AWS_REGION' environment variable with the corresponding value before executing the import command.

The import command runs: 'terraform import [ADDRESS] [ID]'

Arguments:
- ADDRESS: The Terraform address of the resource to import.
- ID: The ID of the resource to import.`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "login",
			Short: "Obtain and save credentials for a remote host",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "logout",
			Short: "Remove locally-stored credentials for a remote host",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "metadata",
			Short: "Metadata related commands",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "modules",
			Short: "Show all declared modules in a working directory",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "output",
			Short: "Show output values from your root module",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "providers",
			Short: "Show the providers required for this configuration",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "show",
			Short: "Show the current state or a saved plan",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "state",
			Short: "Advanced state management",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "taint",
			Short: "Mark a resource instance as not fully functional",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "test",
			Short: "Execute integration tests for Terraform modules",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "untaint",
			Short: "Remove the 'tainted' state from a resource instance",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
	}
}

// attachTerraformCommands attaches static Terraform commands to a provided parent command
func attachTerraformCommands(parentCmd *cobra.Command) {
	parentCmd.PersistentFlags().String("append-user-agent", "", "Sets the TF_APPEND_USER_AGENT environment variable to customize the User-Agent string in Terraform provider requests. Example: 'Atmos/%s (Cloud Posse; +https://atmos.tools)'. This flag works with almost all commands.")
	parentCmd.PersistentFlags().Bool("skip-init", false, "Skip running 'terraform init' before executing the command")

	commands := getTerraformCommands()

	for _, cmd := range commands {
		cmd.FParseErrWhitelist.UnknownFlags = true
		cmd.DisableFlagParsing = true
		if setFlags, ok := commandMaps[cmd.Use]; ok {
			setFlags(cmd)
		}
		cmd.ValidArgsFunction = ComponentsArgCompletion
		cmd.Run = func(cmd_ *cobra.Command, args []string) {
			// Because we disable flag parsing we require manual handle help Request
			handleHelpRequest(cmd, args)
			if len(os.Args) > 2 {
				args = os.Args[2:]
			}

			terraformRun(parentCmd, cmd_, args)
		}
		parentCmd.AddCommand(cmd)
	}
}

var commandMaps = map[string]func(cmd *cobra.Command){
	"deploy": func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool("deploy-run-init", false, "If set atmos will run `terraform init` before executing the command")
		cmd.PersistentFlags().Bool("from-plan", false, "If set atmos will use the previously generated plan file")
		cmd.PersistentFlags().String("planfile", "", "Set the plan file to use")
	},
	"apply": func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool("from-plan", false, "If set atmos will use the previously generated plan file")
		cmd.PersistentFlags().String("planfile", "", "Set the plan file to use")
	},
	"clean": func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool("everything", false, "If set atmos will also delete the Terraform state files and directories for the component.")
		cmd.PersistentFlags().Bool("force", false, "Forcefully delete Terraform state files and directories without interaction")
		cmd.PersistentFlags().Bool("skip-lock-file", false, "Skip deleting the `.terraform.lock.hcl` file")
	},
}
