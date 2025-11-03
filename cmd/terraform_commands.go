package cmd

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/spf13/cobra"
)

// getTerraformCommands returns an array of statically defined Terraform commands with flags.
func getTerraformCommands() []*cobra.Command {
	// List of Terraform commands
	return []*cobra.Command{
		{
			Use:   "plan",
			Short: "Show changes required by the current configuration",
			Long:  "Generate an execution plan, which shows what actions Terraform will take to reach the desired state of the configuration.",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "plan-diff",
			Short: "Compare two Terraform plans and show the differences",
			Long: `The 'atmos terraform plan-diff' command compares two Terraform plans and shows the differences between them.

It takes an original plan file (--orig) and optionally a new plan file (--new). If the new plan file is not provided,
it will generate one by running 'terraform plan' with the current configuration.

The command shows differences in variables, resources, and outputs between the two plans.

Example usage:
  atmos terraform plan-diff myapp -s dev --orig=orig.plan
  atmos terraform plan-diff myapp -s dev --orig=orig.plan --new=new.plan`,
		},
		{
			Use:   "apply",
			Short: "Apply changes to infrastructure",
			Long:  "Apply the changes required to reach the desired state of the configuration. This will prompt for confirmation before making changes.",
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

// attachTerraformCommands attaches static Terraform commands to a provided parent command.
func attachTerraformCommands(parentCmd *cobra.Command) {
	parentCmd.PersistentFlags().String("append-user-agent", "", fmt.Sprintf("Sets the TF_APPEND_USER_AGENT environment variable to customize the User-Agent string in Terraform provider requests. Example: `Atmos/%s (Cloud Posse; +https://atmos.tools)`. This flag works with almost all commands.", version.Version))
	// NOTE: skip-init is already registered by TerraformFlags() via RegisterPersistentFlags()
	// parentCmd.PersistentFlags().Bool("skip-init", false, "Skip running `terraform init` before executing terraform commands")
	parentCmd.PersistentFlags().Bool("init-pass-vars", false, "Pass the generated varfile to `terraform init` using the `--var-file` flag. OpenTofu supports passing a varfile to `init` to dynamically configure backends")
	parentCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing terraform commands")
	parentCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing terraform commands")
	parentCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing specific YAML functions in the Atmos stack manifests when executing terraform commands")

	// NOTE: Identity flag is registered via terraformParser.RegisterPersistentFlags() in terraform.go init().
	// Register shell completion for identity flag.
	AddIdentityCompletion(parentCmd)

	parentCmd.PersistentFlags().StringP("query", "q", "", "Execute `atmos terraform <command>` on the components filtered by a YQ expression, in all stacks or in a specific stack")
	parentCmd.PersistentFlags().StringSlice("components", nil, "Filter by specific components")
	// NOTE: dry-run is already registered by CommonFlags() via RegisterPersistentFlags()
	// parentCmd.PersistentFlags().Bool("dry-run", false, "Simulate the command without making any changes")

	// Flags related to `--affected` (similar to `atmos describe affected`)
	// These flags are only used then executing `atmos terraform <command> --affected`
	parentCmd.PersistentFlags().String("repo-path", "", "Filesystem path to the already cloned target repository with which to compare the current branch: atmos terraform <sub-command> --affected --repo-path <path_to_already_cloned_repo>")
	parentCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch: atmos terraform <sub-command> --affected --ref refs/heads/main. Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details")
	parentCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch: atmos terraform <sub-command> --affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073")
	parentCmd.PersistentFlags().String("ssh-key", "", "Path to PEM-encoded private key to clone private repos using SSH: atmos terraform <sub-command> --affected --ssh-key <path_to_ssh_key>")
	parentCmd.PersistentFlags().String("ssh-key-password", "", "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block: atmos terraform <sub-command> --affected --ssh-key <path_to_ssh_key> --ssh-key-password <password>")
	parentCmd.PersistentFlags().Bool("include-dependents", false, "For each affected component, detect the dependent components and process them in the dependency order, recursively: atmos terraform <sub-command> --affected --include-dependents")
	parentCmd.PersistentFlags().Bool("clone-target-ref", false, "Clone the target reference with which to compare the current branch: atmos terraform <sub-command> --affected --clone-target-ref=true\n"+
		"If set to 'false' (default), the target reference will be checked out instead\n"+
		"This requires that the target reference is already cloned by Git, and the information about it exists in the '.git' directory")

	commands := getTerraformCommands()

	for _, cmd := range commands {
		// Tell Cobra to ignore unknown flags/args so positional args (component names) pass through
		cmd.FParseErrWhitelist.UnknownFlags = true

		// Accept arbitrary positional arguments (component names, etc.)
		// This prevents Cobra from treating component names as unknown subcommands
		cmd.Args = cobra.ArbitraryArgs

		// Register Atmos flags on each subcommand.
		// This ensures flags like --stack, --identity, --dry-run work on all terraform subcommands.
		terraformParser.RegisterFlags(cmd)

		if setFlags, ok := commandMaps[cmd.Use]; ok {
			setFlags(cmd)
		}
		cmd.ValidArgsFunction = ComponentsArgCompletion
		cmd.RunE = func(cmd_ *cobra.Command, args []string) error {
			handleHelpRequest(cmd, args)
			// Heatmap is now tracked via persistent flag, no need for manual check.

			// Parse args with flags.
			// Returns strongly-typed TerraformOptions instead of weak map-based ParsedConfig.
			ctx := cmd_.Context()
			opts, err := terraformParser.Parse(ctx, args)
			if err != nil {
				return err
			}

			// Pass interpreter to terraformRun for type-safe flag access.
			return terraformRun(parentCmd, cmd_, opts)
		}
		parentCmd.AddCommand(cmd)
	}
}

var commandMaps = map[string]func(cmd *cobra.Command){
	"plan": func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool(cfg.UploadStatusFlag, false, "If set atmos will upload the plan result to the pro API")
		cmd.PersistentFlags().Bool("affected", false, "Plan the affected components in dependency order")
		cmd.PersistentFlags().Bool("all", false, "Plan all components in all stacks")
		cmd.PersistentFlags().Bool("skip-planfile", false, "Skip writing the plan to a file by not passing the `-out` flag to Terraform when executing the command. Set it to `true` when using Terraform Cloud since the `-out` flag is not supported. Terraform Cloud automatically stores plans in its backend")
	},
	"deploy": func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool("deploy-run-init", false, "If set atmos will run `terraform init` before executing the command")
		cmd.PersistentFlags().Bool("from-plan", false, "If set atmos will use the previously generated plan file")
		cmd.PersistentFlags().String("planfile", "", "Set the plan file to use")
		cmd.PersistentFlags().Bool("affected", false, "Deploy the affected components in dependency order")
		cmd.PersistentFlags().Bool("all", false, "Deploy all components in all stacks")
	},
	"apply": func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool("from-plan", false, "If set atmos will use the previously generated plan file")
		cmd.PersistentFlags().String("planfile", "", "Set the plan file to use")
		cmd.PersistentFlags().Bool("affected", false, "Apply the affected components in dependency order")
		cmd.PersistentFlags().Bool("all", false, "Apply all components in all stacks")
	},
	"clean": func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool("everything", false, "If set atmos will also delete the Terraform state files and directories for the component.")
		cmd.PersistentFlags().Bool("force", false, "Forcefully delete Terraform state files and directories without interaction")
		cmd.PersistentFlags().Bool("skip-lock-file", false, "Skip deleting the `.terraform.lock.hcl` file")
	},
	"plan-diff": func(cmd *cobra.Command) {
		cmd.PersistentFlags().String("orig", "", "Path to the original Terraform plan file (required)")
		cmd.PersistentFlags().String("new", "", "Path to the new Terraform plan file (optional)")
		err := cmd.MarkPersistentFlagRequired("orig")
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "Error marking 'orig' flag as required", "")
		}
	},
}
