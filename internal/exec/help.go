package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// processHelp processes help commands
func processHelp(
	atmosConfig schema.AtmosConfiguration,
	componentType string,
	command string,
) error {
	if len(command) == 0 {
		u.PrintMessage(fmt.Sprintf("Atmos supports all native '%s' commands.\n", componentType))
		u.PrintMessage("In addition, the 'component' argument and 'stack' flag are required to generate the variables and backend config for the component in the stack.\n")
		u.PrintMessage(fmt.Sprintf("atmos %s <command> <component> -s <stack> [options]", componentType))
		u.PrintMessage(fmt.Sprintf("atmos %s <command> <component> --stack <stack> [options]", componentType))
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
	} else {
		u.PrintMessage(fmt.Sprintf("\nAtmos supports native '%s' commands with all the options, arguments and flags.\n", componentType))
		u.PrintMessage("In addition, 'component' and 'stack' are required in order to generate variables for the component in the stack.\n")
		u.PrintMessage(fmt.Sprintf("atmos %s <subcommand> <component> -s <stack> [options]", componentType))
		u.PrintMessage(fmt.Sprintf("atmos %s <subcommand> <component> --stack <stack> [options]", componentType))
		u.PrintMessage(fmt.Sprintf("\nFor more details, execute '%s --help'\n", componentType))
	}

	return nil
}
