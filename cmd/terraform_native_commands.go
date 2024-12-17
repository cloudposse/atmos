package cmd

import (
	"github.com/spf13/cobra"
)

// getTerraformCommands returns an array of statically defined Terraform commands with flags
func getTerraformCommands() []*cobra.Command {
	// List of Terraform commands
	return []*cobra.Command{
		{
			Use:   "plan",
			Short: "Show changes required by the current configuration",
			Long:  "Generate an execution plan, which shows what actions Terraform will take to reach the desired state of the configuration.",
		},
		{
			Use:   "apply",
			Short: "Apply changes to infrastructure",
			Long:  "Apply the changes required to reach the desired state of the configuration. This will prompt for confirmation before making changes.",
		},
		{
			Use:   "workspace",
			Short: "Manage Terraform workspaces",
			Long:  "Create, list, select, or delete Terraform workspaces, which allow for separate states within the same configuration.",
		},
		{
			Use:   "clean",
			Short: "Clean up resources",
			Long:  "Remove unused or outdated resources to keep the infrastructure clean and reduce costs.",
		},
		{
			Use:   "version",
			Short: "Show the current Terraform version",
			Long:  "Displays the current version of Terraform installed on the system.",
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
		},
		{
			Use:   "import",
			Short: "Associate existing infrastructure with a Terraform resource",
			Long:  "Import existing infrastructure into Terraform management by specifying the resource type and ID.",
		},
		{
			Use:   "refresh",
			Short: "Update the state to match remote systems",
			Long:  "Refresh the Terraform state, reconciling the local state with the actual infrastructure state.",
		},
		{
			Use:   "init",
			Short: "Prepare your working directory for other commands",
			Long:  "Initialize the working directory containing Terraform configuration files. It will download necessary provider plugins and set up the backend.",
		},
		{
			Use:   "validate",
			Short: "Check whether the configuration is valid",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "console",
			Short: "Try Terraform expressions at an interactive command prompt",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "fmt",
			Short: "Reformat your configuration in the standard style",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "force-unlock",
			Short: "Release a stuck lock on the current workspace",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "get",
			Short: "Install or upgrade remote Terraform modules",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "graph",
			Short: "Generate a Graphviz graph of the steps in an operation",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "import",
			Short: "Associate existing infrastructure with a Terraform resource",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "login",
			Short: "Obtain and save credentials for a remote host",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "logout",
			Short: "Remove locally-stored credentials for a remote host",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "metadata",
			Short: "Metadata related commands",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "modules",
			Short: "Show all declared modules in a working directory",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "output",
			Short: "Show output values from your root module",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "providers",
			Short: "Show the providers required for this configuration",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "show",
			Short: "Show the current state or a saved plan",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "state",
			Short: "Advanced state management",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "taint",
			Short: "Mark a resource instance as not fully functional",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "test",
			Short: "Execute integration tests for Terraform modules",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
		{
			Use:   "untaint",
			Short: "Remove the 'tainted' state from a resource instance",
			Annotations: map[string]string{
				"IsNotSupported": "true",
			},
		},
	}
}

// attachTerraformCommands attaches static Terraform commands to a provided parent command
func attachTerraformCommands(parentCmd *cobra.Command) {
	commands := getTerraformCommands()
	for _, cmd := range commands {
		parentCmd.AddCommand(cmd)
	}
}
