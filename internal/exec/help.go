package exec

import (
	"fmt"
	"github.com/fatih/color"
)

// processHelp processes help commands
func processHelp(componentType string, command string) error {
	if len(command) == 0 {
		fmt.Println(fmt.Sprintf("'atmos' supports all native '%s' commands.", componentType))
		fmt.Println(fmt.Sprintf("In addition, 'component' and 'stack' are required in order to generate variables for the component in the stack."))
		color.Cyan(fmt.Sprintf("atmos %s <command> <component> -s <stack> [options]", componentType))
		color.Cyan(fmt.Sprintf("atmos %s <command> <component> --stack <stack> [options]", componentType))

		if componentType == "terraform" {
			fmt.Println()
			color.Cyan("Differences from native terraform:")
			fmt.Println(" - before executing other 'terraform' commands, 'atmos' calls 'terraform init'")
			fmt.Println(" - 'atmos terraform deploy' command executes 'terraform plan' and then 'terraform apply'")
			fmt.Println(" - 'atmos terraform deploy' command supports '--deploy-run-init=true/false' flag to enable/disable running 'terraform init' " +
				"before executing the command")
			fmt.Println(" - 'atmos terraform deploy' command sets '-auto-approve' flag before running 'terraform apply'")
			fmt.Println(" - 'atmos terraform apply' and 'atmos terraform deploy' commands support '--from-plan' flag. If the flag is specified, the commands " +
				"will use the previously generated 'planfile' instead of generating a new 'varfile'")
			fmt.Println(" - 'atmos terraform clean' command deletes the '.terraform' folder, '.terraform.lock.hcl' lock file, " +
				"and the previously generated 'planfile' and 'varfile' for the specified component and stack")
			fmt.Println(" - 'atmos terraform workspace' command first calls 'terraform init -reconfigure', then 'terraform workspace select', " +
				"and if the workspace was not created before, it then calls 'terraform workspace new'")
			fmt.Println(" - 'atmos terraform import' command searches for 'region' in the variables for the specified component and stack, and if it finds it, " +
				"sets 'AWS_REGION=<region>' ENV var before executing the command")
			fmt.Println(" - 'atmos terraform generate backend' command generates the backend file for the component in the stack")
			fmt.Println(" - 'atmos terraform shell' command configures an environment for the component in the stack and starts a new shell allowing executing all native terraform commands")
		}

		if componentType == "helmfile" {
			fmt.Println()
			color.Cyan("Differences from native helmfile:")
			fmt.Println(" - 'atmos helmfile' commands support '[global options]' in the command-line argument '--global-options'. " +
				"Usage: atmos helmfile <command> <component> -s <stack> [command options] [arguments...] --global-options=\"--no-color --namespace=test\"")
			fmt.Println(" - before executing the 'helmfile' commands, 'atmos' calls 'aws eks update-kubeconfig' to read kubeconfig from the EKS cluster " +
				"and use it to authenticate with the cluster")
		}

		err := execCommand(componentType, []string{"--help"}, "", nil)
		if err != nil {
			return err
		}
	} else {
		fmt.Println(fmt.Sprintf("'atmos' supports native '%s %s' command with all the options, arguments and flags.", componentType, command))
		fmt.Println(fmt.Sprintf("In addition, 'component' and 'stack' are required in order to generate variables for the component in the stack."))
		color.Cyan(fmt.Sprintf("atmos %s %s <component> -s <stack> [options]", componentType, command))
		color.Cyan(fmt.Sprintf("atmos %s %s <component> --stack <stack> [options]", componentType, command))

		err := execCommand(componentType, []string{command, "--help"}, "", nil)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}
