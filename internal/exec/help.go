package exec

import (
	"fmt"

	u "github.com/cloudposse/atmos/pkg/utils"
)

// processHelp processes help commands
func processHelp(componentType string, command string) error {
	if len(command) == 0 {
		fmt.Printf("'atmos' supports all native '%s' commands.\n", componentType)
		fmt.Printf("In addition, the 'component' argument and 'stack' flag are required to generate the variables and backend config for the component in the stack.\n")
		u.PrintInfo(fmt.Sprintf("atmos %s <command> <component> -s <stack> [options]", componentType))
		u.PrintInfo(fmt.Sprintf("atmos %s <command> <component> --stack <stack> [options]", componentType))

		if componentType == "terraform" {
			fmt.Println()
			u.PrintInfo("Additions and differences from native terraform:")
			fmt.Println(" - before executing other 'terraform' commands, 'atmos' runs 'terraform init'")
			fmt.Println(" - you can skip over atmos calling 'terraform init' if you know your project is already in a good working state by using " +
				"the '--skip-init' flag like so 'atmos terraform <command> <component> -s <stack> --skip-init")
			fmt.Println(" - 'atmos terraform deploy' command executes 'terraform plan' and then 'terraform apply'")
			fmt.Println(" - 'atmos terraform deploy' command supports '--deploy-run-init=true/false' flag to enable/disable running 'terraform init' " +
				"before executing the command")
			fmt.Println(" - 'atmos terraform deploy' command sets '-auto-approve' flag before running 'terraform apply'")
			fmt.Println(" - 'atmos terraform apply' and 'atmos terraform deploy' commands support '--from-plan' flag. If the flag is specified, " +
				"the commands will use the previously generated 'planfile' instead of generating a new 'varfile'")
			fmt.Println(" - 'atmos terraform clean' command deletes the '.terraform' folder, '.terraform.lock.hcl' lock file, " +
				"and the previously generated 'planfile' and 'varfile' for the specified component and stack")
			fmt.Println(" - 'atmos terraform workspace' command first runs 'terraform init -reconfigure', then 'terraform workspace select', " +
				"and if the workspace was not created before, it then runs 'terraform workspace new'")
			fmt.Println(" - 'atmos terraform import' command searches for 'region' in the variables for the specified component and stack, " +
				"and if it finds it, sets 'AWS_REGION=<region>' ENV var before executing the command")
			fmt.Println(" - 'atmos terraform generate backend' command generates a backend config file for an 'atmos' component in a stack")
			fmt.Println(" - 'atmos terraform generate backends' command generates backend config files for all 'atmos' components in all stacks")
			fmt.Println(" - 'atmos terraform generate varfile' command generates a varfile for an 'atmos' component in a stack")
			fmt.Println(" - 'atmos terraform generate varfiles' command generates varfiles for all 'atmos' components in all stacks")
			fmt.Println(" - 'atmos terraform shell' command configures an environment for an 'atmos' component in a stack and starts a new shell " +
				"allowing executing all native terraform commands inside the shell")
		}

		if componentType == "helmfile" {
			fmt.Println()
			u.PrintInfo("Additions and differences from native helmfile:")
			fmt.Println(" - 'atmos helmfile generate varfile' command generates a varfile for the component in the stack")
			fmt.Println(" - 'atmos helmfile' commands support '[global options]' using the command-line flag '--global-options'. " +
				"Usage: atmos helmfile <command> <component> -s <stack> [command options] [arguments] --global-options=\"--no-color --namespace=test\"")
			fmt.Println(" - before executing the 'helmfile' commands, 'atmos' runs 'aws eks update-kubeconfig' to read kubeconfig from " +
				"the EKS cluster and use it to authenticate with the cluster. This can be disabled in 'atmos.yaml' CLI config " +
				"by setting 'components.helmfile.use_eks' to 'false'")
		}

		err := ExecuteShellCommand(componentType, []string{"--help"}, "", nil, false, true)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("'atmos' supports native '%s %s' command with all the options, arguments and flags.\n", componentType, command)
		fmt.Printf("In addition, 'component' and 'stack' are required in order to generate variables for the component in the stack.\n")
		u.PrintInfo(fmt.Sprintf("atmos %s %s <component> -s <stack> [options]", componentType, command))
		u.PrintInfo(fmt.Sprintf("atmos %s %s <component> --stack <stack> [options]", componentType, command))

		err := ExecuteShellCommand(componentType, []string{command, "--help"}, "", nil, false, true)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}
