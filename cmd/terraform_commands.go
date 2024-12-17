package cmd

import (
	"fmt"
	"os"

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
		},
		{
			Use:   "workspace",
			Short: "Manage Terraform workspaces",
			Long:  "Create, list, select, or delete Terraform workspaces, which allow for separate states within the same configuration. Note, Atmos will automatically select the workspace, if configured.",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "clean",
			Short: "Clean up resources",
			Long:  "Remove unused or outdated resources to keep the infrastructure clean and reduce costs.",
			Run:   terraformRun,
		},
		{
			Use:   "deploy",
			Short: "Deploy the specified infrastructure using Terraform",
			Long: `Deploy the specified infrastructure by running the Terraform plan and apply commands.
This command automates the deployment process, integrates configuration, and ensures streamlined execution.`,
			Run: terraformRun,
		},
		{
			Use:   "shell",
			Short: "Configures an 'atmos' environment and starts a shell for native Terraform commands.",
			Long:  "command configures an environment for an 'atmos' component in a stack and starts a new shell allowing executing all native terraform commands inside the shell without using atmos-specific arguments and flag",
			Run:   terraformRun,
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
			Short: "Associate existing infrastructure with a Terraform resource",
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
	commands := getTerraformCommands()
	fmt.Println(os.Args)
	for _, cmd := range commands {
		cmd.Run = func(cmd_ *cobra.Command, args []string) {
			if len(os.Args) > 3 {
				args = os.Args[2:]
			}
			parentCmd.Run(parentCmd, args)
		}
		parentCmd.AddCommand(cmd)
	}
}
