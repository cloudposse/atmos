package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// processHelp processes help commands
func processHelp(
	cliConfig schema.CliConfiguration,
	componentType string,
	command string,
) error {
	if len(command) == 0 {
		u.PrintMessage(fmt.Sprintf("Atmos supports all native '%s' commands.\n", componentType))
		u.PrintMessage("In addition, the 'component' argument and 'stack' flag are required to generate the variables and backend config for the component in the stack.\n")
		u.PrintMessage(fmt.Sprintf("atmos %s <command> <component> -s <stack> [options]", componentType))
		u.PrintMessage(fmt.Sprintf("atmos %s <command> <component> --stack <stack> [options]", componentType))

		if componentType == "terraform" {
			u.PrintMessage(`
Usage: atmos terraform [global options] <subcommand> [args]

The available commands for execution are listed below.
The primary workflow commands are given first, followed by
less common or more advanced commands.

Atmos commands:
  generate backend          	Command generates a backend config file for an 'atmos' component in a stack
  generate backends          	Command generates backend config files for all 'atmos' components in all stacks
  generate varfile          	Command generates a varfile for an 'atmos' component in a stack
  generate varfiles          	Command generates varfiles for all 'atmos' components in all stacks
  shell          		Command configures an environment for an 'atmos' component in a stack and starts a new shell allowing executing all native terraform commands inside the shell without using atmos-specific arguments and flags
  double-dash '--'          	Can be used to signify the end of the options for Atmos and the start of the additional native arguments and flags for the 'terraform' commands. For example: atmos terraform plan <component> -s <stack> -- -refresh=false -lock=false
  '--append-user-agent'         Flag sets the TF_APPEND_USER_AGENT environment variable to customize the User-Agent string in Terraform provider requests. Example: 'Atmos/0.0.1 (Cloud Posse; +https://atmos.tools)'. If not specified, defaults to 'atmos 0.0.1'

Main commands:
  init          		Prepare your working directory for other commands
  validate      		Check whether the configuration is valid
  plan          		Show changes required by the current configuration
  apply         		Create or update infrastructure
  destroy       		Destroy previously-created infrastructure

All other commands:
  console       		Try Terraform expressions at an interactive command prompt
  fmt           		Reformat your configuration in the standard style
  force-unlock  		Release a stuck lock on the current workspace
  get           		Install or upgrade remote Terraform modules
  graph         		Generate a Graphviz graph of the steps in an operation
  import        		Associate existing infrastructure with a Terraform resource
  login         		Obtain and save credentials for a remote host
  logout        		Remove locally-stored credentials for a remote host
  metadata      		Metadata related commands
  modules       		Show all declared modules in a working directory
  output        		Show output values from your root module
  providers     		Show the providers required for this configuration
  refresh       		Update the state to match remote systems
  show          		Show the current state or a saved plan
  state         		Advanced state management
  taint         		Mark a resource instance as not fully functional
  test          		Execute integration tests for Terraform modules
  untaint       		Remove the 'tainted' state from a resource instance
  version       		Show the current Terraform version
  workspace     		Workspace management

Global options (use these before the subcommand, if any):
  -chdir=DIR    		Switch to a different working directory before executing the
                		given subcommand.
  -help         		Show this help output, or the help for a specified subcommand.
  -version      		An alias for the "version" subcommand.
`)
		}

		if componentType == "helmfile" {
			u.PrintMessage("\nAdditions and differences from native helmfile:")
			u.PrintMessage(" - 'atmos helmfile generate varfile' command generates a varfile for the component in the stack")
			u.PrintMessage(" - 'atmos helmfile' commands support '[global options]' using the command-line flag '--global-options'. " +
				"Usage: atmos helmfile <command> <component> -s <stack> [command options] [arguments] --global-options=\"--no-color --namespace=test\"")
			u.PrintMessage(" - before executing the 'helmfile' commands, 'atmos' runs 'aws eks update-kubeconfig' to read kubeconfig from " +
				"the EKS cluster and use it to authenticate with the cluster. This can be disabled in 'atmos.yaml' CLI config " +
				"by setting 'components.helmfile.use_eks' to 'false'")
			u.PrintMessage(" - double-dash '--' can be used to signify the end of the options for Atmos and the start of the additional " +
				"native arguments and flags for the 'helmfile' commands")
		}
	} else if componentType == "terraform" && command == "clean" {
		u.PrintMessage("\n'atmos terraform clean' command deletes the following folders and files from the component's directory:\n\n" +
			" - '.terraform' folder\n" +
			" - folder that the 'TF_DATA_DIR' ENV var points to\n" +
			" - '.terraform.lock.hcl' file\n" +
			" - generated varfile for the component in the stack\n" +
			" - generated planfile for the component in the stack\n" +
			" - generated 'backend.tf.json' file\n" +
			" - 'terraform.tfstate.d' folder (if '--everything' flag is used)\n\n" +
			"Usage: atmos terraform clean <component> -s <stack> <flags>\n\n" +
			"Use '--everything' flag to also delete the Terraform state files and and directories with confirm message.\n\n" +
			"Use --force to forcefully delete Terraform state files and directories for the component.\n\n" +
			"- If no component is specified, the command will apply to all components and stacks.\n" +
			"- If no stack is specified, the command will apply to all stacks for the specified component.\n" +
			"Use '--skip-lock-file' flag to skip deleting the '.terraform.lock.hcl' file.\n\n" +
			"If no component or stack is specified, the clean operation will apply globally to all components.\n\n" +
			"For more details refer to https://atmos.tools/cli/commands/terraform/clean\n")

	} else if componentType == "terraform" && command == "deploy" {
		u.PrintMessage("\n'atmos terraform deploy' command executes 'terraform apply -auto-approve' on an Atmos component in an Atmos stack.\n\n" +
			"Usage: atmos terraform deploy <component> -s <stack> <flags>\n\n" +
			"The command automatically sets '-auto-approve' flag when running 'terraform apply'.\n\n" +
			"It supports '--deploy-run-init=true|false' flag to enable/disable running terraform init before executing the command.\n\n" +
			"It supports '--from-plan' flag. If the flag is specified, the command will use the planfile previously generated by 'atmos terraform plan' " +
			"command instead of generating a new planfile.\nNote that in this case, the planfile name is in the format supported by Atmos and is " +
			"saved to the component's folder.\n\n" +
			"It supports '--planfile' flag to specify the path to a planfile.\nThe '--planfile' flag should be used instead of the 'planfile' " +
			"argument in the native 'terraform apply <planfile>' command.\n\n" +
			"For more details refer to https://atmos.tools/cli/commands/terraform/deploy\n")
	} else if componentType == "terraform" && command == "shell" {
		u.PrintMessage("\n'atmos terraform shell' command starts a new SHELL configured with the environment for an Atmos component " +
			"in a Stack to allow executing all native terraform commands\ninside the shell without using the atmos-specific arguments and flags.\n\n" +
			"Usage: atmos terraform shell <component> -s <stack>\n\n" +
			"The command does the following:\n\n" +
			" - Processes the stack config files, generates the required variables for the Atmos component in the stack, and writes them to a file in the component's folder\n" +
			" - Generates a backend config file for the Atmos component in the stack and writes it to a file in the component's folder (or as specified by the Atmos configuration setting)\n" +
			" - Creates a Terraform workspace for the component in the stack\n" +
			" - Drops the user into a separate shell (process) with all the required paths and ENV vars set\n" +
			" - Inside the shell, the user can execute all Terraform commands using the native syntax\n\n" +
			"For more details refer to https://atmos.tools/cli/commands/terraform/shell\n")
	} else if componentType == "terraform" && command == "workspace" {
		u.PrintMessage("\n'atmos terraform workspace' command calculates the Terraform workspace for an Atmos component,\n" +
			"and then executes 'terraform init -reconfigure' and selects the Terraform workspace by executing the 'terraform workspace select' command.\n" +
			"If the workspace does not exist, the command creates it by executing the 'terraform workspace new' command.\n\n" +
			"Usage: atmos terraform workspace <component> -s <stack>\n\n" +
			"For more details refer to https://atmos.tools/cli/commands/terraform/workspace\n")
	} else {
		u.PrintMessage(fmt.Sprintf("\nAtmos supports native '%s' commands with all the options, arguments and flags.\n", componentType))
		u.PrintMessage("In addition, 'component' and 'stack' are required in order to generate variables for the component in the stack.\n")
		u.PrintMessage(fmt.Sprintf("atmos %s <subcommand> <component> -s <stack> [options]", componentType))
		u.PrintMessage(fmt.Sprintf("atmos %s <subcommand> <component> --stack <stack> [options]", componentType))
		u.PrintMessage(fmt.Sprintf("\nFor more details, execute '%s --help'\n", componentType))
	}

	return nil
}
